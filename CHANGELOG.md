# Changelog

All notable changes to Tessera are documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/), versions per service.

## frontend

### [0.1.0] — 2026-06-02
- Vue 3 + Vite 8 + Naive UI + Pinia + Vue Router skeleton (Yarn 4).
- Auth flow: login/register views, JWT stored in localStorage, axios refresh-on-401
  (coalesced), route guard, `auth:expired` → /login.
- App shell (a reference tracker-style): sidebar with workspace switcher + groups/projects tree
  (lazy-loaded boards), topbar with theme toggle + logout, NConfigProvider (ruRU,
  light/dark baseline).
- Board view: columns + tasks read-only skeleton with minimal create modals
  (full drag & drop kanban lands in Phase 4).

## backend

### [0.3.0] — 2026-06-02
- CRUD for the full hierarchy: workspaces (+ membership/invite by email),
  project groups, projects, boards, columns, tasks (+ subtasks), tags.
- Drag & drop ordering: server-computed float midpoint positions; `PATCH
  /tasks/:id/move` and `/columns/:id/move` take before_id/after_id.
- Task tags & assignees (M:N); task detail bundles tags/assignees/subtasks.
- Workspace-membership authorization on every nested resource (scope resolvers).
- New users get an auto-created personal workspace; domain events broadcast to
  the WebSocket hub (workspace-scoped).

### [0.2.0] — 2026-06-02
- Full domain schema migration (workspaces, project groups, projects, boards,
  columns, tasks + subtasks, tags, task_tags, assignees, notes, reminders);
  float8 positions for ordering.
- sqlc pipeline (`pgx/v5`, google/uuid codec registered on each connection).
- Auth: register/login/refresh/me with JWT access tokens (15 min) + rotating
  opaque refresh tokens (SHA-256 stored, revoked on use); bcrypt passwords;
  first registered user becomes admin. `middleware.Auth` Bearer guard.

### [0.1.0] — 2026-06-02
- Phase 0 scaffold: gin server with `/api/health`, `/api/version`, `/api/ws`.
- PostgreSQL connection pool (pgx/v5) with startup retry.
- WebSocket fan-out hub (`internal/realtime`) for live board updates.
- golang-migrate setup with embedded SQL migrations + `cmd/migrate` CLI.
- CORS middleware, config with fail-closed prod gates, Docker build.
