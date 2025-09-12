//go:build !integration

package usecase_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/repository" // Add this if it's missing
	"telegram-ai-subscription/internal/usecase"
)

func TestUserUseCase_RegisterOrFetch(t *testing.T) {
	ctx := context.Background()
	testLogger := newTestLogger()
	testTranslator := newTestTranslator()
	mockTxManager := NewMockTxManager()

	t.Run("should fetch existing user and update last active time", func(t *testing.T) {
		// --- Arrange ---
		mockUserRepo := NewMockUserRepo()
		mockChatRepo := NewMockChatSessionRepo()
		mockRegStateRepo := NewMockConversationStateRepo()
		uc := usecase.NewUserUseCase(mockUserRepo, mockChatRepo, mockRegStateRepo, testTranslator, mockTxManager, testLogger)

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
		mockChatRepo := NewMockChatSessionRepo()
		mockRegStateRepo := NewMockConversationStateRepo()
		uc := usecase.NewUserUseCase(mockUserRepo, mockChatRepo, mockRegStateRepo, testTranslator, mockTxManager, testLogger)

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
		expectedErr := errors.New("database is down")

		// For this specific case, overriding with a custom function is correct.
		mockUserRepo.FindByTelegramIDFunc = func(ctx context.Context, tx repository.Tx, tgID int64) (*model.User, error) {
			return nil, expectedErr
		}
		mockChatRepo := NewMockChatSessionRepo()
		mockRegStateRepo := NewMockConversationStateRepo()
		uc := usecase.NewUserUseCase(mockUserRepo, mockChatRepo, mockRegStateRepo, testTranslator, mockTxManager, testLogger)

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
		uc := usecase.NewUserUseCase(mockUserRepo, nil, nil, nil, nil, testLogger)

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
	testTranslator := newTestTranslator()
	mockTxManager := NewMockTxManager()

	t.Run("should disable storage and delete history", func(t *testing.T) {
		// --- Arrange ---
		mockUserRepo := NewMockUserRepo()
		mockChatRepo := NewMockChatSessionRepo()
		mockRegStateRepo := NewMockConversationStateRepo()

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

		// Pass the new mock to the constructor
		uc := usecase.NewUserUseCase(mockUserRepo, mockChatRepo, mockRegStateRepo, testTranslator, mockTxManager, testLogger)

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

func TestUserUseCase_Counting(t *testing.T) {
	ctx := context.Background()
	testLogger := newTestLogger()
	testTranslator := newTestTranslator()
	mockTxManager := NewMockTxManager()

	t.Run("CountInactiveSince should call the repository and return the count", func(t *testing.T) {
		// --- Arrange ---
		mockUserRepo := NewMockUserRepo()
		mockChatRepo := NewMockChatSessionRepo()
		mockRegStateRepo := NewMockConversationStateRepo()

		// Configure the mock to return a specific count
		mockUserRepo.CountInactiveUsersFunc = func(ctx context.Context, tx repository.Tx, olderThan time.Time) (int, error) {
			return 42, nil
		}

		uc := usecase.NewUserUseCase(mockUserRepo, mockChatRepo, mockRegStateRepo, testTranslator, mockTxManager, testLogger)

		// --- Act ---
		count, err := uc.CountInactiveSince(ctx, time.Now())

		// --- Assert ---
		if err != nil {
			t.Fatalf("expected no error, but got: %v", err)
		}
		if count != 42 {
			t.Errorf("expected count of inactive users to be 42, but got %d", count)
		}
	})
}

func TestUserUseCase_RegistrationFlow(t *testing.T) {
	ctx := context.Background()
	testLogger := newTestLogger()
	testTranslator := newTestTranslator()
	mockTxManager := NewMockTxManager()

	t.Run("should guide a user through the full registration flow", func(t *testing.T) {
		// --- Arrange ---
		mockUserRepo := NewMockUserRepo()
		mockChatRepo := NewMockChatSessionRepo()
		mockRegStateRepo := NewMockConversationStateRepo()

		uc := usecase.NewUserUseCase(mockUserRepo, mockChatRepo, mockRegStateRepo, testTranslator, mockTxManager, testLogger)

		const tgID = int64(12345)
		const fullName = "Test"
		const phoneNumber = "+123456789"

		// Seed the mock with a pending user
		user := &model.User{ID: "user-1", TelegramID: tgID, RegistrationStatus: model.RegistrationStatusPending}
		mockUserRepo.Save(ctx, nil, user)

		// --- Act & Assert: Step 1 - Start the flow ---
		// The /start command handler calls this.
		err := uc.StartRegistration(ctx, tgID)
		if err != nil {
			t.Fatalf("StartRegistration failed: %v", err)
		}

		// --- Act & Assert: Step 2 - User provides Full Name ---
		reply, markup, err := uc.ProcessRegistrationStep(ctx, tgID, fullName, "")
		if err != nil {
			t.Fatalf("ProcessRegistrationStep (full name) failed: %v", err)
		}
		if !strings.Contains(reply, "موبایل") {
			t.Errorf("Expected phone prompt, but got: %s", reply)
		}
		if markup == nil || markup.IsInline || !markup.Buttons[0][0].RequestContact {
			t.Error("Expected a 'Share Contact' reply keyboard")
		}

		// Verify state was updated in Redis
		state, _ := mockRegStateRepo.GetState(ctx, tgID)
		if state.Step != usecase.StepAwaitPhone {
			t.Errorf("Expected state to be 'awaiting_phone', but got '%s'", state.Step)
		}
		if state.Data["full_name"] != fullName {
			t.Error("Full name was not saved correctly in the state")
		}

		// --- Act & Assert: Step 3 - User provides Phone Number ---
		reply, markup, err = uc.ProcessRegistrationStep(ctx, tgID, "", phoneNumber)
		if err != nil {
			t.Fatalf("ProcessRegistrationStep (phone) failed: %v", err)
		}
		if !strings.Contains(reply, "ممنون از شما") {
			t.Errorf("Expected verification prompt, but got: %s", reply)
		}
		if markup == nil || !markup.IsInline || len(markup.Buttons) != 3 {
			t.Error("Expected an inline keyboard with 3 verification buttons")
		}

		// Verify user was updated in the database
		updatedUser, _ := mockUserRepo.FindByTelegramID(ctx, nil, tgID)
		if updatedUser.FullName != fullName || updatedUser.PhoneNumber != phoneNumber {
			t.Error("User record was not updated with full name and phone number")
		}

		// --- Act & Assert: Step 4 - User Completes Registration ---
		err = uc.CompleteRegistration(ctx, tgID)
		if err != nil {
			t.Fatalf("CompleteRegistration failed: %v", err)
		}

		// Verify final user status and that Redis state was cleared
		finalUser, _ := mockUserRepo.FindByTelegramID(ctx, nil, tgID)
		if finalUser.RegistrationStatus != model.RegistrationStatusCompleted {
			t.Error("Expected user registration status to be 'completed'")
		}
		finalState, _ := mockRegStateRepo.GetState(ctx, tgID)
		if finalState != nil {
			t.Error("Expected registration state to be cleared from Redis, but it was not")
		}
	})
}
