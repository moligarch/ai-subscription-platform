package usecase

import (
	"context"
	"errors"
	"sync"

	"telegram-ai-subscription/internal/domain"

	"github.com/google/uuid"
)

// ----------------------
// memUserRepo (shared)
// ----------------------
type memUserRepo struct {
	mu      sync.Mutex
	store   map[int64]*domain.User
	saveErr error
}

func newMemUserRepo() *memUserRepo {
	return &memUserRepo{store: make(map[int64]*domain.User)}
}

func (m *memUserRepo) FindByTelegramID(ctx context.Context, telegramID int64) (*domain.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.store[telegramID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *u
	return &cp, nil
}

func (m *memUserRepo) Save(ctx context.Context, user *domain.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.saveErr != nil {
		return m.saveErr
	}
	cp := *user
	m.store[user.TelegramID] = &cp
	return nil
}

// ----------------------
// memPlanRepo (shared)
// ----------------------
type memPlanRepo struct {
	mu      sync.Mutex
	byID    map[string]*domain.SubscriptionPlan
	nameTo  map[string]string
	saveErr error
}

func newMemPlanRepo() *memPlanRepo {
	return &memPlanRepo{
		byID:   make(map[string]*domain.SubscriptionPlan),
		nameTo: make(map[string]string),
	}
}

func (m *memPlanRepo) Save(ctx context.Context, p *domain.SubscriptionPlan) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.saveErr != nil {
		return m.saveErr
	}
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	// unique name check
	if existing, ok := m.nameTo[p.Name]; ok && existing != p.ID {
		return errors.New("plan name already exists")
	}
	cp := *p
	m.byID[p.ID] = &cp
	m.nameTo[p.Name] = p.ID
	return nil
}

func (m *memPlanRepo) FindByID(ctx context.Context, id string) (*domain.SubscriptionPlan, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.byID[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *p
	return &cp, nil
}

func (m *memPlanRepo) ListAll(ctx context.Context) ([]*domain.SubscriptionPlan, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*domain.SubscriptionPlan, 0, len(m.byID))
	for _, p := range m.byID {
		cp := *p
		out = append(out, &cp)
	}
	return out, nil
}

// ----------------------
// memSubRepo (shared) - minimal for subscription tests
// ----------------------
type memSubRepo struct {
	mu      sync.Mutex
	store   map[string]*domain.UserSubscription // keyed by userID for simplicity
	saveErr error
}

func newMemSubRepo() *memSubRepo {
	return &memSubRepo{store: make(map[string]*domain.UserSubscription)}
}

func (m *memSubRepo) Save(ctx context.Context, s *domain.UserSubscription) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.saveErr != nil {
		return m.saveErr
	}
	cp := *s
	m.store[s.UserID] = &cp
	return nil
}

func (m *memSubRepo) FindActiveByUser(ctx context.Context, userID string) (*domain.UserSubscription, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.store[userID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *s
	return &cp, nil
}

func (m *memSubRepo) FindExpiring(ctx context.Context, withinDays int) ([]*domain.UserSubscription, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []*domain.UserSubscription{}
	// naive: include all active for tests; specific tests can seed accordingly
	for _, s := range m.store {
		cp := *s
		out = append(out, &cp)
	}
	return out, nil
}
