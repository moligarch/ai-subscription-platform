//go:build !integration

package apiv1_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
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

/*
These helpers are uniquely prefixed (ac*) to avoid clashing with existing mocks.
They implement the real repository interfaces:

- repository.SubscriptionPlanRepository
- repository.ActivationCodeRepository
*/

type acDummyPricingRepo struct{} // satisfies repository.ModelPricingRepository (unused paths here)

func (d *acDummyPricingRepo) GetByModelName(ctx context.Context, tx repository.Tx, name string) (*model.ModelPricing, error) {
	return nil, domain.ErrNotFound
}
func (d *acDummyPricingRepo) Create(ctx context.Context, tx repository.Tx, p *model.ModelPricing) error {
	return nil
}
func (d *acDummyPricingRepo) Update(ctx context.Context, tx repository.Tx, p *model.ModelPricing) error {
	return nil
}
func (d *acDummyPricingRepo) ListActive(ctx context.Context, tx repository.Tx) ([]*model.ModelPricing, error) {
	return nil, nil
}

// acMemPlanRepo implements repository.SubscriptionPlanRepository
type acMemPlanRepo struct {
	byID map[string]*model.SubscriptionPlan
}

func newAcMemPlanRepo() *acMemPlanRepo {
	return &acMemPlanRepo{byID: map[string]*model.SubscriptionPlan{}}
}

func (r *acMemPlanRepo) Save(ctx context.Context, tx repository.Tx, p *model.SubscriptionPlan) error {
	if p.ID == "" {
		p.ID = "plan-" + time.Now().Format("150405.000")
	}
	cp := *p
	r.byID[p.ID] = &cp
	return nil
}
func (r *acMemPlanRepo) FindByID(ctx context.Context, tx repository.Tx, id string) (*model.SubscriptionPlan, error) {
	if p, ok := r.byID[id]; ok {
		cp := *p
		return &cp, nil
	}
	return nil, domain.ErrNotFound
}
func (r *acMemPlanRepo) ListAll(ctx context.Context, tx repository.Tx) ([]*model.SubscriptionPlan, error) {
	out := make([]*model.SubscriptionPlan, 0, len(r.byID))
	for _, p := range r.byID {
		cp := *p
		out = append(out, &cp)
	}
	return out, nil
}
func (r *acMemPlanRepo) Delete(ctx context.Context, tx repository.Tx, id string) error {
	if _, ok := r.byID[id]; !ok {
		return domain.ErrNotFound
	}
	delete(r.byID, id)
	return nil
}

// acMemCodeRepo implements repository.ActivationCodeRepository
type acMemCodeRepo struct {
	byCode  map[string]*model.ActivationCode
	saveErr error
}

func newAcMemCodeRepo() *acMemCodeRepo {
	return &acMemCodeRepo{byCode: map[string]*model.ActivationCode{}}
}

func (r *acMemCodeRepo) Save(ctx context.Context, tx repository.Tx, c *model.ActivationCode) error {
	if r.saveErr != nil {
		return r.saveErr
	}
	cp := *c
	r.byCode[c.Code] = &cp
	return nil
}
func (r *acMemCodeRepo) FindByCode(ctx context.Context, tx repository.Tx, code string) (*model.ActivationCode, error) {
	if c, ok := r.byCode[code]; ok {
		cp := *c
		return &cp, nil
	}
	return nil, domain.ErrNotFound
}

// Wire real PlanUseCase (no UC mocking) + generated HTTP handlers
func newServerWithPlanUC(planRepo repository.SubscriptionPlanRepository, codeRepo repository.ActivationCodeRepository) *chi.Mux {
	planUC := usecase.NewPlanUseCase(planRepo, &acDummyPricingRepo{}, codeRepo, newLogger())
	srv := apiv1.NewServer(nil, planUC)

	r := chi.NewRouter()
	apiv1.RegisterAPIV1(r, srv)
	return r
}

var acCodePattern = regexp.MustCompile(`^[A-HJ-NP-Z2-9]{4}-[A-HJ-NP-Z2-9]{4}-[A-HJ-NP-Z2-9]{4}$`)

func TestActivationCodes_Generate(t *testing.T) {
	t.Run("201 created (codes returned, persisted)", func(t *testing.T) {
		pr := newAcMemPlanRepo()
		_ = pr.Save(context.Background(), nil, &model.SubscriptionPlan{
			ID:           "plan-1",
			Name:         "Pro",
			DurationDays: 30,
			Credits:      1000,
			PriceIRR:     1_000_000,
			CreatedAt:    time.Now(),
		})
		cr := newAcMemCodeRepo()
		r := newServerWithPlanUC(pr, cr)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/activation-codes/generate",
			bytes.NewBufferString(`{"plan_id":"plan-1","count":2}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("want 201, got %d, body=%s", rec.Code, rec.Body.String())
		}

		var resp struct {
			BatchID string `json:"batch_id"`
			Codes   []struct {
				Code string `json:"code"`
			} `json:"codes"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp.BatchID == "" || len(resp.Codes) != 2 {
			t.Fatalf("unexpected resp: %+v", resp)
		}
		for _, c := range resp.Codes {
			if !acCodePattern.MatchString(c.Code) {
				t.Fatalf("code format mismatch: %q", c.Code)
			}
			if _, err := cr.FindByCode(context.Background(), nil, c.Code); err != nil {
				t.Fatalf("code not saved: %v", err)
			}
		}
	})

	t.Run("404 when plan not found", func(t *testing.T) {
		pr := newAcMemPlanRepo() // no plans
		cr := newAcMemCodeRepo()
		r := newServerWithPlanUC(pr, cr)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/activation-codes/generate",
			bytes.NewBufferString(`{"plan_id":"missing","count":3}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("want 404, got %d, body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("400 when body is missing", func(t *testing.T) {
		pr := newAcMemPlanRepo()
		cr := newAcMemCodeRepo()
		r := newServerWithPlanUC(pr, cr)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/activation-codes/generate", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("want 400, got %d, body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("400 when repository Save fails", func(t *testing.T) {
		pr := newAcMemPlanRepo()
		_ = pr.Save(context.Background(), nil, &model.SubscriptionPlan{
			ID: "plan-1", Name: "Pro", DurationDays: 30, Credits: 1000, PriceIRR: 1_000_000, CreatedAt: time.Now(),
		})
		cr := newAcMemCodeRepo()
		cr.saveErr = errors.New("store down")
		r := newServerWithPlanUC(pr, cr)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/activation-codes/generate",
			bytes.NewBufferString(`{"plan_id":"plan-1","count":1}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("want 400, got %d, body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("501 when PlanUseCase is not wired", func(t *testing.T) {
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
