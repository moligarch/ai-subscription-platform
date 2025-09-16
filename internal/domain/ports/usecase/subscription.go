package usecase

import (
	"context"
	"telegram-ai-subscription/internal/domain/model"
)

// SubscriptionManager defines the subscription-related operations needed by external components like background workers.
type SubscriptionManager interface {
	DeductCredits(ctx context.Context, userID string, amount int64) (*model.UserSubscription, error)
	GetActive(ctx context.Context, userID string) (*model.UserSubscription, error)
}
