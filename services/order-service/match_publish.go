package main

import (
	"context"
	"log/slog"
	"time"
)

// MatchResultPersister captures the narrow persistence seam needed for
// post-commit event publication.
type MatchResultPersister interface {
	PersistMatchResult(ctx context.Context, pair string, result MatchResult) (PersistedMatchResult, error)
}

// PersistAndPublishMatchResult persists first, then best-effort publishes.
//
// Event ordering is deterministic for one matching step:
//  1. order.updated events in PersistedMatchResult.UpdatedOrders order
//     (makers first-seen/de-duplicated, taker last)
//  2. trade.executed events in PersistedMatchResult.Trades order
func PersistAndPublishMatchResult(ctx context.Context, persister MatchResultPersister, publisher Publisher, pair string, result MatchResult) (PersistedMatchResult, error) {
	persisted, err := persister.PersistMatchResult(ctx, pair, result)
	if err != nil {
		return PersistedMatchResult{}, err
	}

	for _, order := range persisted.UpdatedOrders {
		ev := NewOrderEvent(SubjectOrderUpdated, order, time.Now())
		if err := publisher.PublishOrderEvent(ctx, ev); err != nil {
			slog.Error("failed to publish order event after persisted match",
				"subject", SubjectOrderUpdated,
				"order_id", order.ID,
				"error", err,
			)
		}
	}

	for _, trade := range persisted.Trades {
		ev := NewTradeExecutedEvent(trade, time.Now())
		if err := publisher.PublishTradeEvent(ctx, ev); err != nil {
			slog.Error("failed to publish trade event after persisted match",
				"subject", SubjectTradeExecuted,
				"trade_id", trade.TradeID,
				"error", err,
			)
		}
	}

	return persisted, nil
}
