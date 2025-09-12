package postgres

import (
	"context"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"

	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/repository"
)

// Ensure subscriptionRepo implements repository.SubscriptionRepository
var _ repository.SubscriptionRepository = (*subscriptionRepo)(nil)

type subscriptionRepo struct {
	pool *pgxpool.Pool
}

func NewSubscriptionRepo(pool *pgxpool.Pool) *subscriptionRepo {
	return &subscriptionRepo{pool: pool}
}

func (r *subscriptionRepo) Save(ctx context.Context, tx repository.Tx, s *model.UserSubscription) error {
	const q = `
INSERT INTO user_subscriptions (
  id, user_id, plan_id, created_at, scheduled_start_at, start_at, expires_at, remaining_credits, status
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
ON CONFLICT (id) DO UPDATE SET
  user_id=$2, plan_id=$3, scheduled_start_at=$5, start_at=$6, expires_at=$7, remaining_credits=$8, status=$9;`

	_, err := execSQL(ctx, r.pool, tx, q, s.ID, s.UserID, s.PlanID, s.CreatedAt, s.ScheduledStartAt, s.StartAt, s.ExpiresAt, s.RemainingCredits, s.Status)
	if err != nil {
		switch err {
		case domain.ErrInvalidArgument, domain.ErrInvalidExecContext:
			return err
		default:
			if err.(*pgconn.PgError).Code == "23505" {
				return domain.ErrAlreadyHasReserved
			}
			return domain.ErrOperationFailed
		}
	}
	return nil
}

func (r *subscriptionRepo) FindActiveByUserAndPlan(ctx context.Context, tx repository.Tx, userID, planID string) (*model.UserSubscription, error) {
	const q = `
SELECT id, user_id, plan_id, created_at, scheduled_start_at, start_at, expires_at, remaining_credits, status
  FROM user_subscriptions
 WHERE user_id=$1 AND plan_id=$2 AND status='active'
 LIMIT 1;`
	return r.queryOne(ctx, repository.NoTX, q, userID, planID)
}

func (r *subscriptionRepo) FindActiveByUser(ctx context.Context, tx repository.Tx, userID string) (*model.UserSubscription, error) {
	const q = `
SELECT id, user_id, plan_id, created_at, scheduled_start_at, start_at, expires_at, remaining_credits, status
  FROM user_subscriptions
 WHERE user_id=$1 AND status='active'
 ORDER BY created_at DESC
 LIMIT 1;`
	return r.queryOne(ctx, repository.NoTX, q, userID)
}

func (r *subscriptionRepo) FindReservedByUser(ctx context.Context, tx repository.Tx, userID string) ([]*model.UserSubscription, error) {
	const q = `
SELECT id, user_id, plan_id, created_at, scheduled_start_at, start_at, expires_at, remaining_credits, status
  FROM user_subscriptions
 WHERE user_id=$1 AND status='reserved'
 ORDER BY created_at ASC;`
	rows, err := queryRows(ctx, r.pool, nil, q, userID)
	if err != nil {
		switch err {
		case pgx.ErrNoRows:
			return nil, domain.ErrNotFound
		case domain.ErrInvalidArgument, domain.ErrInvalidExecContext:
			return nil, err
		default:
			return nil, domain.ErrOperationFailed
		}
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
	if err := rows.Err(); err != nil {
		return nil, domain.ErrReadDatabaseRow
	}
	return out, nil
}

func (r *subscriptionRepo) FindByID(ctx context.Context, tx repository.Tx, id string) (*model.UserSubscription, error) {
	const q = `
SELECT id, user_id, plan_id, created_at, scheduled_start_at, start_at, expires_at, remaining_credits, status
  FROM user_subscriptions
 WHERE id=$1;`
	return r.queryOne(ctx, tx, q, id)
}

func (r *subscriptionRepo) FindExpiring(ctx context.Context, tx repository.Tx, withinDays int) ([]*model.UserSubscription, error) {
	const q = `
SELECT id, user_id, plan_id, created_at, scheduled_start_at, start_at, expires_at, remaining_credits, status
  FROM user_subscriptions
 WHERE status='active' 
   AND expires_at > NOW() 
   AND expires_at <= NOW() + ($1::int * INTERVAL '1 day')
 ORDER BY expires_at ASC;`
	rows, err := queryRows(ctx, r.pool, nil, q, withinDays)
	if err != nil {
		switch err {
		case pgx.ErrNoRows:
			return nil, domain.ErrNotFound
		case domain.ErrInvalidArgument, domain.ErrInvalidExecContext:
			return nil, err
		default:
			return nil, domain.ErrOperationFailed
		}
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
	if err := rows.Err(); err != nil {
		return nil, domain.ErrReadDatabaseRow
	}
	return out, nil
}

func (r *subscriptionRepo) CountActiveByPlan(ctx context.Context, tx repository.Tx) (map[string]int, error) {
	const q = `
SELECT plan_id, COUNT(*)
  FROM user_subscriptions
 WHERE status IN ('active','reserved')
 GROUP BY plan_id;`
	rows, err := queryRows(ctx, r.pool, nil, q)
	if err != nil {
		switch err {
		case pgx.ErrNoRows:
			return nil, domain.ErrNotFound
		case domain.ErrInvalidArgument, domain.ErrInvalidExecContext:
			return nil, err
		default:
			return nil, domain.ErrOperationFailed
		}
	}
	defer rows.Close()
	m := make(map[string]int)
	for rows.Next() {
		var planID string
		var c int
		if err := rows.Scan(&planID, &c); err != nil {
			return nil, domain.ErrReadDatabaseRow
		}
		m[planID] = c
	}
	if err := rows.Err(); err != nil {
		return nil, domain.ErrReadDatabaseRow
	}
	return m, nil
}

func (r *subscriptionRepo) TotalRemainingCredits(ctx context.Context, tx repository.Tx) (int64, error) {
	const q = `SELECT COALESCE(SUM(remaining_credits),0) FROM user_subscriptions WHERE status IN ('active','reserved');`
	var n int64
	row, err := pickRow(ctx, r.pool, tx, q)
	if err != nil {
		return 0, err
	}

	if err := row.Scan(&n); err != nil {
		return 0, domain.ErrReadDatabaseRow
	}
	return n, nil
}

func (r *subscriptionRepo) CountByStatus(ctx context.Context, tx repository.Tx) (map[model.SubscriptionStatus]int, error) {
	const q = `SELECT status, COUNT(*) FROM user_subscriptions GROUP BY status;`
	rows, err := queryRows(ctx, r.pool, tx, q)
	if err != nil {
		return nil, domain.ErrOperationFailed
	}
	defer rows.Close()

	counts := make(map[model.SubscriptionStatus]int)
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, domain.ErrReadDatabaseRow
		}
		counts[model.SubscriptionStatus(status)] = count
	}
	if err := rows.Err(); err != nil {
		return nil, domain.ErrReadDatabaseRow
	}
	return counts, nil
}

func (r *subscriptionRepo) queryOne(ctx context.Context, tx repository.Tx, sql string, args ...any) (*model.UserSubscription, error) {
	row, err := pickRow(ctx, r.pool, tx, sql, args...)
	if err != nil {
		return nil, err
	}

	s := &model.UserSubscription{}
	var status string
	if err := row.Scan(&s.ID, &s.UserID, &s.PlanID, &s.CreatedAt, &s.ScheduledStartAt, &s.StartAt, &s.ExpiresAt, &s.RemainingCredits, &status); err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, domain.ErrReadDatabaseRow
	}
	s.Status = model.SubscriptionStatus(status)
	return s, nil
}
