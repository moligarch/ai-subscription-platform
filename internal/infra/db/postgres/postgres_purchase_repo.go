// File: internal/infra/db/postgres/postgres_purchase_repo.go
package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"

	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/repository"
)

var _ repository.PurchaseRepository = (*PostgresPurchaseRepo)(nil)

type PostgresPurchaseRepo struct {
	pool *pgxpool.Pool
}

func NewPostgresPurchaseRepo(pool *pgxpool.Pool) *PostgresPurchaseRepo {
	return &PostgresPurchaseRepo{pool: pool}
}

func (r *PostgresPurchaseRepo) Save(ctx context.Context, qx any, pu *model.Purchase) error {
	const q = `
INSERT INTO purchases (id, user_id, plan_id, payment_id, subscription_id, created_at)
VALUES ($1,$2,$3,$4,$5,$6)
ON CONFLICT (id) DO NOTHING;`
	switch v := qx.(type) {
	case pgx.Tx:
		_, err := v.Exec(ctx, q, pu.ID, pu.UserID, pu.PlanID, pu.PaymentID, pu.SubscriptionID, pu.CreatedAt)
		return err
	case *pgxpool.Conn:
		_, err := v.Exec(ctx, q, pu.ID, pu.UserID, pu.PlanID, pu.PaymentID, pu.SubscriptionID, pu.CreatedAt)
		return err
	default:
		_, err := r.pool.Exec(ctx, q, pu.ID, pu.UserID, pu.PlanID, pu.PaymentID, pu.SubscriptionID, pu.CreatedAt)
		return err
	}
}

func (r *PostgresPurchaseRepo) ListByUser(ctx context.Context, qx any, userID string) ([]*model.Purchase, error) {
	const q = `
SELECT id, user_id, plan_id, payment_id, subscription_id, created_at
  FROM purchases WHERE user_id=$1 ORDER BY created_at DESC;`
	var rows pgx.Rows
	var err error
	switch v := qx.(type) {
	case pgx.Tx:
		rows, err = v.Query(ctx, q, userID)
	case *pgxpool.Conn:
		rows, err = v.Query(ctx, q, userID)
	default:
		rows, err = r.pool.Query(ctx, q, userID)
	}
	if err != nil {
		return nil, err
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
	return out, rows.Err()
}
