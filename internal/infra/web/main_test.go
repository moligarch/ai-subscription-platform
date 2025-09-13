package web

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

// findProjectRoot travels up to find the project root, marked by go.mod.
func findProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parentDir := filepath.Dir(dir)
		if parentDir == dir {
			break
		}
		dir = parentDir
	}
	return "", errors.New("could not find project root containing go.mod")
}

func TestMain(m *testing.M) {
	// 1. Start Docker container and get its ID
	containerID, dsn := setupTestDatabase()

	// 2. Connect to the database
	var err error
	for i := 0; i < 15; i++ {
		testPool, err = pgxpool.Connect(context.Background(), dsn)
		if err == nil {
			break
		}
		log.Println("Waiting for web test database to be ready...")
		time.Sleep(1 * time.Second)
	}
	if err != nil {
		log.Fatalf("Unable to connect to web test database: %v", err)
	}

	// 3. Apply schema
	applySchema(testPool)
	log.Println("✅ Web test database is ready.")

	// 4. Run the tests and exit
	exitCode := m.Run()

	// 5. Cleanup
	testPool.Close()
	teardownTestDatabase(containerID)
	os.Exit(exitCode)
}

func setupTestDatabase() (containerID, dsn string) {
	dbName := "web_test_db"
	dbUser := "web_test_user"
	dbPassword := "password"
	dbPort := "5432"

	cmd := exec.Command("docker", "run", "-d", "--rm",
		"--network", "host",
		"-e", fmt.Sprintf("POSTGRES_DB=%s", dbName),
		"-e", fmt.Sprintf("POSTGRES_USER=%s", dbUser),
		"-e", fmt.Sprintf("POSTGRES_PASSWORD=%s", dbPassword),
		"postgres:14-alpine",
	)

	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		log.Fatalf("could not start postgres container for web tests: %v.\n Is Docker running?", err)
	}
	containerID = strings.TrimSpace(out.String())
	dsn = fmt.Sprintf("postgres://%s:%s@localhost:%s/%s?sslmode=disable", dbUser, dbPassword, dbPort, dbName)
	return
}

func teardownTestDatabase(containerID string) {
	log.Printf("Stopping web test container %s", containerID)
	err := exec.Command("docker", "stop", containerID).Run()
	if err != nil {
		log.Printf("Warning: could not stop postgres container %s: %v", containerID, err)
	}
}

func applySchema(pool *pgxpool.Pool) {
	projectRoot, err := findProjectRoot()
	if err != nil {
		log.Fatal(err)
	}
	schemaPath := filepath.Join(projectRoot, "deploy", "postgres", "init.sql")
	schema, _ := os.ReadFile(schemaPath)
	_, err = pool.Exec(context.Background(), string(schema))
	if err != nil {
		log.Fatalf("could not apply schema for web tests: %s", err)
	}
}
