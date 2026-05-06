package main

import (
	"encoding/json"
	"net/http"
	"time"
)

// User represents a mock user.
type User struct {
	ID       string
	Username string
	Password string
}

// mockUsers is the hardcoded user list. No database needed for the mock system.
var mockUsers = []User{
	{ID: "user-1", Username: "alice", Password: "password1"},
	{ID: "user-2", Username: "bob", Password: "password2"},
}

// devUser is the fixed mock user available when DEV_MODE=true.
var devUser = User{ID: "dev-user", Username: "dev", Password: "dev"}

// LoginRequest is the JSON body for POST /login.
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse is the JSON body returned on successful login.
type LoginResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// Handler holds HTTP handlers for the auth-service.
type Handler struct {
	jwtSecret   string
	tokenExpiry time.Duration
	devMode     bool
}

// NewHandler creates a Handler with the given configuration.
func NewHandler(jwtSecret string, tokenExpiry time.Duration, devMode bool) *Handler {
	return &Handler{
		jwtSecret:   jwtSecret,
		tokenExpiry: tokenExpiry,
		devMode:     devMode,
	}
}

// Health is the liveness probe endpoint.
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Ready is the readiness probe endpoint. Always 200 (no DB dependency).
func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

// Login handles POST /login. Validates credentials and returns a JWT.
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.Username == "" || req.Password == "" {
		http.Error(w, `{"error":"username and password are required"}`, http.StatusBadRequest)
		return
	}

	user := h.authenticate(req.Username, req.Password)
	if user == nil {
		http.Error(w, `{"error":"invalid credentials"}`, http.StatusUnauthorized)
		return
	}

	token, err := GenerateToken(user.ID, h.jwtSecret, h.tokenExpiry)
	if err != nil {
		http.Error(w, `{"error":"failed to generate token"}`, http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, LoginResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresIn:   int(h.tokenExpiry.Seconds()),
	})
}

// authenticate checks credentials against mock users.
// In DEV_MODE, the dev/dev credential is also accepted.
func (h *Handler) authenticate(username, password string) *User {
	if h.devMode && username == devUser.Username && password == devUser.Password {
		return &devUser
	}
	for i := range mockUsers {
		if mockUsers[i].Username == username && mockUsers[i].Password == password {
			return &mockUsers[i]
		}
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
