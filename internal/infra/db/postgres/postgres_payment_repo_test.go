//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"telegram-ai-subscription/internal/domain/model"

	"github.com/google/uuid"
)

func TestPaymentRepo_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode.")
	}

	// 1. Setup
	ctx := context.Background()
	repo := NewPaymentRepo(testPool)
	userRepo := NewUserRepo(testPool)
	planRepo := NewPlanRepo(testPool)

	// Create prerequisite data
	user, _ := model.NewUser("", 111, "user1")
	plan, _ := model.NewSubscriptionPlan("", "Pro", 30, 0, 1)

	// Helper to set up a clean state with prerequisites
	setupPrerequisites := func(t *testing.T) {
		cleanup(t)
		if err := userRepo.Save(ctx, nil, user); err != nil {
			t.Fatalf("failed to save user: %v", err)
		}
		if err := planRepo.Save(ctx, nil, plan); err != nil {
			t.Fatalf("failed to save plan: %v", err)
		}
	}

	t.Run("should save and find a payment", func(t *testing.T) {
		setupPrerequisites(t)

		newPayment := &model.Payment{
			ID:        uuid.NewString(),
			UserID:    user.ID,
			PlanID:    plan.ID,
			Provider:  "test",
			Amount:    50000,
			Currency:  "IRR",
			Authority: "auth-123",
			Status:    model.PaymentStatusInitiated,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		// Test Create
		err := repo.Save(ctx, nil, newPayment)
		if err != nil {
			t.Fatalf("Failed to save new payment: %v", err)
		}

		// Test FindByID
		foundByID, err := repo.FindByID(ctx, nil, newPayment.ID)
		if err != nil {
			t.Fatalf("FindByID failed: %v", err)
		}
		if foundByID == nil || foundByID.Authority != "auth-123" {
			t.Fatal("Did not find the correct payment by ID")
		}

		// Test FindByAuthority
		foundByAuth, err := repo.FindByAuthority(ctx, nil, "auth-123")
		if err != nil {
			t.Fatalf("FindByAuthority failed: %v", err)
		}
		if foundByAuth == nil || foundByAuth.ID != newPayment.ID {
			t.Fatal("Did not find the correct payment by Authority")
		}
	})

	t.Run("should correctly update status", func(t *testing.T) {
		setupPrerequisites(t)

		refID := "ref-abc"
		paidAt := time.Now().Truncate(time.Millisecond) // Truncate for reliable comparison
		payment := &model.Payment{ID: uuid.NewString(), UserID: user.ID, PlanID: plan.ID, Status: model.PaymentStatusPending}
		repo.Save(ctx, nil, payment)

		// Test UpdateStatus
		err := repo.UpdateStatus(ctx, nil, payment.ID, model.PaymentStatusSucceeded, &refID, &paidAt)
		if err != nil {
			t.Fatalf("UpdateStatus failed: %v", err)
		}

		updatedPayment, _ := repo.FindByID(ctx, nil, payment.ID)
		if updatedPayment.Status != model.PaymentStatusSucceeded {
			t.Errorf("expected status to be 'succeeded', but got '%s'", updatedPayment.Status)
		}
		if updatedPayment.RefID == nil || *updatedPayment.RefID != refID {
			t.Error("RefID was not updated correctly")
		}
		if updatedPayment.PaidAt == nil || !updatedPayment.PaidAt.Equal(paidAt) {
			t.Errorf("PaidAt was not updated correctly, expected %v got %v", paidAt, updatedPayment.PaidAt)
		}
	})

	t.Run("should correctly update status only if pending", func(t *testing.T) {
		setupPrerequisites(t)
		payment := &model.Payment{ID: uuid.NewString(), UserID: user.ID, PlanID: plan.ID, Status: model.PaymentStatusPending}
		repo.Save(ctx, nil, payment)

		// First update should succeed
		updated, err := repo.UpdateStatusIfPending(ctx, nil, payment.ID, model.PaymentStatusSucceeded, nil, nil)
		if err != nil {
			t.Fatalf("First UpdateStatusIfPending failed: %v", err)
		}
		if !updated {
			t.Error("expected first update to succeed, but it returned false")
		}

		// Second update on the same (now succeeded) payment should fail
		updatedAgain, err := repo.UpdateStatusIfPending(ctx, nil, payment.ID, model.PaymentStatusFailed, nil, nil)
		if err != nil {
			t.Fatalf("Second UpdateStatusIfPending failed: %v", err)
		}
		if updatedAgain {
			t.Error("expected second update to fail, but it returned true")
		}

		finalPayment, _ := repo.FindByID(ctx, nil, payment.ID)
		if finalPayment.Status != model.PaymentStatusSucceeded {
			t.Errorf("expected final status to be 'succeeded', but got '%s'", finalPayment.Status)
		}
	})

	t.Run("should list pending payments older than a cutoff", func(t *testing.T) {
		setupPrerequisites(t)

		// 1. Pending and old, should be found
		p1 := &model.Payment{ID: uuid.NewString(), UserID: user.ID, PlanID: plan.ID, Status: model.PaymentStatusPending, CreatedAt: time.Now().Add(-2 * time.Hour)}
		// 2. Pending but recent, should NOT be found
		p2 := &model.Payment{ID: uuid.NewString(), UserID: user.ID, PlanID: plan.ID, Status: model.PaymentStatusPending, CreatedAt: time.Now().Add(-5 * time.Minute)}
		// 3. Old but succeeded, should NOT be found
		p3 := &model.Payment{ID: uuid.NewString(), UserID: user.ID, PlanID: plan.ID, Status: model.PaymentStatusSucceeded, CreatedAt: time.Now().Add(-2 * time.Hour)}

		repo.Save(ctx, nil, p1)
		repo.Save(ctx, nil, p2)
		repo.Save(ctx, nil, p3)

		// Find pending payments older than 1 hour ago
		cutoff := time.Now().Add(-1 * time.Hour)
		results, err := repo.ListPendingOlderThan(ctx, nil, cutoff, 10)
		if err != nil {
			t.Fatalf("ListPendingOlderThan failed: %v", err)
		}
		if len(results) != 1 {
			t.Errorf("expected to find 1 pending payment, but got %d", len(results))
		}
		if len(results) == 1 && results[0].ID != p1.ID {
			t.Error("found the wrong pending payment")
		}
	})
}
