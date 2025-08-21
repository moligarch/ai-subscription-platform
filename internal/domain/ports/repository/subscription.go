package repository

import (
	"context"
	"telegram-ai-subscription/internal/domain/model"

	"github.com/jackc/pgx/v4"
)

// SubscriptionRepository defines persistence operations for subscriptions.
// It provides both non-transactional methods and TX-aware variants (prefixed with Tx) that accept pgx.Tx.
// This allows usecases to perform multi-step operations in a single transaction.
// SubscriptionRepository is the port for user subscriptions.
type SubscriptionRepository interface {
	// Save inserts or updates subscription.
	Save(ctx context.Context, sub *model.UserSubscription) error

	// SaveTx saves inside a transaction (Postgres repo will need this).
	SaveTx(ctx context.Context, tx pgx.Tx, sub *model.UserSubscription) error

	// FindByID returns subscription by id.
	FindByID(ctx context.Context, subID string) (*model.UserSubscription, error)
	FindByIDTx(ctx context.Context, tx pgx.Tx, subID string) (*model.UserSubscription, error)

	// FindActiveByUser returns the *single* currently active subscription for the user (nil + ErrNotFound if none).
	FindActiveByUser(ctx context.Context, userID string) (*model.UserSubscription, error)
	FindActiveByUserTx(ctx context.Context, tx pgx.Tx, userID string) (*model.UserSubscription, error)

	// FindActiveByUserAndPlan returns active subscription for user+plan (nil if none).
	FindActiveByUserAndPlanTx(ctx context.Context, tx pgx.Tx, userID, planID string) (*model.UserSubscription, error)

	// FindReservedByUser returns list of reserved subscriptions for a user (ordered by ScheduledStartAt asc).
	FindReservedByUser(ctx context.Context, userID string) ([]*model.UserSubscription, error)
	FindReservedByUserTx(ctx context.Context, tx pgx.Tx, userID string) ([]*model.UserSubscription, error)

	// FindExpiring returns subscriptions that expire at/sooner than now + withinDays
	FindExpiring(ctx context.Context, withinDays int) ([]*model.UserSubscription, error)

	// CountActiveByPlan returns map planID -> count of subscriptions whose status != finished.
	CountActiveByPlan(ctx context.Context) (map[string]int, error)

	// TotalRemainingCredits optimized aggregate
	TotalRemainingCredits(ctx context.Context) (int, error)
}
