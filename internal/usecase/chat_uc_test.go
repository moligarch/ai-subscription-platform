package usecase_test

import (
	"context"
	"errors"
	"testing"

	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/repository"
	"telegram-ai-subscription/internal/usecase"

	"github.com/jackc/pgx/v4"
)

func TestChatUseCase_StartChat(t *testing.T) {
	ctx := context.Background()
	testLogger := newTestLogger()
	mockTxManager := NewMockTxManager()

	t.Run("should start a new chat successfully", func(t *testing.T) {
		// --- Arrange ---
		mockChatRepo := NewMockChatSessionRepo()
		mockLocker := NewMockLocker()
		// Simulate that no active chat is found
		mockChatRepo.FindActiveByUserFunc = func(ctx context.Context, tx repository.Tx, userID string) (*model.ChatSession, error) {
			return nil, domain.ErrNotFound
		}

		var savedSession *model.ChatSession
		mockChatRepo.SaveFunc = func(ctx context.Context, tx repository.Tx, s *model.ChatSession) error {
			savedSession = s
			return nil
		}

		uc := usecase.NewChatUseCase(mockChatRepo, nil, nil, nil, mockLocker, mockTxManager, testLogger, false, nil)

		// --- Act ---
		session, err := uc.StartChat(ctx, "user-1", "test-model")

		// --- Assert ---
		if err != nil {
			t.Fatalf("expected no error, but got: %v", err)
		}
		if session == nil {
			t.Fatal("expected a session, but got nil")
		}
		if savedSession == nil {
			t.Fatal("expected session to be saved")
		}
		if savedSession.Status != model.ChatSessionActive {
			t.Errorf("expected new session to be active, but was %s", savedSession.Status)
		}
	})

	t.Run("should fail if a chat is already active", func(t *testing.T) {
		// --- Arrange ---
		mockChatRepo := NewMockChatSessionRepo()
		mockLocker := NewMockLocker()
		// Simulate that an active chat IS found
		mockChatRepo.FindActiveByUserFunc = func(ctx context.Context, tx repository.Tx, userID string) (*model.ChatSession, error) {
			return &model.ChatSession{Status: model.ChatSessionActive}, nil
		}
		uc := usecase.NewChatUseCase(mockChatRepo, nil, nil, nil, mockLocker, mockTxManager, testLogger, false, nil)

		// --- Act ---
		_, err := uc.StartChat(ctx, "user-1", "test-model")

		// --- Assert ---
		if err == nil {
			t.Fatal("expected an error, but got nil")
		}
		if !errors.Is(err, domain.ErrActiveChatExists) {
			t.Errorf("expected error ErrActiveChatExists, but got %v", err)
		}
	})
}

func TestChatUseCase_SendChatMessage(t *testing.T) {
	ctx := context.Background()
	testLogger := newTestLogger()
	mockTxManager := NewMockTxManager()

	mockSubRepo := NewMockSubscriptionRepo()
	mockSubPlanRepo := NewMockPlanRepo()

	subUC := usecase.NewSubscriptionUseCase(mockSubRepo, mockSubPlanRepo, mockTxManager, testLogger)

	t.Run("should queue an AI job successfully", func(t *testing.T) {
		// --- Arrange ---
		mockChatRepo := NewMockChatSessionRepo()
		mockAIJobRepo := NewMockAIJobRepo()
		mockLocker := NewMockLocker()

		// Simulate finding an active chat session
		session := &model.ChatSession{ID: "sess-1", UserID: "user-1", Status: model.ChatSessionActive}
		mockChatRepo.FindByIDFunc = func(ctx context.Context, tx repository.Tx, id string) (*model.ChatSession, error) {
			return session, nil
		}

		var savedMessage *model.ChatMessage
		mockChatRepo.SaveMessageFunc = func(ctx context.Context, tx repository.Tx, m *model.ChatMessage) error {
			savedMessage = m
			return nil
		}

		var savedJob *model.AIJob
		mockAIJobRepo.SaveFunc = func(ctx context.Context, tx repository.Tx, job *model.AIJob) error {
			savedJob = job
			return nil
		}

		// Simulate a successful transaction
		mockTxManager.WithTxFunc = func(ctx context.Context, txOpt pgx.TxOptions, fn func(ctx context.Context, tx repository.Tx) error) error {
			return fn(ctx, nil)
		}

		uc := usecase.NewChatUseCase(mockChatRepo, mockAIJobRepo, nil, subUC, mockLocker, mockTxManager, testLogger, false, nil)

		// --- Act ---
		err := uc.SendChatMessage(ctx, "sess-1", "Hello AI")

		// --- Assert ---
		if err != nil {
			t.Fatalf("expected no error, but got: %v", err)
		}
		if savedMessage == nil {
			t.Fatal("expected user message to be saved")
		}
		if savedMessage.Content != "Hello AI" {
			t.Error("incorrect message content was saved")
		}
		if savedJob == nil {
			t.Fatal("expected an AI job to be created and saved")
		}
		if savedJob.Status != model.AIJobStatusPending {
			t.Errorf("expected new job status to be 'pending', but got '%s'", savedJob.Status)
		}
		if savedJob.UserMessageID != savedMessage.ID {
			t.Error("AI job is not linked to the correct user message")
		}
	})
}
