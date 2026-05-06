package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHealth(t *testing.T) {
	h := NewHandler(NewReadModel())
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	h.Health(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestReady(t *testing.T) {
	h := NewHandler(NewReadModel())
	req := httptest.NewRequest("GET", "/ready", nil)
	w := httptest.NewRecorder()
	h.Ready(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestTickerEndpointReturnsSummaryFields(t *testing.T) {
	m := NewReadModel()
	_ = m.ApplyTradeEvent(mkTradeEvent("t1", "50000", "0.2", time.Now().UTC()))
	h := NewHandler(m)

	req := httptest.NewRequest("GET", "/marketdata/ticker/BTC-KRW", nil)
	w := httptest.NewRecorder()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /marketdata/ticker/{pair}", h.Ticker)
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: %d body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, key := range []string{"last_price", "change_rate_24h", "high_24h", "low_24h", "volume_24h", "as_of"} {
		if !strings.Contains(body, key) {
			t.Fatalf("missing field %s in %s", key, body)
		}
	}
}

func TestOrderBookEndpointRespectsDepth(t *testing.T) {
	m := NewReadModel()
	_ = m.ApplyOrderEvent(mkOrderEvent("o1", "buy", "open", "50000", "1"))
	_ = m.ApplyOrderEvent(mkOrderEvent("o2", "buy", "open", "49900", "1"))
	h := NewHandler(m)

	req := httptest.NewRequest("GET", "/marketdata/orderbook/BTC-KRW?depth=1", nil)
	w := httptest.NewRecorder()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /marketdata/orderbook/{pair}", h.OrderBook)
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: %d", w.Code)
	}
	var resp OrderBookResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Bids) != 1 || resp.Bids[0].Price != "50000" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestCandlesEndpointReturnsBucketsInOrder(t *testing.T) {
	m := NewReadModel()
	t0 := time.Now().UTC().Truncate(time.Minute)
	_ = m.ApplyTradeEvent(mkTradeEvent("t1", "50000", "0.1", t0))
	_ = m.ApplyTradeEvent(mkTradeEvent("t2", "50500", "0.1", t0.Add(2*time.Minute)))
	h := NewHandler(m)

	req := httptest.NewRequest("GET", "/marketdata/candles/BTC-KRW?interval=1m&limit=2", nil)
	w := httptest.NewRecorder()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /marketdata/candles/{pair}", h.Candles)
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: %d body=%s", w.Code, w.Body.String())
	}
	var candles CandlesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &candles); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(candles.Candles) != 2 || !candles.Candles[0].Timestamp.Before(candles.Candles[1].Timestamp) {
		t.Fatalf("unexpected candles ordering: %+v", candles)
	}
}

func TestTradesEndpointReturnsRecentFirst(t *testing.T) {
	m := NewReadModel()
	now := time.Now().UTC()
	_ = m.ApplyTradeEvent(mkTradeEvent("t1", "50000", "0.1", now))
	_ = m.ApplyTradeEvent(mkTradeEvent("t2", "50100", "0.1", now.Add(time.Second)))
	h := NewHandler(m)

	req := httptest.NewRequest("GET", "/marketdata/trades/BTC-KRW?limit=1", nil)
	w := httptest.NewRecorder()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /marketdata/trades/{pair}", h.Trades)
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: %d", w.Code)
	}
	var trades TradesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &trades); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(trades.Trades) != 1 || trades.Trades[0].TradeID != "t2" {
		t.Fatalf("unexpected trades: %+v", trades)
	}
}

func TestEndpointsRejectUnsupportedPair(t *testing.T) {
	h := NewHandler(NewReadModel())
	for _, path := range []string{"/marketdata/ticker/ETH-KRW", "/marketdata/orderbook/ETH-KRW", "/marketdata/candles/ETH-KRW", "/marketdata/trades/ETH-KRW"} {
		req := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		mux := http.NewServeMux()
		mux.HandleFunc("GET /marketdata/ticker/{pair}", h.Ticker)
		mux.HandleFunc("GET /marketdata/orderbook/{pair}", h.OrderBook)
		mux.HandleFunc("GET /marketdata/candles/{pair}", h.Candles)
		mux.HandleFunc("GET /marketdata/trades/{pair}", h.Trades)
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("path %s expected 400 got %d", path, w.Code)
		}
	}
}

func TestEndpointsEmptyStateReturns200(t *testing.T) {
	h := NewHandler(NewReadModel())
	paths := []string{
		"/marketdata/ticker/BTC-KRW",
		"/marketdata/orderbook/BTC-KRW",
		"/marketdata/candles/BTC-KRW",
		"/marketdata/trades/BTC-KRW",
	}
	for _, path := range paths {
		req := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		mux := http.NewServeMux()
		mux.HandleFunc("GET /marketdata/ticker/{pair}", h.Ticker)
		mux.HandleFunc("GET /marketdata/orderbook/{pair}", h.OrderBook)
		mux.HandleFunc("GET /marketdata/candles/{pair}", h.Candles)
		mux.HandleFunc("GET /marketdata/trades/{pair}", h.Trades)
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("path %s expected 200 got %d", path, w.Code)
		}
	}
}

func TestEndpointsValidation(t *testing.T) {
	h := NewHandler(NewReadModel())
	invalid := []string{
		"/marketdata/orderbook/BTC-KRW?depth=0",
		"/marketdata/orderbook/BTC-KRW?depth=abc",
		"/marketdata/orderbook/BTC-KRW?depth=51",
		"/marketdata/trades/BTC-KRW?limit=0",
		"/marketdata/trades/BTC-KRW?limit=201",
		"/marketdata/candles/BTC-KRW?interval=2m",
		"/marketdata/candles/BTC-KRW?limit=0",
		"/marketdata/candles/BTC-KRW?limit=501",
	}
	for _, path := range invalid {
		req := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		mux := http.NewServeMux()
		mux.HandleFunc("GET /marketdata/orderbook/{pair}", h.OrderBook)
		mux.HandleFunc("GET /marketdata/candles/{pair}", h.Candles)
		mux.HandleFunc("GET /marketdata/trades/{pair}", h.Trades)
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("path %s expected 400 got %d", path, w.Code)
		}
	}
}
