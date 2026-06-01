GO ?= go1.25.9
COMPOSE ?= docker compose

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

# ── Local dev ──────────────────────────────────────────────
.PHONY: dev
dev: db-up ## Run backend on the host (Postgres in Docker)
	cd backend && $(GO) run .

.PHONY: db-up
db-up: ## Start only Postgres
	$(COMPOSE) up -d postgres

.PHONY: db-down
db-down: ## Stop Postgres
	$(COMPOSE) stop postgres

# ── Docker ─────────────────────────────────────────────────
.PHONY: up
up: ## Build + start all services
	$(COMPOSE) up -d --build

.PHONY: down
down: ## Stop all services
	$(COMPOSE) down

.PHONY: logs
logs: ## Tail service logs
	$(COMPOSE) logs -f

# ── Migrations ─────────────────────────────────────────────
.PHONY: migrate
migrate: ## Apply pending migrations (host -> localhost:5432)
	cd backend && $(GO) run ./cmd/migrate

.PHONY: migrate-down
migrate-down: ## Roll back one migration
	cd backend && $(GO) run ./cmd/migrate -down 1

.PHONY: migrate-version
migrate-version: ## Print current schema version
	cd backend && $(GO) run ./cmd/migrate -version

# ── Quality gate ───────────────────────────────────────────
.PHONY: tidy
tidy: ## go mod tidy
	cd backend && $(GO) mod tidy

.PHONY: lint-backend
lint-backend: ## Run golangci-lint
	cd backend && golangci-lint run ./...

.PHONY: test-backend
test-backend: ## Run backend tests
	cd backend && $(GO) test ./...

# ── Versioning ─────────────────────────────────────────────
.PHONY: version
version: ## Show service versions
	@echo "backend: $$(cat backend/VERSION)"

.PHONY: bump-api
bump-api: ## Bump backend version (BUMP=patch|minor|major)
	@./tools/bump-version.sh backend $(or $(BUMP),patch)
