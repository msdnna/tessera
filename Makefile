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
	GIT_COMMIT="$$(git rev-parse --short HEAD 2>/dev/null || echo '')" \
	BUILD_DATE="$$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
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

.PHONY: help-index
help-index: ## Rebuild the help-centre index from docs/help
	cd frontend && node scripts/build-help-index.mjs

# Same two-address story as test-e2e-frontend below, and SHOTS_ARGS goes through
# to playwright — `make help-shots SHOTS_ARGS='-g конференц'` repaints one article's
# pictures instead of all of them, which keeps an unrelated diff out of the commit.
SHOTS_ARGS ?=
SHOTS_ENV = E2E_API_URL=http://localhost:$(E2E_PORT)/api \
	TESSERA_API_TARGET=http://localhost:$(E2E_PORT)

.PHONY: help-shots
help-shots: ## Re-take the help-centre screenshots into docs/help/assets (needs `make e2e-backend-up`)
	cd frontend && corepack yarn build
	cd frontend && $(SHOTS_ENV) corepack yarn docs:shots $(SHOTS_ARGS)

# The English twins (#2816): same run, same seed, the interface switched through
# the account's own language preference. They land next to the Russian set as
# `<name>-<scheme>.en.png` — the name helpAssets.js tries first for a reader on an
# English article, falling back to the Russian file when a twin is missing.
# Needs a *clean* E2E_DB_URL: eight of the shots are admin-only and the backend
# hands admin to the instance's first account.
.PHONY: help-shots-en
help-shots-en: ## Re-take the help-centre screenshots in English (needs a clean `make e2e-backend-up`)
	cd frontend && corepack yarn build
	cd frontend && TESSERA_SHOTS_LANG=en $(SHOTS_ENV) corepack yarn docs:shots $(SHOTS_ARGS)

.PHONY: lint-frontend
lint-frontend: ## Lint + format-check frontend
	cd frontend && corepack yarn lint && corepack yarn format:check
	@# The help index is generated but committed (it ships in the bundle), so a
	@# stale one would silently serve yesterday's nav and search. Checked here
	@# rather than built on demand: lint must not rewrite tracked files.
	cd frontend && node scripts/build-help-index.mjs --check

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
# Where this backend keeps uploads. A variable rather than a literal because the
# recording stand (#2877) has to mount the SAME directory into the egress
# container: the backend and the recorder name the file by one absolute path, and
# a mismatch there is invisible until the mp4 does not appear.
E2E_UPLOAD_DIR ?= /tmp/tessera-e2e-uploads

# Conferences (#2876). Left empty by default, which makes the token endpoint
# answer its documented 503 and the media tier of conferences.spec.js skip with
# a printed reason — the room/chat/roster tiers still run. To exercise the media
# path too, start the throwaway SFU and point these at it; both commands are in
# the header of deploy/livekit.e2e.yaml, and they look like:
#   make e2e-backend-up LIVEKIT_URL=http://localhost:7945 \
#     LIVEKIT_PUBLIC_URL=ws://localhost:7945 \
#     LIVEKIT_API_KEY=devkey LIVEKIT_SECRET_FILE=~/.livekit-e2e-secret
# LIVEKIT_PUBLIC_URL is what the *browser* dials, so it has to be an address the
# host can reach (ws:// is fine from http://localhost, which is a secure context).
# The secret goes through a FILE rather than LIVEKIT_API_SECRET= on the command
# line: a make variable lands in the shell history and, worse, in the `ps` line of
# every process this recipe starts. Both are still accepted.
LIVEKIT_URL ?=
LIVEKIT_PUBLIC_URL ?=
LIVEKIT_API_KEY ?=
LIVEKIT_API_SECRET ?=
LIVEKIT_SECRET_FILE ?=

.PHONY: e2e-backend-up
e2e-backend-up: ## Start a throwaway backend on :8092 against tessera_test (for web e2e)
	cd backend && DATABASE_URL="$(E2E_DB_URL)" $(GO) run ./cmd/migrate
	cd backend && $(GO) build -o /tmp/tessera-e2e-bin .
	@LK_SECRET="$(LIVEKIT_API_SECRET)"; LK_FILE="$(LIVEKIT_SECRET_FILE)"; \
	if [ -n "$$LK_FILE" ]; then LK_SECRET=$$(tr -d '\n' < "$$LK_FILE"); fi; \
	PORT=$(E2E_PORT) UPLOAD_DIR=$(E2E_UPLOAD_DIR) JWT_SECRET=e2e \
		DATABASE_URL="$(E2E_DB_URL)" \
		RATE_LIMIT_ENABLED=false \
		LIVEKIT_URL="$(LIVEKIT_URL)" LIVEKIT_PUBLIC_URL="$(LIVEKIT_PUBLIC_URL)" \
		LIVEKIT_API_KEY="$(LIVEKIT_API_KEY)" LIVEKIT_API_SECRET="$$LK_SECRET" \
		nohup /tmp/tessera-e2e-bin > /tmp/tessera-e2e-backend.log 2>&1 & \
		for i in $$(seq 1 40); do sleep 0.5; \
			curl -sf http://localhost:$(E2E_PORT)/api/health > /dev/null && \
			echo "e2e backend up on :$(E2E_PORT) (log: /tmp/tessera-e2e-backend.log)" && exit 0; \
		done; echo "e2e backend did not come up; see /tmp/tessera-e2e-backend.log" >&2; exit 1

.PHONY: e2e-backend-down
e2e-backend-down: ## Stop the throwaway e2e backend
	@fuser -k $(E2E_PORT)/tcp 2>/dev/null || true
	@echo "e2e backend on :$(E2E_PORT) stopped"

# ── Recording stand (#2877) ────────────────────────────────
# Server-side recording is the one part of conferences whose failure modes live
# entirely between containers — Redis dispatch, a headless Chrome joining the
# room, an mp4 written into a directory the backend and egress reach under
# different uids. None of that exists in a unit test, so it gets a stand:
#
#   make recording-e2e-up          # redis + SFU + egress, own project, shifted port
#   make e2e-backend-up LIVEKIT_URL=http://localhost:$(REC_E2E_PORT) \
#     LIVEKIT_PUBLIC_URL=ws://localhost:$(REC_E2E_PORT) \
#     LIVEKIT_API_KEY=$(REC_E2E_KEY) LIVEKIT_SECRET_FILE=~/.livekit-e2e-secret
#   make recording-e2e-publish REC_E2E_ROOM=<room>   # after the room exists
#   make recording-e2e-down
#
# No UPLOAD_DIR override is needed: REC_E2E_UPLOAD_DIR defaults to the backend's
# own E2E_UPLOAD_DIR, and the stand mounts it into egress at the same absolute
# path, so both sides name the finished file identically.
#
# It is deliberately NOT `docker compose --profile recording`: that profile turns
# recording on for the *production* SFU by setting LIVEKIT_REDIS_HOST, i.e. moves
# live conferences into multi-node mode. See deploy/recording.e2e.yml for what
# this stand keeps faithful to production and what it does not.
#
# The secret goes through a file, not a make variable, for the same reason as
# LIVEKIT_SECRET_FILE above: a variable lands in shell history and in the `ps`
# line of every process the recipe starts.
# 7955, not the 7945 of livekit.e2e.yaml: that one belongs to the media tier of
# the web suite, and a box that has run it recently still has a container holding
# it (they also leak — see livekit.e2e.yaml). If 7955 is taken too, move it;
# nothing else depends on the number. Only this one port is published — the SFU's
# RTC ports stay inside the network, where the publisher and egress are.
REC_E2E_PORT ?= 7955
REC_E2E_KEY ?= devkey
REC_E2E_SECRET_FILE ?= $(HOME)/.livekit-e2e-secret
# Deliberately derived from E2E_UPLOAD_DIR rather than repeated: the throwaway
# backend and the egress container must agree on this path exactly, and two
# defaults that merely happen to match would drift the first time one is changed.
REC_E2E_UPLOAD_DIR ?= $(E2E_UPLOAD_DIR)

# The environment both recipes need. Kept in one variable so `up` and `down`
# cannot drift: compose resolves the project's containers from the same
# interpolated file, and a `down` with different values would look at a
# different stack and report success having stopped nothing.
REC_E2E_ENV = REC_E2E_UID=$$(id -u) \
	REC_E2E_KEY=$(REC_E2E_KEY) \
	REC_E2E_SECRET=$$(tr -d '\n' < "$(REC_E2E_SECRET_FILE)") \
	REC_E2E_PORT=$(REC_E2E_PORT) \
	REC_E2E_ROOM=$(REC_E2E_ROOM) \
	REC_E2E_UPLOAD_DIR=$(REC_E2E_UPLOAD_DIR)

.PHONY: recording-e2e-up
recording-e2e-up: ## Start the throwaway recording stand (redis + SFU + egress) for #2877
	@test -f "$(REC_E2E_SECRET_FILE)" || { \
		echo "no $(REC_E2E_SECRET_FILE); create it with:" >&2; \
		echo "  printf devsecret_at_least_32_characters_long > $(REC_E2E_SECRET_FILE)" >&2; \
		exit 1; }
	@grep -qx "port: $(REC_E2E_PORT)" deploy/livekit.recording.e2e.yaml || { \
		echo "REC_E2E_PORT ($(REC_E2E_PORT)) does not match deploy/livekit.recording.e2e.yaml." >&2; \
		echo "It is published one-to-one, so both places have to say the same number." >&2; \
		exit 1; }
	@mkdir -p $(REC_E2E_UPLOAD_DIR)
	cd deploy && $(REC_E2E_ENV) $(COMPOSE) -f recording.e2e.yml up -d
	@echo "recording stand up: SFU on :$(REC_E2E_PORT), recordings in $(REC_E2E_UPLOAD_DIR)/rec"

# The room must already exist — the SFU has auto_create off, so a typo'd name is
# an error from the CLI rather than an empty room that records nothing.
REC_E2E_ROOM ?= rec-smoke

.PHONY: recording-e2e-publish
recording-e2e-publish: ## Publish a demo video track into REC_E2E_ROOM on the recording stand
	cd deploy && $(REC_E2E_ENV) $(COMPOSE) -f recording.e2e.yml --profile publish up -d publisher
	@echo "publishing demo media into room $(REC_E2E_ROOM)"

# --profile publish on both, and it is not decoration: a service behind a profile
# is invisible to `down` and to `logs` unless the profile is named, so without it
# `down` leaves the publisher running, fails to remove the network ("resource is
# still in use"), and the next `up` inherits a container that is still in an old
# room. Observed, not theorised.
.PHONY: recording-e2e-down
recording-e2e-down: ## Stop the throwaway recording stand
	cd deploy && $(REC_E2E_ENV) $(COMPOSE) -f recording.e2e.yml --profile publish down --remove-orphans

.PHONY: recording-e2e-logs
recording-e2e-logs: ## Tail the recording stand's logs
	cd deploy && $(REC_E2E_ENV) $(COMPOSE) -f recording.e2e.yml --profile publish logs -f

# Both knobs are needed and they are NOT the same one: the suite's own API client
# talks to the backend directly (E2E_API_URL), while the preview server proxies
# the browser's /api there (TESSERA_API_TARGET). Deriving both from E2E_PORT keeps
# a non-default `make e2e-backend-up E2E_PORT=…` from leaving the suite pointed at
# :8092 — at somebody else's backend, or at nothing.
# E2E_ARGS is passed through to playwright: `make test-e2e-frontend E2E_ARGS=conferences`
# runs one spec instead of the whole suite.
E2E_ARGS ?=

.PHONY: test-e2e-frontend
test-e2e-frontend: ## Run the Playwright web e2e suite (needs `make e2e-backend-up`)
	cd frontend && corepack yarn build
	cd frontend && E2E_API_URL=http://localhost:$(E2E_PORT)/api \
		TESSERA_API_TARGET=http://localhost:$(E2E_PORT) \
		corepack yarn e2e $(E2E_ARGS)

.PHONY: locale-shots
locale-shots: ## Visual pass over both locales into frontend/e2e/.auth/locale-shots (needs `make e2e-backend-up`)
	cd frontend && corepack yarn build
	cd frontend && E2E_LANG=ru corepack yarn e2e:locale
	cd frontend && E2E_LANG=en corepack yarn e2e:locale

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

# The instrumented smoke tier: needs a connected device or a running emulator,
# and reaches the throwaway backend through the emulator's 10.0.2.2 host alias
# (so `make e2e-backend-up` still runs on the host, exactly as for the JVM tier).
.PHONY: test-android-instrumented
test-android-instrumented: ## Android smoke tier on a device/emulator (needs `make e2e-backend-up`)
	@# The shots package is excluded: it seeds demo-shaped content and writes PNGs,
	@# which is `make android-shots`' job. A smoke tier that depended on it would
	@# start failing over screenshot data.
	@$(ANDROID_GRADLE) :app:connectedDebugAndroidTest \
	  -Pandroid.testInstrumentationRunnerArguments.notPackage=website.msdnna.tessera.shots

# The AVD the instrumented tiers run on. Headless and software-rendered: this
# host has no display, and swiftshader is the renderer that survives it.
ANDROID_AVD ?= tessera_e2e
ANDROID_EMU_LOG := $(ANDROID_DIR)/app/build/emulator.log

.PHONY: android-emulator-up
android-emulator-up: ## Boot the AVD headless and wait for it (needed by android-shots)
	@ANDROID_HOME="$${ANDROID_HOME:-$$HOME/Android/Sdk}"; \
	 ADB="$$ANDROID_HOME/platform-tools/adb"; \
	 if [ "$$($$ADB shell getprop sys.boot_completed 2>/dev/null | tr -d '\r')" = "1" ]; then \
	   echo "emulator: уже поднят"; exit 0; fi; \
	 mkdir -p $(dir $(ANDROID_EMU_LOG)); \
	 nohup "$$ANDROID_HOME/emulator/emulator" -avd $(ANDROID_AVD) -no-window -no-audio \
	   -no-boot-anim -no-snapshot -gpu swiftshader_indirect >$(ANDROID_EMU_LOG) 2>&1 & \
	 echo "emulator: жду загрузку ($(ANDROID_AVD)), лог — $(ANDROID_EMU_LOG)"; \
	 for i in $$(seq 1 120); do \
	   [ "$$($$ADB shell getprop sys.boot_completed 2>/dev/null | tr -d '\r')" = "1" ] && { echo "emulator: готов"; exit 0; }; \
	   sleep 5; \
	 done; \
	 echo "emulator: не поднялся за 10 минут — смотри $(ANDROID_EMU_LOG)" >&2; exit 1

# The mobile counterpart of `make help-shots` (#2795): the help centre's Android
# screenshots, taken on the emulator against the same throwaway backend the e2e
# tiers use.
SHOTS_CLASS := website.msdnna.tessera.shots.HelpShotsTest

# A new scene rarely comes out right the first time, and re-shooting the whole
# manual costs minutes per attempt — `make android-shots SHOTS=tags,markdownEditor`
# runs just those test methods. Empty (the default) means the whole class.
SHOTS_FILTER = $(if $(SHOTS),$(shell echo '$(SHOTS)' | tr ',' '\n' | sed 's|^|$(SHOTS_CLASS)\#|' | paste -sd,),$(SHOTS_CLASS))

.PHONY: android-shots
android-shots: ## Re-take the mobile help screenshots (needs `make e2e-backend-up`; SHOTS=a,b for some)
	@$(MAKE) android-emulator-up
	@# `leaveApksInstalledAfterRun`: by default AGP uninstalls both APKs when the
	@# run ends, and the app's data directory — where the shots are — goes with
	@# them. The run stays green and the fetch below finds nothing.
	@$(ANDROID_GRADLE) :app:connectedDebugAndroidTest \
	  -Pandroid.testInstrumentationRunnerArguments.class=$(SHOTS_FILTER) \
	  -Pandroid.injected.androidTest.leaveApksInstalledAfterRun=true
	@$(MAKE) android-shots-pull

# The English twins of the mobile shots (#2816). The language is the app's own
# profile setting, not the device's, so it is passed to the test as an argument
# and applied through the same `AppLocale` wrapper the app switches with — no
# emulator locale change, no restart.
.PHONY: android-shots-en
android-shots-en: ## Re-take the mobile help screenshots in English (needs `make e2e-backend-up`; SHOTS=a,b)
	@$(MAKE) android-emulator-up
	@$(ANDROID_GRADLE) :app:connectedDebugAndroidTest \
	  -Pandroid.testInstrumentationRunnerArguments.class=$(SHOTS_FILTER) \
	  -Pandroid.testInstrumentationRunnerArguments.shotsLang=en \
	  -Pandroid.injected.androidTest.leaveApksInstalledAfterRun=true
	@$(MAKE) android-shots-pull

# Separate from the run above so a failed fetch can be retried without paying for
# the two-minute instrumented run again — the shots survive on the device.
.PHONY: android-shots-pull
android-shots-pull: ## Fetch the screenshots the last android-shots run left on the device
	@# The shots land in the app's private storage, which the shell user cannot
	@# read on its own since Android 11 — neither in `/data/data/<pkg>` nor in
	@# `/sdcard/Android/data/<pkg>`, where a pull comes back silently empty. The
	@# AVD runs a `google_apis` (userdebug) image, so `adb root` is the way in.
	@#
	@# The fetch stages into `build/` and only then copies into the tree. Pulling
	@# straight into `docs/help/assets` overwrote a good committed shot with a
	@# zero-byte one before the emptiness check could fire: killing the emulator
	@# loses the page cache, and the shots of the interrupted run stay behind as
	@# empty files with their original mtime. Empty shots are now reported (and
	@# fail the target) without touching what is already in the tree.
	@ANDROID_HOME="$${ANDROID_HOME:-$$HOME/Android/Sdk}"; \
	 ADB="$$ANDROID_HOME/platform-tools/adb"; \
	 SRC=/data/data/website.msdnna.tessera/files/help-shots; \
	 echo "android-shots: $$($$ADB root 2>&1 | tr -d '\r')"; $$ADB wait-for-device; \
	 FILES="$$($$ADB shell ls $$SRC 2>&1 | tr -d '\r')"; \
	 case "$$FILES" in *"No such file"*|*"denied"*|"") \
	   echo "android-shots: кадры не читаются ($$SRC): $$FILES" >&2; exit 1;; esac; \
	 STAGE=$(ANDROID_DIR)/app/build/help-shots; rm -rf $$STAGE; mkdir -p $$STAGE; \
	 KEPT=0; EMPTY=""; \
	 for f in $$FILES; do \
	   $$ADB pull "$$SRC/$$f" $$STAGE/$$f >/dev/null || exit 1; \
	   [ -s $$STAGE/$$f ] || { EMPTY="$$EMPTY $$f"; continue; }; \
	   cp $$STAGE/$$f docs/help/assets/$$f || exit 1; KEPT=$$((KEPT+1)); \
	 done; \
	 echo "android-shots: $$KEPT кадров → docs/help/assets"; \
	 [ -z "$$EMPTY" ] || { \
	   echo "android-shots: пустые кадры на устройстве:$$EMPTY" >&2; \
	   echo "android-shots: снимите их заново или уберите остатки —" >&2; \
	   echo "  $$ADB shell rm $$SRC/<имя>" >&2; exit 1; }

.PHONY: android-emulator-down
android-emulator-down: ## Shut the AVD down
	@ANDROID_HOME="$${ANDROID_HOME:-$$HOME/Android/Sdk}"; \
	 "$$ANDROID_HOME/platform-tools/adb" emu kill 2>/dev/null || true

# Compiles the instrumented tier without running it — the only check available on
# a host with no emulator, and worth having: an androidTest source set that never
# builds is not caught by lint or by the unit run.
.PHONY: build-android-instrumented
build-android-instrumented: ## Compile the instrumented smoke tier (no device needed)
	@$(ANDROID_GRADLE) :app:assembleDebugAndroidTest

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
