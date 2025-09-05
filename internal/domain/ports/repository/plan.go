package repository

import (
	"context"
	"telegram-ai-subscription/internal/domain/model"
)

// -----------------------------
// Subscription Plans
// -----------------------------

type SubscriptionPlanRepository interface {
	Save(ctx context.Context, tx Tx, plan *model.SubscriptionPlan) error
	Delete(ctx context.Context, tx Tx, id string) error
	FindByID(ctx context.Context, tx Tx, id string) (*model.SubscriptionPlan, error)
	ListAll(ctx context.Context, tx Tx) ([]*model.SubscriptionPlan, error)
}
