// File: cmd/app/main.go
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	pg "telegram-ai-subscription/internal/infra/db/postgres"
	"telegram-ai-subscription/internal/infra/sched"

	"telegram-ai-subscription/internal/application"
	"telegram-ai-subscription/internal/config"
	"telegram-ai-subscription/internal/domain/ports/adapter"
	aiAdapters "telegram-ai-subscription/internal/infra/adapters/ai"
	payAdapters "telegram-ai-subscription/internal/infra/adapters/payment"
	tele "telegram-ai-subscription/internal/infra/adapters/telegram"

	"telegram-ai-subscription/internal/infra/api"
	"telegram-ai-subscription/internal/infra/logging"
	red "telegram-ai-subscription/internal/infra/redis"
	"telegram-ai-subscription/internal/infra/security"
	"telegram-ai-subscription/internal/usecase"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	// ---- logging (zerolog) ----
	logger := logging.New(cfg.Log, cfg.Runtime.Dev)

	if cfg.Runtime.Dev {
		logger.Info().Msg("[DEV MODE] Enabled")
	}

	// ---- postgres ----
	pool, err := pg.NewPgxPool(ctx, cfg.Database.URL, 10)
	if err != nil {
		logger.Fatal().Err(err).Msg("postgres")
	}
	defer pool.Close()

	// ---- redis ----
	redisClient, err := red.NewClient(ctx, &cfg.Redis)
	if err != nil {
		logger.Fatal().Err(err).Msg("redis")
	}
	rateLimiter := red.NewRateLimiter(redisClient)
	chatCache := red.NewChatCache(redisClient, cfg.Redis.TTL)
	locker := red.NewLocker(redisClient)

	// ---- encryption ----
	encKey := cfg.Security.EncryptionKey
	if len(encKey) != 32 {
		logger.Warn().Msg("security.encryption_key not 32 bytes; using insecure dev key")
		encKey = "0123456789abcdef0123456789abcdef"
	}
	encSvc, err := security.NewEncryptionService(encKey)
	if err != nil {
		logger.Fatal().Err(err).Msg("encryption")
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

	// ---- AI Adapters (Metis -> Gemini -> OpenAI), then concurrency limiter ----
	var ai adapter.AIServiceAdapter
	if cfg.AI.MetisKey != "" {
		ai, err = aiAdapters.NewMetisOpenAIAdapter(cfg.AI.MetisKey, cfg.AI.DefaultModel, cfg.AI.MetisBaseURL)
		if err != nil {
			logger.Fatal().Err(err).Msg("metis adapter")
		}
		logger.Info().Str("base", cfg.AI.MetisBaseURL).Str("model", cfg.AI.DefaultModel).Msg("AI=Metis(OpenAI)")
	} else if cfg.AI.GeminiKey != "" {
		ai, err = aiAdapters.NewGeminiAdapter(cfg.AI.GeminiKey, cfg.AI.GeminiURL, []string{cfg.AI.DefaultModel})
		if err != nil {
			logger.Fatal().Err(err).Msg("gemini adapter")
		}
		logger.Info().Str("base", cfg.AI.GeminiURL).Str("model", cfg.AI.DefaultModel).Msg("AI=Gemini")
	} else if cfg.AI.OpenAIKey != "" {
		ai, err = aiAdapters.NewOpenAIAdapter(cfg.AI.OpenAIKey, cfg.AI.DefaultModel)
		if err != nil {
			logger.Fatal().Err(err).Msg("openai adapter")
		}
		logger.Info().Str("model", cfg.AI.DefaultModel).Msg("AI=OpenAI")
	} else {
		logger.Fatal().Msg("no AI provider configured")
	}
	ai = aiAdapters.NewLimitedAI(ai, cfg.AI.ConcurrentLimit)

	chatUC := usecase.NewChatUseCase(chatRepo, ai, subUC, locker, logger, cfg.Runtime.Dev)

	zp, err := payAdapters.NewZarinPalGateway(cfg.Payment.ZarinPal.MerchantID, cfg.Payment.ZarinPal.CallbackURL, cfg.Payment.ZarinPal.Sandbox)
	if err != nil {
		logger.Fatal().Err(err).Msg("zarinpal gateway")
	}
	paymentUC := usecase.NewPaymentUseCase(payRepo, planRepo, zp)

	statsUC := usecase.NewStatsUseCase(userRepo, subRepo, payRepo)
	notifUC := usecase.NewNotificationUseCase(subRepo)
	_ = statsUC
	_ = notifUC

	// ---- Facade ----
	cbURL := cfg.Payment.ZarinPal.CallbackURL
	facade := application.NewBotFacade(userUC, planUC, subUC, paymentUC, chatUC, cbURL)

	// ---- Telegram ----
	botAdapter, err := tele.NewRealTelegramBotAdapter(&cfg.Bot, userRepo, facade, rateLimiter, cfg.Bot.Workers)
	if err != nil {
		logger.Fatal().Err(err).Msg("telegram adapter")
	}
	if strings.ToLower(cfg.Bot.Mode) != "polling" {
		logger.Warn().Str("mode", cfg.Bot.Mode).Msg("bot.mode not implemented; using polling")
	}
	go func() {
		if err := botAdapter.StartPolling(ctx); err != nil {
			logger.Error().Err(err).Msg("telegram polling stopped")
		}
	}()

	// ---- HTTP callback server with guards ----
	cbPath := "/api/payment/callback"
	if u := strings.TrimSpace(cfg.Payment.ZarinPal.CallbackURL); u != "" {
		if parsed, err := url.Parse(u); err == nil && parsed.Path != "" {
			cbPath = parsed.Path
		}
	}
	srv := api.NewServer(paymentUC, cbPath)
	mux := http.NewServeMux()
	srv.Register(mux)

	handler := api.Chain(mux,
		api.TraceID(logger),
		api.RequestLog(logger),
		api.Recover(logger),
		api.Timeout(2*time.Second),
	)

	httpPort := cfg.Payment.ZarinPal.CallbackPort
	if httpPort == 0 {
		httpPort = cfg.Admin.Port
	}
	server := &http.Server{Addr: fmt.Sprintf(":%d", httpPort), Handler: handler}
	go func() {
		logger.Info().Str("addr", server.Addr).Str("path", cbPath).Msg("http callback listening")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error().Err(err).Msg("http server error")
		}
	}()

	// ---- Expiry worker: hourly sweep ----
	worker := sched.NewExpiryWorker(1*time.Hour, subRepo, planRepo)
	go func() { _ = worker.Run(ctx) }()

	// ---- Shutdown ----
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM)
	<-sigc
	logger.Info().Msg("shutdown requested")
	cancel()
	_ = server.Shutdown(context.Background())
}
