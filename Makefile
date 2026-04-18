# Devices API — developer Makefile.
# Run `make help` to list available targets.

SHELL         := /usr/bin/env bash
.SHELLFLAGS   := -eu -o pipefail -c
.DEFAULT_GOAL := help

# Load local environment if present (silently ignored when absent).
-include .env
export

# -----------------------------------------------------------------------------
# Configuration
# -----------------------------------------------------------------------------

BINARY       := devices-api
BUILD_DIR    := bin
PKG          := ./...
DATABASE_URL ?= postgres://devices:devices@localhost:5432/devices?sslmode=disable

MIGRATE_VERSION := v4.18.3
MIGRATE         := go run -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@$(MIGRATE_VERSION)

# -----------------------------------------------------------------------------
# Help (default target)
# -----------------------------------------------------------------------------

.PHONY: help
help: ## Show this help message.
	@awk 'BEGIN {FS = ":.*##"; printf "\nTargets:\n\n"} /^[a-zA-Z0-9_-]+:.*##/ { printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)
	@echo ""

# -----------------------------------------------------------------------------
# Build / Run
# -----------------------------------------------------------------------------

.PHONY: build
build: ## Build the API binary into ./bin.
	@mkdir -p $(BUILD_DIR)
	go build -trimpath -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY) ./cmd/api

.PHONY: run
run: ## Run the API locally.
	go run ./cmd/api

.PHONY: tidy
tidy: ## Ensure go.mod and go.sum are clean.
	go mod tidy

.PHONY: fmt
fmt: ## Format source files with gofmt.
	gofmt -s -w .

# -----------------------------------------------------------------------------
# Tests
# -----------------------------------------------------------------------------

.PHONY: test
test: ## Run unit tests with race detector (short mode, skips integration).
	go test -race -short $(PKG)

.PHONY: test-integration
test-integration: ## Run integration tests (requires Docker for testcontainers).
	go test -race -tags=integration -count=1 $(PKG)

.PHONY: test-cover
test-cover: ## Run tests with coverage report.
	go test -race -coverprofile=coverage.out $(PKG)
	@go tool cover -func=coverage.out | tail -1

# -----------------------------------------------------------------------------
# Lint
# -----------------------------------------------------------------------------

.PHONY: lint
lint: ## Run golangci-lint.
	golangci-lint run

# -----------------------------------------------------------------------------
# Database migrations
# -----------------------------------------------------------------------------

.PHONY: migrate-up
migrate-up: ## Apply all pending migrations.
	$(MIGRATE) -path ./migrations -database "$(DATABASE_URL)" up

.PHONY: migrate-down
migrate-down: ## Revert the last applied migration.
	$(MIGRATE) -path ./migrations -database "$(DATABASE_URL)" down 1

.PHONY: migrate-create
migrate-create: ## Create a new migration pair. Usage: make migrate-create name=<name>
	@test -n "$(name)" || (echo "usage: make migrate-create name=<migration_name>"; exit 1)
	$(MIGRATE) create -ext sql -dir ./migrations -seq $(name)

# -----------------------------------------------------------------------------
# Docker Compose
# -----------------------------------------------------------------------------

.PHONY: docker-up
docker-up: ## Start the full stack (api + postgres + migrate).
	docker compose up -d --build

.PHONY: docker-down
docker-down: ## Stop the stack (keeps volumes).
	docker compose down

.PHONY: docker-logs
docker-logs: ## Tail logs from all services.
	docker compose logs -f
