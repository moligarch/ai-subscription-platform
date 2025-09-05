//go:build integration

package postgres

import (
	"context"
	"telegram-ai-subscription/internal/domain/model"
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
		err := repo.Save(ctx, nil, plan)
		if err != nil {
			t.Fatalf("Failed to save new plan: %v", err)
		}

		foundPlan, err := repo.FindByID(ctx, nil, plan.ID)
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
		err := repo.Save(ctx, nil, plan)
		if err != nil {
			t.Fatalf("Failed to update plan: %v", err)
		}

		updatedPlan, err := repo.FindByID(ctx, nil, plan.ID)
		if err != nil {
			t.Fatalf("Failed to find updated plan by ID: %v", err)
		}
		if updatedPlan.Name != "Pro Plan v2" || updatedPlan.PriceIRR != 60000 {
			t.Errorf("Plan was not updated correctly. Got name '%s' and price %d", updatedPlan.Name, updatedPlan.PriceIRR)
		}
	})

	t.Run("should list all plans", func(t *testing.T) {
		// Create a second plan to test the list functionality
		standardPlan, _ := model.NewSubscriptionPlan("", "Standard Plan", 30, 5000, 25000)
		repo.Save(ctx, nil, standardPlan)

		allPlans, err := repo.ListAll(ctx, nil)
		if err != nil {
			t.Fatalf("ListAll failed: %v", err)
		}
		if len(allPlans) != 2 {
			t.Errorf("expected to list 2 plans, but got %d", len(allPlans))
		}
	})

	t.Run("should delete a plan", func(t *testing.T) {
		err := repo.Delete(ctx, nil, plan.ID)
		if err != nil {
			t.Fatalf("Delete failed: %v", err)
		}

		deletedPlan, err := repo.FindByID(ctx, nil, plan.ID)
		if err == nil || deletedPlan != nil {
			t.Fatalf("expected error not found for deleted plan, got: %v", err)
		}

		allPlansAfterDelete, _ := repo.ListAll(ctx, nil)
		if len(allPlansAfterDelete) != 1 {
			t.Errorf("expected to list 1 plan after deletion, but got %d", len(allPlansAfterDelete))
		}
	})
}
