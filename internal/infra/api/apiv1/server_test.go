package apiv1_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	apiv1 "telegram-ai-subscription/internal/infra/api/apiv1"
	"telegram-ai-subscription/internal/usecase"
)

// fake implementations satisfy the interfaces without doing work.
type fakePricingUC struct{ usecase.PricingUseCase }
type fakePlanUC struct{ usecase.PlanUseCase }

func newRouter() http.Handler {
	r := chi.NewRouter()
	srv := apiv1.NewServer(&fakePricingUC{}, &fakePlanUC{})
	// NOTE: The generated routes include absolute paths like /api/v1/models,
	// so we register the strict handler at the root of this router.
	apiv1.RegisterAPIV1(r, srv)
	return r
}

func Test_V1_NotImplemented_AllEndpoints(t *testing.T) {
	r := newRouter()

	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"list models", "GET", "/api/v1/models", ""},
		{"create model", "POST", "/api/v1/models", `{"name":"gpt-4o","input_price_micros":1,"output_price_micros":2}`},
		{"get model", "GET", "/api/v1/models/gpt-4o", ""},
		{"update model", "PUT", "/api/v1/models/gpt-4o", `{"input_price_micros":3}`},
		{"delete model", "DELETE", "/api/v1/models/gpt-4o", ""},
		{"generate activation codes", "POST", "/api/v1/activation-codes/generate", `{"plan_id":"01HXYZ...","count":10}`},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			var req *http.Request
			if tc.body != "" {
				req = httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
				req.Header.Set("Content-Type", "application/json")
			} else {
				req = httptest.NewRequest(tc.method, tc.path, nil)
			}
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			if rec.Code != http.StatusNotImplemented {
				t.Fatalf("expected 501 Not Implemented, got %d, body: %s", rec.Code, rec.Body.String())
			}
		})
	}
}
