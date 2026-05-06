package main

import (
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	SupportedPair      = "BTC-KRW"
	quantityScale      = int64(100000000)
	maxRecentTrades    = 1000
	maxCandlesPerFrame = 3000
	defaultDepth       = 20
	maxDepth           = 50
	defaultTradesLimit = 50
	maxTradesLimit     = 200
	defaultCandleLimit = 200
	maxCandleLimit     = 500
)

var errUnsupportedPair = errors.New("unsupported pair")

type tradeItem struct {
	TradeID      string
	Price        int64
	Quantity     int64
	MakerOrderID string
	TakerOrderID string
	ExecutedAt   time.Time
}

type orderItem struct {
	ID        string
	Pair      string
	Side      string
	Price     int64
	Remaining int64
	Status    string
}

type candleBucket struct {
	StartUnix    int64
	Open         int64
	High         int64
	Low          int64
	Close        int64
	Volume       int64
	QuoteVolumeX *big.Int // quote volume scaled by 1e8
}

type candleFrame struct {
	Buckets map[int64]*candleBucket
	Starts  []int64
}

type ReadModel struct {
	mu sync.RWMutex

	orders map[string]orderItem
	bids   map[int64]int64
	asks   map[int64]int64

	recentTrades []tradeItem // newest first
	tickerTrades []tradeItem // oldest first; 24h summary window

	candles map[string]*candleFrame
}

func NewReadModel() *ReadModel {
	return &ReadModel{
		orders:       make(map[string]orderItem),
		bids:         make(map[int64]int64),
		asks:         make(map[int64]int64),
		recentTrades: make([]tradeItem, 0, 256),
		tickerTrades: make([]tradeItem, 0, 1024),
		candles: map[string]*candleFrame{
			"1m": {Buckets: map[int64]*candleBucket{}, Starts: make([]int64, 0, 256)},
			"5m": {Buckets: map[int64]*candleBucket{}, Starts: make([]int64, 0, 256)},
			"1h": {Buckets: map[int64]*candleBucket{}, Starts: make([]int64, 0, 256)},
		},
	}
}

func (m *ReadModel) ApplyOrderEvent(ev OrderEvent) error {
	if ev.Order.Pair != SupportedPair {
		return nil
	}
	price, err := parsePrice(ev.Order.Price)
	if err != nil {
		return err
	}
	remaining, err := parseQuantity(ev.Order.RemainingQuantity)
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if prev, ok := m.orders[ev.Order.ID]; ok {
		m.subDepth(prev)
	}

	curr := orderItem{
		ID:        ev.Order.ID,
		Pair:      ev.Order.Pair,
		Side:      ev.Order.Side,
		Price:     price,
		Remaining: remaining,
		Status:    ev.Order.Status,
	}
	m.orders[ev.Order.ID] = curr
	if isDepthActive(curr.Status) && curr.Remaining > 0 {
		m.addDepth(curr)
	}
	return nil
}

func (m *ReadModel) ApplyTradeEvent(ev TradeEvent) error {
	if ev.Trade.Pair != SupportedPair {
		return nil
	}
	price, err := parsePrice(ev.Trade.Price)
	if err != nil {
		return err
	}
	qty, err := parseQuantity(ev.Trade.Quantity)
	if err != nil {
		return err
	}
	trade := tradeItem{
		TradeID:      ev.Trade.TradeID,
		Price:        price,
		Quantity:     qty,
		MakerOrderID: ev.Trade.MakerOrderID,
		TakerOrderID: ev.Trade.TakerOrderID,
		ExecutedAt:   ev.Trade.ExecutedAt.UTC(),
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.recentTrades = append([]tradeItem{trade}, m.recentTrades...)
	if len(m.recentTrades) > maxRecentTrades {
		m.recentTrades = m.recentTrades[:maxRecentTrades]
	}
	m.tickerTrades = append(m.tickerTrades, trade)
	m.pruneTickerTradesLocked(trade.ExecutedAt)

	for _, interval := range []string{"1m", "5m", "1h"} {
		m.applyTradeToCandle(interval, trade)
	}
	return nil
}

func (m *ReadModel) addDepth(o orderItem) {
	if strings.EqualFold(o.Side, "buy") {
		m.bids[o.Price] += o.Remaining
		if m.bids[o.Price] <= 0 {
			delete(m.bids, o.Price)
		}
		return
	}
	m.asks[o.Price] += o.Remaining
	if m.asks[o.Price] <= 0 {
		delete(m.asks, o.Price)
	}
}

func (m *ReadModel) subDepth(o orderItem) {
	if !isDepthActive(o.Status) || o.Remaining <= 0 {
		return
	}
	if strings.EqualFold(o.Side, "buy") {
		m.bids[o.Price] -= o.Remaining
		if m.bids[o.Price] <= 0 {
			delete(m.bids, o.Price)
		}
		return
	}
	m.asks[o.Price] -= o.Remaining
	if m.asks[o.Price] <= 0 {
		delete(m.asks, o.Price)
	}
}

func isDepthActive(status string) bool {
	return status == "open" || status == "partially_filled"
}

func intervalDuration(interval string) (time.Duration, bool) {
	switch interval {
	case "1m":
		return time.Minute, true
	case "5m":
		return 5 * time.Minute, true
	case "1h":
		return time.Hour, true
	default:
		return 0, false
	}
}

func (m *ReadModel) applyTradeToCandle(interval string, trade tradeItem) {
	frame, ok := m.candles[interval]
	if !ok {
		return
	}
	dur, _ := intervalDuration(interval)
	start := trade.ExecutedAt.Truncate(dur).Unix()
	bucket, exists := frame.Buckets[start]
	if !exists {
		bucket = &candleBucket{
			StartUnix:    start,
			Open:         trade.Price,
			High:         trade.Price,
			Low:          trade.Price,
			Close:        trade.Price,
			Volume:       0,
			QuoteVolumeX: big.NewInt(0),
		}
		frame.Buckets[start] = bucket
		frame.Starts = append(frame.Starts, start)
		sort.Slice(frame.Starts, func(i, j int) bool { return frame.Starts[i] < frame.Starts[j] })
	}
	if !exists {
		bucket.Open = trade.Price
		bucket.High = trade.Price
		bucket.Low = trade.Price
	}
	if trade.Price > bucket.High {
		bucket.High = trade.Price
	}
	if trade.Price < bucket.Low {
		bucket.Low = trade.Price
	}
	bucket.Close = trade.Price
	bucket.Volume += trade.Quantity

	quote := big.NewInt(trade.Price)
	quote.Mul(quote, big.NewInt(trade.Quantity))
	bucket.QuoteVolumeX.Add(bucket.QuoteVolumeX, quote)

	for len(frame.Starts) > maxCandlesPerFrame {
		oldest := frame.Starts[0]
		frame.Starts = frame.Starts[1:]
		delete(frame.Buckets, oldest)
	}
}

type PriceLevel struct {
	Price    string `json:"price"`
	Quantity string `json:"quantity"`
}

type TickerSummary struct {
	Pair          string    `json:"pair"`
	LastPrice     string    `json:"last_price"`
	ChangeRate24h string    `json:"change_rate_24h"`
	High24h       string    `json:"high_24h"`
	Low24h        string    `json:"low_24h"`
	Volume24h     string    `json:"volume_24h"`
	AsOf          time.Time `json:"as_of"`
}

type TradeItemResponse struct {
	TradeID      string    `json:"trade_id"`
	Pair         string    `json:"pair"`
	Price        string    `json:"price"`
	Quantity     string    `json:"quantity"`
	MakerOrderID string    `json:"maker_order_id"`
	TakerOrderID string    `json:"taker_order_id"`
	ExecutedAt   time.Time `json:"executed_at"`
}

type TradesResponse struct {
	Pair   string              `json:"pair"`
	Trades []TradeItemResponse `json:"trades"`
}

type CandleItemResponse struct {
	Timestamp time.Time `json:"timestamp"`
	Open      string    `json:"open"`
	High      string    `json:"high"`
	Low       string    `json:"low"`
	Close     string    `json:"close"`
	Volume    string    `json:"volume"`
}

type CandlesResponse struct {
	Pair     string               `json:"pair"`
	Interval string               `json:"interval"`
	Candles  []CandleItemResponse `json:"candles"`
}

type OrderBookResponse struct {
	Pair  string       `json:"pair"`
	Depth int          `json:"depth"`
	Bids  []PriceLevel `json:"bids"`
	Asks  []PriceLevel `json:"asks"`
	AsOf  time.Time    `json:"as_of"`
}

func (m *ReadModel) TickerSummary(pair string, now time.Time) (TickerSummary, error) {
	if pair != SupportedPair {
		return TickerSummary{}, errUnsupportedPair
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	summary := TickerSummary{Pair: pair, LastPrice: "0", ChangeRate24h: "0", High24h: "0", Low24h: "0", Volume24h: "0", AsOf: now.UTC()}
	if len(m.recentTrades) == 0 {
		return summary, nil
	}
	summary.LastPrice = formatPrice(m.recentTrades[0].Price)
	m.pruneTickerTradesLocked(now.UTC())

	if len(m.tickerTrades) == 0 {
		return summary, nil
	}
	openPrice := m.tickerTrades[0].Price
	lastPrice := m.tickerTrades[len(m.tickerTrades)-1].Price
	high := m.tickerTrades[0].Price
	low := m.tickerTrades[0].Price
	volume := int64(0)
	for _, tr := range m.tickerTrades {
		if tr.Price > high {
			high = tr.Price
		}
		if tr.Price < low {
			low = tr.Price
		}
		volume += tr.Quantity
	}
	if openPrice > 0 {
		change := new(big.Rat).SetFrac(big.NewInt(lastPrice-openPrice), big.NewInt(openPrice))
		summary.ChangeRate24h = formatRat(change, 8)
	}
	summary.High24h = formatPrice(high)
	summary.Low24h = formatPrice(low)
	summary.Volume24h = formatQuantity(volume)
	return summary, nil
}

func (m *ReadModel) pruneTickerTradesLocked(ref time.Time) {
	cutoff := ref.Add(-24 * time.Hour)
	drop := 0
	for drop < len(m.tickerTrades) && m.tickerTrades[drop].ExecutedAt.Before(cutoff) {
		drop++
	}
	if drop > 0 {
		m.tickerTrades = append([]tradeItem(nil), m.tickerTrades[drop:]...)
	}
}

func (m *ReadModel) OrderBook(pair string, depth int, asOf time.Time) (OrderBookResponse, error) {
	if pair != SupportedPair {
		return OrderBookResponse{}, errUnsupportedPair
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	bidPrices := make([]int64, 0, len(m.bids))
	for p := range m.bids {
		bidPrices = append(bidPrices, p)
	}
	sort.Slice(bidPrices, func(i, j int) bool { return bidPrices[i] > bidPrices[j] })
	askPrices := make([]int64, 0, len(m.asks))
	for p := range m.asks {
		askPrices = append(askPrices, p)
	}
	sort.Slice(askPrices, func(i, j int) bool { return askPrices[i] < askPrices[j] })

	resp := OrderBookResponse{Pair: pair, Depth: depth, Bids: make([]PriceLevel, 0, depth), Asks: make([]PriceLevel, 0, depth), AsOf: asOf.UTC()}
	for i, p := range bidPrices {
		if i >= depth {
			break
		}
		resp.Bids = append(resp.Bids, PriceLevel{Price: formatPrice(p), Quantity: formatQuantity(m.bids[p])})
	}
	for i, p := range askPrices {
		if i >= depth {
			break
		}
		resp.Asks = append(resp.Asks, PriceLevel{Price: formatPrice(p), Quantity: formatQuantity(m.asks[p])})
	}
	return resp, nil
}

func (m *ReadModel) RecentTrades(pair string, limit int) (TradesResponse, error) {
	if pair != SupportedPair {
		return TradesResponse{}, errUnsupportedPair
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if limit > len(m.recentTrades) {
		limit = len(m.recentTrades)
	}
	out := make([]TradeItemResponse, 0, limit)
	for i := 0; i < limit; i++ {
		tr := m.recentTrades[i]
		out = append(out, TradeItemResponse{
			TradeID:      tr.TradeID,
			Pair:         pair,
			Price:        formatPrice(tr.Price),
			Quantity:     formatQuantity(tr.Quantity),
			MakerOrderID: tr.MakerOrderID,
			TakerOrderID: tr.TakerOrderID,
			ExecutedAt:   tr.ExecutedAt,
		})
	}
	return TradesResponse{Pair: pair, Trades: out}, nil
}

func (m *ReadModel) Candles(pair, interval string, limit int) (CandlesResponse, error) {
	if pair != SupportedPair {
		return CandlesResponse{}, errUnsupportedPair
	}
	if _, ok := intervalDuration(interval); !ok {
		return CandlesResponse{}, fmt.Errorf("unsupported interval")
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	frame := m.candles[interval]
	if frame == nil {
		return CandlesResponse{Pair: pair, Interval: interval, Candles: []CandleItemResponse{}}, nil
	}
	starts := frame.Starts
	if limit > len(starts) {
		limit = len(starts)
	}
	starts = starts[len(starts)-limit:]
	out := make([]CandleItemResponse, 0, len(starts))
	for _, ts := range starts {
		b := frame.Buckets[ts]
		out = append(out, CandleItemResponse{
			Timestamp: time.Unix(ts, 0).UTC(),
			Open:      formatPrice(b.Open),
			High:      formatPrice(b.High),
			Low:       formatPrice(b.Low),
			Close:     formatPrice(b.Close),
			Volume:    formatQuantity(b.Volume),
		})
	}
	return CandlesResponse{Pair: pair, Interval: interval, Candles: out}, nil
}

func parsePrice(s string) (int64, error) {
	if strings.Contains(s, ".") {
		return 0, fmt.Errorf("price must be integer string")
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil || v < 0 {
		return 0, fmt.Errorf("invalid price: %q", s)
	}
	return v, nil
}

func parseQuantity(s string) (int64, error) {
	return parseDecimalToScaled(s, 8)
}

func parseDecimalToScaled(s string, scale int) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("empty decimal")
	}
	neg := false
	if strings.HasPrefix(s, "-") {
		neg = true
		s = s[1:]
	}
	parts := strings.Split(s, ".")
	if len(parts) > 2 {
		return 0, fmt.Errorf("invalid decimal: %q", s)
	}
	whole := parts[0]
	if whole == "" {
		whole = "0"
	}
	frac := ""
	if len(parts) == 2 {
		frac = parts[1]
	}
	if len(frac) > scale {
		return 0, fmt.Errorf("too many fractional digits: %q", s)
	}
	frac = frac + strings.Repeat("0", scale-len(frac))
	combined := whole + frac
	if combined == "" {
		combined = "0"
	}
	v, err := strconv.ParseInt(combined, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse scaled int: %w", err)
	}
	if neg {
		v = -v
	}
	if v < 0 {
		return 0, fmt.Errorf("negative not allowed")
	}
	return v, nil
}

func formatPrice(v int64) string {
	return strconv.FormatInt(v, 10)
}

func formatSignedPrice(v int64) string {
	if v >= 0 {
		return strconv.FormatInt(v, 10)
	}
	return "-" + strconv.FormatInt(-v, 10)
}

func formatQuantity(v int64) string {
	return formatScaledInt(v, 8)
}

func formatScaledInt(v int64, scale int) string {
	sign := ""
	if v < 0 {
		sign = "-"
		v = -v
	}
	pow := int64(1)
	for range scale {
		pow *= 10
	}
	whole := v / pow
	frac := v % pow
	if frac == 0 {
		return sign + strconv.FormatInt(whole, 10)
	}
	fracFmt := fmt.Sprintf("%0*d", scale, frac)
	fracFmt = strings.TrimRight(fracFmt, "0")
	return sign + strconv.FormatInt(whole, 10) + "." + fracFmt
}

func formatScaledBig(v *big.Int, scale int) string {
	if v == nil {
		return "0"
	}
	sign := ""
	t := new(big.Int).Set(v)
	if t.Sign() < 0 {
		sign = "-"
		t.Abs(t)
	}
	pow := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(scale)), nil)
	whole := new(big.Int).Div(t, pow)
	frac := new(big.Int).Mod(t, pow)
	if frac.Sign() == 0 {
		return sign + whole.String()
	}
	fracFmt := fmt.Sprintf("%0*s", scale, frac.String())
	fracFmt = strings.TrimRight(fracFmt, "0")
	return sign + whole.String() + "." + fracFmt
}

func formatRat(r *big.Rat, precision int) string {
	if r == nil {
		return "0"
	}
	return r.FloatString(precision)
}
