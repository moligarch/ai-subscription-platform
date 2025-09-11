//go:build !integration

package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/repository" // Add this if it's missing
	"telegram-ai-subscription/internal/usecase"
)

func TestUserUseCase_RegisterOrFetch(t *testing.T) {
	ctx := context.Background()
	testLogger := newTestLogger()

	t.Run("should fetch existing user and update last active time", func(t *testing.T) {
		// --- Arrange ---
		mockUserRepo := NewMockUserRepo()
		mockTxManager := NewMockTxManager()
		mockChatRepo := NewMockChatSessionRepo()
		uc := usecase.NewUserUseCase(mockUserRepo, mockChatRepo, mockTxManager, testLogger)

		// Create the initial state
		originalUser := &model.User{
			ID:           "user-123",
			TelegramID:   12345,
			Username:     "old_username",
			LastActiveAt: time.Now().Add(-1 * time.Hour),
		}
		// Seed the mock's in-memory DB directly.
		mockUserRepo.Save(ctx, nil, originalUser)

		// --- Act ---
		// The use case will now interact with the mock's default logic.
		_, err := uc.RegisterOrFetch(ctx, 12345, "new_username")
		if err != nil {
			t.Fatalf("RegisterOrFetch failed: %v", err)
		}

		// --- Assert ---
		// Fetch the user from the mock's DB *after* the operation to check its final state.
		updatedUser, _ := mockUserRepo.FindByID(ctx, nil, "user-123")
		if updatedUser == nil {
			t.Fatal("User not found in mock repo after update")
		}

		// Check that the timestamp was updated.
		if !updatedUser.LastActiveAt.After(originalUser.LastActiveAt) {
			t.Errorf("expected LastActiveAt to be updated. Original: %v, New: %v", originalUser.LastActiveAt, updatedUser.LastActiveAt)
		}
		// Check that the username was updated.
		if updatedUser.Username != "new_username" {
			t.Errorf("expected username to be 'new_username', but got '%s'", updatedUser.Username)
		}
	})

	// The other two test cases ("new user" and "repository failure")
	// from the previous version are correct and can remain as they are.
	t.Run("should register a new user if not found", func(t *testing.T) {
		// --- Arrange ---
		mockUserRepo := NewMockUserRepo()
		mockTxManager := NewMockTxManager()
		mockChatRepo := NewMockChatSessionRepo()
		uc := usecase.NewUserUseCase(mockUserRepo, mockChatRepo, mockTxManager, testLogger)

		const newTelegramID = 54321
		const newUsername = "new_user"

		// --- Act ---
		newUser, err := uc.RegisterOrFetch(ctx, newTelegramID, newUsername)
		if err != nil {
			t.Fatalf("expected no error, but got: %v", err)
		}

		// --- Assert ---
		// Verify by fetching the user back from the mock repo
		savedUser, _ := mockUserRepo.FindByID(ctx, nil, newUser.ID)
		if savedUser == nil {
			t.Fatal("expected user to be saved, but it wasn't found in mock repo")
		}
		if savedUser.TelegramID != newTelegramID {
			t.Errorf("expected saved user's telegram ID to be %d, but got %d", newTelegramID, savedUser.TelegramID)
		}
	})

	t.Run("should propagate error on repository failure", func(t *testing.T) {
		// --- Arrange ---
		mockUserRepo := NewMockUserRepo()
		mockTxManager := NewMockTxManager()
		expectedErr := errors.New("database is down")

		// For this specific case, overriding with a custom function is correct.
		mockUserRepo.FindByTelegramIDFunc = func(ctx context.Context, tx repository.Tx, tgID int64) (*model.User, error) {
			return nil, expectedErr
		}
		mockChatRepo := NewMockChatSessionRepo()
		uc := usecase.NewUserUseCase(mockUserRepo, mockChatRepo, mockTxManager, testLogger)

		// --- Act ---
		_, err := uc.RegisterOrFetch(ctx, 12345, "any_user")

		// --- Assert ---
		if err == nil {
			t.Fatal("expected an error, but got nil")
		}
		if !errors.Is(err, expectedErr) {
			t.Errorf("expected error to wrap '%v', but it didn't", expectedErr)
		}
	})

	t.Run("should count users", func(t *testing.T) {
		mockUserRepo := NewMockUserRepo()
		mockUserRepo.CountUsersFunc = func(ctx context.Context, tx repository.Tx) (int, error) {
			return 99, nil
		}
		uc := usecase.NewUserUseCase(mockUserRepo, nil, nil, testLogger)

		count, err := uc.Count(ctx)
		if err != nil {
			t.Fatalf("Count failed: %v", err)
		}
		if count != 99 {
			t.Errorf("expected count to be 99, got %d", count)
		}
	})
}

func TestUserUseCase_ToggleMessageStorage(t *testing.T) {
	ctx := context.Background()
	testLogger := newTestLogger()
	mockTxManager := NewMockTxManager()

	t.Run("should disable storage and delete history", func(t *testing.T) {
		// --- Arrange ---
		mockUserRepo := NewMockUserRepo()
		mockChatRepo := NewMockChatSessionRepo()

		// User starts with storage enabled
		user := &model.User{ID: "user-1", TelegramID: 123, Privacy: model.PrivacySettings{AllowMessageStorage: true}}
		mockUserRepo.FindByTelegramIDFunc = func(ctx context.Context, tx repository.Tx, tgID int64) (*model.User, error) {
			return user, nil
		}

		var savedUser *model.User
		mockUserRepo.SaveFunc = func(ctx context.Context, tx repository.Tx, u *model.User) error {
			savedUser = u
			return nil
		}

		historyDeleted := false
		mockChatRepo.DeleteAllByUserIDFunc = func(ctx context.Context, tx repository.Tx, userID string) error {
			historyDeleted = true
			return nil
		}

		uc := usecase.NewUserUseCase(mockUserRepo, mockChatRepo, mockTxManager, testLogger)

		// --- Act ---
		err := uc.ToggleMessageStorage(ctx, 123)

		// --- Assert ---
		if err != nil {
			t.Fatalf("expected no error, but got %v", err)
		}
		if savedUser == nil {
			t.Fatal("expected user to be saved")
		}
		if savedUser.Privacy.AllowMessageStorage {
			t.Error("expected AllowMessageStorage to be toggled to false")
		}
		if !historyDeleted {
			t.Error("expected user chat history to be deleted")
		}
	})
}
