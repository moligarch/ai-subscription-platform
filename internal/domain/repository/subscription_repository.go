package repository

import (
	"context"
	"telegram-ai-subscription/internal/domain"
)

// SubscriptionRepository is the port for user subscriptions.
type SubscriptionRepository interface {
	Save(ctx context.Context, sub *domain.UserSubscription) error
	FindActiveByUser(ctx context.Context, userID string) (*domain.UserSubscription, error)
	FindExpiring(ctx context.Context, withinDays int) ([]*domain.UserSubscription, error)
}
