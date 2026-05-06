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

	// Phase 2 — start consumer when the event bus is enabled. Otherwise
	// run in no-op mode so local dev without NATS still works.
	var consumer *Consumer
	if cfg.EventBusEnabled {
		consumer = NewConsumer()
		if err := consumer.Start(context.Background(), cfg.NATSURL, cfg.NATSStream, cfg.NATSDurable); err != nil {
			slog.Error("failed to start nats consumer", "error", err)
			os.Exit(1)
		}
		defer consumer.Stop()
	} else {
		slog.Info("event bus disabled; wallet consumer is in no-op mode")
	}

	h := NewHandler()

	mux := http.NewServeMux()

	// Kubelet probe paths (no prefix)
	mux.HandleFunc("GET /health", h.Health)
	mux.HandleFunc("GET /ready", h.Ready)

	// Ingress-routed paths (matches mock-trading-platform-ingress chart /api/wallet prefix)
	mux.HandleFunc("GET /api/wallet/health", h.Health)
	mux.HandleFunc("GET /api/wallet/ready", h.Ready)

	mux.Handle("GET /metrics", promhttp.Handler())

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      MetricsMiddleware(mux),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("starting wallet-service", "port", cfg.Port, "dev_mode", cfg.DevMode)
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
