package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"

	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/repository"
)

type PostgresPurchaseRepo struct {
	db *pgxpool.Pool
}

func NewPostgresPurchaseRepo(db *pgxpool.Pool) *PostgresPurchaseRepo {
	return &PostgresPurchaseRepo{db: db}
}

var _ repository.PurchaseRepository = (*PostgresPurchaseRepo)(nil)

func (r *PostgresPurchaseRepo) Save(ctx context.Context, pu *model.Purchase) error {
	if pu.CreatedAt.IsZero() {
		pu.CreatedAt = time.Now().UTC()
	}
	_, err := r.db.Exec(ctx, `
		INSERT INTO purchases (id, user_id, plan_id, payment_id, subscription_id, created_at)
		VALUES ($1,$2,$3,$4,$5,$6)
	`, pu.ID, pu.UserID, pu.PlanID, pu.PaymentID, pu.SubscriptionID, pu.CreatedAt)
	return err
}

func (r *PostgresPurchaseRepo) ListByUser(ctx context.Context, userID string) ([]*model.Purchase, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, user_id, plan_id, payment_id, subscription_id, created_at
		FROM purchases WHERE user_id=$1 ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.Purchase
	for rows.Next() {
		var pu model.Purchase
		if err := rows.Scan(&pu.ID, &pu.UserID, &pu.PlanID, &pu.PaymentID, &pu.SubscriptionID, &pu.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &pu)
	}
	return out, rows.Err()
}
