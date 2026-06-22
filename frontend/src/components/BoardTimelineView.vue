<script setup>
import { ref, computed, onBeforeUnmount, nextTick, watch } from 'vue'
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

const DAY_MS = 86400000
const LEFT_W = 224 // px of the fixed task/lane column (expanded)
// Collapsible left column: width animates to 0 to give the chart full width.
const leftCollapsed = ref(false)
const leftW = computed(() => (leftCollapsed.value ? 0 : LEFT_W))

// ── zoom (px per day) ──
const ZOOM = [12, 16, 22, 30, 40, 56, 76]
const zoomIdx = ref(3) // → 30px/day
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

// ── date helpers ──
const startOfDay = (ms) => {
  const d = new Date(ms)
  return new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime()
}
const todayMs = startOfDay(Date.now())
const parse = (s) => (s ? Date.parse(s) : null)

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

const MONTHS = ['янв', 'фев', 'мар', 'апр', 'май', 'июн', 'июл', 'авг', 'сен', 'окт', 'ноя', 'дек']
const WD = ['вс', 'пн', 'вт', 'ср', 'чт', 'пт', 'сб']

const days = computed(() => {
  const out = []
  for (let i = 0; i < range.value.days; i++) {
    const d = new Date(range.value.start + i * DAY_MS)
    const dow = d.getDay()
    out.push({
      ms: d.getTime(),
      day: d.getDate(),
      dow: WD[dow],
      weekend: dow === 0 || dow === 6,
      isToday: d.getTime() === todayMs,
    })
  }
  return out
})
// Month label segments (consecutive days of the same month).
const monthBands = computed(() => {
  const out = []
  for (const d of days.value) {
    const dt = new Date(d.ms)
    const key = `${dt.getFullYear()}-${dt.getMonth()}`
    const last = out[out.length - 1]
    if (last && last.key === key) last.span++
    else out.push({ key, label: `${MONTHS[dt.getMonth()]} ${dt.getFullYear()}`, span: 1 })
  }
  return out
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
  const delta = Math.round((e.clientX - g.startX) / dayW.value)
  if (delta === 0) {
    preview.value = { id: g.id, start: g.baseStart, due: g.baseDue }
    return
  }
  const shift = delta * DAY_MS
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

// Bar geometry for a task, honouring an active drag preview.
function geom(t) {
  let s = parse(t.start_date)
  let d = parse(t.due_date)
  if (preview.value && preview.value.id === t.id) {
    s = preview.value.start
    d = preview.value.due
  }
  const a = startOfDay(s ?? d)
  const b = startOfDay(d ?? s)
  const i0 = Math.round((a - range.value.start) / DAY_MS)
  const i1 = Math.round((b - range.value.start) / DAY_MS)
  return {
    left: i0 * dayW.value,
    width: Math.max(1, i1 - i0 + 1) * dayW.value,
    hasStart: s != null,
    hasDue: d != null,
  }
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
  const anchor = startOfDay(s ?? d)
  const i0 = Math.round((anchor - range.value.start) / DAY_MS)
  // Frame the envelope with a 3px margin on the start/end too (matching the top/bottom inset).
  return { left: i0 * dayW.value - 3, width: Math.max(dayW.value, days * dayW.value) + 6 }
}
const todayLeft = computed(() => dayIndex(todayMs) * dayW.value + dayW.value / 2)

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
watch(scrollEl, (el) => el && nextTick(() => centerToday(false)))

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
  clearTimeout(zoomSettle)
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
          @click="leftCollapsed = !leftCollapsed"
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
    >
      <div class="tl-inner" :class="{ animate: !drag && !zooming }" :style="{ width: `${leftW + axisW}px` }">
        <!-- header: month band + day band -->
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
          </div>
        </div>

        <!-- swimlanes (one continuous today-line spans the whole body) -->
        <div class="tl-body">
          <div class="tl-today" :style="{ left: `${leftW + todayLeft}px` }" />
          <template v-for="lane in lanes" :key="lane.key">
          <div class="tl-lanehead">
            <div class="tl-left lane" :style="{ width: `${leftW}px` }">
              <span
                class="lane-dot"
                :style="{ background: lane.color ? hueGrad(lane.color) : 'var(--t-accent-grad)' }"
              />
              <span class="lane-name">{{ lane.label }}</span>
              <span class="lane-count">{{ lane.tasks.length }}</span>
              <span v-if="laneEffort(lane)" class="lane-effort" title="Суммарная оценка"
                ><n-icon :component="TimerOutline" :size="12" /> {{ laneEffort(lane) }}</span
              >
            </div>
            <div class="tl-track laneband" :style="{ width: `${axisW}px`, '--tl-day-w': `${dayW}px` }" />
          </div>

          <div v-for="t in lane.tasks" :key="t.id" class="tl-row">
            <div class="tl-left" :style="{ width: `${leftW}px` }" :title="t.title" @click="$emit('open', t.id)">
              <span class="row-bar" :style="{ background: hueGrad(PRIORITY_COLORS[t.priority || 0]) }" />
              <span class="row-title" :class="{ done: t.completed_at }">{{ t.title }}</span>
            </div>
            <div class="tl-track" :style="{ width: `${axisW}px`, '--tl-day-w': `${dayW}px` }">
              <div
                v-if="ghostGeom(t)"
                class="ghost"
                :style="{ left: `${ghostGeom(t).left}px`, width: `${ghostGeom(t).width}px`, '--ghost-c': PRIORITY_COLORS[t.priority || 0] }"
                :title="ghostTitle(t)"
              >
                <span class="ghost-est"
                  ><n-icon :component="TimerOutline" :size="11" /> {{ formatEstimate(t.estimate, estCfg) }}</span
                >
              </div>
              <div
                class="bar"
                :class="{ done: t.completed_at, point: !(geom(t).hasStart && geom(t).hasDue) }"
                :style="{
                  left: `${geom(t).left}px`,
                  width: `${geom(t).width}px`,
                  '--bar-grad': hueGrad(PRIORITY_COLORS[t.priority || 0]),
                }"
                @pointerdown="onBarDown($event, t, 'move')"
                @click="$emit('open', t.id)"
                @mouseenter="onBarEnter($event, t)"
                @mouseleave="onBarLeave"
                @contextmenu.prevent.stop="menu.open($event, t)"
              >
                <span class="handle l" @pointerdown.stop="onBarDown($event, t, 'start')" />
                <span class="bar-title">{{ t.title }}</span>
                <span class="handle r" @pointerdown.stop="onBarDown($event, t, 'due')" />
              </div>
            </div>
          </div>
          </template>
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
/* collapsible left column: animate width, clip content as it shrinks to 0 */
.tl-corner,
.tl-left,
.tl-left.lane {
  transition: width 0.22s ease;
  overflow: hidden;
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
/* one continuous today-line spanning every lane + row (a child of .tl-body so it
   scrolls horizontally with the content; hidden behind the sticky left column) */
.tl-body {
  position: relative;
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
