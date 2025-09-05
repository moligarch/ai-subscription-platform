//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"telegram-ai-subscription/internal/domain/model"

	"github.com/google/uuid"
)

func TestPurchaseRepo_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode.")
	}

	// 1. Setup
	ctx := context.Background()
	repo := NewPurchaseRepo(testPool)
	userRepo := NewUserRepo(testPool)
	planRepo := NewPlanRepo(testPool)
	paymentRepo := NewPaymentRepo(testPool)

	// Create prerequisite data
	user1, _ := model.NewUser("", 111, "user1")
	user2, _ := model.NewUser("", 222, "user2")
	plan, _ := model.NewSubscriptionPlan("", "Pro", 30, 0, 1)
	payment1 := &model.Payment{ID: uuid.NewString(), UserID: user1.ID, PlanID: plan.ID, Status: model.PaymentStatusSucceeded}
	payment2 := &model.Payment{ID: uuid.NewString(), UserID: user1.ID, PlanID: plan.ID, Status: model.PaymentStatusSucceeded}
	payment3 := &model.Payment{ID: uuid.NewString(), UserID: user2.ID, PlanID: plan.ID, Status: model.PaymentStatusSucceeded}

	// Helper to set up a clean state with prerequisites
	setupPrerequisites := func(t *testing.T) {
		cleanup(t)
		// We must save prerequisites in the correct order due to foreign keys
		if err := userRepo.Save(ctx, nil, user1); err != nil {
			t.Fatalf("failed to save user1: %v", err)
		}
		if err := userRepo.Save(ctx, nil, user2); err != nil {
			t.Fatalf("failed to save user2: %v", err)
		}
		if err := planRepo.Save(ctx, nil, plan); err != nil {
			t.Fatalf("failed to save plan: %v", err)
		}
		if err := paymentRepo.Save(ctx, nil, payment1); err != nil {
			t.Fatalf("failed to save payment1: %v", err)
		}
		if err := paymentRepo.Save(ctx, nil, payment2); err != nil {
			t.Fatalf("failed to save payment2: %v", err)
		}
		if err := paymentRepo.Save(ctx, nil, payment3); err != nil {
			t.Fatalf("failed to save payment3: %v", err)
		}
	}

	t.Run("should save and list purchases for users", func(t *testing.T) {
		setupPrerequisites(t)

		// Create 2 purchases for user1 and 1 for user2
		purchase1 := &model.Purchase{ID: uuid.NewString(), UserID: user1.ID, PlanID: plan.ID, PaymentID: payment1.ID, SubscriptionID: uuid.NewString(), CreatedAt: time.Now()}
		purchase2 := &model.Purchase{ID: uuid.NewString(), UserID: user1.ID, PlanID: plan.ID, PaymentID: payment2.ID, SubscriptionID: uuid.NewString(), CreatedAt: time.Now()}
		purchase3 := &model.Purchase{ID: uuid.NewString(), UserID: user2.ID, PlanID: plan.ID, PaymentID: payment3.ID, SubscriptionID: uuid.NewString(), CreatedAt: time.Now()}

		if err := repo.Save(ctx, nil, purchase1); err != nil {
			t.Fatalf("failed to save purchase1: %v", err)
		}
		if err := repo.Save(ctx, nil, purchase2); err != nil {
			t.Fatalf("failed to save purchase2: %v", err)
		}
		if err := repo.Save(ctx, nil, purchase3); err != nil {
			t.Fatalf("failed to save purchase3: %v", err)
		}

		// List purchases for user1
		user1Purchases, err := repo.ListByUser(ctx, nil, user1.ID)
		if err != nil {
			t.Fatalf("ListByUser for user1 failed: %v", err)
		}
		if len(user1Purchases) != 2 {
			t.Errorf("expected 2 purchases for user1, but got %d", len(user1Purchases))
		}

		// List purchases for user2
		user2Purchases, err := repo.ListByUser(ctx, nil, user2.ID)
		if err != nil {
			t.Fatalf("ListByUser for user2 failed: %v", err)
		}
		if len(user2Purchases) != 1 {
			t.Errorf("expected 1 purchase for user2, but got %d", len(user2Purchases))
		}
	})

	t.Run("should fail to save a duplicate purchase for the same payment", func(t *testing.T) {
		setupPrerequisites(t)

		// Create an initial, valid purchase
		purchase1 := &model.Purchase{ID: uuid.NewString(), UserID: user1.ID, PlanID: plan.ID, PaymentID: payment1.ID, SubscriptionID: uuid.NewString()}
		err := repo.Save(ctx, nil, purchase1)
		if err != nil {
			t.Fatalf("failed to save initial purchase: %v", err)
		}

		// Create a second purchase with the SAME payment ID
		duplicatePurchase := &model.Purchase{ID: uuid.NewString(), UserID: user1.ID, PlanID: plan.ID, PaymentID: payment1.ID, SubscriptionID: uuid.NewString()}
		err = repo.Save(ctx, nil, duplicatePurchase)

		// This should fail because of the UNIQUE constraint on payment_id
		if err == nil {
			t.Fatal("expected an error when saving a duplicate purchase, but got nil")
		}
	})
}
