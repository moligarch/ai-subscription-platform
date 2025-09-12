package model

import (
	"time"
)

// ActivationCode represents a single-use code that can be redeemed for a subscription plan.
type ActivationCode struct {
	ID               string
	Code             string
	PlanID           string
	IsRedeemed       bool
	RedeemedByUserID *string    // Pointer to allow for NULL
	RedeemedAt       *time.Time // Pointer to allow for NULL
	CreatedAt        time.Time
	ExpiresAt        *time.Time // Pointer to allow for NULL
}
