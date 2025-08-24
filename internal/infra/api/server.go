package api

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"time"

	"telegram-ai-subscription/internal/usecase"
)

// Server wires payment callback route to PaymentUseCase.
type Server struct {
	payUC       usecase.PaymentUseCase
	cbPath      string
	botUsername string
}

// NewServer constructs the HTTP server layer for callbacks.
// callbackPath must match the path portion of payment.zarinpal.callback_url in config (e.g. /api/v1/payment/callback/zp).
func NewServer(payUC usecase.PaymentUseCase, callbackPath string, botUsername string) *Server {
	if callbackPath == "" {
		callbackPath = "/api/payment/callback"
	}
	return &Server{payUC: payUC, cbPath: callbackPath, botUsername: botUsername}
}

// Register attaches handlers to the provided mux.
func (s *Server) Register(mux *http.ServeMux) {
	mux.HandleFunc(s.cbPath, s.handleCallback)
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})
}

func (s *Server) handleCallback(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	q := r.URL.Query()
	authority := q.Get("Authority")
	status := q.Get("Status")

	if authority == "" {
		s.renderHTML(w, http.StatusBadRequest, false, "missing Authority")
		return
	}

	// If the gateway did not return OK, attempt to record failure via ConfirmAuto
	if status != "OK" {
		if _, err := s.payUC.ConfirmAuto(ctx, authority); err != nil {
			s.renderHTML(w, http.StatusBadRequest, false, fmt.Sprintf("payment not approved (Status=%s)", status))
			return
		}
		s.renderHTML(w, http.StatusOK, false, fmt.Sprintf("payment not approved (Status=%s)", status))
		return
	}

	// Gateway says OK -> verify & finalize (idempotent inside UC)
	if _, err := s.payUC.ConfirmAuto(ctx, authority); err != nil {
		s.renderHTML(w, http.StatusBadRequest, false, fmt.Sprintf("verification failed: %v", err))
		return
	}
	s.renderHTML(w, http.StatusOK, true, "payment verified. subscription is now active or queued if you already have one.")
}

var page = template.Must(template.New("cb").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8" />
<meta name="viewport" content="width=device-width,initial-scale=1" />
<title>Payment {{if .OK}}Success{{else}}Result{{end}}</title>
<style>
body{font-family:system-ui,Arial,sans-serif;margin:2rem;}
.card{max-width:560px;border:1px solid #ddd;border-radius:12px;padding:24px;}
.ok{color:#057a55} .fail{color:#b00020}
.btn{display:inline-block;margin-top:16px;padding:10px 16px;border-radius:8px;border:1px solid #888;text-decoration:none}
.small{font-size:12px;color:#666}
</style>
</head>
<body>
<div class="card">
  <h2 class="{{if .OK}}ok{{else}}fail{{end}}">{{if .OK}}✅ Payment Successful{{else}}⚠️ Payment Processed{{end}}</h2>
  <p>{{.Msg}}</p>
  {{if .BotUsername}}
    <a class="btn" href="https://t.me/{{.BotUsername}}">Back to Telegram</a>
    <div class="small">If this button doesn't open the chat, open Telegram and search for @{{.BotUsername}}.</div>
  {{end}}
</div>
</body>
</html>`))

func (s *Server) renderHTML(w http.ResponseWriter, code int, ok bool, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(code)
	_ = page.Execute(w, struct {
		OK          bool
		Msg         string
		BotUsername string
	}{
		OK:          ok,
		Msg:         msg,
		BotUsername: s.botUsername,
	})
}
