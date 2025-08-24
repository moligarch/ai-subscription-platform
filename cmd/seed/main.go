package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"telegram-ai-subscription/internal/config"
	pg "telegram-ai-subscription/internal/infra/db/postgres"
	"telegram-ai-subscription/internal/usecase"
)

func main() {
	// ---- Config ----
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Connect Postgres
	pool, err := pg.NewPgxPool(ctx, cfg.Database.URL, 4)
	if err != nil {
		log.Fatalf("postgres: %v", err)
	}
	defer pool.Close()

	planRepo := pg.NewPostgresPlanRepo(pool)
	planUC := usecase.NewPlanUseCase(planRepo)

	// If plans already exist, do nothing
	plans, err := planUC.List(ctx)
	if err != nil {
		log.Fatalf("list plans: %v", err)
	}
	if len(plans) > 0 {
		fmt.Printf("%d plans already present. No changes.\n", len(plans))
		for _, p := range plans {
			fmt.Printf("  - %s (days=%d, credits=%d, price=%d IRR)\n", p.Name, p.DurationDays, p.Credits, p.PriceIRR)
		}
		return
	}

	// Seed a few sample plans for testing payment flow
	seed := []struct {
		Name  string
		Days  int
		Cred  int
		Price int64
	}{
		{"Starter", 7, 300, 150_000},
		{"Pro", 30, 2000, 690_000},
		{"Ultra", 90, 8000, 1_890_000},
	}

	for _, s := range seed {
		p, err := planUC.Create(ctx, s.Name, s.Days, s.Cred, s.Price)
		if err != nil {
			log.Fatalf("create plan %q: %v", s.Name, err)
		}
		fmt.Printf("seeded: %s (id=%s, days=%d, credits=%d, price=%d IRR)\n", p.Name, p.ID, p.DurationDays, p.Credits, p.PriceIRR)
	}

	fmt.Println("✅ Seeding complete.")
}
