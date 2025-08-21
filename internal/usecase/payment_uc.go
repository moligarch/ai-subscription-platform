package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/repository"
	"telegram-ai-subscription/internal/infra/payment"
)

type PaymentUseCase struct {
	payRepo      repository.PaymentRepository
	purchaseRepo repository.PurchaseRepository
	subUC        SubscriptionUseCase // to grant plan after verification
	gateway      payment.PaymentGateway
	callbackURL  string
	provider     string // "zarinpal"
}

func NewPaymentUseCase(
	payRepo repository.PaymentRepository,
	purchaseRepo repository.PurchaseRepository,
	subUC SubscriptionUseCase,
	gateway payment.PaymentGateway,
	callbackURL string,
) *PaymentUseCase {
	return &PaymentUseCase{
		payRepo:      payRepo,
		purchaseRepo: purchaseRepo,
		subUC:        subUC,
		gateway:      gateway,
		callbackURL:  callbackURL,
		provider:     "zarinpal",
	}
}

var (
	ErrUnsupportedProvider = errors.New("unsupported payment provider")
	ErrPaymentNotFound     = errors.New("payment not found")
	ErrPaymentBadState     = errors.New("payment not in pending state")
)

func (uc *PaymentUseCase) Initiate(ctx context.Context, userID, planID string, amountIRR int64, description string, meta map[string]interface{}) (*model.Payment, string, error) {
	if uc.provider != "zarinpal" {
		return nil, "", ErrUnsupportedProvider
	}

	now := time.Now()
	p := &model.Payment{
		ID:          uuid.NewString(),
		UserID:      userID,
		PlanID:      planID,
		Provider:    uc.provider,
		Amount:      amountIRR,
		Currency:    "IRR",
		Status:      model.PaymentStatusInitiated,
		CreatedAt:   now,
		UpdatedAt:   now,
		Callback:    uc.callbackURL,
		Description: description,
		Meta:        meta,
	}
	if err := uc.payRepo.Save(ctx, p); err != nil {
		return nil, "", err
	}

	authority, payURL, err := uc.gateway.Request(ctx, amountIRR, uc.callbackURL, description, meta)
	if err != nil {
		return nil, "", err
	}
	p.Authority = authority
	p.Status = model.PaymentStatusPending
	p.UpdatedAt = time.Now().UTC()
	if err := uc.payRepo.Update(ctx, p); err != nil {
		return nil, "", err
	}

	// Return payURL so bot can show the link.
	return p, payURL, nil
}

// Confirm is invoked after the user returns from the gateway with Authority, or by admin webhook/command.
func (uc *PaymentUseCase) Confirm(ctx context.Context, authority string, expectedAmount int64) (*model.Payment, error) {
	p, err := uc.payRepo.GetByAuthority(ctx, authority)
	if err != nil || p == nil {
		return nil, ErrPaymentNotFound
	}
	if p.Status != model.PaymentStatusPending {
		return nil, ErrPaymentBadState
	}
	// Verify on gateway
	refID, ok, err := uc.gateway.Verify(ctx, expectedAmount, authority)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if !ok {
		p.Status = model.PaymentStatusFailed
		p.UpdatedAt = now
		if err := uc.payRepo.Update(ctx, p); err != nil {
			return nil, err
		}
		return p, nil
	}

	// Success: update payment
	p.Status = model.PaymentStatusSucceeded
	p.RefID = refID
	p.PaidAt = &now
	p.UpdatedAt = now
	if err := uc.payRepo.Update(ctx, p); err != nil {
		return nil, err
	}

	// Grant subscription (active or reserved based on your existing logic)
	sub, err := uc.subUC.Subscribe(ctx, p.UserID, p.PlanID)
	if err != nil {
		// If subscription failed, we do NOT roll back payment; but we log/return an error.
		return nil, fmt.Errorf("payment ok but failed to grant plan: %w", err)
	}

	// Record purchase history
	pu := &model.Purchase{
		ID:             uuid.NewString(),
		UserID:         p.UserID,
		PlanID:         p.PlanID,
		PaymentID:      p.ID,
		SubscriptionID: sub.ID,
		CreatedAt:      time.Now().UTC(),
	}
	if err := uc.purchaseRepo.Save(ctx, pu); err != nil {
		// Not fatal for user flow, but we surface it so you can fix.
		return p, fmt.Errorf("purchase history save failed: %w", err)
	}

	// Attach subscription to payment for convenience
	p.SubscriptionID = &sub.ID
	p.UpdatedAt = time.Now().UTC()
	_ = uc.payRepo.Update(ctx, p) // best-effort; ignore error

	return p, nil
}

func (uc *PaymentUseCase) TotalPayments(ctx context.Context, period string) (int64, error) {
	now := time.Now().UTC()
	switch period {
	case "week":
		return uc.payRepo.TotalPaymentsSince(ctx, now.Add(-7*24*time.Hour))
	case "month":
		return uc.payRepo.TotalPaymentsSince(ctx, now.Add(-30*24*time.Hour))
	case "year":
		return uc.payRepo.TotalPaymentsSince(ctx, now.AddDate(-1, 0, 0))
	case "all":
		return uc.payRepo.TotalPaymentsAll(ctx)
	default:
		return uc.payRepo.TotalPaymentsAll(ctx)
	}
}

func (uc *PaymentUseCase) GetByAuthority(ctx context.Context, authority string) (*model.Payment, error) {
	payment, err := uc.payRepo.GetByAuthority(ctx, authority)
	if err != nil {
		return nil, fmt.Errorf("failed to get payment by authority %s: %w", authority, err)
	}
	if payment == nil {
		return nil, ErrPaymentNotFound
	}
	return payment, nil
}
