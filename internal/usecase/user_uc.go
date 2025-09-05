package usecase

import (
	"context"
	"time"

	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/repository"
	"telegram-ai-subscription/internal/infra/logging"

	"github.com/jackc/pgx/v4"
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
	tm    repository.TransactionManager
	log   *zerolog.Logger
}

func NewUserUseCase(users repository.UserRepository, tm repository.TransactionManager, logger *zerolog.Logger) *userUC {
	return &userUC{
		users: users,
		tm:    tm,
		log:   logger,
	}
}

func (u *userUC) RegisterOrFetch(ctx context.Context, tgID int64, username string) (*model.User, error) {
	defer logging.TraceDuration(u.log, "UserUC.RegisterOrFetch")()

	var user *model.User
	// This transaction is simple but ensures the read (find) and write (save)
	// are treated as a single atomic operation, preventing race conditions.
	txOpts := pgx.TxOptions{IsoLevel: pgx.Serializable}
	err := u.tm.WithTx(ctx, txOpts, func(ctx context.Context, tx repository.Tx) error {
		usr, err := u.users.FindByTelegramID(ctx, tx, tgID)
		if err != nil {
			// Propagate any database error.
			return err
		}

		if usr != nil {
			// If the user exists, we must update their state and SAVE the changes.
			if usr.Username != username && username != "" {
				usr.Username = username
			}
			usr.Touch() // Update the last active time.

			// The missing Save call is now restored.
			if err = u.users.Save(ctx, tx, usr); err != nil {
				u.log.Error().Err(err).Msg("Failed to update user")
				return err
			}
			user = usr
			return nil
		}

		// If user is nil (not found), create a new one.
		nu, err := model.NewUser("", tgID, username)
		if err != nil {
			return err
		}
		if err := u.users.Save(ctx, tx, nu); err != nil {
			return err
		}
		user = nu
		return nil
	})

	return user, err
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
