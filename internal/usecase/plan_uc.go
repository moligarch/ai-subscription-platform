// File: internal/usecase/plan_uc.go
package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/repository"

	"github.com/google/uuid"
)

// PlanUseCase manages subscription plans.
type PlanUseCase struct {
	repo repository.SubscriptionPlanRepository
}

// NewPlanUseCase constructs a PlanUseCase.
func NewPlanUseCase(repo repository.SubscriptionPlanRepository) *PlanUseCase {
	return &PlanUseCase{repo: repo}
}

// Create validates and saves a new subscription plan.
// It returns an error if a plan with the same name already exists.
func (uc *PlanUseCase) Create(ctx context.Context, plan *domain.SubscriptionPlan) error {
	if plan == nil {
		return fmt.Errorf("plan is nil")
	}
	trimName := strings.TrimSpace(plan.Name)
	if trimName == "" {
		return fmt.Errorf("plan name is required")
	}

	// Check duplicate names (case-insensitive)
	all, err := uc.repo.ListAll(ctx)
	if err != nil {
		return fmt.Errorf("list plans: %w", err)
	}
	lower := strings.ToLower(trimName)
	for _, p := range all {
		if strings.ToLower(strings.TrimSpace(p.Name)) == lower {
			return fmt.Errorf("plan with name %q already exists", plan.Name)
		}
	}

	// assign ID if missing
	if plan.ID == "" {
		plan.ID = uuid.NewString()
	}
	// set CreatedAt if zero
	if plan.CreatedAt.IsZero() {
		plan.CreatedAt = time.Now()
	}

	if err := uc.repo.Save(ctx, plan); err != nil {
		return fmt.Errorf("save plan: %w", err)
	}
	return nil
}

// Get returns a plan by id.
func (uc *PlanUseCase) Get(ctx context.Context, id string) (*domain.SubscriptionPlan, error) {
	return uc.repo.FindByID(ctx, id)
}

// List returns all plans.
func (uc *PlanUseCase) List(ctx context.Context) ([]*domain.SubscriptionPlan, error) {
	return uc.repo.ListAll(ctx)
}
