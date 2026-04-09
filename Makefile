PROTO_DIR   := proto
AUTH_SVC    := services/auth
VOICE_SVC   := services/voice
GEN_SVC     := services/generation

AUTH_PB_OUT := $(AUTH_SVC)/gen/pb
VOICE_PB_OUT := $(VOICE_SVC)/gen/pb
GEN_PB_OUT  := $(GEN_SVC)/gen/pb

# Do not globally export per-service .env values here; Goose variables can
# collide across services and cause migrations to run against the wrong DB.

.PHONY: help proto sqlc migrate-up migrate-down migrate-status build run-auth run-voice run-gen docker-up docker-down docker-logs tidy

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

# ─── Code Generation ──────────────────────────────────────────────────────────

proto: ## Generate gRPC Go and TS code from proto files
	-@mkdir $(AUTH_PB_OUT) 2>nul || exit 0
	-@mkdir $(VOICE_PB_OUT) 2>nul || exit 0
	-@mkdir $(GEN_PB_OUT) 2>nul || exit 0
	cd $(PROTO_DIR)/auth && npx @bufbuild/buf generate
	cd $(PROTO_DIR)/voice && npx @bufbuild/buf generate
	cd $(PROTO_DIR)/generation && npx @bufbuild/buf generate
	@echo "✓ Proto generated"

sqlc: ## Generate type-safe Go from SQL for all services
	cd $(AUTH_SVC) && sqlc generate
	cd $(VOICE_SVC) && sqlc generate
	cd $(GEN_SVC) && sqlc generate
	@echo "✓ sqlc generated"

# ─── Database Migrations (Goose) ──────────────────────────────────────────────

migrate-up: ## Run all pending migrations
	cd $(AUTH_SVC) && goose -env .env -dir db/migrations up
	cd $(VOICE_SVC) && goose -env .env -dir db/migrations up
	cd $(GEN_SVC) && goose -env .env -dir db/migrations up

migrate-down: ## Rollbacking migrations to prev versions
	cd $(AUTH_SVC) && goose -env .env -dir db/migrations down
	cd $(VOICE_SVC) && goose -env .env -dir db/migrations down
	cd $(GEN_SVC) && goose -env .env -dir db/migrations down

migrate-reset: ## Goose migration down
	cd $(AUTH_SVC) && goose -env .env -dir db/migrations reset
	cd $(VOICE_SVC) && goose -env .env -dir db/migrations reset
	cd $(GEN_SVC) && goose -env .env -dir db/migrations reset

migrate-status: ## Show migration status
	cd $(AUTH_SVC) && goose -env .env -dir db/migrations status
	cd $(VOICE_SVC) && goose -env .env -dir db/migrations status
	cd $(GEN_SVC) && goose -env .env -dir db/migrations status

# ─── Build & Run ──────────────────────────────────────────────────────────────

build: ## Build the service binaries
	cd $(AUTH_SVC) && go build -o ../../bin/auth ./cmd/
	cd $(VOICE_SVC) && go build -o ../../bin/voice ./cmd/
	cd $(GEN_SVC) && go build -o ../../bin/generation ./cmd/

run-auth: ## Start Auth service
	cd $(AUTH_SVC) && go run ./cmd/

run-voice: ## Start Voice service
	cd $(VOICE_SVC) && go run ./cmd/

run-gen: ## Start Generation service
	cd $(GEN_SVC) && go run ./cmd/

seed-voices: ## Seed system voices into the voice database
	cd $(VOICE_SVC) && go run ./scripts/seed-system-voices.go

tidy: ## Tidy Go modules
	cd $(AUTH_SVC) && go mod tidy
	cd $(VOICE_SVC) && go mod tidy
	cd $(GEN_SVC) && go mod tidy
	go work sync

# ─── Docker ───────────────────────────────────────────────────────────────────

docker-up: ## Start all containers (detached)
	docker compose --env-file .env up -d

docker-down: ## Stop and remove containers
	docker compose down

docker-logs: ## Tail logs for all services
	docker compose logs -f
