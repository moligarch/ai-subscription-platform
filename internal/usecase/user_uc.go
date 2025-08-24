// File: internal/usecase/user_uc.go
package usecase

import (
	"context"
	"time"

	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/repository"
)

// Compile-time check
var _ UserUseCase = (*userUC)(nil)

// UserUseCase exposes user-related operations used by bot/admin flows.
type UserUseCase interface {
	RegisterOrFetch(ctx context.Context, tgID int64, username string) (*model.User, error)
	GetByTelegramID(ctx context.Context, tgID int64) (*model.User, error)
	Count(ctx context.Context) (int, error)
	CountInactiveSince(ctx context.Context, since time.Time) (int, error)
}

type userUC struct {
	users repository.UserRepository
}

func NewUserUseCase(users repository.UserRepository) *userUC { return &userUC{users: users} }

func (u *userUC) RegisterOrFetch(ctx context.Context, tgID int64, username string) (*model.User, error) {
	usr, err := u.users.FindByTelegramID(ctx, nil, tgID)
	if err == nil {
		// update username/last_active if changed
		if usr.Username != username && username != "" {
			usr.Username = username
		}
		usr.Touch()
		_ = u.users.Save(ctx, nil, usr)
		return usr, nil
	}
	// not found -> create
	nu, e := model.NewUser("", tgID, username)
	if e != nil {
		return nil, e
	}
	if err := u.users.Save(ctx, nil, nu); err != nil {
		return nil, err
	}
	return nu, nil
}

func (u *userUC) GetByTelegramID(ctx context.Context, tgID int64) (*model.User, error) {
	return u.users.FindByTelegramID(ctx, nil, tgID)
}

func (u *userUC) Count(ctx context.Context) (int, error) {
	return u.users.CountUsers(ctx, nil)
}

func (u *userUC) CountInactiveSince(ctx context.Context, since time.Time) (int, error) {
	return u.users.CountInactiveUsers(ctx, nil, since)
}
