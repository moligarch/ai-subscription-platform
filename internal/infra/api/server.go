package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/adapter"
	"telegram-ai-subscription/internal/domain/ports/repository"
	"telegram-ai-subscription/internal/usecase"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Server struct {
	payUC       usecase.PaymentUseCase
	users       repository.UserRepository
	bot         adapter.TelegramBotAdapter
	botUsername string // without leading '@'
}

func NewServer(
	payUC usecase.PaymentUseCase,
	users repository.UserRepository,
	bot adapter.TelegramBotAdapter,
	botUsername string,
) *Server {
	// normalize username (strip '@' if provided)
	botUsername = strings.TrimPrefix(strings.TrimSpace(botUsername), "@")

	return &Server{
		payUC:       payUC,
		users:       users,
		bot:         bot,
		botUsername: botUsername,
	}
}

// Register attaches HTTP handlers to the given mux.
func (s *Server) Register(mux *http.ServeMux) {
	// SPA-first verification endpoint called by the payment UI
	mux.HandleFunc("/api/v1/payment/verify", s.handlePaymentVerify)

	// Prometheus metrics
	mux.Handle("/metrics", promhttp.Handler())
}

// handlePaymentVerify verifies a payment authority server-side and returns JSON.
// Request:  {"authority": "...", "status": "OK"|"NOK"}
// Response: 200 OK { "result":"ok","bot":"<username>" } OR { "result":"fail","reason":"...","bot":"<username>" }
func (s *Server) handlePaymentVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	type reqBody struct {
		Authority string `json:"authority"`
		Status    string `json:"status"`
	}
	type respBody struct {
		Result string `json:"result"`           // "ok" | "fail"
		Reason string `json:"reason,omitempty"` // on failure
		Bot    string `json:"bot,omitempty"`    // bot username (without @)
	}

	var body reqBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	authority := strings.TrimSpace(body.Authority)
	status := strings.ToUpper(strings.TrimSpace(body.Status))

	// Always return bot username so UI can render the button
	bot := s.botUsername

	// Quick validation
	if authority == "" {
		writeJSON(w, http.StatusOK, respBody{Result: "fail", Reason: "missing authority", Bot: bot})
		return
	}
	if status != "OK" {
		writeJSON(w, http.StatusOK, respBody{Result: "fail", Reason: "payment not approved", Bot: bot})
		return
	}

	// Idempotent verify: ConfirmAuto should be safe to call multiple times for the same Authority.
	p, err := s.payUC.ConfirmAuto(r.Context(), authority)
	if err != nil {
		writeJSON(w, http.StatusOK, respBody{Result: "fail", Reason: "verification failed", Bot: bot})
		return
	}

	// Best-effort async DM
	go s.notifyPaymentSuccess(r.Context(), p)

	writeJSON(w, http.StatusOK, respBody{Result: "ok", Bot: bot})
}

func (s *Server) notifyPaymentSuccess(ctx context.Context, p *model.Payment) {
	if s.users == nil || s.bot == nil || p == nil {
		return
	}
	c2, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	u, err := s.users.FindByID(c2, nil, p.UserID)
	if err != nil || u == nil || u.TelegramID == 0 {
		return
	}

	msg := "✅ پرداخت شما با موفقیت تایید شد.\n" +
		"پلن شما فعال شد. برای جزئیات از /status استفاده کنید یا با /chat گفتگو را شروع کنید."

	// Telegram adapter port sends by TelegramID
	_ = s.bot.SendMessage(ctx, adapter.SendMessageParams{
		ChatID: u.TelegramID,
		Text:   msg,
	})
}

// tiny helper
func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
