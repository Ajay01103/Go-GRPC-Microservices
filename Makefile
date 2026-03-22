PROTO_DIR   := proto
AUTH_SVC    := services/auth
AUTH_PB_OUT := $(AUTH_SVC)/gen/pb

-include .env
export

.PHONY: help proto sqlc migrate-up migrate-down build run docker-up docker-down tidy

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

# ─── Code Generation ──────────────────────────────────────────────────────────

proto: ## Generate gRPC Go and TS code from proto files
	-@mkdir $(AUTH_PB_OUT) 2>nul || exit 0
	cd $(PROTO_DIR)/auth && npx @bufbuild/buf generate
	@echo "✓ Proto generated"

sqlc: ## Generate type-safe Go from SQL (auth service)
	cd $(AUTH_SVC) && sqlc generate
	@echo "✓ sqlc generated"

# ─── Database Migrations (Goose) ────────────────────────────────────────────────

migrate-up: ## Run all pending migrations
	goose -dir $(AUTH_SVC)/db/migrations postgres "$(DB_URL)" up

migrate-down: ## Roll back the last migration
	goose -dir $(AUTH_SVC)/db/migrations postgres "$(DB_URL)" down

migrate-status: ## Show migration status
	goose -dir $(AUTH_SVC)/db/migrations postgres "$(DB_URL)" status

# ─── Build & Run ──────────────────────────────────────────────────────────────

build: ## Build the auth service binary
	cd $(AUTH_SVC) && go build -o ../../bin/auth ./cmd/

run: ## Run the auth service (loads .env)
	cd $(AUTH_SVC) && go run ./cmd/

tidy: ## Tidy Go modules
	cd $(AUTH_SVC) && go mod tidy
	go work sync

# ─── Docker ───────────────────────────────────────────────────────────────────

docker-up: ## Start all containers (detached)
	docker compose --env-file .env up -d

docker-down: ## Stop and remove containers
	docker compose down

docker-logs: ## Tail logs for all services
	docker compose logs -f
