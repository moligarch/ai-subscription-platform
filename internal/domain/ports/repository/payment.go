package repository

import (
	"context"
	"telegram-ai-subscription/internal/domain/model"
	"time"
)

type PaymentRepository interface {
	Save(ctx context.Context, p *model.Payment) error
	Update(ctx context.Context, p *model.Payment) error
	Get(ctx context.Context, id string) (*model.Payment, error)
	GetByAuthority(ctx context.Context, authority string) (*model.Payment, error)
	TotalPaymentsSince(ctx context.Context, since time.Time) (int64, error)
	TotalPaymentsAll(ctx context.Context) (int64, error)
}

type PurchaseRepository interface {
	Save(ctx context.Context, pu *model.Purchase) error
	ListByUser(ctx context.Context, userID string) ([]*model.Purchase, error)
}
