package main

import (
	"reflect"
	"testing"
)

func TestProcessCancelOrderCancelsExistingRestingBuy(t *testing.T) {
	book := NewOrderBook()
	mustAppend(t, book, "buy-1", "buy", 1000, 120)
	mustAppend(t, book, "buy-2", "buy", 900, 80)
	mustAppend(t, book, "ask-1", "sell", 1300, 50)

	res, err := ProcessCancelOrder(book, "buy-1")
	if err != nil {
		t.Fatalf("cancel failed: %v", err)
	}
	if res.Status != CancelStatusCancelled || res.OrderID != "buy-1" || res.Side != "buy" || res.Price != 1000 || res.RemainingQuantity != 120 {
		t.Fatalf("unexpected cancel result: %+v", res)
	}
	if _, ok := book.byID["buy-1"]; ok {
		t.Fatalf("expected buy-1 removed from byID")
	}
	if got := snapshotIDs(book.SideSnapshot("buy")); !reflect.DeepEqual(got, []string{"buy-2"}) {
		t.Fatalf("unexpected buy snapshot after cancel: %v", got)
	}
	if got := snapshotIDs(book.SideSnapshot("sell")); !reflect.DeepEqual(got, []string{"ask-1"}) {
		t.Fatalf("sell side mutated unexpectedly: %v", got)
	}
	bestBid, ok := book.BestBid()
	if !ok || bestBid.OrderID != "buy-2" {
		t.Fatalf("best bid mismatch after cancel: ok=%v bid=%+v", ok, bestBid)
	}
}

func TestProcessCancelOrderCancelsExistingRestingSell(t *testing.T) {
	book := NewOrderBook()
	mustAppend(t, book, "bid-1", "buy", 900, 120)
	mustAppend(t, book, "ask-1", "sell", 1100, 70)
	mustAppend(t, book, "ask-2", "sell", 1200, 40)

	res, err := ProcessCancelOrder(book, "ask-1")
	if err != nil {
		t.Fatalf("cancel failed: %v", err)
	}
	if res.Status != CancelStatusCancelled || res.OrderID != "ask-1" || res.Side != "sell" || res.Price != 1100 || res.RemainingQuantity != 70 {
		t.Fatalf("unexpected cancel result: %+v", res)
	}
	if _, ok := book.byID["ask-1"]; ok {
		t.Fatalf("expected ask-1 removed from byID")
	}
	if got := snapshotIDs(book.SideSnapshot("sell")); !reflect.DeepEqual(got, []string{"ask-2"}) {
		t.Fatalf("unexpected sell snapshot after cancel: %v", got)
	}
	bestAsk, ok := book.BestAsk()
	if !ok || bestAsk.OrderID != "ask-2" {
		t.Fatalf("best ask mismatch after cancel: ok=%v ask=%+v", ok, bestAsk)
	}
}

func TestProcessCancelOrderRemovesOnlyOrderPriceLevel(t *testing.T) {
	book := NewOrderBook()
	mustAppend(t, book, "bid-1", "buy", 1000, 50)
	mustAppend(t, book, "ask-1", "sell", 1300, 60)

	res, err := ProcessCancelOrder(book, "bid-1")
	if err != nil {
		t.Fatalf("cancel failed: %v", err)
	}
	if res.Status != CancelStatusCancelled {
		t.Fatalf("expected cancelled status, got %+v", res)
	}
	if book.HasPriceLevel("buy", 1000) {
		t.Fatalf("expected buy price level 1000 removed")
	}
	if _, ok := book.BestBid(); ok {
		t.Fatalf("expected empty bid side")
	}
	if !book.HasPriceLevel("sell", 1300) {
		t.Fatalf("ask side should be untouched")
	}
}

func TestProcessCancelOrderUnknownOrderReturnsNotInBook(t *testing.T) {
	book := NewOrderBook()
	mustAppend(t, book, "bid-1", "buy", 1000, 50)
	mustAppend(t, book, "ask-1", "sell", 1200, 60)

	beforeBids := book.SideSnapshot("buy")
	beforeAsks := book.SideSnapshot("sell")

	res, err := ProcessCancelOrder(book, "unknown")
	if err != nil {
		t.Fatalf("expected nil error for unknown cancel, got %v", err)
	}
	if res.Status != CancelStatusNotInBook || res.OrderID != "unknown" {
		t.Fatalf("unexpected unknown cancel result: %+v", res)
	}

	afterBids := book.SideSnapshot("buy")
	afterAsks := book.SideSnapshot("sell")
	if !reflect.DeepEqual(beforeBids, afterBids) || !reflect.DeepEqual(beforeAsks, afterAsks) {
		t.Fatalf("book mutated on unknown cancel: beforeBids=%+v afterBids=%+v beforeAsks=%+v afterAsks=%+v", beforeBids, afterBids, beforeAsks, afterAsks)
	}
}

func TestProcessCancelOrderAfterPartialFillCancelsRemainingRestingOrder(t *testing.T) {
	book := NewOrderBook()
	mustAppend(t, book, "ask-1", "sell", 1000, 100)

	res := mustProcess(t, book, IncomingOrder{OrderID: "buy-1", Side: "buy", Price: 1000, RemainingQuantity: 150})
	if !res.TakerRested || res.TakerRemainingQuantity != 50 {
		t.Fatalf("expected taker buy-1 rest with 50, got %+v", res)
	}

	cancelRes, err := ProcessCancelOrder(book, "buy-1")
	if err != nil {
		t.Fatalf("cancel failed: %v", err)
	}
	if cancelRes.Status != CancelStatusCancelled || cancelRes.RemainingQuantity != 50 {
		t.Fatalf("unexpected cancel result: %+v", cancelRes)
	}
	if _, ok := book.byID["buy-1"]; ok {
		t.Fatalf("expected buy-1 removed from byID")
	}
	if got := snapshotIDs(book.SideSnapshot("buy")); len(got) != 0 {
		t.Fatalf("expected empty buy side after cancel, got %v", got)
	}
}

func TestProcessCancelOrderRepeatedCancelIsStable(t *testing.T) {
	book := NewOrderBook()
	mustAppend(t, book, "bid-1", "buy", 1000, 100)
	mustAppend(t, book, "bid-2", "buy", 900, 90)
	mustAppend(t, book, "ask-1", "sell", 1300, 70)

	first, err := ProcessCancelOrder(book, "bid-1")
	if err != nil {
		t.Fatalf("first cancel failed: %v", err)
	}
	if first.Status != CancelStatusCancelled {
		t.Fatalf("expected first cancel success, got %+v", first)
	}

	second, err := ProcessCancelOrder(book, "bid-1")
	if err != nil {
		t.Fatalf("second cancel failed: %v", err)
	}
	if second.Status != CancelStatusNotInBook {
		t.Fatalf("expected second cancel not_in_book, got %+v", second)
	}

	if got := snapshotIDs(book.SideSnapshot("buy")); !reflect.DeepEqual(got, []string{"bid-2"}) {
		t.Fatalf("unexpected buy snapshot after repeated cancel: %v", got)
	}
	if got := snapshotIDs(book.SideSnapshot("sell")); !reflect.DeepEqual(got, []string{"ask-1"}) {
		t.Fatalf("unexpected sell snapshot after repeated cancel: %v", got)
	}
}
