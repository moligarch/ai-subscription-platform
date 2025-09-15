//go:build integration

package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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

func TestStatsAPI_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode.")
	}

	// 1. Setup
	defer cleanup(t)
	ctx := context.Background()
	logger := zerolog.New(nil)
	const apiKey = "integration-test-key"

	// Repositories now use the pool from this package's TestMain
	userRepo := postgres.NewUserRepo(testPool)
	planRepo := postgres.NewPlanRepo(testPool)
	subRepo := postgres.NewSubscriptionRepo(testPool)
	paymentRepo := postgres.NewPaymentRepo(testPool)

	// Seed Data
	user, _ := model.NewUser("", 123, "testuser")
	userRepo.Save(ctx, nil, user)

	plan, _ := model.NewSubscriptionPlan("", "Pro", 30, 1000, 50000)
	planRepo.Save(ctx, nil, plan)

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
	paymentRepo.Save(ctx, nil, payment)

	// Usecase and Server
	statsUC := usecase.NewStatsUseCase(userRepo, subRepo, paymentRepo, &logger)
	userUC := usecase.NewUserUseCase(userRepo, nil, nil, nil, nil, &logger)
	subUC := usecase.NewSubscriptionUseCase(subRepo, planRepo, nil, nil, &logger)
	server := NewServer(statsUC, userUC, subUC, apiKey, &logger)

	// HTTP Test Server
	mux := http.NewServeMux()
	server.RegisterRoutes(mux)
	testServer := httptest.NewServer(mux)
	defer testServer.Close()

	t.Run("Success with valid token", func(t *testing.T) {
		// Arrange
		req, _ := http.NewRequest("GET", testServer.URL+"/api/v1/stats", nil)
		req.Header.Set("Authorization", "Bearer "+apiKey)

		// Act
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer res.Body.Close()

		// Assert
		if res.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200 OK, got %d", res.StatusCode)
		}

		var body map[string]interface{}
		json.NewDecoder(res.Body).Decode(&body)

		if body["total_users"].(float64) != 1 {
			t.Errorf("Expected 1 total user, got %v", body["total_users"])
		}
		if body["revenue_irr"].(map[string]interface{})["month"].(float64) != 50000 {
			t.Errorf("Expected month revenue of 50000, got %v", body["revenue_irr"])
		}
	})

	t.Run("Failure with invalid token", func(t *testing.T) {
		// Arrange
		req, _ := http.NewRequest("GET", testServer.URL+"/api/v1/stats", nil)
		req.Header.Set("Authorization", "Bearer invalid-key")

		// Act
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer res.Body.Close()

		// Assert
		if res.StatusCode != http.StatusForbidden {
			t.Errorf("Expected status 403 Forbidden, got %d", res.StatusCode)
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
	userUC := usecase.NewUserUseCase(userRepo, nil, nil, nil, nil, &logger)
	subUC := usecase.NewSubscriptionUseCase(subRepo, planRepo, nil, nil, &logger)
	server := NewServer(nil, userUC, subUC, apiKey, &logger) // statsUC is not needed here

	// HTTP Test Server
	mux := http.NewServeMux()
	server.RegisterRoutes(mux)
	testServer := httptest.NewServer(mux)
	defer testServer.Close()

	// Act: Make the request
	req, _ := http.NewRequest("GET", testServer.URL+"/api/v1/users?limit=2&offset=1", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
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
