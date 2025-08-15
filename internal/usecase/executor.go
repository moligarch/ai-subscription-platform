package usecase

import (
	"context"
	"time"

	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/repository"

	"github.com/google/uuid"
)

// UserSubscriptionDTO is a small, serializable subscription result used by workers and callers.
type UserSubscriptionDTO struct {
	ID               string
	UserID           string
	PlanID           string
	StartAt          time.Time
	ExpiresAt        time.Time
	RemainingCredits int
	Active           bool
	CreatedAt        time.Time
}

// SubscriptionExecutor provides an execution wrapper around SubscriptionUseCase.
// It adapts domain objects to DTOs to keep infra code lighter.
type SubscriptionExecutor struct {
	planRepo repository.SubscriptionPlanRepository
	subRepo  repository.SubscriptionRepository
}

// NewSubscriptionExecutor constructs an executor from repos (or you can pass usecase).
func NewSubscriptionExecutor(planRepo repository.SubscriptionPlanRepository, subRepo repository.SubscriptionRepository) *SubscriptionExecutor {
	return &SubscriptionExecutor{planRepo: planRepo, subRepo: subRepo}
}

// ExecuteSubscribe runs the subscribe flow (create or extend) and returns a DTO.
func (e *SubscriptionExecutor) ExecuteSubscribe(ctx context.Context, userID, planID string) (*UserSubscriptionDTO, error) {
	// Load plan
	plan, err := e.planRepo.FindByID(ctx, planID)
	if err != nil {
		return nil, err
	}
	// Try find existing
	existing, err := e.subRepo.FindActiveByUser(ctx, userID)
	if err != nil && err != domain.ErrNotFound {
		return nil, err
	}

	now := time.Now()
	var sub *model.UserSubscription
	if err == domain.ErrNotFound {
		sub = &model.UserSubscription{
			ID:               uuid.NewString(),
			UserID:           userID,
			PlanID:           planID,
			StartAt:          now,
			ExpiresAt:        now.Add(time.Duration(plan.DurationDays) * 24 * time.Hour),
			RemainingCredits: plan.Credits,
			Active:           true,
			CreatedAt:        now,
		}
	} else {
		// extend
		sub = existing
		sub.ExpiresAt = sub.ExpiresAt.Add(time.Duration(plan.DurationDays) * 24 * time.Hour)
		sub.RemainingCredits += plan.Credits
		sub.Active = true
	}

	if err := e.subRepo.Save(ctx, sub); err != nil {
		return nil, err
	}

	return &UserSubscriptionDTO{
		ID:               sub.ID,
		UserID:           sub.UserID,
		PlanID:           sub.PlanID,
		StartAt:          sub.StartAt,
		ExpiresAt:        sub.ExpiresAt,
		RemainingCredits: sub.RemainingCredits,
		Active:           sub.Active,
		CreatedAt:        sub.CreatedAt,
	}, nil
}
