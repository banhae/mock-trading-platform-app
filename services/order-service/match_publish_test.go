package main

import (
	"context"
	"errors"
	"testing"
)

type fakeMatchPersister struct {
	persisted PersistedMatchResult
	err       error
	calls     int
}

func (f *fakeMatchPersister) PersistMatchResult(_ context.Context, _ string, _ MatchResult) (PersistedMatchResult, error) {
	f.calls++
	if f.err != nil {
		return PersistedMatchResult{}, f.err
	}
	return f.persisted, nil
}

type recordedEvent struct {
	kind string
	id   string
}

type scriptedPublisher struct {
	published []recordedEvent
	fail      map[string]error
}

func (p *scriptedPublisher) PublishOrderEvent(_ context.Context, event OrderEvent) error {
	p.published = append(p.published, recordedEvent{kind: event.Type, id: event.Order.ID})
	if err, ok := p.fail[event.Order.ID]; ok {
		return err
	}
	return nil
}

func (p *scriptedPublisher) PublishTradeEvent(_ context.Context, event TradeEvent) error {
	p.published = append(p.published, recordedEvent{kind: event.Type, id: event.Trade.TradeID})
	if err, ok := p.fail[event.Trade.TradeID]; ok {
		return err
	}
	return nil
}

func TestPersistAndPublishMatchResultOrdersThenTrades(t *testing.T) {
	persister := &fakeMatchPersister{persisted: PersistedMatchResult{
		UpdatedOrders: []*Order{{ID: "maker-1"}, {ID: "maker-2"}, {ID: "taker-1"}},
		Trades:        []Trade{{TradeID: "trade-1"}, {TradeID: "trade-2"}},
	}}
	pub := &scriptedPublisher{}

	_, err := PersistAndPublishMatchResult(context.Background(), persister, pub, "BTC-KRW", MatchResult{TakerOrderID: "taker-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []recordedEvent{
		{kind: SubjectOrderUpdated, id: "maker-1"},
		{kind: SubjectOrderUpdated, id: "maker-2"},
		{kind: SubjectOrderUpdated, id: "taker-1"},
		{kind: SubjectTradeExecuted, id: "trade-1"},
		{kind: SubjectTradeExecuted, id: "trade-2"},
	}
	if len(pub.published) != len(want) {
		t.Fatalf("published len=%d want=%d", len(pub.published), len(want))
	}
	for i := range want {
		if pub.published[i] != want[i] {
			t.Fatalf("published[%d]=%+v want %+v", i, pub.published[i], want[i])
		}
	}
}

func TestPersistAndPublishMatchResultNoPublishOnPersistenceFailure(t *testing.T) {
	persister := &fakeMatchPersister{err: errors.New("db boom")}
	pub := &scriptedPublisher{}

	_, err := PersistAndPublishMatchResult(context.Background(), persister, pub, "BTC-KRW", MatchResult{TakerOrderID: "taker-1"})
	if err == nil {
		t.Fatalf("expected persistence error")
	}
	if len(pub.published) != 0 {
		t.Fatalf("expected no publish on persistence failure, got %d", len(pub.published))
	}
}

func TestPersistAndPublishMatchResultBestEffortPublish(t *testing.T) {
	persister := &fakeMatchPersister{persisted: PersistedMatchResult{
		UpdatedOrders: []*Order{{ID: "maker-1"}, {ID: "taker-1"}},
		Trades:        []Trade{{TradeID: "trade-1"}, {TradeID: "trade-2"}},
	}}
	pub := &scriptedPublisher{fail: map[string]error{
		"maker-1": errors.New("order publish fail"),
		"trade-1": errors.New("trade publish fail"),
	}}

	persisted, err := PersistAndPublishMatchResult(context.Background(), persister, pub, "BTC-KRW", MatchResult{TakerOrderID: "taker-1"})
	if err != nil {
		t.Fatalf("persist+publish should not fail on publish errors: %v", err)
	}
	if len(persisted.UpdatedOrders) != 2 || len(persisted.Trades) != 2 {
		t.Fatalf("persisted result must be returned untouched")
	}
	if len(pub.published) != 4 {
		t.Fatalf("best-effort must continue publishing all events, got %d", len(pub.published))
	}
}

func TestPersistAndPublishMatchResultOneOrderUpdatePerFinalOrder(t *testing.T) {
	persister := &fakeMatchPersister{persisted: PersistedMatchResult{
		UpdatedOrders: []*Order{{ID: "maker-1"}, {ID: "taker-1"}},
		Trades:        []Trade{{TradeID: "trade-1"}, {TradeID: "trade-2"}},
	}}
	pub := &scriptedPublisher{}

	_, err := PersistAndPublishMatchResult(context.Background(), persister, pub, "BTC-KRW", MatchResult{TakerOrderID: "taker-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	orderCount := 0
	tradeCount := 0
	for _, e := range pub.published {
		switch e.kind {
		case SubjectOrderUpdated:
			orderCount++
		case SubjectTradeExecuted:
			tradeCount++
		}
	}
	if orderCount != 2 {
		t.Fatalf("expected one order.updated per final order (2), got %d", orderCount)
	}
	if tradeCount != 2 {
		t.Fatalf("expected one trade.executed per persisted trade row (2), got %d", tradeCount)
	}
}
