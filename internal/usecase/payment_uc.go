// File: internal/usecase/payment_uc.go
package usecase

import (
	"context"
	"time"

	"github.com/google/uuid"

	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/adapter"
	"telegram-ai-subscription/internal/domain/ports/repository"
)

// Compile-time check
var _ PaymentUseCase = (*paymentUC)(nil)

type PaymentUseCase interface {
	// Initiate returns the created payment and a redirect URL to the provider.
	Initiate(ctx context.Context, userID, planID, callbackURL, description string, meta map[string]interface{}) (*model.Payment, string, error)
	// Confirm verifies a payment given provider authority and expected amount.
	Confirm(ctx context.Context, authority string, expectedAmount int64) (*model.Payment, error)
	// ConfirmAuto looks up the payment by authority to determine expected amount automatically.
	ConfirmAuto(ctx context.Context, authority string) (*model.Payment, error)
	// Totals per period (optional, used by stats/panel)
	SumByPeriod(ctx context.Context, qx any, period string) (int64, error)
}

type paymentUC struct {
	payments repository.PaymentRepository
	plans    repository.SubscriptionPlanRepository
	gateway  adapter.PaymentGateway
}

func NewPaymentUseCase(payments repository.PaymentRepository, plans repository.SubscriptionPlanRepository, gateway adapter.PaymentGateway) *paymentUC {
	return &paymentUC{payments: payments, plans: plans, gateway: gateway}
}

func (u *paymentUC) Initiate(ctx context.Context, userID, planID, callbackURL, description string, meta map[string]interface{}) (*model.Payment, string, error) {
	plan, err := u.plans.FindByID(ctx, planID)
	if err != nil {
		return nil, "", err
	}

	// Request payment with provider
	authority, payURL, err := u.gateway.RequestPayment(ctx, plan.PriceIRR, description, callbackURL, meta)
	if err != nil {
		return nil, "", err
	}

	now := time.Now()
	p := &model.Payment{
		ID:          uuid.NewString(),
		UserID:      userID,
		PlanID:      planID,
		Provider:    u.gateway.Name(),
		Amount:      plan.PriceIRR,
		Currency:    "IRR",
		Authority:   authority,
		Status:      model.PaymentStatusPending,
		CreatedAt:   now,
		UpdatedAt:   now,
		Callback:    callbackURL,
		Description: description,
		Meta:        meta,
	}
	if err := u.payments.Save(ctx, nil, p); err != nil {
		return nil, "", err
	}
	return p, payURL, nil
}

func (u *paymentUC) Confirm(ctx context.Context, authority string, expectedAmount int64) (*model.Payment, error) {
	p, err := u.payments.FindByAuthority(ctx, nil, authority)
	if err != nil {
		return nil, err
	}
	refID, verifyErr := u.gateway.VerifyPayment(ctx, authority, expectedAmount)
	now := time.Now()
	if verifyErr != nil {
		_ = u.payments.UpdateStatus(ctx, nil, p.ID, string(model.PaymentStatusFailed), nil, nil)
		p.Status = model.PaymentStatusFailed
		p.UpdatedAt = now
		return p, verifyErr
	}
	// success
	_ = u.payments.UpdateStatus(ctx, nil, p.ID, string(model.PaymentStatusSucceeded), &refID, &now)
	p.Status = model.PaymentStatusSucceeded
	p.RefID = &refID
	p.PaidAt = &now
	p.UpdatedAt = now
	return p, nil
}

func (u *paymentUC) ConfirmAuto(ctx context.Context, authority string) (*model.Payment, error) {
	p, err := u.payments.FindByAuthority(ctx, nil, authority)
	if err != nil {
		return nil, err
	}
	return u.Confirm(ctx, authority, p.Amount)
}

func (u *paymentUC) SumByPeriod(ctx context.Context, qx any, period string) (int64, error) {
	return u.payments.SumByPeriod(ctx, qx, period)
}
