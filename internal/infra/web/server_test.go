//go:build !integration

package web

import (
	"net/http"
	"net/http/httptest"
	"telegram-ai-subscription/internal/usecase"
	"testing"

	"github.com/rs/zerolog"
)

// newTestLogger creates a silent logger for tests.
func newTestLogger() *zerolog.Logger {
	logger := zerolog.New(nil)
	return &logger
}

func TestAuthMiddleware(t *testing.T) {
	// A simple handler that we expect to be called on successful authentication.
	dummyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	const testAPIKey = "secret-key"
	logger := newTestLogger()

	// We don't need a real use case for this middleware test.
	var mockStatsUC usecase.StatsUseCase // Can be nil

	testCases := []struct {
		name           string
		apiKeyInServer string
		authHeader     string
		expectedStatus int
	}{
		{
			name:           "Success",
			apiKeyInServer: testAPIKey,
			authHeader:     "Bearer " + testAPIKey,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Failure - No Header",
			apiKeyInServer: testAPIKey,
			authHeader:     "",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Failure - Malformed Header (No Scheme)",
			apiKeyInServer: testAPIKey,
			authHeader:     testAPIKey,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Failure - Wrong Scheme",
			apiKeyInServer: testAPIKey,
			authHeader:     "Basic " + testAPIKey,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Failure - Wrong Key",
			apiKeyInServer: testAPIKey,
			authHeader:     "Bearer wrong-key",
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "Failure - No API Key Configured",
			apiKeyInServer: "",
			authHeader:     "Bearer " + testAPIKey,
			expectedStatus: http.StatusForbidden,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			server := NewServer(mockStatsUC, nil, nil, nil, tc.apiKeyInServer, logger)
			handlerToTest := server.authMiddleware(dummyHandler)

			req := httptest.NewRequest("GET", "/api/v1/stats", nil)
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			rr := httptest.NewRecorder()

			// Act
			handlerToTest.ServeHTTP(rr, req)

			// Assert
			if status := rr.Code; status != tc.expectedStatus {
				t.Errorf("handler returned wrong status code: got %v want %v", status, tc.expectedStatus)
			}
		})
	}
}
