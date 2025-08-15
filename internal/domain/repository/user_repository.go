package repository

import (
	"context"
	"time"

	"telegram-ai-subscription/internal/domain"
)

// UserRepository defines thread-safe methods (must support concurrent calls)
type UserRepository interface {
	// Save persists a new or existing user
	Save(ctx context.Context, u *domain.User) error
	// FindByTelegramID returns ErrNotFound if missing
	FindByTelegramID(ctx context.Context, tgID int64) (*domain.User, error)

	CountUsers(ctx context.Context) (int, error)
	CountInactiveUsers(ctx context.Context, inactiveSince time.Time) (int, error)
}
