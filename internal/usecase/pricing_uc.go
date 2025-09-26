package usecase

import (
	"context"
	"strings"

	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/repository"

	"github.com/rs/zerolog"
)

// PricingUseCase defines CRUD operations for model pricing.
// Implementation will be provided in a later step.
type PricingUseCase interface {
	// List returns all active model pricing rows, ordered by name.
	List(ctx context.Context) ([]*model.ModelPricing, error)

	// Get returns the active pricing for a specific model name.
	Get(ctx context.Context, modelName string) (*model.ModelPricing, error)

	// Create inserts a new pricing row. If the model already exists and is active,
	// the implementation should return ErrAlreadyExists.
	Create(ctx context.Context, modelName string, inputMicros, outputMicros int64, currency string) (*model.ModelPricing, error)

	// Update mutates fields for an existing pricing row (identified by modelName).
	// Nil pointers mean "no change".
	Update(ctx context.Context, modelName string, inputMicros, outputMicros *int64, currency *string) (*model.ModelPricing, error)

	// Delete deactivates a model's pricing (soft-delete). If there are references,
	// the implementation should handle it safely (e.g., set Active=false).
	Delete(ctx context.Context, modelName string) error
}

var _ PricingUseCase = (*pricingUC)(nil)

type pricingUC struct {
	prices repository.ModelPricingRepository
	tx     repository.TransactionManager
	log    *zerolog.Logger
}

// NewPricingUseCase constructs the use case using the model pricing repository.
// tx and logger may be nil (the implementation tolerates NoTX + noop logging).
func NewPricingUseCase(
	prices repository.ModelPricingRepository,
	tx repository.TransactionManager,
	logger *zerolog.Logger,
) PricingUseCase {
	return &pricingUC{
		prices: prices,
		tx:     tx,
		log:    logger,
	}
}

// List returns active model pricing rows.
func (p *pricingUC) List(ctx context.Context) ([]*model.ModelPricing, error) {
	return p.prices.ListActive(ctx, repository.NoTX)
}

// Get returns pricing for a given model name (case-insensitive, normalized).
func (p *pricingUC) Get(ctx context.Context, modelName string) (*model.ModelPricing, error) {
	mn := normalizeModelName(modelName)
	return p.prices.GetByModelName(ctx, repository.NoTX, mn)
}

// Create inserts a new active pricing row.
// Returns domain.ErrAlreadyExists if the model already has active pricing.
func (p *pricingUC) Create(ctx context.Context, modelName string, inputMicros, outputMicros int64, _ string) (*model.ModelPricing, error) {
	mn := normalizeModelName(modelName)
	if mn == "" {
		return nil, domain.ErrInvalidArgument
	}
	// Check existence
	if existing, err := p.prices.GetByModelName(ctx, repository.NoTX, mn); err == nil && existing != nil {
		return nil, domain.ErrAlreadyExists
	}
	// Create active record
	rec := model.NewModelPricing(mn, inputMicros, outputMicros, true)
	if err := p.prices.Create(ctx, repository.NoTX, rec); err != nil {
		return nil, err
	}
	return rec, nil
}

// Update mutates fields for an existing pricing row.
// Nil pointers mean "no change". Returns domain.ErrNotFound if missing.
func (p *pricingUC) Update(ctx context.Context, modelName string, inputMicros, outputMicros *int64, _ *string) (*model.ModelPricing, error) {
	mn := normalizeModelName(modelName)
	rec, err := p.prices.GetByModelName(ctx, repository.NoTX, mn)
	if err != nil {
		return nil, err // expected to be domain.ErrNotFound
	}
	if inputMicros != nil {
		rec.InputTokenPriceMicros = *inputMicros
	}
	if outputMicros != nil {
		rec.OutputTokenPriceMicros = *outputMicros
	}
	if err := p.prices.Update(ctx, repository.NoTX, rec); err != nil {
		return nil, err
	}
	return rec, nil
}

// Delete implements a soft delete: mark Active=false and persist.
// Returns domain.ErrNotFound if missing; double-delete is idempotent success.
func (p *pricingUC) Delete(ctx context.Context, modelName string) error {
	mn := normalizeModelName(modelName)
	rec, err := p.prices.GetByModelName(ctx, repository.NoTX, mn)
	if err != nil {
		return err // expected to be domain.ErrNotFound
	}
	if !rec.Active {
		// already inactive; idempotent success
		return nil
	}
	rec.Active = false
	return p.prices.Update(ctx, repository.NoTX, rec)
}

func normalizeModelName(s string) string {
	return strings.TrimSpace(s)
}
