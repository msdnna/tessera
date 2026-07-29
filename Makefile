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

.PHONY: lint-frontend
lint-frontend: ## Lint + format-check frontend
	cd frontend && corepack yarn lint && corepack yarn format:check

.PHONY: test-frontend
test-frontend: ## Run frontend tests
	cd frontend && corepack yarn test

.PHONY: build-mcp
build-mcp: ## Build the Tessera MCP server binary (mcp/tessera-mcp)
	cd mcp && $(GO) build -ldflags "-X main.version=$$(cat VERSION)" -o tessera-mcp .

.PHONY: run-mcp
run-mcp: ## Run the MCP server (needs TESSERA_TOKEN; TESSERA_BASE_URL optional)
	cd mcp && $(GO) run .

.PHONY: lint-mcp
lint-mcp: ## Run golangci-lint on the MCP server
	cd mcp && golangci-lint run ./...

.PHONY: test-mcp
test-mcp: ## Run MCP server tests
	cd mcp && $(GO) test ./...

.PHONY: lint
lint: lint-backend lint-frontend lint-mcp ## Lint everything

.PHONY: test
test: test-backend test-frontend test-mcp ## Test everything

# ── Versioning ─────────────────────────────────────────────
.PHONY: version
version: ## Show service versions
	@echo "backend:  $$(cat backend/VERSION)"
	@echo "frontend: $$(cat frontend/VERSION)"
	@echo "android:  $$(cat android/VERSION)"
	@echo "desktop:  $$(cat desktop/VERSION)"
	@echo "mcp:      $$(cat mcp/VERSION)"

.PHONY: bump-api
bump-api: ## Bump backend version (BUMP=patch|minor|major)
	@./tools/bump-version.sh backend $(or $(BUMP),patch)

.PHONY: bump-web
bump-web: ## Bump frontend version (BUMP=patch|minor|major)
	@./tools/bump-version.sh frontend $(or $(BUMP),patch)

.PHONY: bump-android
bump-android: ## Bump Android version (BUMP=patch|minor|major)
	@./tools/bump-version.sh android $(or $(BUMP),patch)

.PHONY: bump-desktop
bump-desktop: ## Bump desktop version (BUMP=patch|minor|major)
	@./tools/bump-version.sh desktop $(or $(BUMP),patch)

.PHONY: bump-mcp
bump-mcp: ## Bump MCP server version (BUMP=patch|minor|major)
	@./tools/bump-version.sh mcp $(or $(BUMP),patch)

# ── Android ────────────────────────────────────────────────
ANDROID_DIR := android

# Gradle wrapper invocation that sources android/local.env (SDK/JDK paths +
# optional SOCKS proxy), mirroring the build scripts.
ANDROID_GRADLE := cd $(ANDROID_DIR) && set -a && [ -f ./local.env ] && . ./local.env; set +a; \
  ANDROID_HOME="$${ANDROID_HOME:-$$HOME/Android/Sdk}" \
  JAVA_HOME="$${JAVA_HOME:-/usr/lib/jvm/java-21-openjdk-amd64}" \
  GRADLE_OPTS="$${SOCKS_PROXY_HOST:+-DsocksProxyHost=$$SOCKS_PROXY_HOST -DsocksProxyPort=$$SOCKS_PROXY_PORT -DsocksProxyVersion=5} -Dorg.gradle.internal.http.socketTimeout=300000" \
  ./gradlew --no-daemon

.PHONY: android
android: ## Build debug Android APK (android/msdnna-tessera-v<version>.apk)
	cd $(ANDROID_DIR) && ./build.sh

.PHONY: android-release
android-release: ## Build signed release APK (requires ANDROID_KEYSTORE_* env vars)
	./tools/build-android-release.sh

.PHONY: lint-android
lint-android: ## Run ktlint + detekt on the Android app
	@$(ANDROID_GRADLE) :app:ktlintCheck :app:detekt

.PHONY: format-android
format-android: ## Auto-format Kotlin sources via ktlint
	@$(ANDROID_GRADLE) :app:ktlintFormat

.PHONY: test-android
test-android: ## Run Android unit tests
	@$(ANDROID_GRADLE) :app:testDebugUnitTest

.PHONY: test-android-cover
test-android-cover: ## Android unit tests + JaCoCo coverage report
	@$(ANDROID_GRADLE) :app:jacocoTestReport
	@echo "Coverage: $(ANDROID_DIR)/app/build/reports/jacoco/jacocoTestReport/html/index.html"

# ── Desktop (Tauri) ────────────────────────────────────────
DESKTOP_DIR := desktop/src-tauri
# Updater signing key (kept outside the repo, like the Android keystore). When
# present it's passed to bundling builds so updater artifacts get signed.
DESKTOP_KEY := $(HOME)/.tessera/tessera-desktop-updater.key
DESKTOP_SIGN := $(if $(wildcard $(DESKTOP_KEY)),TAURI_SIGNING_PRIVATE_KEY="$(DESKTOP_KEY)" TAURI_SIGNING_PRIVATE_KEY_PASSWORD="",)

.PHONY: dev-desktop
dev-desktop: ## Run the desktop app in dev (Vite :5174 + Tauri window)
	cd $(DESKTOP_DIR) && cargo tauri dev

.PHONY: desktop
desktop: ## Build desktop bundles for this OS (Linux: AppImage + .deb)
	cd $(DESKTOP_DIR) && $(DESKTOP_SIGN) cargo tauri build

.PHONY: desktop-release
desktop-release: ## Build + sign desktop bundles and assemble the updater manifest
	./tools/build-desktop-release.sh

.PHONY: lint-desktop
lint-desktop: ## cargo fmt --check + clippy on the desktop crate
	cd $(DESKTOP_DIR) && cargo fmt --check && cargo clippy -- -D warnings
