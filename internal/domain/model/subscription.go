package model

import (
	"time"

	"telegram-ai-subscription/internal/domain"
)

type SubscriptionStatus string

const (
	SubscriptionStatusNone      SubscriptionStatus = "none"
	SubscriptionStatusReserved  SubscriptionStatus = "reserved"
	SubscriptionStatusActive    SubscriptionStatus = "active"
	SubscriptionStatusFinished  SubscriptionStatus = "finished"
	SubscriptionStatusCancelled SubscriptionStatus = "cancelled"
)

// UserSubscription represents a user’s individual subscription instance.

type UserSubscription struct {
	ID               string // UUID
	UserID           string // UUID of user
	PlanID           string // UUID of plan
	CreatedAt        time.Time
	ScheduledStartAt *time.Time // nil if should start immediately
	StartAt          *time.Time // nil until active
	ExpiresAt        *time.Time // nil until scheduled/started
	RemainingCredits int
	Status           SubscriptionStatus
}

// NewUserSubscription creates a new subscription for a user.
func NewUserSubscription(id, userID string, plan *SubscriptionPlan) (*UserSubscription, error) {
	if id == "" || userID == "" || plan == nil {
		return nil, domain.ErrInvalidArgument
	}
	now := time.Now()
	expire := now.Add(time.Duration(plan.DurationDays) * 24 * time.Hour)
	return &UserSubscription{
		ID:               id,
		UserID:           userID,
		PlanID:           plan.ID,
		StartAt:          &now,
		ExpiresAt:        &expire,
		RemainingCredits: plan.Credits,
		Status:           SubscriptionStatusActive,
		CreatedAt:        now,
	}, nil
}

// // UseCredit deducts one credit, returns updated copy or error.
// func (us *UserSubscription) UseCredit() (*UserSubscription, error) {
// 	if us.Status != SubscriptionStatusActive || time.Now().After(*us.ExpiresAt) {
// 		return nil, domain.ErrExpiredSubscription
// 	}
// 	if us.RemainingCredits <= 0 {
// 		return nil, domain.ErrInsufficientCredits
// 	}
// 	copy := *us
// 	copy.RemainingCredits--
// 	return &copy, nil
// }

// // Extend renews the subscription from its expiry or from now if expired.
// func (us *UserSubscription) Extend(plan *SubscriptionPlan) (*UserSubscription, error) {
// 	if plan == nil {
// 		return nil, domain.ErrInvalidArgument
// 	}
// 	copy := *us
// 	start := us.ExpiresAt
// 	if time.Now().After(*us.ExpiresAt) {
// 		*start = time.Now()
// 	}
// 	expire := start.Add(time.Duration(plan.DurationDays) * 24 * time.Hour)
// 	copy.ExpiresAt = &expire
// 	copy.RemainingCredits += plan.Credits
// 	copy.Status = SubscriptionStatusActive
// 	return &copy, nil
// }
