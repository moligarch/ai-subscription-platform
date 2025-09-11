//go:build !integration

package usecase_test

import (
	"context"
	"errors"
	"testing"

	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/usecase"
)

func TestPlanUseCase(t *testing.T) {
	ctx := context.Background()
	testLogger := newTestLogger()

	t.Run("Create should save a new plan", func(t *testing.T) {
		// --- Arrange ---
		mockPlanRepo := NewMockPlanRepo()
		mockPricingRepo := NewMockModelPricingRepo()
		uc := usecase.NewPlanUseCase(mockPlanRepo, mockPricingRepo, testLogger)

		name := "Pro Plan"
		duration := 30
		credits := int64(100000)
		price := int64(50000)

		// --- Act ---
		createdPlan, err := uc.Create(ctx, name, duration, credits, price)

		// --- Assert ---
		if err != nil {
			t.Fatalf("expected no error, but got: %v", err)
		}
		if createdPlan == nil {
			t.Fatal("expected a plan, but got nil")
		}
		if createdPlan.ID == "" {
			t.Error("expected new plan to have an ID")
		}

		// Verify the plan was saved correctly in the mock repo
		savedPlan, _ := mockPlanRepo.FindByID(ctx, nil, createdPlan.ID)
		if savedPlan == nil {
			t.Fatal("plan was not found in the repository after creation")
		}
		if savedPlan.Name != name {
			t.Errorf("expected saved plan name to be '%s', but got '%s'", name, savedPlan.Name)
		}
	})

	t.Run("Update should save changes to an existing plan", func(t *testing.T) {
		// --- Arrange ---
		mockPlanRepo := NewMockPlanRepo()
		mockPricingRepo := NewMockModelPricingRepo()
		uc := usecase.NewPlanUseCase(mockPlanRepo, mockPricingRepo, testLogger)

		// Seed the repo with an existing plan
		existingPlan := &model.SubscriptionPlan{
			ID:           "plan-123",
			Name:         "Old Name",
			DurationDays: 30,
		}
		mockPlanRepo.Save(ctx, nil, existingPlan)

		// Modify the plan
		existingPlan.Name = "New Name"
		existingPlan.DurationDays = 45

		// --- Act ---
		err := uc.Update(ctx, existingPlan)

		// --- Assert ---
		if err != nil {
			t.Fatalf("expected no error, but got: %v", err)
		}

		updatedPlan, _ := mockPlanRepo.FindByID(ctx, nil, "plan-123")
		if updatedPlan.Name != "New Name" {
			t.Errorf("expected plan name to be updated to 'New Name', but got '%s'", updatedPlan.Name)
		}
		if updatedPlan.DurationDays != 45 {
			t.Errorf("expected duration to be updated to 45, but got '%d'", updatedPlan.DurationDays)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		t.Run("should succeed for an unused plan", func(t *testing.T) {
			// --- Arrange ---
			mockPlanRepo := NewMockPlanRepo()
			mockPricingRepo := NewMockModelPricingRepo()
			uc := usecase.NewPlanUseCase(mockPlanRepo, mockPricingRepo, testLogger)

			planToDelete := &model.SubscriptionPlan{ID: "plan-to-delete"}
			mockPlanRepo.Save(ctx, nil, planToDelete)

			// --- Act ---
			err := uc.Delete(ctx, "plan-to-delete")

			// --- Assert ---
			if err != nil {
				t.Fatalf("expected no error, but got: %v", err)
			}

			deletedPlan, _ := mockPlanRepo.FindByID(ctx, nil, "plan-to-delete")
			if deletedPlan != nil {
				t.Error("expected plan to be deleted, but it was still found")
			}
		})

		t.Run("should fail for a plan with active subscriptions", func(t *testing.T) {
			// --- Arrange ---
			mockPlanRepo := NewMockPlanRepo()
			// For this specific case, we override the mock's behavior to simulate the error
			mockPlanRepo.DeleteFunc = func(ctx context.Context, id string) error {
				return domain.ErrSubsciptionWithActiveUser
			}
			// --- Arrange ---
			mockPricingRepo := NewMockModelPricingRepo()
			uc := usecase.NewPlanUseCase(mockPlanRepo, mockPricingRepo, testLogger)

			// --- Act ---
			err := uc.Delete(ctx, "plan-in-use")

			// --- Assert ---
			if err == nil {
				t.Fatal("expected an error, but got nil")
			}
			if !errors.Is(err, domain.ErrSubsciptionWithActiveUser) {
				t.Errorf("expected error to be ErrSubsciptionWithActiveUser, but got %T", err)
			}
		})
	})

	t.Run("Get and List should retrieve plans correctly", func(t *testing.T) {
		// --- Arrange ---
		mockPlanRepo := NewMockPlanRepo()
		mockPricingRepo := NewMockModelPricingRepo()
		uc := usecase.NewPlanUseCase(mockPlanRepo, mockPricingRepo, testLogger)

		plan1 := &model.SubscriptionPlan{ID: "plan-1", PriceIRR: 100}
		plan2 := &model.SubscriptionPlan{ID: "plan-2", PriceIRR: 200}
		mockPlanRepo.Save(ctx, nil, plan1)
		mockPlanRepo.Save(ctx, nil, plan2)

		// --- Act ---
		singlePlan, errGet := uc.Get(ctx, "plan-1")
		allPlans, errList := uc.List(ctx)

		// --- Assert ---
		if errGet != nil {
			t.Fatalf("Get failed: %v", errGet)
		}
		if singlePlan == nil || singlePlan.ID != "plan-1" {
			t.Errorf("Get did not retrieve the correct plan")
		}

		if errList != nil {
			t.Fatalf("List failed: %v", errList)
		}
		if len(allPlans) != 2 {
			t.Errorf("expected List to return 2 plans, but got %d", len(allPlans))
		}
	})
}
