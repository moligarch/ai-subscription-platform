package postgres

import (
	"context"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"

	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/ports/repository"
)

// Ensure compile-time conformance
var _ repository.TransactionManager = (*TxManager)(nil)

// TxManager implements repository.TransactionManager for Postgres (pgx).
// It begins a transaction, invokes the callback, and commits/rolls back.
// The tx handle is passed to the callback via the `qx any` argument (as pgx.Tx).
type TxManager struct {
	pool *pgxpool.Pool
}

func NewTxManager(pool *pgxpool.Pool) *TxManager {
	return &TxManager{pool: pool}
}

// WithTx opens a DB transaction and passes the tx handle to fn via qx.
// If fn returns an error, the transaction is rolled back; otherwise it is committed.
func (m *TxManager) WithTx(ctx context.Context, txOpt pgx.TxOptions, fn func(ctx context.Context, tx repository.Tx) error) error {
	// Default isolation level is ReadCommitted; adjust if you need stricter semantics.
	tx, err := m.pool.BeginTx(ctx, txOpt)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := fn(ctx, tx); err != nil {
		return err // rollback in defer
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}

func getExecutor(pool *pgxpool.Pool, tx repository.Tx) (interface {
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
	Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
}, error) {
	switch v := tx.(type) {
	case pgx.Tx:
		return v, nil
	case *pgxpool.Conn:
		return v, nil
	case *pgxpool.Pool:
		return v, nil
	case nil:
		// Explicitly use the pool if nil is passed
		if pool != nil {
			return pool, nil
		}
		return nil, domain.ErrInvalidArgument
	default:
		// This is now a clear error condition
		return nil, domain.ErrInvalidExecContext
	}
}
