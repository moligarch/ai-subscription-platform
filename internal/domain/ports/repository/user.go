package repository

import (
	"context"
	"telegram-ai-subscription/internal/domain/model"
	"time"
)

// -----------------------------
// Users
// -----------------------------

type UserRepository interface {
	Save(ctx context.Context, tx Tx, u *model.User) error
	FindByTelegramID(ctx context.Context, tx Tx, tgID int64) (*model.User, error)
	FindByID(ctx context.Context, tx Tx, id string) (*model.User, error)
	CountUsers(ctx context.Context, tx Tx) (int, error)
	CountInactiveUsers(ctx context.Context, tx Tx, since time.Time) (int, error)
}
