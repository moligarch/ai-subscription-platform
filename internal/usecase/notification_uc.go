package usecase

import (
	"context"
	"fmt"
	"math"
	"time"

	"telegram-ai-subscription/internal/domain/ports/adapter"
	"telegram-ai-subscription/internal/domain/ports/repository"

	"github.com/rs/zerolog"
)

type NotificationUseCase interface {
	CheckAndSendExpiryNotifications(ctx context.Context) (int, error)
}

type notificationUC struct {
	subs     repository.SubscriptionRepository
	notifLog repository.NotificationLogRepository
	users    repository.UserRepository
	bot      adapter.TelegramBotAdapter
	log      *zerolog.Logger
}

func NewNotificationUseCase(
	subs repository.SubscriptionRepository,
	notifLog repository.NotificationLogRepository,
	users repository.UserRepository,
	bot adapter.TelegramBotAdapter,
	logger *zerolog.Logger,
) NotificationUseCase {
	return &notificationUC{
		subs:     subs,
		notifLog: notifLog,
		users:    users,
		bot:      bot,
		log:      logger,
	}
}

// CheckAndSendExpiryNotifications finds subscriptions expiring soon and sends reminders.
func (n *notificationUC) CheckAndSendExpiryNotifications(ctx context.Context) (int, error) {
	// Define the days before expiration that we want to send a notification.
	thresholds := []int{7, 3, 1}
	sentCount := 0

	// Find all subscriptions expiring within the largest threshold (e.g., 7 days).
	expiringSubs, err := n.subs.FindExpiring(ctx, nil, thresholds[0])
	if err != nil {
		n.log.Error().Err(err).Msg("failed to find expiring subscriptions")
		return 0, err
	}

	for _, sub := range expiringSubs {
		if sub.ExpiresAt == nil {
			continue
		}

		// Calculate how many days are actually left.
		daysLeft := int(math.Ceil(time.Until(*sub.ExpiresAt).Hours() / 24))
		if daysLeft < 0 {
			daysLeft = 0
		}

		// Find the correct notification threshold for the days remaining.
		// e.g., if 6 days are left, the threshold is 7. If 2 days are left, the threshold is 3.
		var applicableThreshold int
		for _, t := range thresholds {
			if daysLeft <= t {
				applicableThreshold = t
			}
		}

		if applicableThreshold == 0 {
			continue // Not within any of our notification windows
		}

		// Check if we've already sent a notification for this specific threshold.
		alreadySent, err := n.notifLog.Exists(ctx, nil, sub.ID, "expiry", applicableThreshold)
		if err != nil {
			n.log.Error().Err(err).Str("sub_id", sub.ID).Msg("failed to check notification log")
			continue
		}

		if !alreadySent {
			user, err := n.users.FindByID(ctx, nil, sub.UserID)
			if err != nil {
				n.log.Error().Err(err).Str("user_id", sub.UserID).Msg("failed to find user for notification")
				continue
			}

			message := fmt.Sprintf("👋 Your subscription is expiring in approximately %d day(s). Use /plans to renew.", daysLeft)
			if err := n.bot.SendMessage(ctx, user.TelegramID, message); err != nil {
				n.log.Error().Err(err).Int64("tg_id", user.TelegramID).Msg("failed to send notification")
				continue // Don't log if we couldn't send
			}

			// Log that we sent the notification to prevent duplicates.
			if err := n.notifLog.Save(ctx, nil, sub.ID, sub.UserID, "expiry", applicableThreshold); err != nil {
				n.log.Error().Err(err).Str("sub_id", sub.ID).Msg("failed to save notification log")
				continue
			}

			n.log.Info().Str("user_id", user.ID).Int("threshold", applicableThreshold).Msg("expiry notification sent")
			sentCount++
		}
	}

	return sentCount, nil
}
