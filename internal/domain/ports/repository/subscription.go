package repository

import (
	"context"
	"telegram-ai-subscription/internal/domain/model"
)

// -----------------------------
// Subscriptions
// -----------------------------

type SubscriptionRepository interface {
	Save(ctx context.Context, tx Tx, s *model.UserSubscription) error
	FindActiveByUserAndPlan(ctx context.Context, tx Tx, userID, planID string) (*model.UserSubscription, error)
	FindActiveByUser(ctx context.Context, tx Tx, userID string) (*model.UserSubscription, error)
	FindReservedByUser(ctx context.Context, tx Tx, userID string) ([]*model.UserSubscription, error)
	FindByID(ctx context.Context, tx Tx, id string) (*model.UserSubscription, error)
	FindExpiring(ctx context.Context, tx Tx, withinDays int) ([]*model.UserSubscription, error)
	CountActiveByPlan(ctx context.Context, tx Tx) (map[string]int, error)
	TotalRemainingCredits(ctx context.Context, tx Tx) (int64, error)
	CountByStatus(ctx context.Context, tx Tx) (map[model.SubscriptionStatus]int, error)
}
