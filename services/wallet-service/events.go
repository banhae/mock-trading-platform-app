package main

import "time"

// Event subject constants for the order domain.
//
// NOTE: these subjects and the OrderEvent envelope below MUST stay in lock
// step with services/order-service/events.go (the publisher side) and
// services/marketdata-service/events.go (the other consumer). We
// intentionally avoid a shared Go module here because the envelope is tiny
// and stable, and the repo layout keeps each service self-contained with
// its own Dockerfile build context.
//
// If you change a field name or add a new subject, update all three places
// in the same PR.
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
