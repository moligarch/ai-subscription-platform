package adapter

import (
	"context"
	"time"
)

// --- Refund types (ZarinPal-compatible) ---

type RefundMethod string

const (
	RefundMethodPaya RefundMethod = "PAYA" // scheduled via PAYA
	RefundMethodCard RefundMethod = "CARD" // instant to card
)

type RefundReason string

const (
	RefundReasonCustomerRequest RefundReason = "CUSTOMER_REQUEST"
	RefundReasonDuplicate       RefundReason = "DUPLICATE_TRANSACTION"
	RefundReasonSuspicious      RefundReason = "SUSPICIOUS_TRANSACTION"
	RefundReasonOther           RefundReason = "OTHER"
)

// RefundResult captures a minimal, provider-agnostic result of a refund request.
type RefundResult struct {
	ID           string    // provider refund/transaction id
	Status       string    // provider status e.g. PENDING / DONE
	RefundAmount int64     // in minor units (IRR)
	RefundTime   time.Time // provider timestamp if available
}

// PaymentGateway is the hex port for payment providers.
type PaymentGateway interface {
	Name() string

	// RequestPayment initiates a payment intent and returns provider authority and a redirect URL.
	RequestPayment(ctx context.Context, amount int64, description, callbackURL string, meta map[string]interface{}) (authority string, payURL string, err error)
	// VerifyPayment verifies a payment given the authority and expected amount; returns provider refID on success.
	VerifyPayment(ctx context.Context, authority string, expectedAmount int64) (refID string, err error)

	// RefundPayment issues a refund for a captured/settled transaction.
	// ZarinPal requires a session_id (their transaction id), amount, a description, a refund method (CARD|PAYA), and a reason code.
	RefundPayment(ctx context.Context, sessionID string, amount int64, description string, method RefundMethod, reason RefundReason) (RefundResult, error)
}
