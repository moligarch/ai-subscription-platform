//go:build integration

package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/model"

	"github.com/google/uuid"
)

func TestActivationCodeRepo_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode.")
	}

	// 1. Setup
	ctx := context.Background()
	repo := NewActivationCodeRepo(testPool)
	userRepo := NewUserRepo(testPool)
	planRepo := NewPlanRepo(testPool)

	// Create prerequisite data
	user, _ := model.NewUser("", 111, "code_user")
	plan, _ := model.NewSubscriptionPlan("", "Pro", 30, 0, 1)

	setupPrerequisites := func(t *testing.T) {
		cleanup(t)
		if err := userRepo.Save(ctx, nil, user); err != nil {
			t.Fatalf("failed to save user: %v", err)
		}
		if err := planRepo.Save(ctx, nil, plan); err != nil {
			t.Fatalf("failed to save plan: %v", err)
		}
	}

	t.Run("should create, find, and redeem an activation code", func(t *testing.T) {
		setupPrerequisites(t)

		// 2. Create a new code
		newCode := &model.ActivationCode{
			ID:        uuid.NewString(),
			Code:      "TESTCODE123",
			PlanID:    plan.ID,
			CreatedAt: time.Now(),
		}

		err := repo.Save(ctx, nil, newCode)
		if err != nil {
			t.Fatalf("Failed to save new activation code: %v", err)
		}

		// 3. Find the unredeemed code
		foundCode, err := repo.FindByCode(ctx, nil, "TESTCODE123")
		if err != nil {
			t.Fatalf("FindByCode failed: %v", err)
		}
		if foundCode == nil {
			t.Fatal("Expected to find the activation code, but got nil")
		}
		if foundCode.PlanID != plan.ID {
			t.Errorf("Found code with incorrect PlanID")
		}
		if foundCode.IsRedeemed {
			t.Error("Expected found code to be unredeemed")
		}

		// 4. Redeem the code
		now := time.Now()
		foundCode.IsRedeemed = true
		foundCode.RedeemedByUserID = &user.ID
		foundCode.RedeemedAt = &now

		err = repo.Save(ctx, nil, foundCode)
		if err != nil {
			t.Fatalf("Failed to update and redeem code: %v", err)
		}

		// 5. Verify the code can no longer be found (since it's redeemed)
		redeemedCode, err := repo.FindByCode(ctx, nil, "TESTCODE123")
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("FindByCode for redeemed code failed unexpectedly: %v", err)
		}
		if redeemedCode != nil {
			t.Fatal("Expected not to find a redeemed code, but it was found")
		}

		// 6. Verify the final state by querying directly (optional check)
		var isRedeemed bool
		var redeemedBy uuid.UUID
		err = testPool.QueryRow(ctx, "SELECT is_redeemed, redeemed_by_user_id FROM activation_codes WHERE id = $1", foundCode.ID).Scan(&isRedeemed, &redeemedBy)
		if err != nil {
			t.Fatalf("Direct query for redeemed code failed: %v", err)
		}
		if !isRedeemed || redeemedBy.String() != user.ID {
			t.Error("Code was not marked as redeemed correctly in the database")
		}
	})
}
