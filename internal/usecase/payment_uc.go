// File: internal/usecase/payment_uc.go
package usecase

import (
	"context"
	"fmt"
	"time"

	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/repository"
)

// PaymentUseCase orchestrates payment flows.
type PaymentUseCase struct {
	payRepo repository.PaymentRepository
	// gateway    domain.PaymentGateway
	subUC *SubscriptionUseCase
}

// NewPaymentUseCase constructs PaymentUseCase.
func NewPaymentUseCase(
	payRepo repository.PaymentRepository,
	// gateway domain.PaymentGateway,
	subUC *SubscriptionUseCase,
) *PaymentUseCase {
	return &PaymentUseCase{
		payRepo: payRepo,
		// gateway: gateway,
		subUC: subUC,
	}
}

// Initiate creates a Payment and returns the payment entity.
func (p *PaymentUseCase) Initiate(ctx context.Context, userID, method string, amount float64) (*domain.Payment, error) {
	payID := domain.NewUUID()
	pay, err := domain.NewPayment(payID, userID, method, amount)
	if err != nil {
		return nil, err
	}
	if err := p.payRepo.Save(ctx, pay); err != nil {
		return nil, err
	}
	return pay, nil
}

// Confirm handles the gateway callback, updates payment & subscription.
func (p *PaymentUseCase) Confirm(ctx context.Context, payID string, success bool) (*domain.Payment, error) {
	existing, err := p.payRepo.FindByID(ctx, payID)
	if err != nil {
		return nil, err
	}
	var updated *domain.Payment
	if success {
		updated = existing.MarkSuccess()
		// upon success, subscribe user with chosen plan/amount
		// here we assume Method stores plan ID or similar mapping
		if _, err := p.subUC.Subscribe(ctx, existing.UserID, existing.Method); err != nil {
			return nil, err
		}
	} else {
		updated = existing.MarkFailed()
	}
	if err := p.payRepo.Save(ctx, updated); err != nil {
		return nil, err
	}
	return updated, nil
}

// TotalPayments returns total payments for given time period ("week", "month", "year") in Toman.
func (uc *PaymentUseCase) TotalPayments(ctx context.Context, period string) (float64, error) {
	now := time.Now()
	var since time.Time

	switch period {
	case "week":
		since = now.Add(-7 * 24 * time.Hour)
	case "month":
		since = now.Add(-30 * 24 * time.Hour)
	case "year":
		since = now.Add(-365 * 24 * time.Hour)
	default:
		return 0, fmt.Errorf("unknown period %q", period)
	}

	sum, err := uc.payRepo.TotalPaymentsInPeriod(ctx, since, now.Add(time.Second))
	if err != nil {
		return 0, fmt.Errorf("total payments for %s: %w", period, err)
	}
	return sum, nil
}
