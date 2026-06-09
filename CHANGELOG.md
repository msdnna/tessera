# Changelog

All notable changes to Tessera are documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/), versions per service.

## frontend

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
