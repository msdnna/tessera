<p align="right"><a href="README.md">Русский</a> · <b>English</b></p>

<p align="center">
  <img src="design/tessera-brand-v2/svg/logo-horizontal-on-purple.svg" alt="Tessera" width="360" />
</p>

<p align="center">
  <b>A self-hosted task tracker</b> — deploy it yourself and own your data.
</p>

<p align="center">
  <a href="https://github.com/msdnna/tessera/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/msdnna/tessera/actions/workflows/ci.yml/badge.svg?branch=main" /></a>
  <img alt="backend coverage" src="https://img.shields.io/endpoint?url=https%3A%2F%2Fraw.githubusercontent.com%2Fmsdnna%2Ftessera%2Fbadges%2Fbackend.json&label=backend" />
  <img alt="web coverage" src="https://img.shields.io/endpoint?url=https%3A%2F%2Fraw.githubusercontent.com%2Fmsdnna%2Ftessera%2Fbadges%2Fweb.json&label=web" />
  <img alt="android coverage" src="https://img.shields.io/endpoint?url=https%3A%2F%2Fraw.githubusercontent.com%2Fmsdnna%2Ftessera%2Fbadges%2Fandroid.json&label=android" />
  <img alt="mcp coverage" src="https://img.shields.io/endpoint?url=https%3A%2F%2Fraw.githubusercontent.com%2Fmsdnna%2Ftessera%2Fbadges%2Fmcp.json&label=mcp" />
  <img alt="license" src="https://img.shields.io/badge/license-Apache--2.0-blue" />
</p>

**Tessera** is a task tracker you run on your own server: tasks, notes and
reminders under full control, without cloud subscriptions and without handing data
to third parties. A single `docker compose up` brings up the backend, database and
web UI; native clients cover mobile and desktop.

## Features

- **Kanban boards** with columns, drag-and-drop, priorities, due dates, estimates
  and a Done column; a unique **group-cards-by-tags** mode (columns = tags).
- **Project hierarchy**: Workspace → project groups (nested) → projects → boards →
  tasks with a subtask tree, tags and assignees (M:N).
- **Extra board views**: list, calendar, timeline, Gantt chart (with
  dependencies) and the Eisenhower matrix.
- **Rich tasks**: Markdown descriptions and comments, attachments, change log,
  dependencies, recurring tasks, milestones, time estimates.
- **Collaboration**: workspaces with roles, email invites, realtime board updates
  over WebSocket, activity toasts.
- **Notifications**: configurable channels and routing rules (email / webhook /
  Telegram), schedules and quiet hours; push — on Android.
- **GitLab Issues integration** (self-hosted): two-way sync of tasks, labels,
  assignees and comments, conflict resolution, a sync journal.
- **Notes, documents and personal reminders**: quick notes, collaborative
  Markdown pages (the "Documents" module) and scheduled reminders.
- **MCP server** — Tessera hands tasks to AI agents as a priority-ranked queue and
  accepts their results, so the project can be driven from agentic tooling (this
  very repository is developed that way).
- **Built-in help center**: a "Help" section with categories, search and offline
  access — articles ship with the app (docs-as-code, `/help`).
- **Internationalization**: Russian and English UI with language auto-detection on
  the auth screens; the chosen language is stored in the profile, and the help
  center is bilingual too.
- **Theming**: light/dark themes, 7 accent schemes, desktop/mobile responsive.

## Clients

| Client  | Technology                              |
|---------|-----------------------------------------|
| Web     | Vue 3 + Vite + Naive UI + Pinia         |
| Android | Kotlin + Jetpack Compose (online-first) |
| Desktop | Tauri v2 (Windows / Linux)              |

## Stack

- **Backend** — Go + gin, PostgreSQL 17 (JSONB), `pgx/v5` + `sqlc` +
  `golang-migrate`; realtime — a WebSocket fan-out hub; JWT authentication.
- **Frontend** — a Vue 3 SPA (Vite, Naive UI, Pinia).
- **Mobile / Desktop** — Android (Compose) and a Tauri wrapper around the same web client.
- **MCP** — a separate Go server (Model Context Protocol) for agentic development.
- **Observability** — optional integration with a self-hosted Sentry (backend and
  frontend error tracking); enabled by setting a DSN, off by default.

## Deployment (self-hosted)

You only need Docker and a domain name.

```bash
git clone https://github.com/msdnna/tessera.git && cd tessera
cp deploy/.env.example deploy/.env      # set DOMAIN, JWT_SECRET, ENCRYPTION_KEY, POSTGRES_PASSWORD
docker compose -f deploy/docker-compose.yml up -d
```

Caddy issues a TLS certificate automatically (Let's Encrypt) and proxies to the
frontend nginx; PostgreSQL is not exposed to the outside; the backend is a
distroless image with fail-closed secret checks. Details and upgrades — in
[`deploy/README.md`](deploy/README.en.md). The first registered user becomes the
administrator.

## Development and contributing

How to bring up the dev environment, the toolchain, conventions and the PR process —
in [**CONTRIBUTING.md**](CONTRIBUTING.en.md).

## License

[Apache License 2.0](LICENSE) — see also [`NOTICE`](NOTICE).
