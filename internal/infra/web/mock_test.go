package web

import (
	"context"
	"sync"
	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/repository"
)

// --- Mock Repositories (Ports) ---

type mockUserRepo struct {
	repository.UserRepository // Embed interface for forward compatibility
	mu                        sync.Mutex
	users                     []*model.User
	FindByIDError             error // To simulate errors
	ListError                 error
	CountError                error
}

func (m *mockUserRepo) List(ctx context.Context, tx repository.Tx, offset, limit int) ([]*model.User, error) {
	if m.ListError != nil {
		return nil, m.ListError
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	end := offset + limit
	if end > len(m.users) {
		end = len(m.users)
	}
	if offset >= len(m.users) || offset > end {
		return []*model.User{}, nil
	}
	return m.users[offset:end], nil
}

func (m *mockUserRepo) CountUsers(ctx context.Context, tx repository.Tx) (int, error) {
	if m.CountError != nil {
		return 0, m.CountError
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.users), nil
}

func (m *mockUserRepo) FindByID(ctx context.Context, tx repository.Tx, id string) (*model.User, error) {
	if m.FindByIDError != nil {
		return nil, m.FindByIDError
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, domain.ErrUserNotFound
}

type mockSubRepo struct {
	repository.SubscriptionRepository // Embed interface
	mu                                sync.Mutex
	subs                              []*model.UserSubscription
	ListByUserIDError                 error
}

func (m *mockSubRepo) ListByUserID(ctx context.Context, tx repository.Tx, userID string) ([]*model.UserSubscription, error) {
	if m.ListByUserIDError != nil {
		return nil, m.ListByUserIDError
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var userSubs []*model.UserSubscription
	for _, s := range m.subs {
		if s.UserID == userID {
			userSubs = append(userSubs, s)
		}
	}
	return userSubs, nil
}

func (m *mockSubRepo) CountActiveByPlan(ctx context.Context, tx repository.Tx) (map[string]int, error) {
	// For the test, we can just return an empty map.
	return make(map[string]int), nil
}

func (m *mockSubRepo) TotalRemainingCredits(ctx context.Context, tx repository.Tx) (int64, error) {
	// For the test, we can just return 0.
	return 0, nil
}

type mockPaymentRepo struct {
	repository.PaymentRepository // Embed interface
	SumByPeriodError             error
}

func (m *mockPaymentRepo) SumByPeriod(ctx context.Context, tx repository.Tx, period string) (int64, error) {
	if m.SumByPeriodError != nil {
		return 0, m.SumByPeriodError
	}
	switch period {
	case "week":
		return 100, nil
	case "month":
		return 1000, nil
	case "year":
		return 10000, nil
	}
	return 0, nil
}

type mockPlanRepo struct {
	repository.SubscriptionPlanRepository // Embed interface
	mu                                    sync.Mutex
	plans                                 map[string]*model.SubscriptionPlan
	ListAllError                          error
	SaveError                             error
}

func (m *mockPlanRepo) FindByID(ctx context.Context, tx repository.Tx, id string) (*model.SubscriptionPlan, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if plan, ok := m.plans[id]; ok {
		return plan, nil
	}
	return nil, domain.ErrNotFound
}

func (m *mockPlanRepo) ListAll(ctx context.Context, tx repository.Tx) ([]*model.SubscriptionPlan, error) {
	if m.ListAllError != nil {
		return nil, m.ListAllError
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	plans := make([]*model.SubscriptionPlan, 0, len(m.plans))
	for _, p := range m.plans {
		plans = append(plans, p)
	}
	return plans, nil
}

func (m *mockPlanRepo) Save(ctx context.Context, tx repository.Tx, plan *model.SubscriptionPlan) error {
	if m.SaveError != nil {
		return m.SaveError
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.plans == nil {
		m.plans = make(map[string]*model.SubscriptionPlan)
	}
	m.plans[plan.ID] = plan
	return nil
}
