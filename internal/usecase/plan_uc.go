// File: internal/usecase/plan_uc.go
package usecase

import (
	"context"

	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/repository"
)

// Compile-time check
var _ PlanUseCase = (*planUC)(nil)

type PlanUseCase interface {
	Create(ctx context.Context, name string, durationDays int, credits int, priceIRR int64) (*model.SubscriptionPlan, error)
	Update(ctx context.Context, plan *model.SubscriptionPlan) error
	List(ctx context.Context) ([]*model.SubscriptionPlan, error)
	Get(ctx context.Context, id string) (*model.SubscriptionPlan, error)
	Delete(ctx context.Context, id string) error
}

type planUC struct {
	plans repository.SubscriptionPlanRepository
}

func NewPlanUseCase(plans repository.SubscriptionPlanRepository) *planUC {
	return &planUC{plans: plans}
}

func (p *planUC) Create(ctx context.Context, name string, durationDays int, credits int, priceIRR int64) (*model.SubscriptionPlan, error) {
	sp := &model.SubscriptionPlan{
		Name:         name,
		DurationDays: durationDays,
		Credits:      credits,
		PriceIRR:     priceIRR,
	}
	if err := p.plans.Save(ctx, sp); err != nil {
		return nil, err
	}
	return sp, nil
}

func (p *planUC) Update(ctx context.Context, plan *model.SubscriptionPlan) error {
	return p.plans.Save(ctx, plan)
}

func (p *planUC) List(ctx context.Context) ([]*model.SubscriptionPlan, error) {
	return p.plans.ListAll(ctx)
}

func (p *planUC) Get(ctx context.Context, id string) (*model.SubscriptionPlan, error) {
	return p.plans.FindByID(ctx, id)
}

func (p *planUC) Delete(ctx context.Context, id string) error {
	return p.plans.Delete(ctx, id)
}
