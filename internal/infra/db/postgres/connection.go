package postgres

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
)

// MustConnectPostgres returns a live *pgxpool.Pool or fatals.
func MustConnectPostgres() *pgxpool.Pool {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.Connect(ctx, dsn)
	if err != nil {
		log.Fatalf("pgxpool.Connect failed: %v", err)
	}
	return pool
}
