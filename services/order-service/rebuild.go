package main

import (
	"context"
	"fmt"
	"time"
)

// RestorableOrder is the minimum persisted order shape required to rebuild
// the in-memory order book on startup.
type RestorableOrder struct {
	ID                string
	Pair              string
	Side              string
	Price             string
	RemainingQuantity string
	Status            string
	CreatedAt         time.Time
}

// LoadRestorableOrders returns persisted resting orders for deterministic book rebuild.
//
// Slice G constraints:
//   - single pair only: BTC-KRW
//   - status in (open, partially_filled)
//   - deterministic ordering: created_at ASC, id ASC
func (s *PostgresStore) LoadRestorableOrders(ctx context.Context) ([]RestorableOrder, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, pair, side, price, remaining_quantity, status, created_at
		 FROM orders
		 WHERE pair = $1
		   AND status IN ($2, $3)
		 ORDER BY created_at ASC, id ASC`,
		"BTC-KRW", StatusOpen, StatusPartiallyFilled,
	)
	if err != nil {
		return nil, fmt.Errorf("query restorable orders: %w", err)
	}
	defer rows.Close()

	out := make([]RestorableOrder, 0)
	for rows.Next() {
		var r RestorableOrder
		if err := rows.Scan(&r.ID, &r.Pair, &r.Side, &r.Price, &r.RemainingQuantity, &r.Status, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan restorable order: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate restorable orders: %w", err)
	}
	return out, nil
}

// RebuildOrderBookFromPersistedOrders deterministically reconstructs the
// in-memory order book from persisted resting orders.
func RebuildOrderBookFromPersistedOrders(restorable []RestorableOrder) (*OrderBook, error) {
	book := NewOrderBook()
	for _, row := range restorable {
		if row.Pair != "BTC-KRW" {
			return nil, fmt.Errorf("order %s: unsupported pair %s", row.ID, row.Pair)
		}
		if row.Status != StatusOpen && row.Status != StatusPartiallyFilled {
			return nil, fmt.Errorf("order %s: unsupported status %s", row.ID, row.Status)
		}

		price, err := ParsePriceKRWMinor(row.Price)
		if err != nil {
			return nil, fmt.Errorf("order %s: parse price: %w", row.ID, err)
		}
		remaining, err := ParseQuantityScaled(row.RemainingQuantity)
		if err != nil {
			return nil, fmt.Errorf("order %s: parse remaining quantity: %w", row.ID, err)
		}

		if _, err := book.AppendRestingOrder(BookOrder{
			OrderID:           row.ID,
			Side:              row.Side,
			Price:             price,
			RemainingQuantity: remaining,
		}); err != nil {
			return nil, fmt.Errorf("order %s: append resting order: %w", row.ID, err)
		}
	}
	return book, nil
}
