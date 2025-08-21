package usecase

import (
	"context"
	"time"

	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/repository"

	"github.com/google/uuid"
)

// UserUseCase handles user-related business logic.
type UserUseCase struct {
	userRepo repository.UserRepository
}

// NewUserUseCase constructs a UserUseCase.
func NewUserUseCase(userRepo repository.UserRepository) *UserUseCase {
	return &UserUseCase{userRepo: userRepo}
}

// RegisterOrFetch ensures a user exists, creating if needed.
// If username is non-nil, it updates it on conflict.
func (u *UserUseCase) RegisterOrFetch(ctx context.Context, tgID int64, username string) (*model.User, error) {
	usr, err := u.userRepo.FindByTelegramID(ctx, tgID)
	if err == nil {
		// Optionally update username if changed
		if username != "" && (usr.Username == "" || usr.Username != username) {
			usr.Username = username
			usr.LastActiveAt = time.Now()
			if err := u.userRepo.Save(ctx, usr); err != nil {
				return nil, err
			}
		}
		return usr, nil
	}
	if err != domain.ErrNotFound {
		return nil, err
	}
	// create new
	newUsr := &model.User{
		ID:           uuid.NewString(),
		TelegramID:   tgID,
		Username:     username,
		RegisteredAt: time.Now(),
		LastActiveAt: time.Now(),
	}
	if err := u.userRepo.Save(ctx, newUsr); err != nil {
		return nil, err
	}
	return newUsr, nil
}

// GetByTelegramID retrieves user or ErrNotFound.
func (u *UserUseCase) GetByTelegramID(ctx context.Context, tgID int64) (*model.User, error) {
	return u.userRepo.FindByTelegramID(ctx, tgID)
}

// CountUsers returns total number of users (delegates to repository)
func (u *UserUseCase) CountUsers(ctx context.Context) (int, error) {
	return u.userRepo.CountUsers(ctx)
}

// CountInactiveUsers returns count of users inactive since the provided time
func (u *UserUseCase) CountInactiveUsers(ctx context.Context, since time.Time) (int, error) {
	return u.userRepo.CountInactiveUsers(ctx, since)
}
