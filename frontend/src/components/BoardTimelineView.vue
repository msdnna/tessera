<script setup>
import { ref, computed, onBeforeUnmount, onMounted, nextTick, watch } from 'vue'
import { NDropdown, NPopconfirm, NIcon } from 'naive-ui'
import { TimerOutline, ChevronBackOutline, ChevronForwardOutline } from '@vicons/ionicons5'
import { useTaskMenu } from '@/composables/useTaskMenu'
import { useDateLocale } from '@/composables/useDateLocale'
import { useThemeStore } from '@/stores/theme'
import { tasks as tasksApi } from '@/api'
import { PRIORITY_COLORS } from '@/styles/tokens'
import { hueGrad, readableHue } from '@/utils/gradient'
import { useWorkspacesStore } from '@/stores/workspaces'
import { formatEstimate, formatEstimateFull, estimateDateRange, sumEstimates, estimateToDays } from '@/utils/estimation'
import {
  DAY_MS,
  HOUR_MS,
  startOfDay,
  isAllDayMs,
  tierFor,
  barSpan,
  anchorMs,
  xAt,
  buildDays,
  buildMonthBands,
  buildWeekBands,
  hourTicksInWindow,
  hourStepFor,
} from '@/utils/timeAxis'
import UserAvatar from './UserAvatar.vue'

const wsStore = useWorkspacesStore()

const theme = useThemeStore()
// Tag colours can be very light; on the preview card's surface they vanish unless
// we pull them to a readable lightness for the current theme (mirrors the board).
function readableTag(hex) {
  return readableHue(hex || '#888', theme.isDark)
}

const props = defineProps({
  tasks: { type: Array, default: () => [] },
  // Real board status columns [{ id, name }] for the context menu + grouping.
  statusColumns: { type: Array, default: () => [] },
  membersMap: { type: Object, default: () => ({}) },
  tagsMap: { type: Object, default: () => ({}) },
  // Swimlane grouping comes from the shared composer-bar (no duplicate control):
  // 'status' | 'tag' (+ tagPrefix) | 'assignee' | 'none'.
  groupMode: { type: String, default: 'assignee' },
  tagPrefix: { type: String, default: '' },
  projectId: { type: String, default: null },
})
const emit = defineEmits(['open', 'changed'])

// Effort total per lane (sum of estimates), shown in the lane header. The unit
// comes from the project's estimation config.
const estCfg = computed(() => wsStore.estimationFor(props.projectId))
function laneEffort(lane) {
  const total = sumEstimates(lane.tasks)
  return total != null ? formatEstimate(total, estCfg.value) : ''
}

const menu = useTaskMenu({
  onOpen: (id) => emit('open', id),
  onChanged: () => emit('changed'),
  columns: () => props.statusColumns,
})

const LEFT_W = 224 // px of the fixed task/lane column (expanded)
// Collapsible left column: collapses to a true 0 (padding + border are zeroed in
// the .collapsed state) to give the chart full width. Toggle is instant —
// animating width across every sticky row thrashed layout; `collapsing` suppresses
// the today-line's left-transition for one frame so it snaps with the rest.
const leftCollapsed = ref(false)
const leftW = computed(() => (leftCollapsed.value ? 0 : LEFT_W))
const collapsing = ref(false)
let collapseSettle = 0
function toggleLeft() {
  collapsing.value = true
  clearTimeout(collapseSettle)
  collapseSettle = setTimeout(() => (collapsing.value = false), 60)
  leftCollapsed.value = !leftCollapsed.value
}

// ── zoom (px per day) ──
// The range spans week-grouping (zoomed out) through hour-precision (zoomed in);
// `tierFor(dayW)` picks the axis granularity. Default sits in the days tier.
// Stops land in each tier: weeks ≤18, days 24–112, hours ≥140 (≥3 hour ticks).
const ZOOM = [6, 10, 14, 18, 24, 32, 44, 60, 84, 112, 140, 180, 230]
const zoomIdx = ref(5) // → 32px/day (days tier)
const dayW = computed(() => ZOOM[zoomIdx.value])
// While the user is actively zooming (a burst of wheel ticks), bars/today/ghost
// SNAP instead of CSS-transitioning — animating every intermediate step made the
// chart jitter on multi-step zoom. `zooming` gates the `.animate` class; a short
// debounce re-enables transitions once the scale has settled.
const zooming = ref(false)
let zoomSettle = 0
// Re-zoom while keeping the day under `anchorClientX` (a viewport x; omit → centre
// of the viewport) pinned in place. We read the day at the anchor BEFORE the width
// changes, then on the next tick (once axisW has reflowed) set scrollLeft so that
// same day lands back under the anchor — no more snapping to the earliest task.
function applyZoom(newIdx, anchorClientX) {
  newIdx = Math.max(0, Math.min(ZOOM.length - 1, newIdx))
  if (newIdx === zoomIdx.value) return
  const el = scrollEl.value
  const oldW = ZOOM[zoomIdx.value]
  const newW = ZOOM[newIdx]
  let anchorX = null
  let dayAtAnchor = null
  if (el) {
    const rect = el.getBoundingClientRect()
    anchorX = anchorClientX != null ? anchorClientX - rect.left : el.clientWidth / 2
    dayAtAnchor = (el.scrollLeft + anchorX - leftW.value) / oldW
  }
  zooming.value = true
  clearTimeout(zoomSettle)
  zoomSettle = setTimeout(() => (zooming.value = false), 200)
  zoomIdx.value = newIdx
  if (el && dayAtAnchor != null) {
    nextTick(() => {
      el.scrollLeft = Math.max(0, dayAtAnchor * newW + leftW.value - anchorX)
    })
  }
}
function zoomIn(anchorX) {
  applyZoom(zoomIdx.value + 1, anchorX)
}
function zoomOut(anchorX) {
  applyZoom(zoomIdx.value - 1, anchorX)
}

// ── date helpers (pure axis math lives in utils/timeAxis) ──
const todayMs = startOfDay(Date.now())
const parse = (s) => (s ? Date.parse(s) : null)
// Current axis granularity tier: 'weeks' | 'days' | 'hours'.
const tier = computed(() => tierFor(dayW.value))
// Horizontal scroll position + viewport width, rAF-throttled — drives the windowed
// hour-tick rendering (only paints the visible slice).
const scrollX = ref(0)
const viewW = ref(0)
// Vertical scroll + viewport height + the body's y-offset within the scroll content
// (header height), rAF-throttled — drives row virtualization below.
const scrollY = ref(0)
const viewH = ref(0)
const bodyTop = ref(0)
const bodyEl = ref(null)

// A task is "scheduled" when it has at least one of start/due.
const scheduled = computed(() => props.tasks.filter((t) => t.start_date || t.due_date))
const unscheduled = computed(() => props.tasks.filter((t) => !t.start_date && !t.due_date))

// Effective span endpoints of a task (a one-ended task is a 1-day bar).
function spanOf(t) {
  const s = parse(t.start_date)
  const d = parse(t.due_date)
  const a = s ?? d
  const b = d ?? s
  return { a: startOfDay(a), b: startOfDay(b), hasStart: s != null, hasDue: d != null }
}

// ── axis range: covers every scheduled task (incl. its estimate ghost) + today ──
const range = computed(() => {
  let lo = todayMs
  let hi = todayMs
  for (const t of scheduled.value) {
    const { a, b } = spanOf(t)
    lo = Math.min(lo, a)
    hi = Math.max(hi, b)
    // The estimate "ghost" can reach past the due date — keep its end on-scale.
    const gd = estimateToDays(t.estimate, estCfg.value)
    if (gd != null) hi = Math.max(hi, a + Math.ceil(gd) * DAY_MS)
  }
  lo -= 3 * DAY_MS
  hi += 7 * DAY_MS
  const days = Math.round((hi - lo) / DAY_MS) + 1
  return { start: startOfDay(lo), days }
})
const axisW = computed(() => range.value.days * dayW.value)
const dayIndex = (ms) => Math.round((startOfDay(ms) - range.value.start) / DAY_MS)

// Axis header bands. Months always show; the second band is per-day (days/hours
// tiers) or per-week (weeks tier); the hours tier adds an hour-tick row.
const days = computed(() => buildDays(range.value.start, range.value.days, todayMs))
const monthBands = computed(() => buildMonthBands(days.value))
const weekBands = computed(() => (tier.value === 'weeks' ? buildWeekBands(days.value) : []))
// Hour ticks only for the visible viewport slice (+ a margin) — "lazy lines" so a
// wide board doesn't render thousands of header nodes in the hours tier.
const hourTicks = computed(() => {
  if (tier.value !== 'hours') return []
  const lo = scrollX.value - leftW.value - 200
  const hi = scrollX.value - leftW.value + viewW.value + 200
  return hourTicksInWindow(range.value.days, dayW.value, lo, hi)
})
// Minor-gridline period (px): the hour-tick step in the hours tier, else one day.
const subW = computed(() => {
  const step = hourStepFor(dayW.value)
  return tier.value === 'hours' && step ? (step / 24) * dayW.value : dayW.value
})

// ── lanes (swimlanes) — driven by the composer-bar's groupMode ──
const lanes = computed(() => {
  const mode = props.groupMode
  const buckets = new Map()
  const ensure = (key, label, color) => {
    if (!buckets.has(key)) buckets.set(key, { key, label, color, tasks: [] })
    return buckets.get(key)
  }
  // For 'status' grouping, seed lanes in column order so empty columns still show
  // and the lane order matches the board.
  if (mode === 'status') {
    for (const col of props.statusColumns) ensure(col.id, col.name, col.color)
  }
  for (const t of scheduled.value) {
    if (mode === 'assignee') {
      const id = (t.assignee_ids || [])[0]
      const m = id ? props.membersMap[id] : null
      ensure(id || '∅', m?.name || 'Не назначено').tasks.push(t)
    } else if (mode === 'tag') {
      // Respect the prefix when grouping by a tag namespace.
      const ids = (t.tag_ids || []).filter((id) => {
        const tag = props.tagsMap[id]
        return tag && (!props.tagPrefix || (tag.name || '').startsWith(props.tagPrefix))
      })
      const id = ids[0]
      const tag = id ? props.tagsMap[id] : null
      ensure(id || '∅', tag?.name || 'Без тега', tag?.color).tasks.push(t)
    } else if (mode === 'status') {
      const col = props.statusColumns.find((c) => c.id === t.column_id)
      ensure(t.column_id || '∅', col?.name || '—', col?.color).tasks.push(t)
    } else {
      ensure('all', 'Все задачи').tasks.push(t)
    }
  }
  const arr = [...buckets.values()].filter((l) => l.tasks.length || mode === 'status')
  // Lane tasks keep the incoming `props.tasks` order — which already reflects the
  // composer's sort (e.g. «Сорт: Статус»); re-sorting by start here would override it.
  if (mode !== 'status') arr.sort((a, b) => (a.key === '∅' ? 1 : 0) - (b.key === '∅' ? 1 : 0))
  return arr
})

const overdueCount = computed(
  () => scheduled.value.filter((t) => t.due_date && !t.completed_at && startOfDay(parse(t.due_date)) < todayMs).length,
)

// ── row virtualization ──
// The body renders one DOM row per lane-header and per task; on a board with 200+
// tasks that (with per-row gridlines + bars) dominates the cost of every zoom and
// scroll. We window it: only rows intersecting the viewport (plus a margin) render,
// with top/bottom spacers that preserve the total scroll height. Bars are positioned
// by pure axis math, so windowing changes nothing about their geometry.
const ROW_H = 36
const laneH = ref(30) // lane-header row height; measured once (styling is uniform)
function measureLaneH() {
  const h = bodyEl.value?.querySelector('.tl-lanehead')?.offsetHeight
  if (h) laneH.value = h
}
// Flat ordered list of visual rows (lane header, then its task rows), matching the
// document flow so spacer heights stay exact.
const flatRows = computed(() => {
  const out = []
  for (const lane of lanes.value) {
    out.push({ t: 'lane', key: `L${lane.key}`, lane })
    for (const task of lane.tasks) out.push({ t: 'task', key: task.id, task })
  }
  return out
})
const rowH = (r) => (r.t === 'lane' ? laneH.value : ROW_H)
const rowLayout = computed(() => {
  const rows = flatRows.value
  const tops = new Array(rows.length)
  let y = 0
  for (let i = 0; i < rows.length; i++) {
    tops[i] = y
    y += rowH(rows[i])
  }
  return { tops, height: y }
})
const VMARGIN = 600 // px of off-screen rows kept rendered above/below the viewport
const vwindow = computed(() => {
  const rows = flatRows.value
  const { tops, height } = rowLayout.value
  const n = rows.length
  if (!n) return { rows: [], top: 0, bottom: 0 }
  const lo = scrollY.value - bodyTop.value - VMARGIN
  const hi = scrollY.value - bodyTop.value + (viewH.value || 800) + VMARGIN
  let start = 0
  while (start < n && tops[start] + rowH(rows[start]) < lo) start++
  if (start >= n) return { rows: [], top: height, bottom: 0 }
  let end = start
  while (end < n && tops[end] < hi) end++
  if (end <= start) end = Math.min(n, start + 1)
  const last = end - 1
  return { rows: rows.slice(start, end), top: tops[start], bottom: height - (tops[last] + rowH(rows[last])) }
})
onMounted(() => nextTick(measureLaneH))
watch(lanes, () => nextTick(measureLaneH))

// ── drag-to-reschedule (move whole bar / resize an edge) ──
// During a drag we hold a transient preview so only the dragged bar re-renders.
const drag = ref(null) // { id, mode, startX, baseStart, baseDue, hasStart, hasDue }
const preview = ref(null) // { id, start, due }

function onBarDown(e, t, mode) {
  if (e.button != null && e.button !== 0) return
  e.preventDefault()
  e.stopPropagation()
  const s = parse(t.start_date)
  const d = parse(t.due_date)
  drag.value = {
    id: t.id,
    mode,
    startX: e.clientX,
    baseStart: s,
    baseDue: d,
    hasStart: s != null,
    hasDue: d != null,
  }
  preview.value = { id: t.id, start: s, due: d }
  window.addEventListener('pointermove', onMove)
  window.addEventListener('pointerup', onUp)
}
function onMove(e) {
  const g = drag.value
  if (!g) return
  // Snap to the hour when zoomed into the hours tier AND the edited endpoint is a
  // timed value; an all-day (UTC-midnight) endpoint keeps day snapping so it stays
  // all-day. Day snapping everywhere else (current behaviour).
  const base = g.mode === 'due' ? g.baseDue ?? g.baseStart : g.baseStart ?? g.baseDue
  const hourSnap = tier.value === 'hours' && base != null && !isAllDayMs(base)
  const unit = hourSnap ? HOUR_MS : DAY_MS
  const unitPx = hourSnap ? dayW.value / 24 : dayW.value
  const delta = Math.round((e.clientX - g.startX) / unitPx)
  if (delta === 0) {
    preview.value = { id: g.id, start: g.baseStart, due: g.baseDue }
    return
  }
  const shift = delta * unit
  let start = g.baseStart
  let due = g.baseDue
  if (g.mode === 'move') {
    // Move both ends; a one-ended task moves just that end.
    if (g.hasStart) start = g.baseStart + shift
    if (g.hasDue) due = g.baseDue + shift
  } else if (g.mode === 'start') {
    start = (g.baseStart ?? g.baseDue) + shift
    if (due != null && start > due) start = due
  } else if (g.mode === 'due') {
    due = (g.baseDue ?? g.baseStart) + shift
    if (start != null && due < start) due = start
  }
  preview.value = { id: g.id, start, due }
}
async function onUp() {
  const g = drag.value
  const p = preview.value
  window.removeEventListener('pointermove', onMove)
  window.removeEventListener('pointerup', onUp)
  drag.value = null
  if (!g || !p) {
    preview.value = null
    return
  }
  const changed = p.start !== g.baseStart || p.due !== g.baseDue
  preview.value = null
  if (!changed) return
  const t = props.tasks.find((x) => x.id === g.id)
  if (!t) return
  try {
    await tasksApi.update(t.id, {
      title: t.title,
      description: t.description || '',
      priority: t.priority || 0,
      due_date: p.due != null ? new Date(p.due).toISOString() : null,
      start_date: p.start != null ? new Date(p.start).toISOString() : null,
      recurrence: t.recurrence || null,
      completed: !!t.completed_at,
    })
    emit('changed')
  } catch {
    emit('changed')
  }
}
onBeforeUnmount(() => {
  window.removeEventListener('pointermove', onMove)
  window.removeEventListener('pointerup', onUp)
})

// Bar geometry for a task, honouring an active drag preview. In the hours tier a
// timed start/due sits at its real clock time (see barSpan).
function geom(t) {
  let s = parse(t.start_date)
  let d = parse(t.due_date)
  if (preview.value && preview.value.id === t.id) {
    s = preview.value.start
    d = preview.value.due
  }
  return barSpan({ start: s, due: d, tier: tier.value, rangeStart: range.value.start, dayW: dayW.value })
}

// Ghost "estimate" envelope: a dashed bar starting at the task's span start and
// extending for as many calendar days as the estimate implies — so you can see at
// a glance whether the planned start→due window matches the effort estimate.
// Only meaningful for the time unit (estimateToDays returns null otherwise).
function ghostGeom(t) {
  const days = estimateToDays(t.estimate, estCfg.value)
  if (days == null) return null
  let s = parse(t.start_date)
  let d = parse(t.due_date)
  if (preview.value && preview.value.id === t.id) {
    s = preview.value.start
    d = preview.value.due
  }
  if (s == null && d == null) return null
  const anchor = anchorMs(s ?? d, tier.value)
  // Frame the envelope with a 3px margin on the start/end too (matching the top/bottom inset).
  return { left: xAt(anchor, range.value.start, dayW.value) - 3, width: Math.max(dayW.value, days * dayW.value) + 6 }
}
// Today-line x: the real current time in the hours tier (sub-day precision), else
// the centre of today's day cell.
const todayLeft = computed(() =>
  tier.value === 'hours' ? xAt(Date.now(), range.value.start, dayW.value) : dayIndex(todayMs) * dayW.value + dayW.value / 2,
)

// ── scroll-to-today ──
// rAF-driven so the glide is reliable: native `scrollTo({behavior:'smooth'})` on
// this overflow container was a no-op in practice (jumped instantly).
const scrollEl = ref(null)
let scrollRaf = 0
function animateScrollLeft(target) {
  const el = scrollEl.value
  if (!el) return
  cancelAnimationFrame(scrollRaf)
  const from = el.scrollLeft
  const dist = target - from
  if (Math.abs(dist) < 1) {
    el.scrollLeft = target
    return
  }
  const t0 = performance.now()
  const dur = 340
  const ease = (p) => 1 - Math.pow(1 - p, 3)
  const step = (now) => {
    const p = Math.min(1, (now - t0) / dur)
    el.scrollLeft = from + dist * ease(p)
    if (p < 1) scrollRaf = requestAnimationFrame(step)
  }
  scrollRaf = requestAnimationFrame(step)
}
function centerToday(smooth = true) {
  const el = scrollEl.value
  if (!el) return
  const left = Math.max(0, dayIndex(todayMs) * dayW.value - el.clientWidth / 2 + leftW.value)
  if (smooth === false) el.scrollLeft = left
  else animateScrollLeft(left)
}
watch(scrollEl, (el) => el && nextTick(() => { centerToday(false); syncScroll() }))

// rAF-throttled scroll/resize sync for the windowed hour ticks.
let scrollRaf2 = 0
function syncScroll() {
  const el = scrollEl.value
  if (!el) return
  scrollX.value = el.scrollLeft
  viewW.value = el.clientWidth
  scrollY.value = el.scrollTop
  viewH.value = el.clientHeight
  if (bodyEl.value) bodyTop.value = bodyEl.value.offsetTop
}
function onScroll() {
  if (scrollRaf2) return
  scrollRaf2 = requestAnimationFrame(() => {
    scrollRaf2 = 0
    syncScroll()
  })
}

// ── pan: middle-button anywhere, or left-drag on empty timeline space ──
// (bars stop propagation on their own pointerdown, so a pointerdown that reaches
// the scroll container is on empty space.)
const pan = ref(null)
function onPanDown(e) {
  const middle = e.button === 1
  const emptyLeft = e.button === 0 && e.target.closest('.tl-track') && !e.target.closest('.bar')
  if (!middle && !emptyLeft) return
  const el = scrollEl.value
  if (!el) return
  e.preventDefault()
  pan.value = { x: e.clientX, y: e.clientY, sl: el.scrollLeft, st: el.scrollTop }
  window.addEventListener('pointermove', onPanMove)
  window.addEventListener('pointerup', onPanUp)
}
function onPanMove(e) {
  const p = pan.value
  const el = scrollEl.value
  if (!p || !el) return
  el.scrollLeft = p.sl - (e.clientX - p.x)
  el.scrollTop = p.st - (e.clientY - p.y)
}
function onPanUp() {
  pan.value = null
  window.removeEventListener('pointermove', onPanMove)
  window.removeEventListener('pointerup', onPanUp)
}
// Ctrl/Cmd + wheel zooms, anchored on the day under the cursor.
function onWheel(e) {
  if (!(e.ctrlKey || e.metaKey)) return
  e.preventDefault()
  if (e.deltaY < 0) zoomIn(e.clientX)
  else zoomOut(e.clientX)
}
onBeforeUnmount(() => {
  cancelAnimationFrame(scrollRaf)
  cancelAnimationFrame(scrollRaf2)
  clearTimeout(zoomSettle)
  clearTimeout(collapseSettle)
  window.removeEventListener('pointermove', onPanMove)
  window.removeEventListener('pointerup', onPanUp)
})

// ── hover preview card ──
const { formatDue } = useDateLocale()
const hover = ref(null) // { task, x, y, above }
let hoverTimer = null
function onBarEnter(e, t) {
  clearTimeout(hoverTimer)
  const r = e.currentTarget.getBoundingClientRect()
  const below = r.bottom + 8
  const above = window.innerHeight - r.top + 8
  const useAbove = window.innerHeight - r.bottom < 200
  hover.value = {
    task: t,
    x: Math.min(Math.max(r.left, 12), window.innerWidth - 280),
    y: useAbove ? above : below,
    above: useAbove,
  }
}
function onBarLeave() {
  hoverTimer = setTimeout(() => (hover.value = null), 80)
}
const hoverTags = computed(() =>
  hover.value ? (hover.value.task.tag_ids || []).map((id) => props.tagsMap[id]).filter(Boolean) : [],
)
const hoverAssignees = computed(() =>
  hover.value ? (hover.value.task.assignee_ids || []).map((id) => props.membersMap[id]).filter(Boolean) : [],
)
// External GitLab assignees (no Tessera account) — shown by avatar URL.
const hoverGlAssignees = computed(() => (hover.value ? hover.value.task.gitlab_assignees || [] : []))
const hoverEstimate = computed(() =>
  hover.value && hover.value.task.estimate != null ? formatEstimateFull(hover.value.task.estimate, estCfg.value) : '',
)
const hoverEstimateRange = computed(() =>
  hover.value ? estimateDateRange(hover.value.task.start_date, hover.value.task.estimate, estCfg.value) : '',
)
// Tooltip on the ghost bar: full expansion + projected window.
function ghostTitle(t) {
  const full = formatEstimateFull(t.estimate, estCfg.value)
  const range = estimateDateRange(t.start_date, t.estimate, estCfg.value)
  return range ? `Оценка: ${full} (${range})` : `Оценка: ${full}`
}
</script>

<template>
  <div class="tl">
    <div class="tl-toolbar">
      <button class="tl-today-btn" type="button" @click="centerToday">Сегодня</button>
      <div class="tl-zoom">
        <button class="tl-zoom-btn" type="button" :disabled="zoomIdx === 0" title="Уменьшить масштаб" @click="zoomOut()">−</button>
        <button class="tl-zoom-btn" type="button" :disabled="zoomIdx === ZOOM.length - 1" title="Увеличить масштаб" @click="zoomIn()">+</button>
        <button
          class="tl-zoom-btn"
          type="button"
          :title="leftCollapsed ? 'Показать колонку задач' : 'Свернуть колонку задач'"
          @click="toggleLeft"
        >
          <n-icon :component="leftCollapsed ? ChevronForwardOutline : ChevronBackOutline" :size="15" />
        </button>
      </div>
      <div class="tl-counters">
        <span v-if="overdueCount" class="tl-counter overdue">{{ overdueCount }} просрочено</span>
        <span v-if="unscheduled.length" class="tl-counter">{{ unscheduled.length }} без дат</span>
      </div>
    </div>

    <div
      ref="scrollEl"
      class="tl-scroll"
      :class="{ panning: !!pan }"
      @pointerdown="onPanDown"
      @wheel="onWheel"
      @scroll="onScroll"
    >
      <div
        class="tl-inner"
        :class="[tier, { animate: !drag && !zooming && !collapsing, collapsed: leftCollapsed }]"
        :style="{ width: `${leftW + axisW}px`, '--tl-day-w': `${dayW}px`, '--tl-week-w': `${dayW * 7}px`, '--tl-sub-w': `${subW}px` }"
      >
        <!-- header: month band + day/week band (+ hour ticks in the hours tier) -->
        <div class="tl-head">
          <div class="tl-corner" :style="{ width: `${leftW}px` }">Задача</div>
          <div class="tl-axis">
            <div class="tl-months">
              <div
                v-for="(m, i) in monthBands"
                :key="i"
                class="tl-month"
                :style="{ width: `${m.span * dayW}px` }"
              >
                {{ m.label }}
              </div>
            </div>
            <div v-if="tier === 'weeks'" class="tl-weeksrow">
              <div v-for="w in weekBands" :key="w.key" class="tl-weekh" :style="{ width: `${w.span * dayW}px` }">
                {{ w.label }}
              </div>
            </div>
            <template v-else>
              <div class="tl-daysrow">
                <div
                  v-for="(d, i) in days"
                  :key="i"
                  class="tl-dayh"
                  :class="{ weekend: d.weekend, today: d.isToday }"
                  :style="{ width: `${dayW}px` }"
                >
                  <span class="dh-num">{{ d.day }}</span>
                  <span class="dh-wd">{{ d.dow }}</span>
                </div>
              </div>
              <div v-if="tier === 'hours'" class="tl-hoursrow">
                <span v-for="h in hourTicks" :key="h.key" class="tl-hourtick" :style="{ left: `${h.left}px` }">{{ h.label }}</span>
              </div>
            </template>
          </div>
        </div>

        <!-- swimlanes (windowed: only rows in/near the viewport render; spacers keep
             the scroll height; one continuous today-line spans the whole body) -->
        <div ref="bodyEl" class="tl-body">
          <div class="tl-today" :style="{ left: `${leftW + todayLeft}px` }" />
          <div class="tl-vspacer" :style="{ height: `${vwindow.top}px` }" />
          <template v-for="r in vwindow.rows" :key="r.key">
            <div v-if="r.t === 'lane'" class="tl-lanehead">
              <div class="tl-left lane" :style="{ width: `${leftW}px` }">
                <span
                  class="lane-dot"
                  :style="{ background: r.lane.color ? hueGrad(r.lane.color) : 'var(--t-accent-grad)' }"
                />
                <span class="lane-name">{{ r.lane.label }}</span>
                <span class="lane-count">{{ r.lane.tasks.length }}</span>
                <span v-if="laneEffort(r.lane)" class="lane-effort" title="Суммарная оценка"
                  ><n-icon :component="TimerOutline" :size="12" /> {{ laneEffort(r.lane) }}</span
                >
              </div>
              <div class="tl-track laneband" :style="{ width: `${axisW}px` }" />
            </div>

            <div v-else class="tl-row">
              <div class="tl-left" :style="{ width: `${leftW}px` }" :title="r.task.title" @click="$emit('open', r.task.id)">
                <span class="row-bar" :style="{ background: hueGrad(PRIORITY_COLORS[r.task.priority || 0]) }" />
                <span class="row-title" :class="{ done: r.task.completed_at }">{{ r.task.title }}</span>
              </div>
              <div class="tl-track" :style="{ width: `${axisW}px` }">
                <div
                  v-if="ghostGeom(r.task)"
                  class="ghost"
                  :style="{ left: `${ghostGeom(r.task).left}px`, width: `${ghostGeom(r.task).width}px`, '--ghost-c': PRIORITY_COLORS[r.task.priority || 0] }"
                  :title="ghostTitle(r.task)"
                >
                  <span class="ghost-est"
                    ><n-icon :component="TimerOutline" :size="11" /> {{ formatEstimate(r.task.estimate, estCfg) }}</span
                  >
                </div>
                <div
                  class="bar"
                  :class="{ done: r.task.completed_at, point: !(geom(r.task).hasStart && geom(r.task).hasDue) }"
                  :style="{
                    left: `${geom(r.task).left}px`,
                    width: `${geom(r.task).width}px`,
                    '--bar-grad': hueGrad(PRIORITY_COLORS[r.task.priority || 0]),
                  }"
                  @pointerdown="onBarDown($event, r.task, 'move')"
                  @click="$emit('open', r.task.id)"
                  @mouseenter="onBarEnter($event, r.task)"
                  @mouseleave="onBarLeave"
                  @contextmenu.prevent.stop="menu.open($event, r.task)"
                >
                  <span class="handle l" @pointerdown.stop="onBarDown($event, r.task, 'start')" />
                  <span class="bar-title">{{ r.task.title }}</span>
                  <span class="handle r" @pointerdown.stop="onBarDown($event, r.task, 'due')" />
                </div>
              </div>
            </div>
          </template>
          <div class="tl-vspacer" :style="{ height: `${vwindow.bottom}px` }" />
        </div>

        <div v-if="!lanes.length" class="tl-empty">Нет задач со сроками. Задайте срок или начало в карточке.</div>
      </div>
    </div>

    <!-- unscheduled -->
    <div v-if="unscheduled.length" class="tl-unsched">
      <span class="us-label">Без дат</span>
      <button
        v-for="t in unscheduled"
        :key="t.id"
        type="button"
        class="us-chip"
        :class="{ done: t.completed_at }"
        :style="{ '--chip': PRIORITY_COLORS[t.priority || 0] }"
        :title="t.title"
        @click="$emit('open', t.id)"
        @contextmenu.prevent.stop="menu.open($event, t)"
      >
        {{ t.title }}
      </button>
    </div>

    <n-dropdown
      trigger="manual"
      placement="bottom-start"
      :show="menu.show.value"
      :x="menu.x.value"
      :y="menu.y.value"
      :options="menu.options.value"
      @select="menu.select"
      @clickoutside="menu.show.value = false"
    />
    <n-popconfirm
      v-model:show="menu.deleteConfirmShow.value"
      :x="menu.x.value"
      :y="menu.y.value"
      :positive-button-props="{ type: 'error' }"
      positive-text="Удалить"
      @positive-click="menu.confirmDelete()"
      @clickoutside="menu.deleteConfirmShow.value = false"
    >
      <template #trigger><span /></template>
      Удалить безвозвратно? Это действие необратимо.
    </n-popconfirm>
    <n-popconfirm
      v-model:show="menu.archiveConfirmShow.value"
      :x="menu.x.value"
      :y="menu.y.value"
      positive-text="В архив"
      @positive-click="menu.confirmArchive()"
      @clickoutside="menu.archiveConfirmShow.value = false"
    >
      <template #trigger><span /></template>
      Перенести задачу в архив?
    </n-popconfirm>

    <!-- hover preview: a compact task-card snapshot -->
    <Teleport to="body">
      <div
        v-if="hover"
        class="tl-preview"
        :style="hover.above ? { left: `${hover.x}px`, bottom: `${hover.y}px` } : { left: `${hover.x}px`, top: `${hover.y}px` }"
      >
        <div class="pv-head">
          <span
            class="pv-flag"
            :style="{ background: hueGrad(PRIORITY_COLORS[hover.task.priority || 0]) }"
          />
          <span class="pv-title" :class="{ done: hover.task.completed_at }">{{ hover.task.title }}</span>
          <span v-if="hover.task.number != null" class="pv-num">#{{ hover.task.number }}</span>
        </div>
        <div v-if="hover.task.start_date || hover.task.due_date" class="pv-dates">
          <span v-if="hover.task.start_date">{{ formatDue(hover.task.start_date) }}</span>
          <span v-if="hover.task.start_date && hover.task.due_date" class="pv-arrow">→</span>
          <span v-if="hover.task.due_date">{{ formatDue(hover.task.due_date) }}</span>
        </div>
        <div v-if="hoverEstimate" class="pv-est">
          <n-icon :component="TimerOutline" :size="13" class="pv-est-ic" />
          <span>Оценка: {{ hoverEstimate }}<template v-if="hoverEstimateRange"> ({{ hoverEstimateRange }})</template></span>
        </div>
        <div v-if="hoverTags.length" class="pv-tags">
          <span
            v-for="tg in hoverTags"
            :key="tg.id"
            class="pv-tag"
            :style="{ color: readableTag(tg.color), borderColor: readableTag(tg.color) }"
          >{{ tg.name }}</span>
        </div>
        <div v-if="hoverAssignees.length || hoverGlAssignees.length" class="pv-assignees">
          <UserAvatar
            v-for="m in hoverAssignees"
            :key="m.user_id"
            class="pv-av"
            :user-id="m.user_id"
            :name="m.name"
            :title="m.name"
          />
          <UserAvatar
            v-for="(g, i) in hoverGlAssignees"
            :key="`g${i}`"
            class="pv-av"
            :src="g.avatar_url"
            :name="g.name || g"
            :title="g.name || g"
          />
        </div>
      </div>
    </Teleport>
  </div>
</template>

<style scoped>
.tl {
  padding: 2px 0 40px;
}
.tl-toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 10px;
}
.tl-today-btn {
  border: 1px solid var(--t-border);
  background: var(--t-surface);
  color: var(--t-text2);
  border-radius: 7px;
  padding: 5px 12px;
  font-size: 13px;
  cursor: pointer;
}
.tl-today-btn:hover {
  background: var(--t-hover);
}
.tl-zoom {
  display: flex;
  gap: 4px;
}
.tl-zoom-btn {
  width: 28px;
  height: 28px;
  display: grid;
  place-items: center;
  border: 1px solid var(--t-border);
  background: var(--t-surface);
  color: var(--t-text2);
  border-radius: 7px;
  font-size: 17px;
  line-height: 1;
  cursor: pointer;
}
.tl-zoom-btn:hover:not(:disabled) {
  background: var(--t-hover);
}
.tl-zoom-btn:disabled {
  opacity: 0.4;
  cursor: default;
}
.tl-counters {
  display: flex;
  gap: 8px;
  margin-left: auto;
}
.tl-counter {
  font-size: 12px;
  color: var(--t-text3);
  background: var(--t-hover);
  border-radius: 20px;
  padding: 3px 10px;
}
.tl-counter.overdue {
  color: #e0533d;
  background: color-mix(in srgb, #e0533d 12%, transparent);
}

.tl-scroll {
  overflow: auto;
  border: 1px solid var(--t-border);
  border-radius: 10px;
  max-height: calc(100vh - 220px);
}
.tl-scroll.panning {
  cursor: grabbing;
  user-select: none;
}
.tl-inner {
  position: relative;
}

/* header */
.tl-head {
  display: flex;
  position: sticky;
  top: 0;
  z-index: 6;
}
.tl-corner {
  box-sizing: border-box;
  position: sticky;
  left: 0;
  z-index: 7;
  flex: 0 0 auto;
  background: var(--t-surface-alt, var(--t-hover));
  border-right: 1px solid var(--t-border);
  border-bottom: 1px solid var(--t-border);
  display: flex;
  align-items: flex-end;
  padding: 6px 12px;
  font-size: 12px;
  font-weight: 600;
  color: var(--t-text3);
}
.tl-axis {
  flex: 0 0 auto;
}
.tl-months {
  display: flex;
  height: 22px;
  border-bottom: 1px solid var(--t-border);
}
.tl-month {
  box-sizing: border-box;
  flex: 0 0 auto;
  font-size: 12px;
  font-weight: 600;
  color: var(--t-text2);
  padding: 3px 8px;
  white-space: nowrap;
  overflow: hidden;
  border-right: 1px solid var(--t-border);
  background: var(--t-surface-alt, var(--t-hover));
  text-transform: capitalize;
}
.tl-daysrow {
  display: flex;
  background: var(--t-surface);
  border-bottom: 1px solid var(--t-border);
}
.tl-dayh {
  /* border-box so a cell's total width == the inline `dayW`, keeping the day
     headers aligned with the bar/gridline/today-line geometry (which use dayW). */
  box-sizing: border-box;
  flex: 0 0 auto;
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 3px 0;
  font-size: 11px;
  color: var(--t-text3);
  border-right: 1px solid color-mix(in srgb, var(--t-border) 55%, transparent);
}
.tl-dayh.weekend {
  background: var(--t-hover);
}
.tl-dayh.today .dh-num {
  background: var(--t-accent-grad);
  color: #fff;
  border-radius: 50%;
  width: 18px;
  height: 18px;
  line-height: 18px;
  text-align: center;
}
.dh-num {
  font-size: 12px;
  color: var(--t-text1);
}
.dh-wd {
  font-size: 9px;
}

/* weeks tier: one cell per week, labelled by its first date */
.tl-weeksrow {
  display: flex;
  background: var(--t-surface);
  border-bottom: 1px solid var(--t-border);
}
.tl-weekh {
  box-sizing: border-box;
  flex: 0 0 auto;
  padding: 4px 6px;
  font-size: 11px;
  color: var(--t-text2);
  white-space: nowrap;
  overflow: hidden;
  border-right: 1px solid color-mix(in srgb, var(--t-border) 55%, transparent);
}
/* hours tier: thin row of hour labels positioned at their fraction of the day */
.tl-hoursrow {
  position: relative;
  height: 14px;
  background: var(--t-surface);
  border-bottom: 1px solid var(--t-border);
}
.tl-hourtick {
  position: absolute;
  top: 0;
  transform: translateX(-50%);
  font-size: 9px;
  line-height: 14px;
  color: var(--t-text3);
  pointer-events: none;
}

/* lanes */
.tl-lanehead {
  display: flex;
}
.tl-left.lane {
  box-sizing: border-box;
  position: sticky;
  left: 0;
  z-index: 5;
  display: flex;
  align-items: center;
  gap: 7px;
  background: var(--t-surface-alt, var(--t-hover));
  border-right: 1px solid var(--t-border);
  border-bottom: 1px solid var(--t-border);
  padding: 5px 12px;
}
.lane-dot {
  width: 9px;
  height: 9px;
  border-radius: 3px;
  flex: 0 0 auto;
}
.lane-name {
  font-size: 12px;
  font-weight: 600;
  color: var(--t-text2);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.lane-count {
  font-size: 11px;
  color: var(--t-text3);
  background: var(--t-surface);
  border-radius: 20px;
  padding: 0 7px;
  margin-left: auto;
}
.lane-effort {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  font-size: 11px;
  color: var(--t-text2);
  white-space: nowrap;
}
.laneband {
  position: relative;
  background: var(--t-surface-alt, var(--t-hover));
  border-bottom: 1px solid var(--t-border);
  flex: 0 0 auto;
}

.tl-row {
  display: flex;
}
.tl-left {
  box-sizing: border-box;
  position: sticky;
  left: 0;
  z-index: 4;
  flex: 0 0 auto;
  display: flex;
  align-items: center;
  gap: 8px;
  background: var(--t-surface);
  border-right: 1px solid var(--t-border);
  border-bottom: 1px solid color-mix(in srgb, var(--t-border) 60%, transparent);
  padding: 0 12px;
  height: 36px;
  cursor: pointer;
}
.tl-left:hover {
  background: var(--t-hover);
}
/* collapsible left column: clip content as the column shrinks; the toggle is
   instant (animating width across every sticky row thrashed layout) */
.tl-corner,
.tl-left,
.tl-left.lane {
  overflow: hidden;
}
/* collapsed: zero the padding + right border so the column reaches a TRUE 0 width
   (box-sizing:border-box otherwise floors it at padding+border ≈ 25px, leaving a
   residual sliver) */
.tl-inner.collapsed .tl-corner,
.tl-inner.collapsed .tl-left,
.tl-inner.collapsed .tl-left.lane {
  padding-left: 0;
  padding-right: 0;
  border-right-width: 0;
}
.row-bar {
  width: 3px;
  height: 18px;
  border-radius: 2px;
  flex: 0 0 auto;
}
.row-title {
  font-size: 13px;
  color: var(--t-text1);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.row-title.done {
  text-decoration: line-through;
  color: var(--t-text3);
}

.tl-track {
  box-sizing: border-box;
  position: relative;
  flex: 0 0 auto;
  height: 36px;
  border-bottom: 1px solid color-mix(in srgb, var(--t-border) 60%, transparent);
  /* faint day gridlines (period follows the current zoom via --tl-day-w) */
  background-image: repeating-linear-gradient(
    90deg,
    transparent 0,
    transparent calc(var(--tl-day-w, 34px) - 1px),
    color-mix(in srgb, var(--t-border) 45%, transparent) calc(var(--tl-day-w, 34px) - 1px),
    color-mix(in srgb, var(--t-border) 45%, transparent) var(--tl-day-w, 34px)
  );
}
/* weeks tier: gridlines every week (daily lines would be too dense) */
.tl-inner.weeks .tl-track {
  background-image: repeating-linear-gradient(
    90deg,
    transparent 0,
    transparent calc(var(--tl-week-w) - 1px),
    color-mix(in srgb, var(--t-border) 45%, transparent) calc(var(--tl-week-w) - 1px),
    color-mix(in srgb, var(--t-border) 45%, transparent) var(--tl-week-w)
  );
}
/* hours tier: faint hour-step minor lines under the stronger day lines */
.tl-inner.hours .tl-track {
  background-image:
    repeating-linear-gradient(
      90deg,
      transparent 0,
      transparent calc(var(--tl-sub-w) - 1px),
      color-mix(in srgb, var(--t-border) 22%, transparent) calc(var(--tl-sub-w) - 1px),
      color-mix(in srgb, var(--t-border) 22%, transparent) var(--tl-sub-w)
    ),
    repeating-linear-gradient(
      90deg,
      transparent 0,
      transparent calc(var(--tl-day-w) - 1px),
      color-mix(in srgb, var(--t-border) 55%, transparent) calc(var(--tl-day-w) - 1px),
      color-mix(in srgb, var(--t-border) 55%, transparent) var(--tl-day-w)
    );
}
/* one continuous today-line spanning every lane + row (a child of .tl-body so it
   scrolls horizontally with the content; hidden behind the sticky left column) */
.tl-body {
  position: relative;
}
/* virtualization spacers: reserve the height of the off-screen rows above/below the
   rendered window so the scrollbar + total height stay correct */
.tl-vspacer {
  flex: 0 0 auto;
}
.tl-today {
  position: absolute;
  top: 0;
  bottom: 0;
  width: 2px;
  background: color-mix(in srgb, var(--t-primary) 70%, transparent);
  z-index: 1;
  pointer-events: none;
}
/* glide bars + today-line on zoom; never while dragging/zooming (would lag) */
.tl-inner.animate .bar,
.tl-inner.animate .ghost {
  transition: left 0.18s ease, width 0.18s ease;
}
.tl-inner.animate .tl-today {
  transition: left 0.18s ease;
}

/* ghost estimate envelope: dashed bar behind the real bar, sized to the estimate */
.ghost {
  position: absolute;
  top: 3px;
  height: 30px;
  box-sizing: border-box;
  border: 2px dashed color-mix(in srgb, var(--ghost-c, var(--t-primary)) 65%, transparent);
  background: color-mix(in srgb, var(--ghost-c, var(--t-primary)) 12%, transparent);
  border-radius: 7px;
  z-index: 1;
  pointer-events: none;
}
.ghost-est {
  position: absolute;
  right: 6px;
  top: 50%;
  transform: translateY(-50%);
  display: inline-flex;
  align-items: center;
  gap: 2px;
  font-size: 10px;
  font-weight: 600;
  white-space: nowrap;
  color: color-mix(in srgb, var(--ghost-c, var(--t-primary)) 80%, var(--t-text1));
}

.bar {
  position: absolute;
  top: 6px;
  height: 24px;
  background: var(--bar-grad);
  border-radius: 6px;
  display: flex;
  align-items: center;
  padding: 0 8px;
  /* border-box so the rendered width equals the date-span px exactly (matches the
     Gantt bar); content-box let the side padding overshoot the due date by 16px. */
  box-sizing: border-box;
  cursor: grab;
  overflow: hidden;
  z-index: 2;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.18);
  user-select: none;
}
.bar:active {
  cursor: grabbing;
}
.bar.point {
  border-radius: 6px;
}
.bar.done {
  opacity: 0.55;
}
.bar-title {
  font-size: 12px;
  color: #fff;
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  pointer-events: none;
}
.bar.done .bar-title {
  text-decoration: line-through;
}
.handle {
  position: absolute;
  top: 0;
  bottom: 0;
  width: 8px;
  cursor: ew-resize;
}
.handle.l {
  left: 0;
}
.handle.r {
  right: 0;
}

.tl-empty {
  padding: 40px;
  text-align: center;
  color: var(--t-text3);
  font-size: 14px;
}

/* unscheduled */
.tl-unsched {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  margin-top: 14px;
}
.us-label {
  font-size: 12px;
  font-weight: 600;
  color: var(--t-text3);
  margin-right: 4px;
}
.us-chip {
  max-width: 220px;
  text-align: left;
  border: none;
  border-left: 3px solid var(--chip, var(--t-primary));
  border-radius: 4px;
  background: var(--t-hover);
  color: var(--t-text1);
  font-size: 12px;
  padding: 3px 8px;
  cursor: pointer;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.us-chip.done {
  text-decoration: line-through;
  color: var(--t-text3);
}

/* hover preview card */
.tl-preview {
  position: fixed;
  z-index: 3000;
  width: 264px;
  background: var(--t-surface);
  border: 1px solid var(--t-border);
  border-radius: 10px;
  box-shadow: 0 8px 28px rgba(0, 0, 0, 0.22);
  padding: 10px 12px;
  pointer-events: none;
  display: flex;
  flex-direction: column;
  gap: 7px;
  animation: tl-pv-in 0.14s ease;
}
@keyframes tl-pv-in {
  from {
    opacity: 0;
    transform: translateY(5px) scale(0.98);
  }
  to {
    opacity: 1;
    transform: none;
  }
}
.pv-head {
  display: flex;
  align-items: flex-start;
  gap: 7px;
}
.pv-flag {
  width: 4px;
  align-self: stretch;
  min-height: 18px;
  border-radius: 2px;
  flex: 0 0 auto;
}
.pv-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--t-text1);
  line-height: 1.3;
  flex: 1;
}
.pv-title.done {
  text-decoration: line-through;
  color: var(--t-text3);
}
.pv-num {
  font-size: 11px;
  color: var(--t-text3);
  flex: 0 0 auto;
}
.pv-dates {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--t-text2);
}
.pv-arrow {
  color: var(--t-text3);
}
.pv-est {
  font-size: 12px;
  color: var(--t-text2);
  display: flex;
  align-items: center;
  gap: 5px;
}
.pv-est-ic {
  font-size: 12px;
}
.pv-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}
.pv-tag {
  font-size: 11px;
  border: 1px solid;
  border-radius: 6px;
  padding: 1px 7px;
  line-height: 1.5;
}
.pv-assignees {
  display: flex;
  gap: 4px;
}
.pv-av {
  width: 22px;
  height: 22px;
  border-radius: 50%;
  background: var(--t-accent-grad);
  color: #fff;
  font-size: 10px;
  font-weight: 600;
  display: grid;
  place-items: center;
}
</style>
