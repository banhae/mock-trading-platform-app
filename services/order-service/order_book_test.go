package main

import (
	"reflect"
	"testing"
)

func mustAppend(t *testing.T, b *OrderBook, id, side string, price, qty int64) BookOrder {
	t.Helper()
	got, err := b.AppendRestingOrder(BookOrder{
		OrderID:           id,
		Side:              side,
		Price:             price,
		RemainingQuantity: qty,
	})
	if err != nil {
		t.Fatalf("append %s failed: %v", id, err)
	}
	return got
}

func snapshotIDs(orders []BookOrder) []string {
	ids := make([]string, 0, len(orders))
	for _, o := range orders {
		ids = append(ids, o.OrderID)
	}
	return ids
}

func TestOrderBookBidOrderingAndFIFO(t *testing.T) {
	book := NewOrderBook()
	mustAppend(t, book, "b1", "buy", 1000, 100)
	mustAppend(t, book, "b2", "buy", 1100, 100)
	mustAppend(t, book, "b3", "buy", 1000, 100)

	got := snapshotIDs(book.SideSnapshot("buy"))
	want := []string{"b2", "b1", "b3"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("bid ordering mismatch: want %v got %v", want, got)
	}
}

func TestOrderBookAskOrderingAndFIFO(t *testing.T) {
	book := NewOrderBook()
	mustAppend(t, book, "a1", "sell", 1200, 100)
	mustAppend(t, book, "a2", "sell", 1100, 100)
	mustAppend(t, book, "a3", "sell", 1200, 100)

	got := snapshotIDs(book.SideSnapshot("sell"))
	want := []string{"a2", "a1", "a3"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ask ordering mismatch: want %v got %v", want, got)
	}
}

func TestOrderBookBestBidBestAsk(t *testing.T) {
	book := NewOrderBook()
	mustAppend(t, book, "b1", "buy", 1000, 100)
	mustAppend(t, book, "b2", "buy", 1000, 100)
	mustAppend(t, book, "a1", "sell", 1300, 100)
	mustAppend(t, book, "a2", "sell", 1200, 100)

	bid, ok := book.BestBid()
	if !ok || bid.OrderID != "b1" {
		t.Fatalf("best bid mismatch: ok=%v order=%+v", ok, bid)
	}
	ask, ok := book.BestAsk()
	if !ok || ask.OrderID != "a2" {
		t.Fatalf("best ask mismatch: ok=%v order=%+v", ok, ask)
	}
}

func TestOrderBookRemoveMiddleHeadTailAndUnknown(t *testing.T) {
	book := NewOrderBook()
	mustAppend(t, book, "o1", "buy", 1000, 100)
	mustAppend(t, book, "o2", "buy", 1000, 100)
	mustAppend(t, book, "o3", "buy", 1000, 100)

	if _, ok := book.RemoveOrderByID("o2"); !ok {
		t.Fatalf("expected middle removal success")
	}
	if got := snapshotIDs(book.SideSnapshot("buy")); !reflect.DeepEqual(got, []string{"o1", "o3"}) {
		t.Fatalf("after middle removal, got %v", got)
	}

	if _, ok := book.RemoveOrderByID("o1"); !ok {
		t.Fatalf("expected head removal success")
	}
	if got := snapshotIDs(book.SideSnapshot("buy")); !reflect.DeepEqual(got, []string{"o3"}) {
		t.Fatalf("after head removal, got %v", got)
	}

	if _, ok := book.RemoveOrderByID("o3"); !ok {
		t.Fatalf("expected tail removal success")
	}
	if got := snapshotIDs(book.SideSnapshot("buy")); len(got) != 0 {
		t.Fatalf("expected empty after tail removal, got %v", got)
	}
	if book.HasPriceLevel("buy", 1000) {
		t.Fatalf("expected empty level to be removed")
	}

	if _, ok := book.RemoveOrderByID("unknown"); ok {
		t.Fatalf("unknown order must return ok=false")
	}
}

func TestOrderBookStabilityRepeatedInsertRemove(t *testing.T) {
	book := NewOrderBook()

	mustAppend(t, book, "b1", "buy", 1000, 100)
	mustAppend(t, book, "b2", "buy", 1100, 100)
	mustAppend(t, book, "a1", "sell", 1400, 100)
	mustAppend(t, book, "a2", "sell", 1300, 100)
	if _, ok := book.RemoveOrderByID("b2"); !ok {
		t.Fatalf("expected remove b2 success")
	}
	mustAppend(t, book, "b3", "buy", 1200, 100)
	if _, ok := book.RemoveOrderByID("a2"); !ok {
		t.Fatalf("expected remove a2 success")
	}
	mustAppend(t, book, "a3", "sell", 1250, 100)

	if got := snapshotIDs(book.SideSnapshot("buy")); !reflect.DeepEqual(got, []string{"b3", "b1"}) {
		t.Fatalf("buy side unstable ordering: %v", got)
	}
	if got := snapshotIDs(book.SideSnapshot("sell")); !reflect.DeepEqual(got, []string{"a3", "a1"}) {
		t.Fatalf("sell side unstable ordering: %v", got)
	}

	bestBid, ok := book.BestBid()
	if !ok || bestBid.OrderID != "b3" {
		t.Fatalf("best bid unstable: %+v ok=%v", bestBid, ok)
	}
	bestAsk, ok := book.BestAsk()
	if !ok || bestAsk.OrderID != "a3" {
		t.Fatalf("best ask unstable: %+v ok=%v", bestAsk, ok)
	}
}

func TestOrderBookSequenceIsMonotonic(t *testing.T) {
	book := NewOrderBook()
	o1 := mustAppend(t, book, "o1", "buy", 1000, 100)
	o2 := mustAppend(t, book, "o2", "buy", 1000, 100)
	o3 := mustAppend(t, book, "o3", "sell", 1000, 100)

	if !(o1.Sequence < o2.Sequence && o2.Sequence < o3.Sequence) {
		t.Fatalf("expected monotonic sequence: %d %d %d", o1.Sequence, o2.Sequence, o3.Sequence)
	}
}

func TestOrderBookSideSnapshotInvalidSideReturnsEmpty(t *testing.T) {
	book := NewOrderBook()
	mustAppend(t, book, "b1", "buy", 1000, 100)
	mustAppend(t, book, "a1", "sell", 1200, 100)

	got := book.SideSnapshot("invalid")
	if len(got) != 0 {
		t.Fatalf("expected empty snapshot for invalid side, got %v", got)
	}
}

func TestOrderBookHasPriceLevelInvalidSideReturnsFalse(t *testing.T) {
	book := NewOrderBook()
	mustAppend(t, book, "b1", "buy", 1000, 100)
	mustAppend(t, book, "a1", "sell", 1200, 100)

	if got := book.HasPriceLevel("invalid", 1000); got {
		t.Fatalf("expected false for invalid side")
	}
}
