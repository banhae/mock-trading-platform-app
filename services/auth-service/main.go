package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	cfg := LoadConfig()

	h := NewHandler(cfg.JWTSecret, cfg.TokenExpiry, cfg.DevMode)

	mux := http.NewServeMux()

	// Kubelet probe paths (no prefix)
	mux.HandleFunc("GET /health", h.Health)
	mux.HandleFunc("GET /ready", h.Ready)
	mux.HandleFunc("POST /login", h.Login)

	// Ingress-routed paths (matches mock-trading-platform-ingress chart /api/auth prefix)
	mux.HandleFunc("GET /api/auth/health", h.Health)
	mux.HandleFunc("GET /api/auth/ready", h.Ready)
	mux.HandleFunc("POST /api/auth/login", h.Login)

	mux.Handle("GET /metrics", promhttp.Handler())

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      MetricsMiddleware(mux),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("starting auth-service", "port", cfg.Port, "dev_mode", cfg.DevMode, "token_expiry", cfg.TokenExpiry.String())
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down server")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("server shutdown error", "error", err)
	}
}
