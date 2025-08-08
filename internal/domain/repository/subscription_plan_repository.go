package repository

import (
	"context"
	"telegram-ai-subscription/internal/domain"
)

// SubscriptionPlanRepository is the port for plan persistence.
type SubscriptionPlanRepository interface {
	Save(ctx context.Context, plan *domain.SubscriptionPlan) error
	FindByID(ctx context.Context, id string) (*domain.SubscriptionPlan, error)
	ListAll(ctx context.Context) ([]*domain.SubscriptionPlan, error)
}
