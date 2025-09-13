//go:build !integration

package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/usecase"
	"testing"
	"time"
)

// mockStatsUseCase allows us to simulate the behavior of the real use case.
type mockStatsUseCase struct {
	TotalsFunc        func(ctx context.Context) (int, map[string]int, int64, error)
	RevenueFunc       func(ctx context.Context) (int64, int64, int64, error)
	InactiveUsersFunc func(ctx context.Context, olderThan time.Time) (int, error)
}

func (m *mockStatsUseCase) Totals(ctx context.Context) (int, map[string]int, int64, error) {
	return m.TotalsFunc(ctx)
}
func (m *mockStatsUseCase) Revenue(ctx context.Context) (int64, int64, int64, error) {
	return m.RevenueFunc(ctx)
}
func (m *mockStatsUseCase) InactiveUsers(ctx context.Context, olderThan time.Time) (int, error) {
	return m.InactiveUsersFunc(ctx, olderThan)
}

func TestStatsHandler(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Arrange
		mockUC := &mockStatsUseCase{
			TotalsFunc: func(ctx context.Context) (int, map[string]int, int64, error) {
				return 10, map[string]int{"pro": 5}, 1000, nil
			},
			RevenueFunc: func(ctx context.Context) (int64, int64, int64, error) {
				return 100, 1000, 10000, nil
			},
		}
		handler := statsHandler(mockUC)
		req := httptest.NewRequest("GET", "/api/v1/stats", nil)
		rr := httptest.NewRecorder()

		// Act
		handler.ServeHTTP(rr, req)

		// Assert
		if status := rr.Code; status != http.StatusOK {
			t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
		}

		var responseBody map[string]interface{}
		if err := json.Unmarshal(rr.Body.Bytes(), &responseBody); err != nil {
			t.Fatalf("Could not parse response JSON: %v", err)
		}

		if responseBody["total_users"].(float64) != 10 {
			t.Errorf("incorrect total_users in response")
		}
		if responseBody["total_remaining_credits"].(float64) != 1000 {
			t.Errorf("incorrect total_remaining_credits in response")
		}
	})

	t.Run("Failure on Totals", func(t *testing.T) {
		// Arrange
		mockUC := &mockStatsUseCase{
			TotalsFunc: func(ctx context.Context) (int, map[string]int, int64, error) {
				return 0, nil, 0, errors.New("db error")
			},
		}
		handler := statsHandler(mockUC)
		req := httptest.NewRequest("GET", "/api/v1/stats", nil)
		rr := httptest.NewRecorder()

		// Act
		handler.ServeHTTP(rr, req)

		// Assert
		if status := rr.Code; status != http.StatusInternalServerError {
			t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusInternalServerError)
		}
	})
}

type mockUserUseCase struct {
	// Embed the interface to avoid having to implement every method.
	usecase.UserUseCase
	ListFunc  func(ctx context.Context, offset, limit int) ([]*model.User, error)
	CountFunc func(ctx context.Context) (int, error)
}

func (m *mockUserUseCase) List(ctx context.Context, offset, limit int) ([]*model.User, error) {
	return m.ListFunc(ctx, offset, limit)
}
func (m *mockUserUseCase) Count(ctx context.Context) (int, error) {
	return m.CountFunc(ctx)
}

func TestUsersListHandler(t *testing.T) {
	t.Run("Success with default pagination", func(t *testing.T) {
		// Arrange
		mockUC := &mockUserUseCase{
			ListFunc: func(ctx context.Context, offset, limit int) ([]*model.User, error) {
				if offset != 0 || limit != 50 {
					t.Errorf("expected default offset=0, limit=50, but got offset=%d, limit=%d", offset, limit)
				}
				return []*model.User{{ID: "user-1"}}, nil
			},
			CountFunc: func(ctx context.Context) (int, error) {
				return 1, nil
			},
		}

		handler := usersListHandler(mockUC)
		req := httptest.NewRequest("GET", "/api/v1/users", nil)
		rr := httptest.NewRecorder()

		// Act
		handler.ServeHTTP(rr, req)

		// Assert
		if status := rr.Code; status != http.StatusOK {
			t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
		}

		var resp struct {
			Data  []*model.User `json:"data"`
			Total int           `json:"total"`
		}
		json.Unmarshal(rr.Body.Bytes(), &resp)

		if resp.Total != 1 {
			t.Errorf("expected total 1, got %d", resp.Total)
		}
		if len(resp.Data) != 1 {
			t.Errorf("expected 1 user in data array, got %d", len(resp.Data))
		}
	})
}
