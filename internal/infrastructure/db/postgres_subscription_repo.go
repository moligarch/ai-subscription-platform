package db

import (
	"context"
	"fmt"

	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/repository"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

// Ensure interface compliance:
var _ repository.SubscriptionRepository = (*PostgresSubscriptionRepo)(nil)

type PostgresSubscriptionRepo struct {
	pool *pgxpool.Pool
}

func NewPostgresSubscriptionRepo(pool *pgxpool.Pool) *PostgresSubscriptionRepo {
	return &PostgresSubscriptionRepo{pool: pool}
}

func (r *PostgresSubscriptionRepo) Save(ctx context.Context, us *domain.UserSubscription) error {
	const sql = `
INSERT INTO user_subscriptions
  (id, user_id, plan_id, start_at, expires_at, remaining_credits, is_active, created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
ON CONFLICT (id) DO UPDATE
  SET expires_at        = EXCLUDED.expires_at,
      remaining_credits = EXCLUDED.remaining_credits,
      is_active         = EXCLUDED.is_active;
`
	_, err := r.pool.Exec(ctx, sql,
		us.ID,
		us.UserID,
		us.PlanID,
		us.StartAt,
		us.ExpiresAt,
		us.RemainingCredits,
		us.Active,
		us.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("Save subscription: %w", err)
	}
	return nil
}

func (r *PostgresSubscriptionRepo) FindActiveByUser(ctx context.Context, userID string) (*domain.UserSubscription, error) {
	const sql = `
SELECT id, user_id, plan_id, start_at, expires_at, remaining_credits, is_active, created_at
  FROM user_subscriptions
 WHERE user_id=$1 AND is_active = true;
`
	row := r.pool.QueryRow(ctx, sql, userID)
	var us domain.UserSubscription
	if err := row.Scan(
		&us.ID,
		&us.UserID,
		&us.PlanID,
		&us.StartAt,
		&us.ExpiresAt,
		&us.RemainingCredits,
		&us.Active,
		&us.CreatedAt,
	); err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("FindActiveByUser: %w", err)
	}
	return &us, nil
}

func (r *PostgresSubscriptionRepo) FindExpiring(ctx context.Context, withinDays int) ([]*domain.UserSubscription, error) {
	const sql = `
SELECT id, user_id, plan_id, start_at, expires_at, remaining_credits, is_active, created_at
  FROM user_subscriptions
 WHERE is_active = true
   AND expires_at <= now() + $1 * interval '1 day';
`
	rows, err := r.pool.Query(ctx, sql, withinDays)
	if err != nil {
		return nil, fmt.Errorf("FindExpiring: %w", err)
	}
	defer rows.Close()

	var out []*domain.UserSubscription
	for rows.Next() {
		var us domain.UserSubscription
		if err := rows.Scan(
			&us.ID,
			&us.UserID,
			&us.PlanID,
			&us.StartAt,
			&us.ExpiresAt,
			&us.RemainingCredits,
			&us.Active,
			&us.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, &us)
	}
	return out, nil
}

// CountActiveByPlan returns a map: plan name -> count of active subscriptions
func (r *PostgresSubscriptionRepo) CountActiveByPlan(ctx context.Context) (map[string]int, error) {
	const sql = `
SELECT p.name, COUNT(*)::int
  FROM user_subscriptions s
  JOIN subscription_plans p ON s.plan_id = p.id
 WHERE s.is_active = TRUE
 GROUP BY p.name;
`
	rows, err := r.pool.Query(ctx, sql)
	if err != nil {
		return nil, fmt.Errorf("postgres CountActiveByPlan query: %w", err)
	}
	defer rows.Close()

	result := make(map[string]int)
	for rows.Next() {
		var name string
		var cnt int
		if err := rows.Scan(&name, &cnt); err != nil {
			return nil, fmt.Errorf("postgres CountActiveByPlan scan: %w", err)
		}
		result[name] = cnt
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres CountActiveByPlan rows: %w", err)
	}
	return result, nil
}

// TotalRemainingCredits returns sum(remaining_credits) over active subscriptions.
func (r *PostgresSubscriptionRepo) TotalRemainingCredits(ctx context.Context) (int, error) {
	const sql = `
SELECT COALESCE(SUM(remaining_credits)::int, 0) FROM user_subscriptions
WHERE is_active = TRUE;
`
	var total int
	row := r.pool.QueryRow(ctx, sql)
	if err := row.Scan(&total); err != nil {
		return 0, fmt.Errorf("postgres TotalRemainingCredits query: %w", err)
	}
	return total, nil
}
