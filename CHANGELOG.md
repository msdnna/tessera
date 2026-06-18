# Changelog

All notable changes to Tessera are documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/), versions per service.

## frontend

### [0.72.0] — 2026-06-18
- **Human-readable board URLs**: `/board/<slug>?task=<number>` instead of UUIDs
  (e.g. `/board/obshchie-zadachi?task=252`). Board links resolve a slug or a legacy
  UUID and the URL is canonicalized to the slug; the open task is reflected as its
  number; notes open as `?note=<slug>`. Search results link by slug/number.
- **Tags are per-project**: the board tag picker and the Теги manager create/list
  tags in the board's project (`/projects/:id/tags`); Home still shows all
  workspace tags for its cross-project list.
- Fix: setting a due date defaults the time to 00:00 (hidden on the card) instead
  of the current wall-clock time — pick a time only when you need one.
- Fix: a kanban card with subtasks no longer overlaps the global search dropdown.
- Fix: closing a task opened via a link now clears `?task=` from the URL (a refresh
  no longer reopens it); the open task/note is reflected in the URL.
- Fix: avatars (not initials) now render in the assignee picker, task history,
  the Home recent-tasks list, and the board list view.

### [0.71.0] — 2026-06-17
- Browser **Back** now closes an open overlay instead of leaving the page: the
  task modal, the members/GitLab modals, the board archive modal, the mobile
  sidebar drawer, and an open note all close on Back (new `useOverlayBack`
  composable pushes a throwaway history entry while open and unwinds it on UI
  close, so Back on the page underneath keeps working).
- Collapsed subtask rows on a card now show a pointer cursor (not a text I-beam) —
  they open the modal on click and were never inline-editable.
- Mobile composer «+» menu: drill-in rows show a right-aligned «›» arrow again,
  the «‹ Назад» header has a roomier gap, and changing level now slides
  (down when drilling in, up when going back).

### [0.70.5] — 2026-06-17
- Fix: GitLab task due dates (date-only) no longer show a +3h-shifted time — a pure
  UTC-midnight due is rendered as its calendar date in UTC with no time, so a GitLab
  «26 июн.» issue date stays «26 июн.» instead of «26 июн., 03:00».

### [0.70.4] — 2026-06-17
- Fix: the composer «+» menu's filter sub-lists now open on tap (the mobile
  drill-down's reopen no longer fought the select-triggered close).
- Context-menu delete actions are now fully red — the **icon** too, not just the label.
- Task-modal comments show GitLab note authors' avatars (consumes backend 0.42),
  local authors keep theirs; both fall back to initials.

### [0.70.3] — 2026-06-17
- Composer «+» menu on mobile now drills into one sub-list at a time with a «‹ Назад»
  instead of fanning out side submenus that ran off the screen.
- Sidebar logo replaced with the accent-gradient «mt» monogram (no badge/wordmark,
  mirrors Android), freeing header space; tooltips on the brand-row tool buttons are
  suppressed on mobile (they overlapped the dropdowns they label).
- Sidebar footer dropdown button labels are left-aligned.
- Task-modal comments show the author's avatar (local via `/api/users/:id/avatar`,
  GitLab authors fall back to initials).
- Destructive items in context menus (delete board/project/group/column/task) are
  now red, matching Android.

### [0.70.2] — 2026-06-17
- Fix: the task-modal tag `+N` no longer clips a half-shown second tag — only
  chips that *wholly* fit are rendered (sized from an invisible measurement row,
  reserving room for the `+N`), so it accounts for both tag and screen width.

### [0.70.1] — 2026-06-17
- Task modal: tag chips in the trigger are clipped to one line and the overflow is
  **measured** into a `+N` chip (no more spilling past the modal edge; the count
  reflects what actually fits, not a fixed cap).
- Mobile composer bar: expanding now grows the bar rightward over the (hiding)
  tools on one line instead of wrapping the search onto a second row.
- GitLab integration modal on mobile: fields no longer overflow the dialog (grid
  children pinned to `min-width:0`, label columns + rule grids tightened), and the
  footer drops the «Последняя синхронизация» line above the buttons instead of
  squeezing it into a wrapping column.
- Notes are single-pane on mobile: the list, then tapping a note opens the editor
  full-width with a «К списку» back button (was a cramped two-pane split).

### [0.70.0] — 2026-06-16
- Notification routing rules can now target the additional event kinds
  «Изменение задачи», «Перемещение задачи» and «Архивирование» (consumes backend
  0.41); native/browser notification titles updated to match.

### [0.69.0] — 2026-06-16
- **Browser device notifications** (consumes backend 0.40): this browser
  auto-registers as a routable **device** channel, so routing rules can target it
  specifically. When a notification is routed here and you've granted permission, it
  shows as a **native browser notification** (foreground / open tab). Notification
  settings list device channels with a «это устройство» badge and a «Разрешить»
  permission button; `device` channels can be renamed / toggled / deleted but aren't
  manually added (they self-register). The bell now also updates live across all
  workspaces (not just the open one).

### [0.68.0] — 2026-06-16
- **Digest** setting in notification settings (consumes backend 0.39): a
  «Группировать в сводку» window (off / 5 / 15 / 30 / 60 мин) that batches a burst
  of notifications into one per-channel message.

### [0.67.1] — 2026-06-16
- Sidebar divider double-click now actually **animates** the rail collapse (forced an
  explicit width transition on the sider; Naive doesn't transition an externally
  bound `:width`).
- Task modal loader is now the brand **Tessera spinner** (was the generic Naive
  spinner), matching the board.
- Mobile sidebar footer shows the inline name + icon buttons (Настройки /
  Администрирование / Выйти) instead of collapsing into the avatar popover — the
  drawer has the room.
- Task modal: tag chips no longer shrink-wrap their text; capped at 3 + a `+N`
  chip. Author avatar no longer squishes when the name is long (`flex: none`).
- Mobile composer bar no longer jumps/wraps on expand — the search field always
  sits on its own row, revealed by the height growth instead of a layout reflow.

### [0.67.0] — 2026-06-16
- **Quiet hours** in notification settings (consumes backend 0.38): a «Тихие часы»
  toggle with start/end time selects. During the window external notifications are
  held until it ends (the bell still updates immediately). The window is evaluated
  in the user's timezone (sent along on save).

### [0.66.0] — 2026-06-16
- **Due-date & reminder notification settings** (consumes backend 0.37): a new
  «Дедлайны и напоминания» block in notification settings sets the per-user
  defaults — notify on/off, how long before the deadline (lead), repeat interval,
  and whether reminders are delivered to channels. The routing-rule editor gains a
  **«Напоминания»** event kind.
- **Per-task override** in the card's due popover (a reference tracker-style): under the
  calendar, set this task's notifications (on/off), lead and repeat — each
  defaulting to «По умолчанию» (inherit the user setting).

### [0.65.0] — 2026-06-16
- **Task due dates now carry a time** (prep for due-date notifications): the due
  picker in the task modal and on the card is now a datetime picker (mirrors the
  reminders picker). A missing/midnight time is treated as 00:00 and hidden in the
  UI — date-only and legacy tasks stay terse, timed ones show the hour (12h/24h per
  the locale setting). Due labels unified through a shared `formatDue` (task modal,
  card, list view, home). The backend already stored a full timestamp — no schema
  change.

### [0.64.0] — 2026-06-16
- **Message-template editor** for notification channels (consumes backend 0.36): a
  separate modal with a syntax-highlighted editor ({{…}} actions highlighted via a
  backdrop overlay), a clickable field reference on the right (inserts at the
  cursor), and a debounced **live preview** rendered server-side against sample
  data (parse errors shown inline). Empty template = built-in default.

### [0.63.0] — 2026-06-16
- Notification channels: new **«Shoutrrr (любой сервис)»** type — a single masked
  Service URL field that unlocks every shoutrrr service (slack, discord, ntfy,
  gotify, matrix, …). Consumes backend 0.35.

### [0.62.0] — 2026-06-16
Notification router phase A (web) — consumes backend 0.34.
- **New «Уведомления» section in Settings**: configure external delivery channels
  (Email / Telegram / Webhook) and Alertmanager-style routing rules. In-app
  notifications (bell / mobile) keep coming unconditionally; this only gates the
  external channels.
- **Channels**: add/edit/delete with type-specific fields (email address;
  telegram chat_id + bot token; webhook url + method + optional Authorization
  header), an enable toggle, a «Тест» button (synchronous send that flips the
  «проверен» badge on success), and a masked-secret edit flow (leave the secret
  blank to keep the stored one).
- **Routing rules**: match by event kind(s) and/or workspace → deliver to a set of
  channels, or «заглушить» to drop. Rules evaluate top-to-bottom, first match wins.
- API client: `notificationChannels` + `notificationRoutes`.

### [0.61.0] — 2026-06-16
- Sidebar divider: double-click now collapses the rail **smoothly** (the resizer
  follows the width animation; a drag only engages past a small move-threshold, so
  the double-click no longer flips the no-transition resizing state).
- Composer bar: the dimmed→active fade on expand/collapse (focus) now transitions
  smoothly instead of snapping.
- Fix: a selected (solid-filled) tag chip in the tag pickers (task card + modal)
  now uses a luminance-readable text colour (`onColor`) instead of hardcoded white,
  so labels on bright tags (e.g. yellow) stay legible.

### [0.60.0] — 2026-06-16
User-management phase U3b (web admin panel) — consumes backend 0.33.
- **Admin panel** at `/admin` (route + sidebar-footer entry, both gated by
  `is_admin`; the router also bounces non-admins home): lists every account on the
  instance with avatar, name/email and status badges (admin / you / deactivated /
  unverified), a name/email search, and per-row actions — copy a password-reset
  link to the clipboard, grant/revoke admin (revoke confirms), and activate/
  deactivate (deactivate confirms). You can't change your own admin/active state.

### [0.59.0] — 2026-06-16
- **GitLab user avatars** now show on synced tasks: the issue author's avatar
  appears on the card and in the task modal, and external GitLab assignees show
  theirs in the modal (consumes backend 0.32 `gitlab_author_avatar_url` /
  `gitlab.author_avatar_url` / `gitlab_assignees[].gl_avatar_url`). Falls back to
  initials when an avatar is private/unavailable.
- Fix: drop a redundant initializer in `localeOptions.js` flagged by lint.

### [0.58.1] — 2026-06-16
- Fix: tag text in the Home (summary) task list is now clamped to a legible
  lightness for the active theme (`readableHue`), so dark tags stay readable on the
  dark theme.

### [0.58.0] — 2026-06-16
- **User avatars on task cards and in the task modal**: assignees and the author/
  creator now show their uploaded avatar (a shared `UserAvatar` falls back to
  gradient initials on miss/error); also in the subtask hover card. GitLab-user
  avatars are wired (via `src`) and populate once the sync provides them.
- GitLab logo icons across the GitLab modal / task cards / task modal (manual edits
  folded in).

### [0.57.0] — 2026-06-15
User-management phase U2b (web) — consumes backend 0.31.x.
- **Workspace invitations** in the members modal: one «Пригласить» field adds an
  already-registered user instantly, or creates an email invitation with a
  copyable link for someone without an account; pending invitations are listed
  with a revoke button.
- **Account-flow pages** reached from email links: `/forgot-password` (request a
  reset), `/reset-password` (set a new one), `/verify-email` (confirm), `/invite`
  (accept an invitation — switches to the workspace). A «Забыли пароль?» link on
  the login screen; after signing in you return to the page you came from (`next`).
- **Settings** shows email verification status with a resend button.

### [0.56.4] — 2026-06-15
- **Fix (proper): boards now show immediately after login.** The per-project lazy
  load raced with the sidebar mounting (a restored-expanded project never fetched
  its boards → «нет досок» until a manual re-expand). Boards for expanded projects
  are now prefetched centrally in `selectWorkspace`, right after projects load.

### [0.56.3] — 2026-06-15
Small UX fixes from review.
- **Fix: boards now appear right after login** — a project whose expanded state was
  restored from persistence (not via a click) didn't fetch its boards, showing
  «нет досок» until a manual collapse+expand; boards are now loaded whenever a
  project is/ becomes expanded.
- **Fix: opening a task from a notification in another workspace now switches the
  sidebar to that workspace** (uses the notification's `workspace_id`).
- Login inputs no longer flash the browser's autofill background — autofilled
  values keep the frosted style and white text.
- The GitLab entry in the integrations menu now shows the GitLab logo.
- Collapsed composer bar is dimmed (and non-interactive) so it reads as one
  tap-to-expand surface rather than individually clickable chips.
- Subtask hover card: replaced the calendar emoji with a proper icon.

### [0.56.2] — 2026-06-15
- **Fix: the uploaded avatar now actually shows** (sidebar + settings). It was
  rendered via `n-avatar :src` which didn't pick up the URL once it arrived from
  `/auth/me`; now a plain `<img>` is used when an avatar is present, falling back to
  the initials avatar otherwise.

### [0.56.1] — 2026-06-15
U1b fixes from review.
- **Fix: avatar upload silently failed** — the request set `Content-Type:
  multipart/form-data` manually, which drops the boundary so the server couldn't
  parse the file; let the browser set it. Uploaded avatars now show (sidebar + settings).
- **Timezone and Country are now searchable selects** (filterable, full list on
  focus) built from native `Intl` data, instead of free-text inputs.
- **Fix: a custom board background now covers the whole board area** — it bleeds
  under the layout padding to the sidebar/header edges instead of only sitting
  behind the columns.

### [0.56.0] — 2026-06-15
User-management phase U1b (web) — consumes backend 0.30.0.
- **Settings page** (`/settings`, from the sidebar account menu): edit the profile
  (display name, split legal name Фамилия/Имя/Отчество, bio, company, job title;
  email read-only), upload/remove an **avatar**, and change the password.
- **Appearance moved from localStorage to the account (DB)**: accent, theme
  (system/light/dark) and a new **board background** (CSS colour or image URL) now
  persist server-side per user and are hydrated on sign-in (localStorage stays a
  first-paint cache; legacy `tessera_color`/`tessera_dark` are migrated).
- **Localization preferences** (language, timezone, country, time/date format,
  week start) stored per user; date pickers now follow the chosen **week start**
  and **date/time format**. (UI language switching itself comes later with i18n.)
- **Inline member role editing** in the members modal — a role dropdown that PATCHes
  the membership, replacing the old remove-and-re-add. Owner row is read-only.
- **Type-to-confirm project deletion**: the project name must be typed before the
  delete is allowed (reusable `ConfirmByName`).

### [0.55.0] — 2026-06-15
- **Composer bar polish**: the inter-chip / inter-row gaps and the vertical
  padding now match the 8px side padding, so a multi-row (expanded / overflowing)
  bar isn't cramped. Tapping anywhere in a collapsed bar — including on a chip, the
  «＋», the clear «×» or the search field — now just expands it; the controls act
  only once the bar is expanded.
- **Animations**: an "airy" animated aurora gradient on the login screen (drifting
  brand-purple blobs, `screen`-blended, with a `prefers-reduced-motion` guard and a
  form fade-up); route transitions on the login↔app switch and the
  Home/Board/Notes/Reminders views; a write↔preview cross-fade in the Markdown
  editor.

### [0.54.3] — 2026-06-14
- Performance (large boards): typing in the board search no longer writes the
  saved-view state to localStorage on every keystroke — the write is debounced
  (and flushed on leave), removing input lag on mid hardware.
- Performance: the per-card priority-flag SVG gradient is no longer inlined once
  per card (hundreds of hidden `<svg>` defs on a big board); cards now reference
  four shared gradient defs (one per priority level) declared once in `App.vue`.

### [0.54.2] — 2026-06-11
- The composer bar and the toolbar buttons beside it are now exactly the same
  height (40px, border-box) — the buttons no longer come up short.

### [0.54.1] — 2026-06-11
- Fix: the board loader stays vertically centred during the initial load instead
  of flashing near the top before jumping to the middle.
- The toolbar buttons to the right of the composer bar (subtasks toggle, saved-
  view load/save) now match the composer bar's height.

### [0.54.0] — 2026-06-11
- **Composer bar** (a reference tracker/GitLab-style): grouping, sort levels and filters now
  render as removable chips in one wide bar with an «＋» menu to add any
  facet (group by status/tags/namespace, a sort field, or a priority/assignee/
  tag/due filter) and an inline name search — replacing the separate
  Группировка / Сортировка / Фильтры toolbar buttons. The grouping chip toggles
  status↔tags on click; a sort chip flips its direction; a clear-all «×» resets
  filters + sort. Subtasks toggle and the saved-view buttons stay on the right.

### [0.53.0] — 2026-06-11
- **Multi-level sort**: the sort control is now a popover with an ordered list of
  sort levels (field + direction), applied as primary/secondary/… tie-breakers —
  e.g. by due date desc, then by priority desc. Fields: priority, due, title,
  number; due-less tasks always sink to the bottom. Persisted per board and in
  saved views (old single-sort views migrate automatically).

### [0.52.0] — 2026-06-11
- **Saved board views** (a reference tracker-style, per-user, server-side): two toolbar
  buttons next to the subtasks toggle — a folder to load a saved view and a disk
  to save the current one (grouping, tag namespace, sort + direction, filters,
  expanded subtasks, layout). Views are stored in the DB (not just localStorage),
  so they follow you across devices (web/Android); save overwrites a same-named
  view, and views can be deleted. The active view's name shows in the load
  button's tooltip.

### [0.51.0] — 2026-06-11
- Avatar initials are smarter: a two-word name → first letter of each
  (`Василий Соколов` → ВС), a dot handle → each part (`a.fokin` → AF), a single
  word → its first two letters (`msdnna` → MS). Shared `utils/initials`.
- `@`-mentions are highlighted even when the user isn't in Tessera (any
  `@handle`, e.g. GitLab `@v.sokolov`), not only known members.
- Descending due-date sort keeps tasks **without** a due date at the bottom
  (instead of floating them to the top).
- Workspace toolbar: GitLab moved under an **«Интеграции»** dropdown button
  (single GitLab item) instead of a standalone button.

### [0.50.0] — 2026-06-11
- Fix: Markdown descriptions that contain inline HTML (e.g. GitLab `<details>`)
  no longer render as plain text — the HTML-vs-Markdown heuristic now only treats
  content as raw HTML when it *starts* with a block tag (legacy editor output),
  so code fences/bold/etc. render correctly.
- Board sort gains a **direction** (по возрастанию/убыванию) shown next to the
  sort selector once a field is chosen; remembered per board.
- Card due dates show the **year** when it isn't the current year, and an
  **overdue** date (past due, not done) is tinted red.
- GitLab settings: fixed the broken "add rule / add value" buttons (now dashed
  with a clear icon + label) and enlarged the Синхронизировать / Сохранить
  buttons.
- De-jargoned UI wording (Последняя синхронизация, Правила меток, …).
- The card tag picker popover is width-capped (no longer stretches across the
  board), and the workspace Tags manager applies the theme-readable tag colour.

### [0.49.0] — 2026-06-11
- Tag grouping on the board can now be scoped to a **tag namespace**: when
  grouping by tags, a namespace picker (auto-detected prefixes like `T: `, `C: `,
  `effort::`, or a custom value) makes the columns just the tags in that
  namespace — e.g. group by type or by team. Empty = all tags (as before). The
  choice is remembered per board.

### [0.48.0] — 2026-06-11
- GitLab integration settings get a **generic rule editor**: add ordered rules
  with a match (prefix or regex), an action (status / priority / board / tag /
  group / ignore) and per-action params (value maps for status/priority/board, a
  keep-prefix toggle for tags), plus a default column and default action for
  unmatched labels. Replaces the fixed status/priority/tag form.

### [0.47.0] — 2026-06-11
- GitLab integration settings gain a **«Источник срока»** (due source) selector:
  issue-then-milestone (default) / issue only / milestone only / off.

### [0.46.0] — 2026-06-11
- Board card and task modal now render **external GitLab assignees** (a GitLab
  user with no Tessera account) alongside Tessera assignees — a muted avatar with
  a hover tooltip; the author → assignee cascade includes them.
- Synced **comments** display their GitLab author (and a `· GitLab` marker) when
  the author isn't a Tessera user; such comments have no local edit/delete.

### [0.45.1] — 2026-06-11
- Tag text is now legible on both themes: the label colour is lightness-clamped
  for the active theme (`readableHue`) when used as text/gradient, so a dark blue
  (`C: Backend`) stays readable on dark and a light grey (`T: Configuration`)
  stays readable on light — the chip background tint is unchanged.
- Board-card author and assignee avatars now show a proper hover tooltip
  (`n-tooltip`) instead of the native `title` text.

### [0.45.0] — 2026-06-11
- Tasks now show an **Автор** (author) field — who created the card, read-only,
  fixed at creation. The task modal gains an Автор row (the GitLab issue author
  for synced tasks, otherwise the Tessera creator); the GitLab line keeps just
  the issue link.
- On the board card, the single assignee avatar is replaced by **author →
  assignee(s)**: the creator's (muted, non-clickable, tooltip-only) avatar with
  an arrow pointing at the clickable assignee avatar(s), cascading to several.

### [0.44.0] — 2026-06-11
- GitLab integration UI (a single modal opened from the workspace toolbar):
  - **Account** — connect a GitLab account by base URL + personal access token
    (validated server-side), shows the linked `@username`, disconnect with
    confirmation.
  - **Integration** — pick the GitLab project path and target board, toggle the
    integration, choose an auto-sync interval, and a **rule editor** for the
    custom bindings: status-label → column rows, priority-label → level rows,
    a default column, label prefixes, and the tag mode / keep-prefix switches.
  - **Синхронизировать** button runs an on-demand pull and reports the counts.
- Synced tasks now show their GitLab provenance: a clickable `!iid` chip on the
  board card, and an author line (`GitLab !iid · автор @login`) in the task
  modal, both linking to the source issue.

### [0.43.0] — 2026-06-10
- Destructive/irreversible actions now confirm via an inline `n-popconfirm`
  popover (Android-style) instead of a centred modal dialog: hard-delete and
  archive of tasks (board card + list/calendar context menus + task modal),
  delete of columns, projects, boards, groups, tags, members, notes, reminders,
  comments, and archived tasks. New confirmations added where there were none —
  detaching a relation and deleting an attachment.
- Archiving a task from the board/list/calendar context menu now asks for
  confirmation too (previously only the task modal did).
- Popover buttons carry the accent gradient (red error gradient for destructive
  actions); the warning icon is repainted with the matching SVG gradient.
- Context-menu confirmation popovers close on an outside click.
- Removed the now-unused custom `confirmHardDelete` dialog helper and the
  `n-dialog-provider` (no `useDialog` consumers remain).

### [0.42.8] — 2026-06-09
- Notification bell badge: accent gradient + centred number (matches the modal
  tab counter), instead of naive's off-centre red.
- Fix the «＋ Создать подзадачу» button (and its spacing) vanishing on a plain
  mouse-press on a card — Sortable adds `.sortable-chosen` on mousedown (before
  any drag), and the hide rule was keyed on it, so cards jumped. The rule now
  only hides on the actual drag clone / placeholder.
- Toolbar / layout toggles (Доска/Список/Календарь, Группировка…) stay a clean
  solid accent when active instead of a clipped gradient: background-clip:text
  clipped the label and an objectBoundingBox SVG gradient is degenerate on the
  axis-aligned <line>s these icons are built from (parts vanished). Gradient is
  kept on shape-based icons (column glyph, flag, check).
- Action icons in the list / calendar task context menus (shared useTaskMenu).

### [0.42.7] — 2026-06-09
- Fix focus rings being clipped on the left/top: containers with `overflow-y:
  auto` (filter panel, column task list, tag manager list) force `overflow-x`
  to clip — added small padding so focused selects / checkboxes / inputs aren't
  cut off.
- More accent-gradient coverage: list-view column dots and overdue dates; Home
  stat-card left borders (gradient + corner wrap) and the column-pointer pill +
  overdue date text; note active border + reminder overdue border; the sidebar
  resizer bar (vertical gradient); the date-picker selected day; the «Удалить»
  note button is now a red ghost; «Переименовать» project/group buttons are
  accent ghosts; and the transparent layout/toolbar toggles gradient their
  label + icon when active.
- Priority flag pill: the flag icon now carries the active priority's gradient
  (per-card SVG gradient via stroke url), matching Android.

### [0.42.6] — 2026-06-09
- Actually fix the colour-swatch gradient seam (the 0.42.5 `background-origin`
  fix was defeated by the inline `background:` shorthand, which resets
  background-origin to padding-box). Swatches now set `background-image` inline
  (the shorthand isn't used), so the stylesheet's `background-origin: border-box`
  applies and the gradient fills the whole circle with no square seam.

### [0.42.5] — 2026-06-09
- Fix the priority/column accent border (root cause: replacing the original
  `border-left`/`border-top` with a `::before` strip lost the corner wrap — a
  strip is clipped at the corner, a border wraps onto the adjacent edges by the
  radius). Now the accent side has a transparent border and the gradient is
  painted on the border-box background layer, so it shows through and wraps the
  rounded corners exactly like the Android client (cards: left → top/bottom
  corners; columns: top → side corners).
- Fix the "square inside the colour-picker circles": with a transparent border
  the gradient defaulted to `background-origin: padding-box` and repeated in the
  border ring, leaving a square-ish seam. Swatches now use
  `background-origin: border-box` (one continuous gradient).

### [0.42.4] — 2026-06-09
- Bigger corner radius on cards (12px) and columns (14px) to match the Android
  client; the priority / column accent bars follow the rounded corners.
- Tab counter: centre the number inside the circle (was shifted) via flex.
- Accent-gradient checkmark in the grouping / sort dropdowns (shared SVG accent
  gradient def in App.vue + a reusable `.grad-icon` helper).
- Reset native `appearance` on colour-picker swatch buttons.

### [0.42.3] — 2026-06-09
- Accent gradient on Naive buttons, done by variant (Naive emits no class for
  text/quaternary, so it's split correctly): filled primary buttons get a
  gradient fill; ghost buttons (any type) get border + label as one same-hue
  gradient unit (1px gradient ring via mask, hue from the button's own colour);
  transparent toolbar/layout toggles opt out via an `ngrad` class so they don't
  turn into solid blocks.
- Column status glyphs now carry the per-column gradient (SVG `linearGradient`
  referenced by fill/stroke, applied via attribute selectors so both fill- and
  stroke-based icons recolour correctly).
- Restore the card's rounded corner on priority cards: the left priority bar is
  clipped to the card radius (overflow:hidden) instead of squaring it off.
- Fix the tag-picker chips: a transparent-fill gradient border was bleeding over
  the label. Picker chips now use a solid gradient fill (selected) / soft tint
  (unselected) with readable text, matching the Android reference.
- Tab counter is a proper small circle (was an oval) and dims on inactive tabs.
- Gradient swatches in the tag-colour picker and the appearance (accent-colour)
  picker too.
- Login / register: drop the "Tessera" wordmark and the card — the form now sits
  directly on the brand gradient with frosted inputs and a white submit button
  (purple label), matching the Android auth screen.

### [0.42.2] — 2026-06-09
- Comprehensive accent-gradient pass (the soft same-hue diagonal from the Android
  client), via a shared `utils/gradient.js` (`hueGrad` / `hueGradVert` /
  `tagPillBg` / `swatchBg`) applied across:
  - column top bars (column-hue diagonal) and card priority bars (vertical
    priority-hue gradient, replacing the flat left border);
  - priority dots everywhere (board / list / mini / calendar-less home / modal);
  - tags — gradient on the border + text, the fill left subtle (matching
    Android), on both the card pills and the tag-picker chips, in card + modal;
  - the task-modal tab underline;
  - colour-picker swatches (column header + project/group icon picker) and the
    sidebar project/group icon tiles;
  - switches, checkboxes, radio-button toggles and ghost-primary button labels
    (global Naive overrides).
- Make the task-modal tab counter a touch smaller (was oversized).

### [0.42.1] — 2026-06-09
- Fix the accent gradient leaking onto board controls: the layout switch, the
  grouping/sort/filter buttons and the subtasks toggle no longer get a gradient
  fill (naive renders quaternary/filled primaries with the same class, so the
  gradient is now scoped to the auth submit button only).
- Fix childless cards showing an empty placeholder box under them in the
  collapsed subtask view (`.subs.collapsed` is now fully hidden instead of
  leaking the list padding/border).
- Fix expanded subtask cards getting clipped after toggling the subtasks view
  off and on again (the subtask list now remounts on mode change, clearing stale
  Sortable layout styles).
- Column status glyphs are tinted with the column colour, falling back to the
  accent (like the column's top bar) instead of a dull grey.
- Context menus now carry action icons (task card, column header, sidebar group/
  project/board and the add menus), matching the Android client.
- Tab counters in the task modal use the accent colour instead of naive's red.

### [0.42.0] — 2026-06-09
- Fix: the «＋ Создать подзадачу» button no longer starts a card drag on desktop.
  The board's task Sortable now filters the add-subtask button and its inline
  input (with `prevent-on-filter` off), so a press opens the input instead of
  grabbing the card. (Mobile tap already worked.)
- Fix stacked tag pills (2+ tags): the peeking layers are now opaque soft tints
  of each tag's own hue instead of translucent shadows (no more see-through
  overlap), and the gap after the tag pill matches the gap before it (reserve
  exactly the stack peek, drop the extra margin).
- Swap the card pill order to priority → due date → tags.
- Accent-gradient port from the Android client: new `--t-accent-grad` /
  `--t-accent-grad-subtle` CSS vars (soft same-hue diagonal, base colour pinned
  at the centre) applied to filled primary buttons, avatars, the «сегодня»
  calendar cell, the mention-list selection and tag text.
- Brand logo + favicon: the sidebar mark and the browser favicon now use the
  Tessera tile (`design/tessera-brand`); added `theme-color` + apple-touch-icon.
- Custom branded loader (`TesseraSpinner`): the Tessera tile tumbles in place of
  naive's default spinner on the board and home loading states.
- Redesigned login/register screens (`AuthLayout`): a full-bleed brand gradient
  with the white monogram + wordmark over a themed form card.

### [0.41.5] — 2026-06-04
- Centre the icon-only Архив/Удалить buttons in the mobile modal footer (drop
  the icon margin reserved for the hidden label).

### [0.41.4] — 2026-06-04
- Task modal on mobile: footer buttons stay on one row (Архив/Удалить become
  icon-only when space is tight); the detail tab strip no longer shows naive's
  horizontal scroll shadow in the overflow gutter.

### [0.41.3] — 2026-06-04
- Childless cards now nest the same way as cards that already have subtasks: no
  dashed «вложить как подзадачу» hint — instead, while a drag is in progress the
  card shows its (empty) subtask block so the dragged task visibly attaches under
  it. (The block stays hidden when idle.)

### [0.41.2] — 2026-06-04
- Fix duplicated «вложить как подзадачу» / «＋ Создать подзадачу» labels while
  dragging onto a childless card: the dragged card carries its own add-subtask
  button and empty nest hint, which Sortable relocates into the drop target.
  Both are now hidden on the card being dragged (sortable-chosen/ghost/drag/
  fallback).

### [0.41.1] — 2026-06-04
- Fix the glitchy subtask-nesting drag preview: replaced the separate empty
  "sink" dropzone (which rendered the dragged card squished/centered and flashed
  a duplicate) with a single always-mounted subtask list bound to the real model.
  A dropped task renders immediately as a full-width subtask; childless cards
  still show a dashed "вложить как подзадачу" zone while dragging.

### [0.41.0] — 2026-06-04
- Drag-and-drop subtask nesting (backlog item): subtask lists now share the
  "tasks" Sortable group with the columns, so a task dragged onto another task's
  subtask area becomes its subtask (`PATCH /tasks/:id/parent`), a subtask dragged
  out onto a column detaches back to top-level, and subtasks can move between
  parents. Childless cards reveal a dashed "вложить как подзадачу" drop zone
  while a drag is in progress.

### [0.40.0] — 2026-06-04
- Sidebar tree alignment: fixed-width chevron + icon columns so groups, projects
  and boards line up at a given level regardless of icon (leaf boards get a
  chevron-width spacer; group/project/board icon boxes share one footprint).
- The tree's expand/collapse state now persists across reloads (per node id,
  localStorage) — groups default open, projects default closed; a restored-open
  project loads its boards on mount.

### [0.39.0] — 2026-06-04
- Hard delete now confirms first (it's irreversible — the task doesn't go to the
  archive): context menus (card / list / calendar) and a new red-ghost «Удалить»
  button in the task modal all show a confirmation dialog. The modal's «В архив»
  is now primary-ghost (was red).
- Sidebar tree: boards under a project now use the same gutter line as
  group→child, so the whole tree shares one indentation style.
- Groups get an icon + colour: extracted a shared `IconColorPicker` (used by
  projects and groups). Group picker has the curated grid + ionicons search (no
  upload), and a colour picker defaulting to transparent; the default icon stays
  the folder. Project picker keeps SVG/PNG upload.

### [0.38.0] — 2026-06-04
- Syntax highlighting for fenced code blocks (highlight.js + marked-highlight):
  json, yaml, python, js/ts, bash, go, sql, html/xml, css, markdown, dockerfile,
  ini and their aliases. A focused language set keeps the bundle lean; `mermaid`
  blocks stay plain text for diagram rendering. Themed token palette + base
  code/pre styling that work in both light and dark.

### [0.37.0] — 2026-06-04
- Images in descriptions & comments: the Markdown editor gains an image button,
  plus paste-image and drag-and-drop — each uploads via `/uploads` and inserts
  `![](url)`. Rendered images are capped to the content width.
- Mermaid diagrams: a ```mermaid``` fenced block (there's a toolbar button to
  insert a starter) renders as an SVG in the preview/comments. New `RichContent`
  component does Markdown + async Mermaid rendering (Mermaid is lazy-loaded only
  when a diagram is present, theme-aware, securityLevel strict). It replaces the
  raw `v-html` used for the description preview and comments.

### [0.36.1] — 2026-06-04
- Project colour picker gains a «без фона» (transparent) swatch — the icon tile
  then renders with no coloured square (glyph/initials use the text colour), so a
  custom icon shows on its own. Applied in the row, rail and flyout.

### [0.36.0] — 2026-06-04
- Project icons: the picker's grid gains a «＋» with a menu to **search the full
  ionicons5 collection** (lazy-loaded on open; the picked icon is stored as its
  own SVG markup) or **upload a custom SVG / alpha PNG** (sanitised SVG or a
  data-URL, capped at 40 KB). New `ProjectIcon` component renders all forms
  (curated key / inline SVG / image / initials) wherever a project icon shows
  (row, collapsed rail, flyout). `icon` storage stays a plain string.

### [0.35.1] — 2026-06-04
- Calendar now fits the screen: the month grid uses `minmax(0, 1fr)` columns and
  zero-min cells, so a long task title is clipped instead of stretching a column
  and pushing the rest off-screen.
- Mobile sub-toolbar no longer overflows off the right (that stray rounded white
  edge was the task-search input): button labels are hidden (icons only) and the
  search fills the remaining width.

### [0.35.0] — 2026-06-04
- Filters popover: removed the redundant «Фильтры» heading (the «Сбросить» link
  stays, right-aligned, only when filters are active).
- Mobile board controls: a new menu button right of the search (`BoardMobileMenu`)
  opens layout selection (Доска/Список/Календарь, active ticked) plus Теги and
  Архив — which were previously desktop-only in the header.

### [0.34.3] — 2026-06-04
- Grouping control is now a dropdown like Sort (opens the option list straight
  from the toolbar button, active option ticked) instead of an in-popover button
  group — scales better as grouping modes grow.

### [0.34.2] — 2026-06-04
- Fix List/Calendar views crashing: `useTaskMenu` received `columns` as a getter
  function but resolved it with `unref` (which doesn't call functions), so the
  options computed ran `.filter` on a function and threw on every render. Resolve
  function / ref / array forms properly. (Regression from 0.33.0.)

### [0.34.1] — 2026-06-04
- Sort dropdown: the active option is now marked with a right-aligned primary
  check icon (via render-label) instead of a trailing text "✓".

### [0.34.0] — 2026-06-04
- Color picker: added the default grey (#9aa0aa, the «К работе» column colour) to
  the column swatches so it isn't lost when recolouring.
- Sub-toolbar: dropped the redundant «Группировка» heading inside the grouping
  popover, and the «Сортировка» button now opens the option list directly
  (n-dropdown, current choice ticked) instead of a popover-wrapped select.
- Sidebar context menus now open on a still touch long-press (new `useLongPress`
  on groups, projects and boards) — moving is a drag, holding opens the menu —
  fixing the menu never appearing on touch inside the mobile drawer.

### [0.33.7] — 2026-06-04
- Mobile: stretch the board columns to fill the screen height (board-scroll gets
  a viewport-based height, columns `align-self: stretch`) so the whole column is
  a drop target — fixes the cramped drag area where the horizontal scrollbar sat
  right under the last card.

### [0.33.6] — 2026-06-04
- On touch devices the "＋ Создать подзадачу" button is always visible (kept
  pale) instead of hidden behind hover — fixes the confusing empty space under a
  card on mobile. Desktop keeps the hover reveal.

### [0.33.5] — 2026-06-04
- Fix the touch context menu never opening: Sortable starts a drag ~160ms into
  any hold (even without movement), so the previous "is a drag active" guard
  always suppressed it. Now we track actual finger movement instead — a still
  long-press opens the menu, a press-and-move drags without opening it.

### [0.33.4] — 2026-06-04
- Drag auto-scroll no longer skips a column: it tracks an explicit target column
  index (re-synced when the pointer returns to centre) instead of reading the
  mid-animation `scrollLeft`, so each step advances exactly one column.
- Suppress the long-press context menu while a touch drag is in progress (detect
  Sortable's `.sortable-fallback` clone) — so dragging a card / sidebar item no
  longer pops the menu; a still long-press still opens it.

### [0.33.3] — 2026-06-04
- Drag auto-scroll now advances one column at a time (smooth snap to the next
  column) instead of sliding continuously while the pointer sits in the edge
  zone — one step per entry, then one more every 600ms if held. More intuitive.

### [0.33.2] — 2026-06-04
- Mobile drag auto-scroll finally works: the board's mobile `scroll-snap-type:
  x mandatory` + `scroll-behavior: smooth` were reverting the per-frame
  `scrollLeft` nudges (snapping back to a column), so no scrolling was visible.
  Both are now disabled for the duration of a drag and restored on drop. This,
  not the coordinate source, was the real blocker.

### [0.33.1] — 2026-06-04
- Edge auto-scroll now tracks Sortable's moving drag clone (`.sortable-fallback`)
  each frame instead of relying on touch/pointer move events, which weren't
  firing reliably during a touch drag — so the board should auto-scroll toward
  off-screen columns on mobile. Desktop still uses the dragover X.

### [0.33.0] — 2026-06-03
- Custom edge auto-scroll while dragging a card or column: the board scrolls
  horizontally when the pointer nears either edge (speed scales with depth),
  driven from pointer/touch/dragover coordinates via requestAnimationFrame — so
  a card can be dragged onto an off-screen column on touch and desktop alike.
- "Переместить в колонку →" added to the task context menu (cards, list and
  calendar rows) as a no-drag way to move a task between columns.

### [0.32.2] — 2026-06-03
- Revert all the experimental drag-and-drop tweaks (Sortable scroll options,
  the `dragging`/user-select guard) back to the 0.31.x baseline. The mobile
  horizontal auto-scroll to an off-screen column remains an open issue, to be
  tackled with a dedicated approach.

### [0.32.1] — 2026-06-03
- Revert `forceFallback` on the task lists — it caused card/column text to get
  selected while dragging on desktop and didn't help touch (Sortable already
  uses fallback on touch). Kept the auto-scroll tuning and, as a guard, disable
  text selection on the board while a drag is in progress.

### [0.32.0] — 2026-06-03
- Right-click context menus extended across the app: tasks in the List and
  Calendar views (new shared `useTaskMenu` composable), collapsed subtasks on
  cards, and sidebar items — groups (new project/group, rename, delete),
  projects (new board, rename, delete) and boards (open, rename, delete).
- Mobile: enable touch drag auto-scroll (`forceFallback` + `scroll`) on the task
  lists so a card dragged toward the screen edge scrolls the board and can drop
  on the neighbouring column.
- Task modal: removed the footer "Открепить от родителя" button (it duplicated
  the "Открепить" action on the Родитель row).

### [0.31.4] — 2026-06-03
- Fix expanded subtask stack clipping the parent's bottom: the first child
  shared the parent's z-index (tie → child won), so children now sit strictly
  below the parent (z-index lowered). Parent's rounded bottom + border show again.
- Softened subtask card / collapsed-list shadows to match the parent card.

### [0.31.3] — 2026-06-03
- Reworked the subtask cascade into a fanned card stack: the parent keeps its
  rounded corners and shadow and sits on top; each expanded subtask card peeks
  ~8px from under the one above with its own shadow and a rounded bottom. Child
  background is `color-mix(--t-surface 70% / --t-bg)`.
- Collapsed subtasks now render as a single card emerging from under the parent
  that lists the subtask rows (instead of loose rows).
- Fixed the "＋ Создать подзадачу" button being invisible (its hover trigger
  still pointed at the old card root after the cascade moved out of the card).

### [0.31.2] — 2026-06-03
- Expanded subtask cards now attach to the parent card: the parent's bottom
  corners flatten, the child has square top corners and no own top border, no
  left indent, and the last child rounds off the bottom. Child background uses
  `--t-hover` (distinct from the column) — tweak via the `--sub-bg` custom
  property on `.card.nested`.

### [0.31.1] — 2026-06-03
- Subtasks now cascade *below* the parent card instead of nesting inside it:
  collapsed = indented text rows; expanded = full property cards one shade
  darker than the parent. Subtask cards drop the "create subtask" button (deeper
  nesting is via the task modal) and don't recurse further on the board.
- Header layout switcher restyled to icon+text buttons like Теги/Архив, but as
  flat text buttons (selected one in the primary colour, no hover/active block).
- Sub-toolbar grouping button is labelled «Группировка» (not the current choice).

### [0.31.0] — 2026-06-03
- a reference tracker-style board chrome overhaul (new `stores/boardView.js` bridges the
  board and the global header):
  - Layout switcher (Доска/Список/Календарь) moved into the header as icon
    buttons, left of the search; the search now centres in the remaining space.
  - Теги and Архив moved to the header (right). Tag manager and the archive
    modal now live there and nudge the board to reload on change.
  - Grouping / sort / filters become a sub-toolbar directly under the header
    (icon dropdowns), with a new "expand subtasks" toggle and a task-name search
    pinned to the right.
- Subtasks on cards can be expanded to full property cards (priority, tags,
  due, assignees) or kept as compact rows, via the sub-toolbar toggle.
- Columns show a a reference tracker-style status glyph (open / half / check, tinted with
  the column colour) instead of the drag grip; drag a column by its header.
- Right-click context menus: task cards (open, complete, priority, add subtask,
  archive, delete) and columns (rename, mark done, delete).

### [0.30.1] — 2026-06-03
- Fix sidebar overflow at intermediate widths: a new "narrow" state (expanded
  but < ~216px) moves the workspace tools to the header and hides the
  add-workspace «+», and the sidebar clips instead of showing a horizontal
  scrollbar — so the logo and buttons no longer get squeezed off-screen.
- The resize divider now centres in the gutter between the sidebar and the
  first card, instead of hugging the sidebar edge.

### [0.30.0] — 2026-06-03
- Drag-resizable sidebar replacing the slide-out collapse trigger: grab the
  divider in the gutter and drag to set the width (persisted, capped at 264px).
  Drag narrow (below ~170px) and it snaps to the compact icon rail; double-click
  the divider to toggle the rail. The board/work area reflows to the new width.
  The divider sits in the gap between sidebar and content and highlights on
  hover/drag.

### [0.29.1] — 2026-06-03
- The detail tab strip no longer slides its active indicator when navigating
  between a task and its subtask/related task (tabs reset cleanly per task).
- More breathing room in the task modal: between «Описание» and its tabs,
  between the tab strip and its content, and before the footer buttons.

### [0.29.0] — 2026-06-03
- Description opens on the «Просмотр» tab when it already has content (empty
  descriptions still open on «Написать» for editing).
- Subtasks moved into their own tab (second, after Комментарии) with a count
  badge, freeing the main column.
- Hovering a subtask shows a kanban-style card (title, number, priority, due
  date, tags, assignees). The same hover-card pattern is intended for user
  mentions later.
- Comment edit/delete actions are right-aligned in the comment header.
- Relations: focusing the task field opens a cross-board task autocomplete
  (number + title, grouped by project/board, searchable) so tasks from other
  projects/boards link easily; «Связать» moved to the right.

### [0.28.0] — 2026-06-03
- Fix @-mention picker needing the `@` typed twice (it read the lagging
  modelValue prop instead of the live textarea value) — one `@` now opens it and
  the inserted mention is clean.
- Selection toolbar fades/slides in, and its link button uses a themed icon
  (was a coloured emoji).
- «Написать»/«Просмотр» tabs restyled to match the modal's bottom tabs
  (Комментарии/Связи…) for a unified look.
- Comment «Отправить» moved to the left so it no longer sits under the modal's
  Сохранить/Отмена buttons.

### [0.27.0] — 2026-06-03
- Markdown editor polish: it now shares the modal's background seamlessly (no
  boxed textarea). Formatting moved to a floating toolbar that appears above the
  selected text (bold/italic/strike/code/heading/list/quote/link); «Написать» /
  «Просмотр» stay as flat underline tabs.
- The link button no longer opens a JS prompt — it inserts a `[text](https://)`
  skeleton and drops the caret after `https://` to keep typing the address.
- Comments: the editor is full-width with «Отправить» moved below it (it no
  longer squeezes the input).
- Links in the Markdown preview / comments are themed (primary colour, underline
  on hover) instead of default browser blue.

### [0.26.0] — 2026-06-03
- Editor rework: replaced TipTap (broke under the dark theme, toolbar/commands
  unresponsive, heavy bundle) with a theme-native `MarkdownEditor` —
  «Написать»/«Просмотр» tabs + a Markdown formatting toolbar (bold, italic,
  strike, code, heading, lists, quote, link) that wraps the textarea selection,
  plus live preview. Content is stored as Markdown again (no migration, old
  descriptions/comments render unchanged). Cuts the board bundle ~535→168 kB.
- @-mentions now render highlighted in posted comments and the preview (not just
  in the picker); the picker is a theme-native dropdown over the textarea.
- List view now spans the full working width (was capped at 1100px). Reminders
  view likewise no longer capped at 720px.
- Fix: the column "завершающая колонка" toggle no longer overflows its button.

### [0.25.0] — 2026-06-03
- WYSIWYG editor (TipTap) replaces the Markdown textarea for task descriptions
  and comments. New `RichEditor` component with a formatting toolbar (bold,
  italic, strike, inline code, heading, bullet/ordered lists, quote, link).
  Existing Markdown content is converted to HTML on load and still renders
  correctly (backward-compatible `renderRich`). Adds the `@tiptap/*` packages.
- @-mentions in comments (feature #3, UI side): type `@` to open a member
  picker and insert a mention chip; mentioned workspace members are notified.
  `addComment` now sends the mentioned user ids alongside the body.
- Board multi-views (feature #6): a Доска / Список / Календарь switcher in the
  toolbar (persisted per board in localStorage). List view groups tasks by the
  active grouping with priority, tags, due date and assignees; Calendar view
  lays tasks out on their due date in a month grid, with a "Без срока" tray.
- Column header gains a "завершающая колонка" toggle (with a check marker) to
  choose which status auto-completes tasks — see backend 0.14.0.

### [0.24.0] — 2026-06-03
- Collapsed sidebar now shows projects and groups as icons too; hovering one
  opens a flyout (budget-style) with its boards / nested projects to navigate
  from. Small separators now divide the functional groups (logo / nav /
  projects) in the rail, with a touch more spacing in the expanded sidebar.

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

### [0.43.0] — 2026-06-18
- **Tags are now per-project, not per-workspace** (migration 0023). A workspace can
  hold unrelated projects whose tag vocabularies shouldn't mix. `tags` gain a
  `project_id` (unique on `(project_id, name)`); existing tags are consolidated into
  the «Неолант Тенакс» project (or each workspace's oldest project) and leaked
  cross-project associations are pruned. Tag create/list move to `/projects/:id/tags`;
  `GET /workspaces/:id/tags` stays as a read-only all-tags-in-workspace endpoint for
  cross-project views (Home). GitLab synced labels attach to the integration board's
  project.
- **Human-readable URL slugs** (migration 0024). Boards and notes gain a `slug`
  (transliterated from the name, e.g. «Общие задачи» → `obshchie-zadachi`), backfilled
  at startup and unique (boards globally, notes per workspace). `GET /boards/:id`
  accepts a slug or a UUID. New `GET /workspaces/:id/tasks/by-number/:number` resolves
  the per-workspace task number for `?task=<number>` deep links.
- Fix: device notification channels no longer lose their custom name on
  re-registration — the auto-generated label only seeds a new channel; an existing one
  keeps the name the user set.
- Fix: the GitLab attachment proxy falls back to a redirect (like the avatar proxy)
  when it can't stream the file server-side, so GL-hosted images in synced comments
  load again in the browser instead of 404ing.
- Task activity events now expose `actor_id` so clients can show the actor's avatar.

### [0.42.0] — 2026-06-17
- GitLab note authors' avatars are captured on sync (migration 0022:
  `task_comments.gl_author_avatar_url`) and routed through the signed avatar proxy,
  so synced comments can show the author's avatar like task cards. The notes
  GraphQL query now selects `avatarUrl`.

### [0.41.0] — 2026-06-16
- **More notification kinds + smarter context.** Archiving a task now notifies its
  participants (`archived` kind); the existing `updated` / `moved` kinds are now
  titled (so they read well in delivered messages and are routable). Notification
  text **inlines short context** — a task title or comment body is quoted inline
  only when it's ≤16 chars (e.g. «Починить чайник»); longer titles/comments and
  description edits show just `#N` + what changed (`updated` already lists the
  changed fields). Comments and @-mentions now carry the short comment body.
- (Notes CRUD notifications are still out — they have no per-note recipient model;
  deferred until a watchers/subscription concept exists.)

### [0.40.0] — 2026-06-16
- **Device notification channels** (Phase C; no schema change — reuses
  `notification_channels`). A new `device` channel type represents a connected
  client (browser / Android / future desktop): it carries a stable `device_id` in
  its config and is **not** sent to an external service — instead, when a routing
  rule targets it, the live WS `notification` event carries `device_targets`
  (the device ids to notify) so the matching client raises a native OS
  notification. This is **per-device**, so rules can differ per client (e.g. only
  `assigned` to the phone, everything to the browser + Telegram). Quiet hours
  suppress device targets (they can't be deferred). New `POST /notification-devices`
  auto-registers/updates a client's device channel idempotently by `device_id`.

### [0.39.0] — 2026-06-16
- **Notification digest / grouping** (Phase C, migration **0021**). A per-user
  digest window (`digest_minutes`, 0 = off) holds external deliveries and combines
  those going to the same channel within the window into **one** message (e.g.
  «Сводка — 3 уведомлений: • … • …»). Implemented on the outbox: a delivery gets a
  `digest_group` at enqueue (v1 = channel id) and a deferred `next_attempt_at`
  (max of the digest window and any quiet-hours hold, so a quiet-release burst is
  combined too); the worker groups due same-group rows and sends once. The
  `digest_group` column lets a future per-kind / per-rule grouping change the key
  without a migration. New `digest_minutes` field on `GET/PUT /notification-prefs`.

### [0.38.0] — 2026-06-16
- **Quiet hours / silence** (migration **0020**, on `notification_prefs`): a
  per-user window (start/end minutes in an IANA timezone, may wrap past midnight)
  during which **external** channel deliveries are held — at enqueue, a delivery
  for a user currently in their quiet window is scheduled (`next_attempt_at`) to
  the window's end instead of now, reusing the outbox. In-app notifications are
  unaffected. The window math (`quietWindow`) is unit-tested; verified e2e (an
  assignment during the window stayed undelivered while the in-app notice
  appeared, then delivered once disabled). New prefs fields on `GET/PUT
  /notification-prefs`.

### [0.37.0] — 2026-06-16
- **Due-date & reminder notifications** (Phase B, migration **0019**). A background
  scanner (60s) emits notifications:
  - **Due dates**: per the recipient's `notification_prefs` (lead minutes before due
    + optional repeat-every-N), with a per-task override (`due_lead_minutes` /
    `due_repeat_minutes` / `due_notify_enabled`, null = inherit). Recipients are the
    task's assignees + creator. A per-(task,user) state dedups and drives the
    repeat; it snapshots the due date it fired for, so editing the due date re-arms
    it. Bounded scan window (≤31d ahead, ≤7d overdue).
  - **Reminders**: routed to the user's channels once at `remind_at` (alongside the
    Android local alarm), gated by `reminder_enabled`.
  - Emitted notifications flow through the existing routing + outbox; the firing
    decision (`dueShouldFire`) is unit-tested.
  - New endpoints: `GET/PUT /notification-prefs` (per-user defaults),
    `PATCH /tasks/:id/due-notify` (per-task override).

### [0.36.0] — 2026-06-16
- **Go-template message templating per channel** (migration **0018**: `template`
  on `notification_channels`). Each channel can render its message from a Go
  `text/template`; empty = the built-in default (`{{.Text}}` + link). Template data
  exposes `Kind / Title / Text / TaskNumber / TaskTitle / Actor / Workspace / Link`.
  Channel create/update validate the template (length + parse against sample data);
  the test send and delivery both render through it. New
  `POST /notification-template-preview` renders a template against sample data for
  the editor's live preview (parse/field errors returned as `{ok:false,error}`).
  Senders no longer auto-append the link — the template owns the full message.

### [0.35.0] — 2026-06-16
- **shoutrrr transport integration.** Telegram is now delivered through the
  [shoutrrr](https://github.com/containrrr/shoutrrr) library (built into a
  `telegram://…` URL from the bot token + chat id), and a new generic **`shoutrrr`**
  channel type accepts any shoutrrr service URL — slack / discord / ntfy / gotify /
  matrix / pushover / teams / … — stored as an encrypted secret (the universal
  escape hatch, so new providers need no code change). The `Sender` interface is
  unchanged; email still uses the server SMTP mailer and `webhook` stays a flexible
  raw-JSON POST. shoutrrr errors are redacted (they can embed the service URL).

### [0.34.1] — 2026-06-16
- **Security: redact channel secrets from delivery errors.** The Go HTTP client
  embeds the full request URL — including the Telegram bot token (`/bot<token>/…`)
  — in its timeout/network error string, which then surfaced verbatim in the
  channel UI and `last_error`. Telegram/webhook senders now scrub the token / auth
  header from any error before it leaves the transport. (Secrets were already
  write-only at rest and never returned; this closes the error-text leak.)
- Notification subjects no longer carry a «Tessera — » prefix — the channels are
  fully app-managed, so the source is implicit (`Назначена задача`, `Новый
  комментарий`, …).

### [0.34.0] — 2026-06-16
Notification router phase A — user-configurable external notification channels +
Alertmanager-style routing. Loose-coupled like the GitLab integration (own
`internal/notify` package, own tables, channel `type` a free string not an enum,
secrets encrypted at rest via the same sealer). Migration **0017**.
- **Channels** (`/api/notification-channels`, per-user CRUD + `…/:id/test`): a
  delivery target of type `email` / `telegram` / `webhook`. Non-secret settings in
  a `config` JSONB; secrets (telegram bot token, webhook auth header) AES-256-GCM
  encrypted and never returned (only `has_secret`). The test endpoint sends a
  sample message synchronously and flips `verified` on success.
- **Routing rules** (`/api/notification-routes`, per-user CRUD): ordered rules with
  a JSONB matcher (event kinds + workspace) → a set of channel ids, or `mute`.
  First matching enabled rule wins; in-app notifications stay unconditional.
- **Dispatch + outbox**: `notify()` now also routes each created notification —
  the first matching rule enqueues a row per channel into `notification_deliveries`
  (outbox). A background worker drains the outbox (claim with `FOR UPDATE SKIP
  LOCKED`, quadratic backoff, fail after 5 attempts), so external delivery is
  async, retried and survives restart.
- **Transports**: telegram (Bot API) + generic webhook via `net/http`; email reuses
  the server SMTP mailer (no-op-logs when unconfigured). Behind a `Sender`
  interface so the Phase B provider long-tail can swap in a unified library.

### [0.33.2] — 2026-06-16
- Fix: the GitLab avatar proxy (`/api/gitlab/avatar`) no longer breaks avatars when
  the server can't fetch the image itself (gravatar egress blocked, an instance
  avatar that needs a GitLab session rather than a PAT, or a redirect). It now
  **streams** only a real image fetched directly (200, non-HTML) and otherwise
  **redirects the client to the original URL** — which a browser loads as before
  (desktop restored) and, for public/gravatar avatars, the mobile app too. Redirects
  aren't auto-followed server-side, so the owner token never leaks cross-host. No
  re-sync needed — redeploy the backend.

### [0.33.1] — 2026-06-16
- Fix: GitLab media now loads on clients without direct GitLab access (the mobile
  app). GitLab **avatars** are routed through a new signed same-origin proxy
  `GET /api/gitlab/avatar` (the server fetches the image; the integration owner's
  token is sent only when the host is the GitLab instance, never to gravatar). The
  **upload/attachment** proxy (`/api/gitlab/asset`) now streams the file
  server-side on older GitLab (<17.4) instead of redirecting to the GitLab host (a
  redirect only resolves for a browser with a GitLab session, never for the app);
  on failure it 404s so the client shows its fallback. Avatar URLs are proxied at
  sync time — a re-sync (manual or the worker) updates already-linked issues.

### [0.33.0] — 2026-06-16
User-management phase U3a (global admin panel backend). All global-admin only,
each handler re-checks `is_admin`; no migration.
- `GET /admin/users` — list every account on the instance (id, email, name,
  is_admin, active, email_verified, created_at, avatar) for the admin panel.
- `PATCH /admin/users/:id/admin` — grant/revoke the global-admin flag; you can't
  change your own (a sole admin can't lock the instance out).
- `POST /admin/users/:id/reset-link` — mint a password-reset link for any account
  and return it, so an operator can hand it over without SMTP (also emailed when
  SMTP is configured). Reuses the self-service reset token (kind `reset`, 1h TTL).
- `SetUserActive` now shares a `requireGlobalAdmin` helper with the new handlers.

### [0.32.0] — 2026-06-16
- **GitLab user avatars** captured on sync. Migration 0016 (additive):
  `gitlab_links.gl_author_avatar_url` + `task_gitlab_assignees.gl_avatar_url`.
  The issues GraphQL query now selects `avatarUrl` for the author and assignees;
  instance-relative URLs are resolved to absolute against the GitLab base, absolute
  (gravatar/external) ones pass through. Exposed as `gitlab_author_avatar_url` on
  the board card list and `gitlab.author_avatar_url` / `gitlab_assignees[].gl_avatar_url`
  on the task detail. Clients render directly and fall back to initials on miss.

### [0.31.1] — 2026-06-15
- `userDTO` (auth/me + auth responses) now includes `email_verified`, so clients can
  show verification status and a resend prompt.

### [0.31.0] — 2026-06-15
User-management phase U2a (backend account lifecycle). Migration 0015; all additive.
Email is sent through the SMTP mailer when configured, otherwise the no-op mailer
logs the full message (so a self-host without SMTP can still read the link).
- **Workspace invitations by email** (`workspace_invitations`): `POST/GET
  /workspaces/:id/invitations` + `DELETE /workspaces/:id/invitations/:invId`
  (owner/admin) — invite anyone by email, even without an account. The create
  response includes the invite `link` (for copying when SMTP is off).
  `POST /invitations/accept` joins the signed-in user (email must match); and
  registering with an invited email **auto-joins** all matching pending invites.
- **Email verification** (`user_tokens` kind=verify): a token is issued on register;
  `POST /auth/verify-email` consumes it; `POST /auth/resend-verification` re-sends.
  (Login is not gated on verification yet.)
- **Password reset** (`user_tokens` kind=reset): `POST /auth/forgot-password` (always
  200 — no account enumeration) emails a 1-hour link; `POST /auth/reset-password`
  sets the new password and revokes existing sessions.
- **Account deactivation**: `PATCH /admin/users/:id/active` (global admin only;
  can't change your own) flips `users.active`; a deactivated user can't log in.

### [0.30.0] — 2026-06-15
User-management phase U1a (backend). All additive (migration 0014).
- **Self-service profile**: `PATCH /users/me` updates display name, split legal
  name (`last/first/middle`) and business fields (`bio/company/job_title`);
  `PUT /users/me/password` changes the password after verifying the current one.
- **Preferences in DB**: `user_preferences` (1:1) holds localizing (`language,
  timezone, country, time_format, date_format, week_start`) + personalizing
  (`theme, accent, board_background`) settings, returned by `GET /auth/me` and the
  auth responses, written via `PUT /users/me/preferences`. Backs the web client's
  move of appearance out of localStorage.
- **Avatars**: `PUT/DELETE /users/me/avatar` (multipart, ≤2 MiB, PNG/JPEG/GIF/WebP)
  stored as a DB blob in `user_avatars`; served publicly at `GET /users/:id/avatar`.
- **Permission matrix enforced**: managing members, roles and workspace settings now
  requires owner/admin (was any member); `PATCH /workspaces/:id/members/:userId`
  changes a role inline (admin/member); the workspace owner can't be demoted or
  removed, and `owner` can't be granted via add/role (ownership transfer is separate).
- **Provider column** `users.provider` (`local` default) separates local accounts
  from external (GitLab) identities ahead of OAuth/SSO.
- **Email scaffold**: `internal/mail` (SMTP + no-op fallback) and SMTP/PUBLIC_URL
  config, ready for U2 invites / verification / password-reset (no live flows yet).

### [0.29.2] — 2026-06-11
- GitLab asset proxy: the uploads-by-secret API only exists in GitLab ≥ 17.4, so
  on older instances (older versions) the proxy now **redirects** to the web
  `/uploads/…` URL — the browser's own GitLab session serves the file. 17.4+ is
  still served via the PAT-authenticated API; the redirect is the fallback.

### [0.29.1] — 2026-06-11
- Fix: the GitLab asset proxy now fetches via the project **uploads API**
  (`/api/v4/projects/:id/uploads/:secret/:filename`, authenticated by the PAT)
  instead of the web `/uploads/` route, which ignored `PRIVATE-TOKEN` and
  returned the GitLab login page. Existing rewritten links keep working (only the
  upstream fetch changed); upstream error status is forwarded for debugging.

### [0.29.0] — 2026-06-11
- GitLab **attachment links** in synced descriptions/comments now resolve: the
  sync rewrites project-relative `/uploads/…` links to a signed, same-origin
  proxy (`GET /api/gitlab/asset`, public but HMAC-signed so only Tessera-minted
  links work) that streams the file from GitLab using the integration owner's
  token. No expiry — the unguessable signature is the capability, like Tessera's
  own public uploads; works for inline images too.

### [0.28.0] — 2026-06-11
- GitLab **subtask grouping**: an issue matched by a `group` rule (e.g. `M:`) now
  pulls its GitLab child items (Work Items Hierarchy widget, GraphQL) and mirrors
  each as a Tessera subtask under the parent — with its own priority, tags,
  assignees, comments and completion (closed child → done). Children are synced
  under the parent's board/column and de-duplicated from the top-level list.
  Per-issue upsert was extracted into a shared path (`syncOneIssue`) used for both
  top-level cards and children; re-parenting is reconciled on each sync.
  Best-effort: a hierarchy-query failure skips children without breaking the sync.

### [0.27.0] — 2026-06-11
- Per-user **saved board views** (migration 0013, `board_views`): `GET/POST
  /boards/:id/views` and `DELETE /views/:id` persist named snapshots of a board's
  toolbar state (config JSON, opaque to the backend) per user, so a view follows
  the user across devices. POST upserts by (board, user, name); delete is
  owner-checked.

### [0.26.0] — 2026-06-11
- GitLab sync: a **closed** issue/work item now lands in the board's done column
  and is marked complete even without a status label mapping there.

### [0.25.0] — 2026-06-11
- The GitLab label rule engine is now **generic**: `label_rules` is an ordered
  list of `{match, match_type: prefix|regex, action, …}` rules instead of the
  hardcoded status/priority/tag split. Actions: `status` (→ column via value_map),
  `priority` (→ level), `board` (→ route the task onto another board, e.g. a
  Backlog board), `tag` (keep-prefix per rule), `group` (recognised; subtask
  grouping wired later via GraphQL hierarchy), `ignore`. First matching rule wins
  per label; unmatched labels follow `default_action`. Legacy configs fall back
  to defaults until re-saved.
- Sync resolves the target board per issue (board action) with a per-board column
  cache, so a routed task lands on / moves to the mapped board.

### [0.24.0] — 2026-06-11
- GitLab due-date sync now also reads the issue's **milestone End date**: a task's
  due = the issue's own due, else the milestone's due (configurable per
  integration via `due_source`: `issue_milestone` default / `issue` / `milestone`
  / `off`; migration 0012). Applies to open and closed issues.
- **Manual due wins:** editing a linked task's due date sets `due_overridden` on
  the link, after which the sync never touches it again (priority: your manual
  date > issue due > milestone due).

### [0.23.0] — 2026-06-11
- GitLab sync now mirrors **assignees** and **comments**, representing GitLab
  users with no Tessera account (migration 0011). A GitLab assignee whose
  username is linked to a Tessera user (via their credential) becomes a real
  assignee; the rest land in `task_gitlab_assignees` (display-only). Comments
  are pulled from issue notes (system notes skipped) and upserted idempotently
  by note id, with the GitLab author denormalised. `GET /tasks/:id` and the
  board task list now include `gitlab_assignees`.
- **Mixed reconciliation:** `task_tags`/`task_assignees` gain a `source`
  (`user`|`gitlab`). Each sync rebuilds only the `gitlab`-sourced set — adding,
  recolouring and pruning to match GitLab — and never touches what you applied
  manually. So a label/assignee removed in GitLab is removed here, while your own
  tags/assignees stay.
- **Linked tasks stay synced after reassignment:** the pull now also refetches
  every already-linked issue by iid (not just issues currently assigned to me),
  so a task reassigned away from you remains on the board and reflects its new
  (external) assignee instead of going stale. Sync never deletes tasks.

### [0.22.0] — 2026-06-11
- GitLab sync now pulls **label colours** (`labels { … color }`) and applies them
  to the synced tags; tags whose GitLab label has no colour get a stable
  auto-colour derived from the name. `EnsureTag` refreshes a tag's colour on
  conflict only when a non-empty colour is supplied (never wipes one).
- GitLab sync pulls the issue **due date** and sets it on create / when the
  issue has one; a due date set manually in Tessera is preserved across re-syncs
  (sync only overrides when GitLab itself has a due date). New `UpdateTaskDueDate`.
- The board task list also returns `gitlab_author` / `gitlab_author_name` so the
  card can show the author for GitLab-synced tasks (Tessera-created tasks already
  carry `created_by`).

### [0.21.0] — 2026-06-11
- The board task list (`GET /boards/:id/tasks`) now carries `gitlab_iid` and
  `gitlab_url` (null for non-synced tasks) via a LEFT JOIN to `gitlab_links`, so
  the kanban can badge cards mirrored from GitLab and link to the source issue
  without a per-card request.

### [0.20.0] — 2026-06-11
- GitLab sync now records the **issue author** (`gitlab_links.gl_author` /
  `gl_author_name`, migration 0010) and exposes it — with the issue number and
  web URL — on `GET /tasks/:id` under a `gitlab` object. Synced tasks no longer
  set `created_by` (the author is a GitLab identity that may not be a Tessera
  user). GraphQL query extended with `author { username name }`.
- **Background auto-sync worker:** integrations gain `owner_user_id`,
  `sync_interval_sec` and `last_synced_at`. A goroutine pulls each enabled
  integration whose interval has elapsed, driven by the owner's stored
  credential (idle while every interval is 0 = manual-only). The pull engine
  was extracted from the HTTP handler into a context-based `runSync` shared by
  the manual endpoint and the worker; per-issue errors are now logged and
  skipped instead of aborting the whole run.
- `PUT /workspaces/:id/gitlab/integration` accepts `sync_interval_sec` and
  records the configuring user as the sync owner.

### [0.19.0] — 2026-06-11
- GitLab integration, phase A (pull-only): mirror the issues assigned to you
  from a self-hosted GitLab into a Tessera board. Migration 0009 adds
  `gitlab_credentials` (per-user, AES-256-GCM-encrypted PAT + resolved GitLab
  identity), `gitlab_integrations` (per-workspace: project path, target board,
  `label_rules` JSONB), and `gitlab_links` (task ↔ work-item mapping with field
  snapshots reserved for a future two-way sync).
- New `ENCRYPTION_KEY` config (fail-closed in production) feeds an AES-GCM
  sealer (`internal/secrets`) for secrets at rest.
- Endpoints: `GET/POST/DELETE /gitlab/connection` (link a PAT — validated
  against GitLab's `currentUser`), `GET/PUT /workspaces/:id/gitlab/integration`
  (config + the label rule engine), `POST /workspaces/:id/gitlab/sync` (manual
  pull). The GraphQL client (`internal/gitlab`) reads issues assigned to the
  user via `project.issues(assigneeUsername:)`; `GITLAB_INSECURE_TLS=true`
  skips cert verification for self-hosted private CAs.
- Label rule engine (`internal/gitlab/rules.go`, unit-tested): `S:`-prefixed
  labels → board column, `P:`-prefixed → native priority, everything else →
  tags (prefix kept). Tags are additive, so manually-applied scope tags are
  never clobbered. Sync is pull-only — it never writes back to GitLab.

### [0.18.0] — 2026-06-10
- Two-way relation history: `POST /tasks/:id/relations` now also logs a
  `relation` event on the referenced task (`inverseRelationKind`: blocks ⇄
  blocked_by, relates/duplicates symmetric) and broadcasts it, so the link
  shows in both tasks' activity. No reverse relation row is created (avoids an
  orphan on one-sided delete). Already running in deploy; previously uncommitted.

### [0.17.0] — 2026-06-04
- Project groups gain `icon` and `color` (migration 0008); `PATCH /groups/:id`
  now accepts them alongside the name.

### [0.16.0] — 2026-06-04
- Inline image upload for descriptions/comments: `POST /api/uploads` (authed,
  image only, ≤8 MiB) stores the file under `UPLOAD_DIR/media/<uuid>.<ext>` and
  returns `{ url }`; `GET /api/uploads/:name` serves it publicly (an `<img>`
  can't send the bearer header) with a path-traversal-safe name check and a
  long cache header.

### [0.15.0] — 2026-06-03
- Task detail (`GET /tasks/:id`) now returns each subtask with its `tag_ids` and
  `assignee_ids` (new `ListSubtasksWithMeta` query), so the UI can render a rich
  hover card per subtask.

### [0.14.0] — 2026-06-03
- Configurable "done" column per board (feature #4): `boards.done_column_id`
  replaces the hardcoded match on the column name "Готово". The completing
  column resolves to the explicitly configured one, falling back to the
  rightmost column by position when unset. New boards seed it to their
  rightmost default column. `PATCH /boards/:id/done-column` sets it (or clears
  it with `column_id: null`).
- Moving a task into the done column still auto-completes it; moving it back
  out now also reopens it (clears `completed_at`, logs a `reopened` event) —
  previously completion was one-way.
- Comment @-mentions (feature #3): `POST /tasks/:id/comments` accepts a
  `mentions` array of user ids. Each one that is a workspace member gets a
  `mention` notification; the generic "commented" notice skips already-mentioned
  users to avoid double-notifying.

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
