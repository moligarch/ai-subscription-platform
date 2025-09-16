//go:build integration

package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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
	server := NewServer(statsUC, userUC, subUC, nil, apiKey, &logger)

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
	server := NewServer(nil, userUC, subUC, nil, apiKey, &logger) // statsUC is not needed here

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
	server := NewServer(nil, nil, nil, planUC, apiKey, &logger)

	mux := http.NewServeMux()
	server.RegisterRoutes(mux)
	testServer := httptest.NewServer(mux)
	defer testServer.Close()

	t.Run("Success", func(t *testing.T) {
		// Define the plan to be created
		planPayload := `
		{
			"name": "API-Integration-Plan",
			"duration_days": 90,
			"credits": 500000,
			"price_irr": 250000,
			"supported_models": ["gpt-4o"]
		}`
		bodyReader := strings.NewReader(planPayload)

		// Act: Make the POST request
		req, _ := http.NewRequest("POST", testServer.URL+"/api/v1/plans", bodyReader)
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Content-Type", "application/json")

		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer res.Body.Close()

		// Assert: Check the HTTP response
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

		// Assert: Verify the plan was actually saved to the database
		savedPlan, err := planRepo.FindByID(ctx, nil, createdPlan.ID)
		if err != nil {
			t.Fatalf("Failed to find the created plan in the database: %v", err)
		}
		if savedPlan.Credits != 500000 {
			t.Errorf("Expected saved plan to have 500000 credits, got %d", savedPlan.Credits)
		}
	})

	t.Run("Failure for bad request", func(t *testing.T) {
		// Arrange: Malformed JSON (credits is a string instead of a number)
		planPayload := `{"name": "Bad-Plan", "credits": "fifty-thousand"}`
		bodyReader := strings.NewReader(planPayload)

		// Act: Make the POST request
		req, _ := http.NewRequest("POST", testServer.URL+"/api/v1/plans", bodyReader)
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Content-Type", "application/json")

		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer res.Body.Close()

		// Assert: Check the HTTP response status code
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
	server := NewServer(nil, nil, nil, planUC, apiKey, &logger)
	mux := http.NewServeMux()
	server.RegisterRoutes(mux)
	testServer := httptest.NewServer(mux)
	defer testServer.Close()

	// Define the update payload
	updatePayload := `{"name": "Updated Plan Name", "duration_days": 45, "credits": 200, "price_irr": 2000, "supported_models": ["gpt-4o"]}`
	bodyReader := strings.NewReader(updatePayload)

	// Act: Make the PUT request
	req, _ := http.NewRequest("PUT", testServer.URL+"/api/v1/plans/"+initialPlan.ID, bodyReader)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer res.Body.Close()

	// Assert: Check the HTTP response
	if res.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200 OK, got %d", res.StatusCode)
	}

	// Assert: Verify the plan was actually updated in the database
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
