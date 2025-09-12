//go:build integration

package postgres

import (
	"context"
	"reflect"
	"sort"
	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/repository"
	"testing"
)

func TestPlanRepo_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode.")
	}

	repo := NewPlanRepo(testPool)
	ctx := context.Background()
	cleanup(t)

	// This plan object will be created and then used across the sub-tests.
	plan, err := model.NewSubscriptionPlan("", "Pro Plan", 30, 10000, 50000)
	if err != nil {
		t.Fatalf("model.NewSubscriptionPlan() failed: %v", err)
	}

	t.Run("should create and read a new plan", func(t *testing.T) {
		err := repo.Save(ctx, repository.NoTX, plan)
		if err != nil {
			t.Fatalf("Failed to save new plan: %v", err)
		}

		foundPlan, err := repo.FindByID(ctx, repository.NoTX, plan.ID)
		if err != nil {
			t.Fatalf("Failed to find plan by ID: %v", err)
		}
		if foundPlan == nil {
			t.Fatal("Expected to find a plan, but got nil")
		}
		if foundPlan.Name != "Pro Plan" || foundPlan.Credits != 10000 {
			t.Errorf("Mismatch in retrieved plan data. Got name '%s' and credits %d", foundPlan.Name, foundPlan.Credits)
		}
	})

	t.Run("should update an existing plan", func(t *testing.T) {
		plan.Name = "Pro Plan v2"
		plan.PriceIRR = 60000
		err := repo.Save(ctx, repository.NoTX, plan)
		if err != nil {
			t.Fatalf("Failed to update plan: %v", err)
		}

		updatedPlan, err := repo.FindByID(ctx, repository.NoTX, plan.ID)
		if err != nil {
			t.Fatalf("Failed to find updated plan by ID: %v", err)
		}
		if updatedPlan.Name != "Pro Plan v2" || updatedPlan.PriceIRR != 60000 {
			t.Errorf("Plan was not updated correctly. Got name '%s' and price %d", updatedPlan.Name, updatedPlan.PriceIRR)
		}
	})

	t.Run("should correctly save and retrieve supported models", func(t *testing.T) {

		// Arrange: Create a plan with a list of supported models.
		planWithModels, _ := model.NewSubscriptionPlan("", "Premium Plan", 30, 99, 99)
		planWithModels.SupportedModels = []string{"gpt-4o", "gemini-1.5-pro"}
		err := repo.Save(ctx, repository.NoTX, planWithModels)
		if err != nil {
			t.Fatalf("Failed to save plan with models: %v", err)
		}

		// Act: Read the plan back from the database.
		foundPlan, err := repo.FindByID(ctx, repository.NoTX, planWithModels.ID)
		if err != nil {
			t.Fatalf("Failed to find plan by ID: %v", err)
		}
		if foundPlan == nil {
			t.Fatal("Expected to find a plan, but got nil")
		}

		// Assert: Verify that the supported models slice is correct.
		// We sort both slices to ensure the comparison is order-independent.
		sort.Strings(planWithModels.SupportedModels)
		sort.Strings(foundPlan.SupportedModels)

		if !reflect.DeepEqual(planWithModels.SupportedModels, foundPlan.SupportedModels) {
			t.Errorf("mismatch in supported models, want: %v, got: %v", planWithModels.SupportedModels, foundPlan.SupportedModels)
		}
	})

	t.Run("should list all plans", func(t *testing.T) {
		// Create a second plan to test the list functionality
		standardPlan, _ := model.NewSubscriptionPlan("", "Standard Plan", 30, 5000, 25000)
		repo.Save(ctx, repository.NoTX, standardPlan)

		allPlans, err := repo.ListAll(ctx, repository.NoTX)
		if err != nil {
			t.Fatalf("ListAll failed: %v", err)
		}
		if len(allPlans) != 3 {
			t.Errorf("expected to list 3 plans, but got %d", len(allPlans))
		}
	})

	t.Run("should delete a plan", func(t *testing.T) {
		err := repo.Delete(ctx, repository.NoTX, plan.ID)
		if err != nil {
			t.Fatalf("Delete failed: %v", err)
		}

		deletedPlan, err := repo.FindByID(ctx, repository.NoTX, plan.ID)
		if err == nil || deletedPlan != nil {
			t.Fatalf("expected error not found for deleted plan, got: %v", err)
		}

		allPlansAfterDelete, _ := repo.ListAll(ctx, repository.NoTX)
		if len(allPlansAfterDelete) != 2 {
			t.Errorf("expected to list 2 plan after deletion, but got %d", len(allPlansAfterDelete))
		}
	})
}
