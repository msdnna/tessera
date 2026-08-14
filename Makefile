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

.PHONY: test-backend-cover
test-backend-cover: ## Backend tests with coverage (backend/coverage.out + cover.html); needs tessera_test DB
	@# -count=1: without it, cached package results contribute nothing to the
	@# merged -coverpkg profile, undercounting total coverage.
	cd backend && $(GO) test -count=1 -race -covermode=atomic -coverpkg=./... -coverprofile=coverage.out.raw ./...
	@# Denominator excludes generated sqlc code, CLI wiring and the embed stub —
	@# see task #2579: those are e2e/codegen territory, not unit-testable logic.
	@cd backend && grep -v -E '^tessera/(internal/db/|cmd/|main\.go|migrations/)' coverage.out.raw > coverage.out
	@rm -f backend/coverage.out.raw
	cd backend && $(GO) tool cover -func=coverage.out | tail -1
	cd backend && $(GO) tool cover -html=coverage.out -o cover.html
	@echo "Coverage report: backend/cover.html"

.PHONY: test-e2e-backend
test-e2e-backend: ## Black-box e2e: real binaries as subprocesses on a throwaway DB (needs Postgres)
	@# Build-tagged, so it is not part of test-backend. E2E_GO hands the suite the
	@# pinned toolchain — it shells out to build the binaries it then runs.
	cd backend && E2E_GO=$(GO) $(GO) test -tags=e2e -count=1 -timeout 15m ./e2e/...

.PHONY: test-e2e-backend-docker
test-e2e-backend-docker: ## E2e including the image tier (docker build + run; several minutes)
	cd backend && E2E_GO=$(GO) E2E_DOCKER=1 $(GO) test -tags=e2e -count=1 -timeout 30m -v ./e2e/...

.PHONY: lint-frontend
lint-frontend: ## Lint + format-check frontend
	cd frontend && corepack yarn lint && corepack yarn format:check

.PHONY: test-frontend
test-frontend: ## Run frontend tests
	cd frontend && corepack yarn test

.PHONY: test-frontend-cover
test-frontend-cover: ## Frontend tests with coverage (frontend/coverage/{index.html,lcov.info})
	cd frontend && corepack yarn test:coverage
	@echo "Coverage report: frontend/coverage/index.html"

# ── Web e2e (Playwright) ───────────────────────────────────
# The browser suite needs a real backend. It is NOT started by Playwright:
# the backend needs migrations applied to tessera_test, which is the DB's
# business, not the test runner's. Bring it up once, then re-run the suite as
# often as you like. Port 8092 — :8090 may hold a zombie with older code.
#
# The auth rate limiter is off here, as it is in the backend e2e harness
# (`backend/e2e/harness_test.go`): every spec seeds its own account, so a suite
# of any size burns through the 10-per-IP register budget and starts failing on
# HTTP 429 instead of on a defect. The limiter itself is covered by
# `backend/middleware/ratelimit_test.go`, so nothing goes untested.
E2E_PORT ?= 8092
E2E_DB_URL ?= postgres://tessera:tessera@localhost:5432/tessera_test?sslmode=disable

.PHONY: e2e-backend-up
e2e-backend-up: ## Start a throwaway backend on :8092 against tessera_test (for web e2e)
	cd backend && DATABASE_URL="$(E2E_DB_URL)" $(GO) run ./cmd/migrate
	cd backend && $(GO) build -o /tmp/tessera-e2e-bin .
	@PORT=$(E2E_PORT) UPLOAD_DIR=/tmp/tessera-e2e-uploads JWT_SECRET=e2e \
		DATABASE_URL="$(E2E_DB_URL)" \
		RATE_LIMIT_ENABLED=false \
		nohup /tmp/tessera-e2e-bin > /tmp/tessera-e2e-backend.log 2>&1 & \
		for i in $$(seq 1 40); do sleep 0.5; \
			curl -sf http://localhost:$(E2E_PORT)/api/health > /dev/null && \
			echo "e2e backend up on :$(E2E_PORT) (log: /tmp/tessera-e2e-backend.log)" && exit 0; \
		done; echo "e2e backend did not come up; see /tmp/tessera-e2e-backend.log" >&2; exit 1

.PHONY: e2e-backend-down
e2e-backend-down: ## Stop the throwaway e2e backend
	@fuser -k $(E2E_PORT)/tcp 2>/dev/null || true
	@echo "e2e backend on :$(E2E_PORT) stopped"

.PHONY: test-e2e-frontend
test-e2e-frontend: ## Run the Playwright web e2e suite (needs `make e2e-backend-up`)
	cd frontend && corepack yarn build && corepack yarn e2e

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

.PHONY: test-mcp-cover
test-mcp-cover: ## MCP tests with coverage (mcp/coverage.out + cover.html)
	cd mcp && $(GO) test -count=1 -covermode=atomic -coverpkg=./... -coverprofile=coverage.out.raw ./...
	@# Exclude the stdio-transport glue in main.go — only exercised end-to-end.
	@cd mcp && grep -v -E '/main\.go:' coverage.out.raw > coverage.out
	@rm -f mcp/coverage.out.raw
	cd mcp && $(GO) tool cover -func=coverage.out | tail -1
	cd mcp && $(GO) tool cover -html=coverage.out -o cover.html
	@echo "Coverage report: mcp/cover.html"

.PHONY: coverage-report
coverage-report: ## Aggregate every component's coverage into reports/coverage/index.html
	@echo "Run the per-component cover targets first (test-backend-cover / test-frontend-cover / test-android-cover / test-mcp-cover)."
	@GO=$(GO) python3 tools/coverage-report.py .

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

# ── Changelog fragments (feature branch → develop) ─────────
.PHONY: changelog-add
changelog-add: ## Scaffold a changelog fragment (COMP=backend TASK=2620 [SLUG=ws-auth] [BUMP=minor])
	@./tools/changelog-add.sh $(COMP) $(TASK) $(SLUG) $(BUMP)

.PHONY: changelog-release
changelog-release: ## Assemble fragments + bump versions on develop (ONLY=backend,frontend DRY=1)
	@python3 tools/changelog-release.py $(if $(DRY),--dry-run,) $(if $(ONLY),--only $(ONLY),)

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

.PHONY: test-e2e-android
test-e2e-android: ## Android e2e suite against the live backend (needs `make e2e-backend-up`)
	@$(ANDROID_GRADLE) :app:testDebugUnitTest -Pe2e --tests 'website.msdnna.tessera.e2e.*'

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
