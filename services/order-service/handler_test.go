package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// mockStore implements OrderStore for testing without a database.
//
// 결정적 테스트를 위해 id 와 시간은 호출 순서대로 결정적으로 생성한다.
type mockStore struct {
	orders []*Order
	now    time.Time
	nextID int
}

func newMockStore() *mockStore {
	return &mockStore{
		now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

func (m *mockStore) Create(_ context.Context, userID string, req CreateOrderRequest) (*Order, error) {
	m.nextID++
	id := "test-order-" + itoa(m.nextID)
	ts := m.now.Add(time.Duration(m.nextID) * time.Second)
	order := &Order{
		ID:                id,
		UserID:            userID,
		Pair:              req.Pair,
		Side:              req.Side,
		Quantity:          req.Quantity,
		RemainingQuantity: req.Quantity,
		Price:             req.Price,
		Status:            StatusOpen,
		CreatedAt:         ts,
		UpdatedAt:         ts,
	}
	m.orders = append(m.orders, order)
	return order, nil
}

func (m *mockStore) GetByID(_ context.Context, id string) (*Order, error) {
	for _, o := range m.orders {
		if o.ID == id {
			return o, nil
		}
	}
	return nil, ErrOrderNotFound
}

func (m *mockStore) List(_ context.Context, params ListOrdersParams) ([]*Order, error) {
	limit := params.Limit
	if limit <= 0 {
		limit = defaultListLimit
	}
	out := make([]*Order, 0)
	// created_at DESC 근사: 입력 순서를 뒤에서 앞으로 읽는다.
	for i := len(m.orders) - 1; i >= 0; i-- {
		o := m.orders[i]
		if o.UserID != params.UserID {
			continue
		}
		if params.Status != "" && o.Status != params.Status {
			continue
		}
		out = append(out, o)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (m *mockStore) Cancel(_ context.Context, userID, id string) (*Order, error) {
	for _, o := range m.orders {
		if o.ID != id {
			continue
		}
		if o.UserID != userID {
			return nil, ErrOrderForbidden
		}
		if !IsCancellable(o.Status) {
			return nil, ErrOrderNotCancellable
		}
		o.Status = StatusCancelled
		o.UpdatedAt = m.now.Add(time.Hour)
		return o, nil
	}
	return nil, ErrOrderNotFound
}

func (m *mockStore) Ping(_ context.Context) error {
	return nil
}

// itoa avoids pulling strconv into test-only logic for a simple counter.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := make([]byte, 0, 4)
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	return string(buf)
}

// ---- helpers ---------------------------------------------------------------

func withUser(r *http.Request, userID string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), userIDKey, userID))
}

func decode[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.NewDecoder(w.Body).Decode(&v); err != nil {
		t.Fatalf("decode response: %v (body=%s)", err, w.Body.String())
	}
	return v
}

func seedOrder(m *mockStore, userID, pair string) *Order {
	o, _ := m.Create(context.Background(), userID, CreateOrderRequest{
		Pair: pair, Side: "buy", Quantity: "0.5", Price: "50000",
	})
	return o
}

// ---- health / ready --------------------------------------------------------

func TestHealth(t *testing.T) {
	h := NewHandler(newMockStore(), NoopPublisher{})
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	h.Health(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestReady(t *testing.T) {
	h := NewHandler(newMockStore(), NoopPublisher{})
	req := httptest.NewRequest("GET", "/ready", nil)
	w := httptest.NewRecorder()
	h.Ready(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

// ---- create ----------------------------------------------------------------

func TestCreateOrder(t *testing.T) {
	h := NewHandler(newMockStore(), NoopPublisher{})

	body := CreateOrderRequest{
		Pair:     "BTC-KRW",
		Side:     "buy",
		Quantity: "0.5",
		Price:    "50000",
	}
	b, _ := json.Marshal(body)

	req := withUser(httptest.NewRequest("POST", "/orders", bytes.NewReader(b)), "test-user")
	w := httptest.NewRecorder()
	h.CreateOrder(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", w.Code, w.Body.String())
	}

	order := decode[Order](t, w)
	if order.Pair != "BTC-KRW" {
		t.Errorf("expected pair BTC-KRW, got %s", order.Pair)
	}
	if order.Side != "buy" {
		t.Errorf("expected side buy, got %s", order.Side)
	}
	if order.UserID != "test-user" {
		t.Errorf("expected user_id test-user, got %s", order.UserID)
	}
	if order.Status != StatusOpen {
		t.Errorf("expected status open, got %s", order.Status)
	}
	if order.RemainingQuantity != "0.5" {
		t.Errorf("expected remaining_quantity 0.5, got %s", order.RemainingQuantity)
	}
	if order.UpdatedAt.IsZero() {
		t.Errorf("expected updated_at to be set")
	}
}

func TestCreateOrderMissingFields(t *testing.T) {
	h := NewHandler(newMockStore(), NoopPublisher{})

	body := CreateOrderRequest{Pair: "BTC-KRW"}
	b, _ := json.Marshal(body)

	req := withUser(httptest.NewRequest("POST", "/orders", bytes.NewReader(b)), "test-user")
	w := httptest.NewRecorder()
	h.CreateOrder(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestCreateOrderInvalidSide(t *testing.T) {
	h := NewHandler(newMockStore(), NoopPublisher{})

	body := CreateOrderRequest{
		Pair:     "BTC-KRW",
		Side:     "hold",
		Quantity: "1",
		Price:    "50000",
	}
	b, _ := json.Marshal(body)

	req := withUser(httptest.NewRequest("POST", "/orders", bytes.NewReader(b)), "test-user")
	w := httptest.NewRecorder()
	h.CreateOrder(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestCreateOrderUnauthorized(t *testing.T) {
	h := NewHandler(newMockStore(), NoopPublisher{})

	body := CreateOrderRequest{
		Pair: "BTC-KRW", Side: "buy", Quantity: "0.5", Price: "50000",
	}
	b, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/orders", bytes.NewReader(b))
	w := httptest.NewRecorder()
	h.CreateOrder(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", w.Code)
	}
}

// ---- get -------------------------------------------------------------------

func TestGetOrder(t *testing.T) {
	store := newMockStore()
	created := seedOrder(store, "test-user", "BTC-KRW")

	h := NewHandler(store, NoopPublisher{})
	req := withUser(httptest.NewRequest("GET", "/orders/"+created.ID, nil), "test-user")
	req.SetPathValue("id", created.ID)
	w := httptest.NewRecorder()
	h.GetOrder(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	order := decode[Order](t, w)
	if order.ID != created.ID {
		t.Errorf("expected id %s, got %s", created.ID, order.ID)
	}
}

func TestGetOrderNotFound(t *testing.T) {
	h := NewHandler(newMockStore(), NoopPublisher{})

	req := withUser(httptest.NewRequest("GET", "/orders/nonexistent", nil), "test-user")
	req.SetPathValue("id", "nonexistent")
	w := httptest.NewRecorder()
	h.GetOrder(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestGetOrderOtherUserReturns404(t *testing.T) {
	store := newMockStore()
	created := seedOrder(store, "owner-user", "BTC-KRW")

	h := NewHandler(store, NoopPublisher{})
	req := withUser(httptest.NewRequest("GET", "/orders/"+created.ID, nil), "intruder-user")
	req.SetPathValue("id", created.ID)
	w := httptest.NewRecorder()
	h.GetOrder(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for foreign order, got %d", w.Code)
	}
}

// ---- list ------------------------------------------------------------------

type listResponse struct {
	Orders []*Order `json:"orders"`
}

func TestListOrdersReturnsOnlyCurrentUserOrders(t *testing.T) {
	store := newMockStore()
	seedOrder(store, "alice", "BTC-KRW")
	seedOrder(store, "bob", "BTC-KRW")
	seedOrder(store, "alice", "BTC-KRW")

	h := NewHandler(store, NoopPublisher{})
	req := withUser(httptest.NewRequest("GET", "/orders", nil), "alice")
	w := httptest.NewRecorder()
	h.ListOrders(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	res := decode[listResponse](t, w)
	if len(res.Orders) != 2 {
		t.Fatalf("expected 2 orders for alice, got %d", len(res.Orders))
	}
	for _, o := range res.Orders {
		if o.UserID != "alice" {
			t.Errorf("leaked foreign order: %+v", o)
		}
	}
}

func TestListOrdersStatusFilter(t *testing.T) {
	store := newMockStore()
	a := seedOrder(store, "alice", "BTC-KRW")
	seedOrder(store, "alice", "BTC-KRW")
	// 하나는 미리 cancel.
	if _, err := store.Cancel(context.Background(), "alice", a.ID); err != nil {
		t.Fatalf("seed cancel: %v", err)
	}

	h := NewHandler(store, NoopPublisher{})
	req := withUser(httptest.NewRequest("GET", "/orders?status=open", nil), "alice")
	w := httptest.NewRecorder()
	h.ListOrders(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	res := decode[listResponse](t, w)
	if len(res.Orders) != 1 {
		t.Fatalf("expected 1 open order, got %d", len(res.Orders))
	}
	if res.Orders[0].Status != StatusOpen {
		t.Errorf("expected open, got %s", res.Orders[0].Status)
	}
}

func TestListOrdersInvalidStatusRejected(t *testing.T) {
	h := NewHandler(newMockStore(), NoopPublisher{})
	req := withUser(httptest.NewRequest("GET", "/orders?status=weird", nil), "alice")
	w := httptest.NewRecorder()
	h.ListOrders(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestListOrdersInvalidLimitRejected(t *testing.T) {
	h := NewHandler(newMockStore(), NoopPublisher{})
	req := withUser(httptest.NewRequest("GET", "/orders?limit=abc", nil), "alice")
	w := httptest.NewRecorder()
	h.ListOrders(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestListOrdersUnauthorized(t *testing.T) {
	h := NewHandler(newMockStore(), NoopPublisher{})
	req := httptest.NewRequest("GET", "/orders", nil)
	w := httptest.NewRecorder()
	h.ListOrders(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// ---- cancel ----------------------------------------------------------------

func TestCancelOrderTransitionsToCancelled(t *testing.T) {
	store := newMockStore()
	created := seedOrder(store, "alice", "BTC-KRW")

	h := NewHandler(store, NoopPublisher{})
	req := withUser(httptest.NewRequest("DELETE", "/orders/"+created.ID, nil), "alice")
	req.SetPathValue("id", created.ID)
	w := httptest.NewRecorder()
	h.CancelOrder(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	order := decode[Order](t, w)
	if order.Status != StatusCancelled {
		t.Errorf("expected cancelled, got %s", order.Status)
	}
	if !order.UpdatedAt.After(created.CreatedAt) {
		t.Errorf("expected updated_at to advance")
	}
}

func TestCancelOrderAlreadyCancelledReturns409(t *testing.T) {
	store := newMockStore()
	created := seedOrder(store, "alice", "BTC-KRW")
	if _, err := store.Cancel(context.Background(), "alice", created.ID); err != nil {
		t.Fatalf("seed cancel: %v", err)
	}

	h := NewHandler(store, NoopPublisher{})
	req := withUser(httptest.NewRequest("DELETE", "/orders/"+created.ID, nil), "alice")
	req.SetPathValue("id", created.ID)
	w := httptest.NewRecorder()
	h.CancelOrder(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}
}

func TestCancelOrderFilledReturns409(t *testing.T) {
	store := newMockStore()
	created := seedOrder(store, "alice", "BTC-KRW")
	// matcher 가 없는 phase 이므로 테스트 목적상 강제 상태 변경.
	store.orders[0].Status = StatusFilled

	h := NewHandler(store, NoopPublisher{})
	req := withUser(httptest.NewRequest("DELETE", "/orders/"+created.ID, nil), "alice")
	req.SetPathValue("id", created.ID)
	w := httptest.NewRecorder()
	h.CancelOrder(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409 for filled order, got %d", w.Code)
	}
}

func TestCancelOrderOtherUserReturns404(t *testing.T) {
	store := newMockStore()
	created := seedOrder(store, "alice", "BTC-KRW")

	h := NewHandler(store, NoopPublisher{})
	req := withUser(httptest.NewRequest("DELETE", "/orders/"+created.ID, nil), "mallory")
	req.SetPathValue("id", created.ID)
	w := httptest.NewRecorder()
	h.CancelOrder(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for foreign order cancel, got %d", w.Code)
	}
	// 원 주문은 그대로 open 이어야 한다.
	if store.orders[0].Status != StatusOpen {
		t.Errorf("foreign cancel mutated order: %s", store.orders[0].Status)
	}
}

func TestCancelOrderNotFound(t *testing.T) {
	h := NewHandler(newMockStore(), NoopPublisher{})
	req := withUser(httptest.NewRequest("DELETE", "/orders/missing", nil), "alice")
	req.SetPathValue("id", "missing")
	w := httptest.NewRecorder()
	h.CancelOrder(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestCancelOrderUnauthorized(t *testing.T) {
	h := NewHandler(newMockStore(), NoopPublisher{})
	req := httptest.NewRequest("DELETE", "/orders/any", nil)
	req.SetPathValue("id", "any")
	w := httptest.NewRecorder()
	h.CancelOrder(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}
