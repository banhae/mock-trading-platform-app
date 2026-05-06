package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
)

// Publisher is the minimal seam the HTTP handler uses to emit order events.
//
// We keep the interface narrow on purpose: one method, one envelope type.
// A slightly larger interface would invite premature generalization, and a
// full "event bus abstraction" is explicitly out of scope for Phase 2.
type Publisher interface {
	// PublishOrderEvent publishes an OrderEvent on the bus. Implementations
	// should treat this as best-effort for Phase 2: the caller has already
	// persisted the authoritative state and the return value is only used
	// for structured logging.
	PublishOrderEvent(ctx context.Context, event OrderEvent) error
	// PublishTradeEvent publishes a TradeEvent on the bus.
	PublishTradeEvent(ctx context.Context, event TradeEvent) error
}

// NoopPublisher is used when the event bus is disabled.
//
// It logs once at startup (via main.go) and then silently drops events.
// This keeps local development runnable without a NATS dependency.
type NoopPublisher struct{}

// PublishOrderEvent implements Publisher.
func (NoopPublisher) PublishOrderEvent(_ context.Context, event OrderEvent) error {
	slog.Debug("event bus disabled; dropping event",
		"type", event.Type,
		"order_id", event.Order.ID,
	)
	return nil
}

// PublishTradeEvent implements Publisher.
func (NoopPublisher) PublishTradeEvent(_ context.Context, event TradeEvent) error {
	slog.Debug("event bus disabled; dropping event",
		"type", event.Type,
		"trade_id", event.Trade.TradeID,
	)
	return nil
}

// NATSPublisher publishes order events to a NATS JetStream stream.
//
// Phase 2 constraints:
//   - Best-effort publish. DB success is authoritative; publish failure is
//     logged by the caller but does not fail the HTTP request.
//   - No transactional outbox, no retry orchestration, no exactly-once.
//   - The publisher tries to ensure the stream exists on startup, but does
//     not hard-fail if the stream is pre-created by an operator with a
//     stricter shape.
type NATSPublisher struct {
	nc     *nats.Conn
	js     nats.JetStreamContext
	stream string
}

func streamSubjects() []string {
	return []string{"order.>", "trade.>"}
}

func streamSubjectsCovered(existing, required []string) bool {
	existingSet := make(map[string]struct{}, len(existing))
	for _, s := range existing {
		existingSet[s] = struct{}{}
	}
	for _, s := range required {
		if _, ok := existingSet[s]; !ok {
			return false
		}
	}
	return true
}

func unionStreamSubjects(existing, required []string) []string {
	out := make([]string, 0, len(existing)+len(required))
	seen := make(map[string]struct{}, len(existing)+len(required))
	for _, s := range existing {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	for _, s := range required {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// NewNATSPublisher connects to NATS and returns a JetStream-backed Publisher.
//
// Stream management errors are fatal: if stream reconciliation fails at
// startup, we return an error to avoid silently dropping trade events in
// upgrade scenarios.
func NewNATSPublisher(url, stream string) (*NATSPublisher, error) {
	if url == "" {
		return nil, errors.New("nats url is empty")
	}
	if stream == "" {
		return nil, errors.New("nats stream is empty")
	}

	nc, err := nats.Connect(url,
		nats.Name("order-service publisher"),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("nats connect: %w", err)
	}

	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("jetstream context: %w", err)
	}

	p := &NATSPublisher{nc: nc, js: js, stream: stream}

	if err := p.ensureStream(); err != nil {
		nc.Close()
		return nil, fmt.Errorf("ensure stream: %w", err)
	}

	slog.Info("nats publisher ready", "stream", stream, "url", url)
	return p, nil
}

// ensureStream makes sure the configured stream exists and covers the
// order.* subjects we publish. If the stream already exists, we leave it
// alone — the operator may have extended it with additional subjects.
func (p *NATSPublisher) ensureStream() error {
	info, err := p.js.StreamInfo(p.stream)
	if err == nil {
		required := streamSubjects()
		if streamSubjectsCovered(info.Config.Subjects, required) {
			return nil
		}
		cfg := info.Config
		cfg.Subjects = unionStreamSubjects(info.Config.Subjects, required)
		if _, err := p.js.UpdateStream(&cfg); err != nil {
			return fmt.Errorf("update stream: %w", err)
		}
		return nil
	}
	if !errors.Is(err, nats.ErrStreamNotFound) {
		return fmt.Errorf("stream info: %w", err)
	}
	_, err = p.js.AddStream(&nats.StreamConfig{
		Name:      p.stream,
		Subjects:  streamSubjects(),
		Retention: nats.LimitsPolicy,
		Storage:   nats.FileStorage,
	})
	if err != nil {
		return fmt.Errorf("add stream: %w", err)
	}
	slog.Info("created jetstream stream", "stream", p.stream, "subjects", streamSubjects())
	return nil
}

// PublishOrderEvent marshals the envelope and publishes to JetStream.
//
// The ctx argument is accepted for interface parity but NATS JetStream
// Publish does not currently take a context; we rely on the underlying
// connection's timeouts. Deliberate: adding a wrapper goroutine here would
// complicate shutdown and buy us nothing for Phase 2.
func (p *NATSPublisher) PublishOrderEvent(_ context.Context, event OrderEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	if _, err := p.js.Publish(event.Type, data); err != nil {
		return fmt.Errorf("jetstream publish: %w", err)
	}
	return nil
}

// PublishTradeEvent marshals the envelope and publishes to JetStream.
func (p *NATSPublisher) PublishTradeEvent(_ context.Context, event TradeEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	if _, err := p.js.Publish(event.Type, data); err != nil {
		return fmt.Errorf("jetstream publish: %w", err)
	}
	return nil
}

// Close releases the underlying NATS connection. Safe to call multiple times.
func (p *NATSPublisher) Close() {
	if p == nil || p.nc == nil {
		return
	}
	p.nc.Close()
}
