package usecase

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
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

	FindUserIDByAuthority(ctx context.Context, authority string) (string, error)
}

// Compile-time check
var _ PaymentUseCase = (*paymentUC)(nil)

type paymentUC struct {
	payments  repository.PaymentRepository
	plans     repository.SubscriptionPlanRepository
	subs      SubscriptionUseCase
	purchases repository.PurchaseRepository
	gateway   adapter.PaymentGateway
	tm        repository.TransactionManager

	log *zerolog.Logger
}

func NewPaymentUseCase(
	payments repository.PaymentRepository,
	plans repository.SubscriptionPlanRepository,
	subs SubscriptionUseCase,
	purchases repository.PurchaseRepository,
	gateway adapter.PaymentGateway,
	tm repository.TransactionManager,
	logger *zerolog.Logger,
) PaymentUseCase {
	return &paymentUC{
		payments:  payments,
		plans:     plans,
		subs:      subs,
		purchases: purchases,
		gateway:   gateway,
		tm:        tm,
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
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, "", domain.ErrPlanNotFound
		}
		return nil, "", err // Propagate other unexpected errors
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

// The original `Confirm` function is now deprecated by the safer `ConfirmAuto`.
// If you still need it, it should be refactored to also use the transaction manager.
func (u *paymentUC) Confirm(ctx context.Context, authority string, expectedAmount int64) (*model.Payment, error) {
	// For now, we can just log a warning and call the main transactional function.
	// In a real scenario, you might want a more complex transactional wrapper here as well.
	u.log.Warn().Msg("Confirm is called, prefer using ConfirmAuto")
	return u.ConfirmAuto(ctx, authority)
}

// ConfirmAuto now wraps the core logic in a transaction.
func (u *paymentUC) ConfirmAuto(ctx context.Context, authority string) (p *model.Payment, err error) {
	if authority == "" {
		return nil, domain.ErrInvalidArgument
	}

	// The entire confirmation flow is now wrapped in a transaction.
	// If any step inside this function returns an error, all database
	// changes will be automatically rolled back.
	err = u.tm.WithTx(ctx, pgx.TxOptions{}, func(ctx context.Context, tx repository.Tx) error {
		// Look up payment to discover amount/plan
		payment, err := u.payments.FindByAuthority(ctx, tx, authority)
		if err != nil {
			return domain.ErrNotFound
		}

		// Re-check fast path
		if payment.Status == model.PaymentStatusSucceeded && payment.SubscriptionID != nil {
			p = payment
			return nil // Already processed, exit transaction successfully
		}

		plan, err := u.plans.FindByID(ctx, tx, payment.PlanID)
		if err != nil {
			return domain.ErrNotFound
		}

		// Core confirmation logic
		confirmedPayment, err := u.confirmPaymentInTx(ctx, tx, payment, int64(plan.PriceIRR))
		if err != nil {
			return err // Propagate error to trigger rollback
		}
		p = confirmedPayment
		return nil
	})

	return p, err
}

func (u *paymentUC) SumByPeriod(ctx context.Context, tx repository.Tx, period string) (int64, error) {
	return u.payments.SumByPeriod(ctx, tx, period)
}

// confirmPaymentInTx contains the actual logic that needs to be atomic.
// It is now a private method that requires a transaction handle `tx`.
func (u *paymentUC) confirmPaymentInTx(ctx context.Context, tx repository.Tx, p *model.Payment, expectedAmount int64) (*model.Payment, error) {
	// Verify with provider
	ref, err := u.gateway.VerifyPayment(ctx, p.Authority, expectedAmount)
	if err != nil {
		// Mark failed best-effort. The transaction will be rolled back anyway,
		// but this call ensures we update the status if the provider fails verification.
		_ = u.payments.UpdateStatus(ctx, tx, p.ID, model.PaymentStatusFailed, nil, nil)
		metrics.IncPayment("failed")
		return nil, err
	}

	now := time.Now()
	// Idempotent success transition (only one caller wins)
	// Pass the `tx` handle to the repository method.
	updated, err := u.payments.UpdateStatusIfPending(ctx, tx, p.ID, model.PaymentStatusSucceeded, &ref, &now)
	if err != nil {
		return nil, err
	}
	if !updated {
		// Someone else finalized it; re-read and return success.
		// Note: We MUST re-read within the same transaction to get the latest locked row.
		return u.payments.FindByID(ctx, tx, p.ID)
	}

	// Reflect local copy
	p.Status = model.PaymentStatusSucceeded
	p.RefID = &ref
	p.PaidAt = &now
	p.UpdatedAt = now

	// Grant subscription (pass `tx` down if SubscriptionUseCase methods are transactional)
	sub, err := u.subs.Subscribe(ctx, p.UserID, p.PlanID)
	if err != nil {
		return nil, err
	}
	// Link payment -> subscription
	p.SubscriptionID = &sub.ID
	p.UpdatedAt = time.Now()
	if err := u.payments.Save(ctx, tx, p); err != nil {
		return nil, err
	}

	// Append purchase record
	pu := &model.Purchase{
		ID:             uuid.NewString(),
		UserID:         p.UserID,
		PlanID:         p.PlanID,
		PaymentID:      p.ID,
		SubscriptionID: sub.ID,
		CreatedAt:      time.Now(),
	}
	if err := u.purchases.Save(ctx, tx, pu); err != nil {
		return nil, err
	}

	metrics.IncPayment("succeeded")
	metrics.AddPaymentRevenue(p.Currency, p.Amount)
	return p, nil
}

func (p *paymentUC) FindUserIDByAuthority(ctx context.Context, authority string) (string, error) {
	a := strings.TrimSpace(authority)
	if a == "" {
		return "", domain.ErrInvalidArgument
	}
	// Best source of truth is the purchase created before redirect (authority is stored there)
	pur, err := p.payments.FindByAuthority(ctx, repository.NoTX, a)
	if err != nil {
		return "", err
	}
	if pur == nil || pur.UserID == "" {
		return "", domain.ErrNotFound
	}
	return pur.UserID, nil
}
