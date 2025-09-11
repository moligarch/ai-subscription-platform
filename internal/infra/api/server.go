package api

import (
	"context"
	"html/template"
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
	cbPath      string
	botUsername string
}

func NewServer(
	payUC usecase.PaymentUseCase,
	users repository.UserRepository,
	bot adapter.TelegramBotAdapter,
	cbPath, botUsername string,
) *Server {
	// Normalize path (must start with /)
	if cbPath == "" || cbPath[0] != '/' {
		cbPath = "/" + cbPath
	}
	return &Server{
		payUC:       payUC,
		users:       users,
		bot:         bot,
		cbPath:      cbPath,
		botUsername: botUsername,
	}
}

// Register attaches all handlers to the given mux.
func (s *Server) Register(mux *http.ServeMux) {
	mux.HandleFunc(s.cbPath, s.handleZarinpalCallback)
	mux.Handle("/metrics", promhttp.Handler())
}

func (s *Server) handleZarinpalCallback(w http.ResponseWriter, r *http.Request) {
	// ZarinPal sends ?Authority=...&Status=OK|NOK
	q := r.URL.Query()
	authority := strings.TrimSpace(q.Get("Authority"))
	status := strings.ToUpper(strings.TrimSpace(q.Get("Status")))

	if authority == "" {
		s.renderFailure(w, "missing authority")
		return
	}
	if status != "OK" {
		s.renderFailure(w, "payment not approved")
		return
	}

	// ConfirmAuto will verify against provider and mutate DB if valid
	p, err := s.payUC.ConfirmAuto(r.Context(), authority)
	if err != nil {
		s.renderFailure(w, "verification failed")
		return
	}

	// Fire-and-forget user DM (best-effort; do not block HTTP)
	go s.notifyPaymentSuccess(r.Context(), p)

	s.renderSuccess(w)
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

var successTpl = template.Must(template.New("ok").Parse(`
<!doctype html>
<html lang="en"><head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>Payment Success</title>
<style>
body{font-family:system-ui,-apple-system,Segoe UI,Roboto,Ubuntu,Helvetica,Arial,sans-serif;margin:40px;color:#0b1320}
.card{max-width:560px;margin:auto;border:1px solid #e7ecf3;border-radius:16px;padding:24px;box-shadow:0 4px 18px rgba(0,0,0,.05)}
.btn{display:inline-block;margin-top:12px;padding:10px 14px;border-radius:10px;border:1px solid #1b74e4;text-decoration:none}
.btn:hover{background:#f5faff}
.small{color:#475569;margin-top:6px}
</style>
</head><body>
<div class="card">
<h2>✅ Payment confirmed</h2>
<p>Your plan is now active. You can go back to Telegram and start chatting.</p>
<a class="btn" href="https://t.me/{{.Username}}">Back to Telegram</a>
<p class="small">If this button doesn’t open a chat, search for <b>@{{.Username}}</b> in Telegram.</p>
</div>
</body></html>
`))

var failureTpl = template.Must(template.New("fail").Parse(`
<!doctype html>
<html lang="en"><head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>Payment Failed</title>
<style>
body{font-family:system-ui,-apple-system,Segoe UI,Roboto,Ubuntu,Helvetica,Arial,sans-serif;margin:40px;color:#0b1320}
.card{max-width:560px;margin:auto;border:1px solid #f4d7da;border-radius:16px;padding:24px;box-shadow:0 4px 18px rgba(0,0,0,.05);background:#fff8f8}
.btn{display:inline-block;margin-top:12px;padding:10px 14px;border-radius:10px;border:1px solid #1b74e4;text-decoration:none}
.btn:hover{background:#f5faff}
.small{color:#475569;margin-top:6px}
</style>
</head><body>
<div class="card">
<h2>❌ Payment failed</h2>
<p>{{.Reason}}</p>
<a class="btn" href="https://t.me/{{.Username}}">Back to Telegram</a>
<p class="small">If this button doesn’t open a chat, search for <b>@{{.Username}}</b> in Telegram.</p>
</div>
</body></html>
`))

func (s *Server) renderSuccess(w http.ResponseWriter) {
	_ = successTpl.Execute(w, map[string]any{
		"Username": s.botUsername,
	})
}

func (s *Server) renderFailure(w http.ResponseWriter, reason string) {
	_ = failureTpl.Execute(w, map[string]any{
		"Reason":   reason,
		"Username": s.botUsername,
	})
}
