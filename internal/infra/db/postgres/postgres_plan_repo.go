package postgres

import (
	"context"
	"fmt"

	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/repository"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

// Ensure interface compliance
var _ repository.SubscriptionPlanRepository = (*PostgresPlanRepo)(nil)

type PostgresPlanRepo struct {
	pool *pgxpool.Pool
}

func NewPostgresPlanRepo(pool *pgxpool.Pool) *PostgresPlanRepo {
	return &PostgresPlanRepo{pool: pool}
}

func (r *PostgresPlanRepo) Save(ctx context.Context, plan *model.SubscriptionPlan) error {
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

func (r *PostgresPlanRepo) FindByID(ctx context.Context, id string) (*model.SubscriptionPlan, error) {
	const sql = `
SELECT id, name, duration_days, credits, created_at
  FROM subscription_plans
 WHERE id = $1;
`
	row := r.pool.QueryRow(ctx, sql, id)
	var p model.SubscriptionPlan
	if err := row.Scan(&p.ID, &p.Name, &p.DurationDays, &p.Credits, &p.CreatedAt); err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("FindByID plan: %w", err)
	}
	return &p, nil
}

func (r *PostgresPlanRepo) ListAll(ctx context.Context) ([]*model.SubscriptionPlan, error) {
	const sql = `
SELECT id, name, duration_days, credits, created_at
  FROM subscription_plans;
`
	rows, err := r.pool.Query(ctx, sql)
	if err != nil {
		return nil, fmt.Errorf("ListAll plans: %w", err)
	}
	defer rows.Close()
	var out []*model.SubscriptionPlan
	for rows.Next() {
		var p model.SubscriptionPlan
		if err := rows.Scan(&p.ID, &p.Name, &p.DurationDays, &p.Credits, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &p)
	}
	return out, nil
}

func (r *PostgresPlanRepo) Delete(ctx context.Context, id string) error {
	// 1) check for active subscriptions
	const countSQL = `
SELECT COUNT(1) FROM user_subscriptions s
WHERE s.plan_id = $1 AND s.is_active = true;
`
	var cnt int
	if err := r.pool.QueryRow(ctx, countSQL, id).Scan(&cnt); err != nil {
		return fmt.Errorf("postgres CountActiveByPlan: %w", err)
	}
	if cnt > 0 {
		return fmt.Errorf("cannot delete plan %s: %d active subscriptions exist", id, cnt)
	}

	// 2) safe to delete
	const delSQL = `DELETE FROM subscription_plans WHERE id = $1;`
	ct, err := r.pool.Exec(ctx, delSQL, id)
	if err != nil {
		return fmt.Errorf("postgres Delete plan: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("plan %s not found", id)
	}
	return nil
}
