//go:build !integration

package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/repository"
	"telegram-ai-subscription/internal/usecase"
	"testing"
)

// --- Mock Repositories (Ports) ---

type mockUserRepo struct {
	repository.UserRepository // Embed interface for forward compatibility
	mu                        sync.Mutex
	users                     []*model.User
	FindByIDError             error // To simulate errors
	ListError                 error
	CountError                error
}

func (m *mockUserRepo) List(ctx context.Context, tx repository.Tx, offset, limit int) ([]*model.User, error) {
	if m.ListError != nil {
		return nil, m.ListError
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	end := offset + limit
	if end > len(m.users) {
		end = len(m.users)
	}
	if offset >= len(m.users) || offset > end {
		return []*model.User{}, nil
	}
	return m.users[offset:end], nil
}

func (m *mockUserRepo) CountUsers(ctx context.Context, tx repository.Tx) (int, error) {
	if m.CountError != nil {
		return 0, m.CountError
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.users), nil
}

func (m *mockUserRepo) FindByID(ctx context.Context, tx repository.Tx, id string) (*model.User, error) {
	if m.FindByIDError != nil {
		return nil, m.FindByIDError
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, domain.ErrUserNotFound
}

type mockSubRepo struct {
	repository.SubscriptionRepository // Embed interface
	mu                                sync.Mutex
	subs                              []*model.UserSubscription
	ListByUserIDError                 error
}

func (m *mockSubRepo) ListByUserID(ctx context.Context, tx repository.Tx, userID string) ([]*model.UserSubscription, error) {
	if m.ListByUserIDError != nil {
		return nil, m.ListByUserIDError
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var userSubs []*model.UserSubscription
	for _, s := range m.subs {
		if s.UserID == userID {
			userSubs = append(userSubs, s)
		}
	}
	return userSubs, nil
}

func (m *mockSubRepo) CountActiveByPlan(ctx context.Context, tx repository.Tx) (map[string]int, error) {
	// For the test, we can just return an empty map.
	return make(map[string]int), nil
}

func (m *mockSubRepo) TotalRemainingCredits(ctx context.Context, tx repository.Tx) (int64, error) {
	// For the test, we can just return 0.
	return 0, nil
}

type mockPaymentRepo struct {
	repository.PaymentRepository // Embed interface
	SumByPeriodError             error
}

func (m *mockPaymentRepo) SumByPeriod(ctx context.Context, tx repository.Tx, period string) (int64, error) {
	if m.SumByPeriodError != nil {
		return 0, m.SumByPeriodError
	}
	switch period {
	case "week":
		return 100, nil
	case "month":
		return 1000, nil
	case "year":
		return 10000, nil
	}
	return 0, nil
}

type mockPlanRepo struct {
	repository.SubscriptionPlanRepository // Embed interface
	mu                                    sync.Mutex
	plans                                 []*model.SubscriptionPlan
	ListAllError                          error
}

func (m *mockPlanRepo) ListAll(ctx context.Context, tx repository.Tx) ([]*model.SubscriptionPlan, error) {
	if m.ListAllError != nil {
		return nil, m.ListAllError
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.plans, nil
}

// --- Handler Tests ---

func TestStatsHandler(t *testing.T) {
	// Arrange: Create real use case with mocked repositories
	userRepo := &mockUserRepo{}
	subRepo := &mockSubRepo{}
	paymentRepo := &mockPaymentRepo{}
	statsUC := usecase.NewStatsUseCase(userRepo, subRepo, paymentRepo, newTestLogger())

	t.Run("Success", func(t *testing.T) {
		handler := statsHandler(statsUC)
		req := httptest.NewRequest("GET", "/api/v1/stats", nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if status := rr.Code; status != http.StatusOK {
			t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
		}

		var resp map[string]interface{}
		json.Unmarshal(rr.Body.Bytes(), &resp)
		if resp["revenue_irr"].(map[string]interface{})["month"].(float64) != 1000 {
			t.Error("handler returned wrong revenue from mock repo")
		}
	})

	t.Run("Failure on Totals", func(t *testing.T) {
		userRepo.CountError = errors.New("db error") // Simulate an error
		handler := statsHandler(statsUC)
		req := httptest.NewRequest("GET", "/api/v1/stats", nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if status := rr.Code; status != http.StatusInternalServerError {
			t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusInternalServerError)
		}
		userRepo.CountError = nil // Reset for other tests
	})

	t.Run("Failure on Revenue", func(t *testing.T) {
		paymentRepo.SumByPeriodError = errors.New("db error") // Simulate an error
		handler := statsHandler(statsUC)
		req := httptest.NewRequest("GET", "/api/v1/stats", nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if status := rr.Code; status != http.StatusInternalServerError {
			t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusInternalServerError)
		}
		paymentRepo.SumByPeriodError = nil // Reset
	})
}

func TestUserHandlers(t *testing.T) {
	// Arrange for all user handler tests
	userRepo := &mockUserRepo{
		users: []*model.User{
			{ID: "user-1", FullName: "User One"},
			{ID: "user-2", FullName: "User Two"},
		},
	}
	subRepo := &mockSubRepo{
		subs: []*model.UserSubscription{
			{ID: "sub-1", UserID: "user-1"},
		},
	}
	userUC := usecase.NewUserUseCase(userRepo, nil, nil, nil, nil, newTestLogger())
	subUC := usecase.NewSubscriptionUseCase(subRepo, nil, nil, nil, newTestLogger())

	t.Run("usersListHandler success", func(t *testing.T) {
		handler := usersListHandler(userUC)
		req := httptest.NewRequest("GET", "/api/v1/users", nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if status := rr.Code; status != http.StatusOK {
			t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
		}
		var resp struct{ Total int }
		json.Unmarshal(rr.Body.Bytes(), &resp)
		if resp.Total != 2 {
			t.Errorf("expected total 2, got %d", resp.Total)
		}
	})

	t.Run("userGetHandler success", func(t *testing.T) {
		handler := userGetHandler(userUC, subUC)
		req := httptest.NewRequest("GET", "/api/v1/users/user-1", nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if status := rr.Code; status != http.StatusOK {
			t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
		}
		var resp struct {
			User          model.User `json:"user"`
			Subscriptions []model.UserSubscription
		}
		json.Unmarshal(rr.Body.Bytes(), &resp)
		if resp.User.ID != "user-1" {
			t.Error("handler returned wrong user")
		}
		if len(resp.Subscriptions) != 1 {
			t.Error("handler returned wrong number of subscriptions")
		}
	})

	t.Run("userGetHandler not found", func(t *testing.T) {
		handler := userGetHandler(userUC, subUC)
		req := httptest.NewRequest("GET", "/api/v1/users/user-does-not-exist", nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if status := rr.Code; status != http.StatusNotFound {
			t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusNotFound)
		}
	})
}

func TestPlansListHandler(t *testing.T) {
	// Arrange: Create real use case with mocked repositories
	planRepo := &mockPlanRepo{
		plans: []*model.SubscriptionPlan{
			{ID: "plan-1", Name: "Pro"},
			{ID: "plan-2", Name: "Standard"},
		},
	}
	// The List method in PlanUseCase only depends on the PlanRepository
	planUC := usecase.NewPlanUseCase(planRepo, nil, nil, newTestLogger())

	t.Run("Success", func(t *testing.T) {
		handler := plansListHandler(planUC)
		req := httptest.NewRequest("GET", "/api/v1/plans", nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if status := rr.Code; status != http.StatusOK {
			t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
		}

		var resp struct {
			Data []*model.SubscriptionPlan `json:"data"`
		}
		json.Unmarshal(rr.Body.Bytes(), &resp)
		if len(resp.Data) != 2 {
			t.Errorf("expected 2 plans, got %d", len(resp.Data))
		}
	})

	t.Run("Failure", func(t *testing.T) {
		planRepo.ListAllError = errors.New("database error")
		handler := plansListHandler(planUC)
		req := httptest.NewRequest("GET", "/api/v1/plans", nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if status := rr.Code; status != http.StatusInternalServerError {
			t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusInternalServerError)
		}
		planRepo.ListAllError = nil // Reset for other tests
	})
}
