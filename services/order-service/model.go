package main

import (
	"errors"
	"time"
)

// Order status enum. 문자열 기반으로 Phase 1 수준의 lifecycle만 표현한다.
// 실제 matching / partial fill 은 Phase 3 에서 도입된다.
const (
	StatusOpen            = "open"
	StatusPartiallyFilled = "partially_filled"
	StatusFilled          = "filled"
	StatusCancelled       = "cancelled"
)

// IsValidStatus 는 status 문자열이 알려진 값인지 확인한다.
func IsValidStatus(s string) bool {
	switch s {
	case StatusOpen, StatusPartiallyFilled, StatusFilled, StatusCancelled:
		return true
	}
	return false
}

// IsCancellable 은 주어진 status 가 cancel 가능한 상태인지 판별한다.
// Phase 1 기준: open / partially_filled 만 취소 가능.
// filled / cancelled 는 종결 상태로 간주한다.
func IsCancellable(s string) bool {
	return s == StatusOpen || s == StatusPartiallyFilled
}

// Store-level sentinel errors. Handler 가 HTTP status 로 매핑한다.
var (
	ErrOrderNotFound       = errors.New("order not found")
	ErrOrderForbidden      = errors.New("order does not belong to current user")
	ErrOrderNotCancellable = errors.New("order is not in a cancellable state")
)

// Order represents a trade order in the exchange system.
//
// 숫자 필드 (Quantity, Price, RemainingQuantity) 는 decimal string 으로 유지한다.
// float 로 변환하지 않는다. 이는 matcher / aggregation 이 붙는 Phase 3+ 에서도
// 그대로 유지되어야 하는 규칙이다.
type Order struct {
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

// Trade is the minimum persisted execution record for Phase 3.
type Trade struct {
	TradeID      string    `json:"trade_id"`
	Pair         string    `json:"pair"`
	Price        int64     `json:"price"`
	Quantity     int64     `json:"quantity"`
	MakerOrderID string    `json:"maker_order_id"`
	TakerOrderID string    `json:"taker_order_id"`
	ExecutedAt   time.Time `json:"executed_at"`
}

// CreateOrderRequest is the JSON body for POST /orders.
type CreateOrderRequest struct {
	Pair     string `json:"pair"`
	Side     string `json:"side"`
	Quantity string `json:"quantity"`
	Price    string `json:"price"`
}

// ListOrdersParams is the filter for listing the current user's orders.
// user 필드는 public API 로 노출되지 않는다. handler 가 context 에서 주입한다.
type ListOrdersParams struct {
	UserID string
	Status string // empty => no status filter
	Limit  int    // <= 0 means default
}
