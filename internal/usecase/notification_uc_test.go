package usecase_test

import (
	"context"
	"testing"
	"time"

	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/repository"
	"telegram-ai-subscription/internal/usecase"
)

func TestNotificationUseCase(t *testing.T) {
	ctx := context.Background()
	testLogger := newTestLogger()

	t.Run("should send notification for an expiring subscription", func(t *testing.T) {
		// --- Arrange ---
		mockSubRepo := NewMockSubscriptionRepo()
		mockNotifLogRepo := NewMockNotificationLogRepo()
		mockUserRepo := NewMockUserRepo()
		mockBot := &MockTelegramBot{}

		// Subscription expires in 3 days
		expiresAt := time.Now().Add(3 * 24 * time.Hour)
		sub := &model.UserSubscription{ID: "sub-1", UserID: "user-1", ExpiresAt: &expiresAt}
		mockSubRepo.FindExpiringFunc = func(ctx context.Context, tx repository.Tx, withinDays int) ([]*model.UserSubscription, error) {
			return []*model.UserSubscription{sub}, nil
		}

		// Notification has not been sent yet
		mockNotifLogRepo.ExistsFunc = func(ctx context.Context, tx repository.Tx, subscriptionID, kind string, thresholdDays int) (bool, error) {
			return false, nil
		}

		// User can be found
		user := &model.User{ID: "user-1", TelegramID: 12345}
		mockUserRepo.FindByIDFunc = func(ctx context.Context, tx repository.Tx, id string) (*model.User, error) {
			return user, nil
		}

		uc := usecase.NewNotificationUseCase(mockSubRepo, mockNotifLogRepo, mockUserRepo, mockBot, testLogger)

		// --- Act ---
		sentCount, err := uc.CheckAndSendExpiryNotifications(ctx)

		// --- Assert ---
		if err != nil {
			t.Fatalf("expected no error, but got: %v", err)
		}
		if sentCount != 1 {
			t.Errorf("expected sent count to be 1, but got %d", sentCount)
		}
		if len(mockBot.Sent) != 1 {
			t.Fatal("expected one message to be sent")
		}
		if mockBot.Sent[0].ID != user.TelegramID {
			t.Error("message sent to wrong telegram user")
		}
	})

	t.Run("should NOT send notification if already sent", func(t *testing.T) {
		// --- Arrange ---
		mockSubRepo := NewMockSubscriptionRepo()
		mockNotifLogRepo := NewMockNotificationLogRepo()
		mockUserRepo := NewMockUserRepo()
		mockBot := &MockTelegramBot{}

		expiresAt := time.Now().Add(3 * 24 * time.Hour)
		sub := &model.UserSubscription{ID: "sub-1", UserID: "user-1", ExpiresAt: &expiresAt}
		mockSubRepo.FindExpiringFunc = func(ctx context.Context, tx repository.Tx, withinDays int) ([]*model.UserSubscription, error) {
			return []*model.UserSubscription{sub}, nil
		}

		// Notification HAS been sent already
		mockNotifLogRepo.ExistsFunc = func(ctx context.Context, tx repository.Tx, subscriptionID, kind string, thresholdDays int) (bool, error) {
			return true, nil
		}

		uc := usecase.NewNotificationUseCase(mockSubRepo, mockNotifLogRepo, mockUserRepo, mockBot, testLogger)

		// --- Act ---
		sentCount, err := uc.CheckAndSendExpiryNotifications(ctx)

		// --- Assert ---
		if err != nil {
			t.Fatalf("expected no error, but got: %v", err)
		}
		if sentCount != 0 {
			t.Errorf("expected sent count to be 0, but got %d", sentCount)
		}
		if len(mockBot.Sent) != 0 {
			t.Fatal("expected zero messages to be sent")
		}
	})
}
