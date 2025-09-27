//go:build !integration

package web

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/usecase"

	"github.com/rs/zerolog"
)

// newTestLogger creates a silent logger for tests.
func newTestLogger() *zerolog.Logger {
	logger := zerolog.New(nil)
	return &logger
}

// ---- minimal mock PlanUseCase used by protected /api/v1/plans route ----
type mockPlanUC struct{}

func (m *mockPlanUC) Create(ctx context.Context, name string, durationDays int, credits int64, priceIRR int64, supportedModels []string) (*model.SubscriptionPlan, error) {
	return &model.SubscriptionPlan{ID: "p1", Name: name}, nil
}
func (m *mockPlanUC) Update(ctx context.Context, plan *model.SubscriptionPlan) error { return nil }
func (m *mockPlanUC) List(ctx context.Context) ([]*model.SubscriptionPlan, error)   { return []*model.SubscriptionPlan{}, nil }
func (m *mockPlanUC) Get(ctx context.Context, id string) (*model.SubscriptionPlan, error) {
	return &model.SubscriptionPlan{ID: id, Name: "n"}, nil
}
func (m *mockPlanUC) Delete(ctx context.Context, id string) error { return nil }
func (m *mockPlanUC) UpdatePricing(ctx context.Context, modelName string, in, out int64) error {
	return nil
}
func (m *mockPlanUC) GenerateActivationCodes(ctx context.Context, planID string, count int) ([]string, error) {
	return []string{"CODE-1"}, nil
}

func TestAuthMiddleware(t *testing.T) {
	// A simple handler that we expect to be called on successful authentication.
	dummyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	logger := newTestLogger()
	auth := NewAuthManager("test-admin-jwt-secret-please-change", false, "", time.Minute)

	// We don't need real UCs for this middleware test.
	var mockStatsUC usecase.StatsUseCase // nil is fine
	server := NewServer(mockStatsUC, nil, nil, nil, "test-admin-key", auth, logger)
	protected := server.authMiddleware(dummyHandler)

	t.Run("no credentials -> 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
		rr := httptest.NewRecorder()
		protected.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rr.Code)
		}
	})

	t.Run("malformed Authorization header (no scheme) -> 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
		req.Header.Set("Authorization", "whatever-token")
		rr := httptest.NewRecorder()
		protected.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rr.Code)
		}
	})

	t.Run("wrong scheme -> 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
		req.Header.Set("Authorization", "Basic aaa.bbb.ccc")
		rr := httptest.NewRecorder()
		protected.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rr.Code)
		}
	})

	t.Run("bearer but invalid jwt -> 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
		req.Header.Set("Authorization", "Bearer invalid.jwt.token")
		rr := httptest.NewRecorder()
		protected.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rr.Code)
		}
	})

	t.Run("valid bearer jwt -> 200", func(t *testing.T) {
		dummy := httptest.NewRecorder()
		token, err := auth.Mint(dummy)
		if err != nil || token == "" {
			t.Fatalf("failed to mint test token: %v", err)
		}
		req := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		protected.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
	})

	t.Run("valid session cookie -> 200", func(t *testing.T) {
		dummy := httptest.NewRecorder()
		token, err := auth.Mint(dummy)
		if err != nil || token == "" {
			t.Fatalf("failed to mint test token: %v", err)
		}
		req := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
		req.AddCookie(&http.Cookie{Name: "admin_session", Value: token})
		rr := httptest.NewRecorder()
		protected.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
	})

	t.Run("no auth manager configured -> 401", func(t *testing.T) {
		serverNoAuth := NewServer(nil, nil, nil, nil, "test-admin-key", nil, logger)
		protectedNoAuth := serverNoAuth.authMiddleware(dummyHandler)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
		rr := httptest.NewRecorder()
		protectedNoAuth.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rr.Code)
		}
	})
}

func TestAdminLoginLogoutFlow(t *testing.T) {
	logger := newTestLogger()
	auth := NewAuthManager("test-admin-jwt-secret-please-change", false, "", time.Minute)

	// Provide a minimal planUC so we can call /api/v1/plans safely.
	planUC := &mockPlanUC{}

	s := NewServer(nil, nil, nil, planUC, "test-admin-key", auth, logger)

	mux := http.NewServeMux()
	s.RegisterRoutes(mux)

	var sessionCookie *http.Cookie

	t.Run("login with wrong key -> 401", func(t *testing.T) {
		body := bytes.NewBufferString(`{"key":"wrong"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/auth/login", body)
		req.Header.Set("content-type", "application/json")
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rr.Code)
		}
	})

	t.Run("login with correct key -> 204 + cookie set", func(t *testing.T) {
		body := bytes.NewBufferString(`{"key":"test-admin-key"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/auth/login", body)
		req.Header.Set("content-type", "application/json")
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		if rr.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d", rr.Code)
		}
		for _, c := range rr.Result().Cookies() {
			if c.Name == "admin_session" {
				sessionCookie = c
				break
			}
		}
		if sessionCookie == nil || sessionCookie.Value == "" {
			t.Fatal("expected admin_session cookie")
		}
	})

	t.Run("protected route with cookie -> 200", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/plans", nil)
		req.AddCookie(sessionCookie)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
	})

	t.Run("logout -> 204", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/auth/logout", nil)
		req.AddCookie(sessionCookie) // optional
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		if rr.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d", rr.Code)
		}
	})

	t.Run("after logout without cookie -> 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/plans", nil)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rr.Code)
		}
	})
}
