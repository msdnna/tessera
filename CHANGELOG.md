# Changelog

All notable changes to Tessera are documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/), versions per service.

## frontend

### [0.23.1] — 2026-06-03
- Fix notification badge shape — it's a clean circle for a single digit again
  (equal min-width/height + pill radius) instead of an oval.

### [0.23.0] — 2026-06-03
- Sidebar tools (notifications, members, appearance) moved to the top of the
  sidebar, right of the logo; the user block stays at the bottom.
- Appearance trigger is now a palette icon instead of a coloured dot.
- Smaller notification count badge (no longer rivals the bell icon).
- Collapsible sidebar (budget-style) with a toggle bar; state persists in
  localStorage. Collapsed = a 60px icon rail (logo, icon-only nav with
  tooltips, avatar); the tools slide into the header's right side. The kanban
  columns re-flow to the reclaimed width automatically.

### [0.22.1] — 2026-06-03
- Search hint updated to note that task descriptions are searched too.

### [0.22.0] — 2026-06-03
- Home / My work (feature #1): the landing page is now a dashboard — summary
  cards (my tasks, all active, overdue, due today, due this week, completed)
  that double as filters, over a cross-board task list (number, priority, title,
  tags, project/board, column, due, assignees). Rows open the task on its board.
  "Главная" added to the sidebar nav.

### [0.21.0] — 2026-06-03
- Advanced task modal (feature #8): the description is now Markdown — rendered
  inline, click to edit, blur to save (sanitised via DOMPurify). A tabbed area
  adds Comments (Markdown, edit/delete your own), Relations (link other tasks by
  #N with relation kind; click to jump to the related task), Files (upload /
  download / delete attachments) and History (the task's activity journal with
  actor and time). Adds `marked` + `dompurify`.

### [0.20.0] — 2026-06-03
- Persistent notifications (feature #3): the bell is now backed by the server —
  it loads on startup, receives new notifications live over the socket, shows a
  real unread count, marks individual or all as read, and each item is
  clickable (opens the related task's board + modal via #N). Replaces the old
  in-memory activity feed.

### [0.19.2] — 2026-06-03
- Kanban columns now reliably fit the screen: columns use `box-sizing:
  border-box` (their 10px padding no longer added ~20px each to the measured
  width, which had pushed the "+ колонка" tile off-screen). The minimum column
  width before horizontal scrolling kicks in dropped to 220px, so the default
  set of columns plus the reserved add-column tile fill the viewport, and
  adding a column or two stays on screen — only a genuinely cramped count
  scrolls.
- Mobile: a column is now slightly under full width (next one peeks) with
  CSS scroll-snap, giving a smooth page-turn swipe between columns.

### [0.19.1] — 2026-06-03
- Fix kanban column width overflowing the screen (desktop and mobile): width is
  now derived from the measured scroll container (ResizeObserver) rather than an
  estimated viewport calc, with slack so columns always fit; mobile shows one
  column just under full width.
- Custom themed scrollbars across the app (page + all scroll areas), adapting to
  the active light/dark theme.

### [0.19.0] — 2026-06-03
- Layout overhaul (feature 7): the top header is now reserved for a single,
  centred search bar (not full width). The user + notifications block moved to
  the bottom of the sidebar — avatar + name + logout on desktop, avatar with a
  popover (name, email, logout) on mobile — alongside members, the activity
  bell and the appearance picker. Kanban columns now have adaptive width: they
  expand to fill the viewport (leaving room for "＋ Создать колонку"), and on
  mobile exactly one full-width column shows at a time (swipe between them).
- Global search (feature 2): the header search queries tasks and notes as you
  type (debounced) and shows a grouped results dropdown; picking a task opens
  its board with the task modal, picking a note opens it in Notes.
- Advanced filters + saved views (feature 6): a single "Вид и фильтры" dropdown
  now holds grouping (statuses/tags), sort, and all filters — including new
  filters by tag and by due date (overdue / today / this week / has / none) on
  top of priority, assignee and text. The whole view configuration is saved per
  board (per device) and restored on return; "Сбросить" clears active filters.

### [0.18.2] — 2026-06-03
- Task modal: tighter assignee avatar cascade with the ring matching the modal
  background, so the overlap reads cleanly on the elevated surface.

### [0.18.1] — 2026-06-03
- Assignee avatars now cascade (overlap with a ring in the surface colour) like
  a reference tracker, instead of sitting side by side.

### [0.18.0] — 2026-06-03
- Show the task number (#N) on cards and in the task modal header.

### [0.17.1] — 2026-06-02
- Archive modal: Restore / Delete are now labelled ghost buttons (icon + text).

### [0.17.0] — 2026-06-02
- Transfer tasks (feature 3): click the location breadcrumb in the task modal to
  pick a project → board and move the task there (subtasks follow).
- Archive (feature 4): the modal's primary destructive action is now "В архив"
  (with the subtasks cascade/detach choice); a board-toolbar "Архив" button opens
  a list of archived tasks to restore or delete permanently.

### [0.16.0] — 2026-06-02
- Fix: creating a subtask/board with Enter no longer duplicates (clear+close
  before the await so the @blur doesn't re-submit) (issue 1).
- Subtask reorder via drag (hold ~0.3s) within a card (issue 4).
- Modal subtasks rendered as functional rows (done toggle, priority dot, due)
  with click-to-open instead of plain checkboxes (issue 3).
- Reattach: modal "Родитель" row picks a parent task to become its subtask;
  "Открепить" detaches (issue 2). (Drop-card-onto-card nesting is a follow-up.)

### [0.15.0] — 2026-06-02
- Subtasks on the board (Phase: feature 2): parent cards render their subtasks
  as compact sub-rows (done toggle, priority dot, due) — clicking a sub-row
  opens its own modal; a hover "＋ Создать подзадачу" button creates one inline.
- Task modal: "Открепить от родителя" for subtasks; deleting a task with
  subtasks asks whether to delete them too or detach them.
- (Subtask drag & drop / reattach-by-drag is a follow-up.)

### [0.14.0] — 2026-06-02
- Inline title editing on task cards: click the title to edit it in place
  (save on blur/Enter); clicking the card body opens the modal.

### [0.13.10] — 2026-06-02
- Fix: changing a tag's color in the TagManager did nothing — clicking a swatch
  blurred the name input, which closed the editor (removing the swatch) before
  the click registered. Swatches now use @mousedown.prevent to keep focus.

### [0.13.9] — 2026-06-02
- Modal plain inputs use a transparent background (not the surface token, which
  was lighter than the modal) so they blend into the modal in any theme.

### [0.13.8] — 2026-06-02
- Tag stack: shadow layers shrink with depth and margin-right scales with the
  number of layers so the next pill always clears the stack; tag border paler.
- Fixed modal plain inputs in dark theme: the `plain` class is the NInput root,
  so `:deep(.n-input)` never matched (modal is teleported) — set --n-color on
  `.plain` directly, so the field keeps the modal colour on focus too.

### [0.13.7] — 2026-06-02
- Stacked tag pill: add margin-right so the next pill clears the box-shadow
  stack; tag border color is now paler than the text but stronger than the
  tint background.

### [0.13.6] — 2026-06-02
- Card tag: the pill (button) carries the tint background + tag-color border;
  the inner span is just text — matching the other card pills (point 1).
- Modal plain inputs: override Naive's inline `--n-color` (with !important) to
  the modal background so the field really blends in, dark theme included (2).

### [0.13.5] — 2026-06-02
- Tags on cards now render as pill-shaped elements (border = tag color) like the
  other card pills, stacked with a right shadow + "+N" (point 1).
- Modal plain inputs: transparent background on inner textarea/input elements
  too, matching the modal background (point 2).
- Project icon grid re-centered (point 3).

### [0.13.4] — 2026-06-02
- Tag stack shadow now offsets only to the right (point 1).
- Plain modal inputs: force transparent background and hide border elements
  (Naive sets them via inline CSS vars) so they read as plain text in dark
  theme too (point 2).
- Project icon grid left-aligned while the popover stays centered (point 3).

### [0.13.3] — 2026-06-02
- Card tag stack reworked: the first tag is a normal colored pill, with offset
  colored shadows behind it (stacked-cards look) + "+N" when there are more; a
  single tag is just a plain colored pill (point 1).
- Modal plain inputs now fully borderless/transparent (point 2).
- Project settings popover centered, action buttons stretch to fit (point 3).
- Board action menu is a popover with ghost icon buttons like elsewhere (4).
- Spacing added to the tag editor (color picker / delete) (point 5).

### [0.13.2] — 2026-06-02
- Modal: title/description/subtask inputs styled as plain editable text;
  location breadcrumb (group chain → project → board) added to the property
  grid (point 2).
- TagManager: double-click a tag to rename it in place; blur saves/closes
  (point 4).
- Task card: when a task has >1 tag they stack (colored squares + first name +
  "+N"); hovering previews all tags, clicking opens the tag picker (point 5).

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

### [0.13.0] — 2026-06-03
- Notifications now also fire when a task is changed or moved by someone else:
  its assignees and creator (minus the actor) are notified (`updated` / `moved`
  kinds, with a short summary of what changed). Previously only assignment and
  comments generated notifications.
- Live-pushed notifications now carry `task_board_id` + `task_number`, so a
  freshly arrived notification is clickable immediately (no reload needed).
- Global search now matches task descriptions too, not just titles.

### [0.12.1] — 2026-06-03
- `GET /tasks/:id/events` now returns the entry `data` as raw JSON instead of a
  base64 string (the generated row carries it as `[]byte`).

### [0.12.0] — 2026-06-03
- Migration 0006: task activity journal (`task_events`), comments
  (`task_comments`), relations (`task_relations`, referenced by #N),
  attachments (`task_attachments`) and per-user persistent notifications
  (`notifications`).
- Rich task endpoints (#8): `GET /tasks/:id/events` (journal);
  comments `GET/POST /tasks/:id/comments`, `PATCH/DELETE /comments/:id`
  (author-only); relations `GET/POST /tasks/:id/relations` (link by #N, with
  404 on unknown number), `DELETE /relations/:id`; attachments
  `GET/POST /tasks/:id/attachments` (multipart, 25 MiB cap, on-disk under
  `UPLOAD_DIR`), `GET /attachments/:id/download`, `DELETE /attachments/:id`.
- Task mutations now write journal entries (created / renamed / description /
  priority / due / completed / reopened / moved / assigned / unassigned /
  archived / restored / comment / relation / attachment).
- Persistent notifications (#3): assigning a task notifies the assignee;
  commenting notifies the task's assignees and creator. `GET /notifications`,
  `GET /notifications/unread-count`, `POST /notifications/:id/read`,
  `POST /notifications/read-all`; new notifications are also pushed live over
  the workspace socket.
- Workspace task aggregation (#1): `GET /workspaces/:id/tasks` (all top-level
  active tasks across boards with location names + tag/assignee ids,
  `?assignee=me` for "My tasks") and `GET /workspaces/:id/summary` (counts:
  total / active / completed / assigned-to-me / overdue / due today / due this
  week / unassigned).
- `UPLOAD_DIR` config (default `./uploads`) for attachment storage.

### [0.11.0] — 2026-06-03
- Global search (feature 2): `GET /workspaces/:id/search?q=` returns matching
  tasks (by title, with board id + number, archived excluded) and notes (title
  or body), case-insensitive, capped at 25 each. Empty query short-circuits.

### [0.10.0] — 2026-06-03
- Per-workspace sequential task numbers (#N): migration 0005 adds
  `workspaces.task_counter` + `tasks.number` (backfilled by creation order);
  CreateTask assigns the next number atomically. Used by cards, notifications,
  task relations.

### [0.9.1] — 2026-06-02
- Fix: archived subtasks now appear in the board archive (and can be restored) —
  the archive list shows individually-archived subtasks too, hiding only
  children archived together with their parent. (Query change, no migration.)

### [0.9.0] — 2026-06-02
- Transfer tasks between boards/projects: `PATCH /tasks/:id/transfer`
  {board_id, column_id?} (same workspace) — becomes top-level on the target
  board; subtasks follow (board/column updated).
- Task archive (soft delete, migration 0004 `archived_at`): board lists exclude
  archived; `PATCH /tasks/:id/archive` (?subtasks=detach keeps them on the
  board), `PATCH /tasks/:id/restore`, `GET /boards/:id/archive`.

### [0.8.0] — 2026-06-02
- Subtasks on the board: `GET /boards/:id/subtasks` returns subtasks with meta
  (tag/assignee ids) for nesting under parent cards.
- `PATCH /tasks/:id/parent` attaches a task to a parent (inheriting its
  board/column) or detaches it (parent_id null → back as a top-level card);
  cycle guards.
- `DELETE /tasks/:id?subtasks=detach` re-parents children to null before
  deleting (default still cascades).

### [0.7.0] — 2026-06-02
- New boards are seeded with default status columns: К работе (grey), В процессе
  (blue), На рассмотрении (purple), Готово (green).
- Moving a task into the "Готово" column auto-marks it completed.

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
