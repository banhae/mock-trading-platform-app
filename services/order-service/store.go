package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// defaultListLimit / maxListLimit control pagination for GET /orders.
const (
	defaultListLimit = 50
	maxListLimit     = 200
)

// OrderStore defines the persistence interface for orders.
type OrderStore interface {
	Create(ctx context.Context, userID string, req CreateOrderRequest) (*Order, error)
	GetByID(ctx context.Context, id string) (*Order, error)
	List(ctx context.Context, params ListOrdersParams) ([]*Order, error)
	Cancel(ctx context.Context, userID, id string) (*Order, error)
	Ping(ctx context.Context) error
}

// PersistedMatchResult is the authoritative post-commit snapshot used for
// event publication in Phase 3 Slice F.
type PersistedMatchResult struct {
	// UpdatedOrders is deterministic:
	//   1) makers in first-seen fill order (de-duplicated)
	//   2) taker last
	UpdatedOrders []*Order
	// Trades preserves persisted trade row order.
	Trades []Trade
}

// PostgresStore implements OrderStore with PostgreSQL.
type PostgresStore struct {
	db *sql.DB
}

// NewPostgresStore opens a connection pool and verifies connectivity.
func NewPostgresStore(databaseURL string) (*PostgresStore, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.PingContext(context.Background()); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}

	return &PostgresStore{db: db}, nil
}

// EnsureTable creates the orders table if it does not exist, and brings a
// pre-Phase-1 dev database up to the current schema.
// DEV ONLY — 운영 환경에서는 마이그레이션 도구로 분리할 것.
//
// Phase 1 기준 스키마:
//   - status: open | partially_filled | filled | cancelled
//   - remaining_quantity: decimal string. 주문 생성 시 quantity 와 동일한 값
//   - updated_at: lifecycle 전이 시 갱신
//
// 기존 Phase 0 데이터에 대한 안전한 업그레이드 순서:
//  1. 새 컬럼을 NULL 허용으로 먼저 추가한다 (DEFAULT 를 걸지 않는다).
//     DEFAULT '0' 이나 DEFAULT now() 로 추가하면 기존 open 주문의
//     remaining_quantity 가 잘못된 값으로 물들어 `/orders` 화면에 즉시
//     노출된다.
//  2. NULL 인 행만 backfill 한다. 이미 값이 있는 행은 건드리지 않으므로
//     반복 실행해도 안전하다.
//  3. 마지막에 NOT NULL 제약을 건다. 이미 NOT NULL 이면 no-op.
func (s *PostgresStore) EnsureTable(ctx context.Context) error {
	slog.Warn("DEV ONLY: auto-creating/altering orders table — use a migration tool in production")
	stmts := []string{
		// 1) 신규 환경 — 처음부터 phase 1 스키마로 생성.
		`CREATE TABLE IF NOT EXISTS orders (
			id                 TEXT PRIMARY KEY,
			user_id            TEXT NOT NULL,
			pair               TEXT NOT NULL,
			side               TEXT NOT NULL,
			quantity           TEXT NOT NULL,
			remaining_quantity TEXT,
			price              TEXT NOT NULL,
			status             TEXT NOT NULL DEFAULT 'open',
			created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at         TIMESTAMPTZ
		)`,

		// 2) 기존 Phase 0 환경 — 컬럼을 NULL 로 추가. DEFAULT 없음.
		`ALTER TABLE orders ADD COLUMN IF NOT EXISTS remaining_quantity TEXT`,
		`ALTER TABLE orders ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ`,

		// 3) NULL 값만 backfill. 이미 값이 있는 행은 건드리지 않는다.
		//    Phase 0 에서는 모든 주문이 사실상 open 이었으므로 remaining = quantity.
		//    updated_at 은 lifecycle 전이가 없었으므로 created_at 과 동치.
		`UPDATE orders SET remaining_quantity = quantity WHERE remaining_quantity IS NULL`,
		`UPDATE orders SET updated_at = created_at WHERE updated_at IS NULL`,

		// 4) NOT NULL 승격. 이미 NOT NULL 이면 no-op.
		`ALTER TABLE orders ALTER COLUMN remaining_quantity SET NOT NULL`,
		`ALTER TABLE orders ALTER COLUMN updated_at SET NOT NULL`,

		`CREATE TABLE IF NOT EXISTS trades (
			trade_id       TEXT PRIMARY KEY,
			pair           TEXT NOT NULL,
			price          BIGINT NOT NULL,
			quantity       BIGINT NOT NULL,
			maker_order_id TEXT NOT NULL,
			taker_order_id TEXT NOT NULL,
			executed_at    TIMESTAMPTZ NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS trades_pair_executed_idx ON trades (pair, executed_at DESC)`,
		`CREATE INDEX IF NOT EXISTS trades_taker_idx ON trades (taker_order_id, executed_at DESC)`,
		`CREATE INDEX IF NOT EXISTS trades_maker_idx ON trades (maker_order_id, executed_at DESC)`,
		`CREATE INDEX IF NOT EXISTS orders_user_created_idx ON orders (user_id, created_at DESC)`,
	}
	for _, q := range stmts {
		if _, err := s.db.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("ensure table: %w", err)
		}
	}
	return nil
}

func (s *PostgresStore) Create(ctx context.Context, userID string, req CreateOrderRequest) (*Order, error) {
	now := time.Now().UTC()
	order := &Order{
		ID:                newUUID(),
		UserID:            userID,
		Pair:              req.Pair,
		Side:              req.Side,
		Quantity:          req.Quantity,
		RemainingQuantity: req.Quantity, // 아직 matcher 없음 — 전체가 남아 있다
		Price:             req.Price,
		Status:            StatusOpen,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO orders (
			id, user_id, pair, side, quantity, remaining_quantity, price, status, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		order.ID, order.UserID, order.Pair, order.Side,
		order.Quantity, order.RemainingQuantity, order.Price,
		order.Status, order.CreatedAt, order.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert order: %w", err)
	}

	return order, nil
}

func (s *PostgresStore) GetByID(ctx context.Context, id string) (*Order, error) {
	order := &Order{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, pair, side, quantity, remaining_quantity, price, status, created_at, updated_at
		 FROM orders WHERE id = $1`, id,
	).Scan(
		&order.ID, &order.UserID, &order.Pair, &order.Side,
		&order.Quantity, &order.RemainingQuantity, &order.Price,
		&order.Status, &order.CreatedAt, &order.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrOrderNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get order: %w", err)
	}
	return order, nil
}

// List returns the current user's orders, optionally filtered by status.
// user 는 params.UserID 로 지정되며 public API 에서는 주입 금지.
func (s *PostgresStore) List(ctx context.Context, params ListOrdersParams) ([]*Order, error) {
	limit := params.Limit
	if limit <= 0 {
		limit = defaultListLimit
	}
	if limit > maxListLimit {
		limit = maxListLimit
	}

	var (
		rows *sql.Rows
		err  error
	)
	base := `SELECT id, user_id, pair, side, quantity, remaining_quantity, price, status, created_at, updated_at
	         FROM orders WHERE user_id = $1`
	if params.Status != "" {
		rows, err = s.db.QueryContext(ctx,
			base+` AND status = $2 ORDER BY created_at DESC LIMIT $3`,
			params.UserID, params.Status, limit,
		)
	} else {
		rows, err = s.db.QueryContext(ctx,
			base+` ORDER BY created_at DESC LIMIT $2`,
			params.UserID, limit,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("list orders: %w", err)
	}
	defer rows.Close()

	out := make([]*Order, 0)
	for rows.Next() {
		o := &Order{}
		if err := rows.Scan(
			&o.ID, &o.UserID, &o.Pair, &o.Side,
			&o.Quantity, &o.RemainingQuantity, &o.Price,
			&o.Status, &o.CreatedAt, &o.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan order row: %w", err)
		}
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate orders: %w", err)
	}
	return out, nil
}

// Cancel transitions an order owned by userID into the cancelled state.
//
// 반환 규칙:
//   - 주문이 없으면          : ErrOrderNotFound
//   - 다른 사용자 소유이면   : ErrOrderForbidden
//   - 이미 종결 상태이면     : ErrOrderNotCancellable
//   - 그 외 정상 cancel 성공 : 업데이트된 Order 반환
//
// 상태 전이는 WHERE status IN (...) 가드로 atomic 하게 수행한다.
func (s *PostgresStore) Cancel(ctx context.Context, userID, id string) (*Order, error) {
	existing, err := s.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing.UserID != userID {
		// user scoping 위반. 존재 자체는 숨기지 않고 forbidden 으로 명확히 한다.
		return nil, ErrOrderForbidden
	}
	if !IsCancellable(existing.Status) {
		return nil, ErrOrderNotCancellable
	}

	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE orders
		 SET status = $1, updated_at = $2
		 WHERE id = $3 AND user_id = $4 AND status IN ($5, $6)`,
		StatusCancelled, now, id, userID, StatusOpen, StatusPartiallyFilled,
	)
	if err != nil {
		return nil, fmt.Errorf("cancel order: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("cancel order rows affected: %w", err)
	}
	if n == 0 {
		// 다른 트랜잭션이 먼저 상태를 바꿨을 가능성. 다시 읽어서 원인을 보고한다.
		return nil, ErrOrderNotCancellable
	}

	existing.Status = StatusCancelled
	existing.UpdatedAt = now
	return existing, nil
}

// PersistMatchResult persists a precomputed matcher result in a single DB transaction.
//
// Write set:
//   - update affected maker orders (remaining_quantity/status/updated_at)
//   - update taker order (remaining_quantity/status/updated_at)
//   - insert one trade row per fill
//
// Rollback policy:
//   - any failure causes full rollback (no partial order/trade persistence).
func (s *PostgresStore) PersistMatchResult(ctx context.Context, pair string, result MatchResult) (PersistedMatchResult, error) {
	if pair != "BTC-KRW" {
		return PersistedMatchResult{}, fmt.Errorf("unsupported pair: %s", pair)
	}
	if result.TakerOrderID == "" {
		return PersistedMatchResult{}, errors.New("missing taker order id")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return PersistedMatchResult{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().UTC()
	makerFilledByID := make(map[string]int64)
	makerIDsInFillOrder := make([]string, 0)
	seenMaker := make(map[string]struct{})
	for _, f := range result.Fills {
		if f.MakerOrderID == "" || f.TakerOrderID == "" || f.Price <= 0 || f.Quantity <= 0 {
			return PersistedMatchResult{}, fmt.Errorf("invalid fill: %+v", f)
		}
		makerFilledByID[f.MakerOrderID] += f.Quantity
		if _, ok := seenMaker[f.MakerOrderID]; !ok {
			seenMaker[f.MakerOrderID] = struct{}{}
			makerIDsInFillOrder = append(makerIDsInFillOrder, f.MakerOrderID)
		}
	}

	for _, makerOrderID := range makerIDsInFillOrder {
		delta := makerFilledByID[makerOrderID]
		if err := updateOrderRemainingAndStatus(ctx, tx, makerOrderID, delta, now); err != nil {
			return PersistedMatchResult{}, fmt.Errorf("update maker order %s: %w", makerOrderID, err)
		}
	}

	takerStatus := deriveTakerStatus(len(result.Fills), result.TakerRemainingQuantity)
	takerRes, err := tx.ExecContext(ctx,
		`UPDATE orders
		 SET remaining_quantity = $1, status = $2, updated_at = $3
		 WHERE id = $4`,
		FormatQuantityScaled(result.TakerRemainingQuantity), takerStatus, now, result.TakerOrderID,
	)
	if err != nil {
		return PersistedMatchResult{}, fmt.Errorf("update taker order: %w", err)
	}
	affected, err := takerRes.RowsAffected()
	if err != nil {
		return PersistedMatchResult{}, fmt.Errorf("taker rows affected: %w", err)
	}
	if affected != 1 {
		return PersistedMatchResult{}, ErrOrderNotFound
	}

	persistedTrades := make([]Trade, 0, len(result.Fills))
	for _, f := range result.Fills {
		tradeID := f.TradeID
		if tradeID == "" {
			tradeID = newUUID()
		}
		var persisted Trade
		if err := tx.QueryRowContext(ctx,
			`INSERT INTO trades (
				trade_id, pair, price, quantity, maker_order_id, taker_order_id, executed_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING trade_id, pair, price, quantity, maker_order_id, taker_order_id, executed_at`,
			tradeID, pair, f.Price, f.Quantity, f.MakerOrderID, f.TakerOrderID, now,
		).Scan(
			&persisted.TradeID,
			&persisted.Pair,
			&persisted.Price,
			&persisted.Quantity,
			&persisted.MakerOrderID,
			&persisted.TakerOrderID,
			&persisted.ExecutedAt,
		); err != nil {
			return PersistedMatchResult{}, fmt.Errorf("insert trade: %w", err)
		}
		persistedTrades = append(persistedTrades, persisted)
	}

	updatedOrders := make([]*Order, 0, len(makerIDsInFillOrder)+1)
	for _, makerOrderID := range makerIDsInFillOrder {
		o, err := getOrderTx(ctx, tx, makerOrderID)
		if err != nil {
			return PersistedMatchResult{}, fmt.Errorf("load maker order %s: %w", makerOrderID, err)
		}
		updatedOrders = append(updatedOrders, o)
	}
	takerOrder, err := getOrderTx(ctx, tx, result.TakerOrderID)
	if err != nil {
		return PersistedMatchResult{}, fmt.Errorf("load taker order %s: %w", result.TakerOrderID, err)
	}
	updatedOrders = append(updatedOrders, takerOrder)

	if err := tx.Commit(); err != nil {
		return PersistedMatchResult{}, fmt.Errorf("commit tx: %w", err)
	}
	return PersistedMatchResult{
		UpdatedOrders: updatedOrders,
		Trades:        persistedTrades,
	}, nil
}

func getOrderTx(ctx context.Context, tx *sql.Tx, id string) (*Order, error) {
	o := &Order{}
	err := tx.QueryRowContext(ctx,
		`SELECT id, user_id, pair, side, quantity, remaining_quantity, price, status, created_at, updated_at
		 FROM orders WHERE id = $1`,
		id,
	).Scan(
		&o.ID, &o.UserID, &o.Pair, &o.Side,
		&o.Quantity, &o.RemainingQuantity, &o.Price,
		&o.Status, &o.CreatedAt, &o.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrOrderNotFound
	}
	if err != nil {
		return nil, err
	}
	return o, nil
}

func updateOrderRemainingAndStatus(ctx context.Context, tx *sql.Tx, orderID string, delta int64, now time.Time) error {
	var remainingStr string
	if err := tx.QueryRowContext(ctx,
		`SELECT remaining_quantity FROM orders WHERE id = $1 FOR UPDATE`,
		orderID,
	).Scan(&remainingStr); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrOrderNotFound
		}
		return err
	}

	remaining, err := ParseQuantityScaled(remainingStr)
	if err != nil {
		return fmt.Errorf("parse remaining_quantity: %w", err)
	}
	if delta < 0 || delta > remaining {
		return fmt.Errorf("invalid maker fill delta: remaining=%d delta=%d", remaining, delta)
	}
	nextRemaining := remaining - delta
	status := StatusPartiallyFilled
	if nextRemaining == 0 {
		status = StatusFilled
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE orders
		 SET remaining_quantity = $1, status = $2, updated_at = $3
		 WHERE id = $4`,
		FormatQuantityScaled(nextRemaining), status, now, orderID,
	); err != nil {
		return err
	}
	return nil
}

func deriveTakerStatus(fillCount int, takerRemaining int64) string {
	if fillCount == 0 {
		return StatusOpen
	}
	if takerRemaining == 0 {
		return StatusFilled
	}
	return StatusPartiallyFilled
}

func (s *PostgresStore) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

// Close releases the database connection pool.
func (s *PostgresStore) Close() error {
	return s.db.Close()
}

// newUUID generates a random UUID v4.
func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
