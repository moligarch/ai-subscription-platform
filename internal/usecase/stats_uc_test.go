//go:build !integration

package usecase_test

import (
	"context"
	"testing"
	"time"

	"telegram-ai-subscription/internal/domain/ports/repository"
	"telegram-ai-subscription/internal/usecase"
)

func TestStatsUseCase(t *testing.T) {
	ctx := context.Background()
	testLogger := newTestLogger()

	t.Run("Totals should return aggregated data from repositories", func(t *testing.T) {
		// --- Arrange ---
		mockUserRepo := NewMockUserRepo()
		mockSubRepo := NewMockSubscriptionRepo()
		mockPaymentRepo := NewMockPaymentRepo() // Though not used in Totals, it's a dependency

		// Configure mock responses
		mockUserRepo.CountUsersFunc = func(ctx context.Context, tx repository.Tx) (int, error) {
			return 150, nil
		}
		expectedPlanCounts := map[string]int{"plan-pro": 25, "plan-std": 50}
		mockSubRepo.CountActiveByPlanFunc = func(ctx context.Context, tx repository.Tx) (map[string]int, error) {
			return expectedPlanCounts, nil
		}
		mockSubRepo.TotalRemainingCreditsFunc = func(ctx context.Context, tx repository.Tx) (int64, error) {
			return 1234567, nil
		}

		uc := usecase.NewStatsUseCase(mockUserRepo, mockSubRepo, mockPaymentRepo, testLogger)

		// --- Act ---
		users, activeByPlan, remainingCredits, err := uc.Totals(ctx)

		// --- Assert ---
		if err != nil {
			t.Fatalf("expected no error, but got %v", err)
		}
		if users != 150 {
			t.Errorf("expected 150 users, but got %d", users)
		}
		if remainingCredits != 1234567 {
			t.Errorf("expected 1234567 remaining credits, but got %d", remainingCredits)
		}
		if len(activeByPlan) != 2 || activeByPlan["plan-pro"] != 25 {
			t.Errorf("mismatch in active plan counts")
		}
	})

	t.Run("Revenue should return sums from the payment repository", func(t *testing.T) {
		// --- Arrange ---
		mockUserRepo := NewMockUserRepo()
		mockSubRepo := NewMockSubscriptionRepo()
		mockPaymentRepo := NewMockPaymentRepo()

		mockPaymentRepo.SumByPeriodFunc = func(ctx context.Context, tx repository.Tx, period string) (int64, error) {
			switch period {
			case "week":
				return 1000, nil
			case "month":
				return 5000, nil
			case "year":
				return 60000, nil
			}
			return 0, nil
		}

		uc := usecase.NewStatsUseCase(mockUserRepo, mockSubRepo, mockPaymentRepo, testLogger)

		// --- Act ---
		week, month, year, err := uc.Revenue(ctx)

		// --- Assert ---
		if err != nil {
			t.Fatalf("expected no error, but got %v", err)
		}
		if week != 1000 {
			t.Errorf("expected weekly revenue of 1000, but got %d", week)
		}
		if month != 5000 {
			t.Errorf("expected monthly revenue of 5000, but got %d", month)
		}
		if year != 60000 {
			t.Errorf("expected yearly revenue of 60000, but got %d", year)
		}
	})

	t.Run("InactiveUsers should return count from the user repository", func(t *testing.T) {
		// --- Arrange ---
		mockUserRepo := NewMockUserRepo()
		mockSubRepo := NewMockSubscriptionRepo()
		mockPaymentRepo := NewMockPaymentRepo()

		mockUserRepo.CountInactiveUsersFunc = func(ctx context.Context, tx repository.Tx, olderThan time.Time) (int, error) {
			return 42, nil
		}

		uc := usecase.NewStatsUseCase(mockUserRepo, mockSubRepo, mockPaymentRepo, testLogger)

		// --- Act ---
		count, err := uc.InactiveUsers(ctx, time.Now())

		// --- Assert ---
		if err != nil {
			t.Fatalf("expected no error, but got %v", err)
		}
		if count != 42 {
			t.Errorf("expected 42 inactive users, but got %d", count)
		}
	})
}
