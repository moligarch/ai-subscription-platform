package sched

import (
	"context"
	"time"

	"telegram-ai-subscription/internal/usecase"

	"github.com/rs/zerolog"
)

type NotificationWorker struct {
	interval time.Duration
	notifUC  usecase.NotificationUseCase
	log      *zerolog.Logger
}

func NewNotificationWorker(interval time.Duration, notifUC usecase.NotificationUseCase, logger *zerolog.Logger) *NotificationWorker {
	compLog := logger.With().Str("component", "NotificationWorker").Logger()
	return &NotificationWorker{
		interval: interval,
		notifUC:  notifUC,
		log:      &compLog,
	}
}

func (w *NotificationWorker) Run(ctx context.Context) error {
	w.log.Info().Msg("Starting notification worker")
	// Run once on startup, then on every tick
	w.runCheck(ctx)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.log.Info().Msg("Stopping notification worker")
			return ctx.Err()
		case <-ticker.C:
			w.runCheck(ctx)
		}
	}
}

func (w *NotificationWorker) runCheck(ctx context.Context) {
	sent, err := w.notifUC.CheckAndSendExpiryNotifications(ctx)
	if err != nil {
		w.log.Error().Err(err).Msg("notification check failed")
	}
	if sent > 0 {
		w.log.Info().Int("count", sent).Msg("expiry notifications sent")
	}
}
