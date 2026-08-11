<script setup>
import { ref, computed, toRef, onBeforeUnmount, onMounted, watch } from 'vue'
import { useTaskMenu } from '@/composables/useTaskMenu'
import { useThemeStore } from '@/stores/theme'
import { tasks as tasksApi, boards as boardsApi } from '@/api'
import { topoByDeps } from '@/utils/dependencyOrder'
import { useWorkspacesStore } from '@/stores/workspaces'
import { startOfDay, parseDate as parse } from '@/utils/timeAxis'
import { useChartTimeline } from '@/composables/useChartTimeline'
import { useChartLanes } from '@/composables/useChartLanes'
import { useChartRows, SUB_STEP, SUB_TOP0 } from '@/composables/useChartRows'
import { useChartBars } from '@/composables/useChartBars'
import ChartToolbar from './chart/ChartToolbar.vue'
import ChartAxisHeader from './chart/ChartAxisHeader.vue'
import ChartLaneHeader from './chart/ChartLaneHeader.vue'
import ChartMilestoneMarkers from './chart/ChartMilestoneMarkers.vue'
import ChartTaskRow from './chart/ChartTaskRow.vue'
import ChartUnscheduled from './chart/ChartUnscheduled.vue'
import ChartTaskMenu from './chart/ChartTaskMenu.vue'
import ChartCursorPill from './chart/ChartCursorPill.vue'

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
const counters = computed(() => {
  const out = []
  if (overdueCount.value)
    out.push({ key: 'overdue', text: `${overdueCount.value} просрочено`, overdue: true })
  if (deps.value.length) out.push({ key: 'deps', text: `${deps.value.length} связей` })
  if (unscheduled.value.length)
    out.push({ key: 'unsched', text: `${unscheduled.value.length} без дат` })
  return out
})

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

// ── bar geometry + drag-to-reschedule (shared with the Timeline view) ──
const { drag, geom, onBarDown, renderRows } = useChartBars({
  tier,
  range,
  dayW,
  estCfg,
  vwindow,
  subBars,
  rowHeight,
  findTask,
  onChanged: () => emit('changed'),
})

// Bars glide on zoom, but never mid-drag/link or mid-collapse (a transition lags).
const animate = computed(() => !drag.value && !link.value && !zooming.value && !collapsing.value)

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
  window.removeEventListener('pointermove', onLinkMove)
  window.removeEventListener('pointerup', onLinkUp)
  clearTimeout(collapseSettle)
})
</script>

<template>
  <div class="tl">
    <ChartToolbar
      :zoom-idx="zoomIdx"
      :zoom-count="ZOOM.length"
      :left-collapsed="leftCollapsed"
      hint="Тяните от правого края задачи к другой, чтобы создать зависимость"
      :counters="counters"
      @today="centerToday"
      @zoom-in="zoomIn()"
      @zoom-out="zoomOut()"
      @toggle-left="toggleLeft"
    />

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
        :class="[tier, { animate, collapsed: leftCollapsed }]"
        :style="{
          width: `${leftW + axisW}px`,
          '--tl-day-w': `${dayW}px`,
          '--tl-week-w': `${dayW * 7}px`,
          '--tl-sub-w': `${subW}px`,
        }"
      >
        <ChartAxisHeader
          :left-w="leftW"
          :day-w="dayW"
          :tier="tier"
          :collapsed="leftCollapsed"
          :month-bands="monthBands"
          :week-bands="weekBands"
          :days="days"
          :hour-ticks="hourTicks"
        />

        <!-- body: windowed lanes + rows (spacers keep the scroll height), with the
             full-height dependency-arrow SVG overlay on top -->
        <div ref="bodyEl" class="tl-body">
          <div class="tl-today" :style="{ left: `${leftW + todayLeft}px` }" />
          <ChartMilestoneMarkers :markers="milestoneMarkers" :left-w="leftW" />
          <div v-if="cursor" class="tl-cursor" :style="{ left: `${leftW + cursor.axisX}px` }" />
          <div class="tl-vspacer" :style="{ height: `${vwindow.top}px` }" />
          <template v-for="r in renderRows" :key="r.key">
            <ChartLaneHeader
              v-if="r.t === 'lane'"
              :lane="r.lane"
              :left-w="leftW"
              :axis-w="axisW"
              :tier="tier"
              :collapsed="leftCollapsed"
              :effort="laneEffort(r.lane)"
              :effort-full="laneEffortFull(r.lane)"
            />
            <ChartTaskRow
              v-else
              :row="r"
              :left-w="leftW"
              :axis-w="axisW"
              :tier="tier"
              :collapsed="leftCollapsed"
              :animate="animate"
              linkable
              :link-from-id="link ? link.fromId : null"
              @open="$emit('open', $event)"
              @bar-down="onBarDown"
              @link-down="onLinkDown"
              @menu="menu.open"
            />
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

    <ChartUnscheduled :tasks="unscheduled" @open="$emit('open', $event)" @menu="menu.open" />

    <ChartTaskMenu
      :show="menu.show.value"
      :x="menu.x.value"
      :y="menu.y.value"
      :options="menu.options.value"
      :delete-show="menu.deleteConfirmShow.value"
      :archive-show="menu.archiveConfirmShow.value"
      @select="menu.select"
      @close="menu.show.value = false"
      @update:delete-show="menu.deleteConfirmShow.value = $event"
      @delete-confirm="menu.confirmDelete()"
      @update:archive-show="menu.archiveConfirmShow.value = $event"
      @archive-confirm="menu.confirmArchive()"
    />

    <ChartCursorPill v-if="cursor" :x="cursorPill.x" :y="cursorPill.top + 4" :label="cursorLabel" />
  </div>
</template>

<style scoped>
@import './chart/chart-view.css';

.tl-scroll.linking {
  cursor: crosshair;
  user-select: none;
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
</style>
