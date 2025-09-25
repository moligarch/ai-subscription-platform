# Makefile - dev / infra / debug helpers
# Put this at the repo root.

# --- Configurable variables ---
DC ?= docker compose
DC_DEBUG_FILES = -f docker-compose.yml -f docker-compose.debug.yml

UI_DIR := ui
DEPLOY_UI_DIR := deploy/admin-ui
MAIN_PKG ?= ./cmd/app

# choose docker-compose service names used in repo
APP_SERVICE := app
CADDY_SERVICE := caddy

# default target
.DEFAULT_GOAL := help

# --- Phony targets ---
.PHONY: help infra-up infra-down build-prod run-prod build-debug run-debug stop-debug restart-caddy \
        build-ui deploy-ui clean-ui logs-app logs-caddy ps seed e2e test clean all

# --- Help ---
help: ## Show help for Makefile targets
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-22s\033[0m %s\n", $$1, $$2}'

# --- Infrastructure ---
infra-up: ## Start only infrastructure containers (postgres, redis, prometheus, grafana, caddy)
	$(DC) up -d postgres redis prometheus grafana $(CADDY_SERVICE)

infra-down: ## Stop & Remove infrastructure containers
	$(DC) down -v postgres redis prometheus grafana $(CADDY_SERVICE)

infra-stop: ## Stop infrastructure containers (does not remove volumes/images)
	$(DC) stop postgres redis prometheus grafana $(CADDY_SERVICE)

# --- Production build / run ---
build-prod: ## Build the production app image (uses Dockerfile in repo)
	$(DC) build $(APP_SERVICE)

run-prod: build-prod ## Build and run the app service in production mode
	$(DC) up -d $(APP_SERVICE)

# --- Debug build / run (Delve) ---
build-debug: ## Build the debug app image (uses Dockerfile.debug via override compose)
	$(DC) $(DC_DEBUG_FILES) build $(APP_SERVICE)

run-debug: build-debug ## Run the debug app (Delve/DAP); binds Delve to localhost:40000 per debug compose
	$(DC) $(DC_DEBUG_FILES) up -d $(APP_SERVICE)

stop-debug: ## Stop the debug app container
	$(DC) $(DC_DEBUG_FILES) stop $(APP_SERVICE)

restart-caddy: ## Recreate caddy to pick up Caddyfile changes
	$(DC) up -d --force-recreate $(CADDY_SERVICE)

# --- UI helpers ---
build-ui: ## Build Svelte UI locally (npm/yarn must be installed on host)
	cd $(UI_DIR) && npm ci && npm run build

deploy-ui: build-ui ## Copy built UI into deploy/admin-ui for Caddy to serve
	@mkdir -p $(DEPLOY_UI_DIR)
	@rm -rf $(DEPLOY_UI_DIR)/*
	@cp -r $(UI_DIR)/dist/* $(DEPLOY_UI_DIR)/

clean-ui: ## Remove deployed UI files
	@rm -rf $(DEPLOY_UI_DIR)/*

# --- Logs / status ---
logs-app: ## Tail logs from the app container
	$(DC) logs -f $(APP_SERVICE)

logs-caddy: ## Tail logs from the caddy container
	$(DC) logs -f $(CADDY_SERVICE)

ps: ## Show docker-compose status
	$(DC) ps

psd: ## show docker compose status when running in debug mode
	$(DC) $(DC_DEBUG_FILES) ps

# --- Utilities for running helper commands inside container or using golang image ---
seed: ## Run the seed command using a ephemeral golang container (mounts config.yaml)
	docker run --rm -it -v "$(PWD)":/src -w /src -v "./config.yaml":/etc/app/config.yaml:ro -e CONFIG_PATH=/etc/app/config.yaml golang:1.24-alpine \
		sh -c 'go mod download && go run ./cmd/seed --config /etc/app/config.yaml'

e2e: ## Run e2e-setup tool similarly to seed (useful for test setup)
	docker run --rm -it -v "$(PWD)":/src -w /src -v "./config.yaml":/etc/app/config.yaml:ro -e CONFIG_PATH=/etc/app/config.yaml golang:1.24-alpine \
		sh -c 'go mod download && go run ./cmd/e2e-setup --config /etc/app/config.yaml'

test: ## Run unit tests in a temporary golang container
	docker run --rm -it -v "$(PWD)":/src -w /src golang:1.24-alpine sh -c 'go test ./...'

# --- Housekeeping ---
clean: clean-ui ## Extendable; currently removes deployed UI

all: infra-up run-prod ## Convenience: start infra and run production app
