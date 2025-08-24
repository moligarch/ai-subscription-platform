// File: internal/usecase/subscription_uc.go
package usecase

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/repository"
)

// Compile-time check
var _ SubscriptionUseCase = (*subscriptionUC)(nil)

type SubscriptionUseCase interface {
	Subscribe(ctx context.Context, userID, planID string) (*model.UserSubscription, error)
	GetActive(ctx context.Context, userID string) (*model.UserSubscription, error)
	GetReserved(ctx context.Context, userID string) ([]*model.UserSubscription, error)
	DeductCredits(ctx context.Context, userID string, amount int) (*model.UserSubscription, error)
	FinishExpired(ctx context.Context) (int, error)
}

type subscriptionUC struct {
	subs  repository.SubscriptionRepository
	plans repository.SubscriptionPlanRepository
}

func NewSubscriptionUseCase(subs repository.SubscriptionRepository, plans repository.SubscriptionPlanRepository) *subscriptionUC {
	return &subscriptionUC{subs: subs, plans: plans}
}

func (u *subscriptionUC) Subscribe(ctx context.Context, userID, planID string) (*model.UserSubscription, error) {
	plan, err := u.plans.FindByID(ctx, planID)
	if err != nil {
		return nil, err
	}

	// Check for active subscription
	active, err := u.subs.FindActiveByUser(ctx, nil, userID)
	if err != nil && err != domain.ErrNotFound {
		return nil, err
	}

	now := time.Now()
	if active == nil {
		// Activate immediately
		s := &model.UserSubscription{
			ID:               uuid.NewString(),
			UserID:           userID,
			PlanID:           plan.ID,
			CreatedAt:        now,
			StartAt:          &now,
			ExpiresAt:        ptrTime(now.AddDate(0, 0, plan.DurationDays)),
			RemainingCredits: plan.Credits,
			Status:           model.SubscriptionStatusActive,
		}
		if err := u.subs.Save(ctx, nil, s); err != nil {
			return nil, err
		}
		return s, nil
	}

	// Otherwise, reserve after the current active
	startAt := *active.ExpiresAt
	s := &model.UserSubscription{
		ID:               uuid.NewString(),
		UserID:           userID,
		PlanID:           plan.ID,
		CreatedAt:        now,
		ScheduledStartAt: &startAt,
		ExpiresAt:        ptrTime(startAt.AddDate(0, 0, plan.DurationDays)),
		RemainingCredits: plan.Credits,
		Status:           model.SubscriptionStatusReserved,
	}
	if err := u.subs.Save(ctx, nil, s); err != nil {
		return nil, err
	}
	return s, nil
}

func (u *subscriptionUC) GetActive(ctx context.Context, userID string) (*model.UserSubscription, error) {
	return u.subs.FindActiveByUser(ctx, nil, userID)
}

func (u *subscriptionUC) GetReserved(ctx context.Context, userID string) ([]*model.UserSubscription, error) {
	return u.subs.FindReservedByUser(ctx, nil, userID)
}

func (u *subscriptionUC) DeductCredits(ctx context.Context, userID string, amount int) (*model.UserSubscription, error) {
	s, err := u.subs.FindActiveByUser(ctx, nil, userID)
	if err != nil {
		// map repo not-found to a typed UC error
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.ErrNoActiveSubscription
		}
		return nil, err
	}
	if amount <= 0 {
		return s, nil
	}
	if s.RemainingCredits > 0 {
		s.RemainingCredits -= amount
		if s.RemainingCredits < 0 {
			s.RemainingCredits = 0
		}
	}
	// If credits exhausted, finish subscription now
	if s.RemainingCredits == 0 {
		now := time.Now()
		s.Status = model.SubscriptionStatusFinished
		s.ExpiresAt = &now
	}
	if err := u.subs.Save(ctx, nil, s); err != nil {
		return nil, err
	}
	return s, nil
}

// FinishExpired transitions any active subscription whose expires_at <= now to finished.
// Returns number of subscriptions updated.
func (u *subscriptionUC) FinishExpired(ctx context.Context) (int, error) {
	expiring, err := u.subs.FindExpiring(ctx, nil, 0)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, s := range expiring {
		if s.Status != model.SubscriptionStatusActive || s.ExpiresAt == nil || s.ExpiresAt.After(time.Now()) {
			continue
		}
		s.Status = model.SubscriptionStatusFinished
		if err := u.subs.Save(ctx, nil, s); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func ptrTime(t time.Time) *time.Time { return &t }
