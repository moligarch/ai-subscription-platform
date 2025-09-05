//go:build integration

package postgres

import (
	"context"
	"testing"
	"telegram-ai-subscription/internal/domain/model"
	"time"
)

func TestUserRepo_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode.")
	}

	repo := NewUserRepo(testPool)
	ctx := context.Background()
	
	t.Run("should perform full CRUD cycle", func(t *testing.T) {
		cleanup(t)

		// 1. Create a new user
		newUser, err := model.NewUser("", 123456789, "integration_user")
		if err != nil {
			t.Fatalf("model.NewUser() failed: %v", err)
		}
		err = repo.Save(ctx, nil, newUser)
		if err != nil {
			t.Fatalf("Failed to save new user: %v", err)
		}

		// 2. Read the user back by Telegram ID
		foundUser, err := repo.FindByTelegramID(ctx, nil, 123456789)
		if err != nil {
			t.Fatalf("Failed to find user by telegram ID: %v", err)
		}
		if foundUser == nil {
			t.Fatal("Expected to find a user, but got nil")
		}
		if foundUser.ID != newUser.ID {
			t.Errorf("Expected user ID to be %s, got %s", newUser.ID, foundUser.ID)
		}
		if foundUser.Username != "integration_user" {
			t.Errorf("Expected username to be 'integration_user', got '%s'", foundUser.Username)
		}

		// 3. Update the user's username
		foundUser.Username = "updated_user"
		err = repo.Save(ctx, nil, foundUser)
		if err != nil {
			t.Fatalf("Failed to update user: %v", err)
		}

		// 4. Read the user back by internal ID and verify the update
		updatedUser, err := repo.FindByID(ctx, nil, foundUser.ID)
		if err != nil {
			t.Fatalf("Failed to find user by ID: %v", err)
		}
		if updatedUser.Username != "updated_user" {
			t.Errorf("Expected username to be 'updated_user', got '%s'", updatedUser.Username)
		}
	})

	t.Run("should correctly count users", func(t *testing.T) {
		cleanup(t)

		// 1. Arrange: Create two users
		user1, _ := model.NewUser("", 111, "user1")
		user2, _ := model.NewUser("", 222, "user2")
		user1.LastActiveAt = time.Now().Add(-48 * time.Hour) // Inactive
		user2.LastActiveAt = time.Now()                     // Active
		
		if err := repo.Save(ctx, nil, user1); err != nil {
			t.Fatalf("Save user1 failed: %v", err)
		}
		if err := repo.Save(ctx, nil, user2); err != nil {
			t.Fatalf("Save user2 failed: %v", err)
		}

		// 2. Act & Assert: CountUsers
		totalCount, err := repo.CountUsers(ctx, nil)
		if err != nil {
			t.Fatalf("CountUsers failed: %v", err)
		}
		if totalCount != 2 {
			t.Errorf("expected total count to be 2, but got %d", totalCount)
		}

		// 3. Act & Assert: CountInactiveUsers
		// Set the threshold to yesterday
		inactiveThreshold := time.Now().Add(-24 * time.Hour)
		inactiveCount, err := repo.CountInactiveUsers(ctx, nil, inactiveThreshold)
		if err != nil {
			t.Fatalf("CountInactiveUsers failed: %v", err)
		}
		if inactiveCount != 1 {
			t.Errorf("expected inactive count to be 1, but got %d", inactiveCount)
		}
	})
}