package main

import (
	"encoding/json"
	"testing"
	"time"
)

func sampleOrderEvent(t *testing.T, subject string, status, remain string) []byte {
	t.Helper()
	ev := OrderEvent{
		Type:       subject,
		Version:    1,
		OccurredAt: time.Unix(1700000000, 0).UTC(),
		Order: OrderPayload{
			ID:                "order-1",
			UserID:            "alice",
			Pair:              "BTC-KRW",
			Side:              "buy",
			Quantity:          "0.5",
			RemainingQuantity: remain,
			Price:             "50000",
			Status:            status,
			CreatedAt:         time.Unix(1700000000, 0).UTC(),
			UpdatedAt:         time.Unix(1700000000, 0).UTC(),
		},
	}
	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return data
}

func sampleTradeEvent(t *testing.T, id, price, qty string, ts time.Time) []byte {
	t.Helper()
	ev := TradeEvent{Type: SubjectTradeExecuted, Version: 1, OccurredAt: ts.UTC(), Trade: TradePayload{TradeID: id, Pair: SupportedPair, Price: price, Quantity: qty, MakerOrderID: "maker-1", TakerOrderID: "taker-1", ExecutedAt: ts.UTC()}}
	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return data
}

func TestConsumerHandlesTradeExecutedUpdatesTickerAndTrades(t *testing.T) {
	m := NewReadModel()
	c := NewConsumer(m)
	now := time.Unix(1700000200, 0).UTC()
	if err := c.Handle(SubjectTradeExecuted, sampleTradeEvent(t, "t-1", "50000", "0.1", now)); err != nil {
		t.Fatalf("handle: %v", err)
	}

	ticker, err := m.TickerSummary(SupportedPair, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("ticker: %v", err)
	}
	if ticker.LastPrice != "50000" {
		t.Fatalf("last_price mismatch: %s", ticker.LastPrice)
	}
	trades, err := m.RecentTrades(SupportedPair, 10)
	if err != nil {
		t.Fatalf("recent trades: %v", err)
	}
	if len(trades.Trades) != 1 || trades.Trades[0].TradeID != "t-1" {
		t.Fatalf("unexpected trades: %#v", trades)
	}
	snap := c.CountersSnapshot()
	if snap.Traded != 1 {
		t.Fatalf("expected traded=1, got %+v", snap)
	}
}

func TestConsumerHandlesOrderCreatedAndUpdated(t *testing.T) {
	c := NewConsumer(NewReadModel())
	if err := c.Handle(SubjectOrderCreated, sampleOrderEvent(t, SubjectOrderCreated, "open", "0.5")); err != nil {
		t.Fatalf("created: %v", err)
	}
	if err := c.Handle(SubjectOrderUpdated, sampleOrderEvent(t, SubjectOrderUpdated, "partially_filled", "0.2")); err != nil {
		t.Fatalf("updated: %v", err)
	}
	snap := c.CountersSnapshot()
	if snap.Created != 1 || snap.Updated != 1 {
		t.Fatalf("unexpected counters: %+v", snap)
	}
}

func TestConsumerRejectsMalformedJSON(t *testing.T) {
	c := NewConsumer(NewReadModel())
	err := c.Handle(SubjectOrderCreated, []byte("not-json"))
	if err == nil {
		t.Fatalf("expected error on malformed payload")
	}
}

func TestConsumerContinuesAfterMalformedPayload(t *testing.T) {
	c := NewConsumer(NewReadModel())
	_ = c.Handle(SubjectOrderCreated, []byte("not-json"))
	if err := c.Handle(SubjectOrderCreated, sampleOrderEvent(t, SubjectOrderCreated, "open", "0.5")); err != nil {
		t.Fatalf("expected consumer to continue after malformed payload: %v", err)
	}
	snap := c.CountersSnapshot()
	if snap.Created != 1 {
		t.Fatalf("expected created counter to keep progressing, got %+v", snap)
	}
}

func TestConsumerUnknownSubjectIncrementsUnknown(t *testing.T) {
	c := NewConsumer(NewReadModel())
	if err := c.Handle("order.other", sampleOrderEvent(t, "order.other", "open", "0.1")); err != nil {
		t.Fatalf("handle: %v", err)
	}
	snap := c.CountersSnapshot()
	if snap.Unknown != 1 {
		t.Fatalf("expected unknown=1, got %d", snap.Unknown)
	}
}

func TestStreamSubjectsIncludeTradePrefix(t *testing.T) {
	got := streamSubjects()
	if len(got) != 2 || got[0] != "order.>" || got[1] != "trade.>" {
		t.Fatalf("unexpected stream subjects: %#v", got)
	}
}

func TestStreamSubjectsCoveredAndUnion(t *testing.T) {
	if !streamSubjectsCovered([]string{"order.>", "trade.>"}, streamSubjects()) {
		t.Fatalf("expected required subjects to be covered")
	}
	if streamSubjectsCovered([]string{"order.>"}, streamSubjects()) {
		t.Fatalf("missing trade.> must not be treated as covered")
	}
	got := unionStreamSubjects([]string{"custom.>", "order.>", "custom.>"}, streamSubjects())
	want := []string{"custom.>", "order.>", "trade.>"}
	if len(got) != len(want) {
		t.Fatalf("len mismatch: got=%d want=%d (%#v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("union[%d]=%q want=%q", i, got[i], want[i])
		}
	}
}

func TestTradeDurableNameIsDeterministicAndDistinct(t *testing.T) {
	base := "marketdata-consumer"
	got := tradeDurableName(base)
	if got != "marketdata-consumer-trade" {
		t.Fatalf("unexpected trade durable name: %s", got)
	}
	if got == base {
		t.Fatalf("trade durable must be distinct from order durable")
	}
}

func TestConsumerUsesSeparateOrderAndTradeSubjects(t *testing.T) {
	if orderEventsSubject != "order.*" {
		t.Fatalf("order subject changed: %s", orderEventsSubject)
	}
	if tradeEventsSubject != "trade.*" {
		t.Fatalf("trade subject changed: %s", tradeEventsSubject)
	}
	if orderEventsSubject == tradeEventsSubject {
		t.Fatalf("subjects must be distinct")
	}
}
