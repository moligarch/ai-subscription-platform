//go:build integration

package postgres

import (
	"context"
	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/infra/security"
	"testing"

	"github.com/google/uuid"
)

func TestChatSessionRepo_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode.")
	}

	// 1. Setup
	ctx := context.Background()
	encSvc, err := security.NewEncryptionService("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("failed to create encryption service: %v", err)
	}
	// We pass nil for the Redis cache, as we are only testing the database layer.
	repo := NewChatSessionRepo(testPool, nil, encSvc)
	userRepo := NewUserRepo(testPool)

	// Create a prerequisite user for the tests
	user, _ := model.NewUser("", 111, "chat_user")

	t.Run("should save, find, and decrypt a session with messages", func(t *testing.T) {
		cleanup(t)
		if err := userRepo.Save(ctx, nil, user); err != nil {
			t.Fatalf("failed to save user: %v", err)
		}

		// Create a session
		session := model.NewChatSession(uuid.NewString(), user.ID, "test-model")
		if err := repo.Save(ctx, nil, session); err != nil {
			t.Fatalf("failed to save session: %v", err)
		}

		// Save messages to the session
		msg1 := &model.ChatMessage{ID: uuid.NewString(), SessionID: session.ID, Role: "user", Content: "Hello World"}
		msg2 := &model.ChatMessage{ID: uuid.NewString(), SessionID: session.ID, Role: "assistant", Content: "Hello User"}
		if err := repo.SaveMessage(ctx, nil, msg1); err != nil {
			t.Fatalf("failed to save message 1: %v", err)
		}
		if err := repo.SaveMessage(ctx, nil, msg2); err != nil {
			t.Fatalf("failed to save message 2: %v", err)
		}

		// Find the session by ID
		foundSession, err := repo.FindByID(ctx, nil, session.ID)
		if err != nil {
			t.Fatalf("FindByID failed: %v", err)
		}
		if foundSession == nil {
			t.Fatal("expected to find a session, but got nil")
		}
		if len(foundSession.Messages) != 2 {
			t.Fatalf("expected to find 2 messages, but got %d", len(foundSession.Messages))
		}
		if foundSession.Messages[0].Content != "Hello World" {
			t.Errorf("message content was not decrypted or retrieved correctly")
		}
	})

	t.Run("should handle active and finished statuses", func(t *testing.T) {
		cleanup(t)
		if err := userRepo.Save(ctx, nil, user); err != nil {
			t.Fatalf("failed to save user: %v", err)
		}

		activeSession := model.NewChatSession(uuid.NewString(), user.ID, "active-model")
		finishedSession := model.NewChatSession(uuid.NewString(), user.ID, "finished-model")
		finishedSession.Status = model.ChatSessionFinished
		repo.Save(ctx, nil, activeSession)
		repo.Save(ctx, nil, finishedSession)

		// FindActiveByUser should only return the active one
		foundActive, err := repo.FindActiveByUser(ctx, nil, user.ID)
		if err != nil {
			t.Fatalf("FindActiveByUser failed: %v", err)
		}
		if foundActive == nil || foundActive.ID != activeSession.ID {
			t.Fatal("did not find the correct active session")
		}

		// Update the status
		err = repo.UpdateStatus(ctx, nil, activeSession.ID, model.ChatSessionFinished)
		if err != nil {
			t.Fatalf("UpdateStatus failed: %v", err)
		}

		// Now, there should be no active session
		foundActive, err = repo.FindActiveByUser(ctx, nil, user.ID)
		if err == nil || foundActive != nil {
			t.Fatal("expected no active session to be found after updating status")
		}
	})

	t.Run("should delete a session and its messages via cascade", func(t *testing.T) {
		cleanup(t)
		if err := userRepo.Save(ctx, nil, user); err != nil {
			t.Fatalf("failed to save user: %v", err)
		}

		session := model.NewChatSession(uuid.NewString(), user.ID, "model-to-delete")
		repo.Save(ctx, nil, session)
		msg := &model.ChatMessage{ID: uuid.NewString(), SessionID: session.ID, Role: "user", Content: "to be deleted"}
		repo.SaveMessage(ctx, nil, msg)

		// Delete the session
		err := repo.Delete(ctx, nil, session.ID)
		if err != nil {
			t.Fatalf("Delete failed: %v", err)
		}

		// Verify the session is gone
		foundSession, _ := repo.FindByID(ctx, nil, session.ID)
		if foundSession != nil {
			t.Error("expected session to be deleted, but it was found")
		}

		// Verify its messages are also gone (due to ON DELETE CASCADE)
		var messageCount int
		err = testPool.QueryRow(ctx, "SELECT COUNT(*) FROM chat_messages WHERE session_id = $1", session.ID).Scan(&messageCount)
		if err != nil {
			t.Fatalf("failed to count messages: %v", err)
		}
		if messageCount != 0 {
			t.Errorf("expected messages to be cascade deleted, but %d were found", messageCount)
		}
	})
}
