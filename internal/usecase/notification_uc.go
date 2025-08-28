package usecase

import (
	"context"
	"telegram-ai-subscription/internal/domain/ports/repository"
)

// Compile-time check
var _ NotificationUseCase = (*notificationUC)(nil)

type NotificationUseCase interface {
	// CheckAndCountExpiring returns how many active subscriptions are expiring within N days.
	CheckAndCountExpiring(ctx context.Context, withinDays int) (int, error)
}

type notificationUC struct {
	subs repository.SubscriptionRepository
}

func NewNotificationUseCase(subs repository.SubscriptionRepository) *notificationUC {
	return &notificationUC{subs: subs}
}

func (n *notificationUC) CheckAndCountExpiring(ctx context.Context, withinDays int) (int, error) {
	items, err := n.subs.FindExpiring(ctx, nil, withinDays)
	if err != nil {
		return 0, err
	}
	return len(items), nil
}
