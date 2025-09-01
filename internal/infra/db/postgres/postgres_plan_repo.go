package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"

	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/repository"
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
	if plan == nil {
		return errors.New("nil plan")
	}
	if plan.ID == "" {
		plan.ID = uuid.NewString()
	}
	const q = `
INSERT INTO subscription_plans (id, name, duration_days, credits, price_irr, created_at)
VALUES ($1, $2, $3, $4, $5, COALESCE($6, NOW()))
ON CONFLICT (id) DO UPDATE SET
  name = EXCLUDED.name,
  duration_days = EXCLUDED.duration_days,
  credits = EXCLUDED.credits,
  price_irr = EXCLUDED.price_irr;
`
	_, err := r.pool.Exec(ctx, q, plan.ID, plan.Name, plan.DurationDays, plan.Credits, plan.PriceIRR, plan.CreatedAt)
	if err != nil {
		return fmt.Errorf("save plan: %w", err)
	}
	return nil
}

func (r *PostgresPlanRepo) FindByID(ctx context.Context, id string) (*model.SubscriptionPlan, error) {
	const q = `SELECT id, name, duration_days, credits, price_irr, created_at FROM subscription_plans WHERE id = $1;`
	row := r.pool.QueryRow(ctx, q, id)
	var p model.SubscriptionPlan
	if err := row.Scan(&p.ID, &p.Name, &p.DurationDays, &p.Credits, &p.PriceIRR, &p.CreatedAt); err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("find plan: %w", err)
	}
	return &p, nil
}

func (r *PostgresPlanRepo) ListAll(ctx context.Context) ([]*model.SubscriptionPlan, error) {
	const q = `SELECT id, name, duration_days, credits, price_irr, created_at FROM subscription_plans ORDER BY created_at ASC;`
	rows, err := r.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list plans: %w", err)
	}
	defer rows.Close()

	var out []*model.SubscriptionPlan
	for rows.Next() {
		var p model.SubscriptionPlan
		if err := rows.Scan(&p.ID, &p.Name, &p.DurationDays, &p.Credits, &p.PriceIRR, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan plan: %w", err)
		}
		out = append(out, &p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows err: %w", err)
	}
	return out, nil
}

func (r *PostgresPlanRepo) Delete(ctx context.Context, id string) error {
	// Guard: prevent deletion if there are active or reserved subscriptions using this plan.
	const qGuard = `
SELECT COUNT(1)
  FROM user_subscriptions
 WHERE plan_id = $1
   AND status IN ('active','reserved');
`
	var n int
	if err := r.pool.QueryRow(ctx, qGuard, id).Scan(&n); err != nil {
		return fmt.Errorf("guard plan delete: %w", err)
	}
	if n > 0 {
		return fmt.Errorf("cannot delete plan with %d active/reserved subscriptions", n)
	}
	const q = `DELETE FROM subscription_plans WHERE id = $1;`
	ct, err := r.pool.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("delete plan: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}
