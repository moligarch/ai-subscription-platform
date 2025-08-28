package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"

	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/repository"
)

// Ensure PostgresSubscriptionRepo implements repository.SubscriptionRepository
var _ repository.SubscriptionRepository = (*PostgresSubscriptionRepo)(nil)

type PostgresSubscriptionRepo struct {
	pool *pgxpool.Pool
}

func NewPostgresSubscriptionRepo(pool *pgxpool.Pool) *PostgresSubscriptionRepo {
	return &PostgresSubscriptionRepo{pool: pool}
}

func (r *PostgresSubscriptionRepo) Save(ctx context.Context, qx any, s *model.UserSubscription) error {
	const q = `
INSERT INTO user_subscriptions (
  id, user_id, plan_id, created_at, scheduled_start_at, start_at, expires_at, remaining_credits, status
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
ON CONFLICT (id) DO UPDATE SET
  user_id=$2, plan_id=$3, scheduled_start_at=$5, start_at=$6, expires_at=$7, remaining_credits=$8, status=$9;
`
	var err error
	switch v := qx.(type) {
	case pgx.Tx:
		_, err = v.Exec(ctx, q, s.ID, s.UserID, s.PlanID, s.CreatedAt, s.ScheduledStartAt, s.StartAt, s.ExpiresAt, s.RemainingCredits, s.Status)
	case *pgxpool.Conn:
		_, err = v.Exec(ctx, q, s.ID, s.UserID, s.PlanID, s.CreatedAt, s.ScheduledStartAt, s.StartAt, s.ExpiresAt, s.RemainingCredits, s.Status)
	default:
		_, err = r.pool.Exec(ctx, q, s.ID, s.UserID, s.PlanID, s.CreatedAt, s.ScheduledStartAt, s.StartAt, s.ExpiresAt, s.RemainingCredits, s.Status)
	}
	return err
}

func (r *PostgresSubscriptionRepo) FindActiveByUserAndPlan(ctx context.Context, qx any, userID, planID string) (*model.UserSubscription, error) {
	const q = `
SELECT id, user_id, plan_id, created_at, scheduled_start_at, start_at, expires_at, remaining_credits, status
  FROM user_subscriptions
 WHERE user_id=$1 AND plan_id=$2 AND status='active'
 LIMIT 1;`
	return r.queryOne(ctx, qx, q, userID, planID)
}

func (r *PostgresSubscriptionRepo) FindActiveByUser(ctx context.Context, qx any, userID string) (*model.UserSubscription, error) {
	const q = `
SELECT id, user_id, plan_id, created_at, scheduled_start_at, start_at, expires_at, remaining_credits, status
  FROM user_subscriptions
 WHERE user_id=$1 AND status='active'
 ORDER BY created_at DESC
 LIMIT 1;`
	return r.queryOne(ctx, qx, q, userID)
}

func (r *PostgresSubscriptionRepo) FindReservedByUser(ctx context.Context, qx any, userID string) ([]*model.UserSubscription, error) {
	const q = `
SELECT id, user_id, plan_id, created_at, scheduled_start_at, start_at, expires_at, remaining_credits, status
  FROM user_subscriptions
 WHERE user_id=$1 AND status='reserved'
 ORDER BY created_at ASC;`
	rows, err := r.queryRows(ctx, qx, q, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.UserSubscription
	for rows.Next() {
		s, err := scanSub(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *PostgresSubscriptionRepo) FindByID(ctx context.Context, qx any, id string) (*model.UserSubscription, error) {
	const q = `
SELECT id, user_id, plan_id, created_at, scheduled_start_at, start_at, expires_at, remaining_credits, status
  FROM user_subscriptions
 WHERE id=$1;`
	return r.queryOne(ctx, qx, q, id)
}

func (r *PostgresSubscriptionRepo) FindExpiring(ctx context.Context, qx any, withinDays int) ([]*model.UserSubscription, error) {
	const q = `
SELECT id, user_id, plan_id, created_at, scheduled_start_at, start_at, expires_at, remaining_credits, status
  FROM user_subscriptions
 WHERE status='active' AND expires_at <= NOW() + ($1::int * INTERVAL '1 day')
 ORDER BY expires_at ASC;`
	rows, err := r.queryRows(ctx, qx, q, withinDays)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.UserSubscription
	for rows.Next() {
		s, err := scanSub(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *PostgresSubscriptionRepo) CountActiveByPlan(ctx context.Context, qx any) (map[string]int, error) {
	const q = `
SELECT plan_id, COUNT(*)
  FROM user_subscriptions
 WHERE status IN ('active','reserved')
 GROUP BY plan_id;`
	rows, err := r.queryRows(ctx, qx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := make(map[string]int)
	for rows.Next() {
		var planID string
		var c int
		if err := rows.Scan(&planID, &c); err != nil {
			return nil, err
		}
		m[planID] = c
	}
	return m, rows.Err()
}

func (r *PostgresSubscriptionRepo) TotalRemainingCredits(ctx context.Context, qx any) (int64, error) {
	const q = `SELECT COALESCE(SUM(remaining_credits),0) FROM user_subscriptions WHERE status IN ('active','reserved');`
	var n int64
	row := pickRow(r.pool, qx, q)
	if err := row.Scan(&n); err != nil {
		return 0, fmt.Errorf("sum credits: %w", err)
	}
	return n, nil
}

func (r *PostgresSubscriptionRepo) queryOne(_ context.Context, qx any, sql string, args ...any) (*model.UserSubscription, error) {
	row := pickRow(r.pool, qx, sql, args...)
	s := &model.UserSubscription{}
	var status string
	if err := row.Scan(&s.ID, &s.UserID, &s.PlanID, &s.CreatedAt, &s.ScheduledStartAt, &s.StartAt, &s.ExpiresAt, &s.RemainingCredits, &status); err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	s.Status = model.SubscriptionStatus(status)
	return s, nil
}

func (r *PostgresSubscriptionRepo) queryRows(ctx context.Context, qx any, sql string, args ...any) (pgx.Rows, error) {
	switch v := qx.(type) {
	case pgx.Tx:
		return v.Query(ctx, sql, args...)
	case *pgxpool.Conn:
		return v.Query(ctx, sql, args...)
	default:
		return r.pool.Query(ctx, sql, args...)
	}
}
