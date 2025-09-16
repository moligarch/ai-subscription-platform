package usecase

import (
	"context"
	"time"

	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/adapter"
	"telegram-ai-subscription/internal/domain/ports/repository"
	"telegram-ai-subscription/internal/infra/worker"

	"github.com/rs/zerolog"
)

type BroadcastUseCase interface {
	BroadcastMessage(ctx context.Context, message string) (int, error)
}

type broadcastUC struct {
	users      repository.UserRepository
	bot        adapter.TelegramBotAdapter
	workerPool *worker.Pool
	log        *zerolog.Logger
}

func NewBroadcastUseCase(
	users repository.UserRepository,
	bot adapter.TelegramBotAdapter,
	pool *worker.Pool,
	logger *zerolog.Logger,
) BroadcastUseCase {
	return &broadcastUC{
		users:      users,
		bot:        bot,
		workerPool: pool,
		log:        logger,
	}
}

func (uc *broadcastUC) BroadcastMessage(ctx context.Context, message string) (int, error) {
	allUsers, err := uc.users.List(ctx, repository.NoTX, 0, 0)
	if err != nil {
		uc.log.Error().Err(err).Msg("Failed to fetch all users for broadcast")
		return 0, err
	}

	var nonAdminUsers []*model.User
	for _, user := range allUsers {
		if !user.IsAdmin {
			nonAdminUsers = append(nonAdminUsers, user)
		}
	}
	// Throttle to respect Telegram's API limits (approx. 30 messages/sec)
	throttle := time.NewTicker(time.Second / 25)

	go func() {
		defer throttle.Stop()
		uc.log.Info().Int("user_count", len(nonAdminUsers)).Msg("Starting broadcast job to non-admins")

		for _, user := range nonAdminUsers {
			<-throttle.C // Wait for the ticker

			// Create a task for the worker pool
			task := uc.createSendTask(user.TelegramID, message)
			if err := uc.workerPool.Submit(task); err != nil {
				uc.log.Warn().Err(err).Int64("tg_id", user.TelegramID).Msg("Failed to submit broadcast task to worker pool")
			}
		}
		uc.log.Info().Msg("Broadcast job finished queuing all tasks")
	}()

	return len(nonAdminUsers), nil
}

// createSendTask creates a closure for the worker pool to execute.
func (uc *broadcastUC) createSendTask(telegramID int64, message string) worker.Task {
	return func(ctx context.Context) error {
		err := uc.bot.SendMessage(ctx, adapter.SendMessageParams{
			ChatID: telegramID,
			Text:   message,
		})
		if err != nil {
			// Log specific errors, e.g., user blocked the bot
			uc.log.Warn().Err(err).Int64("tg_id", telegramID).Msg("Failed to send broadcast message to user")
		}
		return nil // Return nil so the worker pool doesn't log it as a task error
	}
}
