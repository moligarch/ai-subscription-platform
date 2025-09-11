package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"

	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/repository"
)

// Ensure interface compliance
var _ repository.SubscriptionPlanRepository = (*planRepo)(nil)

type planRepo struct {
	pool *pgxpool.Pool
}

func NewPlanRepo(pool *pgxpool.Pool) *planRepo {
	return &planRepo{pool: pool}
}

func (r *planRepo) Save(ctx context.Context, tx repository.Tx, plan *model.SubscriptionPlan) error {
	if plan == nil {
		return domain.ErrInvalidArgument
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
  price_irr = EXCLUDED.price_irr;`

	_, err := execSQL(ctx, r.pool, tx, q, plan.ID, plan.Name, plan.DurationDays, plan.Credits, plan.PriceIRR, plan.CreatedAt)
	if err != nil {
		if err == domain.ErrInvalidArgument || err == domain.ErrInvalidExecContext {
			return err
		}
		return domain.ErrOperationFailed
	}
	return nil
}

func (r *planRepo) Delete(ctx context.Context, tx repository.Tx, id string) error {
	// Guard: prevent deletion if there are active or reserved subscriptions using this plan.
	const qGuard = `
SELECT COUNT(1)
  FROM user_subscriptions
 WHERE plan_id = $1
   AND status IN ('active','reserved');`
	row, err := pickRow(ctx, r.pool, tx, qGuard, id)
	if err != nil {
		return err
	}

	var n int
	if err := row.Scan(&n); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return domain.ErrReadDatabaseRow
	}

	if n > 0 {
		return domain.ErrSubsciptionWithActiveUser
	}

	const q = `DELETE FROM subscription_plans WHERE id = $1;`
	exec, err := getExecutor(r.pool, tx)
	if err != nil {
		return err
	}
	ct, err := exec.Exec(ctx, q, id)
	if err != nil {
		return domain.ErrOperationFailed
	}
	if ct.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *planRepo) FindByID(ctx context.Context, tx repository.Tx, id string) (*model.SubscriptionPlan, error) {
	const q = `SELECT id, name, duration_days, credits, price_irr, created_at FROM subscription_plans WHERE id = $1;`

	row, err := pickRow(ctx, r.pool, nil, q, id)
	if err != nil {
		return nil, err
	}

	var p model.SubscriptionPlan
	if err := row.Scan(&p.ID, &p.Name, &p.DurationDays, &p.Credits, &p.PriceIRR, &p.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, domain.ErrReadDatabaseRow
	}
	return &p, nil
}

func (r *planRepo) ListAll(ctx context.Context, tx repository.Tx) ([]*model.SubscriptionPlan, error) {
	const q = `SELECT id, name, duration_days, credits, price_irr, created_at FROM subscription_plans ORDER BY created_at ASC;`
	rows, err := queryRows(ctx, r.pool, tx, q)
	if err != nil {
		switch err {
		case domain.ErrInvalidExecContext, domain.ErrInvalidArgument:
			return nil, domain.ErrInvalidArgument
		default:
			return nil, domain.ErrReadDatabaseRow
		}
	}
	defer rows.Close()

	var out []*model.SubscriptionPlan
	for rows.Next() {
		var p model.SubscriptionPlan
		if err := rows.Scan(&p.ID, &p.Name, &p.DurationDays, &p.Credits, &p.PriceIRR, &p.CreatedAt); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, domain.ErrNotFound
			}
			return nil, domain.ErrReadDatabaseRow
		}
		out = append(out, &p)
	}
	if err := rows.Err(); err != nil {
		return nil, domain.ErrReadDatabaseRow
	}
	return out, nil
}
