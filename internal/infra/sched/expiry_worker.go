package sched

import (
	"context"
	"telegram-ai-subscription/internal/domain/ports/repository"
	"telegram-ai-subscription/internal/infra/metrics"
	"telegram-ai-subscription/internal/usecase"
	"time"

	"github.com/rs/zerolog"
)

// ExpiryWorker periodically finishes expired subscriptions via the use case.
type ExpiryWorker struct {
	interval time.Duration
	subUC    usecase.SubscriptionUseCase
	log      *zerolog.Logger
}

func NewExpiryWorker(interval time.Duration, subs repository.SubscriptionRepository, plans repository.SubscriptionPlanRepository, subUC usecase.SubscriptionUseCase, logger *zerolog.Logger) *ExpiryWorker {
	exprLog := logger.With().Str("component", "ExpiryWorker").Logger()
	return &ExpiryWorker{
		interval: interval,
		subUC:    subUC,
		log:      &exprLog,
	}
}

func (w *ExpiryWorker) Run(ctx context.Context) error {
	w.log.Info().Msg("Starting expiry worker")
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.log.Info().Msg("Stopping expiry worker")
			return ctx.Err()
		case <-ticker.C:
			n, err := w.subUC.FinishExpired(ctx)
			if err != nil {
				w.log.Error().Err(err).Msg("expiry worker error")
			}
			if n > 0 {
				metrics.IncSubscriptionsExpired(n)
				w.log.Info().Int("count", n).Msg("expired subscriptions finished")
			}
		}
	}
}
