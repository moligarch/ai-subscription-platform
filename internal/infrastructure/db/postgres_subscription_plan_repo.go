package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/repository"
)

// Ensure interface compliance:
var _ repository.SubscriptionPlanRepository = (*PostgresPlanRepo)(nil)

type PostgresPlanRepo struct {
	pool *pgxpool.Pool
}

func NewPostgresPlanRepo(pool *pgxpool.Pool) *PostgresPlanRepo {
	return &PostgresPlanRepo{pool: pool}
}

func (r *PostgresPlanRepo) Save(ctx context.Context, plan *domain.SubscriptionPlan) error {
	const sql = `
INSERT INTO subscription_plans (id, name, duration_days, credits, created_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (id) DO UPDATE
  SET name          = EXCLUDED.name,
      duration_days = EXCLUDED.duration_days,
      credits       = EXCLUDED.credits;
`
	_, err := r.pool.Exec(ctx, sql,
		plan.ID, plan.Name, plan.DurationDays, plan.Credits, plan.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("Save plan: %w", err)
	}
	return nil
}

func (r *PostgresPlanRepo) FindByID(ctx context.Context, id string) (*domain.SubscriptionPlan, error) {
	const sql = `
SELECT id, name, duration_days, credits, created_at
  FROM subscription_plans
 WHERE id = $1;
`
	row := r.pool.QueryRow(ctx, sql, id)
	var p domain.SubscriptionPlan
	if err := row.Scan(&p.ID, &p.Name, &p.DurationDays, &p.Credits, &p.CreatedAt); err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("FindByID plan: %w", err)
	}
	return &p, nil
}

func (r *PostgresPlanRepo) ListAll(ctx context.Context) ([]*domain.SubscriptionPlan, error) {
	const sql = `
SELECT id, name, duration_days, credits, created_at
  FROM subscription_plans;
`
	rows, err := r.pool.Query(ctx, sql)
	if err != nil {
		return nil, fmt.Errorf("ListAll plans: %w", err)
	}
	defer rows.Close()
	var out []*domain.SubscriptionPlan
	for rows.Next() {
		var p domain.SubscriptionPlan
		if err := rows.Scan(&p.ID, &p.Name, &p.DurationDays, &p.Credits, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &p)
	}
	return out, nil
}
