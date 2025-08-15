package repository

import (
	"context"
	"telegram-ai-subscription/internal/domain"
	"time"
)

// PaymentRepository handles payments persistence/queries.
// Note: amounts use float64 in Toman (matches domain.Payment.Amount).
type PaymentRepository interface {
	// Save persists a payment (insert or update).
	Save(ctx context.Context, p *domain.Payment) error

	// FindByID returns a payment by id or domain.ErrNotFound.
	FindByID(ctx context.Context, id string) (*domain.Payment, error)

	// TotalPaymentsInPeriod returns total paid amount (in Toman) between since (inclusive) and till (exclusive).
	TotalPaymentsInPeriod(ctx context.Context, since, till time.Time) (float64, error)
}
