// Package postgres implement persistence ports của slo BC trên PostgreSQL
// (pgx/v5). Transaction dùng pattern tx-in-context (giống alerting BC): SLO
// INSERT + outbox INSERT nằm chung một TX (transactional outbox, ADR-016).
package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/namdam97/logmon/backend/internal/slo/ports"
)

// querier là phần giao của *pgxpool.Pool và pgx.Tx mà adapter dùng.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type txKey struct{}

// TxManager chạy fn trong một transaction; tx được gắn vào ctx cho adapter khác.
type TxManager struct {
	pool *pgxpool.Pool
}

var _ ports.TxManager = (*TxManager)(nil)

// NewTxManager tạo TxManager với pool.
func NewTxManager(pool *pgxpool.Pool) *TxManager { return &TxManager{pool: pool} }

// WithinTx mở tx, gắn vào ctx, chạy fn; commit nếu fn ok, rollback nếu lỗi.
func (m *TxManager) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op sau Commit

	if err := fn(context.WithValue(ctx, txKey{}, tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// dbFrom trả tx trong ctx nếu đang trong WithinTx, ngược lại trả pool.
func dbFrom(ctx context.Context, pool *pgxpool.Pool) querier {
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return tx
	}
	return pool
}
