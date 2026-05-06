package main

import (
	"encoding/json"
	"net/http"
)

// Handler holds HTTP handlers for the wallet-service.
type Handler struct{}

// NewHandler creates a Handler.
func NewHandler() *Handler {
	return &Handler{}
}

// Health is the liveness probe endpoint.
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Ready is the readiness probe endpoint.
func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
