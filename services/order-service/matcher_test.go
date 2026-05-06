package main

import "testing"

func mustProcess(t *testing.T, b *OrderBook, in IncomingOrder) MatchResult {
	t.Helper()
	got, err := ProcessNewOrder(b, in)
	if err != nil {
		t.Fatalf("process failed: %v", err)
	}
	return got
}

func TestProcessNewOrderBuyTakerMatchesAskAtMakerPrice(t *testing.T) {
	book := NewOrderBook()
	mustAppend(t, book, "ask-1", "sell", 1000, 200)

	res := mustProcess(t, book, IncomingOrder{OrderID: "buy-1", Side: "buy", Price: 1100, RemainingQuantity: 100})
	if len(res.Fills) != 1 {
		t.Fatalf("expected 1 fill, got %d", len(res.Fills))
	}
	if res.Fills[0].Price != 1000 {
		t.Fatalf("expected maker price 1000, got %d", res.Fills[0].Price)
	}
	if res.TakerRemainingQuantity != 0 || res.TakerRested {
		t.Fatalf("expected fully matched taker, got remaining=%d rested=%v", res.TakerRemainingQuantity, res.TakerRested)
	}
	ask, ok := book.BestAsk()
	if !ok || ask.RemainingQuantity != 100 {
		t.Fatalf("expected maker remaining 100, got ok=%v ask=%+v", ok, ask)
	}
}

func TestProcessNewOrderSellTakerMatchesBidAtMakerPrice(t *testing.T) {
	book := NewOrderBook()
	mustAppend(t, book, "bid-1", "buy", 1300, 150)

	res := mustProcess(t, book, IncomingOrder{OrderID: "sell-1", Side: "sell", Price: 1200, RemainingQuantity: 50})
	if len(res.Fills) != 1 {
		t.Fatalf("expected 1 fill, got %d", len(res.Fills))
	}
	if res.Fills[0].Price != 1300 {
		t.Fatalf("expected maker price 1300, got %d", res.Fills[0].Price)
	}
	bid, ok := book.BestBid()
	if !ok || bid.RemainingQuantity != 100 {
		t.Fatalf("expected bid remaining 100, got ok=%v bid=%+v", ok, bid)
	}
}

func TestProcessNewOrderPartialFillLeavesMakerResting(t *testing.T) {
	book := NewOrderBook()
	mustAppend(t, book, "ask-1", "sell", 1000, 300)

	res := mustProcess(t, book, IncomingOrder{OrderID: "buy-1", Side: "buy", Price: 1000, RemainingQuantity: 120})
	if len(res.Fills) != 1 || res.Fills[0].Quantity != 120 {
		t.Fatalf("unexpected fills: %+v", res.Fills)
	}
	ask, ok := book.BestAsk()
	if !ok || ask.OrderID != "ask-1" || ask.RemainingQuantity != 180 {
		t.Fatalf("expected partial maker remainder 180, got ok=%v ask=%+v", ok, ask)
	}
}

func TestProcessNewOrderFullFillRemovesMakerFromBook(t *testing.T) {
	book := NewOrderBook()
	mustAppend(t, book, "ask-1", "sell", 1000, 100)

	res := mustProcess(t, book, IncomingOrder{OrderID: "buy-1", Side: "buy", Price: 1000, RemainingQuantity: 100})
	if len(res.Fills) != 1 || res.Fills[0].Quantity != 100 {
		t.Fatalf("unexpected fills: %+v", res.Fills)
	}
	if _, ok := book.BestAsk(); ok {
		t.Fatalf("expected maker removed from ask book")
	}
}

func TestProcessNewOrderSamePriceFIFO(t *testing.T) {
	book := NewOrderBook()
	mustAppend(t, book, "ask-1", "sell", 1000, 60)
	mustAppend(t, book, "ask-2", "sell", 1000, 60)

	res := mustProcess(t, book, IncomingOrder{OrderID: "buy-1", Side: "buy", Price: 1000, RemainingQuantity: 70})
	if len(res.Fills) != 2 {
		t.Fatalf("expected 2 fills, got %+v", res.Fills)
	}
	if res.Fills[0].MakerOrderID != "ask-1" || res.Fills[1].MakerOrderID != "ask-2" {
		t.Fatalf("expected FIFO makers ask-1 -> ask-2, got %+v", res.Fills)
	}
	ask, ok := book.BestAsk()
	if !ok || ask.OrderID != "ask-2" || ask.RemainingQuantity != 50 {
		t.Fatalf("expected ask-2 remain 50, got ok=%v ask=%+v", ok, ask)
	}
}

func TestProcessNewOrderNonCrossingBecomesResting(t *testing.T) {
	book := NewOrderBook()
	mustAppend(t, book, "ask-1", "sell", 1100, 100)

	res := mustProcess(t, book, IncomingOrder{OrderID: "buy-1", Side: "buy", Price: 1000, RemainingQuantity: 80})
	if len(res.Fills) != 0 {
		t.Fatalf("expected no fills, got %+v", res.Fills)
	}
	if !res.TakerRested || res.TakerRemainingQuantity != 80 {
		t.Fatalf("expected taker to rest with 80, got rested=%v remaining=%d", res.TakerRested, res.TakerRemainingQuantity)
	}
	bid, ok := book.BestBid()
	if !ok || bid.OrderID != "buy-1" || bid.RemainingQuantity != 80 {
		t.Fatalf("expected resting bid buy-1(80), got ok=%v bid=%+v", ok, bid)
	}
}

func TestProcessNewOrderTakerMatchesMultipleMakersInSequence(t *testing.T) {
	book := NewOrderBook()
	mustAppend(t, book, "ask-1", "sell", 1000, 50)
	mustAppend(t, book, "ask-2", "sell", 1010, 70)
	mustAppend(t, book, "ask-3", "sell", 1020, 90)

	res := mustProcess(t, book, IncomingOrder{OrderID: "buy-1", Side: "buy", Price: 1020, RemainingQuantity: 140})
	if len(res.Fills) != 3 {
		t.Fatalf("expected 3 fills, got %+v", res.Fills)
	}
	if res.Fills[0].MakerOrderID != "ask-1" || res.Fills[1].MakerOrderID != "ask-2" || res.Fills[2].MakerOrderID != "ask-3" {
		t.Fatalf("unexpected match sequence: %+v", res.Fills)
	}
	ask, ok := book.BestAsk()
	if !ok || ask.OrderID != "ask-3" || ask.RemainingQuantity != 70 {
		t.Fatalf("expected ask-3 remain 70, got ok=%v ask=%+v", ok, ask)
	}
}

func TestProcessNewOrderRemainingTakerRestsWhenCrossingStops(t *testing.T) {
	book := NewOrderBook()
	mustAppend(t, book, "ask-1", "sell", 1000, 50)
	mustAppend(t, book, "ask-2", "sell", 1100, 50)

	res := mustProcess(t, book, IncomingOrder{OrderID: "buy-1", Side: "buy", Price: 1050, RemainingQuantity: 120})
	if len(res.Fills) != 1 {
		t.Fatalf("expected 1 fill, got %+v", res.Fills)
	}
	if res.TakerRemainingQuantity != 70 || !res.TakerRested {
		t.Fatalf("expected taker remainder 70 resting, got remaining=%d rested=%v", res.TakerRemainingQuantity, res.TakerRested)
	}

	bids := book.SideSnapshot("buy")
	if len(bids) != 1 || bids[0].OrderID != "buy-1" || bids[0].RemainingQuantity != 70 || bids[0].Price != 1050 {
		t.Fatalf("expected resting bid buy-1 price 1050 qty 70, got %+v", bids)
	}
	ask, ok := book.BestAsk()
	if !ok || ask.OrderID != "ask-2" || ask.RemainingQuantity != 50 {
		t.Fatalf("expected unmatched ask-2 stay untouched, got ok=%v ask=%+v", ok, ask)
	}
}

func TestProcessNewOrderDuplicateOrderIDRejectedWithoutMutation(t *testing.T) {
	book := NewOrderBook()
	mustAppend(t, book, "dup-1", "sell", 1000, 120)
	mustAppend(t, book, "bid-1", "buy", 900, 80)

	beforeAsks := book.SideSnapshot("sell")
	beforeBids := book.SideSnapshot("buy")

	res, err := ProcessNewOrder(book, IncomingOrder{OrderID: "dup-1", Side: "buy", Price: 1100, RemainingQuantity: 50})
	if err == nil {
		t.Fatalf("expected duplicate error")
	}
	if err != ErrDuplicateOrderID {
		t.Fatalf("expected ErrDuplicateOrderID, got %v", err)
	}
	if len(res.Fills) != 0 {
		t.Fatalf("expected no fills on duplicate rejection, got %+v", res.Fills)
	}
	if res.TakerRested {
		t.Fatalf("duplicate rejection must not rest taker")
	}

	afterAsks := book.SideSnapshot("sell")
	afterBids := book.SideSnapshot("buy")
	if len(beforeAsks) != len(afterAsks) {
		t.Fatalf("sell side length changed: before=%d after=%d", len(beforeAsks), len(afterAsks))
	}
	for i := range beforeAsks {
		if beforeAsks[i] != afterAsks[i] {
			t.Fatalf("sell side mutated at %d: before=%+v after=%+v", i, beforeAsks[i], afterAsks[i])
		}
	}
	if len(beforeBids) != len(afterBids) {
		t.Fatalf("buy side length changed: before=%d after=%d", len(beforeBids), len(afterBids))
	}
	for i := range beforeBids {
		if beforeBids[i] != afterBids[i] {
			t.Fatalf("buy side mutated at %d: before=%+v after=%+v", i, beforeBids[i], afterBids[i])
		}
	}
}
