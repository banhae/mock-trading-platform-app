package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"
)

type Handler struct {
	model *ReadModel
}

func NewHandler(model *ReadModel) *Handler {
	if model == nil {
		model = NewReadModel()
	}
	return &Handler{model: model}
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (h *Handler) Ticker(w http.ResponseWriter, r *http.Request) {
	pair := r.PathValue("pair")
	ticker, err := h.model.TickerSummary(pair, time.Now().UTC())
	if err != nil {
		writePairError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ticker)
}

func (h *Handler) OrderBook(w http.ResponseWriter, r *http.Request) {
	pair := r.PathValue("pair")
	depth, err := parseBoundedPositiveInt(r.URL.Query().Get("depth"), defaultDepth, maxDepth)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	book, err := h.model.OrderBook(pair, depth, time.Now().UTC())
	if err != nil {
		writePairError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, book)
}

func (h *Handler) Candles(w http.ResponseWriter, r *http.Request) {
	pair := r.PathValue("pair")
	interval := r.URL.Query().Get("interval")
	if interval == "" {
		interval = "1m"
	}
	limit, err := parseBoundedPositiveInt(r.URL.Query().Get("limit"), defaultCandleLimit, maxCandleLimit)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	candles, err := h.model.Candles(pair, interval, limit)
	if err != nil {
		if err == errUnsupportedPair {
			writePairError(w, err)
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, candles)
}

func (h *Handler) Trades(w http.ResponseWriter, r *http.Request) {
	pair := r.PathValue("pair")
	limit, err := parseBoundedPositiveInt(r.URL.Query().Get("limit"), defaultTradesLimit, maxTradesLimit)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	trades, err := h.model.RecentTrades(pair, limit)
	if err != nil {
		writePairError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, trades)
}

func parseIntDefault(raw string, def int) int {
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return v
}

func parseBoundedPositiveInt(raw string, def, max int) (int, error) {
	v := def
	if raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			return 0, errors.New("value must be integer")
		}
		v = parsed
	}
	if v <= 0 {
		return 0, errors.New("value must be > 0")
	}
	if v > max {
		return 0, errors.New("value exceeds max")
	}
	return v, nil
}

func writePairError(w http.ResponseWriter, err error) {
	if err == errUnsupportedPair {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported pair"})
		return
	}
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
