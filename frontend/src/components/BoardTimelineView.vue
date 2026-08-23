<script setup>
import { ref, computed, toRef, onBeforeUnmount } from 'vue'
import { NIcon } from 'naive-ui'
import { TimerOutline } from '@vicons/ionicons5'
import { useTaskMenu } from '@/composables/useTaskMenu'
import { useThemeStore } from '@/stores/theme'
import { PRIORITY_COLORS } from '@/styles/tokens'
import { hueGrad, readableHue } from '@/utils/gradient'
import { useWorkspacesStore } from '@/stores/workspaces'
import { formatEstimateFull, estimateRangeShort } from '@/utils/estimation'
import { useFormat } from '@/composables/useFormat'
import { startOfDay, parseDate as parse } from '@/utils/timeAxis'
import { useChartTimeline } from '@/composables/useChartTimeline'
import { useChartLanes } from '@/composables/useChartLanes'
import { useChartRows } from '@/composables/useChartRows'
import { useChartBars } from '@/composables/useChartBars'
import ChartToolbar from './chart/ChartToolbar.vue'
import ChartAxisHeader from './chart/ChartAxisHeader.vue'
import ChartLaneHeader from './chart/ChartLaneHeader.vue'
import ChartMilestoneMarkers from './chart/ChartMilestoneMarkers.vue'
import ChartTaskRow from './chart/ChartTaskRow.vue'
import ChartUnscheduled from './chart/ChartUnscheduled.vue'
import ChartTaskMenu from './chart/ChartTaskMenu.vue'
import ChartCursorPill from './chart/ChartCursorPill.vue'
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
const counters = computed(() => {
  const out = []
  if (overdueCount.value)
    out.push({ key: 'overdue', text: `${overdueCount.value} просрочено`, overdue: true })
  if (unscheduled.value.length)
    out.push({ key: 'unsched', text: `${unscheduled.value.length} без дат` })
  return out
})

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

// ── bar geometry + drag-to-reschedule (shared with the Gantt view) ──
const { drag, onBarDown, renderRows } = useChartBars({
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

// Bars glide on zoom, but never mid-drag or mid-collapse (the transition would lag).
const animate = computed(() => !drag.value && !zooming.value && !collapsing.value)

onBeforeUnmount(() => {
  clearTimeout(collapseSettle)
})

// ── hover preview card ──
const { formatDue, formatters } = useFormat()
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
    ? estimateRangeShort(
        hover.value.task.start_date,
        hover.value.task.estimate,
        estCfg.value,
        formatters.value,
      )
    : '',
)
</script>

<template>
  <div class="tl">
    <ChartToolbar
      :zoom-idx="zoomIdx"
      :zoom-count="ZOOM.length"
      :left-collapsed="leftCollapsed"
      :counters="counters"
      @today="centerToday"
      @zoom-in="zoomIn()"
      @zoom-out="zoomOut()"
      @toggle-left="toggleLeft"
    />

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

        <!-- swimlanes (windowed: only rows in/near the viewport render; spacers keep
             the scroll height; one continuous today-line spans the whole body) -->
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
              @open="$emit('open', $event)"
              @bar-down="onBarDown"
              @bar-enter="onBarEnter"
              @bar-leave="onBarLeave"
              @menu="menu.open"
            />
          </template>
          <div class="tl-vspacer" :style="{ height: `${vwindow.bottom}px` }" />
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

    <ChartCursorPill v-if="cursor" :x="cursorPill.x" :y="cursorPill.top + 4" :label="cursorLabel" />
  </div>
</template>

<style scoped>
@import './chart/chart-view.css';

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
