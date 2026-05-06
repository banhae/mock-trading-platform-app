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

	store, err := NewPostgresStore(cfg.DatabaseURL)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	// DEV ONLY: auto-create tables when DEV_MODE is enabled.
	// 운영 환경에서는 마이그레이션 도구로 분리할 것.
	if cfg.DevMode {
		if err := store.EnsureTable(context.Background()); err != nil {
			slog.Error("failed to ensure table", "error", err)
			os.Exit(1)
		}
	}

	// Phase 2: event publisher. When the event bus is disabled we fall back
	// to a no-op publisher so local development does not require NATS.
	var publisher Publisher = NoopPublisher{}
	var natsPub *NATSPublisher
	if cfg.EventBusEnabled {
		np, err := NewNATSPublisher(cfg.NATSURL, cfg.NATSStream)
		if err != nil {
			slog.Error("failed to start nats publisher", "error", err)
			os.Exit(1)
		}
		natsPub = np
		publisher = np
	} else {
		slog.Info("event bus disabled; using no-op publisher")
	}
	defer func() {
		if natsPub != nil {
			natsPub.Close()
		}
	}()

	runtime, err := NewOrderRuntime(context.Background(), store, publisher)
	if err != nil {
		slog.Error("failed to initialize matcher runtime", "error", err)
		os.Exit(1)
	}
	h := NewHandlerWithRuntime(store, runtime, publisher)
	auth := AuthMiddleware(cfg.JWTSecret, cfg.DevMode)

	mux := http.NewServeMux()

	// Kubelet probe paths (no prefix)
	mux.HandleFunc("GET /health", h.Health)
	mux.HandleFunc("GET /ready", h.Ready)
	mux.Handle("POST /orders", auth(http.HandlerFunc(h.CreateOrder)))
	mux.Handle("GET /orders", auth(http.HandlerFunc(h.ListOrders)))
	mux.Handle("GET /orders/{id}", auth(http.HandlerFunc(h.GetOrder)))
	mux.Handle("DELETE /orders/{id}", auth(http.HandlerFunc(h.CancelOrder)))

	// Ingress-routed paths (matches mock-trading-platform-ingress chart /api/orders prefix)
	mux.HandleFunc("GET /api/orders/health", h.Health)
	mux.HandleFunc("GET /api/orders/ready", h.Ready)
	mux.Handle("POST /api/orders", auth(http.HandlerFunc(h.CreateOrder)))
	mux.Handle("GET /api/orders", auth(http.HandlerFunc(h.ListOrders)))
	mux.Handle("GET /api/orders/{id}", auth(http.HandlerFunc(h.GetOrder)))
	mux.Handle("DELETE /api/orders/{id}", auth(http.HandlerFunc(h.CancelOrder)))

	// Prometheus scrape endpoint. Exposed on the same port; PodMonitor in
	// mock-trading-platform-gitops scrapes this path. Excluded from the metrics middleware
	// to avoid self-instrumentation noise.
	mux.Handle("GET /metrics", promhttp.Handler())

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      MetricsMiddleware(mux),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("starting order-service", "port", cfg.Port, "dev_mode", cfg.DevMode)
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
