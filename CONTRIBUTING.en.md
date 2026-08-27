<p align="right"><a href="CONTRIBUTING.md">Русский</a> · <b>English</b></p>

# Contributing to Tessera

Thanks for your interest in Tessera. Below is how to bring up the environment and
which conventions the project follows. A deeper architecture guide lives in
`CLAUDE.md` and `ROADMAP.md` (Russian only — internal docs).

## Structure

A monorepo of independently versioned components (each has its own `VERSION`):

| Directory   | Component                                                    |
|-------------|-------------------------------------------------------------|
| `backend/`  | Go 1.25 + gin + PostgreSQL 17 (sqlc, pgx/v5, golang-migrate) |
| `frontend/` | Vue 3 + Vite + Naive UI + Pinia (Yarn 4 via Corepack)       |
| `android/`  | Kotlin + Jetpack Compose (online-first)                     |
| `desktop/`  | Tauri v2 (a Rust shell around the same Vue frontend)        |
| `mcp/`      | Go — the MCP server for agents                               |

## Requirements

- **Go 1.25.x** (in the Makefile — the `GO` variable, `go1.25.9` by default; `go` in CI).
- **Node 22+** and **Corepack** (Yarn 4 — only `corepack yarn ...`, not plain `yarn`).
- **Docker** + Docker Compose (Postgres for dev/tests).
- **JDK 21** + Android SDK (compileSdk 37) — for `android/`.
- **Rust stable** + Tauri system deps — for `desktop/`
  (`libwebkit2gtk-4.1-dev libgtk-3-dev libayatana-appindicator3-dev librsvg2-dev patchelf`).

## Quick start

```bash
cp .env.example .env          # fill in JWT_SECRET etc.
make dev                      # Postgres in Docker + backend on :8090
cd frontend && corepack yarn install && corepack yarn dev   # Vite :5174

# tests and linters
make lint                     # backend + frontend + mcp (also lint-android / lint-desktop)
make test                     # backend + frontend + mcp
make test-backend-cover       # backend coverage (needs the tessera_test DB)
make coverage-report          # combined HTML coverage report across components
```

The DB for e2e/integration tests is **`tessera_test` only**; don't touch the live
`tessera` (TRUNCATE/DROP on it is forbidden).

## Conventions

- **Fresh stable dependencies**, no legacy workarounds.
- **Conventional Commits** (`feat:`, `fix:`, `docs:`, `chore:`, `test:`, `refactor:`, …).
- **Per-component versioning**: each meaningful change to a component bumps its
  `VERSION` (semver) and adds an entry to `CHANGELOG.md` (root; `## backend` /
  `## frontend` sections; Android — `android/CHANGELOG.md`). `docs:`/`chore:`/`refactor:`
  without behavior changes — **no** bump.
- **Pre-commit quality gate**: `make lint-<comp>` + `make test-<comp>` green before
  committing. Fix linter issues, don't silence them.
- **Backend**: SQL via sqlc (`db/queries/*.sql` → `~/go/bin/sqlc generate`, don't
  hand-edit `internal/db/`); migrations — `migrations/NNNN_name.{up,down}.sql`.
- **Frontend**: theme/icons — Naive UI + ionicons5; the description editor is the
  custom `MarkdownEditor.vue` (stores Markdown), not TipTap.
- **Integrations/notifiers** are kept loosely coupled to the core (provider-neutral seams).

## Documentation and localization

Repository documentation is **bilingual**, but there's a single canon — **Russian**.

- **Base — Russian:** the canonical file is always `<name>.md` in Russian (that's
  what GitHub renders as the repo/directory homepage).
- **English — companion:** `<name>.en.md` next to it, a word-for-word translation of
  the base. The `.ru.md` suffix is **not used** — Russian is the base.
- **Language switcher** — a short nav line at the very top of each file in the pair:
  `**Русский** · [English](<name>.en.md)` in the base and `[Русский](<name>.md) · **English**`
  in the `.en.md`.
- **Only "front-door" docs are bilingual** (README, CONTRIBUTING and component
  READMEs: `deploy/`, `mcp/`, `desktop/`, `android/ARCHITECTURE.md`). Internal docs
  (`CLAUDE.md`, `ROADMAP.md`, `CHANGELOG.md`, ADRs, `.claude/skills/`, `changelog.d/`)
  are kept **Russian-only** — translating them bloats volume without benefit. When you
  edit a front-door doc, sync both versions in the same change.
- **The help center** (`docs/help/`) follows the same scheme — Russian `<slug>.md` +
  English `<slug>.en.md` (+ platform `.android.md`), with the index rebuilt via
  `make help-index`. The decision is recorded in
  [`docs/adr/0001-help-center.md`](docs/adr/0001-help-center.md) and
  [`docs/adr/0002-doc-localization.md`](docs/adr/0002-doc-localization.md).

## MCP and observability

- **MCP server** (`mcp/`) — a thin REST client of Tessera for agentic development;
  part of `make lint` / `make test`. Locally: build via `make build-mcp`, mint a
  token via `go run ./cmd/token`. Details — in [`mcp/README.md`](mcp/README.en.md).
- **Sentry** — optional self-hosted error tracking (off without a DSN). In dev we
  read errors from logs; with Sentry up — through it (see the `tessera-sentry` skill).

## Pull requests

1. Fork the repository and branch from `develop`.
2. Make sure `make lint` and `make test` are green (CI runs the same + coverage).
3. Describe what changed and why; attach screenshots for UI changes.
4. One PR — one logical change.

## License

By contributing, you agree that your contribution is licensed under the
[Apache License 2.0](LICENSE), under which the project is distributed.
