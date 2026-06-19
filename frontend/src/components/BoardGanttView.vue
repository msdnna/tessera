<script setup>
import { ref, computed, onBeforeUnmount, onMounted, nextTick, watch } from 'vue'
import { NDropdown, NPopconfirm } from 'naive-ui'
import { useTaskMenu } from '@/composables/useTaskMenu'
import { useThemeStore } from '@/stores/theme'
import { tasks as tasksApi, boards as boardsApi } from '@/api'
import { PRIORITY_COLORS } from '@/styles/tokens'
import { hueGrad } from '@/utils/gradient'

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
})
const emit = defineEmits(['open', 'changed'])

const menu = useTaskMenu({
  onOpen: (id) => emit('open', id),
  onChanged: () => emit('changed'),
  columns: () => props.statusColumns,
})

const DAY_MS = 86400000
const LEFT_W = 224 // px of the fixed task column
const LANE_H = 32 // lane-header row height (fixed, so SVG geometry is exact)
const ROW_H = 36 // task row height
const BAR_CY = 18 // bar vertical centre within its row (top 6 + height 24 / 2)

// ── zoom (px per day) ──
const ZOOM = [12, 16, 22, 30, 40, 56, 76]
const zoomIdx = ref(3) // → 30px/day
const dayW = computed(() => ZOOM[zoomIdx.value])
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
    dayAtAnchor = (el.scrollLeft + anchorX - LEFT_W) / oldW
  }
  zoomIdx.value = newIdx
  if (el && dayAtAnchor != null) {
    nextTick(() => {
      el.scrollLeft = Math.max(0, dayAtAnchor * newW + LEFT_W - anchorX)
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

const scheduled = computed(() => props.tasks.filter((t) => t.start_date || t.due_date))
const unscheduled = computed(() => props.tasks.filter((t) => !t.start_date && !t.due_date))

function spanOf(t) {
  const s = parse(t.start_date)
  const d = parse(t.due_date)
  const a = s ?? d
  const b = d ?? s
  return { a: startOfDay(a), b: startOfDay(b), hasStart: s != null, hasDue: d != null }
}

// ── axis range ──
const range = computed(() => {
  let lo = todayMs
  let hi = todayMs
  for (const t of scheduled.value) {
    const { a, b } = spanOf(t)
    lo = Math.min(lo, a)
    hi = Math.max(hi, b)
  }
  lo -= 3 * DAY_MS
  hi += 14 * DAY_MS
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
  if (mode === 'status') {
    for (const col of props.statusColumns) ensure(col.id, col.name, col.color)
  }
  for (const t of scheduled.value) {
    if (mode === 'assignee') {
      const id = (t.assignee_ids || [])[0]
      const m = id ? props.membersMap[id] : null
      ensure(id || '∅', m?.name || 'Не назначено').tasks.push(t)
    } else if (mode === 'tag') {
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
  if (mode !== 'status') arr.sort((a, b) => (a.key === '∅' ? 1 : 0) - (b.key === '∅' ? 1 : 0))
  return arr
})

const overdueCount = computed(
  () => scheduled.value.filter((t) => t.due_date && !t.completed_at && startOfDay(parse(t.due_date)) < todayMs).length,
)

// ── per-task row geometry (exact, computed from the lane model) ──
// Each task occupies a fixed-height row; we walk lanes in render order to get the
// vertical top of every task row, which the arrow overlay and link-drag reuse.
const positions = computed(() => {
  const map = {}
  let y = 0
  for (const lane of lanes.value) {
    y += LANE_H
    for (const t of lane.tasks) {
      map[t.id] = y
      y += ROW_H
    }
  }
  return { map, height: y }
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

// Bar geometry for a task (honours an active reschedule preview).
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

// Arrow paths: blocker's finish → blocked's start (finish-to-start), an S-curve
// with a horizontal stub on each end so the arrowhead enters the start cleanly.
const arrows = computed(() => {
  const pos = positions.value.map
  const out = []
  for (const e of deps.value) {
    if (!(e.blocker in pos) || !(e.blocked in pos)) continue
    const tb = props.tasks.find((t) => t.id === e.blocker)
    const tk = props.tasks.find((t) => t.id === e.blocked)
    if (!tb || !tk) continue
    const gb = geom(tb)
    const gk = geom(tk)
    const x1 = LEFT_W + gb.left + gb.width
    const y1 = pos[e.blocker] + BAR_CY
    const x2 = LEFT_W + gk.left
    const y2 = pos[e.blocked] + BAR_CY
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
  drag.value = { id: t.id, mode, startX: e.clientX, baseStart: s, baseDue: d, hasStart: s != null, hasDue: d != null }
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

// ── drag-to-link (create a dependency) ──
// Drag from a bar's right-edge link knob to another bar/row; the source task is
// set to *block* the target. A live S-curve previews the connection.
const link = ref(null) // { fromId, x1, y1, x, y }
const bodyEl = ref(null)
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
    x1: LEFT_W + g.left + g.width,
    y1: positions.value.map[t.id] + BAR_CY,
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
  const target = props.tasks.find((t) => t.id === targetId)
  if (!target || target.number == null) return
  // Skip if this edge already exists (either direction would create a 2-cycle).
  const dup = deps.value.some(
    (d) => (d.blocker === l.fromId && d.blocked === targetId) || (d.blocker === targetId && d.blocked === l.fromId),
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
  cancelAnimationFrame(scrollRaf)
  window.removeEventListener('pointermove', onPanMove)
  window.removeEventListener('pointerup', onPanUp)
})

const todayLeft = computed(() => dayIndex(todayMs) * dayW.value + dayW.value / 2)

// ── scroll-to-today ──
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
  const left = Math.max(0, dayIndex(todayMs) * dayW.value - el.clientWidth / 2 + LEFT_W)
  if (smooth === false) el.scrollLeft = left
  else animateScrollLeft(left)
}
watch(scrollEl, (el) => el && nextTick(() => centerToday(false)))

// ── pan: middle-button anywhere, or left-drag on empty space ──
const pan = ref(null)
function onPanDown(e) {
  const middle = e.button === 1
  const emptyLeft = e.button === 0 && e.target.closest('.tl-track') && !e.target.closest('.bar') && !e.target.closest('.link-knob')
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
function onWheel(e) {
  if (!(e.ctrlKey || e.metaKey)) return
  e.preventDefault()
  if (e.deltaY < 0) zoomIn(e.clientX)
  else zoomOut(e.clientX)
}
</script>

<template>
  <div class="tl">
    <div class="tl-toolbar">
      <button class="tl-today-btn" type="button" @click="centerToday">Сегодня</button>
      <div class="tl-zoom">
        <button class="tl-zoom-btn" type="button" :disabled="zoomIdx === 0" title="Уменьшить масштаб" @click="zoomOut()">−</button>
        <button class="tl-zoom-btn" type="button" :disabled="zoomIdx === ZOOM.length - 1" title="Увеличить масштаб" @click="zoomIn()">+</button>
      </div>
      <span class="tl-hint">Тяните от правого края задачи к другой, чтобы создать зависимость</span>
      <div class="tl-counters">
        <span v-if="overdueCount" class="tl-counter overdue">{{ overdueCount }} просрочено</span>
        <span v-if="deps.length" class="tl-counter">{{ deps.length }} связей</span>
        <span v-if="unscheduled.length" class="tl-counter">{{ unscheduled.length }} без дат</span>
      </div>
    </div>

    <div ref="scrollEl" class="tl-scroll" :class="{ panning: !!pan, linking: !!link }" @pointerdown="onPanDown" @wheel="onWheel">
      <div class="tl-inner" :class="{ animate: !drag && !link }" :style="{ width: `${LEFT_W + axisW}px` }">
        <!-- header -->
        <div class="tl-head">
          <div class="tl-corner" :style="{ width: `${LEFT_W}px` }">Задача</div>
          <div class="tl-axis">
            <div class="tl-months">
              <div v-for="(m, i) in monthBands" :key="i" class="tl-month" :style="{ width: `${m.span * dayW}px` }">{{ m.label }}</div>
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

        <!-- body: lanes + rows, with the dependency-arrow SVG overlay on top -->
        <div ref="bodyEl" class="tl-body">
          <div class="tl-today" :style="{ left: `${LEFT_W + todayLeft}px` }" />
          <template v-for="lane in lanes" :key="lane.key">
            <div class="tl-lanehead">
              <div class="tl-left lane" :style="{ width: `${LEFT_W}px` }">
                <span class="lane-dot" :style="{ background: lane.color ? hueGrad(lane.color) : 'var(--t-accent-grad)' }" />
                <span class="lane-name">{{ lane.label }}</span>
                <span class="lane-count">{{ lane.tasks.length }}</span>
              </div>
              <div class="tl-track laneband" :style="{ width: `${axisW}px`, '--tl-day-w': `${dayW}px` }" />
            </div>

            <div v-for="t in lane.tasks" :key="t.id" class="tl-row" :data-task-id="t.id">
              <div class="tl-left" :style="{ width: `${LEFT_W}px` }" :title="t.title" @click="$emit('open', t.id)">
                <span class="row-bar" :style="{ background: hueGrad(PRIORITY_COLORS[t.priority || 0]) }" />
                <span class="row-title" :class="{ done: t.completed_at }">{{ t.title }}</span>
              </div>
              <div class="tl-track" :style="{ width: `${axisW}px`, '--tl-day-w': `${dayW}px` }">
                <div
                  class="bar"
                  :class="{ done: t.completed_at, point: !(geom(t).hasStart && geom(t).hasDue), linksrc: link && link.fromId === t.id }"
                  :style="{ left: `${geom(t).left}px`, width: `${geom(t).width}px`, '--bar-grad': hueGrad(PRIORITY_COLORS[t.priority || 0]) }"
                  @pointerdown="onBarDown($event, t, 'move')"
                  @click="$emit('open', t.id)"
                  @contextmenu.prevent.stop="menu.open($event, t)"
                >
                  <span class="handle l" @pointerdown.stop="onBarDown($event, t, 'start')" />
                  <span class="bar-title">{{ t.title }}</span>
                  <span class="handle r" @pointerdown.stop="onBarDown($event, t, 'due')" />
                  <span class="link-knob" title="Создать зависимость" @pointerdown="onLinkDown($event, t)" @click.stop />
                </div>
              </div>
            </div>
          </template>

          <!-- dependency arrows + live link preview -->
          <svg class="g-arrows" :width="LEFT_W + axisW" :height="positions.height" :style="{ height: `${positions.height}px` }">
            <defs>
              <marker id="g-arrowhead" viewBox="0 0 8 8" refX="6.5" refY="4" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
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
            <path v-if="linkPath" class="g-linkpreview" :d="linkPath" marker-end="url(#g-arrowhead)" />
          </svg>
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

/* lanes */
.tl-lanehead {
  display: flex;
}
.tl-left.lane {
  box-sizing: border-box;
  position: sticky;
  left: 0;
  z-index: 5;
  height: 32px;
  display: flex;
  align-items: center;
  gap: 7px;
  background: var(--t-surface-alt, var(--t-hover));
  border-right: 1px solid var(--t-border);
  border-bottom: 1px solid var(--t-border);
  padding: 0 12px;
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
.laneband {
  position: relative;
  background: var(--t-surface-alt, var(--t-hover));
  border-bottom: 1px solid var(--t-border);
  flex: 0 0 auto;
  height: 32px;
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
  background-image: repeating-linear-gradient(
    90deg,
    transparent 0,
    transparent calc(var(--tl-day-w, 34px) - 1px),
    color-mix(in srgb, var(--t-border) 45%, transparent) calc(var(--tl-day-w, 34px) - 1px),
    color-mix(in srgb, var(--t-border) 45%, transparent) var(--tl-day-w, 34px)
  );
}
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
.tl-inner.animate .bar {
  transition: left 0.18s ease, width 0.18s ease;
}
.tl-inner.animate .tl-today {
  transition: left 0.18s ease;
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
.bar.linksrc .link-knob {
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
