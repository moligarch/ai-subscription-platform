package postgres

import (
	"context"
	"fmt"
	"telegram-ai-subscription/internal/domain/model"
	"time"

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
		return nil, err
	}
	s.Status = model.SubscriptionStatus(status)
	return s, nil
}

func pickRow(pool *pgxpool.Pool, qx any, sql string, args ...any) pgx.Row {
	switch v := qx.(type) {
	case pgx.Tx:
		return v.QueryRow(context.Background(), sql, args...)
	case *pgxpool.Conn:
		return v.QueryRow(context.Background(), sql, args...)
	default:
		return pool.QueryRow(context.Background(), sql, args...)
	}
}
