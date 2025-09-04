package usecase

import (
	"context"

	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/repository"

	"github.com/rs/zerolog"
)

// Compile-time check
var _ PlanUseCase = (*planUC)(nil)

type PlanUseCase interface {
	Create(ctx context.Context, name string, durationDays int, credits int64, priceIRR int64) (*model.SubscriptionPlan, error)
	Update(ctx context.Context, plan *model.SubscriptionPlan) error
	List(ctx context.Context) ([]*model.SubscriptionPlan, error)
	Get(ctx context.Context, id string) (*model.SubscriptionPlan, error)
	Delete(ctx context.Context, id string) error
}

type planUC struct {
	plans repository.SubscriptionPlanRepository

	log *zerolog.Logger
}

func NewPlanUseCase(plans repository.SubscriptionPlanRepository, logger *zerolog.Logger) *planUC {
	return &planUC{plans: plans, log: logger}
}

func (p *planUC) Create(ctx context.Context, name string, durationDays int, credits int64, priceIRR int64) (*model.SubscriptionPlan, error) {
	sp := &model.SubscriptionPlan{
		Name:         name,
		DurationDays: durationDays,
		Credits:      credits,
		PriceIRR:     priceIRR,
	}
	if err := p.plans.Save(ctx, repository.NoTX, sp); err != nil {
		return nil, err
	}
	return sp, nil
}

func (p *planUC) Update(ctx context.Context, plan *model.SubscriptionPlan) error {
	return p.plans.Save(ctx, repository.NoTX, plan)
}

func (p *planUC) List(ctx context.Context) ([]*model.SubscriptionPlan, error) {
	return p.plans.ListAll(ctx, repository.NoTX)
}

func (p *planUC) Get(ctx context.Context, id string) (*model.SubscriptionPlan, error) {
	return p.plans.FindByID(ctx, repository.NoTX, id)
}

func (p *planUC) Delete(ctx context.Context, id string) error {
	return p.plans.Delete(ctx, repository.NoTX, id)
}
