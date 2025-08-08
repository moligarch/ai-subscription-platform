package usecase

import (
	"context"

	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/repository"
)

// PlanUseCase manages subscription plans.
type PlanUseCase struct {
	repo repository.SubscriptionPlanRepository
}

// NewPlanUseCase constructs a PlanUseCase.
func NewPlanUseCase(repo repository.SubscriptionPlanRepository) *PlanUseCase {
	return &PlanUseCase{repo: repo}
}

// Create saves or updates a plan.
func (uc *PlanUseCase) Create(ctx context.Context, plan *domain.SubscriptionPlan) error {
	return uc.repo.Save(ctx, plan)
}

// Get retrieves a plan by ID.
func (uc *PlanUseCase) Get(ctx context.Context, id string) (*domain.SubscriptionPlan, error) {
	return uc.repo.FindByID(ctx, id)
}

// List returns all plans.
func (uc *PlanUseCase) List(ctx context.Context) ([]*domain.SubscriptionPlan, error) {
	return uc.repo.ListAll(ctx)
}
