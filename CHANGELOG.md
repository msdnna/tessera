# Changelog

All notable changes to Tessera are documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/), versions per service.

## frontend

### [0.13.1] — 2026-06-02
- Review fixes: dark theme for the date picker panel (point 1); delete buttons
  are now outlined (ghost) red with trash icon everywhere (point 3); board
  rename via double-click + fixed focus/blur (v-for ref) so editing cancels on
  outside click (point 6); project popover groups Rename + Delete at the bottom
  (point 7); mobile DnD uses delay+delayOnTouchOnly so a tap opens instead of
  dragging on touch (point 8).

### [0.13.0] — 2026-06-02
- Task modal redesigned a reference tracker-style (point 4): title on top, then a compact
  property grid (Приоритет / Срок / Исполнители / Теги / Выполнено) with inline
  pill controls (popovers, immediate apply) instead of stacked inputs, then
  Описание and Подзадачи; footer Delete (red) / Отмена / Сохранить.

### [0.12.2] — 2026-06-02
- Card pills polish (point 3): due-date pill opens the calendar directly; tags
  render as independent chips with a chip-toggle multi-select picker (no
  checkboxes); priority now tints the card's left border; assignee picker shows
  avatar + name with a checkmark; the completed circle is larger.

### [0.12.1] — 2026-06-02
- Sidebar review fixes: subtree indentation via nested containers so the drag
  placeholder shows whether an item lands inside a group; empty groups are
  droppable (min-height) — fixes projects not entering nested groups (point 1).
- Project/group "⋯" is now a popover with a Переименовать button and a red
  trash Delete; board rows get a "⋯" rename/delete menu; project initials no
  longer wrap (point 2).
- Board: column-create input focuses and closes on blur (point 5); column
  delete is a red trash button (point 6); "+ СОЗДАТЬ ЗАДАЧУ / КОЛОНКУ" buttons
  unified — uppercase, centered (point 7).

### [0.12.0] — 2026-06-02
- Inline card editing (point 1): task cards are now uniform — every card shows
  priority / tags / due-date / assignee pills (faint placeholders when unset),
  each editable inline via its own popover (immediate apply); completed toggled
  via the leading circle; clicking the title opens the modal for heavy fields.
  Tags can be created on the fly from the card too.

### [0.11.2] — 2026-06-02
- Tag management (point 8): backend UpdateTag (PATCH /tags/:id); TagManager is
  now a popover (from the board "Теги" button) where clicking a tag edits its
  name (inline) and color (immediate) or deletes it.

### [0.11.1] — 2026-06-02
- Sidebar refinements: project icons from an ionicons5 picker (no emoji, point 5);
  inline rename now focuses and saves-on-blur-if-changed / cancels-if-unchanged
  (points 3); project/group settings via immediate-apply popovers (point 4);
  create via "+" dropdowns on each node and header (point 6) — group "+" =
  Проект/Группа, project "+" = board, removed the "+ доска" text; short menu
  labels (point 7); drag drop-placeholder highlight (.sb-ghost, point 2).

### [0.11.0] — 2026-06-02
- Create tags on the fly (Phase 10d): the task modal's tag select is now a tag
  input — typing a new name creates a workspace tag (random palette color) and
  attaches it. Editing/deleting tags stays in the TagManager.

### [0.10.0] — 2026-06-02
- Inline board editing (Phase 10c): "+ задача" opens an inline editable card at
  the column bottom (Enter creates, stays open for rapid entry) instead of a
  modal; "+ Колонка" inline input to the right of the columns.
- ColumnHeader component: inline rename (double-click) + settings popover
  (color swatches + delete) instead of a modal (point 6).
- Removed the create/column-settings modals.

### [0.9.2] — 2026-06-02
- Task modal (Phase 10e): tappable fields (priority, due date, completed) now
  persist immediately and reflect on the board card; the Save button is only
  for the text fields (title/description).

### [0.9.1] — 2026-06-02
- Sidebar drag & drop (Phase 10b-2): projects between groups/root and groups
  reorder/re-parent via vuedraggable; persisted with projects.move /
  groups.move (midpoint position). Shared useSidebarDnd handlers.

### [0.9.0] — 2026-06-02
- Sidebar tree overhaul (Phase 10b): recursive nested groups (folder icon),
  projects with icon/initials square + color, boards with tile icon and tree
  indentation. Inline rename (groups). Per-node "⋯" menus (new project / new
  subgroup / rename / delete) and a "+" popover on the Проекты header (new
  project / group at root). Project settings popover (name/icon/color/delete).
  Replaced bottom create buttons and the flat list.

### [0.8.2] — 2026-06-02
- Deployment (Phase 9): multi-stage Dockerfile (node build → nginx) serving the
  SPA and proxying /api + WebSocket to the backend; .dockerignore. Added to dev
  and prod compose.

### [0.8.1] — 2026-06-02
- Tests (Phase 7): Vitest + activity-store spec (event labels, assigned-to-you,
  noise filtering, cap/markRead). GitHub Actions CI (backend build/vet/test,
  frontend lint/format/test/build). Makefile lint/test aggregate targets.

### [0.8.0] — 2026-06-02
- Collaboration (Phase 6): MembersModal — list workspace members with roles,
  invite by email (member/admin), remove; opened from a topbar people icon.
- Activity bell: in-memory feed built from workspace-scoped WebSocket events
  (task created/updated/moved/assigned, board/project/note created) with an
  unread badge; "assigned to you" highlighted. Persistent notifications deferred.

### [0.7.1] — 2026-06-02
- Replace unicode emoji/symbols with themed ionicons5 icons (sidebar nav,
  hamburger, theme switch sun/moon, task due-date, column grip & menu, deletes).

### [0.7.0] — 2026-06-02
- Notes module (Phase 5b): two-pane NotesView (list + editor), create/edit/delete,
  workspace-scoped; sidebar nav link.
- Reminders (Phase 5c): RemindersView — create (message + datetime), mark done,
  delete, overdue highlight; sidebar nav link.

### [0.6.0] — 2026-06-02
- Column customization (Phase 5a): per-column settings modal (rename, accent
  color swatch, delete with confirm) and drag-to-reorder columns (status mode,
  grip handle) — persisted via column update/move/delete API.

### [0.5.0] — 2026-06-02
- Tag grouping (Phase 4c, killer feature): toolbar toggle "Статусы / Теги".
  In tag mode columns = workspace tags (+ "Без тегов"); dragging a card between
  tag columns adds/removes the tag.
- Filters (priority, assignee, title search) + sorting (manual / priority / due)
  applied before grouping.
- Tag manager modal: create (name + color swatch) and delete workspace tags.
- Task edits and tag changes trigger a full debounced board reload.

### [0.4.0] — 2026-06-02
- Task modal (Phase 4b): full editing — title, description, priority (colored),
  due date, completed toggle, tags & assignees (multi-select, applied
  immediately), subtasks (add + toggle), delete with confirm.
- Opens on card click inside the board; saves reload the board (suppressed).

### [0.3.0] — 2026-06-02
- Kanban board (Phase 4a): drag & drop tasks within/across columns via
  vuedraggable; server recomputes position from before/after neighbours.
- Task cards show priority dot, tag chips, due date, assignee initials.
- Live updates: WebSocket subscription reloads the board on workspace-scoped
  events (debounced, suppressed during local drags/actions).
- Column accent stripe + task counts; inline create column/task.

### [0.2.1] — 2026-06-02
- Fix: board columns stretched full-width (one column filled the row). Columns
  are now fixed 280px flex items so they sit side-by-side with horizontal scroll.

### [0.2.0] — 2026-06-02
- Theme system ported from budget-go: 7 accent color schemes + light/dark,
  WCAG-luminance text-on-primary, full Naive UI `themeOverrides`, CSS custom
  properties (`--t-*`) so plain components follow the theme.
- Appearance popover in the topbar (color swatches + dark switch).
- Adaptive layout: desktop fixed sider; ≤768px collapses to a drawer opened by
  a hamburger (`useResponsive` matchMedia composable).
- a reference tracker-style polish: sidebar brand ("mt" monogram), active-board highlight,
  column accent stripe (column color), priority dots on task cards.

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

### [0.6.1] — 2026-06-02
- UpdateTag (PATCH /tags/:id) — edit tag name/color (membership-authorized).

### [0.6.0] — 2026-06-02
- Nested project groups: `project_groups.parent_id` (self-ref, cascade) — groups
  can contain subgroups (migration 0003). MoveProjectGroup (re-parent + reorder)
  with self-parent guard.
- Project icons: `projects.icon` column; create/update accept it.
- MoveProject (re-group + reorder) and MoveProjectGroup endpoints compute
  midpoint position from before/after neighbours.

### [0.5.2] — 2026-06-02
- Deployment (Phase 9): distroless `Dockerfile.prod` (static binary + embedded
  /migrate), `docker-compose.prod.yml` (no exposed Postgres, APP_ENV=production,
  required secrets). Dev compose backend remapped to host :8090.

### [0.5.1] — 2026-06-02
- Tests (Phase 7): internal/auth (token round-trip, wrong-secret, refresh
  hashing, bcrypt) and handlers positionBetween (incl. strictly-between).

### [0.5.0] — 2026-06-02
- Notes CRUD (workspace-scoped, membership-authorized) — POST/GET
  /workspaces/:id/notes, GET/PATCH/DELETE /notes/:id.
- Reminders CRUD (personal, owner-authorized) — POST/GET /reminders,
  PATCH/DELETE /reminders/:id.

### [0.4.0] — 2026-06-02
- `GET /boards/:id/tasks` now returns each task with aggregated `tag_ids` and
  `assignee_ids` (ListBoardTasksWithMeta) so the kanban renders chips and groups
  by tag without per-card round-trips.

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
