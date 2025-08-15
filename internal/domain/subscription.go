package domain

import "time"

// SubscriptionPlan defines the parameters of a subscription.
type SubscriptionPlan struct {
	ID           string
	Name         string
	DurationDays int
	Credits      int
	CreatedAt    time.Time
}

// NewSubscriptionPlan validates and constructs a plan.
func NewSubscriptionPlan(id, name string, durationDays, credits int) (*SubscriptionPlan, error) {
	if id == "" || name == "" || durationDays <= 0 || credits < 0 {
		return nil, ErrInvalidArgument
	}
	return &SubscriptionPlan{
		ID:           id,
		Name:         name,
		DurationDays: durationDays,
		Credits:      credits,
		CreatedAt:    time.Now(),
	}, nil
}

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
		return nil, ErrInvalidArgument
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
		return nil, ErrExpiredSubscription
	}
	if us.RemainingCredits <= 0 {
		return nil, ErrInsufficientCredits
	}
	copy := *us
	copy.RemainingCredits--
	return &copy, nil
}

// Extend renews the subscription from its expiry or from now if expired.
func (us *UserSubscription) Extend(plan *SubscriptionPlan) (*UserSubscription, error) {
	if plan == nil {
		return nil, ErrInvalidArgument
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
