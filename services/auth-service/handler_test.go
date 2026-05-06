package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "test-secret"
const testExpiry = 1 * time.Hour

func TestHealth(t *testing.T) {
	h := NewHandler(testSecret, testExpiry, false)
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	h.Health(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestReady(t *testing.T) {
	h := NewHandler(testSecret, testExpiry, false)
	req := httptest.NewRequest("GET", "/ready", nil)
	w := httptest.NewRecorder()
	h.Ready(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestLoginSuccess(t *testing.T) {
	h := NewHandler(testSecret, testExpiry, false)

	body, _ := json.Marshal(LoginRequest{Username: "alice", Password: "password1"})
	req := httptest.NewRequest("POST", "/login", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.Login(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp LoginResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.TokenType != "Bearer" {
		t.Errorf("expected token_type Bearer, got %s", resp.TokenType)
	}
	if resp.ExpiresIn != 3600 {
		t.Errorf("expected expires_in 3600, got %d", resp.ExpiresIn)
	}
	if resp.AccessToken == "" {
		t.Error("expected non-empty access_token")
	}

	// Verify the token is valid and contains the correct sub claim
	token, err := jwt.Parse(resp.AccessToken, func(t *jwt.Token) (interface{}, error) {
		return []byte(testSecret), nil
	})
	if err != nil {
		t.Fatalf("failed to parse token: %v", err)
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatal("failed to get claims")
	}
	sub, ok := claims["sub"].(string)
	if !ok || sub != "user-1" {
		t.Errorf("expected sub user-1, got %s", sub)
	}
}

func TestLoginInvalidCredentials(t *testing.T) {
	h := NewHandler(testSecret, testExpiry, false)

	body, _ := json.Marshal(LoginRequest{Username: "alice", Password: "wrong"})
	req := httptest.NewRequest("POST", "/login", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.Login(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestLoginMissingFields(t *testing.T) {
	h := NewHandler(testSecret, testExpiry, false)

	body, _ := json.Marshal(LoginRequest{Username: "alice"})
	req := httptest.NewRequest("POST", "/login", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.Login(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestLoginDevMode(t *testing.T) {
	h := NewHandler(testSecret, testExpiry, true)

	body, _ := json.Marshal(LoginRequest{Username: "dev", Password: "dev"})
	req := httptest.NewRequest("POST", "/login", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.Login(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp LoginResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify dev-user ID in token
	token, err := jwt.Parse(resp.AccessToken, func(t *jwt.Token) (interface{}, error) {
		return []byte(testSecret), nil
	})
	if err != nil {
		t.Fatalf("failed to parse token: %v", err)
	}
	claims := token.Claims.(jwt.MapClaims)
	sub := claims["sub"].(string)
	if sub != "dev-user" {
		t.Errorf("expected sub dev-user, got %s", sub)
	}
}

func TestLoginDevModeRejectsWrongPassword(t *testing.T) {
	h := NewHandler(testSecret, testExpiry, true)

	body, _ := json.Marshal(LoginRequest{Username: "dev", Password: "wrong"})
	req := httptest.NewRequest("POST", "/login", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.Login(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestLoginDevModeAllowsMockUsers(t *testing.T) {
	h := NewHandler(testSecret, testExpiry, true)

	body, _ := json.Marshal(LoginRequest{Username: "bob", Password: "password2"})
	req := httptest.NewRequest("POST", "/login", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.Login(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}
