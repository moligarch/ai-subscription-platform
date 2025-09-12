//go:build !integration

package usecase_test

import (
	"context"
	"errors"
	"reflect"
	"sort"
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
		uc, mockChatRepo, _, _, mockPricingRepo := setupChatUCTestWithMocks()

		// Simulate that pricing IS found for this model
		mockPricingRepo.GetByModelNameFunc = func(ctx context.Context, modelName string) (*model.ModelPricing, error) {
			return &model.ModelPricing{ModelName: modelName, Active: true}, nil
		}
		// Simulate that no active chat is found
		mockChatRepo.FindActiveByUserFunc = func(ctx context.Context, tx repository.Tx, userID string) (*model.ChatSession, error) {
			return nil, domain.ErrNotFound
		}

		var savedSession *model.ChatSession
		mockChatRepo.SaveFunc = func(ctx context.Context, tx repository.Tx, s *model.ChatSession) error {
			savedSession = s
			return nil
		}

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
		mockPricingRepo := NewMockModelPricingRepo()
		mockLocker := NewMockLocker()

		// Simulate pricing is found (required for the pre-flight check)
		mockPricingRepo.GetByModelNameFunc = func(ctx context.Context, modelName string) (*model.ModelPricing, error) {
			return &model.ModelPricing{ModelName: modelName}, nil
		}
		// Simulate that an active chat IS found
		mockChatRepo.FindActiveByUserFunc = func(ctx context.Context, tx repository.Tx, userID string) (*model.ChatSession, error) {
			return &model.ChatSession{Status: model.ChatSessionActive}, nil
		}
		uc := usecase.NewChatUseCase(mockChatRepo, nil, nil, mockPricingRepo, nil, nil, nil, mockLocker, mockTxManager, testLogger, false)

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

	t.Run("should fail if model pricing is not defined", func(t *testing.T) {
		// --- Arrange ---
		mockChatRepo := NewMockChatSessionRepo()
		mockLocker := NewMockLocker()
		mockPricingRepo := NewMockModelPricingRepo()

		// Simulate that pricing is not found for this model
		mockPricingRepo.GetByModelNameFunc = func(ctx context.Context, model string) (*model.ModelPricing, error) {
			return nil, domain.ErrNotFound
		}

		uc := usecase.NewChatUseCase(mockChatRepo, nil, nil, mockPricingRepo, nil, nil, nil, mockLocker, mockTxManager, testLogger, false)

		// --- Act ---
		_, err := uc.StartChat(ctx, "user-1", "unpriced-model")

		// --- Assert ---
		if err == nil {
			t.Fatal("expected an error, but got nil")
		}
		if !errors.Is(err, domain.ErrModelNotAvailable) {
			t.Errorf("expected error ErrModelNotAvailable, but got %v", err)
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
		mockUserRepo := NewMockUserRepo()
		mockLocker := NewMockLocker()

		// Simulate finding an active chat session
		session := &model.ChatSession{ID: "sess-1", UserID: "user-1", Status: model.ChatSessionActive}
		mockChatRepo.FindByIDFunc = func(ctx context.Context, tx repository.Tx, id string) (*model.ChatSession, error) {
			return session, nil
		}

		var savedMessage *model.ChatMessage
		mockChatRepo.SaveMessageFunc = func(ctx context.Context, tx repository.Tx, m *model.ChatMessage) (bool, error) {
			savedMessage = m
			return true, nil
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

		uc := usecase.NewChatUseCase(mockChatRepo, mockUserRepo, nil, nil, mockAIJobRepo, nil, subUC, mockLocker, mockTxManager, testLogger, false)

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
		if *savedJob.UserMessageID != savedMessage.ID {
			t.Error("AI job is not linked to the correct user message")
		}
	})
}

func TestChatUseCase_ListHistory(t *testing.T) {
	ctx := context.Background()
	uc, mockChatRepo, _ := setupChatUCTest()

	t.Run("should list user chat history", func(t *testing.T) {
		// Arrange
		sessions := []*model.ChatSession{
			{ID: "sess-1", Model: "model-1", Messages: []model.ChatMessage{{Content: "Hello"}}},
			{ID: "sess-2", Model: "model-2", Messages: []model.ChatMessage{{Content: "World"}}},
		}
		mockChatRepo.ListByUserFunc = func(ctx context.Context, tx repository.Tx, userID string, offset, limit int) ([]*model.ChatSession, error) {
			return sessions, nil
		}

		// Act
		history, err := uc.ListHistory(ctx, "user-1", 0, 10)

		// Assert
		if err != nil {
			t.Fatalf("expected no error, but got %v", err)
		}
		if len(history) != 2 {
			t.Errorf("expected history length to be 2, but got %d", len(history))
		}
		if history[0].SessionID != "sess-1" || history[1].Model != "model-2" {
			t.Error("history data was not mapped correctly")
		}
	})
}

func TestChatUseCase_EndChat(t *testing.T) {
	ctx := context.Background()
	uc, mockChatRepo, _ := setupChatUCTest()

	t.Run("should end an active chat session", func(t *testing.T) {
		// Arrange
		session := &model.ChatSession{ID: "sess-1", Status: model.ChatSessionActive}
		mockChatRepo.FindByIDFunc = func(ctx context.Context, tx repository.Tx, id string) (*model.ChatSession, error) {
			return session, nil
		}
		var updatedStatus model.ChatSessionStatus
		mockChatRepo.UpdateStatusFunc = func(ctx context.Context, tx repository.Tx, sessionID string, status model.ChatSessionStatus) error {
			updatedStatus = status
			return nil
		}

		// Act
		err := uc.EndChat(ctx, "sess-1")

		// Assert
		if err != nil {
			t.Fatalf("expected no error, but got %v", err)
		}
		if updatedStatus != model.ChatSessionFinished {
			t.Errorf("expected session status to be updated to 'finished', but got '%s'", updatedStatus)
		}
	})
}

func TestChatUseCase_SessionManagement(t *testing.T) {
	ctx := context.Background()
	uc, mockChatRepo, _ := setupChatUCTest()

	t.Run("SwitchActiveSession should update statuses correctly", func(t *testing.T) {
		// Arrange
		activeSession := &model.ChatSession{ID: "sess-active", Status: model.ChatSessionActive}
		mockChatRepo.FindActiveByUserFunc = func(ctx context.Context, tx repository.Tx, userID string) (*model.ChatSession, error) {
			return activeSession, nil
		}

		var updatedStatuses []model.ChatSessionStatus
		mockChatRepo.UpdateStatusFunc = func(ctx context.Context, tx repository.Tx, sessionID string, status model.ChatSessionStatus) error {
			updatedStatuses = append(updatedStatuses, status)
			return nil
		}

		// Act
		err := uc.SwitchActiveSession(ctx, "user-1", "sess-new")

		// Assert
		if err != nil {
			t.Fatalf("expected no error, but got %v", err)
		}
		if len(updatedStatuses) != 2 {
			t.Fatalf("expected two status updates, but got %d", len(updatedStatuses))
		}
		if updatedStatuses[0] != model.ChatSessionFinished {
			t.Error("expected old session to be marked 'finished'")
		}
		if updatedStatuses[1] != model.ChatSessionActive {
			t.Error("expected new session to be marked 'active'")
		}
	})

	t.Run("DeleteSession should call repository delete", func(t *testing.T) {
		// Arrange
		deleteCalledWithID := ""
		mockChatRepo.DeleteFunc = func(ctx context.Context, tx repository.Tx, id string) error {
			deleteCalledWithID = id
			return nil
		}

		// Act
		err := uc.DeleteSession(ctx, "sess-to-delete")

		// Assert
		if err != nil {
			t.Fatalf("expected no error, but got %v", err)
		}
		if deleteCalledWithID != "sess-to-delete" {
			t.Error("repository Delete was not called with the correct session ID")
		}
	})
}

func TestChatUseCase_ListModels(t *testing.T) {
	ctx := context.Background()

	t.Run("should return empty list if user has no active subscription", func(t *testing.T) {
		// --- Arrange ---
		uc, _, mockSubRepo, _, _ := setupChatUCTestWithMocks()
		// CORRECT: We configure the underlying mock repository.
		mockSubRepo.FindActiveByUserFunc = func(ctx context.Context, tx repository.Tx, userID string) (*model.UserSubscription, error) {
			return nil, domain.ErrNotFound
		}

		// --- Act ---
		models, err := uc.ListModels(ctx, "user-with-no-sub")

		// --- Assert ---
		if err != nil {
			t.Fatalf("expected no error, but got %v", err)
		}
		if len(models) != 0 {
			t.Errorf("expected an empty list of models, but got %d", len(models))
		}
	})

	t.Run("should return empty list if plan's supported list is empty", func(t *testing.T) {
		// --- Arrange ---
		uc, _, mockSubRepo, mockPlanRepo, _ := setupChatUCTestWithMocks()
		mockSubRepo.FindActiveByUserFunc = func(ctx context.Context, tx repository.Tx, userID string) (*model.UserSubscription, error) {
			return &model.UserSubscription{PlanID: "empty-plan"}, nil
		}
		mockPlanRepo.FindByIDFunc = func(ctx context.Context, id string) (*model.SubscriptionPlan, error) {
			return &model.SubscriptionPlan{SupportedModels: []string{}}, nil // Plan has an empty list
		}

		// --- Act ---
		models, err := uc.ListModels(ctx, "user-1")

		// --- Assert ---
		if err != nil {
			t.Fatalf("expected no error, but got %v", err)
		}
		if len(models) != 0 {
			t.Errorf("expected an empty list of models, but got %d", len(models))
		}
	})

	t.Run("should return only models that are both supported by the plan and globally active", func(t *testing.T) {
		// --- Arrange ---
		uc, _, mockSubRepo, mockPlanRepo, mockPricingRepo := setupChatUCTestWithMocks()
		mockSubRepo.FindActiveByUserFunc = func(ctx context.Context, tx repository.Tx, userID string) (*model.UserSubscription, error) {
			return &model.UserSubscription{PlanID: "pro-plan"}, nil
		}
		mockPlanRepo.FindByIDFunc = func(ctx context.Context, id string) (*model.SubscriptionPlan, error) {
			return &model.SubscriptionPlan{SupportedModels: []string{"gpt-4o", "disabled-model"}}, nil
		}
		mockPricingRepo.ListActiveFunc = func(ctx context.Context) ([]*model.ModelPricing, error) {
			return []*model.ModelPricing{
				{ModelName: "gpt-4o", Active: true},
				{ModelName: "gemini-1.5-pro", Active: true},
			}, nil
		}

		// --- Act ---
		models, err := uc.ListModels(ctx, "user-1")

		// --- Assert ---
		if err != nil {
			t.Fatalf("expected no error, but got %v", err)
		}
		expected := []string{"gpt-4o"}
		// Use a helper to compare slices since order doesn't matter
		sort.Strings(models)
		sort.Strings(expected)
		if !reflect.DeepEqual(models, expected) {
			t.Errorf("mismatch in filtered models, want: %v, got: %v", expected, models)
		}
	})
}

// Helper function to reduce boilerplate in chat_uc_test.go
func setupChatUCTest() (usecase.ChatUseCase, *MockChatSessionRepo, *MockAIJobRepo) {
	mockChatRepo := NewMockChatSessionRepo()
	mockUserRepo := NewMockUserRepo()
	mockAIJobRepo := NewMockAIJobRepo()
	mockPricingRepo := NewMockModelPricingRepo()
	mockSubRepo := NewMockSubscriptionRepo()
	mockPlanRepo := NewMockPlanRepo()
	mockTxManager := NewMockTxManager()
	testLogger := newTestLogger()

	// Construct a real SubscriptionUseCase with its own mocks
	subUC := usecase.NewSubscriptionUseCase(mockSubRepo, mockPlanRepo, mockTxManager, testLogger)

	// Construct the ChatUseCase with its mocks
	uc := usecase.NewChatUseCase(mockChatRepo, mockUserRepo, nil, mockPricingRepo, mockAIJobRepo, nil, subUC, NewMockLocker(), mockTxManager, testLogger, false)
	return uc, mockChatRepo, mockAIJobRepo
}

// It returns the MOCKS that the use cases depend on, so tests can configure them.
func setupChatUCTestWithMocks() (usecase.ChatUseCase, *MockChatSessionRepo, *MockSubscriptionRepo, *MockPlanRepo, *MockModelPricingRepo) {
	mockChatRepo := NewMockChatSessionRepo()
	mockAIJobRepo := NewMockAIJobRepo()
	mockPricingRepo := NewMockModelPricingRepo()
	mockSubRepo := NewMockSubscriptionRepo()
	mockPlanRepo := NewMockPlanRepo()
	mockUserRepo := NewMockUserRepo()
	mockTxManager := NewMockTxManager()
	testLogger := newTestLogger()

	// Construct the REAL SubscriptionUseCase with its own mocks.
	subUC := usecase.NewSubscriptionUseCase(mockSubRepo, mockPlanRepo, mockTxManager, testLogger)

	// Construct the REAL ChatUseCase with its mocks and the real subUC.
	uc := usecase.NewChatUseCase(
		mockChatRepo,
		mockUserRepo,
		mockPlanRepo,
		mockPricingRepo,
		mockAIJobRepo,
		nil, // AI adapter is not needed for these tests
		subUC,
		NewMockLocker(),
		mockTxManager,
		testLogger,
		false,
	)
	return uc, mockChatRepo, mockSubRepo, mockPlanRepo, mockPricingRepo
}
