//go:build !integration

package usecase_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/adapter"
	"telegram-ai-subscription/internal/domain/ports/repository"
	"telegram-ai-subscription/internal/infra/worker"
	"telegram-ai-subscription/internal/usecase"
)

func TestBroadcastUseCase(t *testing.T) {
	ctx := context.Background()
	logger := newTestLogger() // Assumes newTestLogger() is in mock_test.go

	t.Run("should broadcast message only to non-admin users", func(t *testing.T) {
		// Arrange
		users := []*model.User{
			{ID: "user-1", TelegramID: 101, IsAdmin: false},
			{ID: "user-2", TelegramID: 102, IsAdmin: true}, // Admin, should be skipped
			{ID: "user-3", TelegramID: 103, IsAdmin: false},
			{ID: "user-4", TelegramID: 104, IsAdmin: false},
			{ID: "user-5", TelegramID: 105, IsAdmin: true}, // Admin, should be skipped
		}
		expectedRecipientCount := 3

		// Use the shared mock repository
		mockRepo := NewMockUserRepo()
		mockRepo.ListFunc = func(ctx context.Context, tx repository.Tx, offset, limit int) ([]*model.User, error) {
			return users, nil
		}

		// Use the shared mock bot adapter
		var wg sync.WaitGroup
		wg.Add(expectedRecipientCount)
		mockBot := &MockTelegramBot{
			// Add a custom SendMessageFunc to be thread-safe and signal the WaitGroup
			SendMessageFunc: func(ctx context.Context, params adapter.SendMessageParams) error {
				// This mock doesn't need to store messages, just signal completion.
				wg.Done()
				return nil
			},
		}

		// Use a real worker pool
		pool := worker.NewPool(2)
		pool.Start(ctx)
		defer pool.Stop()

		uc := usecase.NewBroadcastUseCase(mockRepo, mockBot, pool, logger)

		// Act
		count, err := uc.BroadcastMessage(ctx, "Hello everyone")

		// Assert (Immediate)
		if err != nil {
			t.Fatalf("BroadcastMessage returned an error: %v", err)
		}
		if count != expectedRecipientCount {
			t.Errorf("expected count %d, but got %d", expectedRecipientCount, count)
		}

		// Assert (Asynchronous)
		waitChan := make(chan struct{})
		go func() {
			wg.Wait()
			close(waitChan)
		}()

		select {
		case <-waitChan:
			// Success, all messages were sent.
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for broadcast messages to be sent")
		}
	})
}
