// File: internal/infra/web/server.go
package web

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"

	"telegram-ai-subscription/internal/infra/metrics"
	"telegram-ai-subscription/internal/usecase"

	"github.com/rs/zerolog"
)

// Server wires admin HTTP endpoints, including:
//   - Public auth: POST /api/v1/admin/auth/login, POST /api/v1/admin/auth/logout
//   - Protected admin API (session auth only):
//     /api/v1/stats
//     /api/v1/users, /api/v1/users/*  (router in this file)
//     /api/v1/plans, /api/v1/plans/*  (router in this file)
//
// Auth model (Option B):
//   - Login validates ADMIN_API_KEY and mints a short-lived JWT session cookie "admin_session" (HttpOnly+Secure).
//   - All protected routes require that session (cookie) or a Bearer JWT.
//   - The legacy "Bearer ADMIN_API_KEY" is NOT accepted on protected routes anymore.
type Server struct {
	statsUC usecase.StatsUseCase
	userUC  usecase.UserUseCase
	subUC   usecase.SubscriptionUseCase
	planUC  usecase.PlanUseCase

	// apiKey is used ONLY for login. It is NOT accepted on protected routes anymore.
	apiKey string

	// Session/JWT manager (see web/auth.go).
	auth *AuthManager

	log *zerolog.Logger
}

// NewServer is the single constructor used by callers.
// - apiKey:      required for login validation (server-side).
// - auth:        required for session mint/clear/parse (cookie or Bearer JWT).
// - logger:      used for structured logs.
func NewServer(
	statsUC usecase.StatsUseCase,
	userUC usecase.UserUseCase,
	subUC usecase.SubscriptionUseCase,
	planUC usecase.PlanUseCase,
	apiKey string,
	auth *AuthManager,
	logger *zerolog.Logger,
) *Server {
	webLogger := logger.With().Str("component", "web").Logger()
	return &Server{
		statsUC: statsUC,
		userUC:  userUC,
		subUC:   subUC,
		planUC:  planUC,
		apiKey:  strings.TrimSpace(apiKey),
		auth:    auth,
		log:     &webLogger,
	}
}

// RegisterRoutes mounts admin endpoints onto the provided mux.
// Public (no auth):
//
//	POST /api/v1/admin/auth/login
//	POST /api/v1/admin/auth/logout
//
// Protected (session-only):
//
//	/api/v1/stats
//	/api/v1/users,  /api/v1/users/*
//	/api/v1/plans,  /api/v1/plans/*
//
// NOTE: If you also have a generated Admin API (OpenAPI) router, mount it in main.go like:
//
//	adminChi := chi.NewRouter()
//	apiv1.RegisterAPIV1(adminChi, apiServer)
//	mux.Handle("/api/v1/", adminServer.authMiddleware(adminChi))
//
// This keeps RegisterRoutes simple and backward-compatible (single argument).
func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	// ---- Auth endpoints (public) ----
	mux.HandleFunc("POST /api/v1/admin/auth/login", s.handleAdminLogin)
	mux.HandleFunc("POST /api/v1/admin/auth/logout", s.handleAdminLogout)

	// ---- Protected admin routes ----
	// Stats (simple handler function lives in handlers.go)
	mux.Handle("/api/v1/stats", s.authMiddleware(statsHandler(s.statsUC)))

	// Users router (implemented below; dispatches to handlers in handlers.go)
	usersH := s.authMiddleware(s.usersRouter())
	mux.Handle("/api/v1/users", usersH)
	mux.Handle("/api/v1/users/", usersH)

	// Plans router (implemented below; dispatches to handlers in handlers.go)
	plansH := s.authMiddleware(s.plansRouter())
	mux.Handle("/api/v1/plans", plansH)  // POST + GET all
	mux.Handle("/api/v1/plans/", plansH) // PUT + DELETE (by ID)
}

// ------------------------ AUTH HANDLERS ------------------------

func (s *Server) AuthWrap(h http.Handler) http.Handler { return s.authMiddleware(h) }

// POST /api/v1/admin/auth/login
// Body: {"key":"<ADMIN_API_KEY>"}
//
// Behavior:
//   - Validates submitted key against s.apiKey (constant-time).
//   - On success: mints a JWT and sets "admin_session" cookie (HttpOnly+Secure).
//   - Returns 204 No Content on success.
//
// Metrics:
//   - admin_command_total{command="login",status="authorized|unauthorized"}
func (s *Server) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.apiKey == "" {
		http.Error(w, "service missing admin api key", http.StatusInternalServerError)
		return
	}
	var body struct {
		Key string `json:"key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(body.Key)), []byte(s.apiKey)) != 1 {
		metrics.IncAdminCommand("login", "unauthorized")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if s.auth == nil {
		http.Error(w, "auth manager not configured", http.StatusInternalServerError)
		return
	}
	if _, err := s.auth.Mint(w); err != nil {
		if s.log != nil {
			s.log.Error().Err(err).Msg("mint admin session failed")
		}
		http.Error(w, "failed to mint session", http.StatusInternalServerError)
		return
	}
	metrics.IncAdminCommand("login", "authorized")
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/v1/admin/auth/logout
//
// Behavior:
//   - Clears the "admin_session" cookie (AuthManager.Clear).
//   - Returns 204 No Content.
//
// Metrics:
//   - admin_command_total{command="logout",status="authorized"}
func (s *Server) handleAdminLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.auth != nil {
		s.auth.Clear(w)
	}
	metrics.IncAdminCommand("logout", "authorized")
	w.WriteHeader(http.StatusNoContent)
}

// ------------------------ MIDDLEWARE ------------------------

// authMiddleware enforces session-based admin auth over protected routes.
//
// Accepted credentials:
//  1. Session JWT via HttpOnly cookie "admin_session" (minted by login), OR
//  2. Authorization: Bearer <jwt> (optional; useful for headless tools if you export the JWT).
//
// Not accepted anymore:
//   - Authorization: Bearer <ADMIN_API_KEY>  (legacy path removed)
//
// On success: request proceeds.
// On failure: 401 Unauthorized + metrics admin_command_total{command="api",status="unauthorized"}.
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.auth != nil {
			if _, err := s.auth.ParseFromRequest(r); err == nil {
				next.ServeHTTP(w, r)
				return
			}
		}
		metrics.IncAdminCommand("api", "unauthorized")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
}

// ------------------------ ROUTERS (kept in this file) ------------------------
//
// These routers keep the URL mapping local to server.go while delegating to the
// actual handler functions implemented in handlers.go. This matches your previous
// design where usersRouter and plansRouter lived in server.go.
//
// Expected handler funcs in handlers.go (already present):
//   - statsHandler(statsUC usecase.StatsUseCase) http.HandlerFunc
//   - usersListHandler(userUC usecase.UserUseCase) http.HandlerFunc
//   - userGetHandler(userUC usecase.UserUseCase, subUC usecase.SubscriptionUseCase) http.HandlerFunc
//   - plansListHandler(planUC usecase.PlanUseCase) http.HandlerFunc
//   - plansCreateHandler(planUC usecase.PlanUseCase) http.HandlerFunc
//   - plansUpdateHandler(planUC usecase.PlanUseCase) http.HandlerFunc
//   - plansDeleteHandler(planUC usecase.PlanUseCase) http.HandlerFunc

// usersRouter dispatches:
//
//	GET  /api/v1/users           -> list
//	GET  /api/v1/users/{id}      -> get details
func (s *Server) usersRouter() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Exact collection route
		if r.URL.Path == "/api/v1/users" {
			if r.Method == http.MethodGet {
				usersListHandler(s.userUC)(w, r)
				return
			}
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Item route: /api/v1/users/{id}
		if strings.HasPrefix(r.URL.Path, "/api/v1/users/") {
			if r.Method == http.MethodGet {
				userGetHandler(s.userUC, s.subUC)(w, r)
				return
			}
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		http.NotFound(w, r)
	})
}

// plansRouter dispatches:
//
//	GET  /api/v1/plans           -> list
//	POST /api/v1/plans           -> create
//	PUT  /api/v1/plans/{id}      -> update
//	DELETE /api/v1/plans/{id}    -> delete
func (s *Server) plansRouter() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Exact collection route
		if r.URL.Path == "/api/v1/plans" {
			switch r.Method {
			case http.MethodGet:
				plansListHandler(s.planUC)(w, r)
				return
			case http.MethodPost:
				plansCreateHandler(s.planUC)(w, r)
				return
			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
		}

		// Item route: /api/v1/plans/{id}
		if strings.HasPrefix(r.URL.Path, "/api/v1/plans/") {
			switch r.Method {
			case http.MethodPut:
				plansUpdateHandler(s.planUC)(w, r)
				return
			case http.MethodDelete:
				plansDeleteHandler(s.planUC)(w, r)
				return
			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
		}

		http.NotFound(w, r)
	})
}
