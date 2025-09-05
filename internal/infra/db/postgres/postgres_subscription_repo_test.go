//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"telegram-ai-subscription/internal/domain/model"

	"github.com/google/uuid"
)

func TestSubscriptionRepo_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode.")
	}

	// 1. Setup repos and context
	ctx := context.Background()
	repo := NewSubscriptionRepo(testPool)
	userRepo := NewUserRepo(testPool)
	planRepo := NewPlanRepo(testPool)

	// 2. Create prerequisite data (users and plans)
	user1, _ := model.NewUser("", 111, "user1")
	user2, _ := model.NewUser("", 222, "user2")
	proPlan, _ := model.NewSubscriptionPlan("", "Pro", 30, 1000, 1)
	stdPlan, _ := model.NewSubscriptionPlan("", "Standard", 30, 500, 1)

	// Helper function to set up a clean state for each sub-test
	setupPrerequisites := func(t *testing.T) {
		cleanup(t)
		if err := userRepo.Save(ctx, nil, user1); err != nil {
			t.Fatalf("failed to save user1: %v", err)
		}
		if err := userRepo.Save(ctx, nil, user2); err != nil {
			t.Fatalf("failed to save user2: %v", err)
		}
		if err := planRepo.Save(ctx, nil, proPlan); err != nil {
			t.Fatalf("failed to save proPlan: %v", err)
		}
		if err := planRepo.Save(ctx, nil, stdPlan); err != nil {
			t.Fatalf("failed to save stdPlan: %v", err)
		}
	}

	t.Run("should save and find active and reserved subscriptions", func(t *testing.T) {
		setupPrerequisites(t)
		now := time.Now()
		expires := now.Add(30 * 24 * time.Hour)

		activeSub := &model.UserSubscription{
			ID:               uuid.NewString(),
			UserID:           user1.ID,
			PlanID:           proPlan.ID,
			StartAt:          &now,
			ExpiresAt:        &expires,
			RemainingCredits: 1000,
			Status:           model.SubscriptionStatusActive,
		}
		if err := repo.Save(ctx, nil, activeSub); err != nil {
			t.Fatalf("failed to save active sub: %v", err)
		}

		// Test FindActiveByUser
		foundActive, err := repo.FindActiveByUser(ctx, nil, user1.ID)
		if err != nil {
			t.Fatalf("FindActiveByUser failed: %v", err)
		}
		if foundActive == nil || foundActive.ID != activeSub.ID {
			t.Fatal("did not find the correct active subscription")
		}

		// Test FindActiveByUserAndPlan
		foundActiveByPlan, err := repo.FindActiveByUserAndPlan(ctx, nil, user1.ID, proPlan.ID)
		if err != nil {
			t.Fatalf("FindActiveByUserAndPlan failed: %v", err)
		}
		if foundActiveByPlan == nil || foundActiveByPlan.ID != activeSub.ID {
			t.Fatal("did not find the correct active subscription by plan")
		}

		// Test FindReservedByUser
		reservedSub := &model.UserSubscription{
			ID:     uuid.NewString(),
			UserID: user1.ID,
			PlanID: stdPlan.ID,
			Status: model.SubscriptionStatusReserved,
		}
		if err := repo.Save(ctx, nil, reservedSub); err != nil {
			t.Fatalf("failed to save reserved sub: %v", err)
		}
		reservedSubs, err := repo.FindReservedByUser(ctx, nil, user1.ID)
		if err != nil {
			t.Fatalf("FindReservedByUser failed: %v", err)
		}
		if len(reservedSubs) != 1 || reservedSubs[0].ID != reservedSub.ID {
			t.Fatal("did not find the correct reserved subscriptions")
		}
	})

	t.Run("should find expiring subscriptions", func(t *testing.T) {
		setupPrerequisites(t)
		now := time.Now()

		expiresSoon := now.AddDate(0, 0, 2)
		expiresLater := now.AddDate(0, 0, 10)
		alreadyExpired := now.AddDate(0, 0, -1)

		// --- THE FIX ---
		// Assign the active subscriptions to different users to avoid violating the unique index.
		sub1 := &model.UserSubscription{ID: uuid.NewString(), UserID: user1.ID, PlanID: proPlan.ID, Status: model.SubscriptionStatusActive, ExpiresAt: &expiresSoon}
		sub2 := &model.UserSubscription{ID: uuid.NewString(), UserID: user2.ID, PlanID: proPlan.ID, Status: model.SubscriptionStatusActive, ExpiresAt: &expiresLater}   // Assigned to user2
		sub3 := &model.UserSubscription{ID: uuid.NewString(), UserID: user1.ID, PlanID: stdPlan.ID, Status: model.SubscriptionStatusActive, ExpiresAt: &alreadyExpired} // Assigned to a different plan

		if err := repo.Save(ctx, nil, sub1); err != nil {
			t.Fatalf("failed to save sub1: %v", err)
		}
		if err := repo.Save(ctx, nil, sub2); err != nil {
			t.Fatalf("failed to save sub2: %v", err)
		}
		if err := repo.Save(ctx, nil, sub3); err != nil {
			t.Fatalf("failed to save sub3: %v", err)
		}

		// Test Case 1: Find subs expiring within 3 days
		expiring, err := repo.FindExpiring(ctx, nil, 3)
		if err != nil {
			t.Fatalf("FindExpiring(3) failed: %v", err)
		}
		if len(expiring) != 1 {
			t.Errorf("expected to find 1 subscription expiring within 3 days, but got %d", len(expiring))
		}
		if len(expiring) > 0 && expiring[0].ID != sub1.ID {
			t.Error("the wrong subscription was identified as expiring within 3 days")
		}

		// Test Case 2: Find subs expiring within 12 days
		expiring, err = repo.FindExpiring(ctx, nil, 12)
		if err != nil {
			t.Fatalf("FindExpiring(12) failed: %v", err)
		}
		if len(expiring) != 2 {
			t.Errorf("expected to find 2 subscriptions expiring within 12 days, but got %d", len(expiring))
		}
	})

	t.Run("should perform aggregate queries correctly", func(t *testing.T) {
		setupPrerequisites(t)

		// Create 2 active pro subs and 1 active standard sub
		sub1 := &model.UserSubscription{ID: uuid.NewString(), UserID: user1.ID, PlanID: proPlan.ID, Status: model.SubscriptionStatusActive, RemainingCredits: 1000}
		sub2 := &model.UserSubscription{ID: uuid.NewString(), UserID: user2.ID, PlanID: proPlan.ID, Status: model.SubscriptionStatusActive, RemainingCredits: 800}
		sub3 := &model.UserSubscription{ID: uuid.NewString(), UserID: user2.ID, PlanID: stdPlan.ID, Status: model.SubscriptionStatusActive, RemainingCredits: 500}
		repo.Save(ctx, nil, sub1)
		repo.Save(ctx, nil, sub2)
		repo.Save(ctx, nil, sub3)

		// Test CountActiveByPlan
		counts, err := repo.CountActiveByPlan(ctx, nil)
		if err != nil {
			t.Fatalf("CountActiveByPlan failed: %v", err)
		}
		if len(counts) != 2 {
			t.Errorf("expected counts for 2 plans, but got %d", len(counts))
		}
		if counts[proPlan.ID] != 2 {
			t.Errorf("expected 2 active pro plans, but got %d", counts[proPlan.ID])
		}
		if counts[stdPlan.ID] != 1 {
			t.Errorf("expected 1 active standard plan, but got %d", counts[stdPlan.ID])
		}

		// Test TotalRemainingCredits
		totalCredits, err := repo.TotalRemainingCredits(ctx, nil)
		if err != nil {
			t.Fatalf("TotalRemainingCredits failed: %v", err)
		}
		expectedCredits := int64(1000 + 800 + 500)
		if totalCredits != expectedCredits {
			t.Errorf("expected total credits to be %d, but got %d", expectedCredits, totalCredits)
		}
	})
}
