package main

import (
	"context"
	"log"

	"telegram-ai-subscription/internal/config"
	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/infra/db/postgres"
	"telegram-ai-subscription/internal/infra/redis"

	"github.com/jackc/pgx/v4/pgxpool"
)

// This script is for setting up a clean, predictable database state
// for manual end-to-end testing.
func main() {
	ctx := context.Background()

	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("config load: %v", err)
	}

	// --- Connect to Postgres ---
	pool, err := postgres.NewPgxPool(ctx, cfg.Database.URL, 5)
	if err != nil {
		log.Fatalf("postgres connection failed: %v", err)
	}
	defer pool.Close()

	// --- Connect to Redis ---
	redisClient, err := redis.NewClient(ctx, &cfg.Redis)
	if err != nil {
		log.Fatalf("redis connection failed: %v", err)
	}
	defer redisClient.Close()

	log.Println("--- Starting E2E Environment Setup ---")

	// 1. Clean the Redis cache to remove any stale data.
	log.Println("[1/4] Wiping Redis cache...")
	if err := redisClient.FlushDB(ctx); err != nil {
		log.Fatalf("failed to flush redis: %v", err)
	}

	// 2. Clean the database completely.
	log.Println("[2/4] Wiping all existing database data...")
	_, err = pool.Exec(ctx, `
		TRUNCATE 
			users, subscription_plans, user_subscriptions, payments, purchases, 
			chat_sessions, chat_messages, ai_jobs, subscription_notifications,
			model_pricing, activation_codes
		RESTART IDENTITY CASCADE;
	`)
	if err != nil {
		log.Fatalf("failed to truncate tables: %v", err)
	}

	// 3. Seed the database with standard plans and pricing.
	log.Println("[3/4] Seeding standard plans and pricing...")
	seedPlansAndPricing(ctx, pool)

	log.Println("[4/4] (Optional) Seeding specific test data...")
	// You can add more specific data for your tests here if needed.

	log.Println("--- ✅ E2E Environment Setup Complete ---")
}

// seedPlansAndPricing contains the standard data needed for the bot to function.
func seedPlansAndPricing(ctx context.Context, pool *pgxpool.Pool) {
	planRepo := postgres.NewPlanRepo(pool)
	pricingRepo := postgres.NewModelPricingRepo(pool)

	// Create a "Pro" plan
	proPlan, _ := model.NewSubscriptionPlan("", "Pro", 30, 100000, 50000)
	proPlan.SupportedModels = []string{"gpt-4o", "gemini-1.5-pro"}
	if err := planRepo.Save(ctx, nil, proPlan); err != nil {
		log.Printf("failed to save pro plan: %v", err)
	}

	// Create a "Standard" plan
	stdPlan, _ := model.NewSubscriptionPlan("", "Standard", 30, 20000, 10000)
	stdPlan.SupportedModels = []string{"gpt-4o-mini", "gemini-1.5-flash"}
	if err := planRepo.Save(ctx, nil, stdPlan); err != nil {
		log.Printf("failed to save standard plan: %v", err)
	}

	// Create pricing for the models
	gpt4oMini := model.NewModelPricing("gpt-4o-mini", 15, 60, true)
	gpt4o := model.NewModelPricing("gpt-4o", 20, 65, true)
	geminiFlash := model.NewModelPricing("gemini-1.5-flash", 10, 40, true)
	geminiPro := model.NewModelPricing("gemini-1.5-pro", 15, 50, true)
	if err := pricingRepo.Create(ctx, nil, gpt4oMini); err != nil {
		log.Printf("failed to save gpt-4o-mini pricing: %v", err)
	}
	if err := pricingRepo.Create(ctx, nil, gpt4o); err != nil {
		log.Printf("failed to save gpt-4o pricing: %v", err)
	}
	if err := pricingRepo.Create(ctx, nil, geminiFlash); err != nil {
		log.Printf("failed to save gemini-1.5-flash pricing: %v", err)
	}
	if err := pricingRepo.Create(ctx, nil, geminiPro); err != nil {
		log.Printf("failed to save gemini-1.5-pro pricing: %v", err)
	}
}
