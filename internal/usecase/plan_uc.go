package usecase

import (
	"context"
	"time"

	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/repository"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// Compile-time check
var _ PlanUseCase = (*planUC)(nil)

type PlanUseCase interface {
	Create(ctx context.Context, name string, durationDays int, credits int64, priceIRR int64, supportedModels []string) (*model.SubscriptionPlan, error)
	Update(ctx context.Context, plan *model.SubscriptionPlan) error
	List(ctx context.Context) ([]*model.SubscriptionPlan, error)
	Get(ctx context.Context, id string) (*model.SubscriptionPlan, error)
	Delete(ctx context.Context, id string) error
	UpdatePricing(ctx context.Context, modelName string, inputPrice, outputPrice int64) error
	GenerateActivationCodes(ctx context.Context, planID string, count int) ([]string, error)
}

type planUC struct {
	plans  repository.SubscriptionPlanRepository
	prices repository.ModelPricingRepository
	codes  repository.ActivationCodeRepository
	log    *zerolog.Logger
}

func NewPlanUseCase(
	plans repository.SubscriptionPlanRepository,
	prices repository.ModelPricingRepository,
	codes repository.ActivationCodeRepository,
	logger *zerolog.Logger,
) *planUC {
	return &planUC{
		plans:  plans,
		prices: prices,
		codes:  codes,
		log:    logger,
	}
}

func (p *planUC) Create(ctx context.Context, name string, durationDays int, credits int64, priceIRR int64, supportedModels []string) (*model.SubscriptionPlan, error) {
	sp, err := model.NewSubscriptionPlan("", name, durationDays, credits, priceIRR)
	if err != nil {
		return nil, err
	}
	// Set the supported models from the arguments
	sp.SupportedModels = supportedModels
	if err := p.plans.Save(ctx, repository.NoTX, sp); err != nil {
		return nil, err
	}
	return sp, nil
}

func (p *planUC) Update(ctx context.Context, plan *model.SubscriptionPlan) error {
	if _, err := uuid.Parse(plan.ID); err != nil {
		return domain.ErrInvalidArgument
	}
	return p.plans.Save(ctx, repository.NoTX, plan)
}

func (p *planUC) List(ctx context.Context) ([]*model.SubscriptionPlan, error) {
	return p.plans.ListAll(ctx, repository.NoTX)
}

func (p *planUC) Get(ctx context.Context, id string) (*model.SubscriptionPlan, error) {
	if _, err := uuid.Parse(id); err != nil {
		return nil, domain.ErrInvalidArgument // Return a specific error for invalid format
	}
	return p.plans.FindByID(ctx, repository.NoTX, id)
}

func (p *planUC) Delete(ctx context.Context, id string) error {
	// First, validate that the provided ID is a valid UUID.
	if _, err := uuid.Parse(id); err != nil {
		return domain.ErrInvalidArgument // Return a specific error for invalid format
	}
	return p.plans.Delete(ctx, repository.NoTX, id)
}

func (p *planUC) UpdatePricing(ctx context.Context, modelName string, inputPrice, outputPrice int64) error {
	// Note: GetByModelName only finds ACTIVE models. This is a safe default for this command.
	pricing, err := p.prices.GetByModelName(ctx, nil, modelName)
	if err != nil {
		return err // Will be domain.ErrNotFound if not found
	}

	pricing.InputTokenPriceMicros = inputPrice
	pricing.OutputTokenPriceMicros = outputPrice

	// The repo was refactored to use Create/Update.
	return p.prices.Update(ctx, nil, pricing)
}

func (p *planUC) GenerateActivationCodes(ctx context.Context, planID string, count int) ([]string, error) {
	// 1. Validate that the plan exists
	plan, err := p.plans.FindByID(ctx, repository.NoTX, planID)
	if err != nil {
		return nil, domain.ErrPlanNotFound
	}

	if count <= 0 {
		count = 1
	}

	generatedCodes := make([]string, 0, count)
	for i := 0; i < count; i++ {
		codeStr, err := generateActivationCode()
		if err != nil {
			return nil, domain.ErrOperationFailed
		}

		newCode := &model.ActivationCode{
			Code:      codeStr,
			PlanID:    plan.ID,
			CreatedAt: time.Now(),
		}

		if err := p.codes.Save(ctx, repository.NoTX, newCode); err != nil {
			// If we fail, return what we have so far, but log the error
			p.log.Error().Err(err).Msg("failed to save activation code")
			return generatedCodes, err
		}
		generatedCodes = append(generatedCodes, codeStr)
	}

	return generatedCodes, nil
}
