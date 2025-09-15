package web

import (
	"net/http"
	"strings"
	"telegram-ai-subscription/internal/usecase"

	"github.com/rs/zerolog"
)

type Server struct {
	statsUC usecase.StatsUseCase
	userUC  usecase.UserUseCase
	subUC   usecase.SubscriptionUseCase
	apiKey  string
	log     *zerolog.Logger
}

func NewServer(
	statsUC usecase.StatsUseCase,
	userUC usecase.UserUseCase,
	subUC usecase.SubscriptionUseCase,
	apiKey string,
	logger *zerolog.Logger,
) *Server {
	return &Server{
		statsUC: statsUC,
		userUC:  userUC,
		subUC:   subUC,
		apiKey:  apiKey,
		log:     logger,
	}
}

// RegisterRoutes sets up the routing for the admin API.
func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	// All admin routes will be behind the auth middleware
	statsHandler := s.authMiddleware(statsHandler(s.statsUC))
	mux.Handle("/api/v1/stats", statsHandler)

	// A single handler for all /api/v1/users/ routes
	usersRouter := s.authMiddleware(s.usersRouter())
	mux.Handle("/api/v1/users/", usersRouter)
}

// authMiddleware provides simple Bearer token authentication for the admin API.
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.apiKey == "" {
			s.log.Error().Msg("Admin API key is not configured")
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		tokenParts := strings.Split(authHeader, " ")
		if len(tokenParts) != 2 || strings.ToLower(tokenParts[0]) != "bearer" {
			http.Error(w, "Unauthorized: Malformed token", http.StatusUnauthorized)
			return
		}

		if tokenParts[1] != s.apiKey {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) usersRouter() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/v1/users")
		path = strings.TrimSuffix(path, "/")

		if path == "" { // Path is /api/v1/users
			usersListHandler(s.userUC)(w, r)
		} else { // Path is /api/v1/users/{id}
			userGetHandler(s.userUC, s.subUC)(w, r)
		}
	})
}
