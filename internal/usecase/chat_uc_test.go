package usecase

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"

	"telegram-ai-subscription/internal/config"
	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/adapter"
	"telegram-ai-subscription/internal/domain/ports/repository"
	"telegram-ai-subscription/internal/infra/logging"
	red "telegram-ai-subscription/internal/infra/redis"
)

// ---- Fakes ----

type fakeAI struct{}

func (f *fakeAI) ListModels(ctx context.Context) ([]string, error) {
	return []string{"gpt-4o-mini"}, nil
}
func (f *fakeAI) GetModelInfo(model string) (adapter.ModelInfo, error) {
	return adapter.ModelInfo{Name: model}, nil
}
func (f *fakeAI) Chat(ctx context.Context, model string, messages []adapter.Message) (string, error) {
	return "ok", nil
}

type fakeSubs struct{}

// FinishExpired implements SubscriptionUseCase.
func (f *fakeSubs) FinishExpired(ctx context.Context) (int, error) {
	return 0, nil
}

// GetActive implements SubscriptionUseCase.
func (f *fakeSubs) GetActive(ctx context.Context, userID string) (*model.UserSubscription, error) {
	return nil, nil
}

// GetReserved implements SubscriptionUseCase.
func (f *fakeSubs) GetReserved(ctx context.Context, userID string) ([]*model.UserSubscription, error) {
	panic("unimplemented")
}

// Subscribe implements SubscriptionUseCase.
func (f *fakeSubs) Subscribe(ctx context.Context, userID string, planID string) (*model.UserSubscription, error) {
	panic("unimplemented")
}

func (f *fakeSubs) DeductCredits(ctx context.Context, userID string, amount int) (*model.UserSubscription, error) {
	return &model.UserSubscription{}, nil
}

type memChatRepo struct {
	mu       sync.Mutex
	activeBy map[string]*model.ChatSession
	byID     map[string]*model.ChatSession
}

// CleanupOldMessages implements repository.ChatSessionRepository.
func (m *memChatRepo) CleanupOldMessages(ctx context.Context, userID string, retentionDays int) (int64, error) {
	return 0, nil
}

// Delete implements repository.ChatSessionRepository.
func (m *memChatRepo) Delete(ctx context.Context, qx any, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s := m.byID[id]; s != nil {
		delete(m.byID, id)
		if s.Status == model.ChatSessionActive {
			delete(m.activeBy, s.UserID)
		}
		return nil
	}
	return domain.ErrNotFound
}

// FindAllByUser implements repository.ChatSessionRepository.
func (m *memChatRepo) FindAllByUser(ctx context.Context, qx any, userID string) ([]*model.ChatSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var sessions []*model.ChatSession
	for _, s := range m.byID {
		if s.UserID == userID {
			sessions = append(sessions, s)
		}
	}
	return sessions, nil
}

func newMemChatRepo() *memChatRepo {
	return &memChatRepo{
		activeBy: map[string]*model.ChatSession{},
		byID:     map[string]*model.ChatSession{},
	}
}

func (m *memChatRepo) Save(ctx context.Context, tx any, s *model.ChatSession) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.byID[s.ID] = s
	if s.Status == model.ChatSessionActive {
		m.activeBy[s.UserID] = s
	}
	return nil
}

func (m *memChatRepo) SaveMessage(ctx context.Context, tx any, msg *model.ChatMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s := m.byID[msg.SessionID]; s != nil {
		s.Messages = append(s.Messages, *msg)
		return nil
	}
	return domain.ErrNotFound
}
func (m *memChatRepo) FindActiveByUser(ctx context.Context, tx any, userID string) (*model.ChatSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s := m.activeBy[userID]; s != nil && s.Status == model.ChatSessionActive {
		return s, nil
	}
	return nil, domain.ErrNotFound
}
func (m *memChatRepo) FindByID(ctx context.Context, tx any, id string) (*model.ChatSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s := m.byID[id]; s != nil {
		return s, nil
	}
	return nil, domain.ErrNotFound
}
func (m *memChatRepo) UpdateStatus(ctx context.Context, tx any, id string, st model.ChatSessionStatus) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s := m.byID[id]; s != nil {
		s.Status = st
		if st != model.ChatSessionActive {
			delete(m.activeBy, s.UserID)
		}
		return nil
	}
	return domain.ErrNotFound
}

// Ensure memChatRepo implements repository.ChatSessionRepository at compile-time
var _ repository.ChatSessionRepository = (*memChatRepo)(nil)

func RedisTestConfig() config.RedisConfig {
	return config.RedisConfig{
		URL: "localhost:6379",
		DB:  1,
	}
}

// ---- Test ----

func TestStartChat_Concurrent(t *testing.T) {
	ctx := context.Background()

	// Real Redis locker (make sure Redis is running at your cfg.Redis.URL).
	// For test, it's okay to connect to default localhost:6379/db=1.
	cfg := RedisTestConfig()
	cli, err := red.NewClient(ctx, &cfg) // helper below
	if err != nil {
		t.Skip("redis not available:", err)
	}
	locker := red.NewLocker(cli)

	repo := newMemChatRepo()
	ai := &fakeAI{}
	subs := &fakeSubs{}
	log := logging.New(config.LogConfig{Level: "debug", Format: "console"}, true)
	uc := NewChatUseCase(repo, ai, subs, locker, log, true)

	userID := uuid.NewString()
	const K = 32 // 32 concurrent attempts
	wg := sync.WaitGroup{}
	wg.Add(K)

	var success int64
	var exists int64
	var other int64
	mu := sync.Mutex{}

	for i := 0; i < K; i++ {
		go func() {
			defer wg.Done()
			_, err := uc.StartChat(ctx, userID, "gpt-4o-mini")
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				success++
			} else if errors.Is(err, domain.ErrActiveChatExists) {
				exists++
			} else {
				other++
			}
		}()
	}
	wg.Wait()

	if success != 1 {
		t.Fatalf("expected exactly 1 success, got %d (exists=%d other=%d)", success, exists, other)
	}
	if exists != K-1 {
		t.Fatalf("expected %d ErrActiveChatExists, got %d (other=%d)", K-1, exists, other)
	}

	// small sanity: active session is present
	s, err := repo.FindActiveByUser(ctx, nil, userID)
	if err != nil || s == nil {
		t.Fatalf("no active session found after concurrency test")
	}
}
