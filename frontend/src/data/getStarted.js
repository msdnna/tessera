// The «Начало работы» tour (#2753 → v2 #3000): the chapters a user is walked
// through, and the checklist results each chapter counts. Pure data, like
// data/whatsNew.js — the engine that walks it lives in stores/tour.js, the layer
// that draws it in components/TourOverlay.vue, the checklist in
// stores/onboarding.js + components/tour/TourChecklist.vue.
//
// v2 split the old 27 steps in a row into chapters (mockup PackageH): the
// required «Основы» (10 steps, desktop), the optional chapters started from the
// checklist on «Мои задачи», and three special ones — «Знакомство» for an account
// invited into a space that already has projects, «Основы» for the phone (info
// steps over the bottom navigation, drawn as a bottom sheet), and «Что
// изменилось» — the mini-tour for those who used v1.
//
// The wording of every step lives in the catalogue, under
// `tour.steps.<id>.{title,body,wait}`, resolved by the overlay on each render
// (#2799) — text baked in here would keep the language the tour started in.
//
// Step fields:
//   id        unique across all chapters (the catalogue key)
//   anchor    `data-tour` key or a raw CSS selector — the arrow head, the cutout
//             and the «Не видим …» timeout key off it
//   extra     more arrows (show, don't ask)
//   cut       more holes in the dim that the popover must also keep clear of
//   mode      'info' — «Понятно» (↵), «Назад» (←) to the previous info step;
//             'action' — the step ends on the user's real action
//   final     the chapter's closing step («Готово», «К чек-листу»)
//   board     the step is on a board: a chapter started from it (a checklist
//             row) opens the last board first
//   sidebar   the step points into the tree: a collapsed sidebar is opened
//   skipIf    passed over without a word when this already matches
//   item      the checklist result this step stands for: walking it in the tour
//             counts the result (stores/onboarding.js)
//   advanceOn (action steps only — see stores/tour.js for the rules)
//     click   the user clicked this anchor (true = the step's own)
//     count   more elements match than when the step opened (an entity created)
//     set     a task-modal field carries a value ([data-tour-set])
//     appear  anything matches at all (a window opened, a board came up)
//     vanish  the element was there and is gone (a window closed, by any means)
//     moved   the element changed container (drag and drop)
//
// Advancement on action steps follows the author's call (#2753): a step ends when
// the entity is actually created, not when a button was clicked — a form can be
// cancelled, and the guide must not run ahead of the user.
//
// Anchors are `data-tour="<key>"` keys or raw selectors over attributes the
// element already carries (data-testid, data-nav, kanban's data-column-name).
// tests/dt-get-started.spec.js checks every one of them against the markup.

// The card the user creates in «Создайте задачу». Columns are addressed by
// their seeded server-side names — the board was created moments ago, so it has
// the four default columns.
const NEW_CARD = '[data-column-name="К работе"] [data-testid="task-card"]'
const MOVED_CARD = '[data-column-name="В процессе"] [data-testid="task-card"]'

// ── «Основы» (desktop): project → board → task → the card → drag → open ──
const BASICS = [
  { id: 'workspaces', anchor: 'ws-switch', mode: 'info', sidebar: true },
  {
    id: 'tree-add',
    anchor: 'proj-add',
    // The «+» opens a two-item menu; the arrow reaches «Проект» once it's out.
    extra: ['menu-project'],
    mode: 'action',
    sidebar: true,
    // The project form is open — whichever way the user got there.
    advanceOn: { appear: 'project-name' },
  },
  {
    id: 'project-create',
    anchor: 'project-name',
    // The address field is manager-only; when it isn't rendered the extra arrow
    // is simply skipped — only the primary anchor gates the step.
    extra: ['project-slug'],
    // Un-dim «Создать» too, so the user isn't left staring at a greyed control.
    cut: ['project-submit'],
    mode: 'action',
    sidebar: true,
    item: 'basics.project',
    advanceOn: { count: 'project-row' },
  },
  {
    id: 'board-add',
    // Scoped to the project the user just created (`{project}` → its id).
    anchor: '[data-tour-project="{project}"] [data-tour="board-add"]',
    // The inline name field, then the new row — whichever is on screen.
    extra: ['board-name', '[data-tour-board="{board}"]'],
    mode: 'action',
    sidebar: true,
    item: 'basics.board',
    // The board is open. Enter after the name opens it on its own; a board
    // added by a click elsewhere waits for the user to open its row.
    advanceOn: { appear: '[data-tour-open-board="{board}"]' },
  },
  {
    id: 'task-create',
    board: true,
    // The row at the foot of the column shows on hover only; while the tour
    // points at it, it stays out (main.css, [data-tour-active]).
    anchor: '[data-column-name="К работе"] [data-testid="add-task-button"]',
    cut: ['[data-column-name="К работе"]'],
    mode: 'action',
    item: 'basics.task',
    advanceOn: { count: NEW_CARD },
  },
  {
    id: 'card-fields',
    board: true,
    anchor: `${NEW_CARD} [data-tour="card-priority"]`,
    extra: [
      `${NEW_CARD} [data-tour="card-due"]`,
      `${NEW_CARD} [data-tour="card-tags"]`,
      `${NEW_CARD} [data-tour="card-assignees"]`,
    ],
    mode: 'info',
    item: 'basics.fields',
  },
  {
    id: 'dnd-card',
    board: true,
    anchor: NEW_CARD,
    // Don't dim the column the card is asked to land in.
    cut: ['[data-column-name="В процессе"]'],
    mode: 'action',
    item: 'basics.move',
    // Tracked WITHOUT the column in the selector: NEW_CARD stops matching once
    // the card leaves «К работе», and an empty address never ends the step. The
    // first card in document order is the leftmost column's — the one the arrow
    // points at.
    advanceOn: {
      moved: {
        el: '[data-testid="task-card"]',
        within: '[data-column-name]',
        by: 'data-column-name',
      },
    },
  },
  {
    id: 'card-open',
    board: true,
    anchor: MOVED_CARD,
    mode: 'action',
    item: 'basics.open',
    // The task window is up — by a click, ↵ on the selected card, or a link.
    advanceOn: { appear: 'tm-close' },
  },
  {
    id: 'tm-close',
    board: true,
    anchor: 'tm-close',
    mode: 'action',
    // Closed by the button, Esc or a click outside — all the same here.
    advanceOn: { vanish: 'tm-close' },
  },
  { id: 'done', anchor: 'sb-footer', mode: 'info', final: true, sidebar: true },
]

// ── «Задача целиком»: the task window, field by field ──
const TASK = [
  {
    id: 'task-open',
    anchor: '[data-testid="task-card"]',
    mode: 'action',
    // Already inside a task — nothing to open.
    skipIf: 'tm-close',
    advanceOn: { appear: 'tm-close' },
  },
  {
    id: 'tm-due',
    anchor: 'tm-due',
    mode: 'action',
    item: 'task.due',
    advanceOn: { set: '[data-tour="tm-due"][data-tour-set]' },
  },
  {
    id: 'tm-assignees',
    anchor: 'tm-assignees',
    mode: 'action',
    item: 'task.assignees',
    advanceOn: { set: '[data-tour="tm-assignees"][data-tour-set]' },
  },
  {
    id: 'tm-priority',
    anchor: 'tm-priority',
    mode: 'action',
    item: 'task.priority',
    advanceOn: { set: '[data-tour="tm-priority"][data-tour-set]' },
  },
  {
    id: 'tm-tags',
    anchor: 'tm-tags',
    mode: 'action',
    item: 'task.tags',
    advanceOn: { set: '[data-tour="tm-tags"][data-tour-set]' },
  },
  {
    id: 'tm-description',
    anchor: 'tm-description',
    mode: 'action',
    item: 'task.description',
    advanceOn: { set: '[data-tour="tm-description"][data-tour-set]' },
  },
  {
    id: 'tm-tabs',
    anchor: 'tm-activity',
    extra: ['tm-subtasks', 'tm-links'],
    mode: 'info',
    final: true,
  },
]

// ── «Входящие»: the feed, the capture, the actions on a record ──
const INBOX = [
  {
    id: 'inbox-open',
    anchor: '[data-testid="inbox-button"]',
    mode: 'action',
    item: 'inbox.open',
    skipIf: '[data-testid="inbox-panel"]',
    advanceOn: { appear: '[data-testid="inbox-panel"]' },
  },
  { id: 'inbox-feed', anchor: '[data-testid="inbox-list"]', mode: 'info' },
  {
    id: 'inbox-capture',
    anchor: '[data-testid="inbox-add"]',
    extra: ['[data-testid="capture-input"]'],
    mode: 'action',
    item: 'inbox.capture',
    advanceOn: { count: '[data-testid="inbox-item"]' },
  },
  {
    id: 'inbox-actions',
    anchor: '[data-testid="inbox-item"]',
    mode: 'info',
    item: 'inbox.handle',
  },
  { id: 'inbox-done', anchor: '[data-testid="inbox-button"]', mode: 'info', final: true },
]

// ── «Этапы»: the selector, the strip, the stage page ──
const STAGES = [
  {
    id: 'stage-select',
    anchor: '[data-testid="stage-select"]',
    mode: 'info',
    item: 'stages.select',
  },
  {
    id: 'stage-strip',
    anchor: '[data-testid="stage-strip"]',
    mode: 'info',
    item: 'stages.strip',
  },
  {
    id: 'stage-page',
    anchor: '[data-testid="stage-strip-menu"]',
    mode: 'info',
    item: 'stages.page',
  },
  { id: 'stage-nav', anchor: '[data-nav="milestones"]', mode: 'info', sidebar: true, final: true },
]

// ── «Клавиатура и ⌘K» ──
const KEYS = [
  {
    id: 'keys-palette',
    anchor: 'ws-search',
    mode: 'action',
    item: 'keys.palette',
    advanceOn: { appear: '[data-testid="palette-input"]' },
  },
  { id: 'keys-palette-tab', anchor: '[data-testid="palette-input"]', mode: 'info' },
  {
    id: 'keys-sheet',
    anchor: '[data-testid="shortcuts-button"]',
    mode: 'action',
    item: 'keys.sheet',
    advanceOn: { appear: '[data-testid="shortcuts-sheet"]' },
  },
  { id: 'keys-sheet-read', anchor: '[data-testid="shortcuts-sheet"]', mode: 'info' },
  {
    id: 'keys-card',
    anchor: '[data-testid="task-card"]',
    mode: 'info',
    item: 'keys.card',
    final: true,
  },
]

// ── «Группировка по тегам» — the killer feature ──
const TAGS = [
  {
    id: 'tags-group',
    anchor: 'board-grouping',
    mode: 'info',
    item: 'tags.group',
  },
  { id: 'tags-columns', anchor: '[data-column-name]', mode: 'info' },
  {
    id: 'tags-window',
    anchor: 'board-actions',
    mode: 'info',
    item: 'tags.manage',
    final: true,
  },
]

// ── «Проекты и группы» ──
const PROJECTS = [
  {
    id: 'group-add',
    anchor: 'proj-add',
    extra: ['menu-group'],
    mode: 'action',
    sidebar: true,
    item: 'projects.group',
    advanceOn: { count: 'group-row' },
  },
  {
    id: 'dnd-project',
    anchor: 'project-row',
    // The group the user just made is where the project is asked to go.
    cut: ['[data-tour-group="{group}"] [data-tour="group-row"]'],
    mode: 'action',
    sidebar: true,
    item: 'projects.move',
    advanceOn: {
      // The address of the first project row is the group it sits in; a drop
      // into ANY group ends the step — this is about the tree being draggable.
      moved: {
        el: '[data-tour="project-row"]',
        within: '[data-tour-group]',
        by: 'data-tour-group',
      },
    },
  },
  {
    id: 'project-window',
    anchor: 'project-row',
    mode: 'info',
    sidebar: true,
    item: 'projects.window',
    final: true,
  },
]

// ── «GitLab» — open once a board is bound to a GitLab project ──
const GITLAB = [
  { id: 'gitlab-board', anchor: 'board-actions', mode: 'info', item: 'gitlab.open' },
  {
    id: 'gitlab-card',
    anchor: '[data-testid="task-card"]',
    mode: 'info',
    item: 'gitlab.sync',
    final: true,
  },
]

// ── «Знакомство»: invited into a space that already has projects ──
const WELCOME = [
  { id: 'welcome-mine', anchor: '[data-nav="home"]', mode: 'info', sidebar: true },
  { id: 'welcome-board', anchor: 'project-row', mode: 'info', sidebar: true },
  { id: 'welcome-inbox', anchor: '[data-testid="inbox-button"]', mode: 'info' },
  { id: 'welcome-search', anchor: 'ws-search', mode: 'info' },
  { id: 'welcome-done', anchor: 'sb-footer', mode: 'info', sidebar: true, final: true },
]

// ── «Основы» on the phone (#2976, package G): info steps over the bottom
// navigation, drawn as a bottom sheet with a cutout — no arrows ──
const MOBILE = [
  { id: 'm-nav', anchor: '[data-testid="mobile-tabbar"]', mode: 'info' },
  { id: 'm-board', anchor: '[data-testid="mobile-tab-board"]', mode: 'info' },
  { id: 'm-fab', anchor: '[data-testid="quick-fab-button"]', mode: 'info' },
  { id: 'm-inbox', anchor: '[data-testid="mobile-tab-inbox"]', mode: 'info' },
  { id: 'm-search', anchor: '[data-testid="mobile-tab-search"]', mode: 'info' },
  { id: 'm-done', anchor: '[data-testid="mobile-tab-more"]', mode: 'info', final: true },
]

// ── «Что изменилось»: the mini-tour for v1 users (WhatsNewV2) ──
// Every step is info, and every one mirrors a «new» spotlight (data/hints.js,
// `hint`): walking the mini-tour marks those hints as shown.
const WHATS_CHANGED = [
  { id: 'wc-inbox', anchor: '[data-testid="inbox-button"]', mode: 'info', hint: 'inbox' },
  { id: 'wc-capture', anchor: '[data-testid="quick-fab-button"]', mode: 'info', hint: 'capture' },
  { id: 'wc-stage', anchor: '[data-testid="stage-select"]', mode: 'info', hint: 'stage-menu' },
  {
    id: 'wc-view',
    anchor: '[data-testid="board-customize-btn"]',
    mode: 'info',
    hint: 'view-options',
  },
  { id: 'wc-project', anchor: 'crumb-project', mode: 'info', hint: 'project-settings' },
  { id: 'wc-palette', anchor: 'ws-search', mode: 'info', hint: 'palette', final: true },
]

// Chapters, in the checklist's order. `optional` ones are listed under «Главы
// по выбору»; `special` ones are never listed (they start on their own).
// `needs: 'board'` — the chapter's anchors live on a board, so starting it
// opens the last board first; `needs: 'gitlab'` — locked until one is bound.
export const CHAPTERS = [
  { id: 'basics', icon: 'SparklesOutline', required: true, steps: BASICS },
  { id: 'task', icon: 'DocumentTextOutline', optional: true, needs: 'board', steps: TASK },
  { id: 'inbox', icon: 'FileTrayOutline', optional: true, steps: INBOX },
  { id: 'stages', icon: 'FlagOutline', optional: true, needs: 'board', steps: STAGES },
  { id: 'keys', icon: 'KeypadOutline', optional: true, steps: KEYS },
  { id: 'tags', icon: 'PricetagsOutline', optional: true, needs: 'board', steps: TAGS },
  { id: 'projects', icon: 'FolderOutline', optional: true, steps: PROJECTS },
  {
    id: 'gitlab',
    icon: 'GitBranchOutline',
    optional: true,
    needs: 'board',
    locked: 'gitlab',
    steps: GITLAB,
  },
  { id: 'welcome', icon: 'PeopleOutline', special: true, steps: WELCOME },
  { id: 'mobile', icon: 'PhonePortraitOutline', special: true, mobile: true, steps: MOBILE },
  { id: 'whatschanged', icon: 'GiftOutline', special: true, needs: 'board', steps: WHATS_CHANGED },
]

export const chapterById = (id) => CHAPTERS.find((c) => c.id === id) || null
export const OPTIONAL_CHAPTERS = CHAPTERS.filter((c) => c.optional)

// Checklist results: what the checklist counts, per chapter. A result is
// counted by the same event as the tour step (`item` above) — with the tour or
// without it (stores/onboarding.js maps the user's own events onto these).
// «Основы» has 6 results for 10 steps: a result is something done, not a step.
export const ITEMS = {
  basics: [
    'basics.project',
    'basics.board',
    'basics.task',
    'basics.fields',
    'basics.move',
    'basics.open',
  ],
  task: ['task.due', 'task.assignees', 'task.priority', 'task.tags', 'task.description'],
  inbox: ['inbox.open', 'inbox.capture', 'inbox.handle'],
  stages: ['stages.select', 'stages.strip', 'stages.page'],
  keys: ['keys.palette', 'keys.sheet', 'keys.card'],
  tags: ['tags.group', 'tags.manage'],
  projects: ['projects.group', 'projects.move', 'projects.window'],
  gitlab: ['gitlab.open', 'gitlab.sync'],
}

// The tour step that counts a checklist result.
export function stepOfItem(itemId) {
  for (const ch of CHAPTERS) {
    const s = ch.steps.find((x) => x.item === itemId)
    if (s) return { chapter: ch.id, step: s.id }
  }
  return null
}

// Where a checklist row starts the tour: the counting step, or earlier when the
// action steps right before it lead up to it and count nothing of their own —
// «Создать проект» starts at «+» over the tree, not inside a form that isn't
// open (#3000).
export function entryOfItem(itemId) {
  const at = stepOfItem(itemId)
  if (!at) return null
  const steps = chapterById(at.chapter).steps
  let i = steps.findIndex((s) => s.id === at.step)
  while (i > 0 && steps[i - 1].mode === 'action' && !steps[i - 1].item) i--
  return { chapter: at.chapter, step: steps[i].id }
}

// Back-compat: the old flat scenario is «Основы».
export const GET_STARTED = BASICS
