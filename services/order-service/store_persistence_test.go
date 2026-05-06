package main

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"
)

func openTestStore(t *testing.T) *PostgresStore {
	t.Helper()
	dbURL := os.Getenv("ORDER_SERVICE_TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("ORDER_SERVICE_TEST_DATABASE_URL not set; skipping DB integration tests")
	}
	store, err := NewPostgresStore(dbURL)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func resetTables(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx := context.Background()
	stmts := []string{
		`DROP TABLE IF EXISTS trades`,
		`DROP TABLE IF EXISTS orders`,
	}
	for _, q := range stmts {
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatalf("reset tables: %v", err)
		}
	}
}

func insertOrderRow(t *testing.T, db *sql.DB, id, userID, side, quantity, remaining, price, status string) {
	t.Helper()
	now := time.Now().UTC()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO orders (id, user_id, pair, side, quantity, remaining_quantity, price, status, created_at, updated_at)
		 VALUES ($1, $2, 'BTC-KRW', $3, $4, $5, $6, $7, $8, $9)`,
		id, userID, side, quantity, remaining, price, status, now, now,
	)
	if err != nil {
		t.Fatalf("insert order row: %v", err)
	}
}

func fetchOrderState(t *testing.T, db *sql.DB, id string) (remaining int64, status string) {
	t.Helper()
	var remainingStr string
	err := db.QueryRowContext(context.Background(),
		`SELECT remaining_quantity, status FROM orders WHERE id = $1`, id,
	).Scan(&remainingStr, &status)
	if err != nil {
		t.Fatalf("fetch order state: %v", err)
	}
	remaining, err = ParseQuantityScaled(remainingStr)
	if err != nil {
		t.Fatalf("parse remaining_quantity(%s): %v", remainingStr, err)
	}
	return
}

func tradeCount(t *testing.T, db *sql.DB) int {
	t.Helper()
	var c int
	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM trades`).Scan(&c); err != nil {
		t.Fatalf("count trades: %v", err)
	}
	return c
}

func TestEnsureTableIncludesTradesSchema(t *testing.T) {
	store := openTestStore(t)
	resetTables(t, store.db)

	if err := store.EnsureTable(context.Background()); err != nil {
		t.Fatalf("ensure table: %v", err)
	}

	var exists bool
	err := store.db.QueryRowContext(context.Background(),
		`SELECT EXISTS (
			SELECT 1
			FROM information_schema.tables
			WHERE table_schema = 'public' AND table_name = 'trades'
		)`,
	).Scan(&exists)
	if err != nil {
		t.Fatalf("check trades table existence: %v", err)
	}
	if !exists {
		t.Fatalf("expected trades table to exist")
	}
}

func TestPersistMatchResultUpdatesOrdersAndInsertsTrades(t *testing.T) {
	store := openTestStore(t)
	resetTables(t, store.db)
	if err := store.EnsureTable(context.Background()); err != nil {
		t.Fatalf("ensure table: %v", err)
	}

	insertOrderRow(t, store.db, "maker-1", "maker-user", "sell", FormatQuantityScaled(70), FormatQuantityScaled(70), "50000", StatusOpen)
	insertOrderRow(t, store.db, "maker-2", "maker-user", "sell", FormatQuantityScaled(50), FormatQuantityScaled(50), "50010", StatusOpen)
	insertOrderRow(t, store.db, "taker-1", "taker-user", "buy", FormatQuantityScaled(120), FormatQuantityScaled(120), "50100", StatusOpen)

	result := MatchResult{
		TakerOrderID:           "taker-1",
		TakerRemainingQuantity: 30,
		Fills: []Fill{
			{TradeID: "trade-1", MakerOrderID: "maker-1", TakerOrderID: "taker-1", Price: 50000, Quantity: 70},
			{TradeID: "trade-2", MakerOrderID: "maker-2", TakerOrderID: "taker-1", Price: 50010, Quantity: 20},
		},
	}
	persisted, err := store.PersistMatchResult(context.Background(), "BTC-KRW", result)
	if err != nil {
		t.Fatalf("persist match result: %v", err)
	}

	maker1Remain, maker1Status := fetchOrderState(t, store.db, "maker-1")
	if maker1Remain != 0 || maker1Status != StatusFilled {
		t.Fatalf("maker-1 state mismatch: remaining=%d status=%s", maker1Remain, maker1Status)
	}
	maker2Remain, maker2Status := fetchOrderState(t, store.db, "maker-2")
	if maker2Remain != 30 || maker2Status != StatusPartiallyFilled {
		t.Fatalf("maker-2 state mismatch: remaining=%d status=%s", maker2Remain, maker2Status)
	}
	takerRemain, takerStatus := fetchOrderState(t, store.db, "taker-1")
	if takerRemain != 30 || takerStatus != StatusPartiallyFilled {
		t.Fatalf("taker state mismatch: remaining=%d status=%s", takerRemain, takerStatus)
	}
	if got := tradeCount(t, store.db); got != 2 {
		t.Fatalf("trade row count mismatch: got=%d want=2", got)
	}
	if len(persisted.UpdatedOrders) != 3 {
		t.Fatalf("updated orders len=%d want=3", len(persisted.UpdatedOrders))
	}
	if persisted.UpdatedOrders[0].ID != "maker-1" || persisted.UpdatedOrders[1].ID != "maker-2" || persisted.UpdatedOrders[2].ID != "taker-1" {
		t.Fatalf("unexpected updated order order: %s, %s, %s",
			persisted.UpdatedOrders[0].ID, persisted.UpdatedOrders[1].ID, persisted.UpdatedOrders[2].ID)
	}
	if len(persisted.Trades) != 2 || persisted.Trades[0].TradeID != "trade-1" || persisted.Trades[1].TradeID != "trade-2" {
		t.Fatalf("unexpected persisted trades: %#v", persisted.Trades)
	}
}

func TestPersistMatchResultWithFractionalRemainingQuantities(t *testing.T) {
	store := openTestStore(t)
	resetTables(t, store.db)
	if err := store.EnsureTable(context.Background()); err != nil {
		t.Fatalf("ensure table: %v", err)
	}

	insertOrderRow(t, store.db, "maker-frac", "maker-user", "sell", "0.5", "0.5", "50000", StatusOpen)
	insertOrderRow(t, store.db, "taker-frac", "taker-user", "buy", "0.7", "0.7", "50000", StatusOpen)

	makerFill := mustParseQty(t, "0.2")
	result := MatchResult{
		TakerOrderID:           "taker-frac",
		TakerRemainingQuantity: mustParseQty(t, "0.5"),
		Fills: []Fill{
			{TradeID: "trade-frac-1", MakerOrderID: "maker-frac", TakerOrderID: "taker-frac", Price: 50000, Quantity: makerFill},
		},
	}
	if _, err := store.PersistMatchResult(context.Background(), "BTC-KRW", result); err != nil {
		t.Fatalf("persist fractional result: %v", err)
	}

	makerRemain, makerStatus := fetchOrderState(t, store.db, "maker-frac")
	if makerRemain != mustParseQty(t, "0.3") || makerStatus != StatusPartiallyFilled {
		t.Fatalf("maker fractional state mismatch: remaining=%d status=%s", makerRemain, makerStatus)
	}
	takerRemain, takerStatus := fetchOrderState(t, store.db, "taker-frac")
	if takerRemain != mustParseQty(t, "0.5") || takerStatus != StatusPartiallyFilled {
		t.Fatalf("taker fractional state mismatch: remaining=%d status=%s", takerRemain, takerStatus)
	}
	if got := tradeCount(t, store.db); got != 1 {
		t.Fatalf("trade row count mismatch on fractional test: got=%d want=1", got)
	}
}

func TestPersistMatchResultNoFillKeepsTakerOpen(t *testing.T) {
	store := openTestStore(t)
	resetTables(t, store.db)
	if err := store.EnsureTable(context.Background()); err != nil {
		t.Fatalf("ensure table: %v", err)
	}

	insertOrderRow(t, store.db, "taker-1", "taker-user", "buy", FormatQuantityScaled(100), FormatQuantityScaled(100), "50000", StatusOpen)
	result := MatchResult{TakerOrderID: "taker-1", TakerRemainingQuantity: 100, Fills: nil}
	if _, err := store.PersistMatchResult(context.Background(), "BTC-KRW", result); err != nil {
		t.Fatalf("persist no-fill result: %v", err)
	}

	remain, status := fetchOrderState(t, store.db, "taker-1")
	if remain != 100 || status != StatusOpen {
		t.Fatalf("no-fill taker state mismatch: remaining=%d status=%s", remain, status)
	}
	if got := tradeCount(t, store.db); got != 0 {
		t.Fatalf("expected no trades on no-fill path, got=%d", got)
	}
}

func TestPersistMatchResultRollsBackOnTradeInsertFailure(t *testing.T) {
	store := openTestStore(t)
	resetTables(t, store.db)
	if err := store.EnsureTable(context.Background()); err != nil {
		t.Fatalf("ensure table: %v", err)
	}

	insertOrderRow(t, store.db, "maker-1", "maker-user", "sell", FormatQuantityScaled(100), FormatQuantityScaled(100), "50000", StatusOpen)
	insertOrderRow(t, store.db, "taker-1", "taker-user", "buy", FormatQuantityScaled(100), FormatQuantityScaled(100), "50000", StatusOpen)

	result := MatchResult{
		TakerOrderID:           "taker-1",
		TakerRemainingQuantity: 0,
		Fills: []Fill{
			{TradeID: "trade-dup", MakerOrderID: "maker-1", TakerOrderID: "taker-1", Price: 50000, Quantity: 60},
			{TradeID: "trade-dup", MakerOrderID: "maker-1", TakerOrderID: "taker-1", Price: 50000, Quantity: 40},
		},
	}

	if _, err := store.PersistMatchResult(context.Background(), "BTC-KRW", result); err == nil {
		t.Fatalf("expected persistence failure due to duplicate trade_id")
	}

	makerRemain, makerStatus := fetchOrderState(t, store.db, "maker-1")
	if makerRemain != 100 || makerStatus != StatusOpen {
		t.Fatalf("maker must rollback to original state: remaining=%d status=%s", makerRemain, makerStatus)
	}
	takerRemain, takerStatus := fetchOrderState(t, store.db, "taker-1")
	if takerRemain != 100 || takerStatus != StatusOpen {
		t.Fatalf("taker must rollback to original state: remaining=%d status=%s", takerRemain, takerStatus)
	}
	if got := tradeCount(t, store.db); got != 0 {
		t.Fatalf("trades must rollback on failure, got=%d", got)
	}
}

func TestPersistMatchResultMissingTakerRollsBackAllWrites(t *testing.T) {
	store := openTestStore(t)
	resetTables(t, store.db)
	if err := store.EnsureTable(context.Background()); err != nil {
		t.Fatalf("ensure table: %v", err)
	}

	insertOrderRow(t, store.db, "maker-1", "maker-user", "sell", "0.5", "0.5", "50000", StatusOpen)

	result := MatchResult{
		TakerOrderID:           "missing-taker",
		TakerRemainingQuantity: 0,
		Fills: []Fill{
			{TradeID: "trade-missing-taker", MakerOrderID: "maker-1", TakerOrderID: "missing-taker", Price: 50000, Quantity: mustParseQty(t, "0.1")},
		},
	}

	_, err := store.PersistMatchResult(context.Background(), "BTC-KRW", result)
	if err == nil {
		t.Fatalf("expected error when taker row is missing")
	}
	if !errors.Is(err, ErrOrderNotFound) {
		t.Fatalf("expected ErrOrderNotFound, got %v", err)
	}

	makerRemain, makerStatus := fetchOrderState(t, store.db, "maker-1")
	if makerRemain != mustParseQty(t, "0.5") || makerStatus != StatusOpen {
		t.Fatalf("maker must rollback when taker missing: remaining=%d status=%s", makerRemain, makerStatus)
	}
	if got := tradeCount(t, store.db); got != 0 {
		t.Fatalf("trades must not be inserted when taker missing, got=%d", got)
	}
}

type failAllPublisher struct{}

func (failAllPublisher) PublishOrderEvent(context.Context, OrderEvent) error {
	return errors.New("order publish failed")
}

func (failAllPublisher) PublishTradeEvent(context.Context, TradeEvent) error {
	return errors.New("trade publish failed")
}

func TestPersistAndPublishMatchResultPublishFailureKeepsDBState(t *testing.T) {
	store := openTestStore(t)
	resetTables(t, store.db)
	if err := store.EnsureTable(context.Background()); err != nil {
		t.Fatalf("ensure table: %v", err)
	}

	insertOrderRow(t, store.db, "maker-1", "maker-user", "sell", "0.5", "0.5", "50000", StatusOpen)
	insertOrderRow(t, store.db, "taker-1", "taker-user", "buy", "0.5", "0.5", "50000", StatusOpen)

	result := MatchResult{
		TakerOrderID:           "taker-1",
		TakerRemainingQuantity: 0,
		Fills: []Fill{
			{TradeID: "trade-1", MakerOrderID: "maker-1", TakerOrderID: "taker-1", Price: 50000, Quantity: mustParseQty(t, "0.5")},
		},
	}

	if _, err := PersistAndPublishMatchResult(context.Background(), store, failAllPublisher{}, "BTC-KRW", result); err != nil {
		t.Fatalf("publish failure must not fail persisted operation: %v", err)
	}

	makerRemain, makerStatus := fetchOrderState(t, store.db, "maker-1")
	if makerRemain != 0 || makerStatus != StatusFilled {
		t.Fatalf("maker persistence must remain committed: remaining=%d status=%s", makerRemain, makerStatus)
	}
	takerRemain, takerStatus := fetchOrderState(t, store.db, "taker-1")
	if takerRemain != 0 || takerStatus != StatusFilled {
		t.Fatalf("taker persistence must remain committed: remaining=%d status=%s", takerRemain, takerStatus)
	}
	if got := tradeCount(t, store.db); got != 1 {
		t.Fatalf("trade row must remain committed despite publish failure, got=%d", got)
	}
}

func TestDeriveTakerStatus(t *testing.T) {
	cases := []struct {
		name      string
		fills     int
		remaining int64
		want      string
	}{
		{name: "no fill keeps open", fills: 0, remaining: 100, want: StatusOpen},
		{name: "partial fill", fills: 1, remaining: 10, want: StatusPartiallyFilled},
		{name: "full fill", fills: 2, remaining: 0, want: StatusFilled},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := deriveTakerStatus(tc.fills, tc.remaining); got != tc.want {
				t.Fatalf("deriveTakerStatus()=%s, want %s", got, tc.want)
			}
		})
	}
}

func mustParseQty(t *testing.T, s string) int64 {
	t.Helper()
	v, err := ParseQuantityScaled(s)
	if err != nil {
		t.Fatalf("ParseQuantityScaled(%s): %v", s, err)
	}
	return v
}
