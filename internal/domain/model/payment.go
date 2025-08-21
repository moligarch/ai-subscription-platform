package model

import "time"

type PaymentStatus string

const (
	PaymentStatusInitiated PaymentStatus = "initiated" // we created a payment request on provider side
	PaymentStatusPending   PaymentStatus = "pending"   // redirected to gateway; awaiting verification
	PaymentStatusSucceeded PaymentStatus = "succeeded" // verified OK at provider
	PaymentStatusFailed    PaymentStatus = "failed"    // verification failed or explicitly failed
	PaymentStatusCancelled PaymentStatus = "cancelled" // admin/user cancel
)

// Payment records the external payment intent/transaction.
type Payment struct {
	ID          string        // UUID
	UserID      string        // UUID (your internal user ID)
	PlanID      string        // UUID (which plan the user intends to buy)
	Provider    string        // e.g. "zarinpal"
	Amount      int64         // stored in Rials (integer), to avoid float errors
	Currency    string        // ISO-ish code; for zarinpal typically "IRR"
	Authority   string        // provider "authority" (token) returned by payment request
	RefID       string        // provider reference id after verification (if success)
	Status      PaymentStatus // see constants above
	CreatedAt   time.Time
	UpdatedAt   time.Time
	PaidAt      *time.Time             // set when succeeded
	Callback    string                 // the callback URL we used for provider
	Description string                 // human-readable description shown to gateway
	Meta        map[string]interface{} // optional extra metadata (serialized in DB as JSONB)
	// Link to created subscription (optional; set after we grant subscription):
	SubscriptionID *string
}

// Purchase represents “what plan the user had before/now” as a historical trail.
// This is separate from Payment (money) and Subscription (entitlement).
type Purchase struct {
	ID             string // UUID
	UserID         string // UUID
	PlanID         string // UUID
	PaymentID      string // UUID -> Payment
	SubscriptionID string // UUID -> UserSubscription created/updated
	CreatedAt      time.Time
}
