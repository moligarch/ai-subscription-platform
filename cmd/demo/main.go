package main

import (
    "context"
    "log"
    "time"

    "github.com/jackc/pgx/v4/pgxpool"

    "telegram-ai-subscription/internal/config"
    "telegram-ai-subscription/internal/infrastructure/db"
    "telegram-ai-subscription/internal/usecase"
)

func main() {
    // 1. Load config
    cfg, err := config.LoadConfig()
    if err != nil {
        log.Fatalf("config error: %v", err)
    }

    // 2. Connect to Postgres
    pool, err := pgxpool.Connect(context.Background(), cfg.Database.URL)
    if err != nil {
        log.Fatalf("db connect error: %v", err)
    }
    defer pool.Close()

    // 3. Set up only UserRepository & UserUseCase
    userRepo := db.NewPostgresUserRepository(pool)
    userUC := usecase.NewUserUseCase(userRepo)

    // 4. Test RegisterOrFetch
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    tgID := int64(42424242)
    name := "Moein Demo"
    phone := "+989123456789"

    // First call: INSERT
    user, err := userUC.RegisterOrFetch(ctx, tgID, name, phone)
    if err != nil {
        log.Fatalf("first call error: %v", err)
    }
    log.Printf("First call, got user: %+v", user)

    // Second call: FETCH
    user2, err := userUC.RegisterOrFetch(ctx, tgID, "Ignored Name", "")
    if err != nil {
        log.Fatalf("second call error: %v", err)
    }
    log.Printf("Second call, got user: %+v", user2)
}
