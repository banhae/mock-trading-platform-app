package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// fakePublisher captures published events for assertions.
//
// It also supports injecting a failure via `err` to verify the
// documented best-effort policy: publish errors are logged but do NOT
// fail the HTTP response.
type fakePublisher struct {
	mu     sync.Mutex
	events []OrderEvent
	trades []TradeEvent
	err    error
}

func (p *fakePublisher) PublishOrderEvent(_ context.Context, event OrderEvent) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, event)
	return p.err
}

func (p *fakePublisher) PublishTradeEvent(_ context.Context, event TradeEvent) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.trades = append(p.trades, event)
	return p.err
}

func (p *fakePublisher) snapshot() []OrderEvent {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]OrderEvent, len(p.events))
	copy(out, p.events)
	return out
}

// ---- create --------------------------------------------------------------

func TestCreateOrderPublishesOrderCreated(t *testing.T) {
	store := newMockStore()
	pub := &fakePublisher{}
	h := NewHandler(store, pub)

	body, _ := json.Marshal(CreateOrderRequest{
		Pair:     "BTC-KRW",
		Side:     "buy",
		Quantity: "0.5",
		Price:    "50000",
	})
	req := withUser(httptest.NewRequest("POST", "/orders", bytes.NewReader(body)), "alice")
	w := httptest.NewRecorder()
	h.CreateOrder(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	events := pub.snapshot()
	if len(events) != 1 {
		t.Fatalf("expected 1 published event, got %d", len(events))
	}
	ev := events[0]
	if ev.Type != SubjectOrderCreated {
		t.Errorf("expected type %s, got %s", SubjectOrderCreated, ev.Type)
	}
	if ev.Version != eventEnvelopeVersion {
		t.Errorf("expected version %d, got %d", eventEnvelopeVersion, ev.Version)
	}
	if ev.OccurredAt.IsZero() {
		t.Errorf("expected occurred_at to be set")
	}
	if ev.Order.ID == "" {
		t.Errorf("expected order.id to be set")
	}
	if ev.Order.UserID != "alice" {
		t.Errorf("expected order.user_id=alice, got %s", ev.Order.UserID)
	}
	if ev.Order.Pair != "BTC-KRW" {
		t.Errorf("expected pair BTC-KRW, got %s", ev.Order.Pair)
	}
	if ev.Order.Status != StatusOpen {
		t.Errorf("expected status open, got %s", ev.Order.Status)
	}
	if ev.Order.Quantity != "0.5" || ev.Order.RemainingQuantity != "0.5" || ev.Order.Price != "50000" {
		t.Errorf("decimal fields not preserved: %+v", ev.Order)
	}
}

func TestCreateOrderPublishFailureDoesNotFailRequest(t *testing.T) {
	store := newMockStore()
	pub := &fakePublisher{err: errors.New("boom")}
	h := NewHandler(store, pub)

	body, _ := json.Marshal(CreateOrderRequest{
		Pair: "BTC-KRW", Side: "buy", Quantity: "1", Price: "50000",
	})
	req := withUser(httptest.NewRequest("POST", "/orders", bytes.NewReader(body)), "alice")
	w := httptest.NewRecorder()
	h.CreateOrder(w, req)

	// Documented policy: DB success is authoritative, publish failure is
	// logged but the response remains 201.
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201 even on publish failure, got %d", w.Code)
	}
	if len(pub.snapshot()) != 1 {
		t.Errorf("expected publish to have been attempted once")
	}
	if len(store.orders) != 1 {
		t.Errorf("expected order to be persisted despite publish failure")
	}
}

func TestCreateOrderDoesNotPublishOnStoreFailure(t *testing.T) {
	// When the store rejects the order (e.g. validation failed after body
	// parsing), we must NOT emit an event.
	store := newMockStore()
	pub := &fakePublisher{}
	h := NewHandler(store, pub)

	body := []byte(`{"pair":"BTC-KRW"}`) // missing side/qty/price
	req := withUser(httptest.NewRequest("POST", "/orders", bytes.NewReader(body)), "alice")
	w := httptest.NewRecorder()
	h.CreateOrder(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if got := len(pub.snapshot()); got != 0 {
		t.Errorf("expected 0 events on bad request, got %d", got)
	}
}

// ---- cancel --------------------------------------------------------------

func TestCancelOrderPublishesOrderUpdated(t *testing.T) {
	store := newMockStore()
	pub := &fakePublisher{}
	h := NewHandler(store, pub)

	created := seedOrder(store, "alice", "BTC-KRW")

	req := withUser(httptest.NewRequest("DELETE", "/orders/"+created.ID, nil), "alice")
	req.SetPathValue("id", created.ID)
	w := httptest.NewRecorder()
	h.CancelOrder(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	events := pub.snapshot()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	ev := events[0]
	if ev.Type != SubjectOrderUpdated {
		t.Errorf("expected type %s, got %s", SubjectOrderUpdated, ev.Type)
	}
	if ev.Order.ID != created.ID {
		t.Errorf("expected order.id=%s, got %s", created.ID, ev.Order.ID)
	}
	if ev.Order.Status != StatusCancelled {
		t.Errorf("expected status cancelled, got %s", ev.Order.Status)
	}
}

func TestCancelOrderDoesNotPublishOnForbidden(t *testing.T) {
	store := newMockStore()
	pub := &fakePublisher{}
	h := NewHandler(store, pub)

	created := seedOrder(store, "alice", "BTC-KRW")

	req := withUser(httptest.NewRequest("DELETE", "/orders/"+created.ID, nil), "mallory")
	req.SetPathValue("id", created.ID)
	w := httptest.NewRecorder()
	h.CancelOrder(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
	if got := len(pub.snapshot()); got != 0 {
		t.Errorf("expected 0 events when cancel forbidden, got %d", got)
	}
}

func TestCancelOrderPublishFailureDoesNotFailRequest(t *testing.T) {
	store := newMockStore()
	pub := &fakePublisher{err: errors.New("boom")}
	h := NewHandler(store, pub)

	created := seedOrder(store, "alice", "BTC-KRW")

	req := withUser(httptest.NewRequest("DELETE", "/orders/"+created.ID, nil), "alice")
	req.SetPathValue("id", created.ID)
	w := httptest.NewRecorder()
	h.CancelOrder(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 even on publish failure, got %d", w.Code)
	}
	if store.orders[0].Status != StatusCancelled {
		t.Errorf("expected order to be cancelled in store despite publish failure")
	}
}

// ---- noop ----------------------------------------------------------------

func TestNoopPublisherNeverErrors(t *testing.T) {
	var p Publisher = NoopPublisher{}
	if err := p.PublishOrderEvent(context.Background(), OrderEvent{Type: SubjectOrderCreated}); err != nil {
		t.Errorf("noop publisher returned error: %v", err)
	}
	if err := p.PublishTradeEvent(context.Background(), TradeEvent{Type: SubjectTradeExecuted}); err != nil {
		t.Errorf("noop publisher returned error: %v", err)
	}
}

// ---- envelope ------------------------------------------------------------

func TestNewOrderEventEnvelopeShape(t *testing.T) {
	o := &Order{
		ID: "o1", UserID: "alice",
		Pair: "BTC-KRW", Side: "buy",
		Quantity: "0.1", RemainingQuantity: "0.1", Price: "50000",
		Status: StatusOpen,
	}
	ev := NewOrderEvent(SubjectOrderCreated, o, nowFixture())

	// Serialize and re-parse to check the wire-level field names — this
	// catches drift between the envelope definition and consumers.
	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, k := range []string{"type", "version", "occurred_at", "order"} {
		if _, ok := raw[k]; !ok {
			t.Errorf("envelope missing key %q", k)
		}
	}
	order, ok := raw["order"].(map[string]any)
	if !ok {
		t.Fatalf("order is not an object: %T", raw["order"])
	}
	for _, k := range []string{
		"id", "user_id", "pair", "side",
		"quantity", "remaining_quantity", "price", "status",
		"created_at", "updated_at",
	} {
		if _, ok := order[k]; !ok {
			t.Errorf("order payload missing key %q", k)
		}
	}
}

func TestNewTradeEventEnvelopeShape(t *testing.T) {
	tr := Trade{
		TradeID:      "t1",
		Pair:         "BTC-KRW",
		Price:        50000,
		Quantity:     mustParseQty(t, "0.25"),
		MakerOrderID: "maker-1",
		TakerOrderID: "taker-1",
		ExecutedAt:   nowFixture(),
	}
	ev := NewTradeExecutedEvent(tr, nowFixture())

	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, k := range []string{"type", "version", "occurred_at", "trade"} {
		if _, ok := raw[k]; !ok {
			t.Errorf("envelope missing key %q", k)
		}
	}
	tradeRaw, ok := raw["trade"].(map[string]any)
	if !ok {
		t.Fatalf("trade is not an object: %T", raw["trade"])
	}
	if got := tradeRaw["price"]; got != "50000" {
		t.Fatalf("price must be decimal string, got %#v", got)
	}
	if got := tradeRaw["quantity"]; got != "0.25" {
		t.Fatalf("quantity must be decimal string, got %#v", got)
	}
}

func TestStreamSubjectsIncludeOrderAndTrade(t *testing.T) {
	got := streamSubjects()
	if len(got) != 2 || got[0] != "order.>" || got[1] != "trade.>" {
		t.Fatalf("unexpected stream subjects: %#v", got)
	}
}

func TestStreamSubjectsCovered(t *testing.T) {
	if !streamSubjectsCovered([]string{"order.>", "trade.>"}, streamSubjects()) {
		t.Fatalf("expected required subjects to be covered")
	}
	if streamSubjectsCovered([]string{"order.>"}, streamSubjects()) {
		t.Fatalf("missing trade.> must not be treated as covered")
	}
}

func TestUnionStreamSubjectsPreservesExistingAndAddsMissingRequired(t *testing.T) {
	got := unionStreamSubjects(
		[]string{"order.>", "custom.>", "order.>"},
		[]string{"order.>", "trade.>"},
	)
	want := []string{"order.>", "custom.>", "trade.>"}
	if len(got) != len(want) {
		t.Fatalf("len mismatch: got=%d want=%d (%#v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("union[%d]=%q want=%q (all=%#v)", i, got[i], want[i], got)
		}
	}
}

func nowFixture() time.Time { return time.Unix(1700000000, 0).UTC() }
