// File: internal/domain/ports/adapter/payment.go
package adapter

import "context"

// PaymentGateway is the domain port for payment providers (e.g., ZarinPal).
// Concrete implementation: internal/infra/adapters/payment/*
// This API is intentionally minimal for portability and testability.
type PaymentGateway interface {
	// Name returns a short identifier for the gateway (e.g., "zarinpal").
	Name() string
	// RequestPayment creates a payment on the provider side and returns
	// an authority code and a redirect URL for the user.
	RequestPayment(ctx context.Context, amountIRR int64, description string, callbackURL string, meta map[string]interface{}) (authority string, payURL string, err error)
	// VerifyPayment verifies that a payment was completed successfully on the provider
	// given the authority code and expected amount. It returns the provider reference id.
	VerifyPayment(ctx context.Context, authority string, expectedAmount int64) (refID string, err error)
}
