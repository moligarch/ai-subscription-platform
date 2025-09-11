package repository

import (
	"context"
)

// -----------------------------
// Notifications Log
// -----------------------------

type NotificationLogRepository interface {
	// Save records that a notification was sent.
	Save(ctx context.Context, tx Tx, subscriptionID, userID, kind string, thresholdDays int) error
	// Exists checks if a specific notification has already been sent.
	Exists(ctx context.Context, tx Tx, subscriptionID, kind string, thresholdDays int) (bool, error)
}
