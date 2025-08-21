package http

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"telegram-ai-subscription/internal/config"
	"telegram-ai-subscription/internal/domain/ports/repository"
	"telegram-ai-subscription/internal/usecase"
)

type Server struct {
	cfg       *config.Config
	paymentUC usecase.PaymentUseCase
	userRepo  repository.UserRepository
	server    *http.Server
}

func NewServer(cfg *config.Config, paymentUC usecase.PaymentUseCase, userRepo repository.UserRepository) *Server {
	return &Server{
		cfg:       cfg,
		paymentUC: paymentUC,
		userRepo:  userRepo,
	}
}

func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/webhook/zarinpal", s.handleZarinPalWebhook)
	mux.HandleFunc("/health", s.handleHealthCheck)
	mux.HandleFunc("/payment/success", s.handlePaymentSuccess)
	mux.HandleFunc("/payment/failed", s.handlePaymentFailed)

	s.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.cfg.Admin.Port),
		Handler: mux,
	}

	log.Printf("HTTP server listening on port %d", s.cfg.Admin.Port)
	return s.server.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

func (s *Server) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "OK")
}

func (s *Server) handleZarinPalWebhook(w http.ResponseWriter, r *http.Request) {
	log.Printf("Incoming request: %s %s", r.Method, r.URL.String())
	log.Printf("Headers: %v", r.Header)
	log.Printf("Query parameters: %v", r.URL.Query())
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	log.Printf("Received ZarinPal webhook: %s %s", r.Method, r.URL.String())

	// Extract ZarinPal callback parameters from query string
	authority := r.URL.Query().Get("Authority")
	status := r.URL.Query().Get("Status")

	log.Printf("Received ZarinPal callback: Authority=%s, Status=%s", authority, status)

	if status != "OK" {
		log.Printf("Payment failed or cancelled for authority %s", authority)
		// Redirect to a failure page or send a failure message
		http.Redirect(w, r, "/payment/failed", http.StatusSeeOther)
		return
	}

	// We need to get the amount from the database since it's not in the callback
	ctx := r.Context()
	payment, err := s.paymentUC.GetByAuthority(ctx, authority)
	if err != nil {
		log.Printf("Failed to find payment for authority %s: %v", authority, err)
		http.Error(w, "Payment not found", http.StatusNotFound)
		return
	}

	// Verify the payment
	verifiedPayment, err := s.paymentUC.Confirm(ctx, authority, payment.Amount)
	if err != nil {
		log.Printf("Payment confirmation failed: %v", err)
		// Redirect to a failure page
		http.Redirect(w, r, "/payment/failed", http.StatusSeeOther)
		return
	}

	log.Printf("Payment confirmed successfully: %s", verifiedPayment.ID)

	// Redirect to a success page
	http.Redirect(w, r, "/payment/success", http.StatusSeeOther)
}

func (s *Server) handlePaymentSuccess(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `
        <!DOCTYPE html>
        <html>
        <head>
            <title>Payment Successful</title>
            <meta charset="utf-8">
            <meta name="viewport" content="width=device-width, initial-scale=1">
            <style>
                body { font-family: Arial, sans-serif; text-align: center; padding: 50px; }
                .success { color: #4CAF50; }
            </style>
        </head>
        <body>
            <h1 class="success">Payment Successful!</h1>
            <p>Your payment has been processed successfully. You can now return to the Telegram bot.</p>
            <p><a href="https://t.me/wltshmrzBot">Return to Bot</a></p>
        </body>
        </html>
    `)
}

func (s *Server) handlePaymentFailed(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `
        <!DOCTYPE html>
        <html>
        <head>
            <title>Payment Failed</title>
            <meta charset="utf-8">
            <meta name="viewport" content="width=device-width, initial-scale=1">
            <style>
                body { font-family: Arial, sans-serif; text-align: center; padding: 50px; }
                .error { color: #F44336; }
            </style>
        </head>
        <body>
            <h1 class="error">Payment Failed</h1>
            <p>Your payment could not be processed. Please try again.</p>
            <p><a href="https://t.me/wltshmrzBot">Return to Bot</a></p>
        </body>
        </html>
    `)
}
