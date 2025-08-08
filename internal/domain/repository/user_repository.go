package repository

import (
	"context"
	"telegram-ai-subscription/internal/domain"
)

type UserRepository interface {
	FindByTelegramID(ctx context.Context, telegramID int64) (*domain.User, error)
	Save(ctx context.Context, user *domain.User) error
}
