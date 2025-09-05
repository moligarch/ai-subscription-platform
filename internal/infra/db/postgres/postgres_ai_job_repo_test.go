//go:build integration

package postgres

import (
	"context"
	"testing"
	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/infra/security"
	"time"

	"github.com/google/uuid"
)

func TestAIJobRepo_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode.")
	}

	// 1. Setup
	ctx := context.Background()
	repo := NewAIJobRepo(testPool)
	userRepo := NewUserRepo(testPool)
	encSvc, _ := security.NewEncryptionService("0123456789abcdef0123456789abcdef")
	chatRepo := NewChatSessionRepo(testPool, nil, encSvc)

	// Create prerequisite data
	user, _ := model.NewUser("", 111, "job_user")
	session := model.NewChatSession(uuid.NewString(), user.ID, "test-model")
	message := &model.ChatMessage{ID: uuid.NewString(), SessionID: session.ID, Role: "user", Content: "test"}

	// Helper to set up a clean state with prerequisites
	setupPrerequisites := func(t *testing.T) {
		cleanup(t)
		if err := userRepo.Save(ctx, nil, user); err != nil {
			t.Fatalf("failed to save user: %v", err)
		}
		if err := chatRepo.Save(ctx, nil, session); err != nil {
			t.Fatalf("failed to save session: %v", err)
		}
		if err := chatRepo.SaveMessage(ctx, nil, message); err != nil {
			t.Fatalf("failed to save message: %v", err)
		}
	}

	t.Run("should save and update an AI job", func(t *testing.T) {
		setupPrerequisites(t)

		job := &model.AIJob{
			ID:            uuid.NewString(),
			Status:        model.AIJobStatusPending,
			SessionID:     session.ID,
			UserMessageID: message.ID,
			CreatedAt:     time.Now(),
		}
		// Test Create
		if err := repo.Save(ctx, nil, job); err != nil {
			t.Fatalf("failed to save new job: %v", err)
		}

		// Verify creation by querying directly
		var status string
		err := testPool.QueryRow(ctx, "SELECT status FROM ai_jobs WHERE id = $1", job.ID).Scan(&status)
		if err != nil {
			t.Fatalf("failed to query saved job: %v", err)
		}
		if status != string(model.AIJobStatusPending) {
			t.Errorf("expected status to be 'pending', but got '%s'", status)
		}

		// Test Update
		job.Status = model.AIJobStatusCompleted
		if err := repo.Save(ctx, nil, job); err != nil {
			t.Fatalf("failed to update job: %v", err)
		}

		// Verify update by querying directly
		err = testPool.QueryRow(ctx, "SELECT status FROM ai_jobs WHERE id = $1", job.ID).Scan(&status)
		if err != nil {
			t.Fatalf("failed to query updated job: %v", err)
		}
		if status != string(model.AIJobStatusCompleted) {
			t.Errorf("expected status to be 'completed', but got '%s'", status)
		}
	})

	t.Run("should fetch and mark a pending job, skipping locked ones", func(t *testing.T) {
		setupPrerequisites(t)

		// Create two pending jobs
		job1 := &model.AIJob{ID: uuid.NewString(), Status: model.AIJobStatusPending, SessionID: session.ID, UserMessageID: message.ID, CreatedAt: time.Now().Add(-1 * time.Second)}
		job2 := &model.AIJob{ID: uuid.NewString(), Status: model.AIJobStatusPending, SessionID: session.ID, UserMessageID: message.ID, CreatedAt: time.Now()}
		repo.Save(ctx, nil, job1)
		repo.Save(ctx, nil, job2)

		// Manually start a transaction and lock the first job (job1) to simulate a concurrent worker.
		tx, err := testPool.Begin(ctx)
		if err != nil {
			t.Fatalf("failed to begin transaction: %v", err)
		}
		defer tx.Rollback(ctx) // Ensure rollback
		var lockedID string
		err = tx.QueryRow(ctx, "SELECT id FROM ai_jobs WHERE id = $1 FOR UPDATE", job1.ID).Scan(&lockedID)
		if err != nil {
			t.Fatalf("failed to lock job1: %v", err)
		}

		// Now, call the method under test. It should skip the locked job1 and fetch job2.
		fetchedJob, err := repo.FetchAndMarkProcessing(ctx)
		if err != nil {
			t.Fatalf("FetchAndMarkProcessing failed: %v", err)
		}
		if fetchedJob == nil {
			t.Fatal("expected to fetch a job, but got nil")
		}
		if fetchedJob.ID != job2.ID {
			t.Errorf("expected to fetch job2, but got job with ID %s", fetchedJob.ID)
		}
		if fetchedJob.Status != model.AIJobStatusProcessing {
			t.Errorf("expected fetched job status to be 'processing', but got '%s'", fetchedJob.Status)
		}

		// Release the lock on job1
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("failed to commit transaction: %v", err)
		}

		// Call again, it should now fetch job1
		fetchedJob, err = repo.FetchAndMarkProcessing(ctx)
		if err != nil || fetchedJob == nil || fetchedJob.ID != job1.ID {
			t.Fatal("failed to fetch job1 on the second call")
		}

		// Call a third time, no pending jobs should be left
		fetchedJob, err = repo.FetchAndMarkProcessing(ctx)
		if err != domain.ErrNotFound || fetchedJob != nil {
			t.Fatal("expected ErrNotFound when no pending jobs are available")
		}
	})
}