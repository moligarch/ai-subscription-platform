//go:build !integration

package apiv1_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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

// ===== Activation Codes tests (PlanUseCase-backed) =====

// ---------------- in-memory plan + activation code repos (tests) ----------------

type memPlanRepo struct {
	byID map[string]*model.SubscriptionPlan
}

func newMemPlanRepo() *memPlanRepo { return &memPlanRepo{byID: map[string]*model.SubscriptionPlan{}} }

func (m *memPlanRepo) Save(ctx context.Context, tx repository.Tx, p *model.SubscriptionPlan) error {
	cp := *p
	m.byID[p.ID] = &cp
	return nil
}
func (m *memPlanRepo) FindByID(ctx context.Context, tx repository.Tx, id string) (*model.SubscriptionPlan, error) {
	if p, ok := m.byID[id]; ok {
		cp := *p
		return &cp, nil
	}
	return nil, domain.ErrPlanNotFound
}
func (m *memPlanRepo) ListAll(ctx context.Context, tx repository.Tx) ([]*model.SubscriptionPlan, error) {
	out := make([]*model.SubscriptionPlan, 0, len(m.byID))
	for _, v := range m.byID {
		cp := *v
		out = append(out, &cp)
	}
	return out, nil
}
func (m *memPlanRepo) Delete(ctx context.Context, tx repository.Tx, id string) error {
	delete(m.byID, id)
	return nil
}
func (m *memPlanRepo) CountActiveByModel(ctx context.Context, tx repository.Tx) (map[string]int, error) {
	return map[string]int{}, nil
}

type memActivationCodeRepo struct {
	saved   []string
	errSave error
}

func newMemActivationCodeRepo() *memActivationCodeRepo { return &memActivationCodeRepo{} }

func (m *memActivationCodeRepo) Save(ctx context.Context, tx repository.Tx, code *model.ActivationCode) error {
	if m.errSave != nil {
		return m.errSave
	}
	m.saved = append(m.saved, code.Code)
	return nil
}
func (m *memActivationCodeRepo) FindByCode(ctx context.Context, tx repository.Tx, code string) (*model.ActivationCode, error) {
	// Tests don't need redemption lookup; return not found.
	return nil, domain.ErrNotFound
}

func newServerWithPricingAndPlan(repoPricing *memPricingRepo, planUC usecase.PlanUseCase) *chi.Mux {
	tx := &mockTxManager{}
	pricingUC := usecase.NewPricingUseCase(repoPricing, tx, newLogger())

	r := chi.NewRouter()
	srv := apiv1.NewServer(pricingUC, planUC)
	apiv1.RegisterAPIV1(r, srv)
	return r
}

func TestActivationCodes_Generate_AllPaths(t *testing.T) {
	ctx := context.Background()
	logger := newLogger()

	// Common repos
	priceRepo := newMemPricingRepo() // not used by PlanUC.GenerateActivationCodes, but required by ctor
	planRepo := newMemPlanRepo()
	codeRepo := newMemActivationCodeRepo()

	// Seed a plan
	planID := "plan-1"
	planRepo.byID[planID] = &model.SubscriptionPlan{
		ID:           planID,
		Name:         "Basic",
		DurationDays: 30,
		Credits:      1000,
		PriceIRR:     100000,
		CreatedAt:    time.Now(),
	}

	// Real PlanUseCase with in-memory repos
	planUC := usecase.NewPlanUseCase(planRepo, priceRepo, codeRepo, logger)
	router := newServerWithPricingAndPlan(priceRepo, planUC)

	t.Run("success: 201 with codes", func(t *testing.T) {
		body := `{"plan_id":"` + planID + `","count":3}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/activation-codes/generate", strings.NewReader(body))
		req = req.WithContext(ctx)
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("want 201, got %d, body=%s", w.Code, w.Body.String())
		}
		var resp struct {
			BatchID string `json:"batch_id"`
			Codes   []struct {
				Code      string     `json:"code"`
				ExpiresAt *time.Time `json:"expires_at,omitempty"`
			} `json:"codes"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("invalid json: %v", err)
		}
		if resp.BatchID == "" {
			t.Fatalf("batch_id should not be empty")
		}
		if len(resp.Codes) != 3 {
			t.Fatalf("want 3 codes, got %d", len(resp.Codes))
		}
		for i, c := range resp.Codes {
			if c.Code == "" {
				t.Fatalf("code[%d] is empty", i)
			}
		}
	})

	t.Run("missing body -> 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/activation-codes/generate", nil)
		req = req.WithContext(ctx)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("want 400, got %d, body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("invalid plan_id -> 422", func(t *testing.T) {
		body := `{"plan_id":"", "count":2}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/activation-codes/generate", strings.NewReader(body))
		req = req.WithContext(ctx)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != 422 {
			t.Fatalf("want 422, got %d, body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("negative count -> 422", func(t *testing.T) {
		body := `{"plan_id":"` + planID + `","count":-5}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/activation-codes/generate", strings.NewReader(body))
		req = req.WithContext(ctx)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != 422 {
			t.Fatalf("want 422, got %d, body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("plan not found -> 404", func(t *testing.T) {
		body := `{"plan_id":"missing-plan","count":1}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/activation-codes/generate", strings.NewReader(body))
		req = req.WithContext(ctx)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("want 404, got %d, body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("repo save error -> 400", func(t *testing.T) {
		// Flip repo to error mode
		codeRepo.errSave = errors.New("save failed")

		body := `{"plan_id":"` + planID + `","count":2}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/activation-codes/generate", strings.NewReader(body))
		req = req.WithContext(ctx)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("want 400, got %d, body=%s", w.Code, w.Body.String())
		}
		// Reset
		codeRepo.errSave = nil
	})

	t.Run("PlanUseCase is not wired -> 501", func(t *testing.T) {
		router := chi.NewRouter()
		srv := apiv1.NewServer(nil, nil) // PlanUC nil -> 501
		apiv1.RegisterAPIV1(router, srv)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/activation-codes/generate",
			bytes.NewBufferString(`{"plan_id":"plan-1","count":2}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotImplemented {
			t.Fatalf("want 501, got %d, body=%s", rec.Code, rec.Body.String())
		}
	})
}
