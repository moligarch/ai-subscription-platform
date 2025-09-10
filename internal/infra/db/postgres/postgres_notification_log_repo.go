package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"

	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/ports/repository"
)

var _ repository.NotificationLogRepository = (*notificationLogRepo)(nil)

type notificationLogRepo struct {
	pool *pgxpool.Pool
}

func NewNotificationLogRepo(pool *pgxpool.Pool) repository.NotificationLogRepository {
	return &notificationLogRepo{pool: pool}
}

func (r *notificationLogRepo) Save(ctx context.Context, tx repository.Tx, subscriptionID, userID, kind string, thresholdDays int) error {
	const q = `
INSERT INTO subscription_notifications (id, subscription_id, user_id, kind, threshold_days)
VALUES ($1, $2, $3, $4, $5)`

	// We don't check for existence here. We let the database's UNIQUE constraint
	// on (subscription_id, kind, threshold_days) handle duplicate prevention.
	_, err := execSQL(ctx, r.pool, tx, q, uuid.NewString(), subscriptionID, userID, kind, thresholdDays)
	return err
}

func (r *notificationLogRepo) Exists(ctx context.Context, tx repository.Tx, subscriptionID, kind string, thresholdDays int) (bool, error) {
	// SELECT EXISTS(...) is more efficient than SELECT COUNT(*) as it stops on the first match.
	const q = `
SELECT EXISTS(
    SELECT 1 FROM subscription_notifications 
    WHERE subscription_id = $1 AND kind = $2 AND threshold_days = $3
)`
	var exists bool
	row, err := pickRow(ctx, r.pool, tx, q, subscriptionID, kind, thresholdDays)
	if err != nil {
		return false, err
	}

	if err := row.Scan(&exists); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil // Should not happen with SELECT EXISTS, but safe to handle.
		}
		return false, domain.ErrReadDatabaseRow
	}
	return exists, nil
}
