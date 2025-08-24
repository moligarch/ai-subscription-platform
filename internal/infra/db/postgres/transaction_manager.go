// File: internal/infra/db/postgres/transaction_manager.go
package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

// TxManager provides a simple abstraction to run functions inside a SQL transaction.
// It is transport-agnostic: repositories accept a generic `qx any` which can be a
// pgx.Tx, *pgxpool.Conn, or nil. Usecases should call WithTx when multiple repo calls
// must succeed or fail atomically.
type TxManager interface {
	WithTx(ctx context.Context, fn func(ctx context.Context, tx pgx.Tx) error) error
}

var _ TxManager = (*PgxTxManager)(nil)

type PgxTxManager struct {
	pool *pgxpool.Pool
}

func NewTxManager(pool *pgxpool.Pool) *PgxTxManager {
	return &PgxTxManager{pool: pool}
}

func (m *PgxTxManager) WithTx(ctx context.Context, fn func(ctx context.Context, tx pgx.Tx) error) error {
	if fn == nil {
		return fmt.Errorf("nil tx function")
	}
	conn, err := m.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire conn: %w", err)
	}
	defer conn.Release()

	tx, err := conn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		// Ensure rollback if not committed
		_ = tx.Rollback(ctx)
	}()

	if err := fn(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}
