// File: cmd/app/main.go
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"telegram-ai-subscription/internal/application"
	"telegram-ai-subscription/internal/config"
	"telegram-ai-subscription/internal/domain/ports/adapter"
	aiAdapters "telegram-ai-subscription/internal/infra/adapters/ai"
	payAdapters "telegram-ai-subscription/internal/infra/adapters/payment"
	tele "telegram-ai-subscription/internal/infra/adapters/telegram"
	pg "telegram-ai-subscription/internal/infra/db/postgres"
	httpapi "telegram-ai-subscription/internal/infra/http"
	red "telegram-ai-subscription/internal/infra/redis"
	"telegram-ai-subscription/internal/infra/sched"
	"telegram-ai-subscription/internal/infra/security"
	"telegram-ai-subscription/internal/usecase"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ---- CLI flags ----
	cfgPath := flag.String("config", "config.yaml", "path to YAML config file")
	devMode := flag.Bool("dev", false, "enable developer mode (bypass some flows)")
	flag.Parse()

	cfg, err := config.LoadConfig(*cfgPath, *devMode)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	if cfg.Runtime.Dev {
		log.Printf("[DEV MODE] Enabled")
	}

	// ---- Postgres ----
	pool, err := pg.NewPgxPool(ctx, cfg.Database.URL, 10)
	if err != nil {
		log.Fatalf("postgres: %v", err)
	}
	defer pool.Close()

	// ---- Redis ----
	redisClient, err := red.NewClient(ctx, &cfg.Redis)
	if err != nil {
		log.Fatalf("redis: %v", err)
	}
	rateLimiter := red.NewRateLimiter(redisClient)
	chatCache := red.NewChatCache(redisClient, cfg.Redis.TTL)

	// ---- Encryption ----
	encKey := cfg.Security.EncryptionKey
	if len(encKey) != 32 {
		log.Printf("WARNING: security.encryption_key not set or not 32 bytes; falling back to dev key (INSECURE)")
		encKey = "0123456789abcdef0123456789abcdef"
	}
	encSvc, err := security.NewEncryptionService(encKey)
	if err != nil {
		log.Fatalf("encryption: %v", err)
	}

	// ---- Repositories ----
	userRepo := pg.NewPostgresUserRepo(pool)
	planRepo := pg.NewPostgresPlanRepo(pool)
	subRepo := pg.NewPostgresSubscriptionRepo(pool)
	payRepo := pg.NewPostgresPaymentRepo(pool)
	purchaseRepo := pg.NewPostgresPurchaseRepo(pool)
	_ = purchaseRepo
	chatRepo := pg.NewChatSessionRepo(pool, chatCache, encSvc)

	// ---- Use cases ----
	userUC := usecase.NewUserUseCase(userRepo)
	planUC := usecase.NewPlanUseCase(planRepo)
	subUC := usecase.NewSubscriptionUseCase(subRepo, planRepo)

	// ---- AI Adapter (Metis -> Gemini -> OpenAI) ----
	var ai adapter.AIServiceAdapter
	if cfg.AI.MetisKey != "" {
		ai, err = aiAdapters.NewMetisOpenAIAdapter(cfg.AI.MetisKey, cfg.AI.DefaultModel, cfg.AI.MetisBaseURL)
		if err != nil {
			log.Fatalf("metis adapter: %v", err)
		}
		log.Printf("AI adapter: Metis(OpenAI compatible) base=%s model=%s", cfg.AI.MetisBaseURL, cfg.AI.DefaultModel)
	} else if cfg.AI.GeminiKey != "" {
		ai, err = aiAdapters.NewGeminiAdapter(cfg.AI.GeminiKey, cfg.AI.GeminiURL, []string{cfg.AI.DefaultModel})
		if err != nil {
			log.Fatalf("gemini adapter: %v", err)
		}
		log.Printf("AI adapter: Gemini base=%s model=%s", cfg.AI.GeminiURL, cfg.AI.DefaultModel)
	} else if cfg.AI.OpenAIKey != "" {
		ai, err = aiAdapters.NewOpenAIAdapter(cfg.AI.OpenAIKey, cfg.AI.DefaultModel)
		if err != nil {
			log.Fatalf("openai adapter: %v", err)
		}
		log.Printf("AI adapter: OpenAI model=%s", cfg.AI.DefaultModel)
	} else {
		log.Fatalf("no AI provider configured: set ai.metis_key or ai.gemini_key or ai.openai_key in %s", *cfgPath)
	}
	chatUC := usecase.NewChatUseCase(chatRepo, ai, subUC, cfg.Runtime.Dev)

	pgw, err := payAdapters.NewZarinPalGateway(cfg.Payment.ZarinPal.MerchantID, cfg.Payment.ZarinPal.CallbackURL, cfg.Payment.ZarinPal.Sandbox)
	if err != nil {
		log.Fatalf("zarinpal gateway: %v", err)
	}
	paymentUC := usecase.NewPaymentUseCase(payRepo, planRepo, pgw)

	statsUC := usecase.NewStatsUseCase(userRepo, subRepo, payRepo)
	notifUC := usecase.NewNotificationUseCase(subRepo)
	_ = statsUC
	_ = notifUC

	// ---- Facade ----
	cbURL := cfg.Payment.ZarinPal.CallbackURL
	facade := application.NewBotFacade(userUC, planUC, subUC, paymentUC, chatUC, cbURL)

	// ---- Telegram ----
	botAdapter, err := tele.NewRealTelegramBotAdapter(&cfg.Bot, userRepo, facade, rateLimiter, 8)
	if err != nil {
		log.Fatalf("telegram: %v", err)
	}
	if strings.ToLower(cfg.Bot.Mode) != "polling" {
		log.Printf("bot.mode=%s not implemented; falling back to polling", cfg.Bot.Mode)
	}
	go func() {
		if err := botAdapter.StartPolling(ctx); err != nil {
			log.Printf("telegram polling stopped: %v", err)
		}
	}()

	// ---- HTTP callback server ----
	cbPath := "/api/payment/callback"
	if u := strings.TrimSpace(cfg.Payment.ZarinPal.CallbackURL); u != "" {
		if parsed, err := url.Parse(u); err == nil && parsed.Path != "" {
			cbPath = parsed.Path
		}
	}
	srv := httpapi.NewServer(paymentUC, cbPath)
	mux := http.NewServeMux()
	srv.Register(mux)
	httpPort := cfg.Payment.ZarinPal.CallbackPort
	if httpPort == 0 {
		httpPort = cfg.Admin.Port
	}
	server := &http.Server{Addr: fmt.Sprintf(":%d", httpPort), Handler: mux}
	go func() {
		log.Printf("http callback listening on %s path=%s", server.Addr, cbPath)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("http server error: %v", err)
		}
	}()

	// ---- Expiry worker (hourly) ----
	worker := sched.NewExpiryWorker(1*time.Hour, subRepo, planRepo)
	go func() { _ = worker.Run(ctx) }()

	// ---- Graceful shutdown ----
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM)
	<-sigc
	log.Println("shutdown requested")
	cancel()
}
