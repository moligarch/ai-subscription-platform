package model

import (
	"time"

	"telegram-ai-subscription/internal/domain"
)

// SubscriptionPlan represents a purchasable plan with a fixed duration,
// credit allotment, and price in IRR.
type SubscriptionPlan struct {
	ID           string
	Name         string
	DurationDays int
	Credits      int
	PriceIRR     int64
	CreatedAt    time.Time
}

func (p *SubscriptionPlan) IsZero() bool { return p == nil || p.ID == "" }

// NewSubscriptionPlan validates and constructs a plan.
func NewSubscriptionPlan(id, name string, durationDays, credits int, priceIRR int64) (*SubscriptionPlan, error) {
	if id == "" || name == "" || durationDays <= 0 || credits < 0 || priceIRR <= 0 {
		return nil, domain.ErrInvalidArgument
	}
	return &SubscriptionPlan{
		ID:           id,
		Name:         name,
		DurationDays: durationDays,
		Credits:      credits,
		PriceIRR:     priceIRR,
		CreatedAt:    time.Now(),
	}, nil
}
