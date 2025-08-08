package usecase

import (
	"context"
	"time"

	"github.com/google/uuid"
	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/repository"
)

// SubscriptionUseCase manages user subscriptions.
type SubscriptionUseCase struct {
	planRepo repository.SubscriptionPlanRepository
	subRepo  repository.SubscriptionRepository
}

// NewSubscriptionUseCase constructs a SubscriptionUseCase.
func NewSubscriptionUseCase(
	planRepo repository.SubscriptionPlanRepository,
	subRepo repository.SubscriptionRepository,
) *SubscriptionUseCase {
	return &SubscriptionUseCase{planRepo: planRepo, subRepo: subRepo}
}

// Subscribe either creates a new subscription or extends an existing one.
func (uc *SubscriptionUseCase) Subscribe(ctx context.Context, userID, planID string) (*domain.UserSubscription, error) {
	// 1. Load the plan
	plan, err := uc.planRepo.FindByID(ctx, planID)
	if err != nil {
		return nil, err
	}

	// 2. Look for an active subscription
	existing, err := uc.subRepo.FindActiveByUser(ctx, userID)
	if err != nil && err != domain.ErrNotFound {
		return nil, err
	}

	now := time.Now()
	if err == domain.ErrNotFound {
		// 3a. No active subscription → create new
		sub := &domain.UserSubscription{
			ID:               uuid.NewString(),
			UserID:           userID,
			PlanID:           planID,
			StartAt:          now,
			ExpiresAt:        now.Add(time.Duration(plan.DurationDays) * 24 * time.Hour),
			RemainingCredits: plan.Credits,
			Active:           true,
			CreatedAt:        now,
		}
		if err := uc.subRepo.Save(ctx, sub); err != nil {
			return nil, err
		}
		return sub, nil
	}

	// 3b. Existing active subscription → extend
	existing.ExpiresAt = existing.ExpiresAt.Add(time.Duration(plan.DurationDays) * 24 * time.Hour)
	existing.RemainingCredits += plan.Credits

	if err := uc.subRepo.Save(ctx, existing); err != nil {
		return nil, err
	}
	return existing, nil
}

// GetActiveSubscription retrieves the current active subscription for a user.
func (uc *SubscriptionUseCase) GetActiveSubscription(ctx context.Context, userID string) (*domain.UserSubscription, error) {
	return uc.subRepo.FindActiveByUser(ctx, userID)
}

// DeductCredit uses up one credit on the given subscription.
func (uc *SubscriptionUseCase) DeductCredit(ctx context.Context, sub *domain.UserSubscription) (*domain.UserSubscription, error) {
	// Check expiration
	if !sub.Active || time.Now().After(sub.ExpiresAt) {
		return nil, domain.ErrExpiredSubscription
	}
	// Check credits
	if sub.RemainingCredits <= 0 {
		return nil, domain.ErrInsufficientCredits
	}
	// Deduct
	sub.RemainingCredits--

	// Save
	if err := uc.subRepo.Save(ctx, sub); err != nil {
		return nil, err
	}
	return sub, nil
}
