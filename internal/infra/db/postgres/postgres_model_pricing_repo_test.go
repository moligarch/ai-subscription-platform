//go:build integration

package postgres

import (
	"context"
	"telegram-ai-subscription/internal/domain/model"
	"testing"
)

func TestModelPricingRepo_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode.")
	}

	repo := NewModelPricingRepo(testPool)
	ctx := context.Background()

	t.Run("should create and find model pricing", func(t *testing.T) {
		cleanup(t) // Wipes the database for this specific test case.

		newPricing := &model.ModelPricing{ModelName: "gpt-4o-mini", InputTokenPriceMicros: 15, Active: true}
		err := repo.Create(ctx, nil, newPricing)
		if err != nil {
			t.Fatalf("Failed to create new pricing: %v", err)
		}

		foundPricing, err := repo.GetByModelName(ctx, nil, "gpt-4o-mini")
		if err != nil {
			t.Fatalf("GetByModelName failed: %v", err)
		}
		if foundPricing == nil {
			t.Fatal("Expected to find pricing, but got nil")
		}
		if foundPricing.InputTokenPriceMicros != 15 {
			t.Errorf("Expected input price to be 15, got %d", foundPricing.InputTokenPriceMicros)
		}
	})

	t.Run("should update an existing pricing record", func(t *testing.T) {
		cleanup(t) // Wipes the database for this specific test case.

		pricing := &model.ModelPricing{ModelName: "gpt-4o", InputTokenPriceMicros: 100, Active: true}
		repo.Create(ctx, nil, pricing)

		pricing.InputTokenPriceMicros = 120
		err := repo.Update(ctx, nil, pricing)
		if err != nil {
			t.Fatalf("Failed to update pricing: %v", err)
		}

		updatedPricing, _ := repo.GetByModelName(ctx, nil, "gpt-4o")
		if updatedPricing == nil {
			t.Fatal("Could not find updated pricing record")
		}
		if updatedPricing.InputTokenPriceMicros != 120 {
			t.Fatal("Pricing record was not updated correctly")
		}
	})

	t.Run("should list only active models", func(t *testing.T) {
		cleanup(t) // Wipes the database for this specific test case.

		active1 := &model.ModelPricing{ModelName: "active-1", Active: true}
		active2 := &model.ModelPricing{ModelName: "active-2", Active: true}
		inactive1 := &model.ModelPricing{ModelName: "inactive-1", Active: false}
		repo.Create(ctx, nil, active1)
		repo.Create(ctx, nil, active2)
		repo.Create(ctx, nil, inactive1)

		activeModels, err := repo.ListActive(ctx, nil)
		if err != nil {
			t.Fatalf("ListActive failed: %v", err)
		}
		if len(activeModels) != 2 {
			t.Errorf("Expected to list 2 active models, but got %d", len(activeModels))
		}
	})
}
