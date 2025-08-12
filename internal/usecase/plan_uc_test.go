package usecase

import (
	"context"
	"testing"
	"time"

	"telegram-ai-subscription/internal/domain"

	"github.com/google/uuid"
)

// --- plan usecase tests updated to use PlanUseCase API (Create, Get, List) ---

func TestPlanUseCase_CreateAndGet(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := newMemPlanRepo()
	uc := NewPlanUseCase(repo)

	plan := &domain.SubscriptionPlan{
		ID:           "", // let repo assign
		Name:         "basic",
		DurationDays: 7,
		Credits:      10,
		CreatedAt:    time.Now(),
	}

	// create
	if err := uc.Create(ctx, plan); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	// ensure we have an ID assigned (either by usecase or repo)
	if plan.ID == "" {
		t.Fatalf("expected plan.ID to be set after Create")
	}

	// fetch by id using uc.Get
	got, err := uc.Get(ctx, plan.ID)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if got.Name != plan.Name {
		t.Fatalf("expected name %q got %q", plan.Name, got.Name)
	}
	if got.DurationDays != plan.DurationDays {
		t.Fatalf("expected duration %d got %d", plan.DurationDays, got.DurationDays)
	}
	if got.Credits != plan.Credits {
		t.Fatalf("expected credits %d got %d", plan.Credits, got.Credits)
	}
}

func TestPlanUseCase_DuplicateName(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := newMemPlanRepo()
	uc := NewPlanUseCase(repo)

	p1 := &domain.SubscriptionPlan{
		ID:           uuid.NewString(),
		Name:         "pro",
		DurationDays: 30,
		Credits:      100,
		CreatedAt:    time.Now(),
	}
	if err := uc.Create(ctx, p1); err != nil {
		t.Fatalf("create p1: %v", err)
	}

	// attempt to create another plan with same name
	p2 := &domain.SubscriptionPlan{
		ID:           "", // let assign
		Name:         "pro",
		DurationDays: 15,
		Credits:      50,
		CreatedAt:    time.Now(),
	}
	err := uc.Create(ctx, p2)
	if err == nil {
		t.Fatalf("expected error when creating duplicate plan name, got nil")
	}
}

func TestPlanUseCase_ListAll(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := newMemPlanRepo()
	uc := NewPlanUseCase(repo)

	plans := []*domain.SubscriptionPlan{
		{ID: uuid.NewString(), Name: "p-a", DurationDays: 5, Credits: 1, CreatedAt: time.Now()},
		{ID: uuid.NewString(), Name: "p-b", DurationDays: 10, Credits: 2, CreatedAt: time.Now()},
	}
	for _, p := range plans {
		if err := uc.Create(ctx, p); err != nil {
			t.Fatalf("create plan %s: %v", p.Name, err)
		}
	}

	got, err := uc.List(ctx)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(got) != len(plans) {
		t.Fatalf("expected %d plans, got %d", len(plans), len(got))
	}
	// verify names present
	names := map[string]struct{}{}
	for _, p := range got {
		names[p.Name] = struct{}{}
	}
	for _, p := range plans {
		if _, ok := names[p.Name]; !ok {
			t.Fatalf("expected plan %q in list", p.Name)
		}
	}
}

func TestPlanUseCase_GetInvalidID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := newMemPlanRepo()
	uc := NewPlanUseCase(repo)

	_, err := uc.Get(ctx, "non-existent")
	if err == nil {
		t.Fatalf("expected ErrNotFound for invalid id, got nil")
	}
	if err != domain.ErrNotFound {
		t.Fatalf("expected domain.ErrNotFound, got %v", err)
	}
}
