package model

import (
	"time"

	"telegram-ai-subscription/internal/domain"
)

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
		return nil, domain.ErrInvalidArgument
	}
	return &SubscriptionPlan{
		ID:           id,
		Name:         name,
		DurationDays: durationDays,
		Credits:      credits,
		CreatedAt:    time.Now(),
	}, nil
}
