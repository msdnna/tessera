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
  GitBranchOutline,
  SaveOutline,
  FolderOpenOutline,
  TrashOutline,
  AddOutline,
  ChevronForwardOutline,
  ChevronBackOutline,
} from '@vicons/ionicons5'
import {
  boards,
  tasks as tasksApi,
  workspaces as wsApi,
  columns as columnsApi,
  projects as projectsApi,
} from '@/api'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useBoardViewStore } from '@/stores/boardView'
import { useThemeStore } from '@/stores/theme'
import { useRealtime } from '@/composables/useRealtime'
import { useResponsive } from '@/composables/useResponsive'
import { useOverlayBack } from '@/composables/useOverlayBack'
import { PRIORITY_LABELS } from '@/styles/tokens'
import { tagNamespace, prefixLabel, buildTagGroups } from '@/utils/tagGroups'
import { storeToRefs } from 'pinia'
import TaskCard from './TaskCard.vue'
import TaskModal from './TaskModal.vue'
import TesseraSpinner from './TesseraSpinner.vue'
import ColumnHeader from './ColumnHeader.vue'
import BoardListView from './BoardListView.vue'
import BoardCalendarView from './BoardCalendarView.vue'
import BoardMatrixView from './BoardMatrixView.vue'

const props = defineProps({ boardId: { type: String, required: true } })

const message = useMessage()
const wsStore = useWorkspacesStore()
const boardViewStore = useBoardViewStore()
// `layout` lives in the store so the header switcher and the board stay in sync.
const { layout } = storeToRefs(boardViewStore)
const route = useRoute()
const router = useRouter()
const { isMobile } = useResponsive()

const loading = ref(false)
const board = ref(null)
const columns = ref([])
const allTasks = ref([])
const subtasksByParent = ref({})
const lists = ref({})
const tagsMap = reactive({})
const membersMap = reactive({})
const tagsList = computed(() => Object.values(tagsMap))
const membersList = computed(() => Object.values(membersMap))

// view controls (layout comes from the store, above)
const subtasksExpanded = ref(false) // full property cards vs compact rows
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
const filters = reactive({ priorities: [], assignees: [], tags: [], due: '', q: '' })
const sortFieldOptions = [
  { label: 'Приоритет', value: 'priority' },
  { label: 'Срок', value: 'due' },
  { label: 'Название', value: 'title' },
  { label: 'Номер', value: 'number' },
]
// One sort level's comparison (direction applied; due-less tasks always last).
function cmpLevel(a, b, { field, dir }) {
  const d = dir === 'desc' ? -1 : 1
  if (field === 'due') {
    const av = a.due_date ? Date.parse(a.due_date) : null
    const bv = b.due_date ? Date.parse(b.due_date) : null
    if (av === null && bv === null) return 0
    if (av === null) return 1
    if (bv === null) return -1
    return d * (av - bv)
  }
  if (field === 'priority') return d * ((a.priority || 0) - (b.priority || 0))
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
const memberFilterOptions = computed(() =>
  membersList.value.map((m) => ({ label: m.name, value: m.user_id })),
)
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
const activeFilterCount = computed(
  () =>
    filters.priorities.length +
    filters.assignees.length +
    filters.tags.length +
    (filters.due ? 1 : 0) +
    (filters.q.trim() ? 1 : 0),
)
function resetFilters() {
  filters.priorities = []
  filters.assignees = []
  filters.tags = []
  filters.due = ''
  filters.q = ''
}

// ── composer bar: grouping + sort + filters as removable chips ──
// All facets render as chips over the existing state; an "add" dropdown mutates
// the same refs. The search box lives in the bar too (filters.q).
const facetChips = computed(() => {
  const out = []
  out.push({
    kind: 'group',
    label:
      groupMode.value === 'tag'
        ? `Группировка: теги${tagPrefix.value ? ` · ${prefixLabel(tagPrefix.value, tagPrefixNames)}` : ''}`
        : 'Группировка: статусы',
  })
  sortLevels.value.forEach((l, i) => {
    const f = sortFieldOptions.find((o) => o.value === l.field)?.label || l.field
    out.push({ kind: 'sort', i, label: `Сорт: ${f} ${l.dir === 'desc' ? '↓' : '↑'}` })
  })
  filters.priorities.forEach((p) =>
    out.push({ kind: 'priority', value: p, label: `Приоритет: ${PRIORITY_LABELS[p]}` }),
  )
  filters.assignees.forEach((a) =>
    out.push({ kind: 'assignee', value: a, label: `Исполнитель: ${membersMap[a]?.name || '—'}` }),
  )
  filters.tags.forEach((t) =>
    out.push({ kind: 'tag', value: t, label: `Тег: ${tagsMap[t]?.name || '—'}` }),
  )
  if (filters.due) {
    out.push({ kind: 'due', label: `Срок: ${dueOptions.find((o) => o.value === filters.due)?.label || filters.due}` })
  }
  return out
})
const addOptions = computed(() => [
  {
    label: 'Группировка',
    key: 'group',
    children: [
      { label: 'По статусам', key: 'g.status' },
      { label: 'По тегам (все)', key: 'g.tag' },
      ...tagPrefixOptions.value
        .filter((o) => o.value)
        .map((o) => ({ label: `По тегам · ${o.label}`, key: `g.tagp.${encodeURIComponent(o.value)}` })),
    ],
  },
  {
    label: 'Сортировка',
    key: 'sort',
    children: sortFieldOptions.map((o) => ({ label: o.label, key: `s.${o.value}` })),
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
  {
    label: 'Фильтр: тег',
    key: 'ft',
    children: tagFilterMenu.value,
  },
  {
    label: 'Фильтр: срок',
    key: 'fd',
    children: dueOptions.filter((o) => o.value).map((o) => ({ label: o.label, key: `fd.${o.value}` })),
  },
])
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
  } else if (key.startsWith('fd.')) {
    filters.due = key.slice(3)
  }
}
function removeChip(c) {
  if (c.kind === 'sort') sortLevels.value.splice(c.i, 1)
  else if (c.kind === 'priority') filters.priorities = filters.priorities.filter((x) => x !== c.value)
  else if (c.kind === 'assignee') filters.assignees = filters.assignees.filter((x) => x !== c.value)
  else if (c.kind === 'tag') filters.tags = filters.tags.filter((x) => x !== c.value)
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

// ── per-board view persistence (localStorage, per device) ──
const viewKey = computed(() => `tessera_view_${props.boardId}`)
let restoring = false
function writeView() {
  if (restoring) return
  try {
    localStorage.setItem(
      viewKey.value,
      JSON.stringify({
        layout: layout.value,
        groupMode: groupMode.value,
        tagPrefix: tagPrefix.value,
        sortLevels: sortLevels.value,
        subtasksExpanded: subtasksExpanded.value,
        filters,
      }),
    )
  } catch {
    /* storage full / disabled — non-fatal */
  }
}
// The view watcher fires on every search keystroke; a synchronous localStorage
// write per keystroke is a visible input-lag source on mid hardware. Debounce so
// we persist once the user pauses, and flush on unmount so nothing is lost.
let persistTimer = null
function persistView() {
  if (restoring) return
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
      if (v.layout) layout.value = v.layout
      if (v.groupMode) groupMode.value = v.groupMode
      if (typeof v.tagPrefix === 'string') tagPrefix.value = v.tagPrefix
      subtasksExpanded.value = !!v.subtasksExpanded
      if (Array.isArray(v.sortLevels)) sortLevels.value = v.sortLevels
      else if (v.sortBy && v.sortBy !== 'position')
        sortLevels.value = [{ field: v.sortBy, dir: v.sortDir || 'asc' }]
      if (v.filters) {
        filters.priorities = v.filters.priorities || []
        filters.assignees = v.filters.assignees || []
        filters.tags = v.filters.tags || []
        filters.due = v.filters.due || ''
        filters.q = v.filters.q || ''
      }
    } else {
      resetFilters()
    }
  } catch {
    /* corrupt entry — ignore */
  } finally {
    nextTick(() => (restoring = false))
  }
}
watch([layout, groupMode, tagPrefix, sortLevels, subtasksExpanded, filters], persistView, {
  deep: true,
})

// ── saved views (per-user, server-side; cross-device) ──
const savedViews = ref([])
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
  Object.assign(filters, { priorities: [], assignees: [], tags: [], due: '', q: '' }, c.filters || {})
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
  applyViewConfig(v.config)
  currentViewName.value = v.name
  showLoadView.value = false
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
const COL_MIN = 220 // narrowest a column may get before we switch to scrolling
const COL_MAX = 420
const colWidth = computed(() => {
  const cw = containerWidth.value
  if (!cw) return 280 // pre-measure fallback
  // Mobile: one column a little under full width so the next one peeks and the
  // swipe (scroll-snap, see CSS) reads as a smooth page-turn.
  if (isMobile.value) return Math.max(cw - 28, 200)
  const n = displayColumns.value.length || 1
  // Reserve the "+ колонка" tile (status mode) + the gaps, plus a few px slack
  // so sub-pixel rounding can't trip the horizontal scrollbar.
  const reserved = (groupMode.value === 'status' ? ADD_COL_W + GAP : 0) + (n - 1) * GAP + 4
  const w = Math.floor((cw - reserved) / n)
  // Fill the viewport while comfortable; once columns would get narrower than
  // COL_MIN, stop shrinking and let the board scroll horizontally instead.
  return Math.min(Math.max(w, COL_MIN), COL_MAX)
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
// …and the board archive modal (shared via the store, rendered in two menus).
const archiveOpen = computed(() => boardViewStore.archiveOpen)
useOverlayBack(archiveOpen, () => (boardViewStore.archiveOpen = false))

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
    const [b, c, t, s] = await Promise.all([
      boards.get(id),
      boards.columns(id),
      boards.tasks(id),
      boards.subtasks(id),
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
  const [tg, mem, pfx] = await Promise.all([
    projectsApi.tags(projectId),
    wsApi.members(wsId),
    projectsApi.tagPrefixes(projectId).catch(() => ({ data: [] })),
  ])
  for (const k of Object.keys(tagsMap)) delete tagsMap[k]
  for (const t of tg.data || []) tagsMap[t.id] = t
  for (const k of Object.keys(membersMap)) delete membersMap[k]
  for (const m of mem.data || []) membersMap[m.user_id] = m
  for (const k of Object.keys(tagPrefixNames)) delete tagPrefixNames[k]
  for (const p of pfx.data || []) tagPrefixNames[p.prefix] = p.label
  // Mirror tags + prefix names + context to the store so the header Теги manager works.
  boardViewStore.setTags(tagsList.value)
  boardViewStore.setPrefixNames({ ...tagPrefixNames })
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
    arr = arr.filter((t) => (t.assignee_ids || []).some((a) => filters.assignees.includes(a)))
  if (filters.tags.length)
    arr = arr.filter((t) => (t.tag_ids || []).some((id) => filters.tags.includes(id)))
  if (filters.due) arr = arr.filter((t) => matchesDue(t, filters.due))
  const q = filters.q.trim().toLowerCase()
  if (q)
    arr = arr.filter(
      (t) => t.title.toLowerCase().includes(q) || (t.number != null && `#${t.number}`.includes(q)),
    )

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
  return [
    ...groupTags.value.map((t) => ({ key: t.id, name: t.name, color: t.color, tag: t })),
    { key: '__none__', name: 'Без тегов', color: '', tag: null },
  ]
})

function rebuildLists() {
  const map = {}
  if (groupMode.value === 'status') {
    for (const col of columns.value) map[col.id] = []
    for (const t of filteredTasks.value) (map[t.column_id] ||= []).push(t)
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
watch([filteredTasks, groupMode, tagPrefix], rebuildLists)

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
        const before = arr[info.newIndex - 1]
        const after = arr[info.newIndex + 1]
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
  if (dragging.value || Date.now() < suppressReloadUntil) return
  scheduleReload()
})

// Header-hosted actions (Теги manager, Архив) ask the board to reload.
watch(
  () => boardViewStore.reloadNonce,
  () => onChanged(),
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
  document.addEventListener('pointerdown', onDocPointerDown, true)
  restoreView()
  await load(props.boardId)
  loadViews()
  applyTaskQuery()
})
onBeforeUnmount(() => {
  ro?.disconnect()
  document.removeEventListener('pointerdown', onDocPointerDown, true)
  onDragEnd()
  boardViewStore.reset()
})
watch(
  () => props.boardId,
  async (id) => {
    if (!id) return
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
</script>

<template>
  <n-spin :show="loading" :rotate="false" class="board-spin">
    <template #icon><TesseraSpinner /></template>
    <div v-if="board" class="board-wrap" :class="{ 'has-bg': !!themeStore.boardBackground }" :style="boardBgStyle">
      <!-- Sub-toolbar under the header: grouping / sort / filters / subtasks +
           a task-name search on the right. (Layout + Теги/Архив live in the
           global header now.) -->
      <div ref="subbarEl" class="subbar">
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
        <n-tooltip>
          <template #trigger>
            <n-button
              size="small"
              quaternary
              class="ngrad bar-btn"
              :type="subtasksExpanded ? 'primary' : 'default'"
              @click="subtasksExpanded = !subtasksExpanded"
            >
              <template #icon><n-icon :component="GitBranchOutline" /></template>
            </n-button>
          </template>
          {{ subtasksExpanded ? 'Свернуть подзадачи' : 'Развернуть подзадачи' }}
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
            <div v-for="v in savedViews" :key="v.id" class="view-row">
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
            <n-text v-if="!savedViews.length" depth="3" class="views-empty">
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
            <div v-if="savedViews.length" class="views-over">
                <n-text depth="3" class="views-lbl">Перезаписать:</n-text>
                <button
                v-for="v in savedViews"
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

      <BoardMatrixView
        v-else-if="layout === 'matrix'"
        :tasks="filteredTasks"
        :subtasks-by-parent="subtasksByParent"
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
          :disabled="groupMode !== 'status'"
          class="cols"
          :animation="150"
          :delay="160"
          :delay-on-touch-only="true"
          :touch-start-threshold="6"
          @start="onDragStart"
          @end="onDragEnd"
          @change="onColumnReorder"
        >
          <template #item="{ element: dcol }">
            <div class="col" :style="{ '--col-accent': dcol.color || 'var(--t-primary)' }">
              <ColumnHeader
                :dcol="dcol"
                :count="(lists[dcol.key] || []).length"
                :editable="groupMode === 'status'"
                :is-done="groupMode === 'status' && dcol.key === doneColumnId"
                :first="groupMode === 'status' && dcol.key === (colModel[0] && colModel[0].key)"
                @changed="onChanged"
                @set-done="onSetDone"
              />
              <draggable
                :list="lists[dcol.key]"
                group="tasks"
                item-key="id"
                class="drop"
                ghost-class="ghost"
                filter=".add-sub, .sub-add-input"
                :prevent-on-filter="false"
                :animation="150"
                :delay="160"
                :delay-on-touch-only="true"
                :touch-start-threshold="6"
                @start="onDragStart"
                @end="onDragEnd"
                @change="onColChange($event, dcol)"
              >
                <template #item="{ element }">
                  <div>
                    <TaskCard
                      :task="element"
                      :subtasks="subtasksByParent[element.id] || []"
                      :subtasks-expanded="subtasksExpanded"
                      :dragging="dragging"
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
                    />
                  </div>
                </template>
              </draggable>

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
      @update:show="(v) => v || closeTask()"
      @changed="onChanged"
      @open="openTask"
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
  padding: 2px 4px 2px 9px;
  border-radius: 999px;
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
  /* Top accent: a 3px gradient TOP border that wraps the rounded top corners
     onto the side edges (transparent top border reveals the gradient painted on
     the border-box; padding-box keeps the column fill flat). */
  border: 0 solid transparent;
  border-top-width: 3px;
  background:
    linear-gradient(var(--t-surface-alt), var(--t-surface-alt)) padding-box,
    var(--col-grad) border-box;
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
