package repository

import (
	"context"
	"time"

	"telegram-ai-subscription/internal/domain/model"
)

// -----------------------------
// Payments
// -----------------------------

type PaymentRepository interface {
	Save(ctx context.Context, tx Tx, p *model.Payment) error
	FindByID(ctx context.Context, tx Tx, id string) (*model.Payment, error)
	FindByAuthority(ctx context.Context, tx Tx, authority string) (*model.Payment, error)
	UpdateStatus(ctx context.Context, tx Tx, id string, status model.PaymentStatus, refID *string, paidAt *time.Time) error
	SumByPeriod(ctx context.Context, tx Tx, period string) (int64, error)
	// Activation code helpers for manual post-payment activation flow
	SetActivationCode(ctx context.Context, tx Tx, paymentID string, code string, expiresAt time.Time) error
	FindByActivationCode(ctx context.Context, tx Tx, code string) (*model.Payment, error)
	// Reconciliation helper: list pending payments older than cutoff
	ListPendingOlderThan(ctx context.Context, tx Tx, olderThan time.Time, limit int) ([]*model.Payment, error)

	// UpdateStatusIfPending atomically changes status only if current status is 'pending' or 'initiated'.
	// Returns true if a row was updated, false if not (e.g., already processed).
	UpdateStatusIfPending(ctx context.Context, tx Tx, id string, status model.PaymentStatus, refID *string, paidAt *time.Time) (bool, error)
}

// -----------------------------
// Purchases
// -----------------------------

type PurchaseRepository interface {
	Save(ctx context.Context, tx Tx, pu *model.Purchase) error
	ListByUser(ctx context.Context, tx Tx, userID string) ([]*model.Purchase, error)
}
