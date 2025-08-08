package usecase

import (
    "context"
    "time"

    "github.com/google/uuid"
    "telegram-ai-subscription/internal/domain"
)

// UserUseCase handles user-related business logic.
type UserUseCase struct {
    userRepo domain.UserRepository
}

// NewUserUseCase constructs a UserUseCase.
func NewUserUseCase(userRepo domain.UserRepository) *UserUseCase {
    return &UserUseCase{userRepo: userRepo}
}

// RegisterOrFetch ensures a user exists, creating if needed.
// If username is non-nil, it updates it on conflict.
func (u *UserUseCase) RegisterOrFetch(ctx context.Context, tgID int64, username string) (*domain.User, error) {
    usr, err := u.userRepo.FindByTelegramID(ctx, tgID)
    if err == nil {
        // Optionally update username if changed
        if username != "" && (usr.Username == "" || usr.Username != username) {
            usr.Username = username
            usr.RegisteredAt = time.Now()
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
    newUsr := &domain.User{
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
func (u *UserUseCase) GetByTelegramID(ctx context.Context, tgID int64) (*domain.User, error) {
    return u.userRepo.FindByTelegramID(ctx, tgID)
}
