//go:build integration

package postgres

import (
	"context"
	"testing"

	"telegram-ai-subscription/internal/domain/model"

	"github.com/google/uuid"
)

func TestNotificationLogRepo_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode.")
	}

	// 1. Setup
	ctx := context.Background()
	repo := NewNotificationLogRepo(testPool)
	userRepo := NewUserRepo(testPool)
	planRepo := NewPlanRepo(testPool)
	subRepo := NewSubscriptionRepo(testPool)

	// Create prerequisite data
	user, _ := model.NewUser("", 111, "notif_user")
	plan, _ := model.NewSubscriptionPlan("", "Pro", 30, 0, 1)
	sub := &model.UserSubscription{
		ID:     uuid.NewString(),
		UserID: user.ID,
		PlanID: plan.ID,
		Status: model.SubscriptionStatusActive,
	}
	// Helper to set up a clean state
	setupPrerequisites := func(t *testing.T) {
		cleanup(t)
		if err := userRepo.Save(ctx, nil, user); err != nil {
			t.Fatalf("failed to save user: %v", err)
		}
		if err := planRepo.Save(ctx, nil, plan); err != nil {
			t.Fatalf("failed to save plan: %v", err)
		}
		if err := subRepo.Save(ctx, nil, sub); err != nil {
			t.Fatalf("failed to save subscription: %v", err)
		}
	}

	t.Run("should save and check for notification existence", func(t *testing.T) {
		setupPrerequisites(t)

		// 1. Check for a notification that doesn't exist yet
		exists, err := repo.Exists(ctx, nil, sub.ID, "expiry", 3)
		if err != nil {
			t.Fatalf("Exists check failed unexpectedly: %v", err)
		}
		if exists {
			t.Fatal("expected notification to not exist, but it was found")
		}

		// 2. Save the notification log
		err = repo.Save(ctx, nil, sub.ID, user.ID, "expiry", 3)
		if err != nil {
			t.Fatalf("Save failed: %v", err)
		}

		// 3. Check again; it should now exist
		exists, err = repo.Exists(ctx, nil, sub.ID, "expiry", 3)
		if err != nil {
			t.Fatalf("Second Exists check failed unexpectedly: %v", err)
		}
		if !exists {
			t.Fatal("expected notification to exist after saving, but it was not found")
		}

		// 4. Check for a different threshold; it should not exist
		exists, err = repo.Exists(ctx, nil, sub.ID, "expiry", 7)
		if err != nil {
			t.Fatalf("Third Exists check failed unexpectedly: %v", err)
		}
		if exists {
			t.Fatal("found notification for wrong threshold")
		}
	})

	t.Run("should fail to save a duplicate notification", func(t *testing.T) {
		setupPrerequisites(t)
		// Save the notification once, which should succeed.
		err := repo.Save(ctx, nil, sub.ID, user.ID, "expiry", 1)
		if err != nil {
			t.Fatalf("First Save failed unexpectedly: %v", err)
		}

		// Try to save the exact same notification again.
		err = repo.Save(ctx, nil, sub.ID, user.ID, "expiry", 1)
		// This should fail due to the UNIQUE constraint in the database.
		if err == nil {
			t.Fatal("expected an error when saving a duplicate notification, but got nil")
		}
	})
}
