<script setup>
import { ref, reactive, computed, watch, onMounted, onBeforeUnmount, nextTick, h } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import draggable from 'vuedraggable'
import {
  NSpin,
  NButton,
  NInput,
  NText,
  NPopover,
  NIcon,
  NTooltip,
  NDropdown,
  NPopconfirm,
  useMessage,
} from 'naive-ui'
import {
  GitNetworkOutline,
  SaveOutline,
  FolderOpenOutline,
  TrashOutline,
  AddOutline,
  ChevronForwardOutline,
  ChevronBackOutline,
  SettingsOutline,
  RibbonOutline,
  CloseOutline,
  ArchiveOutline,
} from '@vicons/ionicons5'
import {
  boards,
  tasks as tasksApi,
  workspaces as wsApi,
  columns as columnsApi,
  projects as projectsApi,
  gitlab as gitlabApi,
} from '@/api'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useBoardViewStore } from '@/stores/boardView'
import { useThemeStore } from '@/stores/theme'
import { useAuthStore } from '@/stores/auth'
import { useRealtime } from '@/composables/useRealtime'
import { useResponsive } from '@/composables/useResponsive'
import { PRIORITY_LABELS } from '@/styles/tokens'
import { tagNamespace, prefixLabel, buildTagGroups } from '@/utils/tagGroups'
import { sumEstimates, formatEstimate } from '@/utils/estimation'
import { storeToRefs } from 'pinia'
import TaskCard from './TaskCard.vue'
import TaskModal from './TaskModal.vue'
import BoardActivityToasts from './BoardActivityToasts.vue'
import TesseraSpinner from './TesseraSpinner.vue'
import ColumnHeader from './ColumnHeader.vue'
import BoardCustomizePanel from './BoardCustomizePanel.vue'
import BoardListView from './BoardListView.vue'
import BoardCalendarView from './BoardCalendarView.vue'
import BoardMatrixView from './BoardMatrixView.vue'
import BoardTimelineView from './BoardTimelineView.vue'
import BoardGanttView from './BoardGanttView.vue'

const props = defineProps({ boardId: { type: String, required: true } })

const message = useMessage()
const auth = useAuthStore()
const wsStore = useWorkspacesStore()
const boardViewStore = useBoardViewStore()
// Live board-activity toasts (separate from the bell): fed from realtime events.
const activityToasts = ref(null)
// `layout` lives in the store so the header switcher and the board stay in sync.
const { layout } = storeToRefs(boardViewStore)
// Timeline + Gantt share the same time-axis substrate (swimlanes, status sort/
// filter, assignee/none grouping) — gate those facets on either.
const timelineLike = computed(() => layout.value === 'timeline' || layout.value === 'gantt')
const route = useRoute()
const router = useRouter()
const { isMobile } = useResponsive()

const loading = ref(false)
const board = ref(null)
const columns = ref([])
const allTasks = ref([])
const subtasksByParent = ref({})
const lists = ref({})
// ── card-list virtualization (IntersectionObserver windowing) ──────────────
// Every column item keeps its wrapper <div> so vuedraggable's child count,
// indices, drop targets and the before/after math in onColChange stay identical
// (DnD untouched). Cards more than ~800px outside the viewport collapse to a
// cheap placeholder of their last-measured height; only near-viewport cards
// mount the heavy TaskCard. Visibility is driven by each card's *real* viewport
// position (one IO, root = viewport), so there's no model-vs-DOM divergence to
// thrash the scrollbar and the bottom is always reachable. Parity with Android's
// LazyColumn. Measuring before collapsing keeps placeholder height exact → no
// jump. IO swaps are frozen during a drag so SortableJS sees a stable DOM.
const VCARD_EST = 190 // placeholder px until a card has been measured
const vis = reactive({}) // task id → in/near viewport (undefined = not yet known)
const cardH = reactive({}) // task id → last measured px (from the rendered card)
let cardIO = null // toggles visibility by real viewport position
let cardRO = null // measures rendered cards (settles after content layout)
const tagsMap = reactive({})
const membersMap = reactive({})
// GitLab project members (assignable on integration boards even without a Tessera
// account), keyed by gl_user_id; empty for non-integration boards.
const gitlabMembersMap = reactive({})
// True when this board is the workspace's GitLab integration board and the
// integration allows creating issues from tasks (writeback.push_create) — gates the
// "Создать issue в GitLab" action in the task modal.
const gitlabCanCreate = ref(false)
const gitlabFetchTemplates = ref(false)
const gitlabIntegrationId = ref(null)
const tagsList = computed(() => Object.values(tagsMap))
const membersList = computed(() => Object.values(membersMap))
// Project milestones («Этап»), keyed by id; cards/modal resolve a task's milestone_id.
const milestonesMap = reactive({})
const milestonesList = computed(() => Object.values(milestonesMap))
// GitLab roster minus members that already map to a Tessera user in this workspace
// (they appear in the Tessera member list instead — avoids showing one person twice).
const gitlabMembersList = computed(() =>
  Object.values(gitlabMembersMap).filter((g) => !(g.tessera_user_id && membersMap[g.tessera_user_id])),
)

// view controls (layout comes from the store, above)
const subtasksExpanded = ref(false) // full property cards vs compact rows
// "Авто" (Gantt only): order rows by the blocking-dependency graph. The toggle
// resets the composer to no-group/no-sort; `autoActive` below stays on only
// while it remains in that state, so adding any grouping/sort silently exits it.
const autoSort = ref(false)

// ── column collapse + card/board customization (bound to the saved view) ──
// colCollapse stores EXPLICIT per-column overrides (key → true collapsed / false
// expanded). Absence means "follow the auto rule". A reactive object (not a Set)
// so Vue's deep watch tracks it and an auto-collapsed empty column can still be
// manually expanded (an explicit `false` wins over autoCollapseEmpty).
const colCollapse = reactive({})
const autoCollapseEmpty = ref(false)
const cardSize = ref('medium') // 'compact' | 'medium' | 'large'
const stackFields = ref(false) // pills stacked vertically vs. horizontal wrap
const showEmpty = ref(true) // render empty (unset) placeholder pills
const autosaveView = ref(false) // auto re-save the loaded named view on change
function defaultFieldVis() {
  return {
    priority: true,
    due: true,
    assignee: true,
    tags: true,
    estimate: true,
    milestone: true,
    description: true,
    number: true,
    gitlab: true,
  }
}
const fieldVis = reactive(defaultFieldVis())
const customizeOpen = ref(false)
// Board-name mirror for the customize panel's rename input (kept in sync with the
// loaded board; committed via boards.update on blur/enter).
const boardName = ref('')
watch(board, (b) => (boardName.value = b?.name || ''))
// Save board meta (name / icon / colour / icon_mode) from the customize panel. We
// always send the full current meta so an icon change doesn't drop the name and
// vice-versa; the backend still merges tri-state, this just keeps it simple.
async function updateBoard(patch) {
  if (!board.value) return
  const name = (patch.name ?? board.value.name ?? '').trim()
  if (!name) {
    boardName.value = board.value.name
    return
  }
  try {
    const { data } = await boards.update(board.value.id, {
      name,
      icon: patch.icon ?? board.value.icon ?? '',
      color: patch.color ?? board.value.color ?? '',
      icon_mode: patch.icon_mode ?? board.value.icon_mode ?? 'badge',
    })
    board.value = data
    boardName.value = data.name
    wsStore.upsertBoard(data) // reflect rename/icon/colour in the sidebar tree
  } catch (e) {
    message.error(e.message)
    boardName.value = board.value.name
  }
}
function setFieldVis(key, val) {
  fieldVis[key] = val
}

function colCount(dcol) {
  return (lists.value[dcol.key] || []).length
}
// Effective collapsed state: explicit override wins, else the auto-empty rule.
function isColCollapsed(dcol) {
  const k = dcol.key
  if (k in colCollapse) return colCollapse[k]
  return autoCollapseEmpty.value && colCount(dcol) === 0
}
// Collapsed columns stay collapsed even during a drag (the layout doesn't jump —
// the freed width is already redistributed to the expanded columns). Dropping a
// card ONTO a collapsed column works via a full-column drop overlay revealed while
// dragging (see `.cols.dragging .col.collapsed .drop` + onColChange's top-insert).
function colCollapsedNow(dcol) {
  return isColCollapsed(dcol)
}
function toggleCollapse(dcol) {
  colCollapse[dcol.key] = !isColCollapsed(dcol)
}
// Turning "auto-collapse empty" ON must collapse EVERY empty column — including
// ones the user previously expanded by hand (a stale explicit `false` override
// would otherwise beat the auto rule, which reads as "the toggle does nothing").
// Clear overrides on empty columns whenever the toggle flips on.
watch(autoCollapseEmpty, (on) => {
  if (!on) return
  for (const dcol of displayColumns.value) {
    if (colCount(dcol) === 0 && dcol.key in colCollapse) delete colCollapse[dcol.key]
  }
})
// Composer bar: collapsed to a single row (clipping overflow chips) so the
// right-side tool buttons stay in view; tapping expands it to full height and
// slides the tools off-screen. Collapses again on an outside click.
const composerExpanded = ref(false)
const subbarEl = ref(null)
function expandComposer() {
  composerExpanded.value = true
}
function onDocPointerDown(e) {
  if (!composerExpanded.value) return
  const t = e.target
  if (subbarEl.value?.contains(t)) return
  // Ignore clicks inside teleported menus (chip dropdowns / view popovers).
  if (t.closest?.('.n-dropdown-menu, .n-popover, .n-popselect, .n-base-select-menu')) return
  composerExpanded.value = false
}
const groupMode = ref('status') // 'status' | 'tag'
const tagPrefix = ref('') // when grouping by tag: only tags with this namespace prefix become columns
// Friendly display names for tag prefixes (canonical prefix → label), loaded
// per-project. Falls back to the raw prefix where no name is configured.
const tagPrefixNames = reactive({})

// Detected namespaces from the project tags, for the prefix picker. Labels use
// the configured friendly name (else the raw prefix), sorted alphabetically.
const tagPrefixOptions = computed(() => {
  const set = new Set()
  for (const t of tagsList.value) {
    const ns = tagNamespace(t.name)
    if (ns) set.add(ns)
  }
  return [
    { label: 'Все теги', value: '' },
    ...[...set]
      .map((p) => ({ label: prefixLabel(p, tagPrefixNames), value: p }))
      .sort((a, b) => a.label.localeCompare(b.label, 'ru')),
  ]
})
// Tags that become columns in tag-grouping mode (filtered by the chosen prefix).
const groupTags = computed(() =>
  tagPrefix.value
    ? tagsList.value.filter((t) => (t.name || '').startsWith(tagPrefix.value))
    : tagsList.value,
)
// Multi-level sort: an ordered list of { field, dir }. Empty = manual order.
const sortLevels = ref([])
const filters = reactive({ priorities: [], assignees: [], tags: [], statuses: [], milestones: [], due: '', q: '' })

// Archive scope: ?archived=1 shows the board's archived tasks read-only (no DnD, no
// create, no inline edits) with a Restore action. Reuses all board filters/grouping.
const archivedMode = computed(() => route.query.archived === '1')
function exitArchive() {
  const q = { ...route.query }
  delete q.archived
  router.replace({ query: q })
}

// Sprint scope (navigation overlay): ?milestone=<uuid|backlog>. Drives the
// server-side task scope and shows a removable chip; clearing it returns the full
// board (and de-highlights the sidebar sprint node).
const milestoneScope = computed(() => (route.query.milestone ? String(route.query.milestone) : ''))
const milestoneScopeLabel = computed(() => {
  const s = milestoneScope.value
  if (!s) return ''
  if (s === 'backlog') return 'Бэклог'
  return milestonesMap[s]?.title || 'Этап'
})
function clearMilestoneScope() {
  const q = { ...route.query }
  delete q.milestone
  router.replace({ query: q })
}
const sortFieldOptions = [
  { label: 'Приоритет', value: 'priority' },
  { label: 'Срок', value: 'due' },
  { label: 'Этап', value: 'milestone' },
  { label: 'Статус', value: 'status' },
  { label: 'Название', value: 'title' },
  { label: 'Номер', value: 'number' },
]
// Milestone sort order: by the milestone's due date (none last), then its title.
const milestoneSortKey = (t) => {
  const m = t.milestone_id ? milestonesMap[t.milestone_id] : null
  if (!m) return { d: Number.POSITIVE_INFINITY, s: '' }
  return { d: m.due_date ? Date.parse(m.due_date) : Number.POSITIVE_INFINITY, s: m.title || '' }
}
// Status sort/filter is offered only on the timeline for now (the board already
// groups by status into columns, so sorting/filtering by it there is redundant).
const sortFieldsForMenu = computed(() =>
  timelineLike.value ? sortFieldOptions : sortFieldOptions.filter((o) => o.value !== 'status'),
)
// column id → position, for the status sort.
const colPos = computed(() => {
  const m = {}
  columns.value.forEach((c) => (m[c.id] = c.position))
  return m
})
// One sort level's comparison (direction applied; due-less tasks always last).
function cmpLevel(a, b, { field, dir }) {
  const d = dir === 'desc' ? -1 : 1
  if (field === 'status') return d * ((colPos.value[a.column_id] ?? 0) - (colPos.value[b.column_id] ?? 0))
  if (field === 'due') {
    const av = a.due_date ? Date.parse(a.due_date) : null
    const bv = b.due_date ? Date.parse(b.due_date) : null
    if (av === null && bv === null) return 0
    if (av === null) return 1
    if (bv === null) return -1
    return d * (av - bv)
  }
  if (field === 'priority') return d * ((a.priority || 0) - (b.priority || 0))
  if (field === 'milestone') {
    const ka = milestoneSortKey(a)
    const kb = milestoneSortKey(b)
    if (ka.d !== kb.d) return d * (ka.d - kb.d)
    return d * ka.s.localeCompare(kb.s, 'ru')
  }
  if (field === 'title') return d * String(a.title || '').localeCompare(String(b.title || ''), 'ru')
  if (field === 'number') return d * ((a.number || 0) - (b.number || 0))
  return 0
}
const dueOptions = [
  { label: 'Все', value: '' },
  { label: 'Просроченные', value: 'overdue' },
  { label: 'Сегодня', value: 'today' },
  { label: 'Ближайшая неделя', value: 'week' },
  { label: 'Со сроком', value: 'has' },
  { label: 'Без срока', value: 'none' },
]
const priorityFilterOptions = PRIORITY_LABELS.map((label, value) => ({ label, value }))
const memberFilterOptions = computed(() => [
  ...membersList.value.map((m) => ({ label: m.name, value: m.user_id })),
  // GitLab-only assignees (no Tessera account) — value prefixed `gl:` so the filter
  // matches against gitlab_assignee_logins.
  ...gitlabMembersList.value.map((g) => ({
    label: `${g.gl_name || g.gl_username} (GitLab)`,
    value: `gl:${g.gl_username}`,
  })),
])
// Tag filter menu, grouped by prefix (friendly names). Naive `type:'group'`
// renders inline section headers — works on desktop and the mobile drill alike.
// A single prefix-less bucket stays flat (no redundant header).
const tagFilterMenu = computed(() => {
  const groups = buildTagGroups(tagsList.value, tagPrefixNames)
  if (groups.length <= 1) {
    return (groups[0]?.tags || []).map((t) => ({ label: t.name, key: `ft.${t.id}` }))
  }
  return groups.map((g) => ({
    type: 'group',
    label: g.label,
    key: `ftg.${g.key}`,
    children: g.tags.map((t) => ({ label: t.name, key: `ft.${t.id}` })),
  }))
})
// Status filter = which board columns to show (timeline-only facet).
const statusFilterOptions = computed(() => columns.value.map((c) => ({ label: c.name, value: c.id })))
// Milestone filter menu (+ an explicit "Без этапа" bucket).
const milestoneFilterMenu = computed(() => [
  ...milestonesList.value.map((m) => ({ label: m.title, key: `fm.${m.id}` })),
  { label: 'Без этапа', key: 'fm.__none__' },
])
const activeFilterCount = computed(
  () =>
    filters.priorities.length +
    filters.assignees.length +
    filters.tags.length +
    filters.statuses.length +
    filters.milestones.length +
    (filters.due ? 1 : 0) +
    (filters.q.trim() ? 1 : 0),
)
function resetFilters() {
  filters.priorities = []
  filters.assignees = []
  filters.tags = []
  filters.statuses = []
  filters.milestones = []
  filters.due = ''
  filters.q = ''
}

// ── composer bar: grouping + sort + filters as removable chips ──
// All facets render as chips over the existing state; an "add" dropdown mutates
// the same refs. The search box lives in the bar too (filters.q).
const facetChips = computed(() => {
  const out = []
  out.push({ kind: 'group', label: `Группировка: ${groupModeLabel.value}` })
  sortLevels.value.forEach((l, i) => {
    const f = sortFieldOptions.find((o) => o.value === l.field)?.label || l.field
    out.push({ kind: 'sort', i, label: `Сорт: ${f} ${l.dir === 'desc' ? '↓' : '↑'}` })
  })
  filters.priorities.forEach((p) =>
    out.push({ kind: 'priority', value: p, label: `Приоритет: ${PRIORITY_LABELS[p]}` }),
  )
  filters.assignees.forEach((a) => {
    let name
    if (typeof a === 'string' && a.startsWith('gl:')) {
      const u = a.slice(3)
      const g = gitlabMembersList.value.find((x) => x.gl_username === u)
      name = (g && (g.gl_name || g.gl_username)) || u
    } else {
      name = membersMap[a]?.name || '—'
    }
    out.push({ kind: 'assignee', value: a, label: `Исполнитель: ${name}` })
  })
  filters.tags.forEach((t) =>
    out.push({ kind: 'tag', value: t, label: `Тег: ${tagsMap[t]?.name || '—'}` }),
  )
  filters.statuses.forEach((s) =>
    out.push({ kind: 'status', value: s, label: `Статус: ${columns.value.find((c) => c.id === s)?.name || '—'}` }),
  )
  filters.milestones.forEach((m) =>
    out.push({
      kind: 'milestone',
      value: m,
      label: `Этап: ${m === '__none__' ? 'без этапа' : milestonesMap[m]?.title || '—'}`,
    }),
  )
  if (filters.due) {
    out.push({ kind: 'due', label: `Срок: ${dueOptions.find((o) => o.value === filters.due)?.label || filters.due}` })
  }
  return out
})
// Friendly label for the current grouping (status / tag[·prefix] / assignee / none).
const groupModeLabel = computed(() => {
  if (groupMode.value === 'assignee') return 'исполнитель'
  if (groupMode.value === 'none') return 'без группировки'
  if (groupMode.value === 'milestone') return 'этапы'
  if (groupMode.value === 'tag')
    return `теги${tagPrefix.value ? ` · ${prefixLabel(tagPrefix.value, tagPrefixNames)}` : ''}`
  return 'статусы'
})
const addOptions = computed(() => {
  const grouping = [
    { label: 'По статусам', key: 'g.status' },
    { label: 'По тегам (все)', key: 'g.tag' },
    ...tagPrefixOptions.value
      .filter((o) => o.value)
      .map((o) => ({ label: `По тегам · ${o.label}`, key: `g.tagp.${encodeURIComponent(o.value)}` })),
    { label: 'По этапам', key: 'g.milestone' },
  ]
  // Timeline swimlanes can also be per-assignee or ungrouped.
  if (timelineLike.value) {
    grouping.push({ label: 'По исполнителю', key: 'g.assignee' }, { label: 'Без группировки', key: 'g.none' })
  }
  const opts = [
    { label: 'Группировка', key: 'group', children: grouping },
    {
      label: 'Сортировка',
      key: 'sort',
      children: sortFieldsForMenu.value.map((o) => ({ label: o.label, key: `s.${o.value}` })),
    },
    {
      label: 'Фильтр: приоритет',
      key: 'fp',
      children: priorityFilterOptions.map((o) => ({ label: o.label, key: `fp.${o.value}` })),
    },
    {
      label: 'Фильтр: исполнитель',
      key: 'fa',
      children: memberFilterOptions.value.map((o) => ({ label: o.label, key: `fa.${o.value}` })),
    },
    { label: 'Фильтр: тег', key: 'ft', children: tagFilterMenu.value },
    { label: 'Фильтр: этап', key: 'fm', children: milestoneFilterMenu.value },
    {
      label: 'Фильтр: срок',
      key: 'fd',
      children: dueOptions.filter((o) => o.value).map((o) => ({ label: o.label, key: `fd.${o.value}` })),
    },
  ]
  // Status (column) filter — timeline only, so the user can hide e.g. the «done»
  // column's completed cards that otherwise crowd the chart.
  if (timelineLike.value) {
    opts.splice(2, 0, {
      label: 'Фильтр: статус',
      key: 'fs',
      children: statusFilterOptions.value.map((o) => ({ label: o.label, key: `fs.${o.value}` })),
    })
  }
  return opts
})
// Mobile: the "+" menu drills into one sub-list at a time (with a «Назад») rather
// than fanning out side submenus that run off a narrow screen.
const addShow = ref(false)
const addLevel = ref(null) // null = top level; else the parent group's key
const addDir = ref('down') // slide direction for the level transition ('down' = deeper)
const addMenuOptions = computed(() => {
  if (!isMobile.value) return addOptions.value
  if (!addLevel.value) {
    // Top level as drill-in entries (no side submenu).
    return addOptions.value.map((o) => ({ label: o.label, key: `nav.${o.key}` }))
  }
  const parent = addOptions.value.find((o) => o.key === addLevel.value)
  return [
    { label: 'Назад', key: 'nav.back' },
    { type: 'divider', key: 'nav.div' },
    ...(parent?.children || []),
  ]
})
// Custom row rendering on mobile: right-side drill arrows, a spaced «‹ Назад»
// header, and a vertical slide that plays whenever the level changes (keys differ
// between levels → the option nodes remount → the animation replays).
function renderAddLabel(option) {
  if (!isMobile.value) return option.label
  const anim = `t-fdd-${addDir.value} .17s ease`
  const key = option.key
  if (key === 'nav.back') {
    return h('span', { class: 't-fdd-row is-back', style: { animation: anim } }, [
      h(NIcon, { component: ChevronBackOutline, size: 16 }),
      h('span', { class: 't-fdd-txt' }, option.label),
    ])
  }
  if (!addLevel.value && typeof key === 'string' && key.startsWith('nav.')) {
    return h('span', { class: 't-fdd-row is-drill', style: { animation: anim } }, [
      h('span', { class: 't-fdd-txt' }, option.label),
      h(NIcon, { component: ChevronForwardOutline, size: 14, class: 't-fdd-arr' }),
    ])
  }
  return h('span', { class: 't-fdd-row', style: { animation: anim } }, [
    h('span', { class: 't-fdd-txt' }, option.label),
  ])
}
// n-dropdown auto-closes on every select. While drilling we ignore that close
// (keep the controlled menu open) so the sub-list shows instead of snapping shut.
let addDrilling = false
function onAddSelect(key) {
  if (key === 'nav.back' || key.startsWith('nav.')) {
    addDir.value = key === 'nav.back' ? 'up' : 'down'
    addLevel.value = key === 'nav.back' ? null : key.slice(4)
    addDrilling = true
    nextTick(() => (addDrilling = false))
    return
  }
  onAddFacet(key)
  addShow.value = false
  addLevel.value = null
}
function onAddShow(v) {
  if (!v && addDrilling) return // swallow the select-triggered close mid-drill
  addShow.value = v
  if (!v) addLevel.value = null // reset drill state when the menu actually closes
}
function onAddFacet(key) {
  if (key === 'g.status') {
    groupMode.value = 'status'
    tagPrefix.value = ''
  } else if (key === 'g.tag') {
    groupMode.value = 'tag'
    tagPrefix.value = ''
  } else if (key.startsWith('g.tagp.')) {
    groupMode.value = 'tag'
    tagPrefix.value = decodeURIComponent(key.slice('g.tagp.'.length))
  } else if (key === 'g.milestone') {
    groupMode.value = 'milestone'
    tagPrefix.value = ''
  } else if (key === 'g.assignee') {
    groupMode.value = 'assignee'
    tagPrefix.value = ''
  } else if (key === 'g.none') {
    groupMode.value = 'none'
    tagPrefix.value = ''
  } else if (key.startsWith('s.')) {
    const f = key.slice(2)
    if (!sortLevels.value.some((l) => l.field === f)) sortLevels.value.push({ field: f, dir: 'asc' })
  } else if (key.startsWith('fp.')) {
    const v = Number(key.slice(3))
    if (!filters.priorities.includes(v)) filters.priorities.push(v)
  } else if (key.startsWith('fa.')) {
    const v = key.slice(3)
    if (!filters.assignees.includes(v)) filters.assignees.push(v)
  } else if (key.startsWith('ft.')) {
    const v = key.slice(3)
    if (!filters.tags.includes(v)) filters.tags.push(v)
  } else if (key.startsWith('fs.')) {
    const v = key.slice(3)
    if (!filters.statuses.includes(v)) filters.statuses.push(v)
  } else if (key.startsWith('fm.')) {
    const v = key.slice(3)
    if (!filters.milestones.includes(v)) filters.milestones.push(v)
    // Building a custom multi-sprint filter supersedes the tree's single-sprint
    // scope — drop it so the full board loads and the client filter applies.
    if (route.query.milestone) clearMilestoneScope()
  } else if (key.startsWith('fd.')) {
    filters.due = key.slice(3)
  }
}
function removeChip(c) {
  if (c.kind === 'sort') sortLevels.value.splice(c.i, 1)
  else if (c.kind === 'priority') filters.priorities = filters.priorities.filter((x) => x !== c.value)
  else if (c.kind === 'assignee') filters.assignees = filters.assignees.filter((x) => x !== c.value)
  else if (c.kind === 'tag') filters.tags = filters.tags.filter((x) => x !== c.value)
  else if (c.kind === 'status') filters.statuses = filters.statuses.filter((x) => x !== c.value)
  else if (c.kind === 'milestone') filters.milestones = filters.milestones.filter((x) => x !== c.value)
  else if (c.kind === 'due') filters.due = ''
}
function onChipClick(c) {
  if (c.kind === 'group') {
    groupMode.value = groupMode.value === 'status' ? 'tag' : 'status'
  } else if (c.kind === 'sort') {
    const l = sortLevels.value[c.i]
    l.dir = l.dir === 'desc' ? 'asc' : 'desc'
  }
}
const hasClearableFacets = computed(
  () => sortLevels.value.length > 0 || activeFilterCount.value > 0,
)
function clearAll() {
  resetFilters()
  sortLevels.value = []
}

// "Авто" dependency-graph ordering — Gantt only. Active only while the composer
// is in its no-group/no-sort state, so any manual grouping/sort transparently
// turns it off (and removing them brings it back). The button enters that state.
const autoActive = computed(
  () =>
    layout.value === 'gantt' &&
    autoSort.value &&
    groupMode.value === 'none' &&
    sortLevels.value.length === 0,
)
function toggleAuto() {
  if (autoActive.value) {
    autoSort.value = false
    return
  }
  // Reset the composer (and any loaded view) to the bare auto-sort state.
  resetFilters()
  groupMode.value = 'none'
  tagPrefix.value = ''
  sortLevels.value = []
  currentViewName.value = ''
  autoSort.value = true
}

// ── per-board, per-layout toolbar state (localStorage, per device) ──
// Group/sort/filter state is kept independently per layout: switching board↔
// timeline swaps the live refs in and out of `toolbarByLayout`, so each layout
// remembers its own grouping/sort/filters (a status filter set on the timeline
// doesn't leak into the board, where that facet isn't even offered).
const viewKey = computed(() => `tessera_view_${props.boardId}`)
let restoring = false
let swapping = false
const toolbarByLayout = {}

function defaultToolbar(forLayout) {
  return {
    groupMode: forLayout === 'timeline' || forLayout === 'gantt' ? 'assignee' : 'status',
    tagPrefix: '',
    sortLevels: [],
    subtasksExpanded: false,
    autoSort: false,
    filters: { priorities: [], assignees: [], tags: [], statuses: [], milestones: [], due: '', q: '' },
    colCollapse: {},
    autoCollapseEmpty: false,
    cardSize: 'medium',
    stackFields: false,
    showEmpty: true,
    fieldVis: defaultFieldVis(),
    autosaveView: false,
  }
}
function snapshotToolbar() {
  return {
    groupMode: groupMode.value,
    tagPrefix: tagPrefix.value,
    sortLevels: sortLevels.value.map((l) => ({ ...l })),
    subtasksExpanded: subtasksExpanded.value,
    autoSort: autoSort.value,
    filters: {
      priorities: [...filters.priorities],
      assignees: [...filters.assignees],
      tags: [...filters.tags],
      statuses: [...filters.statuses],
      milestones: [...filters.milestones],
      due: filters.due,
      q: filters.q,
    },
    colCollapse: { ...colCollapse },
    autoCollapseEmpty: autoCollapseEmpty.value,
    cardSize: cardSize.value,
    stackFields: stackFields.value,
    showEmpty: showEmpty.value,
    fieldVis: { ...fieldVis },
    autosaveView: autosaveView.value,
  }
}
function loadToolbar(s) {
  groupMode.value = s.groupMode || 'status'
  tagPrefix.value = s.tagPrefix || ''
  sortLevels.value = (s.sortLevels || []).map((l) => ({ ...l }))
  subtasksExpanded.value = !!s.subtasksExpanded
  autoSort.value = !!s.autoSort
  Object.assign(
    filters,
    { priorities: [], assignees: [], tags: [], statuses: [], milestones: [], due: '', q: '' },
    s.filters || {},
  )
  loadCustomize(s)
}
// Restore the customize-view state (collapse + card/field settings). Split out so
// both loadToolbar and applyViewConfig share it; every key defaults so older
// saved views / localStorage blobs missing them stay valid (back-compat).
function loadCustomize(s) {
  for (const k of Object.keys(colCollapse)) delete colCollapse[k]
  Object.assign(colCollapse, s.colCollapse || {})
  autoCollapseEmpty.value = !!s.autoCollapseEmpty
  cardSize.value = s.cardSize || 'medium'
  stackFields.value = !!s.stackFields
  showEmpty.value = s.showEmpty !== false
  Object.assign(fieldVis, defaultFieldVis(), s.fieldVis || {})
  autosaveView.value = !!s.autosaveView
}

function writeView() {
  if (restoring) return
  try {
    toolbarByLayout[layout.value] = snapshotToolbar()
    localStorage.setItem(viewKey.value, JSON.stringify({ layout: layout.value, toolbars: toolbarByLayout }))
  } catch {
    /* storage full / disabled — non-fatal */
  }
}
// The view watcher fires on every search keystroke; a synchronous localStorage
// write per keystroke is a visible input-lag source on mid hardware. Debounce so
// we persist once the user pauses, and flush on unmount so nothing is lost.
let persistTimer = null
function persistView() {
  if (restoring || swapping) return
  if (persistTimer) clearTimeout(persistTimer)
  persistTimer = setTimeout(() => {
    persistTimer = null
    writeView()
  }, 300)
}
onBeforeUnmount(() => {
  if (persistTimer) {
    clearTimeout(persistTimer)
    persistTimer = null
    writeView()
  }
})
function restoreView() {
  restoring = true
  try {
    const raw = localStorage.getItem(viewKey.value)
    if (raw) {
      const v = JSON.parse(raw)
      if (v.toolbars) {
        Object.assign(toolbarByLayout, v.toolbars)
        if (v.layout) layout.value = v.layout
      } else {
        // Migrate the old single-config format into the current layout's slot.
        if (v.layout) layout.value = v.layout
        toolbarByLayout[layout.value] = {
          groupMode: v.groupMode || defaultToolbar(layout.value).groupMode,
          tagPrefix: v.tagPrefix || '',
          sortLevels: Array.isArray(v.sortLevels)
            ? v.sortLevels
            : v.sortBy && v.sortBy !== 'position'
              ? [{ field: v.sortBy, dir: v.sortDir || 'asc' }]
              : [],
          subtasksExpanded: !!v.subtasksExpanded,
          filters: { priorities: [], assignees: [], tags: [], statuses: [], due: '', q: '', ...(v.filters || {}) },
        }
      }
      loadToolbar(toolbarByLayout[layout.value] || defaultToolbar(layout.value))
    } else {
      loadToolbar(defaultToolbar(layout.value))
    }
  } catch {
    loadToolbar(defaultToolbar(layout.value))
  } finally {
    nextTick(() => (restoring = false))
  }
}
// Swap the toolbar state when the layout changes (each layout keeps its own).
watch(layout, (newL, oldL) => {
  if (restoring || newL === oldL) return
  swapping = true
  toolbarByLayout[oldL] = snapshotToolbar()
  loadToolbar(toolbarByLayout[newL] || defaultToolbar(newL))
  nextTick(() => {
    swapping = false
    persistView()
  })
})
watch(
  [
    groupMode,
    tagPrefix,
    sortLevels,
    subtasksExpanded,
    autoSort,
    filters,
    colCollapse,
    autoCollapseEmpty,
    cardSize,
    stackFields,
    showEmpty,
    fieldVis,
    autosaveView,
  ],
  persistView,
  { deep: true },
)

// ── saved views (per-user, server-side; cross-device) ──
const savedViews = ref([])
// Saved views are bound to a visualization: show only those for the current
// layout (board views and timeline views are separate sets). Legacy views with
// no stored layout default to 'board'.
const viewsForLayout = computed(() =>
  savedViews.value.filter((v) => (v.config?.layout || 'board') === layout.value),
)
const currentViewName = ref('')
const newViewName = ref('')
const showSaveView = ref(false)
const showLoadView = ref(false)

async function loadViews() {
  try {
    savedViews.value = (await boards.views(props.boardId)).data || []
  } catch {
    savedViews.value = []
  }
}
// Snapshot / restore the full toolbar state that a view captures.
function currentViewConfig() {
  return {
    layout: layout.value,
    groupMode: groupMode.value,
    tagPrefix: tagPrefix.value,
    sortLevels: sortLevels.value,
    subtasksExpanded: subtasksExpanded.value,
    filters: { ...filters },
    colCollapse: { ...colCollapse },
    autoCollapseEmpty: autoCollapseEmpty.value,
    cardSize: cardSize.value,
    stackFields: stackFields.value,
    showEmpty: showEmpty.value,
    fieldVis: { ...fieldVis },
    autosaveView: autosaveView.value,
  }
}
function applyViewConfig(c) {
  if (!c) return
  if (c.layout) layout.value = c.layout
  if (c.groupMode) groupMode.value = c.groupMode
  tagPrefix.value = c.tagPrefix || ''
  if (Array.isArray(c.sortLevels)) sortLevels.value = c.sortLevels.map((l) => ({ ...l }))
  else if (c.sortBy && c.sortBy !== 'position')
    sortLevels.value = [{ field: c.sortBy, dir: c.sortDir || 'asc' }]
  else sortLevels.value = []
  subtasksExpanded.value = !!c.subtasksExpanded
  Object.assign(
    filters,
    { priorities: [], assignees: [], tags: [], statuses: [], milestones: [], due: '', q: '' },
    c.filters || {},
  )
  loadCustomize(c)
}
async function saveView() {
  const name = (newViewName.value || currentViewName.value).trim()
  if (!name) return
  try {
    await boards.saveView(props.boardId, { name, config: currentViewConfig() })
    currentViewName.value = name
    newViewName.value = ''
    showSaveView.value = false
    await loadViews()
    message.success(`Представление «${name}» сохранено`)
  } catch (e) {
    message.error(e.message)
  }
}
function applyView(v) {
  // Guard the layout-swap watcher: applyViewConfig sets layout AND the toolbar
  // fields itself, so the swap must not also fire and clobber them.
  restoring = true
  applyViewConfig(v.config)
  currentViewName.value = v.name
  showLoadView.value = false
  nextTick(() => {
    restoring = false
    persistView()
  })
}
async function deleteView(v) {
  try {
    await boards.deleteView(v.id)
    if (currentViewName.value === v.name) currentViewName.value = ''
    await loadViews()
  } catch (e) {
    message.error(e.message)
  }
}
// Autosave: when enabled and a named view is loaded, re-save it (debounced,
// silent) as the toolbar/customize state changes. Guarded by restoring/swapping
// like persistView so applying a view doesn't immediately re-save it.
let autosaveTimer = null
async function autosaveCurrent() {
  const name = currentViewName.value.trim()
  if (!name) return
  try {
    await boards.saveView(props.boardId, { name, config: currentViewConfig() })
  } catch {
    /* silent — a failed autosave shouldn't nag; manual save still surfaces errors */
  }
}
watch(
  () => (autosaveView.value && currentViewName.value ? JSON.stringify(currentViewConfig()) : ''),
  (sig) => {
    if (!sig || restoring || swapping) return
    if (autosaveTimer) clearTimeout(autosaveTimer)
    autosaveTimer = setTimeout(() => {
      autosaveTimer = null
      autosaveCurrent()
    }, 700)
  },
)

// ── adaptive column width (#7): fill the viewport, leave room for "+ колонка";
//    exactly one full-width column on mobile. We measure the real scroll
//    container (clientWidth already excludes the sidebar, padding and its own
//    scrollbar) instead of guessing from window size, so columns never overflow.
const boardScroll = ref(null)
const containerWidth = ref(0)
let ro = null
function measure() {
  if (boardScroll.value) containerWidth.value = boardScroll.value.clientWidth
}
watch(boardScroll, (el) => {
  if (!ro || !el) return
  ro.disconnect()
  ro.observe(el)
  measure()
})
const GAP = 12
const ADD_COL_W = 220
const STRIP_W = 44 // collapsed-column strip width (keep in sync with .col.collapsed)
const COL_MIN = 220 // narrowest a column may get before we switch to scrolling
const COL_MAX = 420
// Card-size presets (customize view): the size changes the card composition (see
// TaskCard) AND the column width band — compact is title-only so it can be narrow,
// large carries every field so it's wider. Medium keeps the current width band.
const CARD_SIZES = {
  compact: { min: 160, cap: 230 },
  medium: { min: COL_MIN, cap: COL_MAX },
  large: { min: 300, cap: 560 },
}
const cardCap = computed(() => (CARD_SIZES[cardSize.value] || CARD_SIZES.medium).cap)
const cardMin = computed(() => (CARD_SIZES[cardSize.value] || CARD_SIZES.medium).min)
const colWidth = computed(() => {
  const cw = containerWidth.value
  if (!cw) return 280 // pre-measure fallback
  // Mobile: one column a little under full width so the next one peeks and the
  // swipe (scroll-snap, see CSS) reads as a smooth page-turn.
  if (isMobile.value) return Math.max(cw - 28, 200)
  // Collapsed columns render as a 44px strip (see .col.collapsed) — reserve their
  // fixed width and divide the rest among the EXPANDED columns only, so collapsing
  // a column hands its width to the neighbours instead of leaving a gap on the right.
  const cols = displayColumns.value
  const n = cols.length || 1
  const nCollapsed = cols.filter((c) => isColCollapsed(c)).length
  const nExpanded = Math.max(n - nCollapsed, 1)
  // Reserve the "+ колонка" tile (status mode) + the gaps + the collapsed strips,
  // plus a few px slack so sub-pixel rounding can't trip the horizontal scrollbar.
  const reserved =
    (groupMode.value === 'status' ? ADD_COL_W + GAP : 0) +
    (n - 1) * GAP +
    4 +
    nCollapsed * STRIP_W
  const w = Math.floor((cw - reserved) / nExpanded)
  // Fill the viewport while comfortable; the width band comes from the chosen card
  // size (compact narrower, large wider), so columns never get too narrow to hold
  // their fields and scroll horizontally once they'd shrink past the band's min.
  return Math.min(Math.max(w, cardMin.value), cardCap.value)
})
const colStyleVars = computed(() => ({ '--col-w': colWidth.value + 'px' }))

// User-chosen board background (preference): a CSS colour or an image URL.
const themeStore = useThemeStore()
const boardBgStyle = computed(() => {
  const bg = themeStore.boardBackground
  if (!bg) return {}
  return /^https?:\/\//.test(bg)
    ? { backgroundImage: `url("${bg}")`, backgroundSize: 'cover', backgroundPosition: 'center' }
    : { background: bg }
})

// modals
const selectedTaskId = ref(null)
const showTaskModal = ref(false)
// The open task is reflected in the URL (?task=<id>) so it's shareable and a
// refresh re-opens it. Opening pushes a history entry (so Back closes the modal);
// closing strips the param. The route.query.task watcher keeps state in sync,
// including when the browser Back button pops the entry.
// The URL carries the readable task number (#252), not the UUID. numberOf maps a
// task UUID to its number from the loaded board data.
function numberOf(id) {
  const t = allTasks.value.find((x) => x.id === id)
  if (t?.number != null) return t.number
  for (const arr of Object.values(subtasksByParent.value)) {
    const s = arr.find((x) => x.id === id)
    if (s?.number != null) return s.number
  }
  return null
}
function openTask(id) {
  selectedTaskId.value = id
  showTaskModal.value = true
  const param = numberOf(id) ?? id
  if (String(route.query.task ?? '') !== String(param)) {
    router.push({ query: { ...route.query, task: param } })
  }
}
function closeTask() {
  showTaskModal.value = false
  if (route.query.task) {
    const q = { ...route.query }
    delete q.task
    router.replace({ query: q })
  }
}

const dragging = ref(false) // reactive: also shown to cards for the nest dropzone
let suppressReloadUntil = 0
function suppress() {
  suppressReloadUntil = Date.now() + 1500
}

// ── custom edge auto-scroll during drag ──
// Sortable's built-in auto-scroll doesn't reliably scroll a nested horizontal
// container on touch. Rather than chase flaky move events, we read the position
// of Sortable's own drag image (`.sortable-fallback` on touch, `.sortable-drag`
// on desktop) each animation frame, with a dragover fallback for desktop.
const EDGE = 72 // px from a board edge that triggers scrolling
const STEP_COOLDOWN = 600 // ms between one-column steps while held at the edge
let edgeRAF = null
let pointerX = null // last desktop dragover X (touch uses the drag image)
let lastStep = 0
function onDragOver(e) {
  pointerX = e.clientX
}
function dragX() {
  // Touch: the moving clone follows the finger. (On desktop `.sortable-drag` is
  // the static original, so there we fall back to the dragover X instead.)
  const clone = document.querySelector('.sortable-fallback')
  if (clone) {
    const r = clone.getBoundingClientRect()
    return r.left + r.width / 2
  }
  return pointerX
}
let scrollIdx = null // tracked target column index (avoids reading mid-animation scrollLeft)
// Scroll exactly one column in `dir` (-1 left / +1 right), snapping to its start.
function stepColumn(dir) {
  const el = boardScroll.value
  if (!el) return
  const stride = colWidth.value + GAP
  const maxIdx = Math.round((el.scrollWidth - el.clientWidth) / stride)
  if (scrollIdx == null) scrollIdx = Math.round(el.scrollLeft / stride)
  scrollIdx = Math.max(0, Math.min(maxIdx, scrollIdx + dir))
  el.scrollTo({ left: scrollIdx * stride, behavior: 'smooth' })
}
function autoScrollTick() {
  const el = boardScroll.value
  const px = dragX()
  if (dragging.value && el && px != null) {
    const rect = el.getBoundingClientRect()
    let dir = 0
    if (px < rect.left + EDGE) dir = -1
    else if (px > rect.right - EDGE) dir = 1
    if (dir !== 0) {
      // One column per entry, then one more every cooldown if held at the edge.
      const now = performance.now()
      if (now - lastStep > STEP_COOLDOWN) {
        stepColumn(dir)
        lastStep = now
      }
    } else {
      // Centre: re-sync the target to where we actually are so the next step
      // moves exactly one column (no skipping from a mid-animation read).
      lastStep = 0
      scrollIdx = Math.round(el.scrollLeft / (colWidth.value + GAP))
    }
  }
  edgeRAF = requestAnimationFrame(autoScrollTick)
}
function onDragStart() {
  dragging.value = true
  pointerX = null
  scrollIdx = null
  lastStep = 0
  // Mobile uses scroll-snap (x mandatory) + smooth scrolling, which both revert
  // our per-frame scrollLeft nudges — disable them for the duration of the drag.
  const el = boardScroll.value
  if (el) {
    el.style.scrollSnapType = 'none'
    el.style.scrollBehavior = 'auto'
  }
  window.addEventListener('dragover', onDragOver, { passive: true })
  if (!edgeRAF) edgeRAF = requestAnimationFrame(autoScrollTick)
}
function onDragEnd() {
  dragging.value = false
  pointerX = null
  const el = boardScroll.value
  if (el) {
    el.style.scrollSnapType = ''
    el.style.scrollBehavior = ''
  }
  window.removeEventListener('dragover', onDragOver)
  if (edgeRAF) {
    cancelAnimationFrame(edgeRAF)
    edgeRAF = null
  }
}
let reloadTimer = null
function scheduleReload() {
  clearTimeout(reloadTimer)
  reloadTimer = setTimeout(() => load(props.boardId), 200)
}

async function load(id) {
  loading.value = true
  try {
    // Sprint navigation: the URL ?milestone=<uuid|backlog> scopes the board to one
    // milestone server-side, so a huge project never loads all its cards at once.
    // ?archived=1 loads the read-only archive instead (subtasks skipped — they are
    // archived together with their parents).
    const archived = route.query.archived === '1'
    const ms = route.query.milestone
    const params = archived ? { archived: 1 } : ms ? { milestone: ms } : undefined
    const [b, c, t, s] = await Promise.all([
      boards.get(id),
      boards.columns(id),
      boards.tasks(id, params),
      archived ? Promise.resolve({ data: [] }) : boards.subtasks(id),
    ])
    board.value = b.data
    columns.value = c.data || []
    allTasks.value = t.data || []
    const byParent = {}
    for (const sub of s.data || []) (byParent[sub.parent_id] ||= []).push(sub)
    subtasksByParent.value = byParent
    await loadWorkspaceMeta()
    rebuildLists()
  } catch (e) {
    message.error(e.message)
  } finally {
    loading.value = false
  }
}

async function loadWorkspaceMeta() {
  const wsId = wsStore.currentId
  const projectId = board.value?.project_id
  if (!wsId || !projectId) return
  // Tags + prefix names are project-scoped; members stay workspace-scoped.
  const [tg, mem, pfx, glMem, glInt, ms] = await Promise.all([
    projectsApi.tags(projectId),
    wsApi.members(wsId),
    projectsApi.tagPrefixes(projectId).catch(() => ({ data: [] })),
    wsApi.gitlabMembers(wsId).catch(() => ({ data: [] })),
    gitlabApi.listIntegrations(wsId).catch(() => ({ data: { integrations: [] } })),
    projectsApi.milestones(projectId).catch(() => ({ data: [] })),
  ])
  for (const k of Object.keys(tagsMap)) delete tagsMap[k]
  for (const t of tg.data || []) tagsMap[t.id] = t
  for (const k of Object.keys(milestonesMap)) delete milestonesMap[k]
  for (const m of ms.data || []) milestonesMap[m.id] = m
  for (const k of Object.keys(membersMap)) delete membersMap[k]
  for (const m of mem.data || []) membersMap[m.user_id] = m
  for (const k of Object.keys(gitlabMembersMap)) delete gitlabMembersMap[k]
  for (const g of glMem.data || []) gitlabMembersMap[g.gl_user_id] = g
  // Issue-creation is offered only on the binding that targets THIS board.
  const gi = (glInt.data?.integrations || []).find((b) => b.board_id === props.boardId) || {}
  gitlabIntegrationId.value = gi.id || null
  gitlabCanCreate.value =
    gi.enabled === true && gi.writeback?.push_create === true
  gitlabFetchTemplates.value = gitlabCanCreate.value && gi.writeback?.fetch_templates === true
  for (const k of Object.keys(tagPrefixNames)) delete tagPrefixNames[k]
  for (const p of pfx.data || []) tagPrefixNames[p.prefix] = p.label
  // Mirror tags + prefix names + context to the store so the header Теги manager works.
  boardViewStore.setTags(tagsList.value)
  boardViewStore.setPrefixNames({ ...tagPrefixNames })
  boardViewStore.setMilestones(milestonesList.value)
  boardViewStore.setContext(props.boardId, wsId, projectId)
}

// Due-date predicate for the "Срок" filter.
function matchesDue(t, mode) {
  if (mode === 'none') return !t.due_date
  if (!t.due_date) return false
  if (mode === 'has') return true
  const now = new Date()
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate())
  const due = new Date(t.due_date)
  const dueDay = new Date(due.getFullYear(), due.getMonth(), due.getDate())
  const dayMs = 86400000
  if (mode === 'overdue') return dueDay < today && !t.completed_at
  if (mode === 'today') return dueDay.getTime() === today.getTime()
  if (mode === 'week') return dueDay >= today && dueDay - today <= 7 * dayMs
  return true
}

// filter + sort applied before grouping
const filteredTasks = computed(() => {
  let arr = allTasks.value
  if (filters.priorities.length) arr = arr.filter((t) => filters.priorities.includes(t.priority))
  if (filters.assignees.length)
    arr = arr.filter((t) => {
      const ids = t.assignee_ids || []
      const logins = t.gitlab_assignee_logins || []
      return filters.assignees.some((a) =>
        typeof a === 'string' && a.startsWith('gl:') ? logins.includes(a.slice(3)) : ids.includes(a),
      )
    })
  if (filters.tags.length)
    arr = arr.filter((t) => (t.tag_ids || []).some((id) => filters.tags.includes(id)))
  if (filters.statuses.length) arr = arr.filter((t) => filters.statuses.includes(t.column_id))
  if (filters.milestones.length)
    arr = arr.filter((t) => filters.milestones.includes(t.milestone_id || '__none__'))
  if (filters.due) arr = arr.filter((t) => matchesDue(t, filters.due))
  const q = filters.q.trim().toLowerCase()
  if (q)
    arr = arr.filter(
      (t) => t.title.toLowerCase().includes(q) || (t.number != null && `#${t.number}`.includes(q)),
    )

  return sortByLevels(arr)
})

// Apply the composer's multi-level sort to a task array (empty sort = stored order,
// returned as a fresh copy so callers never mutate the source).
function sortByLevels(arr) {
  const s = [...arr]
  if (sortLevels.value.length) {
    s.sort((a, b) => {
      for (const lvl of sortLevels.value) {
        const r = cmpLevel(a, b, lvl)
        if (r) return r
      }
      return 0
    })
  }
  return s
}

// Subtasks mirror the parent sort: when a composer sort is active each parent's
// children follow it too; with no sort we keep the raw stored order so on-card
// drag-reorder stays authoritative.
const sortedSubtasksByParent = computed(() => {
  if (!sortLevels.value.length) return subtasksByParent.value
  const out = {}
  for (const [pid, subs] of Object.entries(subtasksByParent.value)) out[pid] = sortByLevels(subs)
  return out
})

// The board's task-completing column: explicit if set, else the rightmost.
const doneColumnId = computed(() => {
  if (board.value?.done_column_id) return board.value.done_column_id
  const cols = columns.value
  return cols.length ? cols[cols.length - 1].id : null
})
async function onSetDone(columnId) {
  try {
    const r = await boards.setDoneColumn(props.boardId, columnId)
    board.value = r.data
  } catch (e) {
    message.error(e.message)
  }
}

const displayColumns = computed(() => {
  if (groupMode.value === 'status') {
    return columns.value.map((c) => ({ key: c.id, name: c.name, color: c.color, status: c }))
  }
  if (groupMode.value === 'milestone') {
    return [
      ...milestonesList.value.map((m) => ({ key: m.id, name: m.title, color: '', milestone: m })),
      { key: '__none__', name: 'Без этапа', color: '', milestone: null },
    ]
  }
  return [
    ...groupTags.value.map((t) => ({ key: t.id, name: t.name, color: t.color, tag: t })),
    { key: '__none__', name: 'Без тегов', color: '', tag: null },
  ]
})

// Estimation rollup per milestone column: Σ of the column's tasks' own estimates,
// formatted in the project's unit. Shown only when grouped by milestone.
const estCfg = computed(() => wsStore.estimationFor(board.value?.project_id))
function columnEstimate(dcol) {
  if (groupMode.value !== 'milestone') return ''
  const v = sumEstimates(lists.value[dcol.key] || [])
  return v ? formatEstimate(v, estCfg.value) : ''
}

function rebuildLists() {
  // 'assignee'/'none' are timeline-only swimlane modes; the board/list views only
  // understand status/tag/milestone columns, so skip rebuilding for those.
  if (!['status', 'tag', 'milestone'].includes(groupMode.value)) return
  const map = {}
  if (groupMode.value === 'status') {
    for (const col of columns.value) map[col.id] = []
    for (const t of filteredTasks.value) (map[t.column_id] ||= []).push(t)
  } else if (groupMode.value === 'milestone') {
    const msIds = new Set(milestonesList.value.map((m) => m.id))
    for (const m of milestonesList.value) map[m.id] = []
    map.__none__ = []
    for (const t of filteredTasks.value) {
      if (t.milestone_id && msIds.has(t.milestone_id)) map[t.milestone_id].push(t)
      else map.__none__.push(t)
    }
  } else {
    const tgIds = new Set(groupTags.value.map((t) => t.id))
    for (const tg of groupTags.value) map[tg.id] = []
    map.__none__ = []
    for (const t of filteredTasks.value) {
      const ids = (t.tag_ids || []).filter((id) => tgIds.has(id))
      if (!ids.length) map.__none__.push(t)
      else for (const id of ids) map[id].push(t)
    }
  }
  lists.value = map
}
watch([filteredTasks, groupMode, tagPrefix, milestonesList], rebuildLists)

// Observe each card wrapper (stable per task id via item-key) once. Off-screen
// cards collapse to a placeholder; near-viewport cards mount the real TaskCard.
function regCard(el, id) {
  if (!el || !cardIO) return
  el.dataset.cardId = id
  cardIO.observe(el)
  cardRO.observe(el)
}

// Mutable mirror of displayColumns for column drag-reorder (status mode only).
const colModel = ref([])
watch(displayColumns, (v) => (colModel.value = [...v]), { immediate: true })

async function onColumnReorder(evt) {
  if (groupMode.value !== 'status') return
  const info = evt.moved || evt.added
  if (!info) return
  const arr = colModel.value
  const before = arr[info.newIndex - 1]
  const after = arr[info.newIndex + 1]
  suppress()
  try {
    await columnsApi.move(info.element.key, {
      before_id: before ? before.key : null,
      after_id: after ? after.key : null,
    })
    scheduleReload()
  } catch (e) {
    message.error(e.message)
    load(props.boardId)
  }
}

// Drag persistence: status mode = reposition; tag mode = add/remove tag.
async function onColChange(evt, dcol) {
  suppress()
  try {
    if (groupMode.value === 'status') {
      const info = evt.added || evt.moved
      if (info) {
        const arr = lists.value[dcol.key]
        // Dropping onto a collapsed column: Sortable's newIndex is meaningless
        // (its items are hidden placeholders behind the drop overlay), so pin the
        // card to the top — before = nothing, after = the first existing card.
        const collapsed = evt.added && isColCollapsed(dcol)
        const before = collapsed ? null : arr[info.newIndex - 1]
        const after = collapsed ? arr.find((t) => t.id !== info.element.id) : arr[info.newIndex + 1]
        // A subtask dragged out onto a column becomes top-level again.
        if (evt.added && info.element.parent_id) {
          await tasksApi.setParent(info.element.id, null)
        }
        await tasksApi.move(info.element.id, {
          column_id: dcol.key,
          before_id: before ? before.id : null,
          after_id: after ? after.id : null,
        })
      }
    } else if (groupMode.value === 'milestone') {
      // Single-value: the destination column's `added` sets/clears it; the source's
      // `removed` is ignored (the new value overwrites).
      if (evt.added) {
        const id = evt.added.element.id
        if (dcol.milestone) await tasksApi.setMilestone(id, dcol.milestone.id)
        else await tasksApi.clearMilestone(id)
      }
    } else {
      if (evt.added && dcol.tag) await tasksApi.addTag(evt.added.element.id, dcol.tag.id)
      if (evt.removed && dcol.tag) await tasksApi.removeTag(evt.removed.element.id, dcol.tag.id)
    }
    scheduleReload()
  } catch (e) {
    message.error(e.message)
    load(props.boardId)
  }
}

// ── inline task creation (a reference tracker-style "+ New task" at column bottom) ──
const addingInColumn = ref(null) // dcol.key currently adding into
const newTaskTitle = ref('')
const taskInput = ref(null)
function focusTaskInput() {
  nextTick(() => taskInput.value?.focus?.())
}
function startAddTask(dcol) {
  addingInColumn.value = dcol.key
  newTaskTitle.value = ''
  focusTaskInput()
}
function cancelAddTask() {
  addingInColumn.value = null
  newTaskTitle.value = ''
}
async function submitAddTask(dcol) {
  const title = newTaskTitle.value.trim()
  if (!title) {
    cancelAddTask()
    return
  }
  // Tag mode has no status columns to drop into → use the first board column
  // and pre-tag with the tag column's tag.
  const columnId = groupMode.value === 'status' ? dcol.key : columns.value[0]?.id
  if (!columnId) {
    message.warning('Сначала создайте хотя бы одну колонку-статус')
    return
  }
  suppress()
  newTaskTitle.value = '' // keep the input open for rapid entry
  try {
    const res = await boards.createTask(board.value.id, { column_id: columnId, title })
    if (groupMode.value === 'tag' && dcol.tag) await tasksApi.addTag(res.data.id, dcol.tag.id)
    if (groupMode.value === 'milestone' && dcol.milestone)
      await tasksApi.setMilestone(res.data.id, dcol.milestone.id)
    await load(props.boardId)
    addingInColumn.value = dcol.key // load() reset refs; re-open
    focusTaskInput()
  } catch (e) {
    message.error(e.message)
  }
}

// ── inline column creation ("+ New column" to the right) ──
const addingColumn = ref(false)
const newColumnName = ref('')
const colInput = ref(null)
function startAddColumn() {
  addingColumn.value = true
  newColumnName.value = ''
  nextTick(() => colInput.value?.focus?.())
}
async function submitAddColumn() {
  const name = newColumnName.value.trim()
  if (!name) {
    addingColumn.value = false
    return
  }
  suppress()
  try {
    await boards.createColumn(board.value.id, { name })
    newColumnName.value = ''
    addingColumn.value = false
    await load(props.boardId)
  } catch (e) {
    message.error(e.message)
  }
}

// A task edit or tag-list change touches both tasks and workspace meta, so do
// a full (debounced) board reload.
function onChanged() {
  suppress()
  scheduleReload()
}

// Matrix view quick-add: create the task in the board's first column, then pin it
// to the chosen Eisenhower quadrant (the matrix has no column to drop into).
async function createInQuadrant({ title, quadrant }) {
  const columnId = columns.value[0]?.id
  if (!columnId) {
    message.warning('Сначала создайте хотя бы одну колонку-статус')
    return
  }
  suppress()
  try {
    const res = await boards.createTask(board.value.id, { column_id: columnId, title })
    await tasksApi.eisenhower(res.data.id, quadrant)
    await load(props.boardId)
  } catch (e) {
    message.error(e.message)
  }
}

useRealtime((ev) => {
  if (ev.scope !== wsStore.currentId) return
  // Surface a freshly detected write-back conflict so the user knows to resolve it
  // (the resolver lives in the GitLab modal's «Конфликты» entry).
  if (ev.type === 'gitlab.conflict' && !ev.data?.resolved) {
    message.warning('Конфликт обратной записи GitLab — откройте настройки GitLab, чтобы разрешить')
  }
  // Milestone CRUD elsewhere → refresh the project's milestone list so chips/columns update.
  if (typeof ev.type === 'string' && ev.type.startsWith('milestone.')) reloadMilestones()
  // Board-activity toast for create/move on THIS board (any actor).
  if (ev.type === 'task.created' || ev.type === 'task.moved') pushActivity(ev)
  if (dragging.value || Date.now() < suppressReloadUntil) return
  scheduleReload()
})

// Raise a live activity toast for a task create/move on the currently-open board.
// The verb for a move is refined by comparing the event's completion state to the
// card we still hold locally (the board reload is debounced, so the pre-move state
// is intact here): entering/leaving the done boundary reads as completed/reopened.
function pushActivity(ev) {
  const t = ev.data
  if (!t || typeof t !== 'object' || t.board_id !== props.boardId) return
  let verb = ev.type === 'task.created' ? 'created' : 'moved'
  if (ev.type === 'task.moved') {
    const prev = allTasks.value.find((x) => x.id === t.id)
    const wasDone = !!prev?.completed_at
    const isDone = !!t.completed_at
    if (isDone && !wasDone) verb = 'completed'
    else if (!isDone && wasDone) verb = 'reopened'
  }
  const actorId = ev.actor || t.created_by || null
  const self = !!actorId && actorId === auth.user?.id
  const m = actorId ? membersMap[actorId] : null
  activityToasts.value?.push({
    id: t.id,
    number: t.number ?? null,
    title: t.title || 'Задача',
    verb,
    actorId: m ? actorId : null,
    actorName: m?.name || '',
    self,
  })
}

// Refresh just the project's milestones (after the manager edits them, or a remote
// milestone change) without a full board reload.
async function reloadMilestones() {
  const projectId = board.value?.project_id
  if (!projectId) return
  try {
    const { data } = await projectsApi.milestones(projectId)
    for (const k of Object.keys(milestonesMap)) delete milestonesMap[k]
    for (const m of data || []) milestonesMap[m.id] = m
    boardViewStore.setMilestones(milestonesList.value)
  } catch {
    /* keep the current list on error */
  }
}

// Header-hosted actions (Теги manager, Архив, Этапы) ask the board to reload.
watch(
  () => boardViewStore.reloadNonce,
  () => {
    reloadMilestones()
    onChanged()
  },
)

// Sync the open task to the URL's ?task= (a number, or a legacy UUID). Resolves
// a number to its task, then canonicalizes the URL to the number form.
async function applyTaskQuery() {
  const q = route.query.task
  if (!q) {
    if (showTaskModal.value) closeTask()
    return
  }
  const s = String(q)
  // Already showing this task (opened from a card / just canonicalized)? No work.
  if (
    showTaskModal.value &&
    selectedTaskId.value &&
    (s === selectedTaskId.value || s === String(numberOf(selectedTaskId.value) ?? ''))
  ) {
    return
  }
  let id = s
  if (/^\d+$/.test(s)) {
    try {
      id = (await wsApi.taskByNumber(wsStore.currentId, s)).data.id
    } catch {
      return // unknown number — leave the board as-is
    }
  }
  selectedTaskId.value = id
  showTaskModal.value = true
  const num = numberOf(id)
  if (num != null && String(route.query.task) !== String(num)) {
    router.replace({ query: { ...route.query, task: num } })
  }
}

// Deep-link from the «Этапы» screen: ?milestone=<id> filters the board to exactly
// that milestone (a removable chip), then the param is stripped so the URL stays
// clean. It *replaces* the milestone facet (rather than appending) so re-entering
// from the screen for a different milestone doesn't accumulate the previous one
// that the saved view had persisted.
// Sprint scope is now driven by a persistent ?milestone=<uuid|backlog> param
// (server-side scoped in load(), node-highlighted in the sidebar). Nothing to do
// here — kept as a no-op so existing call sites stay valid.
function applyMilestoneQuery() {}

onMounted(async () => {
  ro = new ResizeObserver(() => measure())
  if (boardScroll.value) {
    ro.observe(boardScroll.value)
    measure()
  }
  // Card-list windowing: reveal cards within 800px of the viewport; collapse the
  // rest. Frozen mid-drag so SortableJS sees a stable DOM.
  cardIO = new IntersectionObserver(
    (entries) => {
      if (dragging.value) return
      for (const en of entries) {
        const id = en.target.dataset.cardId
        if (id) vis[id] = en.isIntersecting
      }
    },
    { rootMargin: '800px 0px' },
  )
  // Measure rendered cards (re-fires as TaskCard content settles, unlike a
  // one-shot read), so a collapsed card's placeholder gets its exact height.
  // Skip wrappers showing a placeholder to avoid feeding back a stale height.
  cardRO = new ResizeObserver((entries) => {
    for (const en of entries) {
      const el = en.target
      const id = el.dataset.cardId
      if (!id || el.firstElementChild?.classList.contains('card-ph')) continue
      const h = Math.round(en.contentRect.height)
      if (h > 0 && cardH[id] !== h) cardH[id] = h
    }
  })
  document.addEventListener('pointerdown', onDocPointerDown, true)
  restoreView()
  await load(props.boardId)
  loadViews()
  applyTaskQuery()
  applyMilestoneQuery()
})
onBeforeUnmount(() => {
  ro?.disconnect()
  cardIO?.disconnect()
  cardRO?.disconnect()
  document.removeEventListener('pointerdown', onDocPointerDown, true)
  onDragEnd()
  boardViewStore.reset()
})
watch(
  () => props.boardId,
  async (id) => {
    if (!id) return
    // Drop the previous board's windowing state so its measured heights /
    // visibility don't bleed in (re-seeded fresh by the IO/RO on render).
    for (const k of Object.keys(vis)) delete vis[k]
    for (const k of Object.keys(cardH)) delete cardH[k]
    restoreView()
    await load(id)
    loadViews()
    applyTaskQuery()
    applyMilestoneQuery()
  },
)
watch(
  () => route.query.task,
  () => applyTaskQuery(),
)
// Switching étapes on the same board (only the query changes, no remount) reloads
// the scoped task set. Entering/leaving the archive reloads too.
watch(
  () => [route.query.milestone, route.query.archived],
  ([ms]) => {
    // Opening an étape via the project tree supersedes any manual étape filter in
    // the composer — clear it, else the scope's tasks would be filtered away.
    if (ms) filters.milestones = []
    load(props.boardId)
  },
)

// Restore a task from the archive view (the only mutation allowed there).
async function restoreFromArchive(taskId) {
  try {
    await tasksApi.restore(taskId)
    allTasks.value = allTasks.value.filter((t) => t.id !== taskId)
    rebuildLists()
    message.success('Задача возвращена из архива')
  } catch (e) {
    message.error(e.message)
  }
}
</script>

<template>
  <n-spin :show="loading" :rotate="false" class="board-spin">
    <template #icon><TesseraSpinner /></template>
    <div v-if="board" class="board-wrap" :class="{ 'has-bg': !!themeStore.boardBackground }" :style="boardBgStyle">
      <!-- Sub-toolbar under the header: grouping / sort / filters / subtasks +
           a task-name search on the right. (Layout + Теги/Архив live in the
           global header now.) -->
      <div ref="subbarEl" class="subbar">
        <!-- Archive banner: read-only view of archived tasks with all board filters. -->
        <span v-if="archivedMode" class="ms-scope-chip archive-chip" title="Архив — только чтение">
          <n-icon :component="ArchiveOutline" />
          <span class="ms-scope-label">Архив (только чтение)</span>
          <n-icon class="ms-scope-x" :component="CloseOutline" @click.stop="exitArchive" />
        </span>
        <!-- Sprint scope chip (navigation overlay from the sidebar). Removing it
             returns the full board and de-highlights the sprint node. -->
        <span v-if="milestoneScope" class="ms-scope-chip" title="Показан один этап">
          <n-icon :component="RibbonOutline" />
          <span class="ms-scope-label">{{ milestoneScopeLabel }}</span>
          <n-icon class="ms-scope-x" :component="CloseOutline" @click.stop="clearMilestoneScope" />
        </span>
        <!-- Composer bar: grouping / sort / filters as removable chips + an add
             menu + the name search, all in one wide bar (a reference tracker/GitLab-style).
             Collapsed to one row so the tools stay visible; tap to expand. -->
        <div
          class="composer"
          :class="{ collapsed: !composerExpanded, 'has-clear': hasClearableFacets }"
          @click="expandComposer"
        >
          <span
            v-for="(c, ci) in facetChips"
            :key="ci"
            class="facet"
            :class="{ group: c.kind === 'group', sortable: c.kind === 'sort' }"
            :title="c.kind === 'group' ? 'Переключить статусы/теги' : c.kind === 'sort' ? 'Сменить направление' : ''"
            @click="onChipClick(c)"
          >
            {{ c.label }}
            <button
              v-if="c.kind !== 'group'"
              class="facet-x"
              title="Убрать"
              @click.stop="removeChip(c)"
            >
              ×
            </button>
          </span>

          <n-dropdown
            :show="addShow"
            scrollable
            trigger="click"
            placement="bottom-start"
            :options="addMenuOptions"
            :render-label="renderAddLabel"
            @update:show="onAddShow"
            @select="onAddSelect"
          >
            <button class="facet-add" title="Добавить группировку / сортировку / фильтр">
              <n-icon :component="AddOutline" :size="14" />
            </button>
          </n-dropdown>

          <input
            v-model="filters.q"
            class="composer-search"
            placeholder="Поиск по названию…"
          />
          <button v-if="hasClearableFacets" class="facet-clear" title="Сбросить всё" @click="clearAll">
            ×
          </button>
        </div>

        <!-- Right-side tools — slide off to the right while the composer is
             expanded so the full chip set has room. -->
        <div class="bar-tools" :class="{ hidden: composerExpanded }">
        <!-- "Авто": dependency-graph ordering (Gantt only). Resets the composer to
             no-group/no-sort and orders rows by the blocking graph. -->
        <n-tooltip v-if="layout === 'gantt'">
          <template #trigger>
            <n-button
              size="small"
              quaternary
              class="ngrad bar-btn"
              :type="autoActive ? 'primary' : 'default'"
              @click="toggleAuto"
            >
              <template #icon><n-icon :component="GitNetworkOutline" /></template>
            </n-button>
          </template>
          {{ autoActive ? 'Авто-сортировка по зависимостям (вкл.)' : 'Авто: сортировать по зависимостям' }}
        </n-tooltip>

        <n-tooltip v-if="!timelineLike">
          <template #trigger>
            <n-button
              size="small"
              quaternary
              class="ngrad bar-btn"
              :type="customizeOpen ? 'primary' : 'default'"
              @click="customizeOpen = true"
            >
              <template #icon><n-icon :component="SettingsOutline" /></template>
            </n-button>
          </template>
          Настроить вид
        </n-tooltip>

        <!-- saved views: load (folder) + save (disk) -->
        <n-popover v-model:show="showLoadView" trigger="click" placement="bottom-start">
            <template #trigger>
            <n-tooltip>
                <template #trigger>
                <n-button
                    size="small"
                    quaternary
                    class="ngrad bar-btn"
                    :type="currentViewName ? 'primary' : 'default'"
                >
                    <template #icon><n-icon :component="FolderOpenOutline" /></template>
                </n-button>
                </template>
                {{ currentViewName ? `Представление: ${currentViewName}` : 'Загрузить представление' }}
            </n-tooltip>
            </template>
            <div class="views-pop">
            <div v-for="v in viewsForLayout" :key="v.id" class="view-row">
                <button class="view-name" :class="{ active: v.name === currentViewName }" @click="applyView(v)">
                {{ v.name }}
                </button>
                <n-popconfirm
                :positive-button-props="{ type: 'error' }"
                positive-text="Удалить"
                @positive-click="deleteView(v)"
                >
                <template #trigger>
                    <n-button text size="tiny" type="error"><n-icon :component="TrashOutline" /></n-button>
                </template>
                Удалить представление «{{ v.name }}»?
                </n-popconfirm>
            </div>
            <n-text v-if="!viewsForLayout.length" depth="3" class="views-empty">
                Нет сохранённых представлений
            </n-text>
            </div>
        </n-popover>

        <n-popover v-model:show="showSaveView" trigger="click" placement="bottom-start">
            <template #trigger>
            <n-tooltip>
                <template #trigger>
                <n-button size="small" quaternary class="ngrad bar-btn">
                    <template #icon><n-icon :component="SaveOutline" /></template>
                </n-button>
                </template>
                Сохранить представление
            </n-tooltip>
            </template>
            <div class="views-pop">
            <n-input
                v-model:value="newViewName"
                size="small"
                placeholder="Название представления"
                @keyup.enter="saveView"
            />
            <div v-if="viewsForLayout.length" class="views-over">
                <n-text depth="3" class="views-lbl">Перезаписать:</n-text>
                <button
                v-for="v in viewsForLayout"
                :key="v.id"
                class="view-chip"
                @click="newViewName = v.name"
                >
                {{ v.name }}
                </button>
            </div>
            <n-button type="primary" size="small" block @click="saveView">Сохранить</n-button>
            </div>
        </n-popover>
        </div>
      </div>

      <BoardListView
        v-if="layout === 'list'"
        :columns="displayColumns"
        :status-columns="columns"
        :lists="lists"
        :tags-map="tagsMap"
        :members-map="membersMap"
        @open="openTask"
        @changed="onChanged"
      />

      <BoardCalendarView
        v-else-if="layout === 'calendar'"
        :tasks="filteredTasks"
        :status-columns="columns"
        @open="openTask"
        @changed="onChanged"
      />

      <BoardTimelineView
        v-else-if="layout === 'timeline'"
        :tasks="filteredTasks"
        :status-columns="columns"
        :members-map="membersMap"
        :tags-map="tagsMap"
        :group-mode="groupMode"
        :tag-prefix="tagPrefix"
        :project-id="board?.project_id"
        :subtasks-by-parent="subtasksByParent"
        :milestones="milestonesList"
        @open="openTask"
        @changed="onChanged"
      />

      <BoardGanttView
        v-else-if="layout === 'gantt'"
        :board-id="boardId"
        :tasks="filteredTasks"
        :status-columns="columns"
        :members-map="membersMap"
        :tags-map="tagsMap"
        :group-mode="groupMode"
        :tag-prefix="tagPrefix"
        :project-id="board?.project_id"
        :auto-sort="autoActive"
        :subtasks-by-parent="subtasksByParent"
        :milestones="milestonesList"
        @open="openTask"
        @changed="onChanged"
      />

      <BoardMatrixView
        v-else-if="layout === 'matrix'"
        :tasks="filteredTasks"
        :subtasks-by-parent="sortedSubtasksByParent"
        :subtasks-expanded="subtasksExpanded"
        :columns="columns"
        :tags-map="tagsMap"
        :members-map="membersMap"
        :tags="tagsList"
        :tag-prefix-names="tagPrefixNames"
        :members="membersList"
        :ws-id="wsStore.currentId"
        :project-id="board?.project_id"
        @open="openTask"
        @changed="onChanged"
        @create="createInQuadrant"
      />

      <div v-else ref="boardScroll" class="board-scroll" :style="colStyleVars">
        <draggable
          :list="colModel"
          group="columns"
          item-key="key"
          handle=".col-drag"
          :disabled="groupMode !== 'status' || archivedMode"
          class="cols"
          :class="{ dragging }"
          :animation="150"
          :delay="160"
          :delay-on-touch-only="true"
          :touch-start-threshold="6"
          @start="onDragStart"
          @end="onDragEnd"
          @change="onColumnReorder"
        >
          <template #item="{ element: dcol }">
            <div
              class="col"
              :class="{ collapsed: colCollapsedNow(dcol) }"
              :style="{
                '--col-accent': dcol.color || 'var(--t-primary)',
                '--col-tint': dcol.color
                  ? `color-mix(in srgb, ${dcol.color} 6%, var(--t-surface-alt))`
                  : 'var(--t-surface-alt)',
              }"
            >
              <!-- Collapsed strip: rotated title + count, click to expand. The real
                   header + card list stay in the DOM below (hidden via CSS) so the
                   drop target survives. -->
              <div v-if="colCollapsedNow(dcol)" class="col-strip" title="Развернуть колонку" @click="toggleCollapse(dcol)">
                <n-icon class="strip-chevron" :component="ChevronForwardOutline" />
                <span class="strip-title">{{ dcol.name }}</span>
                <span class="strip-count">{{ (lists[dcol.key] || []).length }}</span>
              </div>
              <ColumnHeader
                :dcol="dcol"
                :count="(lists[dcol.key] || []).length"
                :estimate="columnEstimate(dcol)"
                :editable="groupMode === 'status'"
                :is-done="groupMode === 'status' && dcol.key === doneColumnId"
                :first="groupMode === 'status' && dcol.key === (colModel[0] && colModel[0].key)"
                :collapsed="isColCollapsed(dcol)"
                @changed="onChanged"
                @set-done="onSetDone"
                @toggle-collapse="toggleCollapse(dcol)"
              />
              <draggable
                :list="lists[dcol.key]"
                group="tasks"
                item-key="id"
                class="drop"
                ghost-class="ghost"
                filter=".add-sub, .sub-add-input"
                :prevent-on-filter="false"
                :disabled="archivedMode"
                :animation="150"
                :delay="160"
                :delay-on-touch-only="true"
                :touch-start-threshold="6"
                @start="onDragStart"
                @end="onDragEnd"
                @change="onColChange($event, dcol)"
              >
                <template #item="{ element, index }">
                  <div :ref="(el) => regCard(el, element.id)" class="card-wrap">
                    <div
                      v-if="colCollapsedNow(dcol) || !(vis[element.id] ?? index < 12)"
                      class="card-ph"
                      :style="{ height: (cardH[element.id] || VCARD_EST) + 'px' }"
                    />
                    <TaskCard
                      v-else
                      :task="element"
                      :subtasks="sortedSubtasksByParent[element.id] || []"
                      :subtasks-expanded="subtasksExpanded"
                      :dragging="dragging"
                      :columns="columns"
                      :tags-map="tagsMap"
                      :members-map="membersMap"
                      :tags="tagsList"
                      :tag-prefix-names="tagPrefixNames"
                      :members="membersList"
                      :gitlab-members="gitlabMembersList"
                      :milestones-map="milestonesMap"
                      :ws-id="wsStore.currentId"
                      :project-id="board?.project_id"
                      :field-vis="fieldVis"
                      :show-empty="showEmpty"
                      :stack-fields="stackFields"
                      :card-size="cardSize"
                      :readonly="archivedMode"
                      @open="openTask"
                      @changed="onChanged"
                      @restore="restoreFromArchive"
                    />
                  </div>
                </template>
              </draggable>

              <template v-if="!archivedMode">
                <div v-if="addingInColumn === dcol.key" class="add-task-input">
                  <n-input
                    :ref="(el) => (taskInput = el)"
                    v-model:value="newTaskTitle"
                    type="textarea"
                    size="small"
                    :autosize="{ minRows: 1, maxRows: 4 }"
                    placeholder="Название задачи, Enter — создать"
                    @keyup.enter.prevent="submitAddTask(dcol)"
                    @keyup.esc="cancelAddTask"
                    @blur="submitAddTask(dcol)"
                  />
                </div>
                <n-button v-else text size="tiny" class="add-btn" @click="startAddTask(dcol)">
                  ＋ Создать задачу
                </n-button>
              </template>
            </div>
          </template>
        </draggable>

        <!-- inline new column (status mode) -->
        <div v-if="groupMode === 'status'" class="add-col" :class="{ 'add-col-mobile': isMobile }">
          <n-input
            v-if="addingColumn"
            ref="colInput"
            v-model:value="newColumnName"
            size="small"
            placeholder="Название колонки"
            @keyup.enter="submitAddColumn"
            @keyup.esc="addingColumn = false"
            @blur="submitAddColumn"
          />
          <n-button v-else dashed block class="add-btn" @click="startAddColumn">
            ＋ Создать колонку
          </n-button>
        </div>
      </div>

      <div v-if="layout === 'board' && !displayColumns.length" class="empty-board">
        <n-text depth="3">
          {{
            groupMode === 'status'
              ? 'Нет колонок — создайте первую кнопкой «＋ Колонка».'
              : 'Нет тегов — добавьте в «Теги».'
          }}
        </n-text>
      </div>
    </div>

    <TaskModal
      :show="showTaskModal"
      :task-id="selectedTaskId"
      :ws-id="wsStore.currentId"
      :project-id="board?.project_id"
      :tags="tagsList"
      :tag-prefix-names="tagPrefixNames"
      :members="membersList"
      :gitlab-members="gitlabMembersList"
      :milestones="milestonesList"
      :gitlab-can-create="gitlabCanCreate"
      :gitlab-fetch-templates="gitlabFetchTemplates"
      :gitlab-integration-id="gitlabIntegrationId"
      :readonly="archivedMode"
      @update:show="(v) => v || closeTask()"
      @changed="onChanged"
      @open="openTask"
      @restore="restoreFromArchive"
    />

    <BoardActivityToasts ref="activityToasts" @open="openTask" />

    <BoardCustomizePanel
      v-model:show="customizeOpen"
      v-model:board-name="boardName"
      v-model:card-size="cardSize"
      v-model:stack-fields="stackFields"
      v-model:show-empty="showEmpty"
      v-model:auto-collapse-empty="autoCollapseEmpty"
      v-model:subtasks-expanded="subtasksExpanded"
      v-model:autosave-view="autosaveView"
      :field-vis="fieldVis"
      :facet-chips="facetChips"
      :add-options="addOptions"
      :current-view-name="currentViewName"
      :board-icon="board?.icon || ''"
      :board-color="board?.color || ''"
      :board-icon-mode="board?.icon_mode || 'badge'"
      @set-field="setFieldVis"
      @add-facet="onAddFacet"
      @remove-chip="removeChip"
      @chip-click="onChipClick"
      @update-board="updateBoard"
    />
  </n-spin>
</template>

<style scoped>
.subbar {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 12px;
  padding-bottom: 8px;
  border-bottom: 1px solid var(--t-border);
}
.subbar-spacer {
  flex: 1;
}
/* Sprint scope chip: a persistent, accent-tinted badge showing the active sprint. */
.ms-scope-chip {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  flex: none;
  height: 40px; /* match the composer bar's height */
  box-sizing: border-box;
  padding: 0 8px;
  border-radius: 6px;
  font-size: 12px;
  font-weight: 600;
  color: var(--t-primary);
  background: color-mix(in srgb, var(--t-primary) 14%, transparent);
  border: 1px solid color-mix(in srgb, var(--t-primary) 34%, transparent);
}
.ms-scope-label {
  max-width: 180px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.ms-scope-x {
  cursor: pointer;
  opacity: 0.75;
}
.ms-scope-x:hover {
  opacity: 1;
}
/* Archive banner: neutral amber tone to signal a read-only mode. */
.archive-chip {
  color: #b5792a;
  background: color-mix(in srgb, #e0922f 15%, transparent);
  border-color: color-mix(in srgb, #e0922f 38%, transparent);
}
/* Keep the loader vertically centred during the initial board load (the spin
   content would otherwise be empty → spinner pinned to the top before content). */
.board-spin :deep(.n-spin-content) {
  min-height: 72vh;
}
/* A custom board background must reach the content-area edges (up to the sidebar
   and the header), so bleed under the layout-content's 16px padding and fill the
   viewport height. The board's own surfaces (columns/composer) sit on top. */
.board-wrap.has-bg {
  margin: -16px;
  padding: 16px;
  min-height: calc(100vh - 53px);
  box-sizing: border-box;
}
/* Right-side toolbar buttons match the composer bar's height. */
.bar-btn {
  height: 40px;
  width: 40px;
}
/* The tool cluster slides off to the right (and yields its width) while the
   composer is expanded, giving the chips the full bar width. */
.bar-tools {
  display: flex;
  align-items: center;
  gap: 6px;
  flex: none;
  overflow: hidden;
  transition:
    max-width 0.28s ease,
    opacity 0.2s ease,
    transform 0.28s ease,
    margin 0.28s ease;
  max-width: 200px;
}
.bar-tools.hidden {
  max-width: 0;
  margin-left: -6px;
  opacity: 0;
  transform: translateX(16px);
  pointer-events: none;
}
/* composer bar */
.composer {
  position: relative;
  flex: 1;
  min-width: 0;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  /* Inter-chip and inter-row gaps + vertical padding match the 8px horizontal
     padding, so a multi-row (expanded / overflowing) bar isn't cramped. */
  gap: 8px;
  box-sizing: border-box;
  min-height: 40px;
  padding: 8px;
  border: 1px solid var(--t-border);
  border-radius: 8px;
  background: var(--t-surface);
  transition: max-height 0.28s ease;
  /* Tame the inherited (Naive) line-height: without this the chips compute to
     ~23px tall — past the 22px add button — so the bar's natural height is ~41px.
     Collapsed clamps it to 40px and focus/expand (which drops the clamp) then made
     the bar jump down 1px. A tight line-height keeps the natural height at 40px. */
  line-height: 1.25;
}
/* Reserve room on the right for the absolutely-positioned clear-×. */
.composer.has-clear {
  padding-right: 26px;
}
/* Collapsed: capped to one row; chips wrap as whole pills (overflow rows are
   clipped) rather than being cut mid-chip (tap to expand). */
.composer.collapsed {
  max-height: 40px;
  overflow: hidden;
  cursor: pointer;
}
/* Collapsed: any tap (even on a chip / add / clear / search) only expands the
   bar — fishing for a blank spot to expand a chip-filled bar was fiddly. Killing
   pointer events on the children lets the click fall through to .composer's
   expand handler; the children act normally once expanded. */
/* Smoothly fade the dim in/out as the bar collapses/expands (focus). */
.composer > * {
  transition: opacity 0.25s ease;
}
.composer.collapsed > * {
  pointer-events: none;
  /* Dimmed while collapsed so it reads as one tap-to-expand surface (the chips
     aren't individually actionable until expanded). */
  opacity: 0.62;
}
.facet {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  height: 22px; /* match the adjacent "add filter" (＋) button */
  box-sizing: border-box;
  padding: 0 4px 0 9px;
  border-radius: 6px;
  background: var(--t-hover);
  color: var(--t-text2);
  white-space: nowrap;
}
.facet.group {
  background: color-mix(in srgb, var(--t-primary) 14%, transparent);
  color: var(--t-text1);
  cursor: pointer;
  padding-right: 9px;
}
.facet.sortable {
  cursor: pointer;
}
.facet-x {
  border: none;
  background: none;
  cursor: pointer;
  color: var(--t-text3);
  font-size: 14px;
  line-height: 1;
  padding: 0 3px;
  border-radius: 50%;
}
.facet-x:hover {
  color: var(--t-text1);
  background: var(--t-border);
}
.facet-add {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  border-radius: 6px;
  border: 1px dashed var(--t-border);
  background: none;
  color: var(--t-primary);
  cursor: pointer;
  flex: none;
}
.facet-add:hover {
  border-color: var(--t-primary);
}
.composer-search {
  flex: 1;
  min-width: 120px;
  border: none;
  background: none;
  outline: none;
  color: var(--t-text1);
  font-size: 13px;
  padding: 2px 4px;
}
.composer-search::placeholder {
  color: var(--t-text3);
}
.facet-clear {
  position: absolute;
  right: 5px;
  top: 50%;
  transform: translateY(-50%);
  border: none;
  background: none;
  cursor: pointer;
  color: var(--t-text3);
  font-size: 16px;
  line-height: 1;
  padding: 0 4px;
}
.facet-clear:hover {
  color: var(--t-text1);
}
.sb-prefix {
  width: 170px;
  flex: none;
}
.sort-pop {
  width: 280px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.sort-row {
  display: grid;
  grid-template-columns: 16px 1fr 1fr 22px;
  gap: 6px;
  align-items: center;
}
.sort-ord {
  font-size: 11px;
  color: var(--t-text3);
  text-align: center;
}
.sort-empty {
  font-size: 12px;
}
.sort-add {
  margin-top: 2px;
  align-self: flex-start;
}
.views-pop {
  width: 240px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.view-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}
.view-name {
  flex: 1;
  text-align: left;
  border: none;
  background: none;
  cursor: pointer;
  padding: 6px 8px;
  border-radius: 6px;
  color: var(--t-text1);
  font-size: 13px;
}
.view-name:hover {
  background: var(--t-hover);
}
.view-name.active {
  color: var(--t-primary);
  font-weight: 600;
}
.views-empty {
  font-size: 12px;
}
.views-over {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
}
.views-lbl {
  font-size: 11px;
  width: 100%;
}
.view-chip {
  border: 1px solid var(--t-border);
  background: var(--t-hover);
  border-radius: 999px;
  padding: 2px 8px;
  font-size: 11px;
  cursor: pointer;
  color: var(--t-text2);
}
.view-chip:hover {
  border-color: var(--t-primary);
  color: var(--t-primary);
}
.task-search {
  width: 220px;
  max-width: 40%;
}
/* Mobile: drop the button labels (icons only) and let the search fill the rest,
   so the sub-toolbar fits the screen instead of overflowing off the right. */
@media (max-width: 768px) {
  /* mobile layout-content padding is 12px (see AppLayout) */
  .board-wrap.has-bg {
    margin: -12px;
    padding: 12px;
    min-height: calc(100dvh - 53px);
  }
  .sb-label {
    display: none;
  }
  .subbar-spacer {
    display: none;
  }
  .task-search {
    flex: 1;
    min-width: 0;
    max-width: none;
    margin-left: 4px;
  }
  /* On mobile the search is shrinkable (min-width:0) so it never forces a wrap
     onto a second row: the collapsed bar stays one line (chips only — search
     hidden), and expanding just reclaims the width freed by the hiding tools,
     growing the bar rightward rather than down. */
  .composer-search {
    min-width: 0;
  }
  .composer.collapsed .composer-search {
    display: none;
  }
}
.vp {
  width: 240px;
  display: flex;
  flex-direction: column;
  gap: 6px;
  max-height: 70vh;
  overflow-y: auto;
  /* overflow-y:auto forces overflow-x to clip; pad so the focus ring on the
     left edge of the select / checkboxes isn't cut off. */
  padding: 3px 4px;
}
.vp-div {
  margin: 8px 0 2px;
}
.vp-fhead {
  display: flex;
  align-items: center;
  justify-content: flex-end;
}
.flbl {
  font-size: 12px;
  margin-top: 6px;
}
.flbl-0 {
  margin-top: 0;
}
.board-scroll {
  display: flex;
  gap: 12px;
  align-items: flex-start;
  overflow-x: auto;
  padding-bottom: 8px;
}
.cols {
  display: flex;
  gap: 12px;
  align-items: flex-start;
}
.add-col {
  box-sizing: border-box;
  flex: 0 0 220px;
}
.add-col-mobile {
  flex: 0 0 var(--col-w, 220px);
}

/* Mobile: snap each column to the start for a smooth one-at-a-time swipe, and
   stretch the columns to fill the screen so the whole area is a drop target
   (offset ≈ header + sub-toolbar; tune if a gap/extra scroll appears). */
@media (max-width: 768px) {
  .board-scroll {
    scroll-snap-type: x mandatory;
    scroll-behavior: smooth;
    align-items: stretch;
    height: calc(100vh - 130px);
    height: calc(100dvh - 130px);
  }
  .cols {
    align-items: stretch;
  }
  .col,
  .add-col-mobile {
    scroll-snap-align: start;
  }
  .col {
    align-self: stretch;
    max-height: none;
  }
}
.add-task-input {
  margin-top: 4px;
}
.col {
  box-sizing: border-box;
  width: var(--col-w, 280px);
  flex: 0 0 var(--col-w, 280px);
  border-radius: 14px;
  padding: 10px;
  align-self: flex-start;
  max-height: calc(100vh - 180px);
  display: flex;
  flex-direction: column;
  /* Same-hue diagonal of the column's accent for its top bar. */
  --col-grad: linear-gradient(
    to top right,
    color-mix(in srgb, var(--col-accent) 86%, #000),
    var(--col-accent) 50%,
    color-mix(in srgb, var(--col-accent) 86%, #fff)
  );
  /* Light 1px border on the sides/bottom (like the cards), plus a 3px gradient
     TOP accent: the top border is transparent and reveals the gradient painted
     on the border-box (wrapping the rounded top corners); the other borders stay
     the opaque neutral colour, and padding-box carries a soft same-hue wash of the
     column colour (a reference tracker-style, ~6%; neutral columns fall back to flat surface). */
  border: 1px solid var(--t-border);
  border-top: 3px solid transparent;
  background:
    linear-gradient(var(--col-tint, var(--t-surface-alt)), var(--col-tint, var(--t-surface-alt))) padding-box,
    var(--col-grad) border-box;
}
/* Collapsed column → a narrow strip. The header, card list and add-task button
   stay mounted (the SortableJS drop target must survive) but are hidden; the
   `.col-strip` overlay shows the rotated title + count. Collapsed columns stay
   collapsed during a drag; a full-column drop overlay (below) lets a dragged card
   land on the strip and get inserted at the top (see onColChange). */
.col.collapsed {
  position: relative;
  width: 44px;
  flex: 0 0 44px;
  padding: 8px 2px;
  cursor: pointer;
  overflow: hidden;
}
.col.collapsed :deep(.col-head),
.col.collapsed .drop,
.col.collapsed .add-btn,
.col.collapsed .add-task-input {
  display: none;
}
/* While dragging, reveal the collapsed column's card list as a transparent
   full-column drop zone laid over the strip so SortableJS can accept the drop. */
.cols.dragging .col.collapsed .drop {
  display: block;
  position: absolute;
  inset: 0;
  z-index: 2;
}
/* …but don't render a card preview inside the strip: the placeholders and the
   dragged card SortableJS relocates here stay invisible, so the collapsed column
   just acts as a plain drop zone (the drop still lands — see onColChange). */
.cols.dragging .col.collapsed .drop > * {
  visibility: hidden;
}
.col-strip {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 4px 0;
}
.strip-chevron {
  font-size: 15px;
  color: var(--t-text3);
  flex: none;
}
.strip-title {
  writing-mode: vertical-rl;
  transform: rotate(180deg);
  font-weight: 600;
  color: var(--t-text1);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-height: 260px;
}
.strip-count {
  font-size: 12px;
  color: var(--t-text3);
  background: var(--t-hover);
  border-radius: 10px;
  padding: 0 7px;
  flex: none;
}
/* Mobile: columns are one-per-screen, so collapse to a strip would be awkward —
   keep full width and hide the strip. */
@media (max-width: 768px) {
  .col.collapsed {
    width: var(--col-w, 280px);
    flex: 0 0 var(--col-w, 280px);
    padding: 10px;
  }
  .col.collapsed :deep(.col-head),
  .col.collapsed .drop,
  .col.collapsed .add-btn {
    display: revert;
  }
  .col-strip {
    display: none;
  }
}
.col-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;
  padding: 0 2px;
}
.col-grip {
  cursor: grab;
  color: var(--t-text3);
  font-size: 12px;
  line-height: 1;
}
.col-title {
  flex: 1;
  font-weight: 600;
  color: var(--t-text1);
}
.col-menu {
  font-size: 16px;
  line-height: 1;
}
.col-swatches {
  display: flex;
  gap: 8px;
  margin-top: 12px;
}
.cw {
  width: 24px;
  height: 24px;
  border-radius: 50%;
  border: 2px solid transparent;
  cursor: pointer;
}
.cw.active {
  border-color: var(--t-text1);
}
.cs-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.count {
  font-size: 12px;
  color: var(--t-text3);
  background: var(--t-hover);
  border-radius: 10px;
  padding: 0 7px;
}
.drop {
  flex: 1;
  overflow-y: auto;
  min-height: 8px;
  /* overflow-y:auto forces overflow-x to clip; pad so a focused subtask/task
     input's left border ring isn't cut off. */
  padding: 0 3px;
}
.ghost {
  opacity: 0.5;
}
/* Off-screen card stand-in: occupies the card's last-measured height so the
   scrollbar and drop positions stay correct while the heavy TaskCard is unmounted
   (see the IntersectionObserver windowing in the script). */
.card-ph {
  pointer-events: none;
}
.add-btn {
  margin-top: 6px;
  width: 100%;
  justify-content: center;
  text-transform: uppercase;
  font-size: 11px;
  letter-spacing: 0.4px;
  color: var(--t-text3);
}
.empty-board {
  padding: 24px;
}
</style>
