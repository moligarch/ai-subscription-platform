package repository

import (
	"context"

	"telegram-ai-subscription/internal/domain/model"
)

type ModelPricingRepository interface {
	// Get active pricing for a model
	GetByModelName(ctx context.Context, tx Tx, model string) (*model.ModelPricing, error)
	// Upsert admin changes
	Save(ctx context.Context, tx Tx, p *model.ModelPricing) error
	// List (for admin UI later)
	ListActive(ctx context.Context, tx Tx) ([]*model.ModelPricing, error)
}
