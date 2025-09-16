//go:build !integration

package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/usecase"
	"testing"

	"github.com/google/uuid"
)

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
		plans: map[string]*model.SubscriptionPlan{ // MODIFIED: Was a slice, now a map.
			"plan-1": {ID: "plan-1", Name: "Pro"},
			"plan-2": {ID: "plan-2", Name: "Standard"},
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

func TestPlansCreateHandler(t *testing.T) {
	// Arrange for all subtests
	planRepo := &mockPlanRepo{
		plans: make(map[string]*model.SubscriptionPlan),
	}
	planUC := usecase.NewPlanUseCase(planRepo, nil, nil, newTestLogger())
	handler := plansCreateHandler(planUC)

	t.Run("Success", func(t *testing.T) {
		planPayload := `{"name": "New-Unit-Plan", "duration_days": 15, "credits": 100, "price_irr": 10000}`
		bodyReader := strings.NewReader(planPayload)

		req := httptest.NewRequest("POST", "/api/v1/plans", bodyReader)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if status := rr.Code; status != http.StatusCreated {
			t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusCreated)
		}

		var createdPlan model.SubscriptionPlan
		json.Unmarshal(rr.Body.Bytes(), &createdPlan)
		if createdPlan.Name != "New-Unit-Plan" {
			t.Errorf("handler returned wrong plan name: got %s", createdPlan.Name)
		}
	})

	t.Run("Failure for bad JSON", func(t *testing.T) {
		planPayload := `{"name": "Bad-JSON",` // Intentionally broken JSON
		bodyReader := strings.NewReader(planPayload)

		req := httptest.NewRequest("POST", "/api/v1/plans", bodyReader)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if status := rr.Code; status != http.StatusBadRequest {
			t.Errorf("handler returned wrong status code for bad json: got %v want %v", status, http.StatusBadRequest)
		}
	})

	t.Run("Failure for invalid data", func(t *testing.T) {
		// Payload is valid JSON, but has no name, which the use case will reject.
		planPayload := `{"duration_days": 15, "credits": 100, "price_irr": 10000}`
		bodyReader := strings.NewReader(planPayload)

		req := httptest.NewRequest("POST", "/api/v1/plans", bodyReader)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if status := rr.Code; status != http.StatusBadRequest {
			t.Errorf("handler returned wrong status code for invalid data: got %v want %v", status, http.StatusBadRequest)
		}
	})
}

func TestPlansUpdateHandler(t *testing.T) {
	// Arrange for all subtests
	planID := uuid.NewString()
	planRepo := &mockPlanRepo{
		plans: map[string]*model.SubscriptionPlan{
			planID: {ID: planID, Name: "Old Name", PriceIRR: 100},
		},
	}
	planUC := usecase.NewPlanUseCase(planRepo, nil, nil, newTestLogger())
	handler := plansUpdateHandler(planUC)

	t.Run("Success", func(t *testing.T) {
		updatePayload := `{"name": "New Name", "price_irr": 200, "duration_days": 30, "credits": 100}`
		bodyReader := strings.NewReader(updatePayload)
		req := httptest.NewRequest("PUT", "/api/v1/plans/"+planID, bodyReader)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if status := rr.Code; status != http.StatusOK {
			t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
		}

		var updatedPlan model.SubscriptionPlan
		json.Unmarshal(rr.Body.Bytes(), &updatedPlan)
		if updatedPlan.Name != "New Name" {
			t.Errorf("expected name to be 'New Name', got '%s'", updatedPlan.Name)
		}
		if planRepo.plans[planID].PriceIRR != 200 {
			t.Error("mock repository data was not updated")
		}
	})

	t.Run("Failure for plan not found", func(t *testing.T) {
		updatePayload := `{"name": "New Name", "price_irr": 200}`
		bodyReader := strings.NewReader(updatePayload)
		nonExistingPlanID := uuid.NewString()
		req := httptest.NewRequest("PUT", "/api/v1/plans/"+nonExistingPlanID, bodyReader)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if status := rr.Code; status != http.StatusNotFound {
			t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusNotFound)
		}
	})
}

func TestPlansDeleteHandler(t *testing.T) {
	// Arrange for all subtests
	plan1 := model.SubscriptionPlan{
		ID:   uuid.NewString(),
		Name: "To Be Deleted",
	}
	planInUse := model.SubscriptionPlan{
		ID:   uuid.NewString(),
		Name: "Active Plan",
	}
	planRepo := &mockPlanRepo{
		plans: map[string]*model.SubscriptionPlan{
			plan1.ID:     &plan1,
			planInUse.ID: &planInUse,
		},
	}
	planUC := usecase.NewPlanUseCase(planRepo, nil, nil, newTestLogger())
	handler := plansDeleteHandler(planUC)

	t.Run("Success", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/api/v1/plans/"+plan1.ID, nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if status := rr.Code; status != http.StatusNoContent {
			t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusNoContent)
		}
		if _, ok := planRepo.plans[plan1.ID]; ok {
			t.Error("plan was not deleted from the mock repo")
		}
	})

	t.Run("Failure for plan not found", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/api/v1/plans/"+uuid.NewString(), nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if status := rr.Code; status != http.StatusNotFound {
			t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusNotFound)
		}
	})

	t.Run("Failure for plan in use", func(t *testing.T) {
		// Simulate the specific domain error for a plan that's in use
		planRepo.DeleteError = domain.ErrSubsciptionWithActiveUser

		req := httptest.NewRequest("DELETE", "/api/v1/plans/"+planInUse.ID, nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if status := rr.Code; status != http.StatusConflict {
			t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusConflict)
		}
		planRepo.DeleteError = nil // Reset for other tests
	})
}
