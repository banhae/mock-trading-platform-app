package main

import "time"

// Event subject constants for the order domain.
//
// NOTE: these subjects and parser-side envelopes below MUST stay in lock
// step with services/order-service/events.go (publisher side) and
// services/wallet-service/events.go (other consumer).
const (
	SubjectOrderCreated  = "order.created"
	SubjectOrderUpdated  = "order.updated"
	SubjectTradeExecuted = "trade.executed"
)

// OrderEvent is the parser-side mirror of the publisher envelope.
type OrderEvent struct {
	Type       string       `json:"type"`
	Version    int          `json:"version"`
	OccurredAt time.Time    `json:"occurred_at"`
	Order      OrderPayload `json:"order"`
}

// TradeEvent is the parser-side mirror of trade.executed envelope.
type TradeEvent struct {
	Type       string       `json:"type"`
	Version    int          `json:"version"`
	OccurredAt time.Time    `json:"occurred_at"`
	Trade      TradePayload `json:"trade"`
}

// TradePayload matches the publisher-side struct exactly.
type TradePayload struct {
	TradeID      string    `json:"trade_id"`
	Pair         string    `json:"pair"`
	Price        string    `json:"price"`
	Quantity     string    `json:"quantity"`
	MakerOrderID string    `json:"maker_order_id"`
	TakerOrderID string    `json:"taker_order_id"`
	ExecutedAt   time.Time `json:"executed_at"`
}

// OrderPayload matches the publisher-side struct exactly.
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
