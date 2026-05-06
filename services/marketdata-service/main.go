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
	model := NewReadModel()

	var consumer *Consumer
	if cfg.EventBusEnabled {
		consumer = NewConsumer(model)
		if err := consumer.Start(context.Background(), cfg.NATSURL, cfg.NATSStream, cfg.NATSDurable); err != nil {
			slog.Error("failed to start nats consumer", "error", err)
			os.Exit(1)
		}
		defer consumer.Stop()
	} else {
		slog.Info("event bus disabled; marketdata consumer is in no-op mode")
	}

	h := NewHandler(model)

	mux := http.NewServeMux()

	// Kubelet probe paths (no prefix)
	mux.HandleFunc("GET /health", h.Health)
	mux.HandleFunc("GET /ready", h.Ready)
	mux.HandleFunc("GET /marketdata/ticker/{pair}", h.Ticker)
	mux.HandleFunc("GET /marketdata/orderbook/{pair}", h.OrderBook)
	mux.HandleFunc("GET /marketdata/candles/{pair}", h.Candles)
	mux.HandleFunc("GET /marketdata/trades/{pair}", h.Trades)

	// Ingress-routed paths (matches mock-trading-platform-ingress chart /api/market prefix)
	mux.HandleFunc("GET /api/market/health", h.Health)
	mux.HandleFunc("GET /api/market/ready", h.Ready)
	mux.HandleFunc("GET /api/market/ticker/{pair}", h.Ticker)
	mux.HandleFunc("GET /api/market/orderbook/{pair}", h.OrderBook)
	mux.HandleFunc("GET /api/market/candles/{pair}", h.Candles)
	mux.HandleFunc("GET /api/market/trades/{pair}", h.Trades)

	mux.Handle("GET /metrics", promhttp.Handler())

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      MetricsMiddleware(mux),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("starting marketdata-service", "port", cfg.Port, "dev_mode", cfg.DevMode)
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
