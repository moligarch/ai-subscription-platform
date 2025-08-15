package model

import (
	"time"

	"telegram-ai-subscription/internal/domain"
)

// UserSubscription represents a user’s individual subscription instance.
type UserSubscription struct {
	ID               string
	UserID           string
	PlanID           string
	StartAt          time.Time
	ExpiresAt        time.Time
	RemainingCredits int
	Active           bool
	CreatedAt        time.Time
}

// NewUserSubscription creates a new subscription for a user.
func NewUserSubscription(id, userID string, plan *SubscriptionPlan) (*UserSubscription, error) {
	if id == "" || userID == "" || plan == nil {
		return nil, domain.ErrInvalidArgument
	}
	now := time.Now()
	return &UserSubscription{
		ID:               id,
		UserID:           userID,
		PlanID:           plan.ID,
		StartAt:          now,
		ExpiresAt:        now.Add(time.Duration(plan.DurationDays) * 24 * time.Hour),
		RemainingCredits: plan.Credits,
		Active:           true,
		CreatedAt:        now,
	}, nil
}

// UseCredit deducts one credit, returns updated copy or error.
func (us *UserSubscription) UseCredit() (*UserSubscription, error) {
	if !us.Active || time.Now().After(us.ExpiresAt) {
		return nil, domain.ErrExpiredSubscription
	}
	if us.RemainingCredits <= 0 {
		return nil, domain.ErrInsufficientCredits
	}
	copy := *us
	copy.RemainingCredits--
	return &copy, nil
}

// Extend renews the subscription from its expiry or from now if expired.
func (us *UserSubscription) Extend(plan *SubscriptionPlan) (*UserSubscription, error) {
	if plan == nil {
		return nil, domain.ErrInvalidArgument
	}
	copy := *us
	start := us.ExpiresAt
	if time.Now().After(us.ExpiresAt) {
		start = time.Now()
	}
	copy.ExpiresAt = start.Add(time.Duration(plan.DurationDays) * 24 * time.Hour)
	copy.RemainingCredits += plan.Credits
	copy.Active = true
	return &copy, nil
}
