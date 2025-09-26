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
	"telegram-ai-subscription/internal/infra/i18n"
	"telegram-ai-subscription/internal/infra/metrics"
	"telegram-ai-subscription/internal/usecase"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
)

type Server struct {
	payUC       usecase.PaymentUseCase
	users       repository.UserRepository
	bot         adapter.TelegramBotAdapter
	botUsername string // without leading '@'
	translator  *i18n.Translator
	log         *zerolog.Logger
}

func NewServer(
	payUC usecase.PaymentUseCase,
	users repository.UserRepository,
	bot adapter.TelegramBotAdapter,
	botUsername string,
	translator *i18n.Translator,
	logger *zerolog.Logger,
) *Server {
	// normalize username (strip '@' if provided)
	botUsername = strings.TrimPrefix(strings.TrimSpace(botUsername), "@")
	pcbLog := logger.With().Str("component", "Payment Callback Server").Logger()

	return &Server{
		payUC:       payUC,
		users:       users,
		bot:         bot,
		botUsername: botUsername,
		translator:  translator,
		log:         &pcbLog,
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
	start := time.Now()
	result := "fail"
	reason := "unknown"
	defer func() {
		metrics.PaymentVerifyDuration.WithLabelValues(result).Observe(time.Since(start).Seconds())
		metrics.PaymentVerifyRequests.WithLabelValues(result, reason).Inc()
	}()

	if r.Method != http.MethodPost {
		reason = "method_not_allowed"
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
		reason = "bad_json"
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	authority := strings.TrimSpace(body.Authority)
	status := strings.ToUpper(strings.TrimSpace(body.Status))

	// Always return bot username so UI can render the button
	bot := s.botUsername

	// Quick validation
	if authority == "" {
		reason = "missing_authority"
		writeJSON(w, http.StatusOK, respBody{Result: "fail", Reason: "missing authority", Bot: bot})
		return
	}
	if status != "OK" {
		reason = "not_ok_status"
		// Best-effort failure DM (detached)
		go s.tryNotifyFailure(context.Background(), authority, "payment not approved")
		writeJSON(w, http.StatusOK, respBody{Result: "fail", Reason: "payment not approved", Bot: bot})
		return
	}

	// Idempotent verify: ConfirmAuto should be safe to call multiple times for the same Authority.
	p, err := s.payUC.ConfirmAuto(r.Context(), authority)
	if err != nil {
		reason = "confirm_error"
		if s.log != nil {
			s.log.Error().Err(err).Str("authority", authority).Msg("payment verification failed")
		}
		// Best-effort failure DM (detached)
		go s.tryNotifyFailure(context.Background(), authority, "verification failed")
		writeJSON(w, http.StatusOK, respBody{Result: "fail", Reason: "verification failed", Bot: bot})
		return
	}

	// Detach from the request context to prevent cancellation after the HTTP response is sent.
	go s.notifyPaymentSuccess(context.Background(), p)

	result = "ok"
	reason = "" // keep empty for ok
	writeJSON(w, http.StatusOK, respBody{Result: "ok", Bot: bot})
}

func (s *Server) tryNotifyFailure(ctx context.Context, authority, reason string) {
	// Resolve userID via PaymentUseCase (short timeout)
	c1, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	userID, err := s.payUC.FindUserIDByAuthority(c1, authority)
	if err != nil || userID == "" {
		if s.log != nil {
			s.log.Warn().
				Err(err).
				Str("authority", authority).
				Msg("unable to resolve user for failure DM")
		}
		metrics.PaymentDMTotal.WithLabelValues("failure", "no_user").Inc()
		return
	}

	s.notifyPaymentFailure(ctx, userID, reason)
}

func (s *Server) notifyPaymentFailure(ctx context.Context, userID, reason string) {
	if s.users == nil || s.bot == nil || userID == "" {
		return
	}
	c2, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	u, err := s.users.FindByID(c2, nil, userID)
	if err != nil || u == nil || u.TelegramID == 0 {
		if s.log != nil {
			s.log.Error().Err(err).
				Str("user_id", userID).
				Msg("notifyPaymentFailure: failed to load user or invalid telegram id")
		}
		metrics.PaymentDMTotal.WithLabelValues("failure", "no_user").Inc()
		return
	}

	msg := s.translator.T("payment_notify_failure", reason)

	if err := s.bot.SendMessage(c2, adapter.SendMessageParams{
		ChatID: u.TelegramID,
		Text:   msg,
	}); err != nil {
		if s.log != nil {
			s.log.Error().Err(err).
				Int64("chat_id", u.TelegramID).
				Msg("notifyPaymentFailure: failed to send telegram DM")
		}
		metrics.PaymentDMTotal.WithLabelValues("failure", "error").Inc()
		return
	}
	metrics.PaymentDMTotal.WithLabelValues("failure", "sent").Inc()
}

func (s *Server) notifyPaymentSuccess(ctx context.Context, p *model.Payment) {
	if s.users == nil || s.bot == nil || p == nil {
		return
	}
	c2, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	u, err := s.users.FindByID(c2, nil, p.UserID)
	if err != nil || u == nil || u.TelegramID == 0 {
		if s.log != nil {
			s.log.Error().Err(err).
				Str("user_id", p.UserID).
				Msg("notifyPaymentSuccess: failed to load user or invalid telegram id")
		}
		metrics.PaymentDMTotal.WithLabelValues("success", "no_user").Inc()
		return
	}

	msg := s.translator.T("payment_notify_success")

	if err := s.bot.SendMessage(c2, adapter.SendMessageParams{
		ChatID: u.TelegramID,
		Text:   msg,
	}); err != nil {
		if s.log != nil {
			s.log.Error().Err(err).Int64("chat_id", u.TelegramID).Msg("notifyPaymentSuccess: failed to send telegram DM")
		}
		metrics.PaymentDMTotal.WithLabelValues("success", "error").Inc()
		return
	}
	metrics.PaymentDMTotal.WithLabelValues("success", "sent").Inc()
}

// tiny helper
func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
