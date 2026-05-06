package main

import (
	"fmt"
	"testing"
	"time"
)

func mkOrderEvent(id, side, status, price, remaining string) OrderEvent {
	now := time.Unix(1700000000, 0).UTC()
	return OrderEvent{Type: SubjectOrderCreated, Version: 1, OccurredAt: now, Order: OrderPayload{ID: id, UserID: "u1", Pair: SupportedPair, Side: side, Quantity: remaining, RemainingQuantity: remaining, Price: price, Status: status, CreatedAt: now, UpdatedAt: now}}
}

func mkTradeEvent(id, price, qty string, ts time.Time) TradeEvent {
	return TradeEvent{Type: SubjectTradeExecuted, Version: 1, OccurredAt: ts, Trade: TradePayload{TradeID: id, Pair: SupportedPair, Price: price, Quantity: qty, MakerOrderID: "maker-1", TakerOrderID: "taker-1", ExecutedAt: ts}}
}

func TestTradeExecutedUpdatesCandleBuckets(t *testing.T) {
	m := NewReadModel()
	t0 := time.Date(2026, 4, 14, 10, 1, 10, 0, time.UTC)
	if err := m.ApplyTradeEvent(mkTradeEvent("t1", "50000", "0.2", t0)); err != nil {
		t.Fatalf("apply t1: %v", err)
	}
	if err := m.ApplyTradeEvent(mkTradeEvent("t2", "51000", "0.3", t0.Add(20*time.Second))); err != nil {
		t.Fatalf("apply t2: %v", err)
	}
	candles, err := m.Candles(SupportedPair, "1m", 10)
	if err != nil {
		t.Fatalf("candles: %v", err)
	}
	if len(candles.Candles) != 1 {
		t.Fatalf("expected 1 candle, got %d", len(candles.Candles))
	}
	c := candles.Candles[0]
	if c.Open != "50000" || c.Close != "51000" || c.High != "51000" || c.Low != "50000" {
		t.Fatalf("unexpected ohlc: %+v", c)
	}
	if c.Volume != "0.5" {
		t.Fatalf("unexpected volume: %s", c.Volume)
	}
}

func TestOrderCreatedAddsDepthAndUpdatedAdjustsOrRemoves(t *testing.T) {
	m := NewReadModel()
	if err := m.ApplyOrderEvent(mkOrderEvent("o1", "buy", "open", "50000", "0.4")); err != nil {
		t.Fatalf("create: %v", err)
	}
	book, _ := m.OrderBook(SupportedPair, 20, time.Now().UTC())
	if len(book.Bids) != 1 || book.Bids[0].Price != "50000" || book.Bids[0].Quantity != "0.4" {
		t.Fatalf("unexpected orderbook after create: %+v", book)
	}

	ev := mkOrderEvent("o1", "buy", "partially_filled", "50000", "0.1")
	ev.Type = SubjectOrderUpdated
	if err := m.ApplyOrderEvent(ev); err != nil {
		t.Fatalf("update partial: %v", err)
	}
	book, _ = m.OrderBook(SupportedPair, 20, time.Now().UTC())
	if len(book.Bids) != 1 || book.Bids[0].Quantity != "0.1" {
		t.Fatalf("unexpected after partial: %+v", book)
	}

	ev2 := mkOrderEvent("o1", "buy", "filled", "50000", "0")
	ev2.Type = SubjectOrderUpdated
	if err := m.ApplyOrderEvent(ev2); err != nil {
		t.Fatalf("update filled: %v", err)
	}
	book, _ = m.OrderBook(SupportedPair, 20, time.Now().UTC())
	if len(book.Bids) != 0 {
		t.Fatalf("filled order must not contribute: %+v", book)
	}
}

func TestCancelledOrderDoesNotContributeDepth(t *testing.T) {
	m := NewReadModel()
	if err := m.ApplyOrderEvent(mkOrderEvent("o2", "sell", "cancelled", "52000", "1.2")); err != nil {
		t.Fatalf("apply: %v", err)
	}
	book, _ := m.OrderBook(SupportedPair, 20, time.Now().UTC())
	if len(book.Asks) != 0 {
		t.Fatalf("cancelled order must not contribute: %+v", book)
	}
}

func TestOrderBookDepthParameter(t *testing.T) {
	m := NewReadModel()
	for i, p := range []string{"50000", "49900", "49800"} {
		if err := m.ApplyOrderEvent(mkOrderEvent("b"+string(rune('1'+i)), "buy", "open", p, "1")); err != nil {
			t.Fatalf("apply: %v", err)
		}
	}
	book, _ := m.OrderBook(SupportedPair, 2, time.Now().UTC())
	if len(book.Bids) != 2 {
		t.Fatalf("expected depth 2, got %d", len(book.Bids))
	}
	if book.Bids[0].Price != "50000" || book.Bids[1].Price != "49900" {
		t.Fatalf("unexpected bid order: %+v", book.Bids)
	}
}

func TestTradesRecentFirstOrder(t *testing.T) {
	m := NewReadModel()
	t0 := time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC)
	_ = m.ApplyTradeEvent(mkTradeEvent("t1", "50000", "0.1", t0))
	_ = m.ApplyTradeEvent(mkTradeEvent("t2", "50100", "0.1", t0.Add(time.Second)))
	trades, _ := m.RecentTrades(SupportedPair, 10)
	if len(trades.Trades) != 2 || trades.Trades[0].TradeID != "t2" || trades.Trades[1].TradeID != "t1" {
		t.Fatalf("unexpected trade ordering: %+v", trades)
	}
}

func TestCandlesInAscendingOrder(t *testing.T) {
	m := NewReadModel()
	t0 := time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC)
	_ = m.ApplyTradeEvent(mkTradeEvent("t1", "50000", "0.1", t0))
	_ = m.ApplyTradeEvent(mkTradeEvent("t2", "50100", "0.1", t0.Add(2*time.Minute)))
	candles, _ := m.Candles(SupportedPair, "1m", 10)
	if len(candles.Candles) != 2 {
		t.Fatalf("expected 2 candles, got %d", len(candles.Candles))
	}
	if !candles.Candles[0].Timestamp.Before(candles.Candles[1].Timestamp) {
		t.Fatalf("candles must be ascending by timestamp: %+v", candles)
	}
}

func TestTickerSummaryUsesFull24hWindowBeyondRecentTradeCap(t *testing.T) {
	m := NewReadModel()
	base := time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC)
	total := maxRecentTrades + 205 // 1205 trades, all within 24h

	for i := 0; i < total; i++ {
		ts := base.Add(time.Duration(i) * time.Second)
		price := 50000 + int64(i%10)
		if err := m.ApplyTradeEvent(mkTradeEvent(fmt.Sprintf("t-%d", i), fmt.Sprintf("%d", price), "0.1", ts)); err != nil {
			t.Fatalf("apply trade %d: %v", i, err)
		}
	}

	recent, err := m.RecentTrades(SupportedPair, 5000)
	if err != nil {
		t.Fatalf("recent trades: %v", err)
	}
	if len(recent.Trades) != maxRecentTrades {
		t.Fatalf("recent trades must stay capped at %d, got %d", maxRecentTrades, len(recent.Trades))
	}

	now := base.Add(2 * time.Hour)
	ticker, err := m.TickerSummary(SupportedPair, now)
	if err != nil {
		t.Fatalf("ticker summary: %v", err)
	}

	if ticker.LastPrice != "50004" {
		t.Fatalf("last price mismatch: %s", ticker.LastPrice)
	}
	if ticker.High24h != "50009" || ticker.Low24h != "50000" {
		t.Fatalf("high/low mismatch: high=%s low=%s", ticker.High24h, ticker.Low24h)
	}
	if ticker.Volume24h != "120.5" {
		t.Fatalf("volume_24h mismatch: %s", ticker.Volume24h)
	}
	if ticker.ChangeRate24h == "0" {
		t.Fatalf("change_rate_24h should not be zero")
	}
}

func TestTickerSummaryZeroOpenPriceReturnsZeroChangeRate(t *testing.T) {
	m := NewReadModel()
	base := time.Date(2026, 4, 14, 9, 0, 0, 0, time.UTC)

	if err := m.ApplyTradeEvent(mkTradeEvent("t-zero", "0", "0.1", base)); err != nil {
		t.Fatalf("apply zero price trade: %v", err)
	}
	if err := m.ApplyTradeEvent(mkTradeEvent("t-next", "50000", "0.1", base.Add(time.Minute))); err != nil {
		t.Fatalf("apply second trade: %v", err)
	}

	ticker, err := m.TickerSummary(SupportedPair, base.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("ticker summary: %v", err)
	}
	if ticker.ChangeRate24h != "0" {
		t.Fatalf("expected zero change_rate_24h when open price is zero, got %s", ticker.ChangeRate24h)
	}
}
