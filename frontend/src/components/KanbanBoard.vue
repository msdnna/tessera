<script setup>
import {
  ref,
  shallowRef,
  markRaw,
  reactive,
  computed,
  toRef,
  watch,
  onMounted,
  onBeforeUnmount,
  nextTick,
  h,
} from 'vue'
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
  ArchiveOutline,
  GitBranchOutline,
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
import { useBoardViewStore, defaultFieldVis } from '@/stores/boardView'
import { useThemeStore } from '@/stores/theme'
import { useAuthStore } from '@/stores/auth'
import { useRealtime } from '@/composables/useRealtime'
import { useResponsive } from '@/composables/useResponsive'
import { useBoardDragScroll } from '@/composables/useBoardDragScroll'
import { useCardViewport, cardKey, VCARD_EST } from '@/composables/useCardViewport'
import { useBoardViewConfig } from '@/composables/useBoardViewConfig'
import { useBoardFacets, CHIP_ICONS } from '@/composables/useBoardFacets'
import { tagParts, metaPrefixesFromRules } from '@/utils/tagGroups'
import { sumEstimates, formatEstimate } from '@/utils/estimation'
import { filterBoardTasks } from '@/utils/taskFilter'
import { classifyEvent, applyTaskPatch, applySubtaskPatch } from '@/utils/boardEvents'
import { emptyFilters, cloneFilters } from '@/utils/facetKeys'
import { planColumnReorder, planColDrop } from '@/utils/boardDnd'
import { BACKLOG_SCOPE, matchesScope } from '@/utils/milestones'
import { normalizeTitle } from '@/utils/title'
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
import UserAvatar from './UserAvatar.vue'

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

// Board context lives in the store (single owner): cards and the task modal read
// it from there instead of taking it as props. These bindings keep the local
// names — the maps are the store's reactive objects, written in place by
// loadBoardMeta; the refs are the store's own, so assignment writes through.
const {
  board,
  columns,
  metaTagPrefixes,
  tagsList,
  membersList,
  milestonesList,
  gitlabMembersList,
  gitlabCanCreate,
  gitlabFetchTemplates,
  gitlabIntegrationId,
  gitlabCanGroup,
  cardSize,
  stackFields,
  showEmpty,
} = storeToRefs(boardViewStore)
const tagsMap = boardViewStore.tagsMap
const membersMap = boardViewStore.membersMap
const gitlabMembersMap = boardViewStore.gitlabMembersMap
const milestonesMap = boardViewStore.milestonesMap
const tagPrefixNames = boardViewStore.prefixNames
const fieldVis = boardViewStore.fieldVis

const loading = ref(false)
// Board rows are replaced wholesale, never patched in place, so deep reactivity here
// buys nothing and costs a Proxy per row per load (thousands of them on a large
// board). The rows themselves are markRaw'd on load, which keeps them unproxied
// inside `lists` too. `lists` stays a plain ref on purpose: vuedraggable binds it
// via `:list` and mutates the column arrays in place, so those must stay reactive.
const allTasks = shallowRef([])
const subtasksByParent = shallowRef({})
const lists = ref({})
// Tessera user id → their GitLab login, for the reverse direction: a GitLab-synced
// task has no `created_by`, so matching its author against a Tessera person goes
// through this map (see utils/boardFilters matchesAuthor).
const glLoginByUserId = computed(() => {
  const m = {}
  for (const g of Object.values(gitlabMembersMap)) {
    if (g.tessera_user_id && g.gl_username) m[g.tessera_user_id] = g.gl_username
  }
  return m
})

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
// cardSize / stackFields / showEmpty / fieldVis are owned by the store (cards read
// them from there) but remain part of this board's saved view — snapshotToolbar and
// loadCustomize below read and write them exactly as before.
const autosaveView = ref(false) // auto re-save the loaded named view on change
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
// Composer bar: always a single row. The right-side tools slide away (yielding
// their width to the chips) only once the chips + a reserved slice for search
// text fill ≥75% of the bar's full width — measured, not hover-driven. Each chip
// is pale on its own and turns vivid only under the cursor (search: on focus).
const composerExpanded = ref(false)
const subbarEl = ref(null)
const composerEl = ref(null)
const searchEl = ref(null)
let composerRO = null
function recomputeComposerFit() {
  const bar = subbarEl.value
  const search = searchEl.value
  if (!bar || !search) return
  // Chips are nowrap and precede the search input, so the input's offset within
  // the composer equals the chips' total width. Reserve room for typed search.
  const chipsW = search.offsetLeft
  const RESERVED_SEARCH = 170
  composerExpanded.value = chipsW + RESERVED_SEARCH >= bar.clientWidth * 0.75
}
const groupMode = ref('status') // 'status' | 'tag'
const tagPrefix = ref('') // when grouping by tag: only tags with this namespace prefix become columns
// Friendly display names for tag prefixes (canonical prefix → label, owned by the
// store) are loaded per-project. Falls back to the raw prefix where no name is
// configured.

// Tags that become columns in tag-grouping mode (filtered by the chosen prefix).
const groupTags = computed(() =>
  tagPrefix.value
    ? tagsList.value.filter((t) => (t.name || '').startsWith(tagPrefix.value))
    : tagsList.value,
)

// Archive scope: ?archived=1 shows the board's archived tasks read-only (no DnD, no
// create, no inline edits) with a Restore action. Reuses all board filters/grouping.
const archivedMode = computed(() => route.query.archived === '1')
function exitArchive() {
  const q = { ...route.query }
  delete q.archived
  router.replace({ query: q })
}

// Sprint scope (navigation overlay): ?milestone=<slug|uuid|backlog>. Drives the
// server-side task scope and shows a removable chip; clearing it returns the full
// board (and de-highlights the sidebar sprint node). The value is passed to the
// API verbatim — the server resolves slug or UUID.
const milestoneScope = computed(() => (route.query.milestone ? String(route.query.milestone) : ''))
const milestoneScopeLabel = computed(() => {
  const s = milestoneScope.value
  if (!s) return ''
  if (s === BACKLOG_SCOPE) return 'Бэклог'
  // Keyed by id, so a slug scope needs a scan (a project has few milestones).
  return (milestonesMap[s] || milestonesList.value.find((m) => matchesScope(m, s)))?.title || 'Этап'
})
function clearMilestoneScope() {
  const q = { ...route.query }
  delete q.milestone
  router.replace({ query: q })
}
// ── composer facets: filters, multi-level sort, chips and the add menu ──
// The facet layer (filter state + the menus and chips over it) lives in
// `useBoardFacets`; grouping stays here because it builds the board's columns.
// The sprint scope lives in the URL, so the composable takes it as a value plus a
// clear-callback rather than reaching for the router itself.
const {
  filters,
  sortLevels,
  sortFieldLabel,
  cmpLevel,
  resetFilters,
  facetChips,
  filterChips,
  groupChip,
  addOptions,
  onAddFacet,
  removeChip,
  onChipClick,
  toggleSortDir,
  removeSort,
  hasClearableFacets,
  clearAll,
} = useBoardFacets({
  groupMode,
  tagPrefix,
  timelineLike,
  columns,
  allTasks,
  tagsList,
  membersList,
  milestonesList,
  gitlabMembersList,
  tagsMap,
  membersMap,
  milestonesMap,
  tagPrefixNames,
  glLoginByUserId,
  milestoneScope,
  clearMilestoneScope,
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
  // Assignee-filter rows carry avatar hints — draw the user's face + name like the
  // on-card assignee picker (both desktop and mobile).
  if (option.avatarUserId || option.avatarSrc) {
    const av = h(UserAvatar, {
      class: 'flt-asgn-av',
      userId: option.avatarUserId,
      src: option.avatarSrc,
      name: option.label,
    })
    const txt = h('span', { class: 't-fdd-txt' }, option.label)
    if (!isMobile.value) return h('span', { class: 'flt-asgn' }, [av, txt])
    return h(
      'span',
      { class: 'flt-asgn t-fdd-row', style: { animation: `t-fdd-${addDir.value} .17s ease` } },
      [av, txt],
    )
  }
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
function toggleSubtasksExpanded() {
  subtasksExpanded.value = !subtasksExpanded.value
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
// Only the *shape* of that state lives here (what a toolbar snapshot holds and how
// it is applied); the storage key, the per-layout slots, the debounced write and the
// restoring/swapping mutex live in `useBoardViewConfig`.
function defaultToolbar(forLayout) {
  return {
    groupMode: forLayout === 'timeline' || forLayout === 'gantt' ? 'assignee' : 'status',
    tagPrefix: '',
    sortLevels: [],
    subtasksExpanded: false,
    autoSort: false,
    filters: emptyFilters(),
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
    filters: cloneFilters(filters),
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
  Object.assign(filters, cloneFilters(s.filters))
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

// Rebuild a pre-per-layout blob into a toolbar snapshot. Built on top of the
// defaults so every key the old blob predates (milestones, autoSort, colCollapse,
// cardSize, fieldVis, …) is present rather than left to loadToolbar's own
// defaulting to paper over.
function migrateToolbar(v, forLayout) {
  return {
    ...defaultToolbar(forLayout),
    groupMode: v.groupMode || defaultToolbar(forLayout).groupMode,
    tagPrefix: v.tagPrefix || '',
    sortLevels: Array.isArray(v.sortLevels)
      ? v.sortLevels
      : v.sortBy && v.sortBy !== 'position'
        ? [{ field: v.sortBy, dir: v.sortDir || 'asc' }]
        : [],
    subtasksExpanded: !!v.subtasksExpanded,
    filters: cloneFilters(v.filters),
  }
}

const {
  restoreView,
  persistView,
  isGuarded: viewLoadInFlight,
  guard: withViewLoad,
} = useBoardViewConfig({
  boardId: toRef(props, 'boardId'),
  layout,
  defaults: defaultToolbar,
  snapshot: snapshotToolbar,
  load: loadToolbar,
  migrate: migrateToolbar,
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
// Re-measure the composer fit when the chip set changes (add/remove a filter,
// sort, group, scope, or the subtasks toggle); resize is handled by the observer.
watch([facetChips, archivedMode, milestoneScope, subtasksExpanded], () =>
  nextTick(recomputeComposerFit),
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
    filters: cloneFilters(filters),
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
  Object.assign(filters, cloneFilters(c.filters))
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
  withViewLoad(() => {
    applyViewConfig(v.config)
    currentViewName.value = v.name
    showLoadView.value = false
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
const AUTOSAVE_MS = 700 // longer than VIEW_PERSIST_MS: this one costs a request
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
    if (!sig || viewLoadInFlight()) return
    if (autosaveTimer) clearTimeout(autosaveTimer)
    autosaveTimer = setTimeout(() => {
      autosaveTimer = null
      autosaveCurrent()
    }, AUTOSAVE_MS)
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
    (groupMode.value === 'status' ? ADD_COL_W + GAP : 0) + (n - 1) * GAP + 4 + nCollapsed * STRIP_W
  const fit = Math.floor((cw - reserved) / nExpanded)
  // Fill the viewport while comfortable; the width band comes from the chosen card
  // size (compact narrower, large wider), so columns never get too narrow to hold
  // their fields and scroll horizontally once they'd shrink past the band's min.
  //
  // A `fit`-wide column always fits the row (that's how `reserved` is derived);
  // clamping UP to the band min is what can overrun the board by a couple px and raise
  // a full-width horizontal scrollbar for columns that essentially fit. So only clamp
  // up — and let the board scroll — once columns would get *meaningfully* narrower than
  // the min; within a gap's worth of it, keep the fitting width so no stray scrollbar
  // appears when everything is already visible (#2743).
  if (fit > cardCap.value) return cardCap.value
  if (fit >= cardMin.value - GAP) return fit
  return cardMin.value
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

// How long our own mutations keep realtime-driven reloads muted: long enough to
// cover the round-trip plus the echo of our own broadcast, short enough that a
// concurrent edit by someone else still lands promptly.
const SUPPRESS_RELOAD_MS = 1500
let suppressReloadUntil = 0
function suppress() {
  suppressReloadUntil = Date.now() + SUPPRESS_RELOAD_MS
}

// Edge auto-scroll + the shared drag flags live in `useBoardDragScroll`; the board
// only supplies the scrolling element and the column stride.
const { dragging, draggingCard, onDragStart, onCardDragStart, onDragEnd } = useBoardDragScroll({
  scrollEl: boardScroll,
  colWidth,
  gap: GAP,
})
// Card-list windowing. Frozen while a drag is in flight so SortableJS sees a
// stable DOM, hence the dependency on `dragging` above.
const { vis, cardH, regCard, reset: resetCardViewport } = useCardViewport({ frozen: dragging })

// Coalesces the burst of realtime events one action produces into a single reload.
// Full reload — used for a resync (dropped event / reconnect), where the board has
// no idea what it missed.
//
// Всегда `silent`: доска уже на экране, пользователь только что сам её потрогал
// (перетащил карточку, закрыл модалку). Гасить её на время догрузки — то самое
// мерцание из #2695, см. комментарий у `load()`.
const RELOAD_DEBOUNCE_MS = 200
let reloadTimer = null
function scheduleReload() {
  clearTimeout(reloadTimer)
  reloadTimer = setTimeout(() => load(props.boardId, { silent: true }), RELOAD_DEBOUNCE_MS)
}

// Partial reloads, one debounce timer per kind: a burst of task events must not
// drag the columns and the workspace meta along with it. Tasks get a slightly
// wider window because one user action fans out into several task events
// (task.updated + task.tagged + task.assigned) that should collapse into one fetch.
const PARTIAL_DEBOUNCE_MS = { tasks: 400, columns: 200, board: 200, meta: 200 }
const partialTimers = { tasks: null, columns: null, board: null, meta: null }
const partialFetch = {
  tasks: () => fetchTasks(props.boardId),
  columns: async () => {
    columns.value = (await boards.columns(props.boardId)).data || []
  },
  board: async () => {
    board.value = (await boards.get(props.boardId)).data
  },
  meta: () => loadWorkspaceMeta(),
}
function schedulePartial(kind) {
  clearTimeout(partialTimers[kind])
  partialTimers[kind] = setTimeout(async () => {
    try {
      await partialFetch[kind]()
    } catch (e) {
      message.error(e.message)
    }
  }, PARTIAL_DEBOUNCE_MS[kind])
}

// Sprint navigation: the URL ?milestone=<slug|uuid|backlog> scopes the board to one
// milestone server-side, so a huge project never loads all its cards at once.
// ?archived=1 loads the read-only archive instead (subtasks skipped — they are
// archived together with their parents). Shared by the full load and the
// tasks-only realtime refetch, which must scope identically or the archive view
// would come back to life full of ordinary tasks.
function taskQuery() {
  const archived = route.query.archived === '1'
  const ms = route.query.milestone
  return { archived, params: archived ? { archived: 1 } : ms ? { milestone: ms } : undefined }
}

// Fetch just the task lists (2 requests instead of the full load's 10).
async function fetchTasks(id) {
  const { archived, params } = taskQuery()
  const [t, s] = await Promise.all([
    boards.tasks(id, params),
    archived ? Promise.resolve({ data: [] }) : boards.subtasks(id),
  ])
  allTasks.value = (t.data || []).map(markRaw)
  const byParent = {}
  for (const sub of s.data || []) (byParent[sub.parent_id] ||= []).push(markRaw(sub))
  subtasksByParent.value = byParent
}

// `silent` — перезагрузить данные, НЕ поднимая `loading`.
//
// Задача #2695: `loading` включает `n-spin`, а Naive гасит ВСЁ содержимое своего
// слота — `.n-spin-content { transition: opacity .3s }` → `opacity: opacityDisabled`
// (`naive-ui/es/spin/src/styles/index.cssr.mjs`). В слоте у нас лежит вся доска
// целиком, поэтому каждая фоновая перезагрузка (после drag'а, после правки из
// модалки) прогоняла страницу через затухание и обратно. Это и есть «мерцание»:
// оно не в тостах и не в композиторе браузера, а в обычном CSS-переходе opacity
// у общего предка.
//
// Спиннер уместен только там, где показывать нечего — первая загрузка и смена
// доски. Догрузка уже показанной доски должна быть незаметной.
async function load(id, { silent = false } = {}) {
  if (!silent) loading.value = true
  try {
    const { archived, params } = taskQuery()
    const [b, c, t, s] = await Promise.all([
      boards.get(id),
      boards.columns(id),
      boards.tasks(id, params),
      archived ? Promise.resolve({ data: [] }) : boards.subtasks(id),
    ])
    board.value = b.data
    columns.value = c.data || []
    // Publish the identity as soon as the board itself is known: cards render
    // before loadWorkspaceMeta resolves, and a stale projectId from the previous
    // board would scope their tag creation / estimation config to the wrong one.
    boardViewStore.setContext(id, wsStore.currentId, b.data?.project_id || null)
    allTasks.value = (t.data || []).map(markRaw)
    const byParent = {}
    for (const sub of s.data || []) (byParent[sub.parent_id] ||= []).push(markRaw(sub))
    subtasksByParent.value = byParent
    await loadWorkspaceMeta()
    // No explicit rebuildLists() here: assigning allTasks already invalidates
    // filteredTasks, and the watcher below rebuilds once. Calling both made every
    // load rebuild the whole column map twice.
  } catch (e) {
    message.error(e.message)
  } finally {
    if (!silent) loading.value = false
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
  boardViewStore.refill(tagsMap, Object.fromEntries((tg.data || []).map((t) => [t.id, t])))
  boardViewStore.refill(milestonesMap, Object.fromEntries((ms.data || []).map((m) => [m.id, m])))
  boardViewStore.refill(membersMap, Object.fromEntries((mem.data || []).map((m) => [m.user_id, m])))
  boardViewStore.refill(
    gitlabMembersMap,
    Object.fromEntries((glMem.data || []).map((g) => [g.gl_user_id, g])),
  )
  // Issue-creation is offered only on the binding that targets THIS board.
  const gi = (glInt.data?.integrations || []).find((b) => b.board_id === props.boardId) || {}
  gitlabIntegrationId.value = gi.id || null
  gitlabCanCreate.value = gi.enabled === true && gi.writeback?.push_create === true
  gitlabFetchTemplates.value = gitlabCanCreate.value && gi.writeback?.fetch_templates === true
  // Grouping is its own flag, not a sub-option of push_create: an integration may push
  // subtasks into an existing issue hierarchy without allowing issue creation from tasks.
  gitlabCanGroup.value = gi.enabled === true && gi.writeback?.push_children === true
  // Prefixes whose tags are governed by a status/priority/meta GitLab rule — hidden
  // from tag pickers so they can't be toggled out of sync with the mapped field.
  // Collected across every integration that targets this board (multi-binding).
  const mp = new Set()
  for (const bi of (glInt.data?.integrations || []).filter((b) => b.board_id === props.boardId)) {
    for (const p of metaPrefixesFromRules(bi.label_rules)) mp.add(p)
  }
  metaTagPrefixes.value = mp
  boardViewStore.refill(
    tagPrefixNames,
    Object.fromEntries((pfx.data || []).map((p) => [p.prefix, p.label])),
  )
  boardViewStore.setContext(props.boardId, wsId, projectId)
}

// Filtering runs over tasks AND their subtasks together (see utils/taskFilter):
// a parent stays visible when it matches or when one of its children does, and
// in the latter case its on-card child list is narrowed to the matches.
const filterResult = computed(() =>
  filterBoardTasks({
    tasks: allTasks.value,
    subtasksByParent: subtasksByParent.value,
    filters,
    glLoginByUserId: glLoginByUserId.value,
  }),
)

// filter + sort applied before grouping
const filteredTasks = computed(() => sortByLevels(filterResult.value.tasks))

// Same map, filter-narrowed — for the views that consume it unsorted.
const filteredSubtasksByParent = computed(() => filterResult.value.subtasksByParent)

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
  const src = filterResult.value.subtasksByParent
  if (!sortLevels.value.length) return src
  const out = {}
  for (const [pid, subs] of Object.entries(src)) out[pid] = sortByLevels(subs)
  return out
})

// The board's task-completing column, or null when it has none. No fallback to
// the rightmost column: it made clearing the done column look like a no-op
// whenever that column was also the rightmost one (#2588).
const doneColumnId = computed(() => board.value?.done_column_id ?? null)
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
    ...groupTags.value.map((t) => ({ key: t.id, name: tagColumnName(t), color: t.color, tag: t })),
    { key: '__none__', name: 'Без тегов', color: '', tag: null },
  ]
})

// Column title for a tag group: the raw "scope::value" is never shown. When the
// board is already filtered to one prefix the scope sits in the group label
// ("Тег · Сложность"), so only the value is left; otherwise the friendly scope
// is spelled out ahead of it.
function tagColumnName(t) {
  const parts = tagParts(t.name, tagPrefixNames)
  if (!parts.hasScope) return parts.label
  return tagPrefix.value ? parts.label : `${parts.scope}: ${parts.label}`
}

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

// Mutable mirror of displayColumns for column drag-reorder (status mode only).
const colModel = ref([])
watch(displayColumns, (v) => (colModel.value = [...v]), { immediate: true })

async function onColumnReorder(evt) {
  const plan = planColumnReorder(evt, colModel.value, groupMode.value)
  if (!plan) return
  suppress()
  try {
    await columnsApi.move(plan.key, { before_id: plan.before_id, after_id: plan.after_id })
    scheduleReload()
  } catch (e) {
    message.error(e.message)
    load(props.boardId)
  }
}

// Drag persistence: the rules (neighbour math, collapsed pin, subtask promotion,
// single-value milestone, tag add/remove) live in utils/boardDnd; here we only
// dispatch the resulting intents to the API, in order.
async function onColChange(evt, dcol) {
  suppress()
  try {
    const collapsed = !!evt.added && isColCollapsed(dcol)
    const intents = planColDrop({
      groupMode: groupMode.value,
      evt,
      dcol,
      list: lists.value[dcol.key],
      collapsed,
    })
    for (const it of intents) {
      if (it.op === 'setParent') await tasksApi.setParent(it.id, it.parentId)
      else if (it.op === 'move')
        await tasksApi.move(it.id, {
          column_id: it.columnId,
          before_id: it.beforeId,
          after_id: it.afterId,
        })
      else if (it.op === 'setMilestone') await tasksApi.setMilestone(it.id, it.milestoneId)
      else if (it.op === 'clearMilestone') await tasksApi.clearMilestone(it.id)
      else if (it.op === 'addTag') await tasksApi.addTag(it.id, it.tagId)
      else if (it.op === 'removeTag') await tasksApi.removeTag(it.id, it.tagId)
    }
    scheduleReload()
  } catch (e) {
    message.error(e.message)
    load(props.boardId)
  }
}

// ── inline task creation ("+ New task" at column bottom) ──
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
  // Enter is caught on keydown so the textarea never gets to insert its newline;
  // normalizeTitle is the backstop for pasted multi-line text.
  const title = normalizeTitle(newTaskTitle.value)
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
  // Custom command dictionary edited elsewhere → the `/`-popup picks it up live.
  if (ev.type === 'workspace_commands.updated') wsStore.setCustomCommands(ev.data?.commands || [])
  // Board-activity toast for create/move on THIS board (any actor).
  if (ev.type === 'task.created' || ev.type === 'task.moved') pushActivity(ev)
  if (dragging.value || Date.now() < suppressReloadUntil) return
  // Route the event instead of reloading the whole board for every one of them.
  const kind = classifyEvent(ev, { boardId: props.boardId })
  if (kind === 'ignore') return
  if (kind === 'patch') {
    // A full task payload can be merged into the card with no request at all —
    // unless the card has to move or isn't here yet, in which case applyTaskPatch
    // returns null and we fall back to re-fetching the lists.
    // The merged row is a fresh plain object; markRaw keeps it consistent with
    // the rest of the rows (see the shallowRef note above).
    const patched = applyTaskPatch(allTasks.value, ev.data)
    if (patched) {
      allTasks.value = patched.map(markRaw)
      return
    }
    const subs = applySubtaskPatch(subtasksByParent.value, ev.data)
    if (subs) {
      for (const arr of Object.values(subs)) arr.forEach(markRaw)
      subtasksByParent.value = subs
      return
    }
    schedulePartial('tasks')
    return
  }
  schedulePartial(kind)
}, scheduleReload)

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
    boardViewStore.refill(milestonesMap, Object.fromEntries((data || []).map((m) => [m.id, m])))
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

onMounted(async () => {
  ro = new ResizeObserver(() => measure())
  if (boardScroll.value) {
    ro.observe(boardScroll.value)
    measure()
  }
  // Re-measure the composer fit on bar resize (window / sidebar toggle).
  composerRO = new ResizeObserver(recomputeComposerFit)
  if (subbarEl.value) composerRO.observe(subbarEl.value)
  nextTick(recomputeComposerFit)
  restoreView()
  await load(props.boardId)
  loadViews()
  applyTaskQuery()
})
onBeforeUnmount(() => {
  ro?.disconnect()
  // Drop any pending debounced reload so it can't fire against a board we've just
  // navigated away from (e.g. its project was deleted) and 404 with a stray toast.
  clearTimeout(reloadTimer)
  for (const t of Object.values(partialTimers)) clearTimeout(t)
  composerRO?.disconnect()
  onDragEnd()
  boardViewStore.reset()
})
watch(
  () => props.boardId,
  async (id) => {
    if (!id) return
    resetCardViewport()
    restoreView()
    await load(id)
    loadViews()
    applyTaskQuery()
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
    message.success('Задача возвращена из архива')
  } catch (e) {
    message.error(e.message)
  }
}
</script>

<template>
  <n-spin :show="loading" :rotate="false" class="board-spin">
    <template #icon><TesseraSpinner /></template>
    <div
      v-if="board"
      class="board-wrap"
      :class="{ 'has-bg': !!themeStore.boardBackground }"
      :style="boardBgStyle"
    >
      <!-- Sub-toolbar under the header: grouping / sort / filters / subtasks +
           a task-name search on the right. (Layout + Теги/Архив live in the
           global header now.) -->
      <div ref="subbarEl" class="subbar">
        <!-- Composer bar: scope (archive/sprint) + grouping / sort / filters as
             chips + an add menu + the name search, all in one single-row bar.
             Each chip is pale until hovered; the right-side tools slide away only
             once the chips fill ≥75% of the bar. Sort chips are drag-reorderable. -->
        <div
          ref="composerEl"
          class="composer"
          :class="{ 'has-clear': hasClearableFacets }"
          data-tour="board-composer"
        >
          <!-- Scope chips: archive = amber tint, sprint = accent tint (no border). -->
          <span v-if="archivedMode" class="facet facet-archive" title="Архив — только чтение">
            <n-icon class="facet-ic" :component="ArchiveOutline" :size="13" />
            Архив (только чтение)
            <button class="facet-x" title="Выйти из архива" @click.stop="exitArchive">×</button>
          </span>
          <span v-if="milestoneScope" class="facet facet-accent" title="Показан один этап">
            <n-icon class="facet-ic" :component="RibbonOutline" :size="13" />
            {{ milestoneScopeLabel }}
            <button class="facet-x" title="Сбросить этап" @click.stop="clearMilestoneScope">
              ×
            </button>
          </span>

          <!-- Subtask-expansion toggle (icon-only): accent when on, grey when off. -->
          <button
            class="facet subtasks-chip"
            :class="{ active: subtasksExpanded }"
            :title="subtasksExpanded ? 'Подзадачи раскрыты' : 'Раскрыть подзадачи'"
            @click.stop="toggleSubtasksExpanded"
          >
            <n-icon class="facet-ic" :component="GitBranchOutline" :size="14" />
          </button>

          <!-- Grouping chip (toggles status/tag grouping). -->
          <span
            v-if="groupChip"
            class="facet group"
            title="Переключить статусы/теги"
            @click="onChipClick(groupChip)"
          >
            <n-icon class="facet-ic" :component="groupChip.icon" :size="13" />
            {{ groupChip.text }}
          </span>

          <!-- Sort chips: click toggles direction, drag reorders primary/secondary. -->
          <draggable
            v-if="sortLevels.length"
            :list="sortLevels"
            item-key="field"
            handle=".facet"
            :animation="150"
            class="sort-chips"
            ghost-class="facet-ghost"
          >
            <template #item="{ element: l }">
              <span
                class="facet sortable"
                title="Клик — направление · перетащите для порядка"
                @click="toggleSortDir(l)"
              >
                <n-icon class="facet-ic" :component="CHIP_ICONS.sort" :size="13" />
                {{ sortFieldLabel(l.field) }} {{ l.dir === 'desc' ? '↓' : '↑' }}
                <button class="facet-x" title="Убрать" @click.stop="removeSort(l)">×</button>
              </span>
            </template>
          </draggable>

          <!-- Filter chips (priority / assignee / tag / status / milestone / due). -->
          <span v-for="(c, ci) in filterChips" :key="ci" class="facet">
            <n-icon class="facet-ic" :component="c.icon" :size="13" />
            {{ c.text }}
            <button class="facet-x" title="Убрать" @click.stop="removeChip(c)">×</button>
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
            ref="searchEl"
            v-model="filters.q"
            class="composer-search"
            data-testid="board-search"
            placeholder="Поиск по названию…"
          />
          <button
            v-if="hasClearableFacets"
            class="facet-clear"
            title="Сбросить всё"
            @click="clearAll"
          >
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
            {{
              autoActive
                ? 'Авто-сортировка по зависимостям (вкл.)'
                : 'Авто: сортировать по зависимостям'
            }}
          </n-tooltip>

          <n-tooltip v-if="!timelineLike">
            <template #trigger>
              <n-button
                size="small"
                quaternary
                class="ngrad bar-btn"
                :type="customizeOpen ? 'primary' : 'default'"
                data-tour="board-customize"
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
                {{
                  currentViewName ? `Представление: ${currentViewName}` : 'Загрузить представление'
                }}
              </n-tooltip>
            </template>
            <div class="views-pop">
              <div v-for="v in viewsForLayout" :key="v.id" class="view-row">
                <button
                  class="view-name"
                  :class="{ active: v.name === currentViewName }"
                  @click="applyView(v)"
                >
                  {{ v.name }}
                </button>
                <n-popconfirm
                  :positive-button-props="{ type: 'error' }"
                  positive-text="Удалить"
                  @positive-click="deleteView(v)"
                >
                  <template #trigger>
                    <n-button text size="tiny" type="error"
                      ><n-icon :component="TrashOutline"
                    /></n-button>
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
        :tag-prefix-names="tagPrefixNames"
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
        :subtasks-by-parent="filteredSubtasksByParent"
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
        :subtasks-by-parent="filteredSubtasksByParent"
        :milestones="milestonesList"
        @open="openTask"
        @changed="onChanged"
      />

      <BoardMatrixView
        v-else-if="layout === 'matrix'"
        :tasks="filteredTasks"
        :subtasks-by-parent="sortedSubtasksByParent"
        :subtasks-total-by-parent="subtasksByParent"
        :subtasks-expanded="subtasksExpanded"
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
              data-testid="column"
              :data-column-key="dcol.key"
              :data-column-name="dcol.name"
              :class="{ collapsed: colCollapsedNow(dcol) }"
              :style="{
                '--col-accent': dcol.color || 'var(--t-primary)',
                '--col-tint': dcol.color
                  ? `color-mix(in srgb, ${dcol.color} 6%, var(--t-surface-alt))`
                  : 'var(--t-surface-alt)',
                '--col-border': dcol.color
                  ? `color-mix(in srgb, ${dcol.color} 12%, var(--t-border))`
                  : 'var(--t-border)',
              }"
            >
              <!-- Collapsed strip: rotated title + count, click to expand. The real
                   header + card list stay in the DOM below (hidden via CSS) so the
                   drop target survives. -->
              <div
                v-if="colCollapsedNow(dcol)"
                class="col-strip"
                title="Развернуть колонку"
                @click="toggleCollapse(dcol)"
              >
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
                class="drop t-hoverscroll"
                ghost-class="ghost"
                filter=".add-sub, .sub-add-input"
                :prevent-on-filter="false"
                :disabled="archivedMode"
                :animation="150"
                :delay="160"
                :delay-on-touch-only="true"
                :touch-start-threshold="6"
                @start="onCardDragStart"
                @end="onDragEnd"
                @change="onColChange($event, dcol)"
              >
                <template #item="{ element, index }">
                  <div :ref="(el) => regCard(el, cardKey(dcol.key, element.id))" class="card-wrap">
                    <div
                      v-if="
                        colCollapsedNow(dcol) || !(vis[cardKey(dcol.key, element.id)] ?? index < 12)
                      "
                      class="card-ph"
                      :style="{
                        height: (cardH[cardKey(dcol.key, element.id)] || VCARD_EST) + 'px',
                      }"
                    />
                    <TaskCard
                      v-else
                      :task="element"
                      :subtasks="sortedSubtasksByParent[element.id] || []"
                      :subtasks-total="(subtasksByParent[element.id] || []).length"
                      :subtasks-expanded="subtasksExpanded"
                      :dragging="draggingCard"
                      :readonly="archivedMode"
                      @open="openTask"
                      @changed="onChanged"
                      @restore="restoreFromArchive"
                    />
                  </div>
                </template>
              </draggable>

              <template v-if="!archivedMode">
                <div
                  v-if="addingInColumn === dcol.key"
                  class="add-task-input"
                  data-testid="add-task-input"
                >
                  <n-input
                    :ref="(el) => (taskInput = el)"
                    v-model:value="newTaskTitle"
                    type="textarea"
                    size="small"
                    :autosize="{ minRows: 1, maxRows: 4 }"
                    placeholder="Название задачи, Enter — создать"
                    @keydown.enter.exact.prevent="submitAddTask(dcol)"
                    @keyup.esc="cancelAddTask"
                    @blur="submitAddTask(dcol)"
                  />
                </div>
                <n-button
                  v-else
                  text
                  size="tiny"
                  class="add-btn"
                  data-testid="add-task-button"
                  @click="startAddTask(dcol)"
                >
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
  </n-spin>

  <!-- Оверлеи держим ВНЕ <n-spin>: его слот целиком гаснет по opacity на время
       загрузки (см. `load()`), а модалка, тосты активности и панель настройки —
       не «содержимое доски», гаснуть вместе с ней они не должны. -->
  <TaskModal
    :show="showTaskModal"
    :task-id="selectedTaskId"
    :board-top-tasks="allTasks"
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
  flex-wrap: nowrap;
  align-items: center;
  gap: 8px;
  box-sizing: border-box;
  min-height: 40px;
  padding: 8px;
  border: 1px solid var(--t-border);
  border-radius: 8px;
  background: var(--t-surface);
  /* Single row; when the chips exceed the bar width (rare — the tools slide away
     to yield space) the bar scrolls horizontally rather than wrapping. */
  overflow-x: auto;
  overflow-y: hidden;
  scrollbar-width: thin;
  /* Tame the inherited (Naive) line-height so a chip stays 22px tall. */
  line-height: 1.25;
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
  /* Each chip is pale until hovered, so an idle bar reads calmly (no whole-bar
     dim/expand). */
  opacity: 0.6;
  transition: opacity 0.15s ease;
}
.facet:hover {
  opacity: 1;
}
.facet.group {
  background: color-mix(in srgb, var(--t-primary) 14%, transparent);
  /* Accent chip text matches its icon (accent, not near-black). */
  color: var(--t-primary);
  cursor: pointer;
  padding-right: 9px;
}
/* Sprint scope chip: accent tint, no border. */
.facet-accent {
  background: color-mix(in srgb, var(--t-primary) 14%, transparent);
  color: var(--t-primary);
}
.facet-accent .facet-ic {
  color: var(--t-primary);
  opacity: 1;
}
/* Archive scope chip: amber tint, no border. */
.facet-archive {
  background: color-mix(in srgb, #e0922f 15%, transparent);
  color: #b5792a;
}
.facet-archive .facet-ic {
  color: #b5792a;
  opacity: 1;
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
/* On accent chips the remove-× carries the chip's accent too (matches text/icon). */
.facet.group .facet-x,
.facet-accent .facet-x {
  color: var(--t-primary);
}
.facet-archive .facet-x {
  color: #b5792a;
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
  /* Pale until the caret is in it (matches the chips' hover behaviour). */
  opacity: 0.6;
  transition: opacity 0.15s ease;
}
.composer-search:focus {
  opacity: 1;
}
.composer-search::placeholder {
  color: var(--t-text3);
}
.facet-clear {
  flex: none;
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
/* Per-kind icon that replaces the text prefix on a chip. */
.facet-ic {
  flex: none;
  opacity: 0.8;
}
.facet.group .facet-ic {
  color: var(--t-primary);
  opacity: 1;
}
/* Sort chips share the composer gap so a run of them reads as one group. */
.sort-chips {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  flex: none;
}
/* Drag placeholder while reordering sort chips. */
.facet-ghost {
  opacity: 0.4;
}
/* Subtask-expansion toggle chip: plain grey when off, accent tint when on. */
/* Subtask-expansion toggle: icon-only, square-ish; grey off / accent on. */
.subtasks-chip {
  flex: none;
  border: none;
  cursor: pointer;
  font: inherit;
  padding: 0 6px;
}
.subtasks-chip.active {
  background: color-mix(in srgb, var(--t-primary) 16%, transparent);
  color: var(--t-text1);
}
.subtasks-chip.active .facet-ic {
  color: var(--t-primary);
  opacity: 1;
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
  /* On mobile the search is shrinkable so it never forces horizontal overflow. */
  .composer-search {
    min-width: 0;
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
  /* When a task is open in the fixed sidebar panel it covers the board's right edge;
     reserve exactly the panel's width of scroll slack so every column can be scrolled
     out from under it (#2743). Zero when no panel is open — no reserve, no scrollbar
     (a flex spacer would drag the `gap` in even at 0 width and force a stray scroll). */
  padding-right: var(--task-panel-w, 0px);
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
     column colour (~6%; neutral columns fall back to flat surface).
     The side/bottom border carries a muted tint of the column colour (~12%, a touch
     stronger than the wash so it doesn't melt into the fill); neutral columns keep
     the flat neutral border. */
  border: 1px solid var(--col-border, var(--t-border));
  border-top: 3px solid transparent;
  background:
    linear-gradient(var(--col-tint, var(--t-surface-alt)), var(--col-tint, var(--t-surface-alt)))
      padding-box,
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
