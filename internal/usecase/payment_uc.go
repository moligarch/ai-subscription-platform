package usecase

import (
	"context"
	"errors"
	"time"

	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/adapter"
	"telegram-ai-subscription/internal/domain/ports/repository"

	"github.com/google/uuid"
)

// PaymentUseCase defines payment orchestration at the application layer.
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

// Compile-time check
var _ PaymentUseCase = (*paymentUC)(nil)

type paymentUC struct {
	payments  repository.PaymentRepository
	plans     repository.SubscriptionPlanRepository
	subs      SubscriptionUseCase
	purchases repository.PurchaseRepository
	gateway   adapter.PaymentGateway
}

func NewPaymentUseCase(payments repository.PaymentRepository, plans repository.SubscriptionPlanRepository, subs SubscriptionUseCase, purchases repository.PurchaseRepository, gateway adapter.PaymentGateway) PaymentUseCase {
	return &paymentUC{
		payments:  payments,
		plans:     plans,
		subs:      subs,
		purchases: purchases,
		gateway:   gateway,
	}
}

// Initiate returns the created payment and a StartPay URL.
func (u *paymentUC) Initiate(ctx context.Context, userID, planID, callbackURL, description string, meta map[string]interface{}) (*model.Payment, string, error) {
	plan, err := u.plans.FindByID(ctx, planID)
	if err != nil {
		return nil, "", err
	}
	p := &model.Payment{
		ID:          uuid.NewString(), // helper that returns string UUID; assumed present in your codebase
		UserID:      userID,
		PlanID:      planID,
		Provider:    u.gateway.Name(),
		Amount:      plan.PriceIRR,
		Currency:    "IRR",
		Status:      model.PaymentStatusPending,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Callback:    callbackURL,
		Description: description,
		Meta:        meta,
	}
	if err := u.payments.Save(ctx, nil, p); err != nil {
		return nil, "", err
	}
	// ask gateway for authority + StartPay link
	auth, startURL, err := u.gateway.RequestPayment(ctx, p.Amount, description, callbackURL, meta)
	if err != nil {
		return nil, "", err
	}
	p.Authority = auth
	p.UpdatedAt = time.Now()
	if err := u.payments.Save(ctx, nil, p); err != nil {
		return nil, "", err
	}
	return p, startURL, nil
}

// Confirm verifies payment and (on success) grants a subscription.
func (u *paymentUC) Confirm(ctx context.Context, authority string, expectedAmount int64) (*model.Payment, error) {
	p, err := u.payments.FindByAuthority(ctx, nil, authority)
	if err != nil {
		return nil, err
	}
	// Idempotency: already succeeded and subscription granted
	if p.Status == model.PaymentStatusSucceeded && p.SubscriptionID != nil {
		return p, nil
	}
	// Verify with gateway
	ref, err := u.gateway.VerifyPayment(ctx, authority, expectedAmount)
	if err != nil {
		// mark failed for visibility
		_ = u.payments.UpdateStatus(ctx, nil, p.ID, model.PaymentStatusFailed, nil, nil)
		return nil, err
	}
	// mark success
	now := time.Now()
	p.RefID = &ref
	p.Status = model.PaymentStatusSucceeded
	p.PaidAt = &now
	p.UpdatedAt = now
	if err := u.payments.UpdateStatus(ctx, nil, p.ID, model.PaymentStatusSucceeded, &ref, &now); err != nil {
		return nil, err
	}
	// Grant subscription (activate or reserve per subUC policy)
	sub, err := u.subs.Subscribe(ctx, p.UserID, p.PlanID)
	if err != nil {
		return nil, err
	}
	// Link payment -> subscription (best-effort; Save upserts subscription_id)
	p.SubscriptionID = &sub.ID
	p.UpdatedAt = time.Now()
	_ = u.payments.Save(ctx, nil, p)
	return p, nil
}

// ConfirmAuto looks up expected amount and calls Confirm.
func (u *paymentUC) ConfirmAuto(ctx context.Context, authority string) (*model.Payment, error) {
	p, err := u.payments.FindByAuthority(ctx, nil, authority)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, errors.New("payment not found")
	}
	return u.Confirm(ctx, authority, p.Amount)
}

// SumByPeriod delegates to repo.
func (u *paymentUC) SumByPeriod(ctx context.Context, qx any, period string) (int64, error) {
	return u.payments.SumByPeriod(ctx, qx, period)
}
