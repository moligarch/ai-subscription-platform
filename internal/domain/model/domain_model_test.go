//go:build !integration

package model

import (
	"errors"
	"testing"
	"time"

	"telegram-ai-subscription/internal/domain"
)

// --- User Model Tests ---

func TestNewUser(t *testing.T) {
	t.Run("should create a new user successfully", func(t *testing.T) {
		startTime := time.Now()
		user, err := NewUser("", 12345, "testuser")

		if err != nil {
			t.Fatalf("expected no error, but got: %v", err)
		}
		if user == nil {
			t.Fatal("expected user to be non-nil, but got nil")
		}
		if user.ID == "" {
			t.Error("expected user ID to be non-empty")
		}
		if user.TelegramID != 12345 {
			t.Errorf("expected telegram ID to be 12345, but got %d", user.TelegramID)
		}
		if user.Username != "testuser" {
			t.Errorf("expected username to be 'testuser', but got %s", user.Username)
		}
		if time.Since(startTime) > time.Second {
			t.Errorf("user.RegisteredAt timestamp is too far from current time")
		}
		if !user.Privacy.AllowMessageStorage {
			t.Error("expected default privacy for AllowMessageStorage to be true")
		}
	})

	t.Run("should fail with invalid telegram ID", func(t *testing.T) {
		user, err := NewUser("", 0, "testuser")
		if err == nil {
			t.Fatal("expected an error for invalid telegram ID, but got nil")
		}
		if user != nil {
			t.Errorf("expected user to be nil on error, but it was not")
		}
		if !errors.Is(err, domain.ErrInvalidArgument) {
			t.Errorf("expected error to be ErrInvalidArgument, but got %T", err)
		}
	})

	t.Run("should fail with empty username", func(t *testing.T) {
		user, err := NewUser("", 12345, "")
		if err == nil {
			t.Fatal("expected an error for empty username, but got nil")
		}
		if user != nil {
			t.Errorf("expected user to be nil on error, but it was not")
		}
		if !errors.Is(err, domain.ErrInvalidArgument) {
			t.Errorf("expected error to be ErrInvalidArgument, but got %T", err)
		}
	})
}

// --- SubscriptionPlan Model Tests ---

func TestNewSubscriptionPlan(t *testing.T) {
	t.Run("should create a new plan successfully", func(t *testing.T) {
		plan, err := NewSubscriptionPlan("plan-1", "Pro", 30, 1000, 50000)
		if err != nil {
			t.Fatalf("expected no error, but got: %v", err)
		}
		if plan == nil {
			t.Fatal("expected plan to be non-nil, but got nil")
		}
		if plan.Name != "Pro" {
			t.Errorf("expected plan name to be 'Pro', but got %s", plan.Name)
		}
		if plan.DurationDays != 30 {
			t.Errorf("expected duration to be 30, but got %d", plan.DurationDays)
		}
	})

	t.Run("should fail with invalid arguments", func(t *testing.T) {
		testCases := []struct {
			name         string
			id           string
			planName     string
			durationDays int
			credits      int64
			priceIRR     int64
		}{
			{"empty name", "plan-1", "", 30, 1000, 50000},
			{"zero duration", "plan-1", "Pro", 0, 1000, 50000},
			{"negative credits", "plan-1", "Pro", 30, -1, 50000},
			{"zero price", "plan-1", "Pro", 30, 1000, 0},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				plan, err := NewSubscriptionPlan(tc.id, tc.planName, tc.durationDays, tc.credits, tc.priceIRR)
				if err == nil {
					t.Fatalf("expected an error for %s, but got nil", tc.name)
				}
				if plan != nil {
					t.Errorf("expected plan to be nil on error, but it was not")
				}
				if !errors.Is(err, domain.ErrInvalidArgument) {
					t.Errorf("expected error to be ErrInvalidArgument, but got %T", err)
				}
			})
		}
	})
}

// --- ChatSession Model Tests ---

func TestChatSession(t *testing.T) {
	t.Run("NewChatSession should initialize correctly", func(t *testing.T) {
		session := NewChatSession("sess-1", "user-1", "gpt-4o-mini")
		if session == nil {
			t.Fatal("expected session to be non-nil, but got nil")
		}
		if session.ID != "sess-1" {
			t.Errorf("expected session ID to be 'sess-1', but got %s", session.ID)
		}
		if session.UserID != "user-1" {
			t.Errorf("expected user ID to be 'user-1', but got %s", session.UserID)
		}
		if session.Status != ChatSessionActive {
			t.Errorf("expected session status to be 'active', but got %s", session.Status)
		}
		if len(session.Messages) != 0 {
			t.Errorf("expected new session to have no messages, but got %d", len(session.Messages))
		}
	})

	t.Run("AddMessage should append messages and update timestamp", func(t *testing.T) {
		session := NewChatSession("sess-1", "user-1", "gpt-4o-mini")
		initialTime := session.UpdatedAt

		time.Sleep(1 * time.Millisecond) // Ensure timestamp has a chance to change
		session.AddMessage("user", "Hello", 1)

		if len(session.Messages) != 1 {
			t.Fatalf("expected 1 message, but got %d", len(session.Messages))
		}
		if session.Messages[0].Role != "user" {
			t.Errorf("expected message role to be 'user', but got %s", session.Messages[0].Role)
		}
		if session.Messages[0].Content != "Hello" {
			t.Errorf("expected message content to be 'Hello', but got %s", session.Messages[0].Content)
		}
		if !session.UpdatedAt.After(initialTime) {
			t.Error("expected UpdatedAt timestamp to be updated after adding a message")
		}
	})

	t.Run("GetRecentMessages should slice history correctly", func(t *testing.T) {
		session := NewChatSession("sess-1", "user-1", "gpt-4o-mini")
		session.AddMessage("user", "1", 1)
		session.AddMessage("assistant", "2", 1)
		session.AddMessage("user", "3", 1)
		session.AddMessage("assistant", "4", 1)
		session.AddMessage("user", "5", 1)

		// Get last 3 messages
		recent := session.GetRecentMessages(3)
		if len(recent) != 3 {
			t.Fatalf("expected 3 recent messages, but got %d", len(recent))
		}
		if recent[0].Content != "3" || recent[1].Content != "4" || recent[2].Content != "5" {
			t.Errorf("GetRecentMessages(3) returned incorrect slice: got contents %s, %s, %s", recent[0].Content, recent[1].Content, recent[2].Content)
		}

		// Get more messages than exist
		all := session.GetRecentMessages(10)
		if len(all) != 5 {
			t.Errorf("expected 5 messages when requesting more than exist, but got %d", len(all))
		}

		// Get zero messages (current logic returns all)
		none := session.GetRecentMessages(0)
		if len(none) != 5 {
			t.Errorf("expected 5 messages when requesting 0, but got %d", len(none))
		}
	})
}
