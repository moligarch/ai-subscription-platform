package sched

import (
	"context"
	"log"
	"time"

	"telegram-ai-subscription/internal/domain/ports/repository"
	"telegram-ai-subscription/internal/usecase"
)

// PaymentReconciler periodically scans for stale pending payments and tries to finalize them
// by calling PaymentUseCase.ConfirmAuto(authority). This covers cases where the callback failed
// or the process crashed mid-confirm.
type PaymentReconciler struct {
	uc         usecase.PaymentUseCase
	payments   repository.PaymentRepository
	interval   time.Duration // how often to scan
	staleAfter time.Duration // how old a pending payment must be to retry
}

func NewPaymentReconciler(uc usecase.PaymentUseCase, payments repository.PaymentRepository, interval, staleAfter time.Duration) *PaymentReconciler {
	if interval <= 0 {
		interval = time.Minute
	}
	if staleAfter <= 0 {
		staleAfter = 10 * time.Minute
	}
	return &PaymentReconciler{uc: uc, payments: payments, interval: interval, staleAfter: staleAfter}
}

func (w *PaymentReconciler) Start(ctx context.Context) {
	t := time.NewTicker(w.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			w.tick(ctx)
		}
	}
}

func (w *PaymentReconciler) tick(ctx context.Context) {
	cutoff := time.Now().Add(-w.staleAfter)
	pending, err := w.payments.ListPendingOlderThan(ctx, nil, cutoff, 200)
	if err != nil {
		log.Printf("payment-reconciler: list pending error: %v", err)
		return
	}
	for _, p := range pending {
		if p.Authority == "" {
			continue
		}
		if _, err := w.uc.ConfirmAuto(ctx, p.Authority); err != nil {
			log.Printf("payment-reconciler: confirm auto failed payment=%s authority=%s err=%v", p.ID, p.Authority, err)
			continue
		}
		log.Printf("payment-reconciler: reconciled payment=%s", p.ID)
	}
}
