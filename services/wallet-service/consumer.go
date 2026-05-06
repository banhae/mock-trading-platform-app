package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
)

// Counters exposes the minimal in-memory metrics the wallet consumer keeps
// about the events it has processed. In Phase 2 we do not implement balance
// hold/release — counters are the only observable side effect.
type Counters struct {
	Created atomic.Uint64
	Updated atomic.Uint64
	Unknown atomic.Uint64
	Failed  atomic.Uint64
}

// Snapshot returns a point-in-time copy of the counters.
func (c *Counters) Snapshot() CountersSnapshot {
	return CountersSnapshot{
		Created: c.Created.Load(),
		Updated: c.Updated.Load(),
		Unknown: c.Unknown.Load(),
		Failed:  c.Failed.Load(),
	}
}

// CountersSnapshot is a plain-value view suited for tests and logging.
type CountersSnapshot struct {
	Created uint64
	Updated uint64
	Unknown uint64
	Failed  uint64
}

// Consumer subscribes to order events and updates in-memory counters.
//
// Intentionally minimal: parse -> log structured metadata -> increment
// counter. No balance logic, no persistence, no API surface.
type Consumer struct {
	counters Counters

	nc  *nats.Conn
	sub *nats.Subscription

	closeOnce sync.Once
}

// NewConsumer creates a Consumer that is not yet connected to NATS.
// Call Start to connect. For unit tests, Handle can be invoked directly
// without going through Start.
func NewConsumer() *Consumer {
	return &Consumer{}
}

// CountersSnapshot returns a copy of the current counters.
func (c *Consumer) CountersSnapshot() CountersSnapshot {
	return c.counters.Snapshot()
}

// Start connects to NATS, ensures the stream exists, binds a durable
// consumer and begins receiving messages.
func (c *Consumer) Start(_ context.Context, url, stream, durable string) error {
	if url == "" {
		return errors.New("nats url is empty")
	}
	if stream == "" {
		return errors.New("nats stream is empty")
	}
	if durable == "" {
		durable = "wallet-consumer"
	}

	nc, err := nats.Connect(url,
		nats.Name("wallet-service consumer"),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
	)
	if err != nil {
		return fmt.Errorf("nats connect: %w", err)
	}

	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return fmt.Errorf("jetstream context: %w", err)
	}

	if err := ensureStream(js, stream); err != nil {
		nc.Close()
		return fmt.Errorf("ensure stream: %w", err)
	}

	sub, err := js.Subscribe("order.*", func(m *nats.Msg) {
		if err := c.Handle(m.Subject, m.Data); err != nil {
			c.counters.Failed.Add(1)
			slog.Error("failed to handle event",
				"subject", m.Subject,
				"error", err,
			)
		}
		_ = m.Ack()
	},
		nats.Durable(durable),
		nats.ManualAck(),
		nats.BindStream(stream),
		nats.DeliverAll(),
	)
	if err != nil {
		nc.Close()
		return fmt.Errorf("subscribe: %w", err)
	}

	c.nc = nc
	c.sub = sub
	slog.Info("wallet consumer ready",
		"stream", stream,
		"durable", durable,
		"url", url,
	)
	return nil
}

// Handle is exported for unit tests. It parses the envelope, updates the
// relevant counter and logs structured metadata.
func (c *Consumer) Handle(subject string, data []byte) error {
	var ev OrderEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("unmarshal order event: %w", err)
	}

	switch subject {
	case SubjectOrderCreated:
		c.counters.Created.Add(1)
	case SubjectOrderUpdated:
		c.counters.Updated.Add(1)
	default:
		c.counters.Unknown.Add(1)
		slog.Warn("received unknown subject",
			"subject", subject,
			"type", ev.Type,
		)
		return nil
	}

	slog.Info("received order event",
		"subject", subject,
		"type", ev.Type,
		"version", ev.Version,
		"order_id", ev.Order.ID,
		"user_id", ev.Order.UserID,
		"pair", ev.Order.Pair,
		"side", ev.Order.Side,
		"status", ev.Order.Status,
	)
	return nil
}

// Stop gracefully tears down the subscription and closes the connection.
func (c *Consumer) Stop() {
	c.closeOnce.Do(func() {
		if c.sub != nil {
			_ = c.sub.Drain()
		}
		if c.nc != nil {
			c.nc.Close()
		}
	})
}

// ensureStream creates the stream with order.* subjects if it does not
// exist. It is tolerant of pre-provisioned streams.
func ensureStream(js nats.JetStreamContext, stream string) error {
	info, err := js.StreamInfo(stream)
	if err == nil {
		required := streamSubjects()
		if streamSubjectsCovered(info.Config.Subjects, required) {
			return nil
		}
		cfg := info.Config
		cfg.Subjects = unionStreamSubjects(info.Config.Subjects, required)
		if _, err := js.UpdateStream(&cfg); err != nil {
			return fmt.Errorf("update stream: %w", err)
		}
		return nil
	}
	if !errors.Is(err, nats.ErrStreamNotFound) {
		return fmt.Errorf("stream info: %w", err)
	}
	_, err = js.AddStream(&nats.StreamConfig{
		Name:      stream,
		Subjects:  streamSubjects(),
		Retention: nats.LimitsPolicy,
		Storage:   nats.FileStorage,
	})
	if err != nil {
		return fmt.Errorf("add stream: %w", err)
	}
	return nil
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
