<script setup>
import { ref, computed, toRef, onBeforeUnmount } from 'vue'
import { NDropdown, NPopconfirm, NIcon, NTooltip } from 'naive-ui'
import {
  TimerOutline,
  ChevronBackOutline,
  ChevronForwardOutline,
  RibbonOutline,
} from '@vicons/ionicons5'
import { useTaskMenu } from '@/composables/useTaskMenu'
import { useDateLocale } from '@/composables/useDateLocale'
import { useThemeStore } from '@/stores/theme'
import { tasks as tasksApi } from '@/api'
import { PRIORITY_COLORS } from '@/styles/tokens'
import { hueGrad, readableHue } from '@/utils/gradient'
import { useWorkspacesStore } from '@/stores/workspaces'
import {
  formatEstimate,
  formatEstimateFull,
  estimateRangeShort,
  estimateTooltip,
  estimateToDays,
} from '@/utils/estimation'
import {
  DAY_MS,
  HOUR_MS,
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
  // { parentId: [subtask, …] } — scheduled subtasks render as thin sub-bars
  // stacked inside their parent's row (no graph linking; see subBars/rowHeight).
  subtasksByParent: { type: Object, default: () => ({}) },
  // Project milestones — rendered as dashed vertical due-markers across the chart.
  milestones: { type: Array, default: () => [] },
})
const emit = defineEmits(['open', 'changed'])

// Estimation unit/config of the project — drives every estimate the chart renders
// (lane totals, the ghost bar, the hover card).
const estCfg = computed(() => wsStore.estimationFor(props.projectId))

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

// A task is "scheduled" when it has at least one of start/due.
const scheduled = computed(() => props.tasks.filter((t) => t.start_date || t.due_date))
const unscheduled = computed(() => props.tasks.filter((t) => !t.start_date && !t.due_date))

// ── the chart engine, shared with the Gantt view ──
// Axis range + header bands, zoom, the scroll viewport, pan/ctrl-wheel and the hover
// cursor guide all live in useChartTimeline; only the bar layout below is ours.
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
  // A bar drag owns the pointer — the guide would otherwise trail the dragged edge.
  cursorBlocked: () => !!drag.value,
})

// ── lanes (swimlanes) — driven by the composer-bar's groupMode ──
// Lane tasks keep the incoming `props.tasks` order, which already reflects the
// composer's sort (e.g. «Сорт: Статус»).
const { lanes, laneEffort, laneEffortFull } = useChartLanes({
  source: scheduled,
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

// ── row geometry + virtualization (shared with the Gantt view) ──
const { subBars, rowHeight, findTask, vwindow } = useChartRows({
  lanes,
  tasks: toRef(props, 'tasks'),
  subtasksByParent: toRef(props, 'subtasksByParent'),
  scrollY,
  viewH,
  bodyTop,
  bodyEl,
})

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
  return barSpan({
    start: s,
    due: d,
    tier: tier.value,
    rangeStart: range.value.start,
    dayW: dayW.value,
  })
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
  return {
    left: xAt(anchor, range.value.start, dayW.value) - 3,
    width: Math.max(dayW.value, days * dayW.value) + 6,
  }
}
// Today-line x: the real current time in the hours tier (sub-day precision), else
// the centre of today's day cell.
onBeforeUnmount(() => {
  clearTimeout(collapseSettle)
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
  hover.value
    ? (hover.value.task.tag_ids || []).map((id) => props.tagsMap[id]).filter(Boolean)
    : [],
)
const hoverAssignees = computed(() =>
  hover.value
    ? (hover.value.task.assignee_ids || []).map((id) => props.membersMap[id]).filter(Boolean)
    : [],
)
// External GitLab assignees (no Tessera account) — shown by avatar URL.
const hoverGlAssignees = computed(() =>
  hover.value ? hover.value.task.gitlab_assignees || [] : [],
)
const hoverEstimate = computed(() =>
  hover.value && hover.value.task.estimate != null
    ? formatEstimateFull(hover.value.task.estimate, estCfg.value)
    : '',
)
const hoverEstimateRange = computed(() =>
  hover.value
    ? estimateRangeShort(hover.value.task.start_date, hover.value.task.estimate, estCfg.value)
    : '',
)
// Tooltip on the ghost bar: full expansion + projected window (the clock label
// it hangs off already marks it as an estimate, so no "Оценка:" prefix).
function ghostTitle(t) {
  return estimateTooltip(t.start_date, t.estimate, estCfg.value)
}
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
      @pointermove="onHoverMove"
      @pointerleave="onHoverLeave"
      @wheel="onWheel"
      @scroll="onScroll"
    >
      <div
        class="tl-inner"
        :class="[tier, { animate: !drag && !zooming && !collapsing, collapsed: leftCollapsed }]"
        :style="{
          width: `${leftW + axisW}px`,
          '--tl-day-w': `${dayW}px`,
          '--tl-week-w': `${dayW * 7}px`,
          '--tl-sub-w': `${subW}px`,
        }"
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

        <!-- swimlanes (windowed: only rows in/near the viewport render; spacers keep
             the scroll height; one continuous today-line spans the whole body) -->
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

            <div v-else class="tl-row">
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
                  }"
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
                <!-- subtask sub-bars stacked under the parent bar; draggable to
                     reschedule (move / resize an edge) just like the parent. A
                     surface border separates same-priority children from the parent
                     and from each other. -->
                <div
                  v-for="(s, i) in subBars(r.task)"
                  :key="s.id"
                  class="tl-subbar"
                  :class="{ done: s.completed_at, point: !(geom(s).hasStart && geom(s).hasDue) }"
                  :style="{
                    left: `${geom(s).left}px`,
                    width: `${geom(s).width}px`,
                    top: `${SUB_TOP0 + i * SUB_STEP}px`,
                    '--bar-grad': hueGrad(PRIORITY_COLORS[s.priority || 0]),
                  }"
                  @pointerdown="onBarDown($event, s, 'move')"
                  @click="$emit('open', s.id)"
                  @mouseenter="onBarEnter($event, s)"
                  @mouseleave="onBarLeave"
                  @contextmenu.prevent.stop="menu.open($event, s)"
                >
                  <span class="handle l" @pointerdown.stop="onBarDown($event, s, 'start')" />
                  <span class="tl-subbar-title">{{ s.title }}</span>
                  <span class="handle r" @pointerdown.stop="onBarDown($event, s, 'due')" />
                </div>
              </div>
            </div>
          </template>
          <div class="tl-vspacer" :style="{ height: `${vwindow.bottom}px` }" />
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

    <!-- hover preview: a compact task-card snapshot -->
    <Teleport to="body">
      <div
        v-if="hover"
        class="tl-preview"
        :style="
          hover.above
            ? { left: `${hover.x}px`, bottom: `${hover.y}px` }
            : { left: `${hover.x}px`, top: `${hover.y}px` }
        "
      >
        <div class="pv-head">
          <span
            class="pv-flag"
            :style="{ background: hueGrad(PRIORITY_COLORS[hover.task.priority || 0]) }"
          />
          <span class="pv-title" :class="{ done: hover.task.completed_at }">{{
            hover.task.title
          }}</span>
          <span v-if="hover.task.number != null" class="pv-num">#{{ hover.task.number }}</span>
        </div>
        <div v-if="hover.task.start_date || hover.task.due_date" class="pv-dates">
          <span v-if="hover.task.start_date">{{ formatDue(hover.task.start_date) }}</span>
          <span v-if="hover.task.start_date && hover.task.due_date" class="pv-arrow">→</span>
          <span v-if="hover.task.due_date">{{ formatDue(hover.task.due_date) }}</span>
        </div>
        <div v-if="hoverEstimate" class="pv-est">
          <n-icon :component="TimerOutline" :size="13" class="pv-est-ic" />
          <span
            >{{ hoverEstimate
            }}<template v-if="hoverEstimateRange"> ({{ hoverEstimateRange }})</template></span
          >
        </div>
        <div v-if="hoverTags.length" class="pv-tags">
          <span
            v-for="tg in hoverTags"
            :key="tg.id"
            class="pv-tag"
            :style="{ color: readableTag(tg.color), borderColor: readableTag(tg.color) }"
            >{{ tg.name }}</span
          >
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

    <!-- cursor-line date pill: pinned to the top of the chart, following the
         pointer's x (a fixed element, so vertical scroll never hides it) -->
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
/* neutral pointer cursor-line: a faint grey dashed guide under the bars (z1, like
   today) so it never obscures them — purely an auxiliary "read the date" aid */
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
/* glide bars + today-line on zoom; never while dragging/zooming (would lag) */
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

/* subtask sub-bar: a thinner bar below the parent, priority-coloured. The
   surface-coloured border guarantees separation even when a child shares the
   parent's priority colour (the requested "contrast / good border"). Named
   tl-subbar to avoid the unrelated .subbar in the board composer. */
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
  overflow: hidden;
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
