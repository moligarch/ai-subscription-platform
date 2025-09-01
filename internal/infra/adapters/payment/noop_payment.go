package payment

import (
	"context"
	"fmt"
	"sync"
	"time"

	"telegram-ai-subscription/internal/domain/ports/adapter"
)

var _ adapter.PaymentGateway = (*NoopPaymentGateway)(nil)

// NoopPaymentGateway is a simple in-memory gateway to use in tests.
type NoopPaymentGateway struct {
	mu      sync.Mutex
	seq     int64
	intents map[string]int64 // authority -> expected amount (IRR)
}

func NewNoopPaymentGateway() *NoopPaymentGateway {
	return &NoopPaymentGateway{
		intents: make(map[string]int64),
	}
}

func (g *NoopPaymentGateway) Name() string { return "noop" }

func (g *NoopPaymentGateway) next() string {
	g.seq++
	return fmt.Sprintf("noop-%d", g.seq)
}

func (g *NoopPaymentGateway) RequestPayment(ctx context.Context, amount int64, description, callbackURL string, meta map[string]interface{}) (authority string, payURL string, err error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	authority = g.next()
	g.intents[authority] = amount
	return authority, "https://example.test/pay/" + authority, nil
}

func (g *NoopPaymentGateway) VerifyPayment(ctx context.Context, authority string, expectedAmount int64) (refID string, err error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	exp, ok := g.intents[authority]
	if !ok {
		return "", fmt.Errorf("noop: authority not found")
	}
	if exp != expectedAmount {
		return "", fmt.Errorf("noop: amount mismatch: expected %d got %d", exp, expectedAmount)
	}
	return "ref-" + authority, nil
}

func (g *NoopPaymentGateway) RefundPayment(ctx context.Context, sessionID string, amount int64, description string, method adapter.RefundMethod, reason adapter.RefundReason) (adapter.RefundResult, error) {
	return adapter.RefundResult{
		ID:           "refund-" + sessionID,
		Status:       "DONE",
		RefundAmount: amount,
		RefundTime:   time.Now(),
	}, nil
}
