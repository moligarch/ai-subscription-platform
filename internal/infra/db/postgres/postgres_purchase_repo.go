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

var _ repository.PurchaseRepository = (*purchaseRepo)(nil)

type purchaseRepo struct {
	pool *pgxpool.Pool
}

func NewPurchaseRepo(pool *pgxpool.Pool) *purchaseRepo {
	return &purchaseRepo{pool: pool}
}

func (r *purchaseRepo) Save(ctx context.Context, tx repository.Tx, pu *model.Purchase) error {
	const q = `
INSERT INTO purchases (id, user_id, plan_id, payment_id, subscription_id, created_at)
VALUES ($1,$2,$3,$4,$5,$6)
ON CONFLICT (id) DO NOTHING;`

	_, err := execSQL(ctx, r.pool, tx, q, pu.ID, pu.UserID, pu.PlanID, pu.PaymentID, pu.SubscriptionID, pu.CreatedAt)
	if err != nil {
		if err == domain.ErrInvalidArgument || err == domain.ErrInvalidExecContext {
			return err
		}
		return domain.ErrOperationFailed
	}
	return nil
}

func (r *purchaseRepo) ListByUser(ctx context.Context, tx repository.Tx, userID string) ([]*model.Purchase, error) {
	const q = `
SELECT id, user_id, plan_id, payment_id, subscription_id, created_at
  FROM purchases WHERE user_id=$1 ORDER BY created_at DESC;`
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

	var out []*model.Purchase
	for rows.Next() {
		var pu model.Purchase
		if err := rows.Scan(&pu.ID, &pu.UserID, &pu.PlanID, &pu.PaymentID, &pu.SubscriptionID, &pu.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan purchase: %w", err)
		}
		out = append(out, &pu)
	}
	return out, nil
}
