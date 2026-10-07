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
} from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
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
  useMessage,
} from 'naive-ui'
import {
  GitNetworkOutline,
  AddOutline,
  ChevronForwardOutline,
  ChevronDownOutline,
  FunnelOutline,
  OptionsOutline,
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
import { useBoardViews } from '@/composables/useBoardViews'
import { useBoardFacets, CHIP_ICONS } from '@/composables/useBoardFacets'
import { useFacetAddMenu } from '@/composables/useFacetAddMenu'
import { tagLaneLabel, tagInPrefix, metaPrefixesFromRules } from '@/utils/tagGroups'
import { sumLaneEffort, formatEstimate } from '@/utils/estimation'
import { countWithSubtasks } from '@/utils/boardCounts'
import { filterBoardTasks } from '@/utils/taskFilter'
import { useFormat } from '@/composables/useFormat'
import {
  classifyEvent,
  partialKinds,
  missedRefetchKinds,
  applyTaskPatch,
  applySubtaskPatch,
  relationTouchesBoard,
  boardGone,
  toastActor,
} from '@/utils/boardEvents'
import { emptyFilters, cloneFilters } from '@/utils/facetKeys'
import { planColumnReorder, planColDrop, canSortWithinLane } from '@/utils/boardDnd'
import { buildLists, collapseKey } from '@/utils/boardLists'
import { LAYOUTS } from '@/utils/boardViews'
import { withoutArchiveQuery, isArchiveQuery } from '@/utils/boardScope'
import { columnWidth, BOARD_GAP } from '@/utils/boardLayout'
import { BACKLOG_SCOPE, matchesScope, newTaskMilestoneId } from '@/utils/milestones'
import { columnName } from '@/utils/defaultNames'
import { normalizeTitle } from '@/utils/title'
import { moveFocus, neighbourColumn, planShift, planComplete } from '@/utils/cardNav'
import { doneTarget } from '@/utils/status'
import { todoColumn } from '@/utils/columnStatus'
import { taskBasePatch } from '@/utils/taskPatch'
import { humanizeError } from '@/utils/errors'
import { useShortcuts } from '@/composables/useShortcuts'
import { usePaletteStore } from '@/stores/palette'
import { useUndoStore } from '@/stores/undo'
import { storeToRefs } from 'pinia'
import TaskCard from './TaskCard.vue'
import TaskModal from './TaskModal.vue'
import BoardActivityToasts from './BoardActivityToasts.vue'
import TesseraSpinner from './TesseraSpinner.vue'
import ColumnHeader from './ColumnHeader.vue'
import BoardCustomizePanel from './BoardCustomizePanel.vue'
import ViewsSection from './boardViews/ViewsSection.vue'
import ViewChip from './boardViews/ViewChip.vue'
import BoardLayoutSwitch from './BoardLayoutSwitch.vue'
import ChartControls from './chart/ChartControls.vue'
import BoardMembers from './BoardMembers.vue'
import ShortcutKeys from './ShortcutKeys.vue'
import BoardListView from './BoardListView.vue'
import BoardCalendarView from './BoardCalendarView.vue'
import BoardMatrixView from './BoardMatrixView.vue'
import BoardTimelineView from './BoardTimelineView.vue'
import BoardGanttView from './BoardGanttView.vue'

const props = defineProps({ boardId: { type: String, required: true } })

const message = useMessage()
// `t` is the translator everywhere below; task/tag loop variables are spelled out
// (`task`, `tag`) so nothing shadows it (#2799).
const { t } = useI18n()
// Only for the «Срок» facet: a card the filter hides (or keeps) has to be the one
// whose own pill reads that way, and the pill renders through this formatter set.
const { formatters } = useFormat()
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
// The «Вид доски» panel (#2986) — docked right of the board in every view.
const customizeOpen = ref(false)
const customizePanel = ref(null)
watch([customizeOpen, isMobile], ([on, mobile]) => (boardViewStore.panelOpen = on && !mobile))
// Board-name mirror for the customize panel's rename input (kept in sync with the
// loaded board; committed via boards.update on blur/enter).
const boardName = ref('')
watch(board, (b) => (boardName.value = b?.name || ''))
// Save board meta (name / icon / colour / icon_mode) from the customize panel. We
// always send the full current meta so an icon change doesn't drop the name and
// vice-versa; the backend still merges tri-state, this just keeps it simple.
// The archive never gets here — the panel doesn't draw the fields that emit this
// (see BoardCustomizePanel). Kept as the guard on the call itself, the way
// BoardMatrixView's startAdd() refuses a stray call with its «＋» already hidden:
// the name input commits on blur, so any path that leaves a stale value behind
// would otherwise write it to the live board.
async function updateBoard(patch) {
  if (!board.value || archivedMode.value) return
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
// Where a lane's explicit override is stored — its key, except for the catch-all
// lane, whose key `__none__` two different groupings share (see collapseKey). The
// tag prefix goes in too: it decides which cards the tag catch-all holds, so each
// narrowing gets its own override.
function colCollapseKey(dcol) {
  return collapseKey(groupMode.value, dcol.key, tagPrefix.value)
}
// Effective collapsed state: explicit override wins, else the auto-empty rule.
function isColCollapsed(dcol) {
  const k = colCollapseKey(dcol)
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
  colCollapse[colCollapseKey(dcol)] = !isColCollapsed(dcol)
}
// Caption of the collapsed strip. The rotated title is clipped (max-height + an
// ellipsis), and the strip used to advertise a bare «expand the column» — so the
// one place where the name is unreadable was also the one place that never said it.
function stripTitle(dcol) {
  return t('board.column.expandTitle', { name: dcol.name })
}
// Turning "auto-collapse empty" ON must collapse EVERY empty column — including
// ones the user previously expanded by hand (a stale explicit `false` override
// would otherwise beat the auto rule, which reads as "the toggle does nothing").
// Clear overrides on empty columns whenever the toggle flips on.
//
// Only when it is the *reader* flipping it, though. The same ref is written in bulk
// by loadCustomize — for a saved view, a layout swap and the localStorage restore —
// and there the overrides arriving alongside it are part of the state being restored,
// not stale leftovers to prune. A view saved with the toggle on AND a couple of empty
// columns expanded by hand loaded correctly only when the board already had the toggle
// on (no flip, no watcher); coming from a board with it off, the flip fired and wiped
// exactly those overrides — and the persist watcher wrote the loss straight back to
// localStorage. Same view, two different results depending on what was on screen
// before it. `viewLoadInFlight` is the flag persistView and the autosave already stand
// down on; it is still set while this watcher runs (it is cleared on `nextTick`, after
// the pre-flush queue this job sits in).
watch(autoCollapseEmpty, (on) => {
  if (!on || viewLoadInFlight()) return
  for (const dcol of displayColumns.value) {
    const k = colCollapseKey(dcol)
    if (colCount(dcol) === 0 && k in colCollapse) delete colCollapse[k]
  }
})
// ── sub-toolbar fit (v2, #2954, #2970) ──
// One row, never scrolled. The view switch and the composer's chips own the
// room (feedback: they are what the reader works with); everything else gives
// way first. Fitting starts from the full bar and gives ground in a fixed order
// until the row fits: the chart's text key hints → the «Группировка» caption →
// the chart's scale segments (into a menu) → the view switch's captions (only
// the active one keeps its own) → filter chips from the end into «+N» → the
// sort chips (taking back what filter chips fit in the room they free) → the
// board's people. Every step is a DOM update + measure inside one frame (the
// observer and nextTick both run before paint), so nothing flickers.
const subbarEl = ref(null)
const composerEl = ref(null)
const chipsClip = ref(null)
const switchAvail = ref(Infinity) // px the view switch may take
const foldFilters = ref(0) // how many filter chips (from the end) sit in «+N»
const foldSorts = ref(false) // sort chips folded into «+N» too
const compactGroup = ref(false) // grouping control without its caption
const hideChartHints = ref(false) // Gantt/timeline: its text key hints go first
const compactChart = ref(false) // Gantt/timeline: the scale as a menu
const hidePeople = ref(false) // the board's avatars, the very last resort
// The open Gantt / timeline view: its exposed controller drives ChartControls.
const chartView = ref(null)
// Its controls join the sub-toolbar once it mounts — fit the row again then.
watch(chartView, () => nextTick(fitSubbar))
const hiddenChips = computed(() => {
  const n = filterChips.value.length
  const k = Math.min(foldFilters.value, n)
  return Array.from({ length: k }, (_, i) => n - k + i)
})
const foldedCount = computed(
  () => hiddenChips.value.length + (foldSorts.value ? sortLevels.value.length : 0),
)
let subbarRO = null
let fitting = false
let refit = false // a fit was asked for while one was running
const overflowing = (bar) => bar.scrollWidth > bar.clientWidth + 1
async function fitSubbar() {
  const bar = subbarEl.value
  if (!bar) return
  if (fitting) {
    refit = true
    return
  }
  fitting = true
  refit = false
  try {
    foldFilters.value = 0
    foldSorts.value = false
    compactGroup.value = false
    hideChartHints.value = false
    compactChart.value = false
    hidePeople.value = false
    switchAvail.value = Infinity
    await nextTick()
    // The chart's key hints are only text: they give their room up first.
    if (overflowing(bar) && timelineLike.value) {
      hideChartHints.value = true
      await nextTick()
    }
    // The switch and the chips outrank the grouping caption: drop it first.
    if (overflowing(bar)) {
      compactGroup.value = true
      await nextTick()
    }
    if (overflowing(bar) && timelineLike.value) {
      compactChart.value = true
      await nextTick()
    }
    // Whatever the rest of the row needs, the switch may not take: past that it
    // drops its captions.
    const sw = [...bar.children].find((el) => el.classList.contains('layout-switch'))
    const rest = bar.scrollWidth - (sw ? sw.offsetWidth : 0)
    switchAvail.value = Math.max(0, bar.clientWidth - rest)
    await nextTick()
    for (let i = 0; i < filterChips.value.length && overflowing(bar); i++) {
      foldFilters.value = i + 1
      await nextTick()
    }
    if (overflowing(bar) && sortLevels.value.length) {
      foldSorts.value = true
      await nextTick()
      // The sorts' room may take some filter chips back out of «+N».
      while (foldFilters.value > 0) {
        foldFilters.value--
        await nextTick()
        if (overflowing(bar)) {
          foldFilters.value++
          await nextTick()
          break
        }
      }
    }
    if (overflowing(bar)) {
      hidePeople.value = true
      await nextTick()
    }
  } finally {
    fitting = false
  }
  // A focused control can scroll the clipped row sideways; the fit owns it.
  bar.scrollLeft = 0
  if (refit) fitSubbar()
}
// Attach the bar's resize observer whenever the element itself arrives — the same
// late binding `boardScroll` has, and for the same reason: the whole board (the
// composer included) sits behind `v-if="board"`, and `board` is still null while the
// first load is on the wire. onMounted therefore ran its `observe` against an empty
// ref and gave up silently, with no second chance — the component is not remounted
// when you switch boards, so the observer stayed unattached for the whole session and
// «re-measure on bar resize» never happened: collapsing the sidebar or narrowing the
// window left the right-side tools in place while the (nowrap) chips ran past the
// edge, until some unrelated facet change happened to re-measure. The explicit
// re-measure here is the first one the mount-time nextTick could not take (the bar
// did not exist yet), and it does not lean on the observer's initial delivery.
watch(subbarEl, (el) => {
  if (!subbarRO || !el) return
  subbarRO.disconnect()
  subbarRO.observe(el)
  nextTick(fitSubbar)
})
const groupMode = ref('status') // 'status' | 'tag'
const tagPrefix = ref('') // when grouping by tag: only tags with this namespace prefix become columns
// Friendly display names for tag prefixes (canonical prefix → label, owned by the
// store) are loaded per-project. Falls back to the raw prefix where no name is
// configured.

// Tags that become columns in tag-grouping mode (filtered by the chosen prefix).
// Membership is asked through utils/tagGroups (`tagInPrefix`), i.e. by canonical
// namespace — the same key the filter menu's groups, the prefix labels and the
// backend's prefix registry use. A raw `startsWith` made «Сложность::» and
// «сложность::» two different namespaces to the lanes alone (#2932).
const groupTags = computed(() =>
  tagPrefix.value
    ? tagsList.value.filter((tag) => tagInPrefix(tag.name, tagPrefix.value))
    : tagsList.value,
)

// Archive scope: ?archived=1 shows the board's archived tasks read-only (no DnD, no
// create, no inline edits) with a Restore action. Reuses all board filters/grouping.
const archivedMode = computed(() => isArchiveQuery(route.query))
// Is the «+ колонка» tile part of this row? Status grouping only (tag/milestone lanes
// are not columns to create), and never in the archive — the tile would create a live
// column on the board from a read-only view. Asked in two places that must agree: the
// tile's own v-if and colWidth's reservation of ADD_COL_W. They were written out
// separately, and when the archive gate was added to the template the arithmetic kept
// reserving the tile's 232px there — every column in an archived board came out that
// much narrower between them, with a dead band along the right edge.
const showAddColumn = computed(() => groupMode.value === 'status' && !archivedMode.value)
// Can the columns be dragged into a new order right now? Status grouping only (tag
// and milestone lanes have no stored order to write back), never in the archive.
// Asked in two places that must agree: the column draggable's `:disabled`, and the
// `.col-drag` handle class on the collapsed strip — a collapsed column renders no
// header, so the strip is the only element left that can carry the handle.
const canReorderColumns = computed(() => groupMode.value === 'status' && !archivedMode.value)
function exitArchive() {
  router.replace({ query: withoutArchiveQuery(route.query) })
}

// Sprint scope (navigation overlay): ?milestone=<slug|uuid|backlog>. Drives the
// server-side task scope and shows a removable chip; clearing it returns the full
// board (and de-highlights the sidebar sprint node). The value is passed to the
// API verbatim — the server resolves slug or UUID.
const milestoneScope = computed(() => (route.query.milestone ? String(route.query.milestone) : ''))
const milestoneScopeLabel = computed(() => {
  const s = milestoneScope.value
  if (!s) return ''
  if (s === BACKLOG_SCOPE) return t('board.scope.backlog')
  // Keyed by id, so a slug scope needs a scan (a project has few milestones).
  return (
    (milestonesMap[s] || milestonesList.value.find((m) => matchesScope(m, s)))?.title ||
    t('board.scope.milestoneFallback')
  )
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
  nextGroupModeLabel,
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
// than fanning out side submenus that run off a narrow screen. The behaviour lives
// in a composable because the customize drawer renders the same menu behind its own
// «+» — and on a phone that drawer is the settings surface one can always reach.
// Each call site keeps its own open/level state (one shared `show` would pop both
// dropdowns at once), so this instance belongs to the composer's button alone.
const {
  show: addShow,
  menuOptions: addMenuOptions,
  renderLabel: renderAddLabel,
  onSelect: onAddSelect,
  onShow: onAddShow,
} = useFacetAddMenu({ options: addOptions, isMobile, onPick: onAddFacet })
function toggleSubtasksExpanded() {
  subtasksExpanded.value = !subtasksExpanded.value
}

// Is the composer holding a manual sort right now? Then the order on screen is NOT
// the stored `position` order — `lists` comes from `filteredTasks`, which is
// `sortByLevels(...)`, and `sortedSubtasksByParent` re-orders each card's children
// the same way — so every neighbour read off a rendered list means something other
// than it says. Asked in three places that must agree: the card list's `:sort`, the
// planner that turns a drop into an API call, and the child list on each card.
const composerSorted = computed(() => sortLevels.value.length > 0)
// May a card be re-ordered inside its own lane? Only status columns own card
// order (`position` is per column, and a tag/milestone lane spans all of them),
// and only while nothing re-orders them on the way to the screen — the rule and
// both reasons live in utils/boardDnd, next to the planner that has no intent for
// such a drop. Cross-lane drags stay on in every grouping.
const canSortCards = computed(() => canSortWithinLane(groupMode.value, composerSorted.value))

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
  ],
  persistView,
  { deep: true },
)
// Re-fit the sub-toolbar when the chip set changes (add/remove a filter, sort,
// group, scope) or the view does; resizes are handled by the observer.
watch(
  [
    facetChips,
    archivedMode,
    milestoneScope,
    layout,
    isMobile,
    () => boardViewStore.membersList.length,
  ],
  () => nextTick(fitSubbar),
)

// ── saved views (#2970): own + shared, server-side; the loaded one and its
// «изменено» live in useBoardViews. The board hands over what a view captures
// (currentViewConfig) and how to load one (applyViewConfig, as a bulk load). ──
// Snapshot / restore the full toolbar state that a view captures. The header's
// title search is the reader's moment, not part of a view.
function currentViewConfig() {
  return {
    layout: layout.value,
    groupMode: groupMode.value,
    tagPrefix: tagPrefix.value,
    sortLevels: sortLevels.value.map((l) => ({ ...l })),
    subtasksExpanded: subtasksExpanded.value,
    autoSort: autoSort.value,
    filters: { ...cloneFilters(filters), q: '' },
    colCollapse: { ...colCollapse },
    autoCollapseEmpty: autoCollapseEmpty.value,
    cardSize: cardSize.value,
    stackFields: stackFields.value,
    showEmpty: showEmpty.value,
    fieldVis: { ...fieldVis },
  }
}
function applyViewConfig(c) {
  if (!c) return
  if (c.layout) layout.value = c.layout
  groupMode.value = c.groupMode || defaultToolbar(layout.value).groupMode
  tagPrefix.value = c.tagPrefix || ''
  if (Array.isArray(c.sortLevels)) sortLevels.value = c.sortLevels.map((l) => ({ ...l }))
  else if (c.sortBy && c.sortBy !== 'position')
    sortLevels.value = [{ field: c.sortBy, dir: c.sortDir || 'asc' }]
  else sortLevels.value = []
  subtasksExpanded.value = !!c.subtasksExpanded
  // Gantt's «Авто» is part of the view, not of the session: without this a view
  // saved in auto order loaded as plain scheduled order, and an auto run left on
  // leaked into the next view (autoActive asks only for gantt + no group + no
  // sort, which is what most views look like). Views predating the key read as
  // off — the state they were actually saved in, since it could not be captured.
  autoSort.value = !!c.autoSort
  // The header search stays what the reader typed.
  const q = filters.q
  Object.assign(filters, cloneFilters(c.filters))
  filters.q = q
  loadCustomize(c)
}
const bv = reactive(
  useBoardViews({
    boardId: toRef(props, 'boardId'),
    currentConfig: currentViewConfig,
    applyConfig: (c) => withViewLoad(() => applyViewConfig(c)),
  }),
)
// The chip comes and goes with the loaded view, and its dot with «изменено».
watch([() => bv.loaded?.name, () => bv.changed], () => nextTick(fitSubbar))
function openPanel(section) {
  customizeOpen.value = true
  if (section) nextTick(() => customizePanel.value?.reveal(section))
}
// ⌘⇧S, the chip's «Сохранить текущее как…», a locked view's «Сохранить как
// моё»: the panel opens on its views with the field ready.
watch(
  () => bv.saveAsDraft,
  (d) => d && openPanel('views'),
)
// The closed «Представления» section's summary.
const viewsSummary = computed(() =>
  bv.loaded
    ? bv.changed
      ? `${bv.loaded.name} · ${t('board.views.changed')}`
      : bv.loaded.name
    : bv.views.length
      ? String(bv.views.length)
      : '',
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
const GAP = BOARD_GAP
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
// The layout maths lives in utils/boardLayout.js; the board only supplies what it
// draws. `addTile` is the same condition as the tile's own v-if — the two used to
// be spelled out separately and drifted apart in the archive (see showAddColumn).
const colWidth = computed(() =>
  columnWidth({
    containerWidth: containerWidth.value,
    isMobile: isMobile.value,
    count: displayColumns.value.length,
    collapsed: displayColumns.value.filter((c) => isColCollapsed(c)).length,
    addTile: showAddColumn.value,
    min: cardMin.value,
    cap: cardCap.value,
  }),
)
const colStyleVars = computed(() => ({ '--col-w': colWidth.value + 'px' }))
// The grouping control lists the same choices as the add menu's «Группировка».
const groupOptions = computed(() => addOptions.value.find((o) => o.key === 'group')?.children || [])
// A column with no colour of its own (status column left on default, «Без тегов»)
// is tinted with the neutral grey of the v2 lanes (#2954).
const NEUTRAL_COLUMN = '#9aa0aa'

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
function rowById(id) {
  const t = allTasks.value.find((x) => x.id === id)
  if (t) return t
  for (const arr of Object.values(subtasksByParent.value)) {
    const s = arr.find((x) => x.id === id)
    if (s) return s
  }
  return null
}
function numberOf(id) {
  return rowById(id)?.number ?? null
}
// Is this task one the board currently draws (a card or a subtask under one)?
// Only the realtime router asks: an event naming another board is ours after all
// when it names a card we still hold — that is the card leaving (see classifyEvent).
function taskOnBoard(id) {
  return !!rowById(id)
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
  // A full reload re-reads everything a partial fetch could, so the backlog of
  // events dropped during the drag is settled by it — dropping a card must not
  // cost an extra task fetch on top of the reload the drop already schedules.
  missedKinds.clear()
  reloadTimer = setTimeout(() => load(props.boardId, { silent: true }), RELOAD_DEBOUNCE_MS)
}

// Поколение загрузки. Доска НЕ перемонтируется при смене доски (см. комментарий у
// loadViews), а `?milestone=`/`?archived=1` меняют охват прямо на месте — значит
// ответ, снятый под прошлый запрос, приходит уже на чужую доску или на чужой охват,
// и без сверки просто ПРИСВАИВАЕТСЯ: карточки, колонки, и главное — setContext,
// то есть projectId, по которому дети потом заводят теги и читают конфиг оценок.
// Номер поколения поднимает только полная загрузка; частичные догрузки его лишь
// читают, иначе они обесценивали бы полную, с которой идут бок о бок (#2932).
let loadSeq = 0

// Partial reloads, one debounce timer per kind: a burst of task events must not
// drag the columns and the workspace meta along with it. Tasks get a slightly
// wider window because one user action fans out into several task events
// (task.updated + task.tagged + task.assigned) that should collapse into one fetch.
const PARTIAL_DEBOUNCE_MS = { tasks: 400, columns: 200, board: 200, meta: 200 }
const partialTimers = { tasks: null, columns: null, board: null, meta: null }
const partialFetch = {
  tasks: () => fetchTasks(props.boardId),
  // Эти двое живут на таймере: пока он тикал и пока летел ответ, читатель мог уйти
  // на другую доску — событие покинутой не должно подменить колонки открытой.
  columns: async () => {
    const mine = loadSeq
    const { data } = await boards.columns(props.boardId)
    if (mine !== loadSeq) return
    columns.value = data || []
  },
  board: async () => {
    const mine = loadSeq
    const { data } = await boards.get(props.boardId)
    if (mine !== loadSeq) return
    board.value = data
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

// Realtime events the drag guard threw away, as the partial re-fetches that catch
// up on them (see utils/boardEvents `missedRefetchKinds`). The guard has to drop
// them — the board holds still while a card is in the air — but a drag that ends
// without moving anything schedules no reload of its own, and until this backlog
// existed those events were simply lost.
//
// The suppression window needs no such bookkeeping: it is armed by our OWN
// mutations, and every one of them ends in a load() or a scheduleReload() that
// brings the same data back.
const missedKinds = new Set()
function rememberMissed(ev) {
  const kind = classifyEvent(ev, { boardId: props.boardId, hasTask: taskOnBoard })
  for (const k of missedRefetchKinds(kind)) missedKinds.add(k)
}
function flushMissed() {
  if (!missedKinds.size) return
  const kinds = [...missedKinds]
  missedKinds.clear()
  for (const k of kinds) schedulePartial(k)
}
// vuedraggable emits `change` before `end`, so a drag that really moved something
// has already called scheduleReload() by now — and that clears the backlog, so the
// two never fetch the same thing twice.
function onBoardDragEnd() {
  onDragEnd()
  flushMissed()
}

// A drag inside another layout (the Eisenhower matrix has a SortableJS grid of its
// own) holds the board just as still: `dragging` above is raised only by the
// kanban's own draggables, so without this the matrix's gesture was the one case
// where a realtime event re-fetched the tasks and rebuilt the quadrants from under
// Sortable (#2932). The layout reports its gesture; the guard and the backlog are
// the same ones the kanban uses. No autoscroll setup here on purpose — that rAF
// loop steers the column strip, which this layout does not render.
const layoutDragging = ref(false)
function onLayoutDragState(active) {
  layoutDragging.value = !!active
  if (!active) flushMissed()
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

// The same scope, in the form `applyTaskPatch` checks a patched card against (see
// utils/boardEvents `inBoardScope`). Derived from `taskQuery()` on purpose: a card
// may stay drawn only while the server would still list it, so the two must not
// drift — including the archive branch, where the sprint scope is not sent at all.
// The server resolves the slug or UUID in `?milestone=`; here it is matched against
// the project's loaded milestones, and a scope we cannot resolve yet becomes `''`,
// which matches no task and so re-fetches instead of merging.
function patchScope() {
  const { archived } = taskQuery()
  const s = milestoneScope.value
  if (archived || !s) return {}
  if (s === BACKLOG_SCOPE) return { milestone: null }
  const m = milestonesMap[s] || milestonesList.value.find((mm) => matchesScope(mm, s))
  return { milestone: m ? m.id : '' }
}

// The same resolved scope, in the form a newly created card is stamped with (see
// utils/milestones `newTaskMilestoneId`). Derived from `patchScope()` so the id a
// card is born with is the very one the list is filtered by: `undefined` on an
// unscoped board and in the archive, `null` on the backlog scope, `''` for a scope
// that cannot be resolved yet.
function scopeMilestoneId() {
  return patchScope().milestone
}

// Fetch just the task lists (2 requests instead of the full load's 10).
async function fetchTasks(id) {
  const mine = loadSeq
  const { archived, params } = taskQuery()
  const [t, s] = await Promise.all([
    boards.tasks(id, params),
    archived ? Promise.resolve({ data: [] }) : boards.subtasks(id),
  ])
  if (mine !== loadSeq) return
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
  const mine = ++loadSeq
  if (!silent) loading.value = true
  try {
    const { archived, params } = taskQuery()
    const [b, c, t, s] = await Promise.all([
      boards.get(id),
      boards.columns(id),
      boards.tasks(id, params),
      archived ? Promise.resolve({ data: [] }) : boards.subtasks(id),
    ])
    // Пока летели десять запросов, читатель мог уйти на другую доску, в архив или
    // на другой этап. Тогда этот ответ описывает не то, что сейчас на экране.
    if (mine !== loadSeq) return
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
    // Отказ покинутой доски (её проект удалили, этап исчез) не должен кричать
    // поверх той, что читатель открыл вместо неё.
    if (mine !== loadSeq) return
    message.error(e.message)
  } finally {
    // Спиннер снимает только тот запрос, который его и зажёг: иначе ответ
    // покинутой доски открывал бы ещё пустую новую.
    if (!silent && mine === loadSeq) loading.value = false
  }
}

async function loadWorkspaceMeta() {
  const mine = loadSeq
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
  // Теги, участники и этапы покинутой доски — чужой словарь для открытой.
  if (mine !== loadSeq) return
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
    formatters: formatters.value,
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
// drag-reorder stays authoritative. Which is why `composerSorted` also travels
// down to the card — while the sort re-orders this list, the row a child is
// dropped next to is not its neighbour in the parent's ladder, so the gesture is
// refused there rather than persisted against the wrong siblings.
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
    return columns.value.map((c) => ({
      key: c.id,
      name: columnName(c),
      rawName: c.name,
      color: c.color,
      status: c,
    }))
  }
  if (groupMode.value === 'milestone') {
    return [
      ...milestonesList.value.map((m) => ({
        key: m.id,
        name: m.title,
        rawName: m.title,
        color: '',
        milestone: m,
      })),
      {
        key: '__none__',
        name: t('board.group.noMilestone'),
        rawName: t('board.group.noMilestone'),
        color: '',
        milestone: null,
      },
    ]
  }
  return [
    ...groupTags.value.map((tag) => ({
      key: tag.id,
      name: tagColumnName(tag),
      rawName: tagColumnName(tag),
      color: tag.color,
      tag,
    })),
    {
      key: '__none__',
      name: t('board.group.noTags'),
      rawName: t('board.group.noTags'),
      color: '',
      tag: null,
    },
  ]
})

// Column title for a tag group: the raw "scope::value" is never shown. The rule
// lives in utils/tagGroups, next to the split it is built on, because the
// timeline/gantt swimlanes stand for the same tags and have to read the same.
function tagColumnName(tag) {
  return tagLaneLabel(tag, { tagPrefix: tagPrefix.value, prefixNames: tagPrefixNames })
}

// Estimation rollup per milestone column: Σ of the column's tasks' own estimates,
// formatted in the project's unit. Shown only when grouped by milestone.
const estCfg = computed(() => wsStore.estimationFor(board.value?.project_id))
// Header counts per column: cards, and cards + all their subtasks (#2850). Built
// once per list/filter change rather than per render — each entry walks the whole
// subtask tree.
//
// Counted off the FILTERED map, which now carries every depth (#2932). It used to
// be keyed by top-level tasks only, so the raw map had to be merged under it to
// supply the levels below — and that merge is what made the number disagree with
// the board: under a filter a narrowed parent reported its matching children, but
// their children came back unfiltered, so the total counted work the composer had
// just hidden. The filtered map is self-contained now: a branch it keeps, it keeps
// whole.
const countSubtaskMap = computed(() => filteredSubtasksByParent.value)
const countsByCol = computed(() => {
  const out = {}
  for (const [key, arr] of Object.entries(lists.value)) {
    out[key] = countWithSubtasks(arr, countSubtaskMap.value)
  }
  return out
})
const EMPTY_COUNTS = { tasks: 0, total: 0 }
function colCounts(dcol) {
  return countsByCol.value[dcol.key] || EMPTY_COUNTS
}
function colCountTitle(dcol) {
  const c = colCounts(dcol)
  return c.total > c.tasks
    ? t('board.column.countTitle', { tasks: c.tasks, total: c.total })
    : t('board.column.countTitleFlat', { tasks: c.tasks })
}

// Composer-bar "showing" counter (#2851): how much the current facet set leaves
// on the board, across every layout. Counted from filteredTasks rather than by
// summing the column headers — with tag grouping a task carrying two column tags
// sits in both lists, and the arithmetic sum would count it twice.
const shownCounts = computed(() => countWithSubtasks(filteredTasks.value, countSubtaskMap.value))
// The header search is this board's title filter (v2, #2954): mirror it both
// ways (a saved view can bring its own query), and publish how many tasks the
// filter leaves so the header can fall back to a workspace search on zero.
watch(
  () => boardViewStore.searchQ,
  (v) => {
    if ((filters.q || '') !== v) filters.q = v
  },
)
watch(
  () => filters.q,
  (v) => {
    if (boardViewStore.searchQ !== (v || '')) boardViewStore.searchQ = v || ''
  },
)
watch(
  () => shownCounts.value.tasks,
  (n) => (boardViewStore.shownTasks = n),
  { immediate: true },
)
watch(
  () => boardViewStore.assigneeFilter,
  (req) => {
    if (req?.userId) filters.assignees = [req.userId]
  },
)
// The count itself is drawn next to the board in the sidebar (v2, as in the
// mockup); the board publishes it with its tooltip.
const shownCountTitle = computed(() => {
  const c = shownCounts.value
  return c.total > c.tasks
    ? t('board.composer.shownTitle', { tasks: c.tasks, total: c.total })
    : t('board.composer.shownTitleFlat', { tasks: c.tasks })
})
watch(
  () => shownCountTitle.value,
  (v) => (boardViewStore.shownTitle = v),
  { immediate: true },
)

// «Σ» in a milestone lane's header. Counted through sumLaneEffort, not by summing
// the rows' own `estimate`: a card with no estimate of its own prints the rollup of
// its subtasks, and the header has to agree with the cards standing under it. The
// FILTERED child map is the one the cards are drawn from (sortedSubtasksByParent is
// the same membership, re-ordered), so the total narrows with the composer exactly
// as the cards do.
function columnEstimate(dcol) {
  if (groupMode.value !== 'milestone') return ''
  const v = sumLaneEffort(lists.value[dcol.key] || [], filteredSubtasksByParent.value)
  return v ? formatEstimate(v, estCfg.value) : ''
}

function rebuildLists() {
  // 'assignee'/'none' are timeline-only swimlane modes; the board/list views only
  // understand status/tag/milestone columns, so skip rebuilding for those.
  if (!['status', 'tag', 'milestone'].includes(groupMode.value)) return
  lists.value = buildLists({
    groupMode: groupMode.value,
    columns: columns.value,
    milestones: milestonesList.value,
    groupTags: groupTags.value,
    tasks: filteredTasks.value,
  })
}
// Watch what the LANES are built from, not a hand-kept subset of it. The sources
// used to be spelled out one by one ([filteredTasks, groupMode, tagPrefix,
// milestonesList]) and two were missing — `columns` and the tag list behind
// `groupTags` — so a lane that appeared without the cards changing came up with no
// bucket at all, i.e. undroppable (see utils/boardLists). Both gaps were reachable
// from realtime alone: a `column.*` event re-reads only the columns, a `tag.*` event
// only the workspace meta. `displayColumns` is exactly that set, already assembled —
// it is the lanes the board draws, and it is what has to have a bucket each.
watch([filteredTasks, displayColumns], rebuildLists)

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

// Sends drop/shift intents (utils/boardDnd, utils/cardNav) to the API, in order.
async function runIntents(intents) {
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
      // The tags that ARE lanes right now (prefix narrowing included) — the drop
      // into «Без тегов» takes exactly those off the card and leaves the rest.
      groupTags: groupTags.value,
      // A manual sort re-orders the rendered list, so `newIndex`'s neighbours are
      // not the card's neighbours on the `position` ladder — the slot is left to
      // the server (and a pure re-order plans nothing at all).
      sorted: composerSorted.value,
    })
    await runIntents(intents)
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
    message.warning(t('board.column.needStatus'))
    return
  }
  // A card typed into a lane is stamped with that lane's milestone; one typed on a
  // sprint-scoped board is stamped with the scope, or the reload that follows would
  // not list it back (see newTaskMilestoneId).
  const milestoneId = newTaskMilestoneId({
    groupMode: groupMode.value,
    lane: dcol,
    scopeId: scopeMilestoneId(),
  })
  suppress()
  // Cleared BEFORE the await on purpose: it keeps the input open for rapid entry and
  // makes a second Enter read an empty title (the re-entrancy submitAddColumn needs an
  // explicit flag for). The cost is that the typed title is already gone by the time a
  // failure comes back — so the failure path puts it back, the way the column input
  // keeps its name.
  newTaskTitle.value = ''
  try {
    const res = await boards.createTask(board.value.id, { column_id: columnId, title })
    if (groupMode.value === 'tag' && dcol.tag) await tasksApi.addTag(res.data.id, dcol.tag.id)
    if (milestoneId) await tasksApi.setMilestone(res.data.id, milestoneId)
    await load(props.boardId)
    reopenForRapidEntry(dcol)
  } catch (e) {
    message.error(e.message)
    restoreFailedTitle(dcol, title)
  }
}
// Rapid entry: once a card lands, the input stays open on the column it was typed into,
// focused, so the next title can follow straight away — the reload in between re-renders
// the column and takes the focus with it.
//
// Only where the reader actually still is, though. `load()` does not touch
// `addingInColumn`, so on the ordinary path it is this column already and nothing but
// the focus needs restoring; re-assigning it was a no-op there and a bug everywhere
// else. The create is awaited, and while the POST was on the wire the reader may have
// left: the input commits on blur, so a click away or an Esc closes the gesture
// (`addingInColumn === null`), and clicking «＋ задача» on another column opens it
// there. Writing `dcol.key` back then yanked the open input out of that column and
// stole the focus — and since `newTaskTitle` is one shared ref, the title they had
// already begun typing came along, so the next Enter filed it in the lane they had just
// left. That is the same rule restoreFailedTitle follows on the failure path, from the
// other side.
function reopenForRapidEntry(dcol) {
  if (addingInColumn.value !== dcol.key) return
  focusTaskInput()
}
// A create that failed must not eat the title with it: the toast says «не удалось», the
// task isn't on the board, and retyping it is the reader's work the error has no claim
// on. The input also commits on blur, so after a failure a stray click outside it used
// to close the whole add gesture with nothing left behind.
//
// Only restored into an input the reader hasn't moved on from: while the POST was on
// the wire they may have typed the next task (their text wins) or opened the «＋ задача»
// of another column (dropping this title into that column's input would file it in the
// wrong lane). A closed input is reopened on the column the title was meant for.
function restoreFailedTitle(dcol, title) {
  if (newTaskTitle.value) return
  if (addingInColumn.value !== null && addingInColumn.value !== dcol.key) return
  newTaskTitle.value = title
  addingInColumn.value = dcol.key
  focusTaskInput()
}

// ── keyboard (v2, #2956) ──
// A selected card (arrows), actions on it (↵ E S A P D L ⇧N ⌘←/→) and the
// board's own keys (C F ⇧G V H ⇧C). `focusedCard` is not `selectedTaskId` —
// that one is the task open in the modal — and it carries the column too:
// with tag grouping one task sits in several columns.
const focusedCard = ref(null)
// The column under the mouse, for «C — task in the hovered column» and «H».
const hoveredColKey = ref(null)
const groupCtlEl = ref(null)

const kanbanOn = () => !!board.value && layout.value === 'board' && !isMobile.value
const editable = () => !!board.value && !archivedMode.value

function navCols() {
  return colModel.value.map((d) => ({
    key: d.key,
    ids: (lists.value[d.key] || []).map((x) => x.id),
    collapsed: isColCollapsed(d),
  }))
}
// The selection is kept for the arrows to carry on from (a click on a card
// sets it), but it is only drawn once the keyboard is in use: a pointer — and a
// phone, where there is no keyboard at all — never shows the ring.
const kbdFocus = ref(false)
function onAnyPointer() {
  kbdFocus.value = false
}
function isFocused(dcol, id) {
  const f = focusedCard.value
  return kbdFocus.value && !isMobile.value && !!f && f.colKey === dcol.key && f.taskId === id
}
function focusedTask() {
  const f = focusedCard.value
  return f ? (lists.value[f.colKey] || []).find((x) => x.id === f.taskId) || null : null
}
function focusedCol() {
  const key = focusedCard.value?.colKey || hoveredColKey.value
  return colModel.value.find((d) => d.key === key) || null
}
function revealFocused() {
  nextTick(() => {
    const f = focusedCard.value
    if (!f) return
    boardScroll.value
      ?.querySelector(`.card-wrap[data-card="${CSS.escape(cardKey(f.colKey, f.taskId))}"]`)
      ?.scrollIntoView({ block: 'nearest', inline: 'nearest' })
  })
}
function stepFocus(dir) {
  // The first arrow after the mouse shows where the selection is, without
  // moving it off the card that was clicked.
  if (!kbdFocus.value && focusedTask()) {
    kbdFocus.value = true
    revealFocused()
    return
  }
  kbdFocus.value = true
  focusedCard.value = moveFocus(navCols(), focusedCard.value, dir)
  revealFocused()
}
// A card the filter, a move or a delete took away drops the selection.
watch(lists, () => {
  if (focusedCard.value && !focusedTask()) focusedCard.value = null
})
// Clicking a card also selects it, so the arrows carry on from there after
// the modal closes.
function onCardOpen(dcol, id) {
  focusedCard.value = { colKey: dcol.key, taskId: id }
  openTask(id)
}

// ── J/K in the open task (v2, #2958): the neighbours in the order on screen —
// the kanban read column by column (collapsed ones skipped, a task in several
// tag columns counted once), any other view in its own list order. ──
const navOrder = computed(() => {
  if (!showTaskModal.value) return []
  if (layout.value !== 'board') return filteredTasks.value.map((x) => x.id)
  const seen = new Set()
  const out = []
  for (const c of navCols()) {
    if (c.collapsed) continue
    for (const id of c.ids) if (!seen.has(id)) seen.add(id) && out.push(id)
  }
  return out
})
const taskNav = computed(() => {
  const i = navOrder.value.indexOf(selectedTaskId.value)
  return i < 0 ? null : { index: i, total: navOrder.value.length }
})
function navigateTask(dir) {
  const order = navOrder.value
  const id = order[order.indexOf(selectedTaskId.value) + dir]
  if (!id) return
  selectedTaskId.value = id
  // The selection follows, so closing the modal leaves the arrows on this card.
  if (layout.value === 'board') {
    const col = navCols().find((c) => c.ids.includes(id))
    if (col) focusedCard.value = { colKey: col.key, taskId: id }
  }
  // Replace, not push: walking through twenty tasks shouldn't take twenty Backs.
  router.replace({ query: { ...route.query, task: numberOf(id) ?? id } })
}

async function completeFocused() {
  const task = focusedTask()
  if (!task) return
  const plan = planComplete({
    completed: !!task.completed_at,
    columnId: task.column_id,
    doneColumnId: doneTarget(columns.value, doneColumnId.value)?.id,
  })
  suppress()
  try {
    if (plan.op === 'move') {
      // No neighbour: the server appends to the end of the WHOLE column. The
      // loaded cards may be a sprint-scoped slice, whose last card sits
      // mid-column (#2932, see moveNeighbors in utils/status).
      await tasksApi.move(task.id, { column_id: plan.columnId, before_id: null, after_id: null })
    } else {
      await tasksApi.update(task.id, { ...taskBasePatch(task), completed: plan.op === 'complete' })
    }
    scheduleReload()
  } catch (e) {
    message.error(e.message)
  }
}

async function setFocusedPriority(value) {
  const task = focusedTask()
  if (!task || value > 4) return
  suppress()
  try {
    await tasksApi.update(task.id, { ...taskBasePatch(task), priority: value })
    scheduleReload()
  } catch (e) {
    message.error(e.message)
  }
}

async function shiftFocused(dir) {
  const task = focusedTask()
  const from = focusedCol()
  const to = from && neighbourColumn(colModel.value, from.key, dir)
  if (!task || !to) return
  const intents = planShift({
    groupMode: groupMode.value,
    from,
    to,
    taskId: task.id,
    // null = let the server append to the whole column (see completeFocused).
    tailId: null,
  })
  if (!intents.length) return
  suppress()
  try {
    await runIntents(intents)
    kbdFocus.value = true
    focusedCard.value = { colKey: to.key, taskId: task.id }
    await load(props.boardId, { silent: true })
    revealFocused()
  } catch (e) {
    message.error(e.message)
    load(props.boardId)
  }
}

// V, then 1–6: the six views, in the order of the view switch (#2970).
const LAYOUT_KEYS = LAYOUTS
const paletteStore = usePaletteStore()
const openField = (field) => () => {
  const task = focusedTask()
  if (task) paletteStore.openTask(task.id, field)
}

useShortcuts(
  {
    'card.up': () => stepFocus('up'),
    'card.down': () => stepFocus('down'),
    'card.left': () => stepFocus('left'),
    'card.right': () => stepFocus('right'),
  },
  { when: kanbanOn },
)
useShortcuts(
  {
    'card.clear': () => (focusedCard.value = null),
    'card.open': () => openTask(focusedCard.value.taskId),
  },
  { when: () => kanbanOn() && kbdFocus.value && !!focusedTask() },
)
useShortcuts(
  {
    'card.complete': completeFocused,
    'card.status': openField('status'),
    'card.assignees': openField('assignees'),
    'card.priorityPick': openField('priority'),
    'card.priority': setFocusedPriority,
    'card.due': openField('due'),
    'card.tags': openField('tags'),
    'card.subtask': openField('subtask'),
    'card.moveLeft': () => shiftFocused('left'),
    'card.moveRight': () => shiftFocused('right'),
  },
  { when: () => kanbanOn() && editable() && kbdFocus.value && !!focusedTask() },
)
useShortcuts(
  {
    'board.addTask': () => {
      const dcol = focusedCol() || colModel.value.find((d) => !isColCollapsed(d))
      if (dcol) startAddTask(dcol)
    },
    'view.collapse': () => {
      const dcol = focusedCol()
      if (dcol) toggleCollapse(dcol)
    },
  },
  { when: () => kanbanOn() && editable() },
)
useShortcuts(
  {
    'board.filter': () => onAddShow(true),
    'board.group': () => groupCtlEl.value?.click(),
    'board.layout': (n) => {
      const next = LAYOUT_KEYS[n - 1]
      if (next) layout.value = next
    },
    'view.cardSize': () => {
      const sizes = ['compact', 'medium', 'large']
      cardSize.value = sizes[(sizes.indexOf(cardSize.value) + 1) % sizes.length]
    },
    'view.panel': () => (customizeOpen.value ? (customizeOpen.value = false) : openPanel()),
    // Saved views (#2987): ⌘S updates the loaded one (or saves anew when none
    // is loaded / it is not the reader's to change), ⌘⇧S saves as new, ⌘⌫ resets.
    'views.update': () => bv.update(),
    'views.saveAs': () => bv.requestSaveAs(''),
    'views.reset': () => bv.reset(),
  },
  { when: () => !!board.value && !isMobile.value },
)
// 1–9 / 0 pick a view while the panel is open (the chip's menu answers to the
// digits on its own).
useShortcuts(
  { 'views.pick': (n) => bv.pick(n) },
  { when: () => !!board.value && !isMobile.value && customizeOpen.value },
)

// «N, then T» on a board: the inline quick-add of the «К работе» column — the
// seeded todo column, else one so named, else the first. Other groupings have
// no status columns; their first column takes it (submitAddTask files it).
watch(
  () => boardViewStore.quickAdd,
  () => {
    if (!editable() || layout.value !== 'board') return
    const cols = colModel.value
    const todo = groupMode.value === 'status' ? todoColumn(cols) : cols[0]
    if (todo) {
      if (isColCollapsed(todo)) toggleCollapse(todo)
      startAddTask(todo)
    }
  },
)

// ── inline column creation ("+ New column" to the right) ──
const addingColumn = ref(false)
const newColumnName = ref('')
const colInput = ref(null)
// A create already on the wire. The name input commits on BOTH Enter and blur, and it
// stays mounted — with the typed name still in it — until the POST comes back: the
// gesture is re-entrant. A second Enter on a slow link, or clicking away right after
// the first one, runs submitAddColumn again with the same non-empty name, and the
// server appends unconditionally (CreateColumn has no dedupe) — two identical columns,
// of which only the last one gets the expand override below. submitAddTask happens to
// be immune because it clears its input synchronously before awaiting; doing that here
// would throw the typed name away whenever the request fails, so the guard is explicit.
const columnInFlight = ref(false)
// «Новая колонка справа» (#2969) remembers which column the new one goes after;
// the plain «＋ колонка» at the end of the board leaves it null.
const addColumnAfter = ref(null)
function startAddColumn(after = null) {
  // A click handler hands over its event (a KeyboardEvent has a `key` too), so
  // only a column of this board counts.
  addColumnAfter.value = colModel.value.some((d) => d.key === after?.key) ? after.key : null
  addingColumn.value = true
  newColumnName.value = ''
  nextTick(() => colInput.value?.focus?.())
}
async function submitAddColumn() {
  if (columnInFlight.value) return
  const name = newColumnName.value.trim()
  if (!name) {
    addingColumn.value = false
    return
  }
  columnInFlight.value = true
  suppress()
  try {
    const { data } = await boards.createColumn(board.value.id, { name })
    // A column is born empty, so with «сворачивать пустые колонки» on the auto rule
    // (isColCollapsed) folded it the instant it appeared: the reader typed a name and
    // got a 44px strip back, with no header, no «＋ задача» and nothing to drop into
    // (`.col.collapsed` hides them all) — the create gesture undone by the setting.
    // Write the same explicit `false` a manual expand writes, BEFORE the reload that
    // draws it, so it never flashes collapsed. Turning the toggle on afterwards still
    // wins: its watcher clears the overrides of every empty column.
    if (data?.id) colCollapse[colCollapseKey({ key: data.id })] = false
    // Placed right of the column whose menu asked for it: the server appends,
    // so move it into the gap between that column and its right neighbour.
    const after = addColumnAfter.value
    if (data?.id && after) {
      const cols = colModel.value
      const i = cols.findIndex((d) => d.key === after)
      const next = i >= 0 ? cols[i + 1] : null
      if (i >= 0 && next) await columnsApi.move(data.id, { before_id: after, after_id: next.key })
    }
    addColumnAfter.value = null
    newColumnName.value = ''
    addingColumn.value = false
    await load(props.boardId)
  } catch (e) {
    // The name is deliberately left in the input here: the column was not created,
    // and retyping it is the reader's work, not the error's to take.
    message.error(e.message)
  } finally {
    columnInFlight.value = false
  }
}

// A task edit or tag-list change touches both tasks and workspace meta, so do
// a full (debounced) board reload.
// ── column actions of the v2 interaction model (#2969) ──
const undoStore = useUndoStore()
// The column a column key acts on: the one under the pointer, or the selected
// card's — but only while that selection is visible (kbdFocus); a card merely
// clicked is a starting point, not a target for ⌫.
function colTarget() {
  if (kbdFocus.value && focusedCard.value) {
    return colModel.value.find((d) => d.key === focusedCard.value.colKey) || null
  }
  return colModel.value.find((d) => d.key === hoveredColKey.value) || null
}
const colRenameReq = ref(null)
function requestColumnRename(dcol) {
  if (!dcol) return
  if (isColCollapsed(dcol)) toggleCollapse(dcol)
  colRenameReq.value = { key: dcol.key, n: (colRenameReq.value?.n || 0) + 1 }
}
function renameNextColumn(dcol) {
  const cols = colModel.value
  const next = cols[cols.findIndex((d) => d.key === dcol.key) + 1]
  if (next) requestColumnRename(next)
}
// Where the server sends a deleted column's cards (backend DeleteColumn): the
// first remaining column, or — for the completing column — the one that takes
// over the done flag, the rightmost.
function deleteTarget(dcol) {
  const rest = colModel.value.filter((d) => d.key !== dcol.key)
  if (!rest.length) return null
  return dcol.key === doneColumnId.value ? rest[rest.length - 1] : rest[0]
}
async function deleteColumn(dcol) {
  if (groupMode.value !== 'status' || !editable()) return
  const target = deleteTarget(dcol)
  if (!target) return
  const { key, name } = dcol
  const targetName = target.name
  const n = colCounts(dcol).tasks
  suppress()
  try {
    await columnsApi.remove(key)
  } catch (e) {
    message.error(humanizeError(e))
    return
  }
  if (focusedCard.value?.colKey === key) focusedCard.value = null
  if (hoveredColKey.value === key) hoveredColKey.value = null
  await load(props.boardId, { silent: true })
  undoStore.push({
    icon: 'trash',
    text: () =>
      n
        ? t('board.column.deleted', { name, target: targetName, tasks: t('board.column.tasks', n) })
        : t('board.column.deletedEmpty', { name }),
    doneText: () => t('board.column.restored', { name }),
    errorText: () => t('board.column.restoreFailed'),
    undo: async () => {
      suppress()
      await columnsApi.restore(key)
      await load(props.boardId, { silent: true })
    },
  })
}
async function moveColumn(dcol, dir) {
  if (groupMode.value !== 'status' || !editable()) return
  const cols = colModel.value
  const i = cols.findIndex((d) => d.key === dcol.key)
  if (i < 0) return
  const plan =
    dir === 'left'
      ? i > 0 && { before_id: cols[i - 2]?.key ?? null, after_id: cols[i - 1].key }
      : i < cols.length - 1 && { before_id: cols[i + 1].key, after_id: cols[i + 2]?.key ?? null }
  if (!plan) return
  suppress()
  try {
    await columnsApi.move(dcol.key, plan)
    await load(props.boardId, { silent: true })
  } catch (e) {
    message.error(humanizeError(e))
    load(props.boardId)
  }
}
useShortcuts(
  {
    'col.rename': () => requestColumnRename(colTarget()),
    'col.delete': () => {
      const d = colTarget()
      if (d && colModel.value.length > 1) deleteColumn(d)
    },
    'col.moveLeft': () => colTarget() && moveColumn(colTarget(), 'left'),
    'col.moveRight': () => colTarget() && moveColumn(colTarget(), 'right'),
  },
  { when: () => kanbanOn() && editable() && groupMode.value === 'status' && !!colTarget() },
)

function onChanged() {
  suppress()
  scheduleReload()
}

// Matrix view quick-add: create the task in the board's first column, then pin it
// to the chosen Eisenhower quadrant (the matrix has no column to drop into).
//
// No lanes here, so the grouping has no say in the new card's milestone — but the
// sprint scope still filters what the matrix is showing, so it does.
async function createInQuadrant({ title, quadrant }) {
  const columnId = columns.value[0]?.id
  if (!columnId) {
    message.warning(t('board.column.needStatus'))
    return
  }
  const milestoneId = newTaskMilestoneId({ scopeId: scopeMilestoneId() })
  suppress()
  try {
    const res = await boards.createTask(board.value.id, { column_id: columnId, title })
    await tasksApi.eisenhower(res.data.id, quadrant)
    if (milestoneId) await tasksApi.setMilestone(res.data.id, milestoneId)
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
    message.warning(t('board.toast.gitlabConflict'))
  }
  // The open board was retired. DeleteBoard takes its columns, cards and uploads
  // with it and announces a bare `board.deleted`, so there is nothing left to
  // re-fetch: leave for Home, the way onProjectGone does one level up. Above the
  // drag/suppress guard on purpose — this must not be swallowed by a drag in
  // flight, and it fires for the deleter too: the sidebar's «Удалить» doesn't
  // navigate, so whoever pressed it was left standing on a dead board where every
  // card drag and every new task 404s (#2932).
  if (boardGone(ev, props.boardId)) {
    message.info(t('board.toast.boardGone'))
    router.push('/')
    return
  }
  // Milestone CRUD elsewhere → refresh the project's milestone list so chips/columns update.
  if (typeof ev.type === 'string' && ev.type.startsWith('milestone.')) reloadMilestones()
  // The board's shared views changed (#2988) → re-read the list.
  if (ev.type === 'board_views.changed' && ev.data?.board_id === props.boardId) bv.load()
  // Custom command dictionary edited elsewhere → the `/`-popup picks it up live.
  if (ev.type === 'workspace_commands.updated') wsStore.setCustomCommands(ev.data?.commands || [])
  // A link between two tasks changed → nudge the Gantt to re-read its edge list.
  // Only pairs with an end on this board can move an arrow here, and only the
  // Gantt draws them, so this costs nothing on the other layouts.
  if (relationTouchesBoard(ev, props.boardId)) depsToken.value++
  // Board-activity toast for create/move on THIS board (any actor).
  if (ev.type === 'task.created' || ev.type === 'task.moved') pushActivity(ev)
  // A drag holds the board still (SortableJS needs a stable DOM), so the event
  // cannot be acted on now — but it must not be forgotten either: remember what it
  // would have cost and fetch that once the card lands (#2932).
  if (dragging.value || layoutDragging.value) {
    rememberMissed(ev)
    return
  }
  if (Date.now() < suppressReloadUntil) return
  // Route the event instead of reloading the whole board for every one of them.
  const kind = classifyEvent(ev, { boardId: props.boardId, hasTask: taskOnBoard })
  if (kind === 'ignore') return
  if (kind === 'patch') {
    // A full task payload can be merged into the card with no request at all —
    // unless the card has to move or isn't here yet, in which case applyTaskPatch
    // returns null and we fall back to re-fetching the lists.
    // The merged row is a fresh plain object; markRaw keeps it consistent with
    // the rest of the rows (see the shallowRef note above).
    const patched = applyTaskPatch(allTasks.value, ev.data, patchScope())
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
  for (const k of partialKinds(kind)) schedulePartial(k)
}, scheduleReload)

// Raise a live activity toast for a task create/move on the currently-open board.
// The verb for a move is refined by comparing the event's completion state to the
// card we still hold locally (the board reload is debounced, so the pre-move state
// is intact here): entering/leaving the done boundary reads as completed/reopened.
function pushActivity(ev) {
  const task = ev.data
  if (!task || typeof task !== 'object' || task.board_id !== props.boardId) return
  let verb = ev.type === 'task.created' ? 'created' : 'moved'
  if (ev.type === 'task.moved') {
    const prev = allTasks.value.find((x) => x.id === task.id)
    const wasDone = !!prev?.completed_at
    const isDone = !!task.completed_at
    if (isDone && !wasDone) verb = 'completed'
    else if (!isDone && wasDone) verb = 'reopened'
  }
  activityToasts.value?.push({
    id: task.id,
    number: task.number ?? null,
    title: task.title || t('task.fallbackTitle'),
    verb,
    ...toastActor(ev, { selfId: auth.user?.id ?? null, members: membersMap }),
  })
}

// Relation-change counter handed to the Gantt view. The chart owns its edge list
// (a fetch of its own), and a relation change touches no task row, so a plain
// board reload would not refresh a single arrow (#2932).
const depsToken = ref(0)

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

// The phone's board menu (BoardMobileMenu, in the header) opens the customize
// drawer through the store: the gear that opens it here sits in the composer's
// right-hand tools, which slide away once the chips fill the bar. The timeline
// layouts have no customize panel, so the nonce is ignored there — same gate the
// gear carries.
watch(
  () => boardViewStore.customizeNonce,
  () => openPanel(),
)

// The phone's board menu lists the saved views too (it has no sub-toolbar chip
// room to spare): publish the list and the loaded one, answer its ask.
watch(
  [() => bv.views, () => bv.loadedId],
  () => {
    boardViewStore.savedViews = bv.views.map((v) => ({
      id: v.id,
      name: v.name,
      layout: v.config?.layout || 'board',
    }))
    boardViewStore.currentViewId = bv.loadedId
  },
  { immediate: true },
)
watch(
  () => boardViewStore.viewRequest,
  (req) => {
    if (!req) return
    if (req.op === 'apply') {
      const v = bv.views.find((x) => x.id === req.id)
      if (v) bv.apply(v)
    } else if (req.op === 'clear') bv.clear()
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

// Opening a board: its locally remembered state, then the board itself, then its
// saved views — the default one wins on the first open of a session. A board
// this device has never shown, with no default view, opens in the view it was
// created in (#3017: boards.default_layout — Канбан or Список).
async function openBoard(id) {
  let seen
  try {
    seen = !!localStorage.getItem(`tessera_view_${id}`)
  } catch {
    seen = true // no storage — leave the layout alone
  }
  restoreView()
  await load(id)
  await bv.load()
  const applied = bv.onOpen()
  const start = board.value?.default_layout
  if (!applied && !seen && start && start !== layout.value) layout.value = start
  applyTaskQuery()
}

onMounted(async () => {
  window.addEventListener('pointerdown', onAnyPointer, true)
  ro = new ResizeObserver(() => measure())
  if (boardScroll.value) {
    ro.observe(boardScroll.value)
    measure()
  }
  // Re-fit the sub-toolbar on resize (window / sidebar toggle / chip set). The bar
  // is usually not in the DOM yet here (it waits for the first load): the
  // `subbarEl` watcher above is the attach that matters (#2932).
  subbarRO = new ResizeObserver(() => fitSubbar())
  if (subbarEl.value) subbarRO.observe(subbarEl.value)
  nextTick(fitSubbar)
  await openBoard(props.boardId)
})
onBeforeUnmount(() => {
  window.removeEventListener('pointerdown', onAnyPointer, true)
  ro?.disconnect()
  // Drop any pending debounced reload so it can't fire against a board we've just
  // navigated away from (e.g. its project was deleted) and 404 with a stray toast.
  clearTimeout(reloadTimer)
  for (const t of Object.values(partialTimers)) clearTimeout(t)
  subbarRO?.disconnect()
  onDragEnd()
  boardViewStore.reset()
})
watch(
  () => props.boardId,
  async (id) => {
    if (!id) return
    resetCardViewport()
    // The loaded view is per-board: drop it before this board's state comes in.
    bv.forget()
    await openBoard(id)
  },
)
watch(
  () => route.query.task,
  () => applyTaskQuery(),
)
// Switching étapes on the same board (only the query changes, no remount) reloads
// the scoped task set. Entering/leaving the archive reloads too.
//
// TWO getters, not one getter returning a pair. A single `() => [a, b]` source hands
// `watch` a fresh array every run, so its `Object.is` comparison always says
// "changed" — the callback then fires whenever the *dependency* is touched, and the
// dependency here is `route.query` as a whole (vue-router swaps that object on every
// navigation, whatever key moved). The board's own URL carries the open card
// (`?task=<number>`), so opening a card, canonicalizing its number and closing it
// again each ran this callback: three full, NON-silent `load()`s — ten requests apiece
// — for a query key this watcher does not care about, and every one of them raised the
// n-spin that fades the whole board (the #2695 flicker `silent` exists to avoid).
// Modal edits already come back through `@changed` → `onChanged`. Given per-key
// getters, `watch` compares the values themselves and stays quiet.
watch([() => route.query.milestone, () => route.query.archived], ([ms]) => {
  // Opening an étape via the project tree supersedes any manual étape filter in
  // the composer — clear it, else the scope's tasks would be filtered away.
  if (ms) filters.milestones = []
  load(props.boardId)
})

// Restore a task from the archive view (the only mutation allowed there).
async function restoreFromArchive(taskId) {
  try {
    await tasksApi.restore(taskId)
    allTasks.value = allTasks.value.filter((t) => t.id !== taskId)
    message.success(t('board.toast.restored'))
    // Restoring from the ACTIVE board — an archived task opened past the archive
    // scope (a relation, a notification, a document link) now offers Restore in
    // its read-only footer (#2932). The filter above is the archive list's move:
    // here the task was never in it, and the card that must now appear is missing
    // until the board is re-read. Silent, so the board doesn't flash for it.
    if (!archivedMode.value) await load(props.boardId, { silent: true })
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
      <!-- Sub-toolbar under the header (v2, #2954): the view switch, the composer
           (sort + filter chips only — the title search is the header's field),
           the grouping control, the board's people and its tools. The view switch
           and the composer share the free width: the switch shows as many views
           as fit beside the chips, and only when it is down to the active one do
           extra filter chips fold into «+N». Nothing else shrinks or truncates. -->
      <div ref="subbarEl" class="subbar">
        <!-- On a phone the view switch is part of the header's board menu. -->
        <BoardLayoutSwitch v-if="!isMobile" :available="switchAvail" @measured="fitSubbar" />
        <!-- The loaded saved view (#2987): its chip and quick menu. -->
        <ViewChip :bv="bv" @open-panel="openPanel('views')" />
        <div
          ref="composerEl"
          class="composer"
          :class="{ 'has-clear': hasClearableFacets }"
          data-tour="board-composer"
        >
          <n-icon class="composer-funnel" :component="FunnelOutline" :size="14" />
          <div ref="chipsClip" class="chips-clip">
            <!-- Scope chips: archive = amber tint, sprint = accent tint. -->
            <span
              v-if="archivedMode"
              class="facet facet-archive"
              :title="t('board.scope.archiveTitle')"
            >
              <n-icon class="facet-ic" :component="ArchiveOutline" :size="13" />
              {{ t('board.scope.archiveChip') }}
              <button
                class="facet-x"
                :title="t('board.scope.archiveExit')"
                @click.stop="exitArchive"
              >
                ×
              </button>
            </span>
            <span
              v-if="milestoneScope"
              class="facet facet-accent"
              :title="t('board.scope.milestoneTitle')"
            >
              <n-icon class="facet-ic" :component="RibbonOutline" :size="13" />
              {{ milestoneScopeLabel }}
              <button
                class="facet-x"
                :title="t('board.scope.milestoneClear')"
                @click.stop="clearMilestoneScope"
              >
                ×
              </button>
            </span>

            <!-- Sort chips: click toggles direction, drag reorders primary/secondary. The
                 touch trio keeps a swipe across the bar from reordering sort
                 precedence on a phone (#2932). -->
            <draggable
              v-if="sortLevels.length && !foldSorts"
              :list="sortLevels"
              item-key="field"
              handle=".facet"
              :animation="150"
              :delay="160"
              :delay-on-touch-only="true"
              :touch-start-threshold="6"
              class="sort-chips"
              ghost-class="facet-ghost"
            >
              <template #item="{ element: l }">
                <span
                  class="facet sortable"
                  :title="t('board.composer.sortHint')"
                  @click="toggleSortDir(l)"
                >
                  <n-icon class="facet-ic" :component="CHIP_ICONS.sort" :size="13" />
                  {{ sortFieldLabel(l.field) }} {{ l.dir === 'desc' ? '↓' : '↑' }}
                  <button
                    class="facet-x"
                    :title="t('board.composer.remove')"
                    @click.stop="removeSort(l)"
                  >
                    ×
                  </button>
                </span>
              </template>
            </draggable>

            <!-- Filter chips (priority / assignee / tag / status / milestone / due).
                 The ones that no longer fit stay laid out but invisible; «+N»
                 below lists them. -->
            <template v-for="(c, ci) in filterChips" :key="ci">
              <span v-if="!hiddenChips.includes(ci)" class="facet">
                <n-icon class="facet-ic" :component="c.icon" :size="13" />
                {{ c.text }}
                <button
                  class="facet-x"
                  :title="t('board.composer.remove')"
                  @click.stop="removeChip(c)"
                >
                  ×
                </button>
              </span>
            </template>
          </div>

          <n-popover v-if="foldedCount" trigger="click" placement="bottom-start">
            <template #trigger>
              <button
                type="button"
                class="facet facet-more"
                data-testid="composer-more"
                :title="t('board.composer.more', { count: foldedCount })"
              >
                +{{ foldedCount }}
              </button>
            </template>
            <div class="more-chips">
              <template v-if="foldSorts">
                <span
                  v-for="l in sortLevels"
                  :key="l.field"
                  class="facet sortable"
                  @click="toggleSortDir(l)"
                >
                  <n-icon class="facet-ic" :component="CHIP_ICONS.sort" :size="13" />
                  {{ sortFieldLabel(l.field) }} {{ l.dir === 'desc' ? '↓' : '↑' }}
                  <button
                    class="facet-x"
                    :title="t('board.composer.remove')"
                    @click.stop="removeSort(l)"
                  >
                    ×
                  </button>
                </span>
              </template>
              <span v-for="ci in hiddenChips" :key="ci" class="facet">
                <n-icon class="facet-ic" :component="filterChips[ci].icon" :size="13" />
                {{ filterChips[ci].text }}
                <button
                  class="facet-x"
                  :title="t('board.composer.remove')"
                  @click.stop="removeChip(filterChips[ci])"
                >
                  ×
                </button>
              </span>
            </div>
          </n-popover>

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
            <button class="facet-add" :title="t('board.composer.add')">
              <n-icon :component="AddOutline" :size="14" />
            </button>
          </n-dropdown>

          <button
            v-if="hasClearableFacets"
            class="facet-clear"
            :title="t('board.composer.clear')"
            @click="clearAll"
          >
            ×
          </button>
        </div>

        <!-- Grouping: its own control next to the composer (v2, #2954) — a
             setting of the board, not one more filter. Opens the same grouping
             options as the composer's add menu. -->
        <n-dropdown
          v-if="groupChip"
          trigger="click"
          placement="bottom-start"
          :options="groupOptions"
          @select="onAddFacet"
        >
          <button
            ref="groupCtlEl"
            type="button"
            class="group-ctl"
            data-testid="group-control"
            data-tour="board-grouping"
          >
            <n-icon class="gc-ic" :component="groupChip.icon" :size="14" />
            <span v-if="!isMobile && !compactGroup" class="gc-lbl">{{
              t('board.facet.menu.group')
            }}</span>
            <span class="gc-val">{{ groupChip.text }}</span>
            <n-icon class="gc-ic" :component="ChevronDownOutline" :size="12" />
          </button>
        </n-dropdown>

        <!-- Gantt / timeline: scale, today, key hints, view settings (v2, #2959) -->
        <ChartControls
          v-if="timelineLike"
          :ctl="chartView?.ctl"
          :hints="!hideChartHints && !isMobile"
          :compact="isMobile || compactChart"
        />

        <BoardMembers v-if="!isMobile && !hidePeople" class="subbar-people" />

        <!-- Right-side tools — slide off to the right while the composer is
             expanded so the full chip set has room. -->
        <div class="bar-tools">
          <!-- Subtasks: compact rows on the card ⇄ full child cards (v2: a tool of
               its own, out of the composer). -->
          <n-tooltip>
            <template #trigger>
              <n-button
                size="small"
                quaternary
                class="ngrad bar-btn"
                :type="subtasksExpanded ? 'primary' : 'default'"
                data-testid="subtasks-toggle"
                @click="toggleSubtasksExpanded"
              >
                <template #icon><n-icon :component="GitBranchOutline" /></template>
              </n-button>
            </template>
            {{ t(subtasksExpanded ? 'board.composer.subtasksOn' : 'board.composer.subtasksOff') }}
          </n-tooltip>

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
            {{ t(autoActive ? 'board.tools.autoOn' : 'board.tools.autoOff') }}
          </n-tooltip>

          <!-- «Вид доски» (#2986): the one entry to every board setting, in every
               view. The dot says the loaded view has been changed. -->
          <n-tooltip>
            <template #trigger>
              <span class="panel-wrap">
                <n-button
                  size="small"
                  quaternary
                  class="ngrad bar-btn"
                  :type="customizeOpen ? 'primary' : 'default'"
                  data-tour="board-customize"
                  data-testid="board-customize-btn"
                  @click="customizeOpen ? (customizeOpen = false) : openPanel()"
                >
                  <template #icon><n-icon :component="OptionsOutline" /></template>
                </n-button>
                <span v-if="bv.changed" class="panel-dot" data-testid="board-customize-dot" />
              </span>
            </template>
            {{ t('board.tools.customize') }} · <ShortcutKeys id="view.panel" />
          </n-tooltip>
        </div>
      </div>

      <!-- The board and its «Вид доски» panel side by side (#2986): the panel is
           docked, not floating — whatever the view, it gives up the width. -->
      <div class="board-body">
        <div class="board-main">
          <BoardListView
            v-if="layout === 'list'"
            :columns="displayColumns"
            :status-columns="columns"
            :lists="lists"
            :tags-map="tagsMap"
            :members-map="membersMap"
            :gitlab-members="gitlabMembersList"
            :tag-prefix-names="tagPrefixNames"
            :readonly="archivedMode"
            @open="openTask"
            @changed="onChanged"
            @restore="restoreFromArchive"
          />

          <BoardCalendarView
            v-else-if="layout === 'calendar'"
            :tasks="filteredTasks"
            :status-columns="columns"
            :readonly="archivedMode"
            @open="openTask"
            @changed="onChanged"
            @restore="restoreFromArchive"
          />

          <BoardTimelineView
            v-else-if="layout === 'timeline'"
            ref="chartView"
            :done-column-id="board?.done_column_id || null"
            :tasks="filteredTasks"
            :status-columns="columns"
            :members-map="membersMap"
            :gitlab-members="gitlabMembersList"
            :tags-map="tagsMap"
            :group-mode="groupMode"
            :tag-prefix="tagPrefix"
            :tag-prefix-names="tagPrefixNames"
            :project-id="board?.project_id"
            :subtasks-by-parent="filteredSubtasksByParent"
            :milestones="milestonesList"
            :readonly="archivedMode"
            @open="openTask"
            @changed="onChanged"
            @restore="restoreFromArchive"
          />

          <BoardGanttView
            v-else-if="layout === 'gantt'"
            ref="chartView"
            :done-column-id="board?.done_column_id || null"
            :board-id="boardId"
            :tasks="filteredTasks"
            :status-columns="columns"
            :members-map="membersMap"
            :gitlab-members="gitlabMembersList"
            :tags-map="tagsMap"
            :group-mode="groupMode"
            :tag-prefix="tagPrefix"
            :tag-prefix-names="tagPrefixNames"
            :project-id="board?.project_id"
            :auto-sort="autoActive"
            :subtasks-by-parent="filteredSubtasksByParent"
            :milestones="milestonesList"
            :deps-token="depsToken"
            :readonly="archivedMode"
            @open="openTask"
            @changed="onChanged"
            @restore="restoreFromArchive"
          />

          <BoardMatrixView
            v-else-if="layout === 'matrix'"
            :tasks="filteredTasks"
            :subtasks-by-parent="sortedSubtasksByParent"
            :subtasks-total-by-parent="subtasksByParent"
            :subtasks-expanded="subtasksExpanded"
            :readonly="archivedMode"
            @open="openTask"
            @changed="onChanged"
            @create="createInQuadrant"
            @restore="restoreFromArchive"
            @drag-state="onLayoutDragState"
          />

          <div v-else ref="boardScroll" class="board-scroll" :style="colStyleVars">
            <draggable
              :list="colModel"
              group="columns"
              item-key="key"
              handle=".col-drag"
              :disabled="!canReorderColumns"
              class="cols"
              :class="{ dragging }"
              :animation="150"
              :delay="160"
              :delay-on-touch-only="true"
              :touch-start-threshold="6"
              @start="onDragStart"
              @end="onBoardDragEnd"
              @change="onColumnReorder"
            >
              <template #item="{ element: dcol }">
                <div
                  class="col"
                  data-testid="column"
                  :data-column-key="dcol.key"
                  :data-column-name="dcol.rawName"
                  :class="{ collapsed: colCollapsedNow(dcol) }"
                  :style="{ '--col-c': dcol.color || NEUTRAL_COLUMN }"
                  @mouseenter="hoveredColKey = dcol.key"
                  @mouseleave="hoveredColKey === dcol.key && (hoveredColKey = null)"
                >
                  <!-- Collapsed strip: rotated title + count, click to expand. The real
                   header + card list stay in the DOM below (hidden via CSS) so the
                   drop target survives.
                   A real `<button>`, not a clickable div: the collapse chevron in
                   the header IS reachable by keyboard (an n-button), and collapsing
                   hides that header outright — so a column folded from the keyboard
                   had no way back, the strip never entering the tab order. Its
                   caption carries the column name too: the vertical title is capped
                   at 260px and ellipsised, and under TAG grouping that name is a tag
                   («сложность::высокая»), which is the very case `.col-title` got its
                   own `title` for.
                   It is also the column's drag handle while collapsed. The board's
                   column draggable takes `handle: '.col-drag'`, and ColumnHeader
                   stamps that class on the status glyph and the caption — both inside
                   `.col-head`, which a collapsed column hides outright. So a folded
                   column could not be reordered at all, and with «collapse empty
                   columns» on that is EVERY empty column — precisely the ones being
                   arranged while a board is still being set up. -->
                  <button
                    v-if="colCollapsedNow(dcol)"
                    class="col-strip"
                    :class="{ 'col-drag': canReorderColumns }"
                    type="button"
                    :title="stripTitle(dcol)"
                    @click="toggleCollapse(dcol)"
                  >
                    <n-icon class="strip-chevron" :component="ChevronForwardOutline" />
                    <span class="strip-title">{{ dcol.name }}</span>
                    <span class="strip-count" :title="colCountTitle(dcol)">{{
                      colCounts(dcol).tasks
                    }}</span>
                  </button>
                  <ColumnHeader
                    :dcol="dcol"
                    :count="colCounts(dcol).tasks"
                    :total="colCounts(dcol).total"
                    :estimate="columnEstimate(dcol)"
                    :editable="groupMode === 'status'"
                    :readonly="archivedMode"
                    :is-done="groupMode === 'status' && dcol.key === doneColumnId"
                    :first="groupMode === 'status' && dcol.key === (colModel[0] && colModel[0].key)"
                    :collapsed="isColCollapsed(dcol)"
                    :prefix-names="tagPrefixNames"
                    :last="
                      groupMode === 'status' && dcol.key === colModel[colModel.length - 1]?.key
                    "
                    :can-delete="groupMode === 'status' && colModel.length > 1"
                    :rename-request="colRenameReq"
                    @changed="onChanged"
                    @set-done="onSetDone"
                    @toggle-collapse="toggleCollapse(dcol)"
                    @rename-next="renameNextColumn(dcol)"
                    @delete="deleteColumn(dcol)"
                    @move="(dir) => moveColumn(dcol, dir)"
                    @add-right="startAddColumn(dcol)"
                  />
                  <draggable
                    :list="lists[dcol.key]"
                    group="tasks"
                    item-key="id"
                    class="drop t-hoverscroll"
                    ghost-class="ghost"
                    :force-fallback="true"
                    :fallback-on-body="true"
                    filter=".add-sub, .sub-add-input"
                    :prevent-on-filter="false"
                    :disabled="archivedMode"
                    :sort="canSortCards"
                    :animation="150"
                    :delay="160"
                    :delay-on-touch-only="true"
                    :touch-start-threshold="6"
                    @start="onCardDragStart"
                    @end="onBoardDragEnd"
                    @change="onColChange($event, dcol)"
                  >
                    <template #item="{ element, index }">
                      <div
                        :ref="(el) => regCard(el, cardKey(dcol.key, element.id))"
                        class="card-wrap"
                        :data-card="cardKey(dcol.key, element.id)"
                      >
                        <div
                          v-if="
                            colCollapsedNow(dcol) ||
                            !(vis[cardKey(dcol.key, element.id)] ?? index < 12)
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
                          :sorted="composerSorted"
                          :dragging="draggingCard"
                          :readonly="archivedMode"
                          :selected="isFocused(dcol, element.id)"
                          @open="(id) => onCardOpen(dcol, id)"
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
                        :placeholder="t('board.column.taskPlaceholder')"
                        @keydown.enter.exact.prevent="submitAddTask(dcol)"
                        @keyup.esc="cancelAddTask"
                        @blur="submitAddTask(dcol)"
                      />
                    </div>
                    <button
                      v-else
                      type="button"
                      class="add-task"
                      data-testid="add-task-button"
                      @click="startAddTask(dcol)"
                    >
                      <n-icon :component="AddOutline" :size="15" />
                      <span class="add-task-label">{{ t('board.column.addTask') }}</span>
                      <ShortcutKeys v-if="!isMobile" id="board.addTask" class="add-task-key" />
                    </button>
                  </template>
                </div>
              </template>
            </draggable>

            <!-- inline new column (status mode; never in the archive — the tile would
             create a live column on the board from a read-only view) -->
            <div v-if="showAddColumn" class="add-col" :class="{ 'add-col-mobile': isMobile }">
              <n-input
                v-if="addingColumn"
                ref="colInput"
                v-model:value="newColumnName"
                size="small"
                :loading="columnInFlight"
                :placeholder="t('board.column.namePlaceholder')"
                @keyup.enter="submitAddColumn"
                @keyup.esc="addingColumn = false"
                @blur="submitAddColumn"
              />
              <button
                v-else
                type="button"
                class="add-col-btn"
                data-testid="add-column"
                @click="startAddColumn"
              >
                <n-icon :component="AddOutline" :size="16" />
                <span>{{ t('board.column.add') }}</span>
              </button>
            </div>
          </div>

          <div v-if="layout === 'board' && !displayColumns.length" class="empty-board">
            <n-text depth="3">
              {{ t(groupMode === 'status' ? 'board.column.emptyStatus' : 'board.column.emptyTag') }}
            </n-text>
          </div>
        </div>
        <BoardCustomizePanel
          ref="customizePanel"
          v-model:show="customizeOpen"
          v-model:board-name="boardName"
          v-model:card-size="cardSize"
          v-model:stack-fields="stackFields"
          v-model:show-empty="showEmpty"
          v-model:auto-collapse-empty="autoCollapseEmpty"
          v-model:subtasks-expanded="subtasksExpanded"
          :layout="layout"
          :readonly="archivedMode"
          :field-vis="fieldVis"
          :facet-chips="facetChips"
          :filters="filters"
          :group-next-label="nextGroupModeLabel"
          :group-label="groupChip?.text || ''"
          :add-options="addOptions"
          :board-icon="board?.icon || ''"
          :board-color="board?.color || ''"
          :board-icon-mode="board?.icon_mode || 'badge'"
          :chart-ctl="timelineLike ? chartView?.ctl || null : null"
          :views-summary="viewsSummary"
          :digits-hint="bv.views.length > 0"
          @set-field="setFieldVis"
          @add-facet="onAddFacet"
          @remove-chip="removeChip"
          @chip-click="onChipClick"
          @update-board="updateBoard"
        >
          <template #views>
            <ViewsSection
              :bv="bv"
              :can-share="wsStore.canManage"
              :members="boardViewStore.membersList.length"
            />
          </template>
        </BoardCustomizePanel>
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
    :nav="taskNav"
    @update:show="(v) => v || closeTask()"
    @changed="onChanged"
    @navigate="navigateTask"
    @open="openTask"
    @restore="restoreFromArchive"
  />

  <BoardActivityToasts ref="activityToasts" @open="openTask" />
</template>

<style scoped>
/* v2 (#2954): the sub-toolbar sits on the canvas with no rule under it — the
   tinted lanes below are what set the board apart. */
.subbar {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 10px;
  /* The fit keeps everything inside; this only guards a frame mid-resize. */
  overflow: hidden;
}
.subbar-spacer {
  flex: 1;
}
/* The board and its docked «Вид доски» panel (#2986): the views take what the
   panel leaves. */
.board-body {
  display: flex;
  align-items: flex-start;
  gap: 12px;
}
.board-main {
  flex: 1;
  min-width: 0;
}
.panel-wrap {
  position: relative;
  display: inline-flex;
}
/* «изменено»: the loaded view no longer matches the board. */
.panel-dot {
  position: absolute;
  top: 6px;
  right: 6px;
  width: 7px;
  height: 7px;
  border-radius: 4px;
  background: linear-gradient(
    to top right,
    color-mix(in srgb, #f0a020 86%, #000),
    #f0a020 50%,
    color-mix(in srgb, #f0a020 86%, #fff)
  );
  box-shadow: 0 0 0 2px var(--t-bg);
  pointer-events: none;
}
.subbar-people {
  flex: none;
  margin-left: auto;
}
.bar-tools {
  margin-left: auto;
}
.subbar-people + .bar-tools {
  margin-left: 0;
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
  min-height: calc(100vh - 52px);
  box-sizing: border-box;
}
/* Right-side toolbar buttons match the composer bar's height. */
.bar-btn {
  height: 34px;
  width: 34px;
}
.bar-tools {
  display: flex;
  align-items: center;
  gap: 4px;
  flex: none;
}
/* composer bar — v2 (#2954): a filled control well holding only the sort and
   filter chips (+ the add button). It takes its content's width; when the row
   is short it shrinks and the chips clip, the overflow folding into «+N». */
.composer {
  position: relative;
  flex: none;
  display: flex;
  flex-wrap: nowrap;
  align-items: center;
  gap: 6px;
  box-sizing: border-box;
  min-height: 34px;
  padding: 0 6px 0 10px;
  border-radius: var(--t-r-ctl);
  background: var(--t-ctl);
  /* Tame the inherited (Naive) line-height so a chip stays 24px tall. */
  line-height: 1.25;
}
.chips-clip {
  flex: none;
  display: flex;
  align-items: center;
  gap: 6px;
}
.chips-clip:empty {
  display: none;
}
.facet.facet-more {
  flex: none;
  border: none;
  padding: 0 9px;
  background: var(--t-pill);
  color: var(--t-text2);
  font: inherit;
  font-size: 12.5px;
  font-weight: 600;
  cursor: pointer;
}
.more-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  max-width: 320px;
}
.facet {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  font-size: 12.5px;
  height: 24px;
  box-sizing: border-box;
  padding: 0 4px 0 9px;
  border-radius: var(--t-r-pill);
  background: linear-gradient(
    to top right,
    color-mix(in srgb, var(--t-primary) 14%, var(--t-ctl)),
    color-mix(in srgb, var(--t-primary) 24%, var(--t-ctl))
  );
  color: var(--t-text1);
  white-space: nowrap;
}
.composer-funnel {
  flex: none;
  color: var(--t-text3);
}
/* Grouping control beside the composer: «Группировка <value> ⌄» on the control
   well — a board setting, so neutral, not an accent chip. */
.group-ctl {
  flex: none;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 34px;
  padding: 0 10px;
  border: none;
  border-radius: var(--t-r-ctl);
  background: var(--t-ctl);
  font: inherit;
  font-size: 13px;
  white-space: nowrap;
  cursor: pointer;
}
.group-ctl:hover {
  background: var(--t-hover);
}
.gc-ic {
  color: var(--t-text3);
}
.gc-lbl {
  color: var(--t-text3);
}
.gc-val {
  font-weight: 600;
  color: var(--t-text1);
}
/* Sprint scope chip: the accent capsule like any filter. */
.facet-accent .facet-ic {
  color: var(--t-primary);
  opacity: 1;
}
/* Archive scope chip: amber capsule. */
.facet-archive {
  background: linear-gradient(
    to top right,
    color-mix(in srgb, #e0922f 14%, var(--t-ctl)),
    color-mix(in srgb, #e0922f 24%, var(--t-ctl))
  );
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
  background: var(--t-hover);
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
  border-radius: 50%;
  border: none;
  background: var(--t-pill);
  color: var(--t-text2);
  cursor: pointer;
  flex: none;
}
.facet-add:hover {
  color: var(--t-text1);
}
/* Reference figure, not a control — flat neutral grey, no accent gradient. */
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
    min-height: calc(100dvh - 52px);
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
  /* The bar is already squeezed there; the counter is the first thing to drop. */
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
  flex: 0 0 200px;
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
  flex: none;
}
.col {
  box-sizing: border-box;
  width: var(--col-w, 256px);
  flex: 0 0 var(--col-w, 256px);
  border-radius: var(--t-r-col);
  padding: 8px;
  align-self: flex-start;
  max-height: calc(100vh - 180px);
  display: flex;
  flex-direction: column;
  gap: 8px;
  /* v2 «coloured lanes» (#2954): the column is a wash of its own colour over the
     column level — no border, no top bar. The share is a theme token (15% dark,
     11% light). */
  background: color-mix(in srgb, var(--col-c) var(--t-col-tone), var(--t-col));
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
  padding: 8px 0 14px;
  cursor: pointer;
  overflow: hidden;
}
.col.collapsed :deep(.col-head),
.col.collapsed .drop,
.col.collapsed .add-task,
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
  gap: 10px;
  padding: 4px 0;
  /* The strip IS the tap target (it carries the expand click), so it has to fill
     the collapsed tile rather than stop at the height of its own text. Desktop
     never noticed: `.col` is `align-self: flex-start`, so the tile is already
     exactly the strip's height. On a phone `.col` stretches to the full board
     height (mobile block above) while the strip stayed content-sized — chevron,
     a title capped at 260px, a count — leaving most of the tall 44px tile as
     dead area that showed `cursor: pointer` and swallowed the tap. And there is
     nothing else to tap: `.col-head` and its collapse button are hidden on a
     collapsed column, which is why #2893 kept the strip as the only way back
     out. The drag drop overlay already covers the whole tile (`inset: 0`) — the
     tap target now agrees with it. */
  flex: 1 0 auto;
  /* Button reset: the strip is a `<button>` so it can be tabbed to and opened with
     Enter/Space (the collapse chevron it replaces is an n-button, and collapsing
     hides it). A button doesn't inherit the page font and brings its own chrome,
     so the reset has to be explicit — `width: 100%` because a button won't stretch
     to the tile the way the old block-level div did. */
  appearance: none;
  background: none;
  border: 0;
  width: 100%;
  font: inherit;
  color: inherit;
  cursor: pointer;
}
/* The grab hand for the strip-as-drag-handle. ColumnHeader has the same rule for
   its own `.col-drag` elements, but that one is `<style scoped>` on a different
   component and never reaches this button — and `.col-strip` right above sets
   `cursor: pointer`, which a bare `.col-drag` selector would tie with and lose to
   on source order. Two classes, so it wins outright. */
.col-strip.col-drag {
  cursor: grab;
}
.col-strip:focus-visible {
  outline: 2px solid var(--t-primary);
  outline-offset: -2px;
  border-radius: 8px;
}
.strip-chevron {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border-radius: var(--t-r-ctl);
  font-size: 15px;
  color: var(--t-text3);
  flex: none;
}
.col-strip:hover .strip-chevron {
  background: var(--t-hover);
  color: var(--t-text1);
}
.strip-title {
  writing-mode: vertical-rl;
  font-size: 13.5px;
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
  font-variant-numeric: tabular-nums;
  flex: none;
}
/* Mobile keeps the same 44px strip as the desktop (#2893), with NO override of
   its own. The one that used to live here ("stay full width on a phone, hide the
   strip") showed the column's shell but never its cards: a collapsed column
   renders every card as a bare `.card-ph` spacer (see the template), so revealing
   `.drop` produced a full-width column with nothing in it. A strip is also what
   "collapse" is for on the narrowest screen — one column fills the viewport, so
   folding the ones you aren't using is how you reach the rest.
   The tempting half-measure — `scroll-snap-align: none` on the strip, so a swipe
   carries past a run of them instead of catching on each 44px tile — is a trap,
   and the mobile tier caught it: `.board-scroll` snaps `x mandatory`, so a column
   with no snap point of its own is not merely skipped by a swipe, it cannot be
   rested on at all. Collapsing the leftmost column scrolled it off the screen on
   the spot and there was no way back to it. Strips keep their snap point. */
/* `.col-head` is ColumnHeader's ROOT element, so this scoped rule does reach it
   (Vue stamps the parent's scope id on a child component's root) — it is the only
   part of the old inline header that still applies here. Everything INSIDE that
   component (`.col-title`, `.col-menu`, the swatch grid, the count chip) is a
   non-root child and carries only ColumnHeader's own scope id: rules for those
   sat here doing nothing, a standing trap for the next person who edits the
   caption's layout in this file and sees no effect. They live in
   ColumnHeader.vue's own <style scoped>. */
.col-head {
  display: flex;
  align-items: center;
}
.drop {
  flex: 1;
  overflow-y: auto;
  min-height: 8px;
  /* overflow-y:auto forces overflow-x to clip; pad so a focused subtask/task
     input's left border ring isn't cut off. */
  padding: 0 3px;
}
/* The card under the pointer (v2, #2954). Sortable runs in fallback mode so the
   moving copy is a real DOM clone (the browser's native drag image cannot be
   styled): lifted on the drag shadow and tilted. Sortable drives the clone's own
   transform and opacity inline, so the tilt goes on the card inside it. The
   class is Sortable's own: useBoardDragScroll follows that clone to auto-scroll
   the board at its edges. */
.sortable-fallback {
  opacity: 1 !important;
}
.sortable-fallback :deep(.card) {
  --card-fill: var(--t-card-hover);
  box-shadow: var(--t-sh-drag);
  transform: translateX(10px) rotate(2.5deg);
}
.sortable-fallback :deep(.card-actions) {
  display: none;
}
/* Where the dragged card will land: a recessed slot in the drop colour (v2,
   #2954) rather than a faded copy of the card. */
.ghost :deep(.card),
.ghost :deep(.card:hover) {
  background: var(--t-drop);
  box-shadow: var(--t-sh-drop);
}
.ghost :deep(.card > *) {
  visibility: hidden;
}
/* Off-screen card stand-in: occupies the card's last-measured height so the
   scrollbar and drop positions stay correct while the heavy TaskCard is unmounted
   (see the IntersectionObserver windowing in the script). */
.card-ph {
  pointer-events: none;
}
/* «Создать задачу» at the foot of the lane: a 34px row in the column's own hue
   that unfolds while the pointer is over the lane (the lane grows by it, as in
   the v2 design); touch screens have no hover, so there it is always open.
   Folded, it also cancels the lane's 8px gap so no stray space is left. */
.add-task {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: none;
  height: 0;
  margin-top: -8px;
  overflow: hidden;
  padding: 0 7px 0 10px;
  border: none;
  border-radius: var(--t-r-card);
  background: color-mix(in srgb, var(--col-c) 12%, transparent);
  color: var(--t-text2);
  font: inherit;
  font-size: 13px;
  font-weight: 500;
  text-align: left;
  cursor: pointer;
  opacity: 0;
  transition:
    height 0.15s ease,
    margin 0.15s ease,
    opacity 0.15s ease,
    background 0.15s ease;
}
.add-task-label {
  flex: 1;
}
.col:hover .add-task,
.add-task:focus-visible {
  height: 34px;
  margin-top: 0;
  opacity: 1;
}
.add-task:hover {
  background: color-mix(in srgb, var(--col-c) 20%, transparent);
  color: var(--t-text1);
}
@media (hover: none) {
  .add-task-key {
    display: none;
  }
  .add-task {
    height: 34px;
    margin-top: 0;
    opacity: 1;
  }
}
.add-col-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  width: 200px;
  height: 44px;
  border: none;
  border-radius: var(--t-r-col);
  background: var(--t-col);
  color: var(--t-text3);
  font: inherit;
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
}
.add-col-btn:hover {
  color: var(--t-text1);
}
.add-col-mobile .add-col-btn {
  width: 100%;
}
.empty-board {
  padding: 24px;
}
</style>
