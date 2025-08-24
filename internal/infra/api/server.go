// File: internal/infra/api/server.go
package api

import (
	"encoding/json"
	"log"
	"net/http"

	"telegram-ai-subscription/internal/usecase"
)

// Server exposes minimal endpoints for provider callbacks.
type Server struct {
	paymentUC    usecase.PaymentUseCase
	callbackPath string
}

func NewServer(paymentUC usecase.PaymentUseCase, callbackPath string) *Server {
	if callbackPath == "" {
		callbackPath = "/api/payment/callback"
	}
	return &Server{paymentUC: paymentUC, callbackPath: callbackPath}
}

// Register attaches handlers to a mux.
func (s *Server) Register(mux *http.ServeMux) {
	mux.HandleFunc(s.callbackPath, s.handlePaymentCallback)
}

func (s *Server) handlePaymentCallback(w http.ResponseWriter, r *http.Request) {
	authority := r.URL.Query().Get("Authority")
	status := r.URL.Query().Get("Status")
	if authority == "" || status == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "missing parameters"})
		return
	}
	p, err := s.paymentUC.ConfirmAuto(r.Context(), authority)
	if err != nil {
		log.Printf("payment confirm failed: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"result": "failed"})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"result": "ok", "payment_id": p.ID})
}
