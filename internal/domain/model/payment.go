package model

import "time"

type PaymentStatus string

const (
	PaymentStatusInitiated PaymentStatus = "initiated" // created a payment request on provider side
	PaymentStatusPending   PaymentStatus = "pending"   // redirected to gateway; awaiting verification
	PaymentStatusSucceeded PaymentStatus = "succeeded" // verified OK at provider
	PaymentStatusFailed    PaymentStatus = "failed"    // verification failed or explicitly failed
	PaymentStatusCancelled PaymentStatus = "cancelled" // admin/user cancel
)

// Payment records the external payment intent/transaction.
type Payment struct {
	ID          string        // UUID
	UserID      string        // UUID -> users.id
	PlanID      string        // UUID -> subscription_plans.id
	Provider    string        // e.g., "zarinpal"
	Amount      int64         // in IRR
	Currency    string        // e.g., "IRR"
	Authority   string        // provider authority code
	RefID       *string       // provider ref id (after verify)
	Status      PaymentStatus // lifecycle status
	CreatedAt   time.Time
	UpdatedAt   time.Time
	PaidAt      *time.Time     // set when succeeded
	Callback    string         // callback URL
	Description string         // human-readable description
	Meta        map[string]any // extra data (JSONB)

	// Link to created subscription (optional; set after we grant subscription):
	SubscriptionID *string

	// Manual post-payment activation support (optional v1 path):
	ActivationCode      *string
	ActivationExpiresAt *time.Time
}

// Purchase represents the historical link between user, plan and payment that
// resulted in a subscription grant.
type Purchase struct {
	ID             string // UUID
	UserID         string // UUID
	PlanID         string // UUID
	PaymentID      string // UUID -> Payment
	SubscriptionID string // UUID -> user_subscriptions.id
	CreatedAt      time.Time
}
