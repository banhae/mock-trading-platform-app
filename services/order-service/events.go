package main

import "time"

// Event subject constants for the order domain.
//
// NOTE: these subjects are part of the public wire contract shared with
// marketdata-service and wallet-service. Any change here must be mirrored in
// those services (they define their own parser-side copy to avoid a heavy
// shared Go module just for a tiny envelope).
//
// The contract for this phase is deliberately narrow:
//   - order.created : emitted after an order is persisted
//   - order.updated : emitted after an order's status transitions
//     (for Phase 1 code this mainly means cancellation)
//   - trade.executed: emitted after trade rows are persisted
const (
	SubjectOrderCreated  = "order.created"
	SubjectOrderUpdated  = "order.updated"
	SubjectTradeExecuted = "trade.executed"
)

// eventEnvelopeVersion is the schema version for OrderEvent.
// Consumers should tolerate newer minor versions but treat unknown major
// versions as a reason to refuse / log and skip.
const eventEnvelopeVersion = 1

// OrderEvent is the wire envelope published on the order bus.
//
// Keep this struct small, explicit and versioned. Do not inline business
// logic. Numeric fields are decimal strings — never float (CLAUDE.md rule).
type OrderEvent struct {
	Type       string       `json:"type"`
	Version    int          `json:"version"`
	OccurredAt time.Time    `json:"occurred_at"`
	Order      OrderPayload `json:"order"`
}

// TradeEvent is the wire envelope for execution events.
//
// Numeric fields are decimal strings for external contracts.
type TradeEvent struct {
	Type       string       `json:"type"`
	Version    int          `json:"version"`
	OccurredAt time.Time    `json:"occurred_at"`
	Trade      TradePayload `json:"trade"`
}

// TradePayload mirrors the persisted trade row shape that consumers need.
type TradePayload struct {
	TradeID      string    `json:"trade_id"`
	Pair         string    `json:"pair"`
	Price        string    `json:"price"`
	Quantity     string    `json:"quantity"`
	MakerOrderID string    `json:"maker_order_id"`
	TakerOrderID string    `json:"taker_order_id"`
	ExecutedAt   time.Time `json:"executed_at"`
}

// OrderPayload mirrors the subset of the order domain model that we expose
// to consumers. It intentionally excludes anything that is not already stable
// in Phase 1.
type OrderPayload struct {
	ID                string    `json:"id"`
	UserID            string    `json:"user_id"`
	Pair              string    `json:"pair"`
	Side              string    `json:"side"`
	Quantity          string    `json:"quantity"`
	RemainingQuantity string    `json:"remaining_quantity"`
	Price             string    `json:"price"`
	Status            string    `json:"status"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// NewOrderEvent builds a publishable envelope for the given order.
// The caller chooses the subject (order.created / order.updated).
func NewOrderEvent(subject string, o *Order, now time.Time) OrderEvent {
	return OrderEvent{
		Type:       subject,
		Version:    eventEnvelopeVersion,
		OccurredAt: now.UTC(),
		Order: OrderPayload{
			ID:                o.ID,
			UserID:            o.UserID,
			Pair:              o.Pair,
			Side:              o.Side,
			Quantity:          o.Quantity,
			RemainingQuantity: o.RemainingQuantity,
			Price:             o.Price,
			Status:            o.Status,
			CreatedAt:         o.CreatedAt,
			UpdatedAt:         o.UpdatedAt,
		},
	}
}

// NewTradeExecutedEvent builds a publishable envelope for one persisted trade.
func NewTradeExecutedEvent(t Trade, now time.Time) TradeEvent {
	return TradeEvent{
		Type:       SubjectTradeExecuted,
		Version:    eventEnvelopeVersion,
		OccurredAt: now.UTC(),
		Trade: TradePayload{
			TradeID:      t.TradeID,
			Pair:         t.Pair,
			Price:        FormatPriceKRWMinor(t.Price),
			Quantity:     FormatQuantityScaled(t.Quantity),
			MakerOrderID: t.MakerOrderID,
			TakerOrderID: t.TakerOrderID,
			ExecutedAt:   t.ExecutedAt,
		},
	}
}
