package usecase

import (
	"context"
	"fmt"
	"time"

	"telegram-ai-subscription/internal/domain/ports/repository"
)

// StatsUseCase aggregates read-only statistics used by admin endpoints / bot commands.
type StatsUseCase struct {
	userRepo repository.UserRepository
	subRepo  repository.SubscriptionRepository
	payRepo  repository.PaymentRepository
}

// NewStatsUseCase constructs StatsUseCase.
func NewStatsUseCase(u repository.UserRepository, s repository.SubscriptionRepository, p repository.PaymentRepository) *StatsUseCase {
	return &StatsUseCase{userRepo: u, subRepo: s, payRepo: p}
}

// GetCounts returns user counts, inactive count, active subscriptions by plan and total credits.
func (uc *StatsUseCase) GetCounts(ctx context.Context, inactiveWindow time.Duration) (totalUsers int, inactiveUsers int, activeByPlan map[string]int, totalCredits int, err error) {
	totalUsers, err = uc.userRepo.CountUsers(ctx)
	if err != nil {
		err = fmt.Errorf("count users: %w", err)
		return
	}

	since := time.Now().Add(-inactiveWindow)
	inactiveUsers, err = uc.userRepo.CountInactiveUsers(ctx, since)
	if err != nil {
		err = fmt.Errorf("count inactive users: %w", err)
		return
	}

	activeByPlan, err = uc.subRepo.CountActiveByPlan(ctx)
	if err != nil {
		err = fmt.Errorf("count active by plan: %w", err)
		return
	}

	totalCredits, err = uc.subRepo.TotalRemainingCredits(ctx)
	if err != nil {
		err = fmt.Errorf("total remaining credits: %w", err)
		return
	}

	return
}

// GetPaymentsForPeriods returns the payments (Toman) for last 7/30/365 days.
func (uc *StatsUseCase) GetPaymentsForPeriods(ctx context.Context) (week, month, year float64, err error) {
	now := time.Now()
	epsilon := time.Second

	weekSince := now.Add(-7 * 24 * time.Hour)
	week, err = uc.payRepo.TotalPaymentsInPeriod(ctx, weekSince, now.Add(epsilon))
	if err != nil {
		err = fmt.Errorf("week payments: %w", err)
		return
	}

	monthSince := now.Add(-30 * 24 * time.Hour)
	month, err = uc.payRepo.TotalPaymentsInPeriod(ctx, monthSince, now.Add(epsilon))
	if err != nil {
		err = fmt.Errorf("month payments: %w", err)
		return
	}

	yearSince := now.Add(-365 * 24 * time.Hour)
	year, err = uc.payRepo.TotalPaymentsInPeriod(ctx, yearSince, now.Add(epsilon))
	if err != nil {
		err = fmt.Errorf("year payments: %w", err)
		return
	}
	return
}
