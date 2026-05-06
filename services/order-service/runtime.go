package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

var ErrUnsupportedPair = errors.New("unsupported pair")
var ErrRuntimeUnavailable = errors.New("matcher runtime unavailable")

const runtimeRecoveryTimeout = 3 * time.Second

type RuntimeStore interface {
	Create(ctx context.Context, userID string, req CreateOrderRequest) (*Order, error)
	Cancel(ctx context.Context, userID, id string) (*Order, error)
	PersistMatchResult(ctx context.Context, pair string, result MatchResult) (PersistedMatchResult, error)
	LoadRestorableOrders(ctx context.Context) ([]RestorableOrder, error)
}

type OrderRuntime struct {
	mu        sync.Mutex
	store     RuntimeStore
	publisher Publisher
	book      *OrderBook
	closed    bool
	closeErr  error
}

func NewOrderRuntime(ctx context.Context, store RuntimeStore, publisher Publisher) (*OrderRuntime, error) {
	if publisher == nil {
		publisher = NoopPublisher{}
	}
	r := &OrderRuntime{store: store, publisher: publisher}
	if err := r.rebuildLocked(ctx); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *OrderRuntime) SubmitOrder(ctx context.Context, userID string, req CreateOrderRequest) (*Order, error) {
	if req.Pair != "BTC-KRW" {
		return nil, ErrUnsupportedPair
	}
	price, err := ParsePriceKRWMinor(req.Price)
	if err != nil {
		return nil, err
	}
	qty, err := ParseQuantityScaled(req.Quantity)
	if err != nil {
		return nil, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil, fmt.Errorf("%w: %v", ErrRuntimeUnavailable, r.closeErr)
	}

	created, err := r.store.Create(ctx, userID, req)
	if err != nil {
		return nil, err
	}

	// order.created: always after the create insert succeeds.
	r.publishOrderEvent(ctx, SubjectOrderCreated, created)

	result, err := ProcessNewOrder(r.book, IncomingOrder{
		OrderID:           created.ID,
		Side:              created.Side,
		Price:             price,
		RemainingQuantity: qty,
	})
	if err != nil {
		return nil, fmt.Errorf("match order: %w", err)
	}

	persisted, err := PersistAndPublishMatchResult(ctx, r.store, r.publisher, req.Pair, result)
	if err != nil {
		if rebuildErr := r.recoverBookLocked(); rebuildErr != nil {
			r.failClosedLocked(fmt.Errorf("persist match result=%w; recovery rebuild failed=%v", err, rebuildErr))
			return nil, fmt.Errorf("%w: %v", ErrRuntimeUnavailable, r.closeErr)
		}
		return nil, fmt.Errorf("persist match result: %w", err)
	}

	for _, o := range persisted.UpdatedOrders {
		if o.ID == created.ID {
			return o, nil
		}
	}
	return created, nil
}

func (r *OrderRuntime) CancelOrder(ctx context.Context, userID, id string) (*Order, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil, fmt.Errorf("%w: %v", ErrRuntimeUnavailable, r.closeErr)
	}

	order, err := r.store.Cancel(ctx, userID, id)
	if err != nil {
		return nil, err
	}

	if _, ok := r.book.RemoveOrderByID(id); !ok {
		if rebuildErr := r.recoverBookLocked(); rebuildErr != nil {
			r.failClosedLocked(fmt.Errorf("cancel succeeded in db but book removal failed for %s; recovery rebuild failed=%v", id, rebuildErr))
			return nil, fmt.Errorf("%w: %v", ErrRuntimeUnavailable, r.closeErr)
		}
		slog.Warn("cancelled order was missing in runtime book; rebuilt from persistence", "order_id", id)
	}

	r.publishOrderEvent(ctx, SubjectOrderUpdated, order)
	return order, nil
}

func (r *OrderRuntime) recoverBookLocked() error {
	recoverCtx, cancel := context.WithTimeout(context.Background(), runtimeRecoveryTimeout)
	defer cancel()
	return r.rebuildLocked(recoverCtx)
}

func (r *OrderRuntime) failClosedLocked(err error) {
	r.closed = true
	r.closeErr = err
	slog.Error("matcher runtime entered fail-closed mode", "error", err)
}

func (r *OrderRuntime) rebuildLocked(ctx context.Context) error {
	restorable, err := r.store.LoadRestorableOrders(ctx)
	if err != nil {
		return fmt.Errorf("load restorable orders: %w", err)
	}
	book, err := RebuildOrderBookFromPersistedOrders(restorable)
	if err != nil {
		return fmt.Errorf("rebuild order book: %w", err)
	}
	r.book = book
	return nil
}

func (r *OrderRuntime) publishOrderEvent(ctx context.Context, subject string, order *Order) {
	event := NewOrderEvent(subject, order, time.Now())
	if err := r.publisher.PublishOrderEvent(ctx, event); err != nil {
		slog.Error("failed to publish order event",
			"subject", subject,
			"order_id", order.ID,
			"status", order.Status,
			"error", err,
		)
	}
}
