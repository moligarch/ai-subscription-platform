package usecase

import (
	"context"
	"testing"
	"time"

	"telegram-ai-subscription/internal/domain"
)

// --- In-memory mocks ---

type memPlanRepo struct {
	plans map[string]*domain.SubscriptionPlan
}

func newMemPlanRepo() *memPlanRepo { return &memPlanRepo{plans: map[string]*domain.SubscriptionPlan{}} }

func (m *memPlanRepo) Save(ctx context.Context, plan *domain.SubscriptionPlan) error {
	m.plans[plan.ID] = plan
	return nil
}
func (m *memPlanRepo) FindByID(ctx context.Context, id string) (*domain.SubscriptionPlan, error) {
	p, ok := m.plans[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return p, nil
}
func (m *memPlanRepo) ListAll(ctx context.Context) ([]*domain.SubscriptionPlan, error) {
	out := make([]*domain.SubscriptionPlan, 0, len(m.plans))
	for _, p := range m.plans {
		out = append(out, p)
	}
	return out, nil
}

type memSubRepo struct {
	// map[userID] *domain.UserSubscription (only one active per user enforced by repo used in usecase)
	store map[string]*domain.UserSubscription
}

func newMemSubRepo() *memSubRepo { return &memSubRepo{store: map[string]*domain.UserSubscription{}} }

func (m *memSubRepo) Save(ctx context.Context, sub *domain.UserSubscription) error {
	m.store[sub.UserID] = sub
	return nil
}

func (m *memSubRepo) FindActiveByUser(ctx context.Context, userID string) (*domain.UserSubscription, error) {
	sub, ok := m.store[userID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	// return copy to simulate repo behavior
	cp := *sub
	return &cp, nil
}

func (m *memSubRepo) FindExpiring(ctx context.Context, withinDays int) ([]*domain.UserSubscription, error) {
	out := []*domain.UserSubscription{}
	now := time.Now()
	limit := now.Add(time.Duration(withinDays) * 24 * time.Hour)
	for _, s := range m.store {
		if s.Active && s.ExpiresAt.Before(limit) || s.ExpiresAt.Equal(limit) {
			cp := *s
			out = append(out, &cp)
		}
	}
	return out, nil
}

// --- Tests ---

func TestSubscribeCreatesNewSubscription(t *testing.T) {
	ctx := context.Background()
	planRepo := newMemPlanRepo()
	subRepo := newMemSubRepo()

	plan := &domain.SubscriptionPlan{
		ID:           "plan-1",
		Name:         "Basic",
		DurationDays: 7,
		Credits:      10,
		CreatedAt:    time.Now(),
	}
	if err := planRepo.Save(ctx, plan); err != nil {
		t.Fatalf("save plan: %v", err)
	}

	uc := NewSubscriptionUseCase(planRepo, subRepo)
	userID := "user-1"

	sub, err := uc.Subscribe(ctx, userID, plan.ID)
	if err != nil {
		t.Fatalf("Subscribe error: %v", err)
	}

	if sub.UserID != userID {
		t.Fatalf("expected userID %s got %s", userID, sub.UserID)
	}
	if sub.PlanID != plan.ID {
		t.Fatalf("expected planID %s got %s", plan.ID, sub.PlanID)
	}
	if sub.RemainingCredits != plan.Credits {
		t.Fatalf("expected credits %d got %d", plan.Credits, sub.RemainingCredits)
	}
	if sub.Active != true {
		t.Fatalf("expected active true got false")
	}
	if time.Until(sub.ExpiresAt) < time.Duration(plan.DurationDays-1)*24*time.Hour {
		t.Fatalf("expiresAt seems too short: %v", sub.ExpiresAt)
	}
}

func TestSubscribeExtendsExisting(t *testing.T) {
	ctx := context.Background()
	planRepo := newMemPlanRepo()
	subRepo := newMemSubRepo()

	plan := &domain.SubscriptionPlan{
		ID:           "plan-x",
		Name:         "ExtendPlan",
		DurationDays: 3,
		Credits:      5,
		CreatedAt:    time.Now(),
	}
	_ = planRepo.Save(ctx, plan)

	// create an existing subscription with 1 day left
	now := time.Now()
	existing := &domain.UserSubscription{
		ID:               "sub-1",
		UserID:           "u1",
		PlanID:           plan.ID,
		StartAt:          now.Add(-6 * 24 * time.Hour),
		ExpiresAt:        now.Add(24 * time.Hour),
		RemainingCredits: 2,
		Active:           true,
		CreatedAt:        now.Add(-6 * 24 * time.Hour),
	}
	_ = subRepo.Save(ctx, existing)

	uc := NewSubscriptionUseCase(planRepo, subRepo)
	sub, err := uc.Subscribe(ctx, existing.UserID, plan.ID)
	if err != nil {
		t.Fatalf("Subscribe extend error: %v", err)
	}

	// After extension, expires should have increased by plan.DurationDays (approx)
	expectedMin := existing.ExpiresAt.Add(time.Duration(plan.DurationDays-1) * 24 * time.Hour)
	if !sub.ExpiresAt.After(expectedMin) && !sub.ExpiresAt.Equal(expectedMin) {
		t.Fatalf("expected expires to be extended; got %v (expected > %v)", sub.ExpiresAt, expectedMin)
	}
	// Credits increased by plan.Credits
	if sub.RemainingCredits != existing.RemainingCredits+plan.Credits {
		t.Fatalf("expected remaining %d got %d", existing.RemainingCredits+plan.Credits, sub.RemainingCredits)
	}
}

func TestDeductCreditSuccess(t *testing.T) {
	ctx := context.Background()
	planRepo := newMemPlanRepo()
	subRepo := newMemSubRepo()

	var (
		id           = "p1"
		name         = "P"
		durationDays = 7
		credits      = 5
	)

	plan := &domain.SubscriptionPlan{ID: id, Name: name, DurationDays: durationDays, Credits: credits, CreatedAt: time.Now()}
	_ = planRepo.Save(ctx, plan)

	uc := NewSubscriptionUseCase(planRepo, subRepo)
	userID := "u-deduct"

	// create subscription directly by Subscribe
	sub, err := uc.Subscribe(ctx, userID, plan.ID)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	updated, err := uc.DeductCredit(ctx, sub)
	if err != nil {
		t.Fatalf("DeductCredit failed: %v", err)
	}
	if updated.RemainingCredits != credits-1 {
		t.Fatalf("expected remaining %d got %d", credits-1, updated.RemainingCredits)
	}
}

func TestDeductCreditExpired(t *testing.T) {
	ctx := context.Background()
	planRepo := newMemPlanRepo()
	subRepo := newMemSubRepo()

	plan := &domain.SubscriptionPlan{ID: "p-exp", Name: "P", DurationDays: 1, Credits: 1, CreatedAt: time.Now()}
	_ = planRepo.Save(ctx, plan)

	now := time.Now()
	expired := &domain.UserSubscription{
		ID:               "sub-exp",
		UserID:           "user-exp",
		PlanID:           plan.ID,
		StartAt:          now.Add(-10 * 24 * time.Hour),
		ExpiresAt:        now.Add(-1 * time.Hour),
		RemainingCredits: 5,
		Active:           true,
		CreatedAt:        now.Add(-10 * 24 * time.Hour),
	}
	_ = subRepo.Save(ctx, expired)

	uc := NewSubscriptionUseCase(planRepo, subRepo)
	_, err := uc.DeductCredit(ctx, expired)
	if err == nil {
		t.Fatalf("expected ErrExpiredSubscription")
	}
	if err != domain.ErrExpiredSubscription {
		t.Fatalf("expected ErrExpiredSubscription got %v", err)
	}
}

func TestDeductNoCredits(t *testing.T) {
	ctx := context.Background()
	planRepo := newMemPlanRepo()
	subRepo := newMemSubRepo()

	plan := &domain.SubscriptionPlan{ID: "p-empty", Name: "P", DurationDays: 7, Credits: 0, CreatedAt: time.Now()}
	_ = planRepo.Save(ctx, plan)

	now := time.Now()
	sub := &domain.UserSubscription{
		ID:               "sub-empty",
		UserID:           "user-empty",
		PlanID:           plan.ID,
		StartAt:          now,
		ExpiresAt:        now.Add(7 * 24 * time.Hour),
		RemainingCredits: 0,
		Active:           true,
		CreatedAt:        now,
	}
	_ = subRepo.Save(ctx, sub)

	uc := NewSubscriptionUseCase(planRepo, subRepo)
	_, err := uc.DeductCredit(ctx, sub)
	if err == nil {
		t.Fatalf("expected ErrInsufficientCredits")
	}
	if err != domain.ErrInsufficientCredits {
		t.Fatalf("expected ErrInsufficientCredits got %v", err)
	}
}
