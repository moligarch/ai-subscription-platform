package usecase

import (
	"context"
	"fmt"
	"time"

	"telegram-ai-subscription/internal/domain/ports/adapter"
	"telegram-ai-subscription/internal/domain/ports/repository"
)

// NotificationUseCase finds expiring subscriptions and sends notifications using a Telegram adapter.
type NotificationUseCase struct {
	subRepo repository.SubscriptionRepository
	bot     adapter.TelegramBotAdapter
}

// NewNotificationUseCase constructs and returns a NotificationUseCase.
// subRepo: repository to read subscriptions (FindExpiring).
// bot: adapter to actually send Telegram messages (can be noop or real).
func NewNotificationUseCase(subRepo repository.SubscriptionRepository, bot adapter.TelegramBotAdapter) *NotificationUseCase {
	return &NotificationUseCase{
		subRepo: subRepo,
		bot:     bot,
	}
}

// CheckAndNotify finds subscriptions that will expire within `withinDays` days and notifies their users.
// Returns number of successful notifications and the first error encountered (if any).
func (n *NotificationUseCase) CheckAndNotify(ctx context.Context, withinDays int) (int, error) {
	if withinDays <= 0 {
		withinDays = 1
	}

	subs, err := n.subRepo.FindExpiring(ctx, withinDays)
	if err != nil {
		return 0, fmt.Errorf("find expiring subscriptions: %w", err)
	}

	var firstErr error
	sent := 0

	for _, sub := range subs {
		msg := fmt.Sprintf("🔔 Your subscription (plan %s) will expire on %s. Please renew to avoid interruption.",
			sub.PlanID, sub.ExpiresAt.Format(time.RFC1123))

		// Stop a single send from blocking the rest
		sendCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := n.bot.SendMessage(sendCtx, sub.UserID, msg)
		cancel()

		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("notify user %s: %w", sub.UserID, err)
			}
			continue
		}
		sent++
	}

	return sent, firstErr
}
