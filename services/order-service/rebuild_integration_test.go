package main

import (
	"context"
	"reflect"
	"testing"
	"time"
)

func insertOrderRowWithTime(t *testing.T, store *PostgresStore, id, userID, pair, side, quantity, remaining, price, status string, createdAt time.Time) {
	t.Helper()
	_, err := store.db.ExecContext(context.Background(),
		`INSERT INTO orders (id, user_id, pair, side, quantity, remaining_quantity, price, status, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9)`,
		id, userID, pair, side, quantity, remaining, price, status, createdAt.UTC(),
	)
	if err != nil {
		t.Fatalf("insert order row with time: %v", err)
	}
}

func mustLoadAndRebuild(t *testing.T, store *PostgresStore) (*OrderBook, []RestorableOrder) {
	t.Helper()
	restorable, err := store.LoadRestorableOrders(context.Background())
	if err != nil {
		t.Fatalf("load restorable orders: %v", err)
	}
	book, err := RebuildOrderBookFromPersistedOrders(restorable)
	if err != nil {
		t.Fatalf("rebuild order book: %v", err)
	}
	return book, restorable
}

func TestRebuildLoadsOnlyRestingBTCKRWOrdersWithDeterministicOrdering(t *testing.T) {
	store := openTestStore(t)
	resetTables(t, store.db)
	if err := store.EnsureTable(context.Background()); err != nil {
		t.Fatalf("ensure table: %v", err)
	}

	base := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	insertOrderRowWithTime(t, store, "bid-open-1", "u1", "BTC-KRW", "buy", "1", "1", "1000", StatusOpen, base.Add(1*time.Second))
	insertOrderRowWithTime(t, store, "bid-partial-1", "u1", "BTC-KRW", "buy", "5", "2", "1000", StatusPartiallyFilled, base.Add(2*time.Second))
	insertOrderRowWithTime(t, store, "ask-open-b", "u2", "BTC-KRW", "sell", "4", "4", "1100", StatusOpen, base.Add(3*time.Second))
	insertOrderRowWithTime(t, store, "ask-open-a", "u2", "BTC-KRW", "sell", "3", "3", "1100", StatusOpen, base.Add(3*time.Second))
	insertOrderRowWithTime(t, store, "ask-filled-1", "u2", "BTC-KRW", "sell", "2", "0", "1050", StatusFilled, base.Add(4*time.Second))
	insertOrderRowWithTime(t, store, "bid-cancel-1", "u3", "BTC-KRW", "buy", "2", "2", "900", StatusCancelled, base.Add(5*time.Second))
	insertOrderRowWithTime(t, store, "eth-open-1", "u4", "ETH-KRW", "buy", "1", "1", "2000000", StatusOpen, base.Add(6*time.Second))

	book, restorable := mustLoadAndRebuild(t, store)

	if len(restorable) != 4 {
		t.Fatalf("restorable len=%d want=4", len(restorable))
	}
	gotOrderIDs := []string{restorable[0].ID, restorable[1].ID, restorable[2].ID, restorable[3].ID}
	wantOrderIDs := []string{"bid-open-1", "bid-partial-1", "ask-open-a", "ask-open-b"}
	if !reflect.DeepEqual(gotOrderIDs, wantOrderIDs) {
		t.Fatalf("deterministic load order mismatch: got=%v want=%v", gotOrderIDs, wantOrderIDs)
	}

	bids := book.SideSnapshot("buy")
	if len(bids) != 2 {
		t.Fatalf("buy snapshot len=%d want=2", len(bids))
	}
	if bids[0].OrderID != "bid-open-1" || bids[1].OrderID != "bid-partial-1" {
		t.Fatalf("buy FIFO mismatch after rebuild: %+v", bids)
	}
	if bids[1].RemainingQuantity != mustParseQty(t, "2") {
		t.Fatalf("partially_filled must rebuild from remaining_quantity=2, got=%d", bids[1].RemainingQuantity)
	}

	asks := book.SideSnapshot("sell")
	if len(asks) != 2 {
		t.Fatalf("sell snapshot len=%d want=2", len(asks))
	}
	if asks[0].OrderID != "ask-open-a" || asks[1].OrderID != "ask-open-b" {
		t.Fatalf("sell FIFO tie-break mismatch after rebuild: %+v", asks)
	}

	bestBid, ok := book.BestBid()
	if !ok || bestBid.OrderID != "bid-open-1" || bestBid.Price != 1000 {
		t.Fatalf("best bid mismatch: ok=%v bid=%+v", ok, bestBid)
	}
	bestAsk, ok := book.BestAsk()
	if !ok || bestAsk.OrderID != "ask-open-a" || bestAsk.Price != 1100 {
		t.Fatalf("best ask mismatch: ok=%v ask=%+v", ok, bestAsk)
	}
}

func TestRebuildThenProcessNewOrderUsesRebuiltPriceTimePriority(t *testing.T) {
	store := openTestStore(t)
	resetTables(t, store.db)
	if err := store.EnsureTable(context.Background()); err != nil {
		t.Fatalf("ensure table: %v", err)
	}

	base := time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)
	insertOrderRowWithTime(t, store, "ask-open-a", "u1", "BTC-KRW", "sell", "3", "3", "1100", StatusOpen, base)
	insertOrderRowWithTime(t, store, "ask-open-b", "u2", "BTC-KRW", "sell", "4", "4", "1100", StatusOpen, base)

	book, _ := mustLoadAndRebuild(t, store)
	res, err := ProcessNewOrder(book, IncomingOrder{
		OrderID:           "buy-taker-1",
		Side:              "buy",
		Price:             1100,
		RemainingQuantity: mustParseQty(t, "5"),
	})
	if err != nil {
		t.Fatalf("process new order on rebuilt book: %v", err)
	}

	if len(res.Fills) != 2 {
		t.Fatalf("fills len=%d want=2", len(res.Fills))
	}
	if res.Fills[0].MakerOrderID != "ask-open-a" || res.Fills[1].MakerOrderID != "ask-open-b" {
		t.Fatalf("rebuild FIFO priority broken: fills=%+v", res.Fills)
	}
	if res.TakerRemainingQuantity != 0 || res.TakerRested {
		t.Fatalf("taker should be fully matched, got %+v", res)
	}

	asks := book.SideSnapshot("sell")
	if len(asks) != 1 || asks[0].OrderID != "ask-open-b" || asks[0].RemainingQuantity != mustParseQty(t, "2") {
		t.Fatalf("unexpected asks after match on rebuilt book: %+v", asks)
	}
}

func TestRebuildThenProcessCancelOrderRemovesRestingOrder(t *testing.T) {
	store := openTestStore(t)
	resetTables(t, store.db)
	if err := store.EnsureTable(context.Background()); err != nil {
		t.Fatalf("ensure table: %v", err)
	}

	base := time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC)
	insertOrderRowWithTime(t, store, "bid-open-1", "u1", "BTC-KRW", "buy", "1", "1", "1000", StatusOpen, base)
	insertOrderRowWithTime(t, store, "ask-open-1", "u2", "BTC-KRW", "sell", "1", "1", "1200", StatusOpen, base.Add(time.Second))

	book, _ := mustLoadAndRebuild(t, store)
	cancelRes, err := ProcessCancelOrder(book, "bid-open-1")
	if err != nil {
		t.Fatalf("cancel on rebuilt book failed: %v", err)
	}
	if cancelRes.Status != CancelStatusCancelled {
		t.Fatalf("unexpected cancel status: %+v", cancelRes)
	}
	if got := snapshotIDs(book.SideSnapshot("buy")); len(got) != 0 {
		t.Fatalf("expected empty buy side after cancel, got=%v", got)
	}
	if got := snapshotIDs(book.SideSnapshot("sell")); !reflect.DeepEqual(got, []string{"ask-open-1"}) {
		t.Fatalf("sell side must remain untouched, got=%v", got)
	}
}

func TestRebuildTwiceFromSameDBYieldsSameSnapshotOrdering(t *testing.T) {
	store := openTestStore(t)
	resetTables(t, store.db)
	if err := store.EnsureTable(context.Background()); err != nil {
		t.Fatalf("ensure table: %v", err)
	}

	base := time.Date(2026, 4, 4, 0, 0, 0, 0, time.UTC)
	insertOrderRowWithTime(t, store, "bid-1", "u1", "BTC-KRW", "buy", "2", "2", "1000", StatusOpen, base)
	insertOrderRowWithTime(t, store, "bid-2", "u1", "BTC-KRW", "buy", "3", "3", "1000", StatusOpen, base)
	insertOrderRowWithTime(t, store, "ask-1", "u2", "BTC-KRW", "sell", "1", "1", "1100", StatusOpen, base)
	insertOrderRowWithTime(t, store, "ask-2", "u2", "BTC-KRW", "sell", "1", "1", "1100", StatusOpen, base)

	book1, _ := mustLoadAndRebuild(t, store)
	book2, _ := mustLoadAndRebuild(t, store)

	bids1 := book1.SideSnapshot("buy")
	bids2 := book2.SideSnapshot("buy")
	asks1 := book1.SideSnapshot("sell")
	asks2 := book2.SideSnapshot("sell")

	if !reflect.DeepEqual(bids1, bids2) {
		t.Fatalf("buy snapshot mismatch across rebuilds: first=%+v second=%+v", bids1, bids2)
	}
	if !reflect.DeepEqual(asks1, asks2) {
		t.Fatalf("sell snapshot mismatch across rebuilds: first=%+v second=%+v", asks1, asks2)
	}
}
