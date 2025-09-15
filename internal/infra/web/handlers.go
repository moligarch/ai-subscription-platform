package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/usecase"
)

// statsHandler returns an http.HandlerFunc that serves bot statistics.
func statsHandler(statsUC usecase.StatsUseCase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		users, activeByPlan, remainingCredits, err := statsUC.Totals(ctx)
		if err != nil {
			http.Error(w, "Failed to get totals", http.StatusInternalServerError)
			return
		}

		week, month, year, err := statsUC.Revenue(ctx)
		if err != nil {
			http.Error(w, "Failed to get revenue", http.StatusInternalServerError)
			return
		}

		// Consolidate into a single response struct
		response := struct {
			TotalUsers       int            `json:"total_users"`
			ActiveSubsByPlan map[string]int `json:"active_subs_by_plan"`
			TotalCredits     int64          `json:"total_remaining_credits"`
			Revenue          struct {
				Week  int64 `json:"week"`
				Month int64 `json:"month"`
				Year  int64 `json:"year"`
			} `json:"revenue_irr"`
		}{
			TotalUsers:       users,
			ActiveSubsByPlan: activeByPlan,
			TotalCredits:     remainingCredits,
			Revenue: struct {
				Week  int64 `json:"week"`
				Month int64 `json:"month"`
				Year  int64 `json:"year"`
			}{
				Week:  week,
				Month: month,
				Year:  year,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}
}

// usersListHandler returns a paginated list of users.
// It accepts 'offset' and 'limit' query parameters.
func usersListHandler(userUC usecase.UserUseCase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Parse query parameters with defaults
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 {
			limit = 50 // Default page size
		}
		if offset < 0 {
			offset = 0
		}

		// Fetch data from the use case
		users, err := userUC.List(ctx, offset, limit)
		if err != nil {
			http.Error(w, "Failed to list users", http.StatusInternalServerError)
			return
		}

		// Also fetch the total count for pagination metadata
		total, err := userUC.Count(ctx)
		if err != nil {
			http.Error(w, "Failed to count users", http.StatusInternalServerError)
			return
		}

		// Create a structured response
		response := struct {
			Data   []*model.User `json:"data"`
			Total  int           `json:"total"`
			Limit  int           `json:"limit"`
			Offset int           `json:"offset"`
		}{
			Data:   users,
			Total:  total,
			Limit:  limit,
			Offset: offset,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}
}

func userGetHandler(userUC usecase.UserUseCase, subUC usecase.SubscriptionUseCase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Extract user ID from URL path: /api/v1/users/{id}
		id := strings.TrimPrefix(r.URL.Path, "/api/v1/users/")
		if id == "" {
			http.Error(w, "User ID is required", http.StatusBadRequest)
			return
		}

		user, err := userUC.GetByTelegramID(ctx, 123)
		if err != nil {
			if err == domain.ErrUserNotFound {
				http.NotFound(w, r)
				return
			}
			http.Error(w, "Failed to get user", http.StatusInternalServerError)
			return
		}

		subscriptions, err := subUC.ListByUserID(ctx, user.ID)
		if err != nil {
			http.Error(w, "Failed to get user subscriptions", http.StatusInternalServerError)
			return
		}

		// Create a structured response for the user details
		response := struct {
			User          *model.User               `json:"user"`
			Subscriptions []*model.UserSubscription `json:"subscriptions"`
		}{
			User:          user,
			Subscriptions: subscriptions,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}
}
