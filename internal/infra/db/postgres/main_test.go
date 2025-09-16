//go:build integration

package postgres

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
)

var testPool *pgxpool.Pool

// findProjectRoot travels up from the current directory to find the project root,
// marked by the presence of a go.mod file.
func findProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for i := 0; i < 6; i++ { // Limit to 6 levels up
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parentDir := filepath.Dir(dir)
		if parentDir == dir { // Reached the filesystem root
			break
		}
		dir = parentDir
	}
	return "", errors.New("could not find project root containing go.mod")
}

func TestMain(m *testing.M) {
	ctx := context.Background()
	dbName := "test-db"
	dbUser := "user"
	dbPassword := "password"
	dbPort := "5432"

	// 1. Start the container
	cmd := exec.Command("docker", "run", "-d", "--rm",
		"--network", "host",
		"-e", fmt.Sprintf("POSTGRES_DB=%s", dbName),
		"-e", fmt.Sprintf("POSTGRES_USER=%s", dbUser),
		"-e", fmt.Sprintf("POSTGRES_PASSWORD=%s", dbPassword),
		"postgres:14",
	)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		log.Fatalf("could not start postgres container: %v. Is Docker running?", err)
	}
	containerID := strings.TrimSpace(out.String())[:12]

	// 2. Readiness Probe and Connection
	connStr := fmt.Sprintf("postgres://%s:%s@localhost:%s/%s?sslmode=disable", dbUser, dbPassword, dbPort, dbName)
	var err error
	const maxRetries = 15
	for i := 0; i < maxRetries; i++ {
		testPool, err = pgxpool.Connect(ctx, connStr)
		if err == nil {
			break
		}
		log.Printf("Waiting for database to be ready... (attempt %d/%d)", i+1, maxRetries)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		// If we can't connect, still try to stop the container before failing.
		exec.Command("docker", "stop", containerID).Run()
		log.Fatalf("Unable to connect to test database after multiple retries: %v\n", err)
	}

	// 3. Apply Schema
	projectRoot, err := findProjectRoot()
	if err != nil {
		log.Fatalf("Error finding project root: %v", err)
	}
	schemaPath := filepath.Join(projectRoot, "deploy", "postgres", "init.sql")
	schema, err := os.ReadFile(schemaPath)
	if err != nil {
		log.Fatalf("could not read init.sql from path %s: %s", schemaPath, err)
	}
	_, err = testPool.Exec(ctx, string(schema))
	if err != nil {
		log.Fatalf("could not apply schema: %s", err)
	}
	log.Println("Test database is ready.")

	// 4. Run Tests and capture the exit code
	exitCode := m.Run()

	// 5. Cleanup: Close the pool and stop the container *before* exiting.
	testPool.Close()
	log.Println("Stopping test container...")
	if err := exec.Command("docker", "stop", containerID).Run(); err != nil {
		log.Printf("could not stop postgres container %s: %v", containerID, err)
	}

	// 6. Exit with the captured exit code
	os.Exit(exitCode)
}

func cleanup(t *testing.T) {
	t.Helper()
	_, err := testPool.Exec(context.Background(), `
		TRUNCATE 
			users, subscription_plans, user_subscriptions, payments, purchases, 
			chat_sessions, chat_messages, ai_jobs, subscription_notifications,
			model_pricing
		RESTART IDENTITY CASCADE
	`)
	if err != nil {
		t.Fatalf("Failed to clean up database: %v", err)
	}
}
