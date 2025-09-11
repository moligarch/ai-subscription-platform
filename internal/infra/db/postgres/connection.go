package postgres

import (
	"context"
	"fmt"
	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/repository"
	"time"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

// NewPgxPool creates a pgx connection pool with sensible defaults.
// Pass a PostgreSQL DSN like: postgres://user:pass@host:5432/dbname?sslmode=disable
func NewPgxPool(ctx context.Context, dsn string, maxConns int32) (*pgxpool.Pool, error) {
	if dsn == "" {
		return nil, fmt.Errorf("empty postgres dsn")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}
	if maxConns <= 0 {
		maxConns = 10
	}
	cfg.MaxConns = maxConns
	cfg.MinConns = 1
	cfg.HealthCheckPeriod = 30 * time.Second
	cfg.MaxConnLifetime = 60 * time.Minute
	cfg.MaxConnIdleTime = 10 * time.Minute
	cfg.ConnConfig.ConnectTimeout = 5 * time.Second

	pool, err := pgxpool.ConnectConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect pgxpool: %w", err)
	}
	// quick ping
	ctxPing, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(ctxPing); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return pool, nil
}

// TryConnect attempts to create a pgx pool with retry/backoff and a readiness ping.
// maxWait <= 0 defaults to 30s.
func TryConnect(ctx context.Context, dsn string, maxConns int32, maxWait time.Duration) (*pgxpool.Pool, error) {
	if maxWait <= 0 {
		maxWait = 30 * time.Second
	}

	deadline := time.Now().Add(maxWait)
	backoff := 200 * time.Millisecond
	var lastErr error

	for attempt := 1; ; attempt++ {
		// Short per-attempt timeout for dialing the pool
		dctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		pool, err := NewPgxPool(dctx, dsn, maxConns)
		cancel()

		if err == nil {
			// Readiness ping via a trivial query
			pctx, pcancel := context.WithTimeout(ctx, 3*time.Second)
			var one int
			qerr := pool.QueryRow(pctx, "select 1").Scan(&one)
			pcancel()

			if qerr == nil && one == 1 {
				return pool, nil
			}
			lastErr = qerr
			pool.Close()
		} else {
			lastErr = err
		}

		// No more time left?
		if time.Now().After(deadline) {
			break
		}

		// Sleep with capped exponential backoff
		time.Sleep(backoff)
		if backoff < 2*time.Second {
			backoff *= 2
			if backoff > 2*time.Second {
				backoff = 2 * time.Second
			}
		}
	}

	return nil, fmt.Errorf("connect pgxpool (retry for %s) failed: %w", maxWait, lastErr)
}

// ClosePgxPool is a convenience wrapper.
func ClosePgxPool(pool *pgxpool.Pool) {
	if pool != nil {
		pool.Close()
	}
}

// ---------------- helpers -----------------

func scanSub(row pgx.Row) (*model.UserSubscription, error) {
	s := &model.UserSubscription{}
	var status string
	if err := row.Scan(&s.ID, &s.UserID, &s.PlanID, &s.CreatedAt, &s.ScheduledStartAt, &s.StartAt, &s.ExpiresAt, &s.RemainingCredits, &status); err != nil {
		return nil, domain.ErrReadDatabaseRow
	}
	s.Status = model.SubscriptionStatus(status)
	return s, nil
}

func pickRow(ctx context.Context, pool *pgxpool.Pool, tx repository.Tx, sql string, args ...any) (pgx.Row, error) {
	exec, err := getExecutor(pool, tx)
	if err != nil {
		return nil, err
	}

	row := exec.QueryRow(ctx, sql, args...)
	return row, nil
}

func queryRows(ctx context.Context, pool *pgxpool.Pool, tx repository.Tx, sql string, args ...any) (pgx.Rows, error) {
	exec, err := getExecutor(pool, tx)
	if err != nil {
		return nil, err
	}
	return exec.Query(ctx, sql, args...)
}

func execSQL(ctx context.Context, pool *pgxpool.Pool, tx repository.Tx, sql string, args ...any) (pgconn.CommandTag, error) {
	exec, err := getExecutor(pool, tx)
	if err != nil {
		return nil, err
	}
	return exec.Exec(ctx, sql, args...)
}
