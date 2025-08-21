package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/model"
)

func isExpiredOrInactiveErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, domain.ErrExpiredSubscription) {
		return true
	}
	// some implementations may return textual errors like "subscription not active" or "expired"
	l := strings.ToLower(err.Error())
	if strings.Contains(l, "expired") || strings.Contains(l, "not active") || strings.Contains(l, "inactive") {
		return true
	}
	return false
}

func TestSubscribeCreatesNewSubscription(t *testing.T) {
	ctx := context.Background()
	planRepo := newMemPlanRepo()
	subRepo := newMemSubRepo()

	plan := &model.SubscriptionPlan{
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
	if sub.Status != model.SubscriptionStatusActive {
		t.Fatalf("expected status SubscriptionStatusActive got %s", sub.Status)
	}
	if time.Until(*sub.ExpiresAt) < time.Duration(plan.DurationDays-1)*24*time.Hour {
		t.Fatalf("expiresAt seems too short: %v", sub.ExpiresAt)
	}
}

func TestSubscribeExpiredExisting(t *testing.T) {
	ctx := context.Background()
	planRepo := newMemPlanRepo()
	subRepo := newMemSubRepo()

	plan := &model.SubscriptionPlan{
		ID:           "plan-x",
		Name:         "ExtendPlan",
		DurationDays: 3,
		Credits:      5,
		CreatedAt:    time.Now(),
	}
	_ = planRepo.Save(ctx, plan)

	// create an existing subscription that is already expired
	start := time.Now().Add(-6 * 24 * time.Hour)
	expire := time.Now().Add(-24 * time.Hour) // expired
	existing := &model.UserSubscription{
		ID:               "sub-1",
		UserID:           "u1",
		PlanID:           plan.ID,
		StartAt:          &start,
		ExpiresAt:        &expire,
		RemainingCredits: 2,
		Status:           model.SubscriptionStatusFinished, // was active but expired
		CreatedAt:        start,
	}
	_ = subRepo.Save(ctx, existing)

	uc := NewSubscriptionUseCase(planRepo, subRepo)
	sub, err := uc.Subscribe(ctx, existing.UserID, plan.ID)
	if err != nil {
		t.Fatalf("Subscribe error: %v", err)
	}

	// Since previous subscription is expired, we expect a NEW active subscription (not extension of the old one)
	if sub.ID == existing.ID {
		t.Fatalf("expected a new subscription to be created, but got same ID %s", sub.ID)
	}
	if sub.PlanID != plan.ID {
		t.Fatalf("expected planID %s got %s", plan.ID, sub.PlanID)
	}
	// New subscription should have full credits (plan.Credits)
	if sub.RemainingCredits != plan.Credits {
		t.Fatalf("expected remaining %d got %d", plan.Credits, sub.RemainingCredits)
	}
	if sub.Status != model.SubscriptionStatusActive {
		t.Fatalf("expected new subscription to be active, got status %s", sub.Status)
	}
	// StartAt/ExpiresAt should be set for the new subscription
	if sub.StartAt == nil || sub.ExpiresAt == nil {
		t.Fatalf("expected start_at and expires_at to be set for new subscription")
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

	plan := &model.SubscriptionPlan{ID: id, Name: name, DurationDays: durationDays, Credits: credits, CreatedAt: time.Now()}
	_ = planRepo.Save(ctx, plan)

	uc := NewSubscriptionUseCase(planRepo, subRepo)
	userID := "u-deduct"

	// create subscription directly by Subscribe
	sub, err := uc.Subscribe(ctx, userID, plan.ID)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	updated, err := uc.DeductCredits(ctx, sub, 1)
	if err != nil {
		t.Fatalf("DeductCredit failed: %v", err)
	}
	if updated.RemainingCredits != credits-1 {
		t.Fatalf("expected remaining %d got %d", credits-1, updated.RemainingCredits)
	}
}

func _TestDeductCreditExpired(t *testing.T) {
	ctx := context.Background()
	planRepo := newMemPlanRepo()
	subRepo := newMemSubRepo()

	plan := &model.SubscriptionPlan{ID: "p-exp", Name: "P", DurationDays: 1, Credits: 1, CreatedAt: time.Now()}
	_ = planRepo.Save(ctx, plan)

	start := time.Now().Add(-10 * 24 * time.Hour)
	expire := time.Now().Add(-1 * time.Hour)
	expired := &model.UserSubscription{
		ID:               "sub-exp",
		UserID:           "user-exp",
		PlanID:           plan.ID,
		StartAt:          &start,
		ExpiresAt:        &expire,
		RemainingCredits: 5,
		Status:           model.SubscriptionStatusActive,
		CreatedAt:        start,
	}
	_ = subRepo.Save(ctx, expired)

	uc := NewSubscriptionUseCase(planRepo, subRepo)
	_, err := uc.DeductCredits(ctx, expired, 1)
	if err == nil {
		t.Fatalf("expected an error when deducting from an expired subscription, got nil")
	}

	// Accept domain.ErrExpiredSubscription or an error message mentioning "expired" or "not active"
	if isExpiredOrInactiveErr(err) {
		t.Fatalf("expected ErrExpiredSubscription or inactive error, got: %v", err)
	}
}

func TestDeductNoCredits(t *testing.T) {
	ctx := context.Background()
	planRepo := newMemPlanRepo()
	subRepo := newMemSubRepo()

	plan := &model.SubscriptionPlan{ID: "p-empty", Name: "P", DurationDays: 7, Credits: 0, CreatedAt: time.Now()}
	_ = planRepo.Save(ctx, plan)

	now := time.Now()
	expire := now.Add(7 * 24 * time.Hour)
	sub := &model.UserSubscription{
		ID:               "sub-empty",
		UserID:           "user-empty",
		PlanID:           plan.ID,
		StartAt:          &now,
		ExpiresAt:        &expire,
		RemainingCredits: 0,
		Status:           model.SubscriptionStatusActive,
		CreatedAt:        now,
	}
	_ = subRepo.Save(ctx, sub)

	uc := NewSubscriptionUseCase(planRepo, subRepo)
	_, err := uc.DeductCredits(ctx, sub, 1)
	if err == nil {
		t.Fatalf("expected ErrInsufficientCredits")
	}
	if !errors.Is(err, domain.ErrInsufficientCredits) && !strings.Contains(strings.ToLower(err.Error()), "insufficient") {
		t.Fatalf("expected ErrInsufficientCredits got %v", err)
	}
}

func TestSubscribePlanNotFound(t *testing.T) {
	ctx := context.Background()
	planRepo := newMemPlanRepo()
	subRepo := newMemSubRepo()

	uc := NewSubscriptionUseCase(planRepo, subRepo)
	_, err := uc.Subscribe(ctx, "user-x", "no-such-plan")
	if err == nil {
		t.Fatalf("expected ErrNotFound")
	}
	if err != domain.ErrNotFound {
		t.Fatalf("expected ErrNotFound got %v", err)
	}
}

func TestDeductCreditInactiveSubscription(t *testing.T) {
	ctx := context.Background()
	planRepo := newMemPlanRepo()
	subRepo := newMemSubRepo()

	plan := &model.SubscriptionPlan{ID: "p-inactive", Name: "P", DurationDays: 7, Credits: 5, CreatedAt: time.Now()}
	_ = planRepo.Save(ctx, plan)

	now := time.Now()
	expire := now.Add(7 * 24 * time.Hour)
	sub := &model.UserSubscription{
		ID:               "sub-inactive",
		UserID:           "user-inactive",
		PlanID:           plan.ID,
		StartAt:          &now,
		ExpiresAt:        &expire,
		RemainingCredits: 5,
		Status:           model.SubscriptionStatusNone, // explicitly not active
		CreatedAt:        now,
	}
	_ = subRepo.Save(ctx, sub)

	uc := NewSubscriptionUseCase(planRepo, subRepo)
	_, err := uc.DeductCredits(ctx, sub, 1)
	if err == nil {
		t.Fatalf("expected ErrExpiredSubscription/ErrNotActive for inactive subscription")
	}
	if !isExpiredOrInactiveErr(err) {
		t.Fatalf("expected expired/inactive error, got %v", err)
	}
}

func TestSubscribeExtendsExpired(t *testing.T) {
	ctx := context.Background()
	planRepo := newMemPlanRepo()
	subRepo := newMemSubRepo()

	plan := &model.SubscriptionPlan{ID: "plan-exp", Name: "P", DurationDays: 5, Credits: 3, CreatedAt: time.Now()}
	_ = planRepo.Save(ctx, plan)

	start := time.Now().Add(-10 * 24 * time.Hour)
	expire := time.Now().Add(-24 * time.Hour)
	sub := &model.UserSubscription{
		ID:               "sub-expired",
		UserID:           "user-expired",
		PlanID:           plan.ID,
		StartAt:          &start,
		ExpiresAt:        &expire, // already expired
		RemainingCredits: 0,
		Status:           model.SubscriptionStatusActive,
		CreatedAt:        start,
	}
	_ = subRepo.Save(ctx, sub)

	uc := NewSubscriptionUseCase(planRepo, subRepo)
	updated, err := uc.Subscribe(ctx, sub.UserID, plan.ID)
	if err != nil {
		t.Fatalf("Subscribe on expired failed: %v", err)
	}

	if updated.RemainingCredits != plan.Credits {
		t.Fatalf("expected credits reset to %d got %d", plan.Credits, updated.RemainingCredits)
	}
	if updated.ExpiresAt.Before(time.Now()) {
		t.Fatalf("expected expiry to be in the future, got %v", updated.ExpiresAt)
	}
}
