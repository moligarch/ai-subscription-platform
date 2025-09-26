//go:build !integration

package usecase_test

import (
	"context"
	"errors"
	"testing"

	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/repository"
	"telegram-ai-subscription/internal/usecase"

	"github.com/google/uuid"
)

func TestPlanUseCase(t *testing.T) {
	ctx := context.Background()
	testLogger := newTestLogger()

	t.Run("Create should save a new plan with supported models", func(t *testing.T) {
		// --- Arrange ---
		mockPlanRepo := NewMockPlanRepo()
		mockPricingRepo := NewMockModelPricingRepo()
		mockCodeRepo := NewMockActivationCodeRepo()
		uc := usecase.NewPlanUseCase(mockPlanRepo, mockPricingRepo, mockCodeRepo, testLogger)

		var savedPlan *model.SubscriptionPlan
		mockPlanRepo.SaveFunc = func(ctx context.Context, tx repository.Tx, p *model.SubscriptionPlan) error {
			savedPlan = p // Capture the plan passed to the repository
			return nil
		}
		name := "Pro Plan"
		duration := 30
		credits := int64(100000)
		price := int64(50000)
		supportedModels := []string{"gpt-4o", "gemini-1.5-pro"}

		// --- Act ---
		_, err := uc.Create(ctx, name, duration, credits, price, supportedModels)

		// --- Assert ---
		if err != nil {
			t.Fatalf("expected no error, but got: %v", err)
		}
		if savedPlan == nil {
			t.Fatal("expected a plan to be saved, but it wasn't")
		}
		if savedPlan.Name != name {
			t.Errorf("expected saved plan name to be '%s', but got '%s'", name, savedPlan.Name)
		}
		// Use a helper to compare slices since order doesn't matter
		if !equalSlices(savedPlan.SupportedModels, supportedModels) {
			t.Errorf("mismatch in supported models, want: %v, got: %v", supportedModels, savedPlan.SupportedModels)
		}
	})

	t.Run("Update should save changes to an existing plan", func(t *testing.T) {
		// --- Arrange ---
		mockPlanRepo := NewMockPlanRepo()
		mockPricingRepo := NewMockModelPricingRepo()
		mockCodeRepo := NewMockActivationCodeRepo()
		uc := usecase.NewPlanUseCase(mockPlanRepo, mockPricingRepo, mockCodeRepo, testLogger)

		// Seed the repo with an existing plan
		existingPlan := &model.SubscriptionPlan{
			ID:           uuid.NewString(),
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

		updatedPlan, _ := mockPlanRepo.FindByID(ctx, nil, existingPlan.ID)
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
			mockCodeRepo := NewMockActivationCodeRepo()
			uc := usecase.NewPlanUseCase(mockPlanRepo, mockPricingRepo, mockCodeRepo, testLogger)
			idToDelete := uuid.NewString()
			planToDelete := &model.SubscriptionPlan{ID: idToDelete}
			mockPlanRepo.Save(ctx, nil, planToDelete)

			// --- Act ---
			err := uc.Delete(ctx, idToDelete)

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
			mockPlanRepo.DeleteFunc = func(ctx context.Context, tx repository.Tx, id string) error {
				return domain.ErrSubsciptionWithActiveUser
			}
			// --- Arrange ---
			mockPricingRepo := NewMockModelPricingRepo()
			mockCodeRepo := NewMockActivationCodeRepo()
			uc := usecase.NewPlanUseCase(mockPlanRepo, mockPricingRepo, mockCodeRepo, testLogger)

			// --- Act ---
			err := uc.Delete(ctx, uuid.NewString())

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
		mockCodeRepo := NewMockActivationCodeRepo()
		uc := usecase.NewPlanUseCase(mockPlanRepo, mockPricingRepo, mockCodeRepo, testLogger)

		id1 := uuid.NewString()
		id2 := uuid.NewString()
		// Seed the repo with some plans
		plan1 := &model.SubscriptionPlan{ID: id1, PriceIRR: 100}
		plan2 := &model.SubscriptionPlan{ID: id2, PriceIRR: 200}
		mockPlanRepo.Save(ctx, nil, plan1)
		mockPlanRepo.Save(ctx, nil, plan2)

		// --- Act ---
		singlePlan, errGet := uc.Get(ctx, id1)
		allPlans, errList := uc.List(ctx)

		// --- Assert ---
		if errGet != nil {
			t.Fatalf("Get failed: %v", errGet)
		}
		if singlePlan == nil || singlePlan.ID != id1 {
			t.Errorf("Get did not retrieve the correct plan")
		}

		if errList != nil {
			t.Fatalf("List failed: %v", errList)
		}
		if len(allPlans) != 2 {
			t.Errorf("expected List to return 2 plans, but got %d", len(allPlans))
		}
	})

	t.Run("UpdatePricing should modify an existing model's prices", func(t *testing.T) {
		// Arrange
		mockPlanRepo := NewMockPlanRepo()
		mockPricingRepo := NewMockModelPricingRepo()
		mockCodeRepo := NewMockActivationCodeRepo()
		uc := usecase.NewPlanUseCase(mockPlanRepo, mockPricingRepo, mockCodeRepo, testLogger)

		existingPricing := &model.ModelPricing{ModelName: "gpt-4o", InputTokenPriceMicros: 100, OutputTokenPriceMicros: 200}
		mockPricingRepo.Seed(existingPricing) // Seed the mock with our model

		var updatedPricing *model.ModelPricing
		mockPricingRepo.UpdateFunc = func(ctx context.Context, p *model.ModelPricing) error {
			updatedPricing = p // Capture the updated model for assertion
			return nil
		}

		// Act
		err := uc.UpdatePricing(ctx, "gpt-4o", 150, 300)

		// Assert
		if err != nil {
			t.Fatalf("expected no error, but got: %v", err)
		}
		if updatedPricing == nil {
			t.Fatal("expected pricing repo Update to be called, but it wasn't")
		}
		if updatedPricing.InputTokenPriceMicros != 150 {
			t.Errorf("expected input price to be 150, but got %d", updatedPricing.InputTokenPriceMicros)
		}
		if updatedPricing.OutputTokenPriceMicros != 300 {
			t.Errorf("expected output price to be 300, but got %d", updatedPricing.OutputTokenPriceMicros)
		}
	})
}

func TestPlanUseCase_GenerateActivationCodes(t *testing.T) {
	ctx := context.Background()
	testLogger := newTestLogger()

	t.Run("should generate the correct number of codes for a valid plan", func(t *testing.T) {
		// --- Arrange ---
		mockPlanRepo := NewMockPlanRepo()
		mockPricingRepo := NewMockModelPricingRepo()
		mockCodeRepo := NewMockActivationCodeRepo()

		// Simulate finding a valid plan
		plan := &model.SubscriptionPlan{ID: "plan-123"}
		mockPlanRepo.FindByIDFunc = func(ctx context.Context, tx repository.Tx, id string) (*model.SubscriptionPlan, error) {
			return plan, nil
		}

		var savedCodes []*model.ActivationCode
		mockCodeRepo.SaveFunc = func(ctx context.Context, tx repository.Tx, code *model.ActivationCode) error {
			savedCodes = append(savedCodes, code)
			return nil
		}

		uc := usecase.NewPlanUseCase(mockPlanRepo, mockPricingRepo, mockCodeRepo, testLogger)

		// --- Act ---
		generated, err := uc.GenerateActivationCodes(ctx, "plan-123", 5)

		// --- Assert ---
		if err != nil {
			t.Fatalf("expected no error, but got %v", err)
		}
		if len(generated) != 5 {
			t.Errorf("expected 5 codes to be generated, but got %d", len(generated))
		}
		if len(savedCodes) != 5 {
			t.Errorf("expected 5 codes to be saved, but got %d", len(savedCodes))
		}
		if savedCodes[0].PlanID != "plan-123" {
			t.Error("generated codes are not linked to the correct plan ID")
		}
	})
}
