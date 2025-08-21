package payment

import "context"

type PaymentGateway interface {
	Request(ctx context.Context, amountIRR int64, callbackURL, description string, meta map[string]any) (authority string, payURL string, err error)
	Verify(ctx context.Context, amountIRR int64, authority string) (refID string, ok bool, err error)
}
