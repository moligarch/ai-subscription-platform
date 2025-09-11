# Makefile for the Telegram AI Subscription Bot

# --- Variables ---
# Get the current git commit hash and version tag for build stamping.
COMMIT := $(shell git rev-parse --short HEAD)
VERSION := $(shell git describe --tags --abbrev=0 2>/dev/null || echo "dev")
# Define the output directory for binaries.
OUTPUT_DIR := bin

# Use .PHONY to declare targets that are not files. This prevents conflicts
# with files of the same name and improves performance.
.PHONY: all help build build-linux build-windows test integration-test docker-up docker-down docker-run clean

# The default target executed when you run `make`.
default: help

## --------------------------------------
## Build Commands
## --------------------------------------

# Build for all target platforms.
build: build-linux build-windows
	@echo "✅ All builds completed."

# Build a static binary for Linux.
build-linux:
	@echo "Building for Linux..."
	@mkdir -p $(OUTPUT_DIR)
	@GOOS=linux GOARCH=amd64 go build -ldflags="-s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)" -o $(OUTPUT_DIR)/app-linux-amd64 ./cmd/app

# Build an executable for Windows.
build-windows:
	@echo "Building for Windows..."
	@mkdir -p $(OUTPUT_DIR)
	@GOOS=windows GOARCH=amd64 go build -ldflags="-s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)" -o $(OUTPUT_DIR)/app-windows-amd64.exe ./cmd/app

## --------------------------------------
## Test Commands
## --------------------------------------

# Run all fast unit tests (skips integration tests).
test:
	@echo "Running unit tests..."
	@go test -v -race ./...

# Run slower integration tests. Requires Docker to be running.
integration-test:
	@echo "Running integration tests (requires Docker)..."
	@go test -v -race -tags=integration ./...

## --------------------------------------
## Docker Compose Commands
## --------------------------------------

# Start all services defined in docker-compose.yml in detached mode.
docker-up:
	@echo "Starting all Docker services..."
	@docker-compose up -d

# Stop and remove all containers, networks, and volumes.
docker-down:
	@echo "Stopping and removing all Docker services and volumes..."
	@docker-compose down -v

# Run a specific service from the docker-compose file.
# Example: make docker-run service=postgres
docker-run:
	@if [ -z "$(service)" ]; then echo "Error: 'service' variable not set. Usage: make docker-run service=<service_name>"; exit 1; fi
	@echo "Starting service: $(service)..."
	@docker-compose up -d $(service)

## --------------------------------------
## Utility Commands
## --------------------------------------

# Clean up build artifacts.
clean:
	@echo "Cleaning up build artifacts..."
	@rm -rf $(OUTPUT_DIR)

# Display this help message.
help:
	@echo "Available commands:"
	@echo "  build            - Build the application for all target platforms (Linux, Windows)."
	@echo "  build-linux      - Build the application for Linux."
	@echo "  build-windows    - Build the application for Windows."
	@echo "  test             - Run all unit tests."
	@echo "  integration-test - Run all integration tests (requires Docker)."
	@echo "  docker-up        - Start all services with Docker Compose."
	@echo "  docker-down      - Stop and remove all services and volumes."
	@echo "  docker-run       - Start a specific service (e.g., 'make docker-run service=redis')."
	@echo "  clean            - Remove all build artifacts."
	@echo "  help             - Show this help message."
