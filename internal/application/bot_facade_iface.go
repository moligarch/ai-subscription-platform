package application

import (
	"context"
	"telegram-ai-subscription/internal/domain/model"
	"time"
)

// ---- small interfaces to decouple the facade from concrete usecase structs ----
// These describe the minimal surface that the facade needs. Using interfaces
// enables tests to pass in light-weight mocks.
type UserUseCaseIface interface {
	RegisterOrFetch(ctx context.Context, tgID int64, username string) (*model.User, error)
	GetByTelegramID(ctx context.Context, tgID int64) (*model.User, error)
	// CountUsers returns total number of users (delegates to repository)
	CountUsers(ctx context.Context) (int, error)
	// CountInactiveUsers returns count of users inactive since the provided time
	CountInactiveUsers(ctx context.Context, since time.Time) (int, error)
}

type PlanUseCaseIface interface {
	Create(ctx context.Context, p *model.SubscriptionPlan) error
	Get(ctx context.Context, id string) (*model.SubscriptionPlan, error)
	List(ctx context.Context) ([]*model.SubscriptionPlan, error)
	Update(ctx context.Context, p *model.SubscriptionPlan) error
	Delete(ctx context.Context, id string) error
}

type SubscriptionUseCaseIface interface {
	Subscribe(ctx context.Context, userID, planID string) (*model.UserSubscription, error)
	GetActiveSubscription(ctx context.Context, userID string) (*model.UserSubscription, error)
	DeductCredits(ctx context.Context, sub *model.UserSubscription, amount int) (*model.UserSubscription, error)
	CountActiveSubscriptionsByPlan(ctx context.Context) (map[string]int, error)
	TotalRemainingCredits(ctx context.Context) (int, error)
}

type PaymentUseCaseIface interface {
	Initiate(ctx context.Context, userID, planID string, amountIRR int64, description string, meta map[string]interface{}) (*model.Payment, string, error)
	Confirm(ctx context.Context, payID string, expectedAmount int64) (*model.Payment, error)
	TotalPayments(ctx context.Context, period string) (int64, error)
	// TotalPaymentsInPeriod(ctx context.Context, since, until time.Time) (float64, error)
	// TotalPaymentsForUser(ctx context.Context, userID string, since, until time.Time) (float64, error)
	// TotalPaymentsForPlan(ctx context.Context, plan
}

type StatsUseCaseIface interface {
	// Provide methods used by HandleStats
	GetCounts(ctx context.Context, inactiveWindow time.Duration) (totalUsers int, inactiveUsers int, byPlan map[string]int, totalCredits int, err error)
	GetPaymentsForPeriods(ctx context.Context) (week, month, year int64, err error)
}

type NotificationUseCaseIface interface {
	CheckAndNotify(ctx context.Context, withinDays int) (int, error)
}
