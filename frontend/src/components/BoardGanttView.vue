<script setup>
import { ref, computed, toRef, onBeforeUnmount, onMounted, watch } from 'vue'
import { NDropdown, NPopconfirm, NIcon, NTooltip } from 'naive-ui'
import {
  TimerOutline,
  ChevronBackOutline,
  ChevronForwardOutline,
  RibbonOutline,
} from '@vicons/ionicons5'
import { useTaskMenu } from '@/composables/useTaskMenu'
import { useThemeStore } from '@/stores/theme'
import { tasks as tasksApi, boards as boardsApi } from '@/api'
import { PRIORITY_COLORS } from '@/styles/tokens'
import { hueGrad } from '@/utils/gradient'
import { topoByDeps } from '@/utils/dependencyOrder'
import { useWorkspacesStore } from '@/stores/workspaces'
import { formatEstimate, estimateTooltip, estimateToDays } from '@/utils/estimation'
import {
  HOUR_MS,
  DAY_MS,
  startOfDay,
  isAllDayMs,
  barSpan,
  anchorMs,
  xAt,
  parseDate as parse,
} from '@/utils/timeAxis'
import { useChartTimeline } from '@/composables/useChartTimeline'
import { useChartLanes } from '@/composables/useChartLanes'
import { useChartRows, SUB_STEP, SUB_TOP0 } from '@/composables/useChartRows'

const wsStore = useWorkspacesStore()

// The Gantt view is the Timeline engine (axis / bars / zoom / pan / today-line /
// drag-to-reschedule) plus task dependencies: blocking arrows between bars and
// drag-from-bar to create a link. Bars are laid out one task per row so arrows
// have a clean endpoint on every task.
useThemeStore()

const props = defineProps({
  boardId: { type: String, default: '' },
  tasks: { type: Array, default: () => [] },
  statusColumns: { type: Array, default: () => [] },
  membersMap: { type: Object, default: () => ({}) },
  tagsMap: { type: Object, default: () => ({}) },
  // Swimlane grouping from the shared composer-bar: 'status' | 'tag' (+ prefix) |
  // 'assignee' | 'none'.
  groupMode: { type: String, default: 'assignee' },
  tagPrefix: { type: String, default: '' },
  projectId: { type: String, default: null },
  // { parentId: [subtask, …] } — scheduled subtasks render as thin sub-bars
  // stacked inside their parent's row (no dependency arrows on subtasks).
  subtasksByParent: { type: Object, default: () => ({}) },
  // "Авто": when on, rows are ordered by the blocking-dependency graph (DFS
  // pre-order) instead of the incoming list order. Only meaningful with no
  // grouping/sort, which the composer-bar enforces before setting this.
  autoSort: { type: Boolean, default: false },
  // Project milestones — dashed vertical due-markers across the chart.
  milestones: { type: Array, default: () => [] },
})
const emit = defineEmits(['open', 'changed'])

// Estimation unit/config of the project — drives every estimate the chart renders
// (lane totals, the ghost bar, the bar tooltip).
const estCfg = computed(() => wsStore.estimationFor(props.projectId))

const menu = useTaskMenu({
  onOpen: (id) => emit('open', id),
  onChanged: () => emit('changed'),
  columns: () => props.statusColumns,
})

const LEFT_W = 224 // px of the fixed task column (expanded)
// Collapsible left column: collapses to a true 0 (padding + border are zeroed in
// the .collapsed state) so the chart and the arrow overlay stay pixel-aligned.
// Toggle is instant — animating width across every sticky row thrashed layout;
// `collapsing` suppresses the today-line's left-transition for one frame so it
// snaps together with the rest instead of sliding.
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
const LANE_H = 32 // initial lane-header row height, before the real one is measured
const BAR_CY = 18 // bar vertical centre within its row (top 6 + height 24 / 2)
// Sub-bars stack below the parent bar within the same row (SUB_STEP / SUB_TOP0 come
// from useChartRows). Arrows stay anchored to the parent bar (BAR_CY), so taller
// rows don't disturb the dependency geometry.

const scheduled = computed(() => props.tasks.filter((t) => t.start_date || t.due_date))
const unscheduled = computed(() => props.tasks.filter((t) => !t.start_date && !t.due_date))

// "Авто" order: rows follow the blocking-dependency graph (see topoByDeps).
const orderedScheduled = computed(() =>
  props.autoSort ? topoByDeps(scheduled.value, deps.value) : scheduled.value,
)

// ── the chart engine, shared with the Timeline view ──
// Axis range + header bands, zoom, the scroll viewport, pan/ctrl-wheel and the hover
// cursor guide all live in useChartTimeline; the dependency layer below is ours.
const {
  scrollEl,
  bodyEl,
  ZOOM,
  zoomIdx,
  dayW,
  zooming,
  zoomIn,
  zoomOut,
  todayMs,
  tier,
  range,
  axisW,
  days,
  monthBands,
  weekBands,
  hourTicks,
  subW,
  todayLeft,
  milestoneMarkers,
  scrollY,
  viewH,
  bodyTop,
  onScroll,
  centerToday,
  pan,
  onPanDown,
  onWheel,
  cursor,
  cursorPill,
  cursorLabel,
  onHoverMove,
  onHoverLeave,
} = useChartTimeline({
  scheduled,
  milestones: toRef(props, 'milestones'),
  estCfg,
  leftW,
  // The link knob sits ON the track, so a pointerdown there must draw a dependency
  // rather than start a pan.
  panBlockers: ['.bar', '.link-knob'],
  cursorBlocked: () => !!drag.value || !!link.value,
})

// ── lanes (swimlanes) — driven by the composer-bar's groupMode ──
const { lanes, laneEffort, laneEffortFull } = useChartLanes({
  source: orderedScheduled,
  statusColumns: toRef(props, 'statusColumns'),
  membersMap: toRef(props, 'membersMap'),
  tagsMap: toRef(props, 'tagsMap'),
  groupMode: toRef(props, 'groupMode'),
  tagPrefix: toRef(props, 'tagPrefix'),
  estCfg,
})

const overdueCount = computed(
  () =>
    scheduled.value.filter(
      (t) => t.due_date && !t.completed_at && startOfDay(parse(t.due_date)) < todayMs,
    ).length,
)

// ── row geometry + virtualization (shared with the Timeline view) ──
const { subBars, rowHeight, findTask, flatRows, rowLayout, vwindow } = useChartRows({
  lanes,
  tasks: toRef(props, 'tasks'),
  subtasksByParent: toRef(props, 'subtasksByParent'),
  scrollY,
  viewH,
  bodyTop,
  bodyEl,
  laneH0: LANE_H,
})

// Vertical top of every task row (the arrow overlay + link-drag reuse this); derived
// from the same layout so DOM windowing never shifts an arrow endpoint.
const positions = computed(() => {
  const rows = flatRows.value
  const { tops, height } = rowLayout.value
  const map = {}
  for (let i = 0; i < rows.length; i++) if (rows[i].t === 'task') map[rows[i].task.id] = tops[i]
  return { map, height }
})
// Vertical bar-centre for every linkable entity — top-level tasks AND scheduled
// subtasks — so drag-to-link and dependency arrows anchor on sub-bars too. A
// top-level bar centres at BAR_CY; a subtask's i-th sub-bar centres at its offset.
const SUB_BAR_H = 14
const anchorY = computed(() => {
  const pos = positions.value.map
  const m = {}
  for (const id in pos) m[id] = pos[id] + BAR_CY
  for (const t of props.tasks) {
    const top = pos[t.id]
    if (top == null) continue
    const subs = subBars(t)
    for (let i = 0; i < subs.length; i++)
      m[subs[i].id] = top + SUB_TOP0 + i * SUB_STEP + SUB_BAR_H / 2
  }
  return m
})
// ── dependencies (blocking edges) ──
const deps = ref([]) // normalised [{ id, blocker, blocked }]
async function loadDeps() {
  if (!props.boardId) return
  try {
    const rows = (await boardsApi.dependencies(props.boardId)).data || []
    // Normalise raw relation rows to blocker→blocked. Dedupe so a pair linked from
    // both sides ('blocks' + 'blocked_by') draws a single arrow.
    const seen = new Set()
    const out = []
    for (const r of rows) {
      const blocker = r.kind === 'blocked_by' ? r.related_task_id : r.task_id
      const blocked = r.kind === 'blocked_by' ? r.task_id : r.related_task_id
      const key = `${blocker}>${blocked}`
      if (seen.has(key)) continue
      seen.add(key)
      out.push({ id: r.id, blocker, blocked })
    }
    deps.value = out
  } catch {
    deps.value = []
  }
}
onMounted(loadDeps)
// Refetch when the task set changes (a link added elsewhere, a task removed).
watch(
  () => props.tasks.map((t) => t.id).join(','),
  () => loadDeps(),
)

// Bar geometry for a task (honours an active reschedule preview). In the hours tier
// a timed start/due sits at its real clock time (see barSpan).
function geom(t) {
  let s = parse(t.start_date)
  let d = parse(t.due_date)
  if (preview.value && preview.value.id === t.id) {
    s = preview.value.start
    d = preview.value.due
  }
  return barSpan({
    start: s,
    due: d,
    tier: tier.value,
    rangeStart: range.value.start,
    dayW: dayW.value,
  })
}

// Ghost "estimate" envelope: dashed bar from the span start, length = the estimate
// in calendar days (time unit only; null otherwise). Mirrors the timeline view.
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
  return {
    left: xAt(anchor, range.value.start, dayW.value) - 3,
    width: Math.max(dayW.value, days * dayW.value) + 6,
  }
}
// Tooltip on the ghost bar: full expansion + projected window (the clock label
// it hangs off already marks it as an estimate, so no "Оценка:" prefix).
function ghostTitle(t) {
  return estimateTooltip(t.start_date, t.estimate, estCfg.value)
}

// Arrow paths: blocker's finish → blocked's start (finish-to-start), an S-curve
// with a horizontal stub on each end so the arrowhead enters the start cleanly.
const arrows = computed(() => {
  const ay = anchorY.value
  const out = []
  for (const e of deps.value) {
    if (!(e.blocker in ay) || !(e.blocked in ay)) continue
    const tb = findTask(e.blocker)
    const tk = findTask(e.blocked)
    if (!tb || !tk) continue
    const gb = geom(tb)
    const gk = geom(tk)
    const x1 = leftW.value + gb.left + gb.width
    const y1 = ay[e.blocker]
    const x2 = leftW.value + gk.left
    const y2 = ay[e.blocked]
    const dx = Math.max(22, Math.abs(x2 - x1) * 0.4)
    const d = `M ${x1} ${y1} C ${x1 + dx} ${y1}, ${x2 - dx} ${y2}, ${x2} ${y2}`
    out.push({ id: e.id, d, mx: (x1 + x2) / 2, my: (y1 + y2) / 2 })
  }
  return out
})

// ── drag-to-reschedule (move whole bar / resize an edge) ──
const drag = ref(null)
const preview = ref(null)

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
  // Hour-snap when zoomed into the hours tier and the edited endpoint is timed; an
  // all-day (UTC-midnight) endpoint keeps day snapping so it stays all-day.
  const base = g.mode === 'due' ? (g.baseDue ?? g.baseStart) : (g.baseStart ?? g.baseDue)
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
  const t = findTask(g.id)
  if (!t) return
  try {
    // Omit description — board tasks don't carry it; the backend preserves the
    // stored text on a full-replace that leaves description out.
    await tasksApi.update(t.id, {
      title: t.title,
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

// ── drag-to-link (create a dependency) ──
// Drag from a bar's right-edge link knob to another bar/row; the source task is
// set to *block* the target. A live S-curve previews the connection.
const link = ref(null) // { fromId, x1, y1, x, y }
function bodyCoords(e) {
  const r = bodyEl.value.getBoundingClientRect()
  return { x: e.clientX - r.left, y: e.clientY - r.top }
}
function onLinkDown(e, t) {
  if (e.button != null && e.button !== 0) return
  e.preventDefault()
  e.stopPropagation()
  const g = geom(t)
  const c = bodyCoords(e)
  link.value = {
    fromId: t.id,
    x1: leftW.value + g.left + g.width,
    y1: anchorY.value[t.id] ?? positions.value.map[t.id] + BAR_CY,
    x: c.x,
    y: c.y,
  }
  window.addEventListener('pointermove', onLinkMove)
  window.addEventListener('pointerup', onLinkUp)
}
function onLinkMove(e) {
  if (!link.value) return
  const c = bodyCoords(e)
  link.value.x = c.x
  link.value.y = c.y
}
async function onLinkUp(e) {
  const l = link.value
  window.removeEventListener('pointermove', onLinkMove)
  window.removeEventListener('pointerup', onLinkUp)
  link.value = null
  if (!l) return
  // Resolve the drop target from whatever element is under the pointer.
  const el = document.elementFromPoint(e.clientX, e.clientY)
  const host = el?.closest('[data-task-id]')
  const targetId = host?.getAttribute('data-task-id')
  if (!targetId || targetId === l.fromId) return
  const target = findTask(targetId)
  if (!target || target.number == null) return
  // Skip if this edge already exists (either direction would create a 2-cycle).
  const dup = deps.value.some(
    (d) =>
      (d.blocker === l.fromId && d.blocked === targetId) ||
      (d.blocker === targetId && d.blocked === l.fromId),
  )
  if (dup) return
  try {
    await tasksApi.addRelation(l.fromId, target.number, 'blocks')
    await loadDeps()
    emit('changed')
  } catch {
    /* ignore */
  }
}

const linkPath = computed(() => {
  const l = link.value
  if (!l) return ''
  const dx = Math.max(22, Math.abs(l.x - l.x1) * 0.4)
  return `M ${l.x1} ${l.y1} C ${l.x1 + dx} ${l.y1}, ${l.x - dx} ${l.y}, ${l.x} ${l.y}`
})

// ── delete an edge ──
async function removeEdge(id) {
  try {
    await tasksApi.removeRelation(id)
    await loadDeps()
    emit('changed')
  } catch {
    /* ignore */
  }
}

onBeforeUnmount(() => {
  window.removeEventListener('pointermove', onMove)
  window.removeEventListener('pointerup', onUp)
  window.removeEventListener('pointermove', onLinkMove)
  window.removeEventListener('pointerup', onLinkUp)
  clearTimeout(collapseSettle)
})
</script>

<template>
  <div class="tl">
    <div class="tl-toolbar">
      <button class="tl-today-btn" type="button" @click="centerToday">Сегодня</button>
      <div class="tl-zoom">
        <button
          class="tl-zoom-btn"
          type="button"
          :disabled="zoomIdx === 0"
          title="Уменьшить масштаб"
          @click="zoomOut()"
        >
          −
        </button>
        <button
          class="tl-zoom-btn"
          type="button"
          :disabled="zoomIdx === ZOOM.length - 1"
          title="Увеличить масштаб"
          @click="zoomIn()"
        >
          +
        </button>
        <button
          class="tl-zoom-btn"
          type="button"
          :title="leftCollapsed ? 'Показать колонку задач' : 'Свернуть колонку задач'"
          @click="toggleLeft"
        >
          <n-icon
            :component="leftCollapsed ? ChevronForwardOutline : ChevronBackOutline"
            :size="15"
          />
        </button>
      </div>
      <span class="tl-hint">Тяните от правого края задачи к другой, чтобы создать зависимость</span>
      <div class="tl-counters">
        <span v-if="overdueCount" class="tl-counter overdue">{{ overdueCount }} просрочено</span>
        <span v-if="deps.length" class="tl-counter">{{ deps.length }} связей</span>
        <span v-if="unscheduled.length" class="tl-counter">{{ unscheduled.length }} без дат</span>
      </div>
    </div>

    <div
      ref="scrollEl"
      class="tl-scroll"
      :class="{ panning: !!pan, linking: !!link }"
      @pointerdown="onPanDown"
      @pointermove="onHoverMove"
      @pointerleave="onHoverLeave"
      @wheel="onWheel"
      @scroll="onScroll"
    >
      <div
        class="tl-inner"
        :class="[
          tier,
          { animate: !drag && !link && !zooming && !collapsing, collapsed: leftCollapsed },
        ]"
        :style="{
          width: `${leftW + axisW}px`,
          '--tl-day-w': `${dayW}px`,
          '--tl-week-w': `${dayW * 7}px`,
          '--tl-sub-w': `${subW}px`,
        }"
      >
        <!-- header: months + day/week band (+ hour ticks in the hours tier) -->
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
              <div
                v-for="w in weekBands"
                :key="w.key"
                class="tl-weekh"
                :style="{ width: `${w.span * dayW}px` }"
              >
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
                <span
                  v-for="h in hourTicks"
                  :key="h.key"
                  class="tl-hourtick"
                  :style="{ left: `${h.left}px` }"
                  >{{ h.label }}</span
                >
              </div>
            </template>
          </div>
        </div>

        <!-- body: windowed lanes + rows (spacers keep the scroll height), with the
             full-height dependency-arrow SVG overlay on top -->
        <div ref="bodyEl" class="tl-body">
          <div class="tl-today" :style="{ left: `${leftW + todayLeft}px` }" />
          <div
            v-for="m in milestoneMarkers"
            :key="m.id"
            class="tl-ms"
            :style="{ left: `${leftW + m.left}px` }"
          >
            <span class="tl-ms-label"
              ><n-icon :component="RibbonOutline" :size="11" /> {{ m.title }}</span
            >
          </div>
          <div v-if="cursor" class="tl-cursor" :style="{ left: `${leftW + cursor.axisX}px` }" />
          <div class="tl-vspacer" :style="{ height: `${vwindow.top}px` }" />
          <template v-for="r in vwindow.rows" :key="r.key">
            <div v-if="r.t === 'lane'" class="tl-lanehead">
              <div class="tl-left lane" :style="{ width: `${leftW}px` }">
                <span
                  class="lane-dot"
                  :style="{
                    background: r.lane.color ? hueGrad(r.lane.color) : 'var(--t-accent-grad)',
                  }"
                />
                <span class="lane-name">{{ r.lane.label }}</span>
                <span class="lane-count">{{ r.lane.tasks.length }}</span>
                <n-tooltip v-if="laneEffort(r.lane)">
                  <template #trigger>
                    <span class="lane-effort"
                      ><n-icon :component="TimerOutline" :size="12" />
                      {{ laneEffort(r.lane) }}</span
                    >
                  </template>
                  Суммарная оценка: {{ laneEffortFull(r.lane) }}
                </n-tooltip>
              </div>
              <div class="tl-track laneband" :style="{ width: `${axisW}px` }" />
            </div>

            <div v-else class="tl-row" :data-task-id="r.task.id">
              <div
                class="tl-left"
                :style="{ width: `${leftW}px`, height: `${rowHeight(r.task)}px` }"
                :title="r.task.title"
                @click="$emit('open', r.task.id)"
              >
                <span
                  class="row-bar"
                  :style="{ background: hueGrad(PRIORITY_COLORS[r.task.priority || 0]) }"
                />
                <span class="row-title" :class="{ done: r.task.completed_at }">{{
                  r.task.title
                }}</span>
              </div>
              <div
                class="tl-track"
                :style="{ width: `${axisW}px`, height: `${rowHeight(r.task)}px` }"
              >
                <div
                  v-if="ghostGeom(r.task)"
                  class="ghost"
                  :style="{
                    left: `${ghostGeom(r.task).left}px`,
                    width: `${ghostGeom(r.task).width}px`,
                    '--ghost-c': PRIORITY_COLORS[r.task.priority || 0],
                  }"
                >
                  <n-tooltip>
                    <template #trigger>
                      <span class="ghost-est"
                        ><n-icon :component="TimerOutline" :size="11" />
                        {{ formatEstimate(r.task.estimate, estCfg) }}</span
                      >
                    </template>
                    {{ ghostTitle(r.task) }}
                  </n-tooltip>
                </div>
                <div
                  class="bar"
                  :class="{
                    done: r.task.completed_at,
                    point: !(geom(r.task).hasStart && geom(r.task).hasDue),
                    linksrc: link && link.fromId === r.task.id,
                  }"
                  :style="{
                    left: `${geom(r.task).left}px`,
                    width: `${geom(r.task).width}px`,
                    '--bar-grad': hueGrad(PRIORITY_COLORS[r.task.priority || 0]),
                  }"
                  @pointerdown="onBarDown($event, r.task, 'move')"
                  @click="$emit('open', r.task.id)"
                  @contextmenu.prevent.stop="menu.open($event, r.task)"
                >
                  <span class="handle l" @pointerdown.stop="onBarDown($event, r.task, 'start')" />
                  <span class="bar-title">{{ r.task.title }}</span>
                  <span class="handle r" @pointerdown.stop="onBarDown($event, r.task, 'due')" />
                  <span
                    class="link-knob"
                    title="Создать зависимость"
                    @pointerdown="onLinkDown($event, r.task)"
                    @click.stop
                  />
                </div>
                <!-- subtask sub-bars stacked under the parent bar; draggable to
                     reschedule (move / resize an edge) and to link (right-edge knob)
                     so subtasks can carry blocking dependencies too. -->
                <div
                  v-for="(s, i) in subBars(r.task)"
                  :key="s.id"
                  class="tl-subbar"
                  :class="{
                    done: s.completed_at,
                    point: !(geom(s).hasStart && geom(s).hasDue),
                    linksrc: link && link.fromId === s.id,
                  }"
                  :data-task-id="s.id"
                  :style="{
                    left: `${geom(s).left}px`,
                    width: `${geom(s).width}px`,
                    top: `${SUB_TOP0 + i * SUB_STEP}px`,
                    '--bar-grad': hueGrad(PRIORITY_COLORS[s.priority || 0]),
                  }"
                  @pointerdown="onBarDown($event, s, 'move')"
                  @click="$emit('open', s.id)"
                  @contextmenu.prevent.stop="menu.open($event, s)"
                >
                  <span class="handle l" @pointerdown.stop="onBarDown($event, s, 'start')" />
                  <span class="tl-subbar-title">{{ s.title }}</span>
                  <span class="handle r" @pointerdown.stop="onBarDown($event, s, 'due')" />
                  <span
                    class="link-knob"
                    title="Создать зависимость"
                    @pointerdown="onLinkDown($event, s)"
                    @click.stop
                  />
                </div>
              </div>
            </div>
          </template>
          <div class="tl-vspacer" :style="{ height: `${vwindow.bottom}px` }" />

          <!-- dependency arrows + live link preview -->
          <svg
            class="g-arrows"
            :width="leftW + axisW"
            :height="positions.height"
            :style="{ height: `${positions.height}px` }"
          >
            <defs>
              <marker
                id="g-arrowhead"
                viewBox="0 0 8 8"
                refX="6.5"
                refY="4"
                markerWidth="7"
                markerHeight="7"
                orient="auto-start-reverse"
              >
                <path d="M0 0 L8 4 L0 8 z" :fill="'var(--t-primary)'" />
              </marker>
            </defs>
            <g v-for="a in arrows" :key="a.id" class="g-arrow">
              <path class="g-hit" :d="a.d" />
              <path class="g-line" :d="a.d" marker-end="url(#g-arrowhead)" />
              <g class="g-del" :transform="`translate(${a.mx},${a.my})`" @click="removeEdge(a.id)">
                <circle r="8" />
                <path d="M-3 -3 L3 3 M3 -3 L-3 3" />
              </g>
            </g>
            <path
              v-if="linkPath"
              class="g-linkpreview"
              :d="linkPath"
              marker-end="url(#g-arrowhead)"
            />
          </svg>
        </div>

        <div v-if="!lanes.length" class="tl-empty">
          Нет задач со сроками. Задайте срок или начало в карточке.
        </div>
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

    <!-- cursor-line date pill: pinned to the top of the chart, following the
         pointer's x (fixed, so vertical scroll never hides it) -->
    <Teleport to="body">
      <div
        v-if="cursor"
        class="tl-cursor-pill"
        :style="{ left: `${cursorPill.x}px`, top: `${cursorPill.top + 4}px` }"
      >
        {{ cursorLabel }}
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
.tl-hint {
  font-size: 12px;
  color: var(--t-text3);
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
.tl-scroll.linking {
  cursor: crosshair;
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
  box-sizing: border-box;
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
/* task-row left column: top-align so the title stays beside the parent bar when
   the row grows to fit subtask sub-bars (height comes from an inline style) */
.tl-row .tl-left {
  align-items: flex-start;
  padding-top: 9px;
}
/* collapsible left column: clip content as the column shrinks; the toggle is
   instant (animating width across every sticky row thrashed layout) */
.tl-corner,
.tl-left,
.tl-left.lane {
  overflow: hidden;
}
/* collapsed: zero the padding + right border so the column reaches a TRUE 0 width
   (box-sizing:border-box otherwise floors it at padding+border ≈ 25px, which left a
   residual sliver and shoved the arrow overlay out of alignment with the bars) */
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
.tl-body {
  position: relative;
}
/* virtualization spacers: reserve the height of the off-screen rows above/below the
   rendered window so the scrollbar + arrow-overlay geometry stay correct */
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
/* neutral pointer cursor-line: a faint grey dashed guide under the bars (z1) —
   purely an auxiliary "read the date" aid */
.tl-cursor {
  position: absolute;
  top: 0;
  bottom: 0;
  width: 0;
  border-left: 1px dashed color-mix(in srgb, var(--t-text3) 60%, transparent);
  z-index: 1;
  pointer-events: none;
}
/* date/time pill that rides the cursor-line, pinned to the chart's top edge —
   a muted neutral chip, not an accent element */
.tl-cursor-pill {
  position: fixed;
  z-index: 3500;
  transform: translateX(-50%);
  background: var(--t-surface);
  color: var(--t-text2);
  border: 1px solid var(--t-border);
  font-size: 11px;
  font-weight: 500;
  padding: 2px 7px;
  border-radius: 6px;
  white-space: nowrap;
  pointer-events: none;
  box-shadow: 0 2px 6px rgba(0, 0, 0, 0.16);
}
.tl-inner.animate .bar,
.tl-inner.animate .ghost {
  transition:
    left 0.18s ease,
    width 0.18s ease;
}
.tl-inner.animate .tl-today {
  transition: left 0.18s ease;
}
/* milestone due-marker: dashed vertical line + a small label at the top */
.tl-ms {
  position: absolute;
  top: 0;
  bottom: 0;
  width: 0;
  border-left: 1.5px dashed color-mix(in srgb, var(--t-primary) 55%, transparent);
  z-index: 1;
  pointer-events: none;
}
.tl-ms-label {
  position: sticky;
  top: 2px;
  display: inline-flex;
  align-items: center;
  gap: 2px;
  margin-left: 3px;
  padding: 0 5px;
  height: 16px;
  border-radius: 8px;
  font-size: 10px;
  white-space: nowrap;
  color: var(--t-primary);
  background: color-mix(in srgb, var(--t-primary) 12%, var(--t-surface));
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
  /* the envelope is pointer-events:none (so the bar stays draggable); re-enable
     events on the label itself so its estimate tooltip can trigger */
  pointer-events: auto;
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
  /* border-box so the rendered width equals the date-span px exactly — without it
     the 8px side padding pushed the visible right edge 16px past `geom.width`, and
     the dependency arrows (anchored at geom.left+geom.width) started inside the bar. */
  box-sizing: border-box;
  cursor: grab;
  z-index: 2;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.18);
  user-select: none;
}
.bar:active {
  cursor: grabbing;
}
.bar.done {
  opacity: 0.55;
}
.bar.linksrc {
  outline: 2px solid var(--t-primary);
  outline-offset: 1px;
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

/* subtask sub-bar: a thinner bar below the parent, priority-coloured. The
   surface-coloured border guarantees separation even when a child shares the
   parent's priority colour. Named tl-subbar to avoid the board composer's .subbar. */
.tl-subbar {
  position: absolute;
  height: 14px;
  background: var(--bar-grad);
  border: 1px solid var(--t-surface);
  border-radius: 5px;
  box-sizing: border-box;
  display: flex;
  align-items: center;
  padding: 0 6px;
  cursor: grab;
  /* visible (not hidden) so the right-edge link knob isn't clipped; the title clamps
     itself via flex:1 + min-width:0 below. */
  overflow: visible;
  z-index: 2;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.16);
  user-select: none;
}
.tl-subbar:active {
  cursor: grabbing;
}
.tl-subbar.done {
  opacity: 0.5;
}
.tl-subbar-title {
  flex: 1;
  min-width: 0;
  font-size: 10px;
  color: #fff;
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  pointer-events: none;
}
.tl-subbar.done .tl-subbar-title {
  text-decoration: line-through;
}

/* drag-to-link knob on the bar's right edge (reveal on hover) */
.link-knob {
  position: absolute;
  right: -7px;
  top: 50%;
  transform: translateY(-50%);
  width: 14px;
  height: 14px;
  border-radius: 50%;
  background: var(--t-surface);
  border: 2px solid var(--t-primary);
  cursor: crosshair;
  opacity: 0;
  transition: opacity 0.12s ease;
  z-index: 3;
}
.bar:hover .link-knob,
.bar.linksrc .link-knob,
.tl-subbar:hover .link-knob,
.tl-subbar.linksrc .link-knob {
  opacity: 1;
}

/* dependency arrows */
.g-arrows {
  position: absolute;
  top: 0;
  left: 0;
  pointer-events: none;
  z-index: 3;
  overflow: visible;
}
.g-line {
  fill: none;
  stroke: var(--t-primary);
  stroke-width: 1.6;
  opacity: 0.7;
}
.g-hit {
  fill: none;
  stroke: transparent;
  stroke-width: 12;
  pointer-events: stroke;
  cursor: pointer;
}
.g-arrow:hover .g-line {
  opacity: 1;
  stroke-width: 2.2;
}
.g-del {
  opacity: 0;
  pointer-events: none;
  cursor: pointer;
}
.g-arrow:hover .g-del {
  opacity: 1;
  pointer-events: auto;
}
.g-del circle {
  fill: #e0533d;
}
.g-del path {
  stroke: #fff;
  stroke-width: 1.6;
  stroke-linecap: round;
}
.g-linkpreview {
  fill: none;
  stroke: var(--t-primary);
  stroke-width: 2;
  stroke-dasharray: 5 4;
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
</style>
