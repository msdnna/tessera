<script setup>
import { ref, computed, onBeforeUnmount, nextTick, watch } from 'vue'
import { NSelect, NDropdown, NPopconfirm } from 'naive-ui'
import { useTaskMenu } from '@/composables/useTaskMenu'
import { tasks as tasksApi } from '@/api'
import { PRIORITY_COLORS } from '@/styles/tokens'
import { hueGrad } from '@/utils/gradient'

const props = defineProps({
  tasks: { type: Array, default: () => [] },
  // Real board status columns [{ id, name }] for the context menu + grouping.
  statusColumns: { type: Array, default: () => [] },
  membersMap: { type: Object, default: () => ({}) },
  tagsMap: { type: Object, default: () => ({}) },
})
const emit = defineEmits(['open', 'changed'])

const menu = useTaskMenu({
  onOpen: (id) => emit('open', id),
  onChanged: () => emit('changed'),
  columns: () => props.statusColumns,
})

const DAY_W = 34 // px per day on the axis
const DAY_MS = 86400000
const LEFT_W = 224 // px of the fixed task/lane column

// ── group-by (swimlanes) ──
const groupBy = ref('assignee')
const GROUP_OPTS = [
  { label: 'По исполнителю', value: 'assignee' },
  { label: 'По тегу', value: 'tag' },
  { label: 'По статусу', value: 'status' },
  { label: 'Без группировки', value: 'none' },
]

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

// ── axis range: covers every scheduled task + today, padded a few days ──
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
const axisW = computed(() => range.value.days * DAY_W)
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

// ── lanes (swimlanes) ──
const lanes = computed(() => {
  const mode = groupBy.value
  const buckets = new Map()
  const ensure = (key, label, color) => {
    if (!buckets.has(key)) buckets.set(key, { key, label, color, tasks: [] })
    return buckets.get(key)
  }
  for (const t of scheduled.value) {
    if (mode === 'assignee') {
      const id = (t.assignee_ids || [])[0]
      const m = id ? props.membersMap[id] : null
      ensure(id || '∅', m?.name || 'Не назначено').tasks.push(t)
    } else if (mode === 'tag') {
      const id = (t.tag_ids || [])[0]
      const tag = id ? props.tagsMap[id] : null
      ensure(id || '∅', tag?.name || 'Без тега', tag?.color).tasks.push(t)
    } else if (mode === 'status') {
      const col = props.statusColumns.find((c) => c.id === t.column_id)
      ensure(t.column_id || '∅', col?.name || '—').tasks.push(t)
    } else {
      ensure('all', 'Все задачи').tasks.push(t)
    }
  }
  const arr = [...buckets.values()]
  // Sort each lane's tasks by start, then keep "unassigned/empty" lanes last.
  for (const l of arr) l.tasks.sort((x, y) => spanOf(x).a - spanOf(y).a)
  arr.sort((a, b) => (a.key === '∅' ? 1 : 0) - (b.key === '∅' ? 1 : 0))
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
  const delta = Math.round((e.clientX - g.startX) / DAY_W)
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
    left: i0 * DAY_W,
    width: Math.max(1, i1 - i0 + 1) * DAY_W,
    hasStart: s != null,
    hasDue: d != null,
  }
}
const todayLeft = computed(() => dayIndex(todayMs) * DAY_W + DAY_W / 2)

// ── scroll-to-today ──
const scrollEl = ref(null)
function centerToday() {
  const el = scrollEl.value
  if (!el) return
  el.scrollLeft = Math.max(0, dayIndex(todayMs) * DAY_W - el.clientWidth / 2 + LEFT_W)
}
watch(scrollEl, (el) => el && nextTick(centerToday))
</script>

<template>
  <div class="tl">
    <div class="tl-toolbar">
      <n-select
        size="small"
        :value="groupBy"
        :options="GROUP_OPTS"
        style="width: 190px"
        @update:value="(v) => (groupBy = v)"
      />
      <button class="tl-today-btn" type="button" @click="centerToday">Сегодня</button>
      <div class="tl-counters">
        <span v-if="overdueCount" class="tl-counter overdue">{{ overdueCount }} просрочено</span>
        <span v-if="unscheduled.length" class="tl-counter">{{ unscheduled.length }} без дат</span>
      </div>
    </div>

    <div ref="scrollEl" class="tl-scroll">
      <div class="tl-inner" :style="{ width: `${LEFT_W + axisW}px` }">
        <!-- header: month band + day band -->
        <div class="tl-head">
          <div class="tl-corner" :style="{ width: `${LEFT_W}px` }">Задача</div>
          <div class="tl-axis">
            <div class="tl-months">
              <div
                v-for="(m, i) in monthBands"
                :key="i"
                class="tl-month"
                :style="{ width: `${m.span * DAY_W}px` }"
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
                :style="{ width: `${DAY_W}px` }"
              >
                <span class="dh-num">{{ d.day }}</span>
                <span class="dh-wd">{{ d.dow }}</span>
              </div>
            </div>
          </div>
        </div>

        <!-- swimlanes -->
        <template v-for="lane in lanes" :key="lane.key">
          <div class="tl-lanehead">
            <div class="tl-left lane" :style="{ width: `${LEFT_W}px` }">
              <span
                class="lane-dot"
                :style="{ background: lane.color ? hueGrad(lane.color) : 'var(--t-accent-grad)' }"
              />
              <span class="lane-name">{{ lane.label }}</span>
              <span class="lane-count">{{ lane.tasks.length }}</span>
            </div>
            <div class="tl-track laneband" :style="{ width: `${axisW}px` }">
              <div class="today-line" :style="{ left: `${todayLeft}px` }" />
            </div>
          </div>

          <div v-for="t in lane.tasks" :key="t.id" class="tl-row">
            <div class="tl-left" :style="{ width: `${LEFT_W}px` }" :title="t.title" @click="$emit('open', t.id)">
              <span class="row-bar" :style="{ background: hueGrad(PRIORITY_COLORS[t.priority || 0]) }" />
              <span class="row-title" :class="{ done: t.completed_at }">{{ t.title }}</span>
            </div>
            <div class="tl-track" :style="{ width: `${axisW}px` }">
              <div class="today-line" :style="{ left: `${todayLeft}px` }" />
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
                @contextmenu.prevent.stop="menu.open($event, t)"
              >
                <span class="handle l" @pointerdown.stop="onBarDown($event, t, 'start')" />
                <span class="bar-title">{{ t.title }}</span>
                <span class="handle r" @pointerdown.stop="onBarDown($event, t, 'due')" />
              </div>
            </div>
          </div>
        </template>

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
.tl-inner {
  position: relative;
}

/* header */
.tl-head {
  display: flex;
  position: sticky;
  top: 0;
  z-index: 5;
}
.tl-corner {
  position: sticky;
  left: 0;
  z-index: 6;
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
  flex: 0 0 auto;
  font-size: 12px;
  font-weight: 600;
  color: var(--t-text2);
  padding: 3px 8px;
  white-space: nowrap;
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
  position: sticky;
  left: 0;
  z-index: 3;
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
  position: sticky;
  left: 0;
  z-index: 2;
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
  /* faint day gridlines */
  background-image: repeating-linear-gradient(
    90deg,
    transparent 0,
    transparent calc(34px - 1px),
    color-mix(in srgb, var(--t-border) 45%, transparent) calc(34px - 1px),
    color-mix(in srgb, var(--t-border) 45%, transparent) 34px
  );
}
.today-line {
  position: absolute;
  top: 0;
  bottom: 0;
  width: 2px;
  background: color-mix(in srgb, var(--t-primary) 70%, transparent);
  z-index: 1;
  pointer-events: none;
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
</style>
