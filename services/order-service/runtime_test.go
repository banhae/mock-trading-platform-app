package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

type runtimeMemStore struct {
	orders map[string]*Order
	trades []Trade
	nextID int
	now    time.Time
}

func newRuntimeMemStore() *runtimeMemStore {
	return &runtimeMemStore{orders: map[string]*Order{}, now: time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC)}
}

func (s *runtimeMemStore) Create(_ context.Context, userID string, req CreateOrderRequest) (*Order, error) {
	s.nextID++
	id := "o-" + itoa(s.nextID)
	ts := s.now.Add(time.Duration(s.nextID) * time.Second)
	o := &Order{ID: id, UserID: userID, Pair: req.Pair, Side: req.Side, Quantity: req.Quantity, RemainingQuantity: req.Quantity, Price: req.Price, Status: StatusOpen, CreatedAt: ts, UpdatedAt: ts}
	s.orders[id] = o
	return cloneOrder(o), nil
}

func (s *runtimeMemStore) Cancel(_ context.Context, userID, id string) (*Order, error) {
	o, ok := s.orders[id]
	if !ok {
		return nil, ErrOrderNotFound
	}
	if o.UserID != userID {
		return nil, ErrOrderForbidden
	}
	if !IsCancellable(o.Status) {
		return nil, ErrOrderNotCancellable
	}
	o.Status = StatusCancelled
	o.UpdatedAt = s.now.Add(99 * time.Second)
	return cloneOrder(o), nil
}

func (s *runtimeMemStore) PersistMatchResult(_ context.Context, pair string, result MatchResult) (PersistedMatchResult, error) {
	if pair != "BTC-KRW" {
		return PersistedMatchResult{}, ErrUnsupportedPair
	}
	makerFilled := map[string]int64{}
	makerIDs := make([]string, 0)
	seen := map[string]struct{}{}
	for _, f := range result.Fills {
		makerFilled[f.MakerOrderID] += f.Quantity
		if _, ok := seen[f.MakerOrderID]; !ok {
			seen[f.MakerOrderID] = struct{}{}
			makerIDs = append(makerIDs, f.MakerOrderID)
		}
	}

	updated := make([]*Order, 0, len(makerIDs)+1)
	for _, makerID := range makerIDs {
		o, ok := s.orders[makerID]
		if !ok {
			return PersistedMatchResult{}, ErrOrderNotFound
		}
		remaining, _ := ParseQuantityScaled(o.RemainingQuantity)
		remaining -= makerFilled[makerID]
		o.RemainingQuantity = FormatQuantityScaled(remaining)
		if remaining == 0 {
			o.Status = StatusFilled
		} else {
			o.Status = StatusPartiallyFilled
		}
		updated = append(updated, cloneOrder(o))
	}

	taker, ok := s.orders[result.TakerOrderID]
	if !ok {
		return PersistedMatchResult{}, ErrOrderNotFound
	}
	taker.RemainingQuantity = FormatQuantityScaled(result.TakerRemainingQuantity)
	taker.Status = deriveTakerStatus(len(result.Fills), result.TakerRemainingQuantity)
	updated = append(updated, cloneOrder(taker))

	persistedTrades := make([]Trade, 0, len(result.Fills))
	for i, f := range result.Fills {
		tradeID := f.TradeID
		if tradeID == "" {
			tradeID = "trade-" + itoa(i+1)
		}
		tr := Trade{TradeID: tradeID, Pair: pair, Price: f.Price, Quantity: f.Quantity, MakerOrderID: f.MakerOrderID, TakerOrderID: f.TakerOrderID, ExecutedAt: s.now.Add(time.Duration(i+1) * time.Millisecond)}
		persistedTrades = append(persistedTrades, tr)
		s.trades = append(s.trades, tr)
	}

	return PersistedMatchResult{UpdatedOrders: updated, Trades: persistedTrades}, nil
}

func (s *runtimeMemStore) LoadRestorableOrders(_ context.Context) ([]RestorableOrder, error) {
	out := make([]RestorableOrder, 0)
	for _, o := range s.orders {
		if o.Pair != "BTC-KRW" {
			continue
		}
		if o.Status != StatusOpen && o.Status != StatusPartiallyFilled {
			continue
		}
		out = append(out, RestorableOrder{ID: o.ID, Pair: o.Pair, Side: o.Side, Price: o.Price, RemainingQuantity: o.RemainingQuantity, Status: o.Status, CreatedAt: o.CreatedAt})
	}
	return out, nil
}

func cloneOrder(o *Order) *Order {
	cp := *o
	return &cp
}

type seqPublisher struct{ seq []string }

func (p *seqPublisher) PublishOrderEvent(_ context.Context, event OrderEvent) error {
	p.seq = append(p.seq, event.Type+":"+event.Order.ID)
	return nil
}
func (p *seqPublisher) PublishTradeEvent(_ context.Context, event TradeEvent) error {
	p.seq = append(p.seq, event.Type+":"+event.Trade.TradeID)
	return nil
}

func TestRuntimeSubmitOrderOpenPartialFilledAndPublishOrdering(t *testing.T) {
	store := newRuntimeMemStore()
	pub := &seqPublisher{}
	r, err := NewOrderRuntime(context.Background(), store, pub)
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	maker1, _ := r.SubmitOrder(context.Background(), "maker", CreateOrderRequest{Pair: "BTC-KRW", Side: "sell", Quantity: "1", Price: "100"})
	if maker1.Status != StatusOpen {
		t.Fatalf("maker1 status=%s want=open", maker1.Status)
	}
	maker2, _ := r.SubmitOrder(context.Background(), "maker", CreateOrderRequest{Pair: "BTC-KRW", Side: "sell", Quantity: "1", Price: "100"})

	takerPartial, _ := r.SubmitOrder(context.Background(), "taker", CreateOrderRequest{Pair: "BTC-KRW", Side: "buy", Quantity: "1.5", Price: "100"})
	if takerPartial.Status != StatusFilled {
		t.Fatalf("taker should be filled, got %s", takerPartial.Status)
	}
	if store.orders[maker2.ID].Status != StatusPartiallyFilled {
		t.Fatalf("maker2 should be partially filled, got %s", store.orders[maker2.ID].Status)
	}
	if len(store.trades) != 2 || store.trades[0].MakerOrderID != maker1.ID || store.trades[1].MakerOrderID != maker2.ID {
		t.Fatalf("price-time priority broken: trades=%+v", store.trades)
	}

	last6 := pub.seq[len(pub.seq)-6:]
	if last6[0] != "order.created:"+takerPartial.ID {
		t.Fatalf("expected taker order.created first, got %v", last6)
	}
	if last6[1] != "order.updated:"+maker1.ID || last6[2] != "order.updated:"+maker2.ID || last6[3] != "order.updated:"+takerPartial.ID {
		t.Fatalf("unexpected order.updated ordering: %v", last6)
	}
	if len(last6[4]) < len("trade.executed:") || len(last6[5]) < len("trade.executed:") ||
		last6[4][:len("trade.executed:")] != "trade.executed:" || last6[5][:len("trade.executed:")] != "trade.executed:" {
		t.Fatalf("expected trade.executed after order.updated, got %v", last6)
	}
}

func TestRuntimeCancelLifecycleRules(t *testing.T) {
	store := newRuntimeMemStore()
	r, _ := NewOrderRuntime(context.Background(), store, NoopPublisher{})

	o, _ := r.SubmitOrder(context.Background(), "u1", CreateOrderRequest{Pair: "BTC-KRW", Side: "buy", Quantity: "1", Price: "90"})
	if _, err := r.CancelOrder(context.Background(), "u1", o.ID); err != nil {
		t.Fatalf("open cancel failed: %v", err)
	}
	if store.orders[o.ID].Status != StatusCancelled {
		t.Fatalf("expected cancelled")
	}

	// partially_filled cancel
	maker, _ := r.SubmitOrder(context.Background(), "maker", CreateOrderRequest{Pair: "BTC-KRW", Side: "sell", Quantity: "2", Price: "100"})
	_, _ = r.SubmitOrder(context.Background(), "taker", CreateOrderRequest{Pair: "BTC-KRW", Side: "buy", Quantity: "1", Price: "100"})
	if store.orders[maker.ID].Status != StatusPartiallyFilled {
		t.Fatalf("maker should be partial")
	}
	if _, err := r.CancelOrder(context.Background(), "maker", maker.ID); err != nil {
		t.Fatalf("partial cancel failed: %v", err)
	}

	filled, _ := r.SubmitOrder(context.Background(), "u2", CreateOrderRequest{Pair: "BTC-KRW", Side: "sell", Quantity: "1", Price: "120"})
	_, _ = r.SubmitOrder(context.Background(), "u3", CreateOrderRequest{Pair: "BTC-KRW", Side: "buy", Quantity: "1", Price: "120"})
	if _, err := r.CancelOrder(context.Background(), "u2", filled.ID); !errors.Is(err, ErrOrderNotCancellable) {
		t.Fatalf("filled cancel should fail with ErrOrderNotCancellable, got %v", err)
	}
	if _, err := r.CancelOrder(context.Background(), "u1", o.ID); !errors.Is(err, ErrOrderNotCancellable) {
		t.Fatalf("cancelled recancel should fail with ErrOrderNotCancellable, got %v", err)
	}
}

func TestRuntimeRebuildLoadsOnlyOpenAndPartiallyFilled(t *testing.T) {
	store := newRuntimeMemStore()
	store.orders["open-1"] = &Order{ID: "open-1", Pair: "BTC-KRW", Side: "buy", Price: "100", Quantity: "1", RemainingQuantity: "1", Status: StatusOpen, CreatedAt: time.Now().UTC()}
	store.orders["partial-1"] = &Order{ID: "partial-1", Pair: "BTC-KRW", Side: "sell", Price: "101", Quantity: "2", RemainingQuantity: "1", Status: StatusPartiallyFilled, CreatedAt: time.Now().UTC().Add(time.Second)}
	store.orders["filled-1"] = &Order{ID: "filled-1", Pair: "BTC-KRW", Side: "sell", Price: "99", Quantity: "1", RemainingQuantity: "0", Status: StatusFilled, CreatedAt: time.Now().UTC()}
	store.orders["cancel-1"] = &Order{ID: "cancel-1", Pair: "BTC-KRW", Side: "buy", Price: "98", Quantity: "1", RemainingQuantity: "1", Status: StatusCancelled, CreatedAt: time.Now().UTC()}

	r, err := NewOrderRuntime(context.Background(), store, NoopPublisher{})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	if got := len(r.book.SideSnapshot("buy")); got != 1 {
		t.Fatalf("expected 1 bid in rebuilt book, got %d", got)
	}
	if got := len(r.book.SideSnapshot("sell")); got != 1 {
		t.Fatalf("expected 1 ask in rebuilt book, got %d", got)
	}
}

type recoveryAwareStore struct {
	*runtimeMemStore
	persistErr                error
	lastRecoveryCtx           context.Context
	lastRecoveryHadReqMarker  bool
	failRecoveryLoadAfterInit bool
	loadCalls                 int
}

func (s *recoveryAwareStore) PersistMatchResult(_ context.Context, _ string, _ MatchResult) (PersistedMatchResult, error) {
	return PersistedMatchResult{}, s.persistErr
}

func (s *recoveryAwareStore) LoadRestorableOrders(ctx context.Context) ([]RestorableOrder, error) {
	s.loadCalls++
	s.lastRecoveryCtx = ctx
	s.lastRecoveryHadReqMarker = ctx.Value("req-marker") != nil
	if s.failRecoveryLoadAfterInit && s.loadCalls > 1 {
		return nil, errors.New("rebuild failed")
	}
	return s.runtimeMemStore.LoadRestorableOrders(ctx)
}

func TestRuntimeRecoveryUsesFreshBoundedContext(t *testing.T) {
	base := newRuntimeMemStore()
	store := &recoveryAwareStore{
		runtimeMemStore: base,
		persistErr:      errors.New("persist failed"),
	}
	r, err := NewOrderRuntime(context.Background(), store, NoopPublisher{})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), "req-marker", true))
	cancel() // 요청 컨텍스트가 이미 취소된 상황을 강제

	_, err = r.SubmitOrder(ctx, "u1", CreateOrderRequest{
		Pair: "BTC-KRW", Side: "buy", Quantity: "1", Price: "100",
	})
	if err == nil {
		t.Fatalf("expected submit error")
	}
	if store.lastRecoveryCtx == nil {
		t.Fatalf("expected recovery to invoke LoadRestorableOrders")
	}
	if store.lastRecoveryHadReqMarker {
		t.Fatalf("recovery must not reuse request context")
	}
}

func TestRuntimeFailClosedWhenRecoveryFails(t *testing.T) {
	store := &recoveryAwareStore{
		runtimeMemStore:           newRuntimeMemStore(),
		persistErr:                errors.New("persist failed"),
		failRecoveryLoadAfterInit: true,
	}
	r, err := NewOrderRuntime(context.Background(), store, NoopPublisher{})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	_, err = r.SubmitOrder(context.Background(), "u1", CreateOrderRequest{
		Pair: "BTC-KRW", Side: "buy", Quantity: "1", Price: "100",
	})
	if !errors.Is(err, ErrRuntimeUnavailable) {
		t.Fatalf("expected ErrRuntimeUnavailable on fail-closed path, got %v", err)
	}

	_, err = r.SubmitOrder(context.Background(), "u1", CreateOrderRequest{
		Pair: "BTC-KRW", Side: "buy", Quantity: "1", Price: "100",
	})
	if !errors.Is(err, ErrRuntimeUnavailable) {
		t.Fatalf("runtime should stay fail-closed, got %v", err)
	}
}
