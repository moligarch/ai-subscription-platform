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
	Save(ctx context.Context, qx any, u *model.User) error
	FindByTelegramID(ctx context.Context, qx any, tgID int64) (*model.User, error)
	FindByID(ctx context.Context, qx any, id string) (*model.User, error)
	CountUsers(ctx context.Context, qx any) (int, error)
	CountInactiveUsers(ctx context.Context, qx any, since time.Time) (int, error)
}
