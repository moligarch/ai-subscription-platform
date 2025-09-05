package usecase

import (
	"context"
	"time"

	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/repository"
	"telegram-ai-subscription/internal/infra/logging"

	"github.com/rs/zerolog"
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

	log *zerolog.Logger
}

func NewUserUseCase(users repository.UserRepository, logger *zerolog.Logger) *userUC {
	return &userUC{
		users: users,
		log:   logger,
	}
}

func (u *userUC) RegisterOrFetch(ctx context.Context, tgID int64, username string) (*model.User, error) {
	defer logging.TraceDuration(u.log, "UserUC.RegisterOrFetch")()
	usr, err := u.users.FindByTelegramID(ctx, repository.NoTX, tgID)
	if err == nil {
		// update username/last_active if changed
		if usr.Username != username && username != "" {
			usr.Username = username
		}
		usr.Touch()
		err = u.users.Save(ctx, repository.NoTX, usr)
		if err != nil {
			u.log.Error().Err(err).Msg("Failed to update user")
		}
		return usr, nil
	}
	// not found -> create
	nu, e := model.NewUser("", tgID, username)
	if e != nil {
		return nil, e
	}
	if err := u.users.Save(ctx, repository.NoTX, nu); err != nil {
		return nil, err
	}
	return nu, nil
}

func (u *userUC) GetByTelegramID(ctx context.Context, tgID int64) (*model.User, error) {
	defer logging.TraceDuration(u.log, "UserUC.GetByTelegramID")()
	return u.users.FindByTelegramID(ctx, repository.NoTX, tgID)
}

func (u *userUC) Count(ctx context.Context) (int, error) {
	defer logging.TraceDuration(u.log, "UserUC.Count")()
	return u.users.CountUsers(ctx, repository.NoTX)
}

func (u *userUC) CountInactiveSince(ctx context.Context, since time.Time) (int, error) {
	defer logging.TraceDuration(u.log, "UserUC.CountInactiveSince")()
	return u.users.CountInactiveUsers(ctx, repository.NoTX, since)
}
