package repository

import (
	"context"
	"telegram-ai-subscription/internal/domain/model"
)

// SubscriptionPlanRepository is the port for plan persistence.
type SubscriptionPlanRepository interface {
	Save(ctx context.Context, plan *model.SubscriptionPlan) error
	FindByID(ctx context.Context, id string) (*model.SubscriptionPlan, error)
	ListAll(ctx context.Context) ([]*model.SubscriptionPlan, error)
	Delete(ctx context.Context, id string) error
}
