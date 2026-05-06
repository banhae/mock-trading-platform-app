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

const (
	orderEventsSubject = "order.*"
	tradeEventsSubject = "trade.*"
)

type Counters struct {
	Created atomic.Uint64
	Updated atomic.Uint64
	Traded  atomic.Uint64
	Unknown atomic.Uint64
	Failed  atomic.Uint64
}

type CountersSnapshot struct {
	Created uint64
	Updated uint64
	Traded  uint64
	Unknown uint64
	Failed  uint64
}

func (c *Counters) Snapshot() CountersSnapshot {
	return CountersSnapshot{
		Created: c.Created.Load(),
		Updated: c.Updated.Load(),
		Traded:  c.Traded.Load(),
		Unknown: c.Unknown.Load(),
		Failed:  c.Failed.Load(),
	}
}

type Consumer struct {
	counters Counters
	model    *ReadModel

	nc       *nats.Conn
	orderSub *nats.Subscription
	tradeSub *nats.Subscription

	closeOnce sync.Once
}

func NewConsumer(model *ReadModel) *Consumer {
	if model == nil {
		model = NewReadModel()
	}
	return &Consumer{model: model}
}

func (c *Consumer) CountersSnapshot() CountersSnapshot {
	return c.counters.Snapshot()
}

func (c *Consumer) Start(_ context.Context, url, stream, durable string) error {
	if url == "" {
		return errors.New("nats url is empty")
	}
	if stream == "" {
		return errors.New("nats stream is empty")
	}
	if durable == "" {
		durable = "marketdata-consumer"
	}

	nc, err := nats.Connect(url,
		nats.Name("marketdata-service consumer"),
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

	orderSub, err := js.Subscribe(orderEventsSubject, func(m *nats.Msg) {
		if err := c.Handle(m.Subject, m.Data); err != nil {
			c.counters.Failed.Add(1)
			slog.Error("failed to handle event", "subject", m.Subject, "error", err)
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
		return fmt.Errorf("subscribe order.*: %w", err)
	}

	tradeDurable := tradeDurableName(durable)
	tradeSub, err := js.Subscribe(tradeEventsSubject, func(m *nats.Msg) {
		if err := c.Handle(m.Subject, m.Data); err != nil {
			c.counters.Failed.Add(1)
			slog.Error("failed to handle event", "subject", m.Subject, "error", err)
		}
		_ = m.Ack()
	},
		nats.Durable(tradeDurable),
		nats.ManualAck(),
		nats.BindStream(stream),
		nats.DeliverAll(),
	)
	if err != nil {
		_ = orderSub.Drain()
		nc.Close()
		return fmt.Errorf("subscribe trade.*: %w", err)
	}

	c.nc = nc
	c.orderSub = orderSub
	c.tradeSub = tradeSub
	slog.Info("marketdata consumer ready", "stream", stream, "order_durable", durable, "trade_durable", tradeDurable, "url", url)
	return nil
}

func (c *Consumer) Handle(subject string, data []byte) error {
	switch subject {
	case SubjectOrderCreated, SubjectOrderUpdated:
		var ev OrderEvent
		if err := json.Unmarshal(data, &ev); err != nil {
			return fmt.Errorf("unmarshal order event: %w", err)
		}
		if err := c.model.ApplyOrderEvent(ev); err != nil {
			return fmt.Errorf("apply order event: %w", err)
		}
		if subject == SubjectOrderCreated {
			c.counters.Created.Add(1)
		} else {
			c.counters.Updated.Add(1)
		}
		slog.Info("received order event", "subject", subject, "type", ev.Type, "order_id", ev.Order.ID, "pair", ev.Order.Pair, "status", ev.Order.Status)
		return nil
	case SubjectTradeExecuted:
		var ev TradeEvent
		if err := json.Unmarshal(data, &ev); err != nil {
			return fmt.Errorf("unmarshal trade event: %w", err)
		}
		if err := c.model.ApplyTradeEvent(ev); err != nil {
			return fmt.Errorf("apply trade event: %w", err)
		}
		c.counters.Traded.Add(1)
		slog.Info("received trade event", "subject", subject, "type", ev.Type, "trade_id", ev.Trade.TradeID, "pair", ev.Trade.Pair)
		return nil
	default:
		c.counters.Unknown.Add(1)
		slog.Warn("received unknown subject", "subject", subject)
		return nil
	}
}

func (c *Consumer) Stop() {
	c.closeOnce.Do(func() {
		if c.orderSub != nil {
			_ = c.orderSub.Drain()
		}
		if c.tradeSub != nil {
			_ = c.tradeSub.Drain()
		}
		if c.nc != nil {
			c.nc.Close()
		}
	})
}

func tradeDurableName(base string) string {
	return base + "-trade"
}

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
	_, err = js.AddStream(&nats.StreamConfig{Name: stream, Subjects: streamSubjects(), Retention: nats.LimitsPolicy, Storage: nats.FileStorage})
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
