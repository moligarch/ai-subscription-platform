//go:build !integration

package apiv1_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v4"
	"github.com/rs/zerolog"

	apiv1 "telegram-ai-subscription/internal/infra/api/apiv1"
	"telegram-ai-subscription/internal/usecase"

	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/repository"
)

//
// ---------------- in-memory infra mocks (repos/tx) ----------------
//

type memPricingRepo struct {
	byName map[string]*model.ModelPricing

	// optional error hooks to exercise 400 mapping paths
	errList   error
	errCreate error
	errUpdate error
	errGet    error
}

func newMemPricingRepo() *memPricingRepo {
	return &memPricingRepo{byName: map[string]*model.ModelPricing{}}
}

func (m *memPricingRepo) GetByModelName(ctx context.Context, tx repository.Tx, name string) (*model.ModelPricing, error) {
	if m.errGet != nil {
		return nil, m.errGet
	}
	p, ok := m.byName[name]
	if !ok || !p.Active {
		return nil, domain.ErrNotFound
	}
	cp := *p
	return &cp, nil
}

func (m *memPricingRepo) Create(ctx context.Context, tx repository.Tx, p *model.ModelPricing) error {
	if m.errCreate != nil {
		return m.errCreate
	}
	if existing, ok := m.byName[p.ModelName]; ok && existing.Active {
		return domain.ErrAlreadyExists
	}
	cp := *p
	if cp.UpdatedAt.IsZero() {
		cp.UpdatedAt = time.Now()
	}
	m.byName[p.ModelName] = &cp
	return nil
}

func (m *memPricingRepo) Update(ctx context.Context, tx repository.Tx, p *model.ModelPricing) error {
	if m.errUpdate != nil {
		return m.errUpdate
	}
	if _, ok := m.byName[p.ModelName]; !ok {
		return domain.ErrNotFound
	}
	cp := *p
	cp.UpdatedAt = time.Now()
	m.byName[p.ModelName] = &cp
	return nil
}

func (m *memPricingRepo) ListActive(ctx context.Context, tx repository.Tx) ([]*model.ModelPricing, error) {
	if m.errList != nil {
		return nil, m.errList
	}
	out := make([]*model.ModelPricing, 0, len(m.byName))
	for _, v := range m.byName {
		if v.Active {
			cp := *v
			out = append(out, &cp)
		}
	}
	return out, nil
}

type noTx struct{}

type mockTxManager struct{}

func (m *mockTxManager) WithTx(ctx context.Context, _ pgx.TxOptions, fn func(ctx context.Context, tx repository.Tx) error) error {
	return fn(ctx, noTx{})
}

//
// -------------------- test helpers --------------------
//

func newLogger() *zerolog.Logger { l := zerolog.Nop(); return &l }

func newServerWithRepo(repo *memPricingRepo) *chi.Mux {
	tx := &mockTxManager{}
	uc := usecase.NewPricingUseCase(repo, tx, newLogger())

	r := chi.NewRouter()
	srv := apiv1.NewServer(uc, nil)

	// generated mux registers absolute paths (/api/v1/...), so mount at root
	apiv1.RegisterAPIV1(r, srv)
	return r
}

func seedOneActive() *memPricingRepo {
	repo := newMemPricingRepo()
	repo.byName["gpt-4o"] = &model.ModelPricing{
		ModelName:              "gpt-4o",
		InputTokenPriceMicros:  1,
		OutputTokenPriceMicros: 2,
		Active:                 true,
		UpdatedAt:              time.Now(),
	}
	return repo
}

//
// -------------------- tests --------------------
//

func TestModels_List_BasicsAndErrors(t *testing.T) {
	t.Run("empty list returns 200 and empty items", func(t *testing.T) {
		repo := newMemPricingRepo()
		r := newServerWithRepo(repo)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/models", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("want 200, got %d, body=%s", rec.Code, rec.Body.String())
		}
		var body struct {
			Items []apiv1.Model `json:"items"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(body.Items) != 0 {
			t.Fatalf("want empty list, got %d", len(body.Items))
		}
	})

	t.Run("non-empty list returns 200 and one item", func(t *testing.T) {
		repo := newMemPricingRepo()
		repo.byName["gpt-4o"] = &model.ModelPricing{
			ModelName:              "gpt-4o",
			InputTokenPriceMicros:  1,
			OutputTokenPriceMicros: 2,
			Active:                 true,
			UpdatedAt:              time.Now(),
		}
		r := newServerWithRepo(repo)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/models?q=anything", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("want 200, got %d, body=%s", rec.Code, rec.Body.String())
		}
		var body struct {
			Items []apiv1.Model `json:"items"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(body.Items) != 1 || body.Items[0].Name != "gpt-4o" {
			t.Fatalf("items mismatch: %+v", body.Items)
		}
	})

	t.Run("repo error maps to 400", func(t *testing.T) {
		repo := newMemPricingRepo()
		repo.errList = errors.New("boom")
		r := newServerWithRepo(repo)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/models", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("want 400, got %d, body=%s", rec.Code, rec.Body.String())
		}
	})
}

func TestModels_Create_AllPaths(t *testing.T) {
	t.Run("201 created", func(t *testing.T) {
		repo := newMemPricingRepo()
		r := newServerWithRepo(repo)

		body := `{"name":"gpt-4o","input_price_micros":1,"output_price_micros":2,"currency":"IRR"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/models", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("got %d, body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("409 duplicate", func(t *testing.T) {
		repo := seedOneActive()
		r := newServerWithRepo(repo)

		body := `{"name":"gpt-4o","input_price_micros":1,"output_price_micros":2,"currency":"IRR"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/models", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusConflict {
			t.Fatalf("want 409, got %d, body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("422 invalid name", func(t *testing.T) {
		repo := newMemPricingRepo()
		r := newServerWithRepo(repo)

		body := `{"name":"  ","input_price_micros":1,"output_price_micros":2,"currency":"IRR"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/models", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("want 422, got %d, body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("400 missing body", func(t *testing.T) {
		repo := newMemPricingRepo()
		r := newServerWithRepo(repo)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/models", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("want 400, got %d, body=%s", rec.Code, rec.Body.String())
		}
	})
}

func TestModels_Get_Update_Delete_AllPaths(t *testing.T) {
	t.Run("get 200", func(t *testing.T) {
		r := newServerWithRepo(seedOneActive())

		req := httptest.NewRequest(http.MethodGet, "/api/v1/models/gpt-4o", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("got %d, body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("get 404", func(t *testing.T) {
		r := newServerWithRepo(seedOneActive())

		req := httptest.NewRequest(http.MethodGet, "/api/v1/models/nope", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("want 404, got %d, body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("update 200 (partial)", func(t *testing.T) {
		r := newServerWithRepo(seedOneActive())

		body := `{"input_price_micros":3}`
		req := httptest.NewRequest(http.MethodPut, "/api/v1/models/gpt-4o", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("got %d, body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("update 404", func(t *testing.T) {
		r := newServerWithRepo(seedOneActive())

		body := `{"input_price_micros":3}`
		req := httptest.NewRequest(http.MethodPut, "/api/v1/models/missing", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("want 404, got %d, body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("update 400 (missing body)", func(t *testing.T) {
		r := newServerWithRepo(seedOneActive())

		req := httptest.NewRequest(http.MethodPut, "/api/v1/models/gpt-4o", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("want 400, got %d, body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("delete 204", func(t *testing.T) {
		r := newServerWithRepo(seedOneActive())

		req := httptest.NewRequest(http.MethodDelete, "/api/v1/models/gpt-4o", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("want 204, got %d, body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("delete again -> 404 (already inactive)", func(t *testing.T) {
		repo := seedOneActive()
		r := newServerWithRepo(repo)

		// first delete succeeds
		req1 := httptest.NewRequest(http.MethodDelete, "/api/v1/models/gpt-4o", nil)
		rec1 := httptest.NewRecorder()
		r.ServeHTTP(rec1, req1)
		if rec1.Code != http.StatusNoContent {
			t.Fatalf("first delete want 204, got %d, body=%s", rec1.Code, rec1.Body.String())
		}

		// second delete returns not found per current handler mapping
		req2 := httptest.NewRequest(http.MethodDelete, "/api/v1/models/gpt-4o", nil)
		rec2 := httptest.NewRecorder()
		r.ServeHTTP(rec2, req2)
		if rec2.Code != http.StatusNotFound {
			t.Fatalf("second delete want 404, got %d, body=%s", rec2.Code, rec2.Body.String())
		}
	})

	t.Run("delete 404 (never existed)", func(t *testing.T) {
		r := newServerWithRepo(seedOneActive())

		req := httptest.NewRequest(http.MethodDelete, "/api/v1/models/never", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("want 404, got %d, body=%s", rec.Code, rec.Body.String())
		}
	})
}
