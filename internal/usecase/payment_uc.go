package usecase

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/adapter"
	"telegram-ai-subscription/internal/domain/ports/repository"
	"telegram-ai-subscription/internal/infra/metrics"
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
	SumByPeriod(ctx context.Context, tx repository.Tx, period string) (int64, error)
}

// Compile-time check
var _ PaymentUseCase = (*paymentUC)(nil)

type paymentUC struct {
	payments  repository.PaymentRepository
	plans     repository.SubscriptionPlanRepository
	subs      SubscriptionUseCase
	purchases repository.PurchaseRepository
	gateway   adapter.PaymentGateway

	log *zerolog.Logger
}

func NewPaymentUseCase(
	payments repository.PaymentRepository,
	plans repository.SubscriptionPlanRepository,
	subs SubscriptionUseCase,
	purchases repository.PurchaseRepository,
	gateway adapter.PaymentGateway,
	logger *zerolog.Logger,
) PaymentUseCase {
	return &paymentUC{
		payments:  payments,
		plans:     plans,
		subs:      subs,
		purchases: purchases,
		gateway:   gateway,
		log:       logger,
	}
}

func (u *paymentUC) Initiate(ctx context.Context, userID, planID, callbackURL, description string, meta map[string]interface{}) (*model.Payment, string, error) {
	if userID == "" || planID == "" {
		return nil, "", domain.ErrInvalidArgument
	}

	if u.subs != nil {
		if reserved, _ := u.subs.GetReserved(ctx, userID); len(reserved) > 0 {
			return nil, "", domain.ErrAlreadyHasReserved
		}
	}

	plan, err := u.plans.FindByID(ctx, repository.NoTX, planID)
	if err != nil || plan == nil {
		return nil, "", domain.ErrNotFound
	}
	amount := plan.PriceIRR

	authority, startURL, err := u.gateway.RequestPayment(ctx, amount, description, callbackURL, meta)
	if err != nil {
		return nil, "", err
	}

	now := time.Now()
	p := &model.Payment{
		ID:          uuid.NewString(),
		UserID:      userID,
		PlanID:      planID,
		Provider:    u.gateway.Name(),
		Amount:      amount,
		Currency:    "IRR",
		Authority:   authority,
		Status:      model.PaymentStatusPending,
		CreatedAt:   now,
		UpdatedAt:   now,
		Callback:    callbackURL,
		Description: description,
		Meta:        map[string]any{},
	}

	if meta != nil {
		p.Meta = meta
	}

	if err := u.payments.Save(ctx, repository.NoTX, p); err != nil {
		return nil, "", err
	}
	metrics.IncPayment("initiated")
	return p, startURL, nil
}

func (u *paymentUC) Confirm(ctx context.Context, authority string, expectedAmount int64) (*model.Payment, error) {
	if authority == "" {
		return nil, errors.New("missing authority")
	}

	p, err := u.payments.FindByAuthority(ctx, repository.NoTX, authority)
	if err != nil || p == nil {
		return nil, errors.New("payment not found")
	}

	// Short circuit if already completed and linked
	if p.Status == model.PaymentStatusSucceeded && p.SubscriptionID != nil {
		return p, nil
	}

	// Verify with provider
	ref, err := u.gateway.VerifyPayment(ctx, authority, expectedAmount)
	if err != nil {
		// mark failed best-effort
		_ = u.payments.UpdateStatus(ctx, repository.NoTX, p.ID, model.PaymentStatusFailed, nil, nil)
		metrics.IncPayment("failed")
		return nil, err
	}

	now := time.Now()
	// Idempotent success transition (only one caller wins)
	updated, err := u.payments.UpdateStatusIfPending(ctx, repository.NoTX, p.ID, model.PaymentStatusSucceeded, &ref, &now)
	if err != nil {
		return nil, err
	}
	if !updated {
		// Someone else finalized; re-read and move on
		p, err = u.payments.FindByID(ctx, repository.NoTX, p.ID)
		if err != nil {
			return nil, err
		}
		// If succeeded and already linked, done
		if p.Status == model.PaymentStatusSucceeded && p.SubscriptionID != nil {
			return p, nil
		}
	} else {
		// Reflect local copy
		p.Status = model.PaymentStatusSucceeded
		p.RefID = &ref
		p.PaidAt = &now
		p.UpdatedAt = now
	}

	// Grant subscription (policy handled inside subs UC)
	sub, err := u.subs.Subscribe(ctx, p.UserID, p.PlanID)
	if err != nil {
		return nil, err
	}
	// Link payment -> subscription
	p.SubscriptionID = &sub.ID
	p.UpdatedAt = time.Now()
	_ = u.payments.Save(ctx, repository.NoTX, p)

	// Append purchase (idempotent if DB has UNIQUE(payment_id))
	pu := &model.Purchase{
		ID:             uuid.NewString(),
		UserID:         p.UserID,
		PlanID:         p.PlanID,
		PaymentID:      p.ID,
		SubscriptionID: sub.ID,
		CreatedAt:      time.Now(),
	}
	_ = u.purchases.Save(ctx, repository.NoTX, pu)
	metrics.IncPayment("succeeded")
	return p, nil
}

func (u *paymentUC) ConfirmAuto(ctx context.Context, authority string) (*model.Payment, error) {
	if authority == "" {
		return nil, errors.New("missing authority")
	}
	// Load payment to discover amount/plan
	p, err := u.payments.FindByAuthority(ctx, repository.NoTX, authority)
	if err != nil || p == nil {
		return nil, errors.New("payment not found")
	}

	// Re-check fast path
	if p.Status == model.PaymentStatusSucceeded && p.SubscriptionID != nil {
		return p, nil
	}

	plan, err := u.plans.FindByID(ctx, repository.NoTX, p.PlanID)
	if err != nil || plan == nil {
		return nil, errors.New("plan not found")
	}
	return u.Confirm(ctx, authority, int64(plan.PriceIRR))
}

func (u *paymentUC) SumByPeriod(ctx context.Context, tx repository.Tx, period string) (int64, error) {
	return u.payments.SumByPeriod(ctx, tx, period)
}
