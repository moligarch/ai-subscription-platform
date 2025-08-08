package usecase

import (
    "context"

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
func (u *UserUseCase) RegisterOrFetch(ctx context.Context, tgID int64, fullName, phone string) (*domain.User, error) {
    usr, err := u.userRepo.FindByTelegramID(ctx, tgID)
    if err == nil {
        return usr, nil // already exists
    }
    if err != domain.ErrNotFound {
        return nil, err // unexpected
    }
    // create new
    id := domain.NewUUID() // assume helper generating UUID string
    newUsr, err := domain.NewUser(id, tgID, fullName, phone)
    if err != nil {
        return nil, err
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
