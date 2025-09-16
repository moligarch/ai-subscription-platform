package usecase

import (
	"context"
	"errors"
	"strings"
	"time"

	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/adapter"
	"telegram-ai-subscription/internal/domain/ports/repository"
	"telegram-ai-subscription/internal/infra/i18n"
	"telegram-ai-subscription/internal/infra/logging"
	"telegram-ai-subscription/internal/infra/metrics"

	"github.com/go-redis/redis/v8"
	"github.com/jackc/pgx/v4"
	"github.com/rs/zerolog"
)

// Define registration-specific Step constants here, where they belong.
const (
	StepAwaitFullName     = "awaiting_fullname"
	StepAwaitPhone        = "awaiting_phone"
	StepAwaitVerification = "awaiting_verification"
)

// Compile-time check
var _ UserUseCase = (*userUC)(nil)

// UserUseCase exposes user-related operations used by bot/admin flows.
type UserUseCase interface {
	RegisterOrFetch(ctx context.Context, tgID int64, username string) (*model.User, error)
	GetByTelegramID(ctx context.Context, tgID int64) (*model.User, error)
	FindByID(ctx context.Context, tx repository.Tx, id string) (*model.User, error)
	Count(ctx context.Context) (int, error)
	CountInactiveSince(ctx context.Context, since time.Time) (int, error)
	ToggleMessageStorage(ctx context.Context, tgID int64) error
	ProcessRegistrationStep(ctx context.Context, tgID int64, messageText, phoneNumber string) (reply string, markup *adapter.ReplyMarkup, err error)
	CompleteRegistration(ctx context.Context, tgID int64) error
	ClearRegistrationState(ctx context.Context, tgID int64) error
	StartRegistration(ctx context.Context, tgID int64) error
	SetConversationState(ctx context.Context, tgID int64, state *repository.ConversationState) error
	GetConversationState(ctx context.Context, tgID int64) (*repository.ConversationState, error)
	ClearConversationState(ctx context.Context, tgID int64) error
	List(ctx context.Context, offset, limit int) ([]*model.User, error)
}

type userUC struct {
	users      repository.UserRepository
	sessions   repository.ChatSessionRepository
	stateRepo  repository.StateRepository
	translator *i18n.Translator
	tm         repository.TransactionManager
	log        *zerolog.Logger
}

func NewUserUseCase(
	users repository.UserRepository,
	sessions repository.ChatSessionRepository,
	stateRepo repository.StateRepository,
	translator *i18n.Translator,
	tm repository.TransactionManager,
	logger *zerolog.Logger,
) *userUC {
	return &userUC{
		users:      users,
		sessions:   sessions,
		stateRepo:  stateRepo,
		translator: translator,
		tm:         tm,
		log:        logger,
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
			if err != domain.ErrUserNotFound {
				return err
			}
			u.log.Warn().Err(err).Int64("tg_id", tgID).Msg("Failed to find user by Telegram ID")
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
		metrics.IncUsersRegistered()
		// Start the registration flow for the new user
		initialState := &repository.ConversationState{Step: StepAwaitFullName, Data: make(map[string]string)}
		if err := u.stateRepo.SetState(ctx, tgID, initialState); err != nil {
			// Log the error but don't fail the transaction
			u.log.Error().Err(err).Int64("tg_id", tgID).Msg("failed to set initial registration state")
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

func (u *userUC) FindByID(ctx context.Context, tx repository.Tx, id string) (*model.User, error) {
	defer logging.TraceDuration(u.log, "UserUC.FindByID")()
	return u.users.FindByID(ctx, tx, id)
}

func (u *userUC) Count(ctx context.Context) (int, error) {
	defer logging.TraceDuration(u.log, "UserUC.Count")()
	return u.users.CountUsers(ctx, repository.NoTX)
}

func (u *userUC) CountInactiveSince(ctx context.Context, since time.Time) (int, error) {
	defer logging.TraceDuration(u.log, "UserUC.CountInactiveSince")()
	return u.users.CountInactiveUsers(ctx, repository.NoTX, since)
}

func (u *userUC) ToggleMessageStorage(ctx context.Context, tgID int64) error {
	return u.tm.WithTx(ctx, pgx.TxOptions{}, func(ctx context.Context, tx repository.Tx) error {
		user, err := u.users.FindByTelegramID(ctx, tx, tgID)
		if err != nil {
			return err
		}
		if user == nil {
			return domain.ErrUserNotFound
		}

		// Toggle the setting
		user.Privacy.AllowMessageStorage = !user.Privacy.AllowMessageStorage
		if err := u.users.Save(ctx, tx, user); err != nil {
			return err
		}

		// If storage was just disabled, delete all their chat history.
		if !user.Privacy.AllowMessageStorage {
			if err := u.sessions.DeleteAllByUserID(ctx, tx, user.ID); err != nil {
				// Log the error but don't fail the whole transaction,
				// as the primary goal (updating the setting) succeeded.
				u.log.Error().Err(err).Str("user_id", user.ID).Msg("failed to delete user chat history after disabling storage")
			}
		}
		return nil
	})
}

// ProcessRegistrationStep is the core of the conversational state machine.
func (u *userUC) ProcessRegistrationStep(ctx context.Context, tgID int64, messageText, phoneNumber string) (reply string, markup *adapter.ReplyMarkup, err error) {
	state, err := u.stateRepo.GetState(ctx, tgID)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			// This case is for when /start is hit by a pending user whose state expired.
			// The bot handler will re-trigger the start flow.
			return u.translator.T("reg_start", ""), nil, nil
		}
		return u.translator.T("reg_state_expired"), nil, nil
	}

	switch state.Step {
	case StepAwaitFullName:
		// Validate that the user sent non-empty, plain text.
		if strings.TrimSpace(messageText) == "" || phoneNumber != "" {
			return u.translator.T("reg_invalid_fullname"), nil, nil
		}

		state.Data["full_name"] = messageText
		state.Step = StepAwaitPhone
		if err := u.stateRepo.SetState(ctx, tgID, state); err != nil {
			return "", nil, err
		}

		contactMarkup := &adapter.ReplyMarkup{
			Buttons:    [][]adapter.Button{{{Text: u.translator.T("button_share_contact"), RequestContact: true}}},
			IsInline:   false,
			IsOneTime:  true,
			IsPersonal: true,
		}
		return u.translator.T("reg_ask_for_phone"), contactMarkup, nil

	case StepAwaitPhone:
		// Validate that the user sent their contact info and not plain text.
		if phoneNumber == "" {
			contactMarkup := &adapter.ReplyMarkup{
				Buttons:    [][]adapter.Button{{{Text: u.translator.T("button_share_contact"), RequestContact: true}}},
				IsInline:   false,
				IsOneTime:  true,
				IsPersonal: true,
			}
			return u.translator.T("reg_invalid_phone"), contactMarkup, nil
		}

		err := u.tm.WithTx(ctx, pgx.TxOptions{}, func(ctx context.Context, tx repository.Tx) error {
			user, err := u.users.FindByTelegramID(ctx, tx, tgID)
			if err != nil {
				return err
			}
			user.FullName = state.Data["full_name"]
			user.PhoneNumber = phoneNumber
			return u.users.Save(ctx, tx, user)
		})
		if err != nil {
			return "", nil, err
		}

		state.Step = StepAwaitVerification
		if err := u.stateRepo.SetState(ctx, tgID, state); err != nil {
			return "", nil, err
		}

		reply := u.translator.T("reg_ask_for_verification", state.Data["full_name"], phoneNumber)
		verifyMarkup := &adapter.ReplyMarkup{
			Buttons: [][]adapter.Button{
				{{Text: u.translator.T("button_verify_reg"), Data: "reg:verify"}},
				{{Text: u.translator.T("button_read_policy"), Data: "reg:policy"}},
				{{Text: u.translator.T("button_cancel_reg"), Data: "reg:cancel"}},
			},
			IsInline: true,
		}
		return reply, verifyMarkup, nil
	}

	return "مرحله ثبت نام نامشخص است. لطفا با /start مجددا شروع کنید.", nil, nil
}

// CompleteRegistration finalizes the user's registration.
func (u *userUC) CompleteRegistration(ctx context.Context, tgID int64) error {
	err := u.tm.WithTx(ctx, pgx.TxOptions{}, func(ctx context.Context, tx repository.Tx) error {
		user, err := u.users.FindByTelegramID(ctx, tx, tgID)
		if err != nil {
			return err
		}
		user.RegistrationStatus = model.RegistrationStatusCompleted
		return u.users.Save(ctx, tx, user)
	})
	if err != nil {
		return err
	}

	// Clean up the temporary state from Redis
	return u.stateRepo.ClearState(ctx, tgID)
}

// ClearRegistrationState removes a user's pending registration state from Redis.
func (u *userUC) ClearRegistrationState(ctx context.Context, tgID int64) error {
	return u.stateRepo.ClearState(ctx, tgID)
}

// StartRegistration explicitly sets the initial state for the registration flow.
func (u *userUC) StartRegistration(ctx context.Context, tgID int64) error {
	initialState := &repository.ConversationState{
		Step: StepAwaitFullName,
		Data: make(map[string]string),
	}
	return u.stateRepo.SetState(ctx, tgID, initialState)
}

// SetConversationState allows other parts of the application (like bot handlers)
// to initiate a new conversational flow for a user.
func (u *userUC) SetConversationState(ctx context.Context, tgID int64, state *repository.ConversationState) error {
	return u.stateRepo.SetState(ctx, tgID, state)
}

func (u *userUC) GetConversationState(ctx context.Context, tgID int64) (*repository.ConversationState, error) {
	return u.stateRepo.GetState(ctx, tgID)
}

func (u *userUC) ClearConversationState(ctx context.Context, tgID int64) error {
	return u.stateRepo.ClearState(ctx, tgID)
}

func (u *userUC) List(ctx context.Context, offset, limit int) ([]*model.User, error) {
	defer logging.TraceDuration(u.log, "UserUC.List")()
	return u.users.List(ctx, repository.NoTX, offset, limit)
}
