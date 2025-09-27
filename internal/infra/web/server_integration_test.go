//go:build integration

package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/infra/db/postgres"
	"telegram-ai-subscription/internal/usecase"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// cleanup truncates all tables for this test package.
func cleanup(t *testing.T) {
	t.Helper()
	_, err := testPool.Exec(context.Background(), `
		TRUNCATE
			users, subscription_plans, user_subscriptions, payments, purchases,
			chat_sessions, chat_messages, ai_jobs, subscription_notifications,
			model_pricing, activation_codes
		RESTART IDENTITY CASCADE
	`)
	if err != nil {
		t.Fatalf("Failed to clean up database: %v", err)
	}
}

// helper: perform login and return the admin session cookie
func loginAndGetCookie(t *testing.T, baseURL, apiKey string) *http.Cookie {
	t.Helper()
	body := bytes.NewBufferString(`{"key":"` + apiKey + `"}`)
	req, _ := http.NewRequest(http.MethodPost, baseURL+"/api/v1/admin/auth/login", body)
	req.Header.Set("content-type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("login request failed: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 on login, got %d", res.StatusCode)
	}
	var sessionCookie *http.Cookie
	for _, c := range res.Cookies() {
		if c.Name == "admin_session" {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil || sessionCookie.Value == "" {
		t.Fatal("expected admin_session cookie from login")
	}
	return sessionCookie
}

func TestStatsAPI_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode.")
	}

	// 1. Setup
	defer cleanup(t)
	ctx := context.Background()
	logger := zerolog.New(nil)
	const apiKey = "integration-test-key"

	// Repositories use the shared testPool (set up in TestMain).
	userRepo := postgres.NewUserRepo(testPool)
	planRepo := postgres.NewPlanRepo(testPool)
	subRepo := postgres.NewSubscriptionRepo(testPool)
	paymentRepo := postgres.NewPaymentRepo(testPool)

	// Seed Data
	user, _ := model.NewUser("", 123, "testuser")
	_ = userRepo.Save(ctx, nil, user)

	plan, _ := model.NewSubscriptionPlan("", "Pro", 30, 1000, 50000)
	_ = planRepo.Save(ctx, nil, plan)

	now := time.Now()
	payment := &model.Payment{
		ID:       uuid.NewString(),
		UserID:   user.ID,
		PlanID:   plan.ID,
		Status:   model.PaymentStatusSucceeded,
		Amount:   50000,
		Currency: "IRR",
		PaidAt:   &now,
	}
	_ = paymentRepo.Save(ctx, nil, payment)

	// Usecase and Server
	statsUC := usecase.NewStatsUseCase(userRepo, subRepo, paymentRepo, &logger)
	userUC := usecase.NewUserUseCase(userRepo, nil, nil, nil, nil, nil, &logger)
	subUC := usecase.NewSubscriptionUseCase(subRepo, planRepo, nil, nil, &logger)

	// Auth manager for session-based auth (no TLS in tests → secure=false).
	auth := NewAuthManager("integration-admin-jwt-secret", false, "", 15*time.Minute)
	server := NewServer(statsUC, userUC, subUC, nil, apiKey, auth, &logger)

	// HTTP Test Server
	mux := http.NewServeMux()
	server.RegisterRoutes(mux)
	testServer := httptest.NewServer(mux)
	defer testServer.Close()

	// Login to obtain session cookie
	sessionCookie := loginAndGetCookie(t, testServer.URL, apiKey)

	t.Run("Success with valid session", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, testServer.URL+"/api/v1/stats", nil)
		req.AddCookie(sessionCookie)

		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200 OK, got %d", res.StatusCode)
		}

		var body map[string]interface{}
		_ = json.NewDecoder(res.Body).Decode(&body)

		if body["total_users"].(float64) != 1 {
			t.Errorf("Expected 1 total user, got %v", body["total_users"])
		}
		if body["revenue_irr"].(map[string]interface{})["month"].(float64) != 50000 {
			t.Errorf("Expected month revenue of 50000, got %v", body["revenue_irr"])
		}
	})

	t.Run("Failure with invalid bearer jwt -> 401", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, testServer.URL+"/api/v1/stats", nil)
		req.Header.Set("Authorization", "Bearer invalid.jwt.token")

		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusUnauthorized {
			t.Errorf("Expected status 401 Unauthorized, got %d", res.StatusCode)
		}
	})
}

func TestUsersListAPI_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode.")
	}
	defer cleanup(t)
	ctx := context.Background()
	logger := zerolog.New(nil)
	const apiKey = "integration-test-key"

	// Repositories
	userRepo := postgres.NewUserRepo(testPool)
	planRepo := postgres.NewPlanRepo(testPool)
	subRepo := postgres.NewSubscriptionRepo(testPool)

	// Create 3 users using the constructor and check for errors on save.
	for i := 1; i <= 3; i++ {
		user, err := model.NewUser("", int64(i), fmt.Sprintf("testuser%d", i))
		if err != nil {
			t.Fatalf("model.NewUser() failed: %v", err)
		}
		if err := userRepo.Save(ctx, nil, user); err != nil {
			t.Fatalf("userRepo.Save() failed for user %d: %v", i, err)
		}
	}

	// Usecase and Server
	userUC := usecase.NewUserUseCase(userRepo, nil, nil, nil, nil, nil, &logger)
	subUC := usecase.NewSubscriptionUseCase(subRepo, planRepo, nil, nil, &logger)

	auth := NewAuthManager("integration-admin-jwt-secret", false, "", 15*time.Minute)
	server := NewServer(nil, userUC, subUC, nil, apiKey, auth, &logger) // statsUC, planUC not needed here

	// HTTP Test Server
	mux := http.NewServeMux()
	server.RegisterRoutes(mux)
	testServer := httptest.NewServer(mux)
	defer testServer.Close()

	// Login to obtain session cookie
	sessionCookie := loginAndGetCookie(t, testServer.URL, apiKey)

	// Act: Make the request with cookie
	req, _ := http.NewRequest("GET", testServer.URL+"/api/v1/users?limit=2&offset=1", nil)
	req.AddCookie(sessionCookie)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer res.Body.Close()

	// Assert
	if res.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200 OK, got %d", res.StatusCode)
	}

	var body struct {
		Data   []*model.User `json:"data"`
		Total  int           `json:"total"`
		Limit  int           `json:"limit"`
		Offset int           `json:"offset"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if body.Total != 3 {
		t.Errorf("Expected total=3, got %d", body.Total)
	}
	if len(body.Data) != 2 {
		t.Errorf("Expected 2 users in data array, got %d", len(body.Data))
	}
	if body.Offset != 1 {
		t.Errorf("Expected offset=1, got %d", body.Offset)
	}
}

func TestPlansCreateAPI_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode.")
	}
	defer cleanup(t)
	ctx := context.Background()
	logger := zerolog.New(nil)
	const apiKey = "integration-test-key"

	// Arrange: Setup repositories, use cases, and the test server
	planRepo := postgres.NewPlanRepo(testPool)
	planUC := usecase.NewPlanUseCase(planRepo, nil, nil, &logger)

	auth := NewAuthManager("integration-admin-jwt-secret", false, "", 15*time.Minute)
	server := NewServer(nil, nil, nil, planUC, apiKey, auth, &logger)

	mux := http.NewServeMux()
	server.RegisterRoutes(mux)
	testServer := httptest.NewServer(mux)
	defer testServer.Close()

	// Login to obtain session cookie
	sessionCookie := loginAndGetCookie(t, testServer.URL, apiKey)

	t.Run("Success", func(t *testing.T) {
		planPayload := `
		{
			"name": "API-Integration-Plan",
			"duration_days": 90,
			"credits": 500000,
			"price_irr": 250000,
			"supported_models": ["gpt-4o"]
		}`
		bodyReader := strings.NewReader(planPayload)

		req, _ := http.NewRequest("POST", testServer.URL+"/api/v1/plans", bodyReader)
		req.AddCookie(sessionCookie)
		req.Header.Set("Content-Type", "application/json")

		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusCreated {
			t.Fatalf("Expected status 201 Created, got %d", res.StatusCode)
		}

		var createdPlan model.SubscriptionPlan
		if err := json.NewDecoder(res.Body).Decode(&createdPlan); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if createdPlan.Name != "API-Integration-Plan" {
			t.Errorf("Expected plan name to be 'API-Integration-Plan', got '%s'", createdPlan.Name)
		}
		if createdPlan.ID == "" {
			t.Error("Expected a plan ID in the response, but it was empty")
		}

		// Verify saved in DB
		savedPlan, err := planRepo.FindByID(ctx, nil, createdPlan.ID)
		if err != nil {
			t.Fatalf("Failed to find the created plan in the database: %v", err)
		}
		if savedPlan.Credits != 500000 {
			t.Errorf("Expected saved plan to have 500000 credits, got %d", savedPlan.Credits)
		}
	})

	t.Run("Failure for bad request", func(t *testing.T) {
		planPayload := `{"name": "Bad-Plan", "credits": "fifty-thousand"}`
		bodyReader := strings.NewReader(planPayload)

		req, _ := http.NewRequest("POST", testServer.URL+"/api/v1/plans", bodyReader)
		req.AddCookie(sessionCookie)
		req.Header.Set("Content-Type", "application/json")

		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected status 400 Bad Request, got %d", res.StatusCode)
		}
	})
}

func TestPlansUpdateAPI_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode.")
	}
	defer cleanup(t)
	ctx := context.Background()
	logger := zerolog.New(nil)
	const apiKey = "integration-test-key"

	// Arrange: Setup and seed an initial plan
	planRepo := postgres.NewPlanRepo(testPool)
	initialPlan, _ := model.NewSubscriptionPlan("", "Initial Plan", 30, 100, 1000)
	if err := planRepo.Save(ctx, nil, initialPlan); err != nil {
		t.Fatalf("failed to save initial plan: %v", err)
	}

	planUC := usecase.NewPlanUseCase(planRepo, nil, nil, &logger)

	auth := NewAuthManager("integration-admin-jwt-secret", false, "", 15*time.Minute)
	server := NewServer(nil, nil, nil, planUC, apiKey, auth, &logger)

	mux := http.NewServeMux()
	server.RegisterRoutes(mux)
	testServer := httptest.NewServer(mux)
	defer testServer.Close()

	// Login to obtain session cookie
	sessionCookie := loginAndGetCookie(t, testServer.URL, apiKey)

	updatePayload := `{"name": "Updated Plan Name", "duration_days": 45, "credits": 200, "price_irr": 2000, "supported_models": ["gpt-4o"]}`
	bodyReader := strings.NewReader(updatePayload)

	req, _ := http.NewRequest("PUT", testServer.URL+"/api/v1/plans/"+initialPlan.ID, bodyReader)
	req.AddCookie(sessionCookie)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200 OK, got %d", res.StatusCode)
	}

	// Verify the plan was updated
	savedPlan, err := planRepo.FindByID(ctx, nil, initialPlan.ID)
	if err != nil {
		t.Fatalf("Failed to find the updated plan in the database: %v", err)
	}
	if savedPlan.Name != "Updated Plan Name" {
		t.Errorf("Expected saved plan name to be 'Updated Plan Name', got '%s'", savedPlan.Name)
	}
	if savedPlan.DurationDays != 45 {
		t.Errorf("Expected saved plan duration to be 45, got %d", savedPlan.DurationDays)
	}
}

func TestPlansDeleteAPI_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode.")
	}
	defer cleanup(t)
	ctx := context.Background()
	logger := zerolog.New(nil)
	const apiKey = "integration-test-key"

	// Arrange: Setup repositories
	planRepo := postgres.NewPlanRepo(testPool)
	userRepo := postgres.NewUserRepo(testPool)
	subRepo := postgres.NewSubscriptionRepo(testPool)

	// Seed Data for a successful deletion
	planToDelete, _ := model.NewSubscriptionPlan("", "To Delete", 30, 100, 1000)
	if err := planRepo.Save(ctx, nil, planToDelete); err != nil {
		t.Fatalf("failed to save planToDelete: %v", err)
	}

	// Seed Data for a conflict scenario
	planInUse, _ := model.NewSubscriptionPlan("", "In Use", 30, 100, 1000)
	user, _ := model.NewUser("", 123, "sub-user")
	if err := planRepo.Save(ctx, nil, planInUse); err != nil {
		t.Fatalf("failed to save planInUse: %v", err)
	}
	if err := userRepo.Save(ctx, nil, user); err != nil {
		t.Fatalf("failed to save user: %v", err)
	}
	sub, _ := model.NewUserSubscription(uuid.NewString(), user.ID, planInUse)
	if err := subRepo.Save(ctx, nil, sub); err != nil {
		t.Fatalf("failed to save subscription: %v", err)
	}

	planUC := usecase.NewPlanUseCase(planRepo, nil, nil, &logger)

	auth := NewAuthManager("integration-admin-jwt-secret", false, "", 15*time.Minute)
	server := NewServer(nil, nil, nil, planUC, apiKey, auth, &logger)

	mux := http.NewServeMux()
	server.RegisterRoutes(mux)
	testServer := httptest.NewServer(mux)
	defer testServer.Close()

	// Login to obtain session cookie
	sessionCookie := loginAndGetCookie(t, testServer.URL, apiKey)

	t.Run("Success", func(t *testing.T) {
		req, _ := http.NewRequest("DELETE", testServer.URL+"/api/v1/plans/"+planToDelete.ID, nil)
		req.AddCookie(sessionCookie)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusNoContent {
			t.Fatalf("Expected status 204 No Content, got %d", res.StatusCode)
		}

		// Verify deletion from database
		_, err = planRepo.FindByID(ctx, nil, planToDelete.ID)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Error("Expected plan to be deleted from DB, but it was found")
		}
	})

	t.Run("Failure for plan in use", func(t *testing.T) {
		req, _ := http.NewRequest("DELETE", testServer.URL+"/api/v1/plans/"+planInUse.ID, nil)
		req.AddCookie(sessionCookie)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusConflict {
			t.Fatalf("Expected status 409 Conflict, got %d", res.StatusCode)
		}
	})
}
