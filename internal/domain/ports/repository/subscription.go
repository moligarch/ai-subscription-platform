package repository

import (
	"context"
	"telegram-ai-subscription/internal/domain/model"
)

// SubscriptionRepository is the port for user subscriptions.
type SubscriptionRepository interface {
	Save(ctx context.Context, sub *model.UserSubscription) error
	FindActiveByUser(ctx context.Context, userID string) (*model.UserSubscription, error)
	FindExpiring(ctx context.Context, withinDays int) ([]*model.UserSubscription, error)

	// --- Statistics read-only methods ---
	// CountActiveByPlan returns a map where the key is the plan name (or plan id)
	// and the value is the number of active subscriptions for that plan.
	CountActiveByPlan(ctx context.Context) (map[string]int, error)

	// TotalRemainingCredits returns the sum of remaining credits for currently active subscriptions.
	TotalRemainingCredits(ctx context.Context) (int, error)
}
