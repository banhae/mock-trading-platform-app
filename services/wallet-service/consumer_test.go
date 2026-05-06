package main

import (
	"encoding/json"
	"testing"
	"time"
)

func sampleOrderEvent(t *testing.T, subject string) []byte {
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
			RemainingQuantity: "0.5",
			Price:             "50000",
			Status:            "open",
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

func TestConsumerHandlesOrderCreated(t *testing.T) {
	c := NewConsumer()
	if err := c.Handle(SubjectOrderCreated, sampleOrderEvent(t, SubjectOrderCreated)); err != nil {
		t.Fatalf("handle: %v", err)
	}
	snap := c.CountersSnapshot()
	if snap.Created != 1 {
		t.Errorf("expected created=1, got %d", snap.Created)
	}
	if snap.Updated != 0 || snap.Unknown != 0 || snap.Failed != 0 {
		t.Errorf("unexpected counters: %+v", snap)
	}
}

func TestConsumerHandlesOrderUpdated(t *testing.T) {
	c := NewConsumer()
	if err := c.Handle(SubjectOrderUpdated, sampleOrderEvent(t, SubjectOrderUpdated)); err != nil {
		t.Fatalf("handle: %v", err)
	}
	snap := c.CountersSnapshot()
	if snap.Updated != 1 {
		t.Errorf("expected updated=1, got %d", snap.Updated)
	}
}

func TestConsumerCountsMultipleEvents(t *testing.T) {
	c := NewConsumer()
	for i := 0; i < 2; i++ {
		if err := c.Handle(SubjectOrderCreated, sampleOrderEvent(t, SubjectOrderCreated)); err != nil {
			t.Fatalf("handle created: %v", err)
		}
	}
	for i := 0; i < 3; i++ {
		if err := c.Handle(SubjectOrderUpdated, sampleOrderEvent(t, SubjectOrderUpdated)); err != nil {
			t.Fatalf("handle updated: %v", err)
		}
	}
	snap := c.CountersSnapshot()
	if snap.Created != 2 || snap.Updated != 3 {
		t.Errorf("expected created=2 updated=3, got %+v", snap)
	}
}

func TestConsumerRejectsMalformedJSON(t *testing.T) {
	c := NewConsumer()
	err := c.Handle(SubjectOrderCreated, []byte("{ nope"))
	if err == nil {
		t.Fatalf("expected error on malformed payload")
	}
	if c.CountersSnapshot().Created != 0 {
		t.Errorf("expected created=0 on malformed payload")
	}
}

func TestConsumerUnknownSubjectIncrementsUnknown(t *testing.T) {
	c := NewConsumer()
	if err := c.Handle("order.other", sampleOrderEvent(t, "order.other")); err != nil {
		t.Fatalf("handle: %v", err)
	}
	snap := c.CountersSnapshot()
	if snap.Unknown != 1 {
		t.Errorf("expected unknown=1, got %d", snap.Unknown)
	}
}

func TestStreamSubjectsIncludeTradePrefix(t *testing.T) {
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

func TestUnionStreamSubjectsDeterministicDedup(t *testing.T) {
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
