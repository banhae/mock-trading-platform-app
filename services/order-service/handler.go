package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"
)

// Handler holds HTTP handlers for the order-service.
type Handler struct {
	store     OrderStore
	publisher Publisher
	runtime   *OrderRuntime
}

// NewHandler creates a Handler with the given store and event publisher.
//
// publisher must be non-nil. Use NoopPublisher{} when the event bus is
// disabled — the handler never branches on publisher == nil to keep the
// hot path simple and explicit.
func NewHandler(store OrderStore, publisher Publisher) *Handler {
	if publisher == nil {
		publisher = NoopPublisher{}
	}
	return &Handler{store: store, publisher: publisher}
}

func NewHandlerWithRuntime(store OrderStore, runtime *OrderRuntime, publisher Publisher) *Handler {
	if publisher == nil {
		publisher = NoopPublisher{}
	}
	return &Handler{store: store, runtime: runtime, publisher: publisher}
}

// publishOrderEvent emits an order lifecycle event on a best-effort basis.
//
// Phase 2 policy — explicit and consistent:
//   - The database write is authoritative and has already succeeded.
//   - Publish failures are logged but do NOT fail the HTTP response. The
//     client's request is considered accepted the moment the row is
//     persisted.
//   - There is no retry, no outbox, and no exactly-once — those belong to
//     later phases if and when we find we actually need them.
//
// This behavior is tested in publisher_test.go.
func (h *Handler) publishOrderEvent(ctx context.Context, subject string, order *Order) {
	event := NewOrderEvent(subject, order, time.Now())
	if err := h.publisher.PublishOrderEvent(ctx, event); err != nil {
		slog.Error("failed to publish order event",
			"subject", subject,
			"order_id", order.ID,
			"status", order.Status,
			"error", err,
		)
		return
	}
	slog.Info("published order event",
		"subject", subject,
		"order_id", order.ID,
		"status", order.Status,
	)
}

// Health is the liveness probe endpoint.
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Ready is the readiness probe endpoint. Returns 503 if DB is unreachable.
func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	if err := h.store.Ping(r.Context()); err != nil {
		slog.Error("readiness check failed", "error", err)
		http.Error(w, `{"error":"not ready"}`, http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

// currentUserID extracts the authenticated user id from the request context.
// middleware 가 반드시 주입한다. 비어 있으면 401.
func currentUserID(r *http.Request) (string, bool) {
	v, ok := r.Context().Value(userIDKey).(string)
	if !ok || v == "" {
		return "", false
	}
	return v, true
}

// CreateOrder handles POST /orders.
func (h *Handler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	userID, ok := currentUserID(r)
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var req CreateOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.Pair == "" || req.Side == "" || req.Quantity == "" || req.Price == "" {
		http.Error(w, `{"error":"missing required fields: pair, side, quantity, price"}`, http.StatusBadRequest)
		return
	}
	if req.Pair != "BTC-KRW" {
		http.Error(w, `{"error":"pair must be BTC-KRW"}`, http.StatusBadRequest)
		return
	}
	if req.Side != "buy" && req.Side != "sell" {
		http.Error(w, `{"error":"side must be buy or sell"}`, http.StatusBadRequest)
		return
	}

	if h.runtime != nil {
		order, err := h.runtime.SubmitOrder(r.Context(), userID, req)
		if err != nil {
			switch {
			case errors.Is(err, ErrUnsupportedPair),
				errors.Is(err, ErrInvalidPriceFormat),
				errors.Is(err, ErrInvalidQuantityFormat),
				errors.Is(err, ErrNonPositiveValue):
				http.Error(w, `{"error":"invalid order values"}`, http.StatusBadRequest)
				return
			case errors.Is(err, ErrRuntimeUnavailable):
				http.Error(w, `{"error":"matcher runtime unavailable"}`, http.StatusServiceUnavailable)
				return
			}
			slog.Error("failed to submit order via runtime", "error", err)
			http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusCreated, order)
		return
	}

	order, err := h.store.Create(r.Context(), userID, req)
	if err != nil {
		slog.Error("failed to create order", "error", err)
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	// Phase 2: publish AFTER persistence. DB success is authoritative;
	// publish failure is logged but does not fail the HTTP response.
	h.publishOrderEvent(r.Context(), SubjectOrderCreated, order)

	writeJSON(w, http.StatusCreated, order)
}

// GetOrder handles GET /orders/{id}.
//
// 현재 사용자 scoping: 다른 사용자의 주문 id 를 알고 있어도 접근할 수 없다.
// 404 로 통일하여 enumerate 을 방지한다.
func (h *Handler) GetOrder(w http.ResponseWriter, r *http.Request) {
	userID, ok := currentUserID(r)
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	id := r.PathValue("id")
	if id == "" {
		http.Error(w, `{"error":"missing order id"}`, http.StatusBadRequest)
		return
	}

	order, err := h.store.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrOrderNotFound) {
			http.Error(w, `{"error":"order not found"}`, http.StatusNotFound)
			return
		}
		slog.Error("failed to get order", "error", err)
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}
	if order.UserID != userID {
		// 존재 자체를 노출하지 않기 위해 404 로 응답.
		http.Error(w, `{"error":"order not found"}`, http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, order)
}

// ListOrders handles GET /orders?status=&limit=.
//
// 반드시 현재 로그인 사용자의 주문만 반환한다. user query param 은 받지 않는다.
func (h *Handler) ListOrders(w http.ResponseWriter, r *http.Request) {
	userID, ok := currentUserID(r)
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	q := r.URL.Query()
	status := q.Get("status")
	if status != "" && !IsValidStatus(status) {
		http.Error(w, `{"error":"invalid status filter"}`, http.StatusBadRequest)
		return
	}

	limit := 0
	if s := q.Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 0 {
			http.Error(w, `{"error":"invalid limit"}`, http.StatusBadRequest)
			return
		}
		limit = n
	}

	orders, err := h.store.List(r.Context(), ListOrdersParams{
		UserID: userID,
		Status: status,
		Limit:  limit,
	})
	if err != nil {
		slog.Error("failed to list orders", "error", err)
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"orders": orders,
	})
}

// CancelOrder handles DELETE /orders/{id}.
//
// 상태 전이 규칙:
//   - open / partially_filled -> cancelled (성공)
//   - filled / cancelled       -> 409 Conflict
//   - 타 사용자 소유           -> 404 (존재 감춤)
//   - 없는 주문                -> 404
func (h *Handler) CancelOrder(w http.ResponseWriter, r *http.Request) {
	userID, ok := currentUserID(r)
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	id := r.PathValue("id")
	if id == "" {
		http.Error(w, `{"error":"missing order id"}`, http.StatusBadRequest)
		return
	}

	if h.runtime != nil {
		order, err := h.runtime.CancelOrder(r.Context(), userID, id)
		if err != nil {
			switch {
			case errors.Is(err, ErrOrderNotFound), errors.Is(err, ErrOrderForbidden):
				http.Error(w, `{"error":"order not found"}`, http.StatusNotFound)
				return
			case errors.Is(err, ErrOrderNotCancellable):
				http.Error(w, `{"error":"order is not in a cancellable state"}`, http.StatusConflict)
				return
			case errors.Is(err, ErrRuntimeUnavailable):
				http.Error(w, `{"error":"matcher runtime unavailable"}`, http.StatusServiceUnavailable)
				return
			}
			slog.Error("failed to cancel order via runtime", "error", err)
			http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, order)
		return
	}

	order, err := h.store.Cancel(r.Context(), userID, id)
	if err != nil {
		switch {
		case errors.Is(err, ErrOrderNotFound), errors.Is(err, ErrOrderForbidden):
			http.Error(w, `{"error":"order not found"}`, http.StatusNotFound)
			return
		case errors.Is(err, ErrOrderNotCancellable):
			http.Error(w, `{"error":"order is not in a cancellable state"}`, http.StatusConflict)
			return
		}
		slog.Error("failed to cancel order", "error", err)
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	// Phase 2: publish AFTER persistence. Same best-effort policy as create.
	h.publishOrderEvent(r.Context(), SubjectOrderUpdated, order)

	writeJSON(w, http.StatusOK, order)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
