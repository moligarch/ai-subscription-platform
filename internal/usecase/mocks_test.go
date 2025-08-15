// File: internal/usecase/mocks_test.go
package usecase

import (
	"context"
	"fmt"
	"sync"
	"time"

	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/model"
)

// memUserRepo is a small in-memory implementation used by unit tests.
type memUserRepo struct {
	mu      sync.RWMutex
	store   map[int64]*model.User // map by TelegramID
	saveErr error                 // used by tests to simulate save failures
}

func newMemUserRepo() *memUserRepo {
	return &memUserRepo{store: make(map[int64]*model.User)}
}

func (m *memUserRepo) FindByTelegramID(ctx context.Context, telegramID int64) (*model.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	u, ok := m.store[telegramID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *u
	return &cp, nil
}

func (m *memUserRepo) Save(ctx context.Context, user *model.User) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *user
	m.store[user.TelegramID] = &cp
	return nil
}

func (m *memUserRepo) FindByID(ctx context.Context, id string) (*model.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, u := range m.store {
		if u.ID == id {
			cp := *u
			return &cp, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (m *memUserRepo) CountUsers(ctx context.Context) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.store), nil
}

func (m *memUserRepo) CountInactiveUsers(ctx context.Context, inactiveSince time.Time) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cnt := 0
	for _, u := range m.store {
		la := u.LastActiveAt
		if la.IsZero() {
			if u.RegisteredAt.Before(inactiveSince) || u.RegisteredAt.Equal(inactiveSince) {
				cnt++
			}
			continue
		}
		if la.Before(inactiveSince) || la.Equal(inactiveSince) {
			cnt++
		}
	}
	return cnt, nil
}

// memSubRepo provides in-memory subscriptions for tests, and satisfies SubscriptionRepository including stats methods.
type memSubRepo struct {
	mu   sync.RWMutex
	subs map[string]*model.UserSubscription // map userID -> subscription
}

func newMemSubRepo() *memSubRepo {
	return &memSubRepo{subs: make(map[string]*model.UserSubscription)}
}

func (m *memSubRepo) Save(ctx context.Context, sub *model.UserSubscription) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *sub
	m.subs[sub.UserID] = &cp
	return nil
}

func (m *memSubRepo) FindActiveByUser(ctx context.Context, userID string) (*model.UserSubscription, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.subs[userID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *s
	return &cp, nil
}

func (m *memSubRepo) FindExpiring(ctx context.Context, withinDays int) ([]*model.UserSubscription, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []*model.UserSubscription
	cut := time.Now().Add(time.Duration(withinDays) * 24 * time.Hour)
	for _, s := range m.subs {
		if s.Active && s.ExpiresAt.Before(cut) {
			cp := *s
			out = append(out, &cp)
		}
	}
	return out, nil
}

// CountActiveByPlan implements the new statistics method: map[planName]count
func (m *memSubRepo) CountActiveByPlan(ctx context.Context) (map[string]int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]int)
	for _, s := range m.subs {
		if s.Active {
			out[s.PlanID] = out[s.PlanID] + 1
		}
	}
	return out, nil
}

// TotalRemainingCredits returns sum of remaining credits for active subscriptions.
func (m *memSubRepo) TotalRemainingCredits(ctx context.Context) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	sum := 0
	for _, s := range m.subs {
		if s.Active {
			sum += s.RemainingCredits
		}
	}
	return sum, nil
}

// memPlanRepo minimal mock used by tests
type memPlanRepo struct {
	mu    sync.RWMutex
	plans map[string]*model.SubscriptionPlan
}

func newMemPlanRepo() *memPlanRepo {
	return &memPlanRepo{plans: make(map[string]*model.SubscriptionPlan)}
}

func (m *memPlanRepo) Save(ctx context.Context, p *model.SubscriptionPlan) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *p
	if cp.ID == "" {
		cp.ID = fmt.Sprintf("plan-%d", time.Now().UnixNano())
	}
	m.plans[cp.ID] = &cp
	return nil
}

func (m *memPlanRepo) FindByID(ctx context.Context, id string) (*model.SubscriptionPlan, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.plans[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *p
	return &cp, nil
}

func (m *memPlanRepo) ListAll(ctx context.Context) ([]*model.SubscriptionPlan, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*model.SubscriptionPlan, 0, len(m.plans))
	for _, p := range m.plans {
		cp := *p
		out = append(out, &cp)
	}
	return out, nil
}
