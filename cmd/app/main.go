// File: cmd/app/main.go
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"

	"telegram-ai-subscription/internal/config"
	"telegram-ai-subscription/internal/infrastructure/db"
	"telegram-ai-subscription/internal/infrastructure/telegram"
	"telegram-ai-subscription/internal/usecase"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Load config
	cfg, err := config.LoadConfig()
	if err != nil {
		dir, oerr := os.Getwd()
		if oerr != nil {
			log.Println("Error:", oerr)
			return
		}
		log.Println("Current Directory:", dir)
		log.Fatalf("failed to load config: %v", err)
	}

	// Connect to Postgres
	pool, err := pgxpool.Connect(context.Background(), cfg.Database.URL)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	// ensure pool closed at exit
	defer pool.Close()

	// Repositories
	userRepo := db.NewPostgresUserRepository(pool)
	if userRepo == nil {
		log.Fatalf("failed to init user repo")
	}

	subRepo := db.NewPostgresSubscriptionRepo(pool)
	if subRepo == nil {
		log.Fatalf("failed to init subscription repo")
	}

	payRepo := db.NewPostgresPaymentRepo(pool)
	if payRepo == nil {
		log.Fatalf("failed to init payment repo")
	}

	// Usecases
	statsUC := usecase.NewStatsUseCase(userRepo, subRepo, payRepo)

	// Create RealTelegramBotAdapter with concurrency workers (5 here)
	updateWorkers := 5
	botAdapter, err := telegram.NewRealTelegramBotAdapter(&cfg.Bot, userRepo, statsUC, updateWorkers)
	if err != nil {
		log.Fatalf("failed to init telegram bot adapter: %v", err)
	}

	// Start polling updates in background
	go func() {
		if err := botAdapter.StartPolling(ctx); err != nil {
			log.Printf("telegram polling stopped with error: %v", err)
		}
	}()

	// Graceful shutdown handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	<-sigCh
	log.Println("Shutting down gracefully...")
	// stop bot polling
	botAdapter.StopPolling()

	// give components a moment to finish
	time.Sleep(2 * time.Second)
}
