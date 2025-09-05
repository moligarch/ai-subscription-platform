package sched

import (
	"context"
	"log"
	"time"

	"telegram-ai-subscription/internal/domain/ports/repository"
	"telegram-ai-subscription/internal/usecase"
)

// ExpiryWorker periodically finishes expired subscriptions via the use case.
type ExpiryWorker struct {
	interval time.Duration
	subUC    usecase.SubscriptionUseCase
}

func NewExpiryWorker(interval time.Duration, subs repository.SubscriptionRepository, plans repository.SubscriptionPlanRepository, subUC usecase.SubscriptionUseCase) *ExpiryWorker {
	return &ExpiryWorker{interval: interval, subUC: subUC}
}

func (w *ExpiryWorker) Run(ctx context.Context) error {
	t := time.NewTicker(w.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			n, err := w.subUC.FinishExpired(ctx)
			if err != nil {
				log.Printf("expiry worker error: %v", err)
			}
			if n > 0 {
				log.Printf("expired subscriptions finished: %d", n)
			}
		}
	}
}
