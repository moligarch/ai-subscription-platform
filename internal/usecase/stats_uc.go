package usecase

import (
	"context"
	"time"

	"telegram-ai-subscription/internal/domain/ports/repository"
)

// Compile-time check
var _ StatsUseCase = (*statsUC)(nil)

type StatsUseCase interface {
	Totals(ctx context.Context) (users int, activeByPlan map[string]int, remainingCredits int, err error)
	Revenue(ctx context.Context) (week int64, month int64, year int64, err error)
	InactiveUsers(ctx context.Context, olderThan time.Time) (int, error)
}

type statsUC struct {
	users    repository.UserRepository
	subs     repository.SubscriptionRepository
	payments repository.PaymentRepository
}

func NewStatsUseCase(users repository.UserRepository, subs repository.SubscriptionRepository, payments repository.PaymentRepository) *statsUC {
	return &statsUC{users: users, subs: subs, payments: payments}
}

func (s *statsUC) Totals(ctx context.Context) (int, map[string]int, int, error) {
	users, err := s.users.CountUsers(ctx, nil)
	if err != nil {
		return 0, nil, 0, err
	}
	active, err := s.subs.CountActiveByPlan(ctx, nil)
	if err != nil {
		return 0, nil, 0, err
	}
	rem, err := s.subs.TotalRemainingCredits(ctx, nil)
	if err != nil {
		return 0, nil, 0, err
	}
	return users, active, rem, nil
}

func (s *statsUC) Revenue(ctx context.Context) (int64, int64, int64, error) {
	w, err := s.payments.SumByPeriod(ctx, nil, "week")
	if err != nil {
		return 0, 0, 0, err
	}
	m, err := s.payments.SumByPeriod(ctx, nil, "month")
	if err != nil {
		return 0, 0, 0, err
	}
	y, err := s.payments.SumByPeriod(ctx, nil, "year")
	if err != nil {
		return 0, 0, 0, err
	}
	return w, m, y, nil
}

func (s *statsUC) InactiveUsers(ctx context.Context, olderThan time.Time) (int, error) {
	return s.users.CountInactiveUsers(ctx, nil, olderThan)
}
