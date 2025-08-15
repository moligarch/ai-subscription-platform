package repository

import (
	"context"
	"telegram-ai-subscription/internal/domain/model"
	"time"
)

// UserRepository defines thread-safe methods (must support concurrent calls)
type UserRepository interface {
	// Save persists a new or existing user
	Save(ctx context.Context, u *model.User) error
	// FindByTelegramID returns ErrNotFound if missing
	FindByTelegramID(ctx context.Context, tgID int64) (*model.User, error)
	// FindByID looks up a domain user by internal ID (UUID string). Returns ErrNotFound if missing.
	FindByID(ctx context.Context, id string) (*model.User, error)

	CountUsers(ctx context.Context) (int, error)
	CountInactiveUsers(ctx context.Context, inactiveSince time.Time) (int, error)
}
