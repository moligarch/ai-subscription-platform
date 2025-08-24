package repository

import (
	"context"
	"telegram-ai-subscription/internal/domain/model"
	"time"
)

// -----------------------------
// Payments
// -----------------------------

type PaymentRepository interface {
	Save(ctx context.Context, qx any, p *model.Payment) error
	FindByID(ctx context.Context, qx any, id string) (*model.Payment, error)
	FindByAuthority(ctx context.Context, qx any, authority string) (*model.Payment, error)
	UpdateStatus(ctx context.Context, qx any, id string, status string, refID *string, paidAt *time.Time) error
	SumByPeriod(ctx context.Context, qx any, period string) (int64, error)
	// Activation code helpers for manual post-payment activation flow
	SetActivationCode(ctx context.Context, qx any, paymentID string, code string, expiresAt time.Time) error
	FindByActivationCode(ctx context.Context, qx any, code string) (*model.Payment, error)
}

// -----------------------------
// Purchases
// -----------------------------

type PurchaseRepository interface {
	Save(ctx context.Context, qx any, pu *model.Purchase) error
	ListByUser(ctx context.Context, qx any, userID string) ([]*model.Purchase, error)
}
