package repository

import (
	"context"
	"telegram-ai-subscription/internal/domain/model"
)

// -----------------------------
// Subscriptions
// -----------------------------

type SubscriptionRepository interface {
	Save(ctx context.Context, qx any, s *model.UserSubscription) error
	FindActiveByUserAndPlan(ctx context.Context, qx any, userID, planID string) (*model.UserSubscription, error)
	FindActiveByUser(ctx context.Context, qx any, userID string) (*model.UserSubscription, error)
	FindReservedByUser(ctx context.Context, qx any, userID string) ([]*model.UserSubscription, error)
	FindByID(ctx context.Context, qx any, id string) (*model.UserSubscription, error)
	FindExpiring(ctx context.Context, qx any, withinDays int) ([]*model.UserSubscription, error)
	CountActiveByPlan(ctx context.Context, qx any) (map[string]int, error)
	TotalRemainingCredits(ctx context.Context, qx any) (int, error)
}

// -----------------------------
// Notifications Log
// -----------------------------

type NotificationLogRepository interface {
	SaveExpiry(ctx context.Context, qx any, subscriptionID, userID string, thresholdDays int) error
	ExistsExpiry(ctx context.Context, qx any, subscriptionID string, thresholdDays int) (bool, error)
}
