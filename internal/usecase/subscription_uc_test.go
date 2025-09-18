//go:build !integration

package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/repository"
	"telegram-ai-subscription/internal/usecase"
)

func TestSubscriptionUseCase_Subscribe(t *testing.T) {
	ctx := context.Background()
	testLogger := newTestLogger()
	mockTxManager := NewMockTxManager()

	// Shared plan for tests
	plan := &model.SubscriptionPlan{
		ID:           "plan-pro",
		Name:         "Pro",
		DurationDays: 30,
		Credits:      1000,
	}

	t.Run("should create an active subscription for a user with no existing subs", func(t *testing.T) {
		// --- Arrange ---
		mockSubRepo := NewMockSubscriptionRepo()
		mockPlanRepo := NewMockPlanRepo()
		mockCodeRepo := NewMockActivationCodeRepo() // <-- Add new mock
		mockPlanRepo.Save(ctx, nil, plan)

		var savedSub *model.UserSubscription
		mockSubRepo.SaveFunc = func(ctx context.Context, tx repository.Tx, s *model.UserSubscription) error {
			savedSub = s
			return nil
		}
		// Simulate no active subscription found
		mockSubRepo.FindActiveByUserFunc = func(ctx context.Context, tx repository.Tx, userID string) (*model.UserSubscription, error) {
			return nil, domain.ErrNotFound
		}

		uc := usecase.NewSubscriptionUseCase(mockSubRepo, mockPlanRepo, mockCodeRepo, mockTxManager, testLogger) // <-- Update constructor

		// --- Act ---
		_, err := uc.Subscribe(ctx, "user-123", "plan-pro")

		// --- Assert ---
		if err != nil {
			t.Fatalf("expected no error, but got: %v", err)
		}
		if savedSub == nil {
			t.Fatal("expected a subscription to be saved, but it wasn't")
		}
		if savedSub.Status != model.SubscriptionStatusActive {
			t.Errorf("expected new subscription to be 'active', but got '%s'", savedSub.Status)
		}
		if savedSub.StartAt == nil || savedSub.ExpiresAt == nil {
			t.Error("expected new active subscription to have StartAt and ExpiresAt times")
		}
	})

	t.Run("should create a reserved subscription for a user with an active sub", func(t *testing.T) {
		// --- Arrange ---
		mockSubRepo := NewMockSubscriptionRepo()
		mockPlanRepo := NewMockPlanRepo()
		mockCodeRepo := NewMockActivationCodeRepo() // <-- Add new mock
		mockPlanRepo.Save(ctx, nil, plan)

		var savedSub *model.UserSubscription
		mockSubRepo.SaveFunc = func(ctx context.Context, tx repository.Tx, s *model.UserSubscription) error {
			savedSub = s
			return nil
		}

		// Simulate an existing active subscription
		expiresAt := time.Now().Add(10 * 24 * time.Hour)
		activeSub := &model.UserSubscription{ID: "sub-abc", UserID: "user-123", Status: model.SubscriptionStatusActive, ExpiresAt: &expiresAt}
		mockSubRepo.FindActiveByUserFunc = func(ctx context.Context, tx repository.Tx, userID string) (*model.UserSubscription, error) {
			return activeSub, nil
		}

		uc := usecase.NewSubscriptionUseCase(mockSubRepo, mockPlanRepo, mockCodeRepo, mockTxManager, testLogger) // <-- Update constructor

		// --- Act ---
		_, err := uc.Subscribe(ctx, "user-123", "plan-pro")

		// --- Assert ---
		if err != nil {
			t.Fatalf("expected no error, but got: %v", err)
		}
		if savedSub == nil {
			t.Fatal("expected a subscription to be saved, but it wasn't")
		}
		if savedSub.Status != model.SubscriptionStatusReserved {
			t.Errorf("expected new subscription to be 'reserved', but got '%s'", savedSub.Status)
		}
		if savedSub.ScheduledStartAt == nil || *savedSub.ScheduledStartAt != expiresAt {
			t.Error("expected ScheduledStartAt to match the previous subscription's expiration")
		}
	})
}

func TestSubscriptionUseCase_DeductCredits(t *testing.T) {
	ctx := context.Background()
	testLogger := newTestLogger()
	mockTxManager := NewMockTxManager()

	t.Run("should deduct credits from an active subscription", func(t *testing.T) {
		// --- Arrange ---
		mockSubRepo := NewMockSubscriptionRepo()
		mockCodeRepo := NewMockActivationCodeRepo()
		uc := usecase.NewSubscriptionUseCase(mockSubRepo, nil, mockCodeRepo, mockTxManager, testLogger)

		activeSub := &model.UserSubscription{ID: "sub-1", UserID: "user-1", Status: model.SubscriptionStatusActive, RemainingCredits: 1000}
		mockSubRepo.Save(ctx, nil, activeSub)

		var savedSub *model.UserSubscription
		mockSubRepo.SaveFunc = func(ctx context.Context, tx repository.Tx, s *model.UserSubscription) error {
			savedSub = s
			return nil
		}

		// --- Act ---
		_, err := uc.DeductCredits(ctx, "user-1", 100)

		// --- Assert ---
		if err != nil {
			t.Fatalf("expected no error, but got: %v", err)
		}
		if savedSub == nil {
			t.Fatal("expected subscription to be saved, but it wasn't")
		}
		if savedSub.RemainingCredits != 900 {
			t.Errorf("expected remaining credits to be 900, but got %d", savedSub.RemainingCredits)
		}
		if savedSub.Status != model.SubscriptionStatusActive {
			t.Errorf("expected status to remain 'active', but got '%s'", savedSub.Status)
		}
	})

	t.Run("should finish subscription if exact credits are deducted", func(t *testing.T) {
		// --- Arrange ---
		mockSubRepo := NewMockSubscriptionRepo()
		mockCodeRepo := NewMockActivationCodeRepo()
		uc := usecase.NewSubscriptionUseCase(mockSubRepo, nil, mockCodeRepo, mockTxManager, testLogger)

		activeSub := &model.UserSubscription{ID: "sub-1", UserID: "user-1", Status: model.SubscriptionStatusActive, RemainingCredits: 100}
		mockSubRepo.Save(ctx, nil, activeSub)

		var savedSub *model.UserSubscription
		mockSubRepo.SaveFunc = func(ctx context.Context, tx repository.Tx, s *model.UserSubscription) error {
			savedSub = s
			return nil
		}

		// --- Act ---
		_, err := uc.DeductCredits(ctx, "user-1", 100)

		// --- Assert ---
		if err != nil {
			t.Fatalf("expected no error, but got: %v", err)
		}
		if savedSub.RemainingCredits != 0 {
			t.Errorf("expected remaining credits to be 0, but got %d", savedSub.RemainingCredits)
		}
		if savedSub.Status != model.SubscriptionStatusFinished {
			t.Errorf("expected status to become 'finished', but got '%s'", savedSub.Status)
		}
	})

	t.Run("should return error if no active subscription is found", func(t *testing.T) {
		// --- Arrange ---
		mockSubRepo := NewMockSubscriptionRepo()
		// Simulate repo returning ErrNotFound
		mockSubRepo.FindActiveByUserFunc = func(ctx context.Context, tx repository.Tx, userID string) (*model.UserSubscription, error) {
			return nil, domain.ErrNotFound
		}
		mockCodeRepo := NewMockActivationCodeRepo()
		uc := usecase.NewSubscriptionUseCase(mockSubRepo, nil, mockCodeRepo, mockTxManager, testLogger)

		// --- Act ---
		_, err := uc.DeductCredits(ctx, "user-1", 100)

		// --- Assert ---
		if err == nil {
			t.Fatal("expected an error, but got nil")
		}
		if !errors.Is(err, domain.ErrNoActiveSubscription) {
			t.Errorf("expected error to be ErrNoActiveSubscription, but got %T", err)
		}
	})
}

func TestSubscriptionUseCase_FinishExpired(t *testing.T) {
	ctx := context.Background()
	testLogger := newTestLogger()
	mockTxManager := NewMockTxManager()

	t.Run("should transition expired active subscriptions to finished", func(t *testing.T) {
		// --- Arrange ---
		mockSubRepo := NewMockSubscriptionRepo()

		now := time.Now()
		// This is the only subscription that the use case should process.
		expiredSub := &model.UserSubscription{ID: "sub-expired", Status: model.SubscriptionStatusActive, ExpiresAt: &now}

		// Configure the mock's FindExpiring method to return only the expired subscription.
		// The other variables (activeSub, reservedSub) were unnecessary.
		mockSubRepo.FindExpiringFunc = func(ctx context.Context, tx repository.Tx, withinDays int) ([]*model.UserSubscription, error) {
			return []*model.UserSubscription{expiredSub}, nil
		}

		var savedSubs []*model.UserSubscription
		mockSubRepo.SaveFunc = func(ctx context.Context, tx repository.Tx, s *model.UserSubscription) error {
			savedSubs = append(savedSubs, s)
			return nil
		}
		mockCodeRepo := NewMockActivationCodeRepo()
		uc := usecase.NewSubscriptionUseCase(mockSubRepo, nil, mockCodeRepo, mockTxManager, testLogger)

		// --- Act ---
		count, err := uc.FinishExpired(ctx)

		// --- Assert ---
		if err != nil {
			t.Fatalf("expected no error, but got: %v", err)
		}
		if count != 1 {
			t.Errorf("expected count of expired subscriptions to be 1, but got %d", count)
		}
		if len(savedSubs) != 1 {
			t.Fatalf("expected Save to be called once, but it was called %d times", len(savedSubs))
		}
		if savedSubs[0].ID != "sub-expired" {
			t.Error("the wrong subscription was saved")
		}
		if savedSubs[0].Status != model.SubscriptionStatusFinished {
			t.Errorf("expected expired subscription status to be 'finished', but got '%s'", savedSubs[0].Status)
		}
	})
}

func TestSubscriptionUseCase_RedeemActivationCode(t *testing.T) {
	ctx := context.Background()
	testLogger := newTestLogger()
	mockTxManager := NewMockTxManager()

	t.Run("should redeem a valid code and grant a subscription", func(t *testing.T) {
		// --- Arrange ---
		mockSubRepo := NewMockSubscriptionRepo()
		mockPlanRepo := NewMockPlanRepo()
		mockCodeRepo := NewMockActivationCodeRepo()

		// Simulate finding a valid, unredeemed code
		code := &model.ActivationCode{ID: "code-1", Code: "VALID-CODE", PlanID: "plan-1"}
		mockCodeRepo.FindByCodeFunc = func(ctx context.Context, tx repository.Tx, c string) (*model.ActivationCode, error) {
			return code, nil
		}

		var savedCode *model.ActivationCode
		mockCodeRepo.SaveFunc = func(ctx context.Context, tx repository.Tx, c *model.ActivationCode) error {
			savedCode = c
			return nil
		}

		// Simulate subscription granting
		mockSubRepo.SaveFunc = func(ctx context.Context, tx repository.Tx, s *model.UserSubscription) error {
			return nil
		}
		mockPlanRepo.FindByIDFunc = func(ctx context.Context, id string) (*model.SubscriptionPlan, error) {
			return &model.SubscriptionPlan{ID: id, DurationDays: 30}, nil
		}

		uc := usecase.NewSubscriptionUseCase(mockSubRepo, mockPlanRepo, mockCodeRepo, mockTxManager, testLogger)

		// --- Act ---
		_, err := uc.RedeemActivationCode(ctx, "user-1", "VALID-CODE")

		// --- Assert ---
		if err != nil {
			t.Fatalf("expected no error, but got %v", err)
		}
		if savedCode == nil {
			t.Fatal("expected the activation code to be saved (updated)")
		}
		if !savedCode.IsRedeemed {
			t.Error("expected code to be marked as redeemed")
		}
		if savedCode.RedeemedByUserID == nil || *savedCode.RedeemedByUserID != "user-1" {
			t.Error("code was not marked as redeemed by the correct user")
		}
	})

	t.Run("should fail to redeem a non-existent code", func(t *testing.T) {
		// --- Arrange ---
		mockCodeRepo := NewMockActivationCodeRepo()
		// Simulate that the code is not found
		mockCodeRepo.FindByCodeFunc = func(ctx context.Context, tx repository.Tx, c string) (*model.ActivationCode, error) {
			return nil, domain.ErrNotFound
		}
		uc := usecase.NewSubscriptionUseCase(nil, nil, mockCodeRepo, mockTxManager, testLogger)

		// --- Act ---
		_, err := uc.RedeemActivationCode(ctx, "user-1", "INVALID-CODE")

		// --- Assert ---
		if !errors.Is(err, domain.ErrCodeNotFound) {
			t.Errorf("expected ErrCodeNotFound, but got %v", err)
		}
	})
}

func TestSubscriptionUseCase_ListByUserID(t *testing.T) {
	ctx := context.Background()
	testLogger := newTestLogger()
	mockTxManager := NewMockTxManager()

	t.Run("should call repository and return a list of subscriptions", func(t *testing.T) {
		// --- Arrange ---
		mockSubRepo := NewMockSubscriptionRepo()
		mockCodeRepo := NewMockActivationCodeRepo()
		uc := usecase.NewSubscriptionUseCase(mockSubRepo, nil, mockCodeRepo, mockTxManager, testLogger)

		expectedSubs := []*model.UserSubscription{
			{ID: "sub-1", UserID: "user-123"},
			{ID: "sub-2", UserID: "user-123"},
		}

		// Configure the mock to return our expected subscriptions
		mockSubRepo.ListByUserIDFunc = func(ctx context.Context, tx repository.Tx, userID string) ([]*model.UserSubscription, error) {
			if userID != "user-123" {
				t.Errorf("expected to be called with userID 'user-123', but got '%s'", userID)
			}
			return expectedSubs, nil
		}

		// --- Act ---
		resultSubs, err := uc.ListByUserID(ctx, "user-123")

		// --- Assert ---
		if err != nil {
			t.Fatalf("expected no error, but got: %v", err)
		}
		if len(resultSubs) != 2 {
			t.Errorf("expected to get 2 subscriptions, but got %d", len(resultSubs))
		}
		if resultSubs[0].ID != "sub-1" {
			t.Error("mismatch in returned subscription data")
		}
	})
}
