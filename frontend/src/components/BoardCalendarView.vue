<script setup>
// «Календарь» (v2, #2973 / #3019; mockups CalendarView, PackageD frame 6,
// CatalogD «День календаря»).
//
// Two modes: the month — a grid of whole weeks, every day a cell on the column
// surface (weekends paler, other months faded), tasks as pills in the soft tone
// of their status, at most three and «+N ещё» for the rest; and the week —
// seven full-height days with every task, the timed ones first. The mode is the
// board's state, saved with a view (`calendar.mode`).
//
// A pill dragged onto another day moves the due there (#3019): a date-only due
// stays date-only, a timed one keeps its time, the start moves along so the span
// stays. Tasks without a due are one click away («N без срока») and can be
// dragged onto a day from there.
//
// Keyboard (desktop): ←→/↑↓ walk the days, ⇧←→ the months (weeks), T today,
// C a task due on the picked day; ↵ steps into the day's tasks (↑↓ between them,
// ↵ opens, ⇧←→ moves the due by a day, E / P / D / A / L / S as on the board,
// Esc back to the day). The cursor and the picked pill show only once the
// keyboard is in use.
//
// Which day a due falls on follows the formatter set (#2932): a timed due in the
// user's profile timezone, a pure UTC midnight (date-only) in UTC everywhere —
// the same day its own due pill on the card prints.
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NDropdown, NIcon, NPopconfirm } from 'naive-ui'
import { AddOutline, ChevronBackOutline, ChevronForwardOutline } from '@vicons/ionicons5'
import { useTaskMenu } from '@/composables/useTaskMenu'
import { useFormat } from '@/composables/useFormat'
import { useResponsive } from '@/composables/useResponsive'
import { useShortcuts } from '@/composables/useShortcuts'
import { useFillHeight } from '@/composables/useFillHeight'
import { capitalizeFirst } from '@/utils/format'
import { isTaskOverdue } from '@/utils/taskFilter'
import { sortedColumns } from '@/utils/status'
import { taskStatus } from '@/utils/columnStatus'
import {
  addDays,
  addMonths,
  dayKey,
  dayOrder,
  isDateOnlyMs,
  monthGrid,
  movedDates,
  pillsShown,
  sameDay,
  weekGrid,
} from '@/utils/calendarView'
import CalendarPill from './calendar/CalendarPill.vue'
import InlineEdit from './kit/InlineEdit.vue'
import ShortcutKeys from './ShortcutKeys.vue'

const props = defineProps({
  tasks: { type: Array, default: () => [] },
  // Real board status columns [{ id, name }] for the "move to column" menu and
  // the pills' status glyph.
  statusColumns: { type: Array, default: () => [] },
  doneColumnId: { type: String, default: null },
  // 'month' | 'week'
  mode: { type: String, default: 'month' },
  // The board's task actions (KanbanBoard): setDates, create, complete,
  // priority, field.
  actions: { type: Object, default: () => ({}) },
  // Archive view (?archived=1): look, don't touch — the right-click menu is the
  // only thing here that writes, and every item on it edits the live task.
  readonly: { type: Boolean, default: false },
})
const emit = defineEmits(['open', 'changed', 'restore', 'update:mode', 'drag-state'])

const { t } = useI18n()
const { isMobile } = useResponsive()

const menu = useTaskMenu({
  onOpen: (id) => emit('open', id),
  onChanged: () => emit('changed'),
  // The archive's own action: this layout has no card, so the menu is the only
  // place «Вернуть из архива» can live (see useTaskMenu).
  onRestore: (id) => emit('restore', id),
  columns: () => props.statusColumns,
  readonly: () => props.readonly,
})

// Month and weekday names are dates, not interface text: they come from Intl in
// the user's language (#2798's formatting layer). The grid cells are calendar
// coordinates rather than instants, so their own labels pass `timeZone: null`.
const { formatDate, formatTime, formatters } = useFormat()
const run = (name, ...args) => props.actions?.[name]?.(...args)

// Where the week starts, as a JS getDay() value — Settings → Локализация, the
// same source the due editor and the Gantt week bands follow.
const firstDow = computed(() => (formatters.value.prefs.weekStart === 0 ? 0 : 1))
const asDate = (p) => new Date(p.year, p.month, p.day)
const weekdays = computed(() =>
  Array.from({ length: 7 }, (_, i) =>
    capitalizeFirst(
      formatDate(new Date(2024, 0, 1 + ((i + firstDow.value + 6) % 7)), {
        weekday: 'short',
        timeZone: null,
      }),
    ),
  ),
)

// The user's today — the calendar day in their timezone preference.
const today = computed(() => {
  const p = formatters.value.today()
  return { year: p.year, month: p.month, day: p.day }
})
// The day the view is built around (the month / week holding it).
const anchor = ref({ ...today.value })

// ── the days on screen ──
const grid = computed(() =>
  props.mode === 'week'
    ? { cells: weekGrid(anchor.value, firstDow.value), rows: 1 }
    : monthGrid(anchor.value, firstDow.value),
)
const cells = computed(() => grid.value.cells)
const title = computed(() => {
  // The month and the year apart: Intl's joined form adds «г.» in Russian.
  if (props.mode !== 'week') {
    const month = formatDate(asDate(anchor.value), { month: 'long', timeZone: null })
    return `${capitalizeFirst(month)} ${anchor.value.year}`
  }
  const a = cells.value[0]
  const b = cells.value[6]
  const dm = (p) => formatDate(asDate(p), { day: 'numeric', month: 'short', timeZone: null })
  return `${dm(a)} – ${dm(b)} ${b.year}`
})

// Tasks by the day the user sees their due on (see the header comment).
const byDay = computed(() => {
  const map = {}
  for (const task of props.tasks) {
    if (!task.due_date) continue
    const p = formatters.value.parts(task.due_date)
    if (p) (map[dayKey(p)] ||= []).push(task)
  }
  for (const k of Object.keys(map)) map[k] = dayOrder(map[k])
  return map
})
const dayTasks = (c) => byDay.value[dayKey(c)] || []
const noDate = computed(() => props.tasks.filter((task) => !task.due_date))

const cols = computed(() => sortedColumns(props.statusColumns))
const statusOf = (task) => taskStatus(task, cols.value, props.doneColumnId)
const overdue = (task) => isTaskOverdue(task, new Date(), formatters.value)
// The time in front of a timed due — in the week and in a day's list, where
// the day reads in time order; the month cell has no room for it.
const timeOf = (task, always = false) => {
  const ms = Date.parse(task.due_date)
  if (Number.isNaN(ms) || isDateOnlyMs(ms)) return ''
  return always || props.mode === 'week' ? formatTime(ms) : ''
}

// ── how many pills a month cell has room for ──
const gridEl = ref(null)
const gridH = ref(0)
let ro = null
onMounted(() => {
  if (typeof ResizeObserver === 'undefined' || !gridEl.value) return
  ro = new ResizeObserver(() => (gridH.value = gridEl.value?.clientHeight || 0))
  ro.observe(gridEl.value)
})
onBeforeUnmount(() => ro?.disconnect())
const cellH = computed(() => {
  const rows = grid.value.rows
  // Not measured yet (or no layout at all): the mockup's cell.
  if (!gridH.value) return 140
  return (gridH.value - (rows - 1) * 6) / rows
})
function shown(c) {
  const list = dayTasks(c)
  if (props.mode === 'week') return list
  return list.slice(0, pillsShown(list.length, cellH.value))
}
const hiddenCount = (c) => dayTasks(c).length - shown(c).length

// ── navigation ──
function shiftPeriod(n) {
  anchor.value = props.mode === 'week' ? addDays(anchor.value, 7 * n) : addMonths(anchor.value, n)
  if (cursor.value) cursor.value = props.mode === 'week' ? addDays(cursor.value, 7 * n) : null
}
function goToday() {
  anchor.value = { ...today.value }
  cursor.value = { ...today.value }
  picked.value = null
}
function setMode(m) {
  if (m !== props.mode) emit('update:mode', m)
}
const inView = (p) => cells.value.some((c) => sameDay(c, p) && c.inMonth)

// ── keyboard: a day cursor, and a picked task inside it ──
const cursor = ref(null)
const picked = ref(null) // task id
const kbd = ref(false)
const hoverKey = ref(null)
function onAnyPointer() {
  kbd.value = false
}
onMounted(() => window.addEventListener('pointerdown', onAnyPointer, true))
onBeforeUnmount(() => window.removeEventListener('pointerdown', onAnyPointer, true))
const cursorTasks = computed(() => (cursor.value ? byDay.value[dayKey(cursor.value)] || [] : []))
const pickedTask = computed(
  () => (picked.value && props.tasks.find((x) => x.id === picked.value)) || null,
)
watch(
  () => props.tasks,
  () => {
    if (picked.value && !pickedTask.value) picked.value = null
  },
)
function moveDay(n) {
  if (!kbd.value && cursor.value) {
    kbd.value = true
    return
  }
  kbd.value = true
  picked.value = null
  const from = cursor.value || today.value
  cursor.value = cursor.value ? addDays(from, n) : { ...from }
  if (!inView(cursor.value)) anchor.value = { ...cursor.value }
}
function onUpDown(dir) {
  if (picked.value && kbd.value) {
    stepPick(dir)
    return
  }
  moveDay(dir * 7)
}
function stepPick(dir) {
  const list = cursorTasks.value
  const i = list.findIndex((x) => x.id === picked.value)
  const next = list[Math.max(0, Math.min(list.length - 1, i + dir))]
  if (next) picked.value = next.id
  revealPick()
}
function enter() {
  kbd.value = true
  if (picked.value) {
    emit('open', picked.value)
    return
  }
  if (!cursor.value) {
    cursor.value = { ...today.value }
    return
  }
  const first = cursorTasks.value[0]
  if (first) {
    picked.value = first.id
    revealPick()
  }
}
function escape() {
  if (dayPop.value) {
    dayPop.value = null
    return
  }
  if (picked.value) picked.value = null
  else {
    cursor.value = null
    kbd.value = false
  }
}
// ⇧←/→: the period — or, on a picked task, its due by a day.
async function shiftOrPeriod(n) {
  const task = pickedTask.value
  if (task && kbd.value && !props.readonly) {
    const from = formatters.value.parts(task.due_date)
    if (!from) return
    const to = addDays({ year: from.year, month: from.month, day: from.day }, n)
    const dates = movedDates(task, to, formatters.value)
    if (!dates) return
    cursor.value = to
    if (!inView(to)) anchor.value = { ...to }
    await run('setDates', task, dates)
    return
  }
  shiftPeriod(n)
}
// A picked task past «+N ещё» is shown in the day's list.
function revealPick() {
  const c = cursor.value
  if (!c || props.mode === 'week') return
  const i = cursorTasks.value.findIndex((x) => x.id === picked.value)
  if (i >= pillsShown(cursorTasks.value.length, cellH.value)) openDay(c)
  else if (dayPop.value && dayKey(dayPop.value.day) !== dayKey(c)) dayPop.value = null
}

const deskOn = () => !isMobile.value && !adding.value
useShortcuts(
  {
    'cal.today': () => {
      goToday()
      kbd.value = true
    },
    'cal.left': () => moveDay(-1),
    'cal.right': () => moveDay(1),
    'cal.up': () => onUpDown(-1),
    'cal.down': () => onUpDown(1),
    'cal.prev': () => shiftOrPeriod(-1),
    'cal.next': () => shiftOrPeriod(1),
    'cal.enter': enter,
    'cal.clear': escape,
  },
  { when: deskOn },
)
const pickOn = () => deskOn() && kbd.value && !!pickedTask.value && !props.readonly
useShortcuts(
  {
    'card.complete': () => run('complete', [pickedTask.value]),
    'card.priority': (v) => run('priority', [pickedTask.value], v),
    'card.priorityPick': () => run('field', pickedTask.value, 'priority'),
    'card.due': () => run('field', pickedTask.value, 'due'),
    'card.assignees': () => run('field', pickedTask.value, 'assignees'),
    'card.tags': () => run('field', pickedTask.value, 'tags'),
    'card.status': () => run('field', pickedTask.value, 'status'),
  },
  { when: pickOn },
)
useShortcuts(
  {
    'board.addTask': () => {
      const hovered = hoverKey.value && cells.value.find((c) => c.key === hoverKey.value)
      startAdd(hovered || cursor.value || today.value)
    },
  },
  { when: () => deskOn() && !props.readonly },
)
const isCursor = (c) => kbd.value && !isMobile.value && sameDay(c, cursor.value)

// ── a click on a pill: open it, and remember it for the keys ──
function onPillClick(task, c) {
  if (suppressClick) return
  cursor.value = { year: c.year, month: c.month, day: c.day }
  picked.value = task.id
  emit('open', task.id)
}

// ── «+N ещё» and the day list (also the phone's tap on a day) ──
const dayPop = ref(null) // { day, x, y, w }
const rootEl = ref(null)
const { height: fillH } = useFillHeight(rootEl, { bottom: 16, min: 420 })
function openDay(c, el) {
  const cell = el || rootEl.value?.querySelector(`[data-day="${c.key}"]`)
  const host = rootEl.value?.getBoundingClientRect()
  const r = cell?.getBoundingClientRect()
  if (!host || !r) {
    dayPop.value = { day: c, x: 0, y: 0, w: 240 }
    return
  }
  const w = Math.max(r.width + 24, 240)
  dayPop.value = {
    day: c,
    x: Math.min(Math.max(0, r.left - host.left - 12), host.width - w),
    y: r.top - host.top - 6,
    w,
  }
}
const dayPopTitle = computed(() =>
  dayPop.value
    ? capitalizeFirst(
        formatDate(asDate(dayPop.value.day), {
          weekday: 'long',
          day: 'numeric',
          month: 'long',
          timeZone: null,
        }),
      )
    : '',
)
function onCellClick(c, e) {
  if (isMobile.value && dayTasks(c).length) openDay(c, e.currentTarget)
}

// ── the undated tasks ──
const undatedOpen = ref(false)
function onDocDown(e) {
  if (drag.value) return
  if (undatedOpen.value && !e.target.closest?.('.cal-undated, .cal-undated-btn')) {
    undatedOpen.value = false
  }
  if (dayPop.value && !e.target.closest?.('.cal-daypop, .cal-more')) dayPop.value = null
}
onMounted(() => document.addEventListener('pointerdown', onDocDown))
onBeforeUnmount(() => document.removeEventListener('pointerdown', onDocDown))

// ── a new task due on a day (C, the cell's «＋») ──
const adding = ref(null) // day key
const addDraft = ref('')
let addDay = null
function startAdd(c) {
  if (props.readonly || !c) return
  const day = { year: c.year, month: c.month, day: c.day }
  if (!inView(day)) anchor.value = { ...day }
  addDay = day
  addDraft.value = ''
  adding.value = dayKey(day)
}
async function commitAdd(value, info) {
  const title = (value || '').trim()
  const day = addDay
  if (!title || !day) {
    adding.value = null
    return
  }
  addDraft.value = ''
  if (info?.via !== 'enter') adding.value = null
  await run('create', day, title)
}

// ── drag a pill onto another day (#3019) ──
const drag = ref(null) // { task, x, y, w }
const dropKey = ref(null)
let pending = null
let suppressClick = false
function onPillDown(e, task) {
  if (props.readonly || e.button !== 0 || e.pointerType === 'touch') return
  const r = e.currentTarget.getBoundingClientRect()
  pending = {
    task,
    x0: e.clientX,
    y0: e.clientY,
    dx: e.clientX - r.left,
    dy: e.clientY - r.top,
    w: r.width,
  }
  window.addEventListener('pointermove', onDragMove)
  window.addEventListener('pointerup', onDragUp)
}
function onDragMove(e) {
  if (!pending) return
  if (!drag.value) {
    if (Math.hypot(e.clientX - pending.x0, e.clientY - pending.y0) < 5) return
    drag.value = { task: pending.task, w: pending.w, x: 0, y: 0 }
    emit('drag-state', true)
  }
  drag.value.x = e.clientX - pending.dx
  drag.value.y = e.clientY - pending.dy
  const el = document.elementFromPoint?.(e.clientX, e.clientY)?.closest?.('[data-day]')
  dropKey.value = el?.getAttribute('data-day') || null
}
async function onDragUp() {
  window.removeEventListener('pointermove', onDragMove)
  window.removeEventListener('pointerup', onDragUp)
  const d = drag.value
  pending = null
  if (!d) return
  suppressClick = true
  setTimeout(() => (suppressClick = false))
  drag.value = null
  emit('drag-state', false)
  const key = dropKey.value
  dropKey.value = null
  const to = key && cells.value.find((c) => c.key === key)
  if (!to) return
  const dates = movedDates(
    d.task,
    { year: to.year, month: to.month, day: to.day },
    formatters.value,
  )
  if (!dates) return
  undatedOpen.value = false
  dayPop.value = null
  await run('setDates', d.task, dates)
}
onBeforeUnmount(() => {
  window.removeEventListener('pointermove', onDragMove)
  window.removeEventListener('pointerup', onDragUp)
  if (drag.value) emit('drag-state', false)
})
const dropCaption = computed(() => {
  const c = dropKey.value && cells.value.find((x) => x.key === dropKey.value)
  return c
    ? t('board.calendar.dropTo', {
        date: formatDate(asDate(c), { day: 'numeric', month: 'short', timeZone: null }),
      })
    : ''
})

const dayLabel = (c) =>
  c.day === 1 ? formatDate(asDate(c), { month: 'short', timeZone: null }) : ''
</script>

<template>
  <div
    ref="rootEl"
    :style="fillH ? { height: `${fillH}px` } : null"
    class="cal"
    :class="{ mobile: isMobile, week: mode === 'week', dragging: !!drag }"
    data-testid="calendar-view"
  >
    <div class="cal-bar">
      <span class="cal-title">{{ title }}</span>
      <span class="cal-nav">
        <button
          type="button"
          class="cal-icon"
          :title="t(mode === 'week' ? 'board.calendar.prevWeek' : 'board.calendar.prevMonth')"
          @click="shiftPeriod(-1)"
        >
          <n-icon :component="ChevronBackOutline" :size="15" />
        </button>
        <button
          type="button"
          class="cal-icon"
          :title="t(mode === 'week' ? 'board.calendar.nextWeek' : 'board.calendar.nextMonth')"
          @click="shiftPeriod(1)"
        >
          <n-icon :component="ChevronForwardOutline" :size="15" />
        </button>
      </span>
      <button type="button" class="cal-today" data-testid="calendar-today" @click="goToday">
        {{ t('board.calendar.today') }}
        <ShortcutKeys v-if="!isMobile" id="cal.today" />
      </button>
      <span class="cal-space" />
      <span v-if="!isMobile" class="cal-meta">{{ t('board.calendar.byDue') }}</span>
      <button
        v-if="noDate.length"
        type="button"
        class="cal-undated-btn"
        :class="{ sep: !isMobile }"
        data-testid="calendar-undated"
        @click="undatedOpen = !undatedOpen"
      >
        {{ t('board.calendar.undated', noDate.length, { named: { n: noDate.length } }) }}
      </button>
      <div class="cal-seg" role="tablist">
        <button
          v-for="m in ['month', 'week']"
          :key="m"
          type="button"
          role="tab"
          class="cal-seg-btn"
          :class="{ on: mode === m }"
          :aria-selected="mode === m"
          :data-testid="`calendar-mode-${m}`"
          @click="setMode(m)"
        >
          {{ t(`board.calendar.mode.${m}`) }}
        </button>
      </div>

      <div v-if="undatedOpen" class="cal-pop cal-undated" data-testid="calendar-undated-list">
        <div class="cal-pop-head">
          {{ t('board.calendar.noDue') }}
          <span class="cal-pop-note">{{ readonly ? '' : t('board.calendar.dragHint') }}</span>
        </div>
        <div class="cal-pop-list">
          <CalendarPill
            v-for="task in noDate"
            :key="task.id"
            class="cal-chip"
            :task="task"
            :status="statusOf(task)"
            :ghost="drag?.task.id === task.id"
            @pointerdown="onPillDown($event, task)"
            @click="!suppressClick && $emit('open', task.id)"
            @contextmenu.prevent.stop="menu.open($event, task)"
          />
        </div>
      </div>
    </div>

    <div class="cal-wds">
      <div v-for="w in weekdays" :key="w" class="cal-wd">{{ w }}</div>
    </div>

    <div
      ref="gridEl"
      class="cal-grid"
      :style="{ gridTemplateRows: `repeat(${grid.rows}, minmax(0, 1fr))` }"
    >
      <div
        v-for="c in cells"
        :key="c.key"
        class="cal-cell"
        :class="{
          out: !c.inMonth,
          weekend: c.weekend,
          today: sameDay(c, today),
          cursor: isCursor(c),
          drop: dropKey === c.key,
        }"
        :data-day="c.key"
        data-testid="calendar-day"
        @mouseenter="hoverKey = c.key"
        @mouseleave="hoverKey === c.key && (hoverKey = null)"
        @click="onCellClick(c, $event)"
      >
        <span v-if="sameDay(c, today)" class="cal-todayline" />
        <div class="cal-dayrow">
          <span class="cal-daynum">{{ c.day }}</span>
          <span v-if="dayLabel(c)" class="cal-mon">{{ dayLabel(c) }}</span>
          <span v-if="sameDay(c, today) && !isMobile" class="cal-todaytxt accent-grad-text">{{
            t('board.calendar.todayMark')
          }}</span>
          <span class="cal-space" />
          <ShortcutKeys v-if="isCursor(c) && !readonly && !picked" id="cal.add" />
          <button
            v-else-if="!readonly && !isMobile"
            type="button"
            class="cal-add"
            :title="t('board.calendar.addTask')"
            data-testid="calendar-day-add"
            @click.stop="startAdd(c)"
          >
            <n-icon :component="AddOutline" :size="13" />
          </button>
        </div>

        <template v-if="isMobile && mode === 'month'">
          <span v-if="dayTasks(c).length" class="cal-dots">
            <span
              v-for="task in dayTasks(c).slice(0, 4)"
              :key="task.id"
              class="cal-dot"
              :style="{ background: statusOf(task).color || '#9aa0aa' }"
            />
          </span>
        </template>
        <div v-else class="cal-pills">
          <CalendarPill
            v-for="task in shown(c)"
            :key="task.id"
            class="cal-chip"
            :task="task"
            :status="statusOf(task)"
            :overdue="overdue(task)"
            :time="timeOf(task)"
            :picked="kbd && picked === task.id"
            :ghost="drag?.task.id === task.id"
            @pointerdown="onPillDown($event, task)"
            @click.stop="onPillClick(task, c)"
            @contextmenu.prevent.stop="menu.open($event, task)"
          />
          <button
            v-if="hiddenCount(c) > 0"
            type="button"
            class="cal-more"
            data-testid="calendar-more"
            @click.stop="openDay(c)"
          >
            {{ t('board.calendar.more', { n: hiddenCount(c) }) }}
          </button>
        </div>

        <div v-if="adding === c.key" class="cal-addrow" @click.stop>
          <InlineEdit
            v-model="addDraft"
            hint="create"
            :placeholder="t('board.column.taskPlaceholder')"
            data-testid="calendar-add-input"
            @commit="commitAdd"
            @cancel="adding = null"
          />
        </div>
        <div v-if="dropKey === c.key" class="cal-dropcap">{{ dropCaption }}</div>
      </div>
    </div>

    <div
      v-if="dayPop"
      class="cal-pop cal-daypop"
      :style="{ left: `${dayPop.x}px`, top: `${dayPop.y}px`, width: `${dayPop.w}px` }"
      data-testid="calendar-day-list"
    >
      <div class="cal-pop-head">{{ dayPopTitle }}</div>
      <div class="cal-pop-list">
        <CalendarPill
          v-for="task in dayTasks(dayPop.day)"
          :key="task.id"
          class="cal-chip"
          :task="task"
          :status="statusOf(task)"
          :overdue="overdue(task)"
          :time="timeOf(task, true)"
          :picked="kbd && picked === task.id"
          :ghost="drag?.task.id === task.id"
          @pointerdown="onPillDown($event, task)"
          @click.stop="onPillClick(task, dayPop.day)"
          @contextmenu.prevent.stop="menu.open($event, task)"
        />
      </div>
    </div>

    <CalendarPill
      v-if="drag"
      class="cal-flying"
      flying
      :task="drag.task"
      :status="statusOf(drag.task)"
      :style="{ left: `${drag.x}px`, top: `${drag.y}px`, width: `${drag.w}px` }"
    />

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
      :positive-text="t('task.confirm.deleteYes')"
      @positive-click="menu.confirmDelete()"
      @clickoutside="menu.deleteConfirmShow.value = false"
    >
      <template #trigger><span /></template>
      {{ t('task.confirm.delete') }}
    </n-popconfirm>
    <n-popconfirm
      v-model:show="menu.archiveConfirmShow.value"
      :x="menu.x.value"
      :y="menu.y.value"
      :positive-text="t('task.confirm.archiveYes')"
      @positive-click="menu.confirmArchive()"
      @clickoutside="menu.archiveConfirmShow.value = false"
    >
      <template #trigger><span /></template>
      {{ t('task.confirm.archive') }}
    </n-popconfirm>
  </div>
</template>

<style scoped>
.cal {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: 8px;
  height: calc(100vh - 150px);
  height: calc(100dvh - 150px);
  min-height: 480px;
  padding: 0 2px 8px;
  box-sizing: border-box;
}
.cal.dragging,
.cal.dragging * {
  cursor: grabbing;
}
.cal.dragging .cal-add {
  display: none;
}
.cal-bar {
  position: relative;
  display: flex;
  align-items: center;
  gap: 10px;
  height: 36px;
  flex: none;
}
.cal-title {
  font-size: 17px;
  font-weight: 700;
  color: var(--t-text1);
  white-space: nowrap;
}
.cal-nav {
  display: flex;
  gap: 2px;
}
.cal-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  padding: 0;
  border: none;
  border-radius: 8px;
  background: transparent;
  color: var(--t-text2);
  cursor: pointer;
}
.cal-icon:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.cal-today {
  display: flex;
  align-items: center;
  gap: 6px;
  height: 28px;
  padding: 0 5px 0 9px;
  border: none;
  border-radius: 8px;
  background: var(--t-ctl);
  color: var(--t-text1);
  font: inherit;
  font-size: 12.5px;
  cursor: pointer;
}
.cal-today:hover {
  background: var(--t-hover);
}
.cal-space {
  flex: 1;
}
.cal-meta {
  font-size: 12.5px;
  color: var(--t-text3);
  white-space: nowrap;
}
.cal-undated-btn {
  margin-left: -6px;
  padding: 0;
  border: none;
  background: transparent;
  color: var(--t-text3);
  font: inherit;
  font-size: 12.5px;
  cursor: pointer;
  text-decoration: underline dotted;
  text-underline-offset: 3px;
}
.cal-undated-btn.sep::before {
  content: '·';
  display: inline-block;
  margin-right: 4px;
  text-decoration: none;
}
.cal-undated-btn:hover {
  color: var(--t-text1);
}
.cal-seg {
  display: flex;
  gap: 2px;
  padding: 3px;
  border-radius: 8px;
  background: var(--t-ctl);
}
.cal-seg-btn {
  height: 24px;
  padding: 0 10px;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: var(--t-text2);
  font: inherit;
  font-size: 12.5px;
  cursor: pointer;
}
.cal-seg-btn.on {
  background: var(--t-card);
  box-shadow: var(--t-sh-card);
  color: var(--t-text1);
  font-weight: 600;
}
.cal-wds {
  display: grid;
  grid-template-columns: repeat(7, minmax(0, 1fr));
  gap: 6px;
  padding: 0 8px;
  flex: none;
}
.cal-wd {
  font-size: 11.5px;
  font-weight: 600;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  color: var(--t-text3);
}
.cal-grid {
  flex: 1;
  min-height: 0;
  display: grid;
  grid-template-columns: repeat(7, minmax(0, 1fr));
  gap: 6px;
}
.cal-cell {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: 3px;
  min-width: 0;
  min-height: 0;
  padding: 6px;
  box-sizing: border-box;
  border-radius: 10px;
  background: var(--t-col);
  overflow: hidden;
}
.cal-cell.weekend {
  background: color-mix(in srgb, var(--t-col) 55%, transparent);
}
/* Another month: paler surface and text (not `opacity` on the cell — that made
   the whole cell a compositing layer of its own, and the hover tint went under). */
.cal-cell.out {
  background: color-mix(in srgb, var(--t-col) 50%, transparent);
}
.cal-cell.out .cal-daynum,
.cal-cell.out .cal-mon {
  color: var(--t-placeholder);
}
.cal-cell.out .cal-pills {
  opacity: 0.55;
}
.cal-cell:hover {
  background: color-mix(in srgb, var(--t-hover) 80%, var(--t-col));
}
.cal-cell.cursor {
  outline: 2px solid var(--t-primary);
  outline-offset: -2px;
}
.cal-cell.drop {
  background: var(--t-drop);
  box-shadow: var(--t-sh-drop);
}
.cal-todayline {
  position: absolute;
  left: 10px;
  right: 10px;
  top: 0;
  height: 2px;
  border-radius: 1px;
  background: var(--t-accent-grad);
}
.cal-dayrow {
  display: flex;
  align-items: center;
  gap: 4px;
  height: 22px;
  padding: 0 2px;
  flex: none;
}
.cal-daynum {
  font-size: 12.5px;
  font-weight: 500;
  color: var(--t-text2);
}
.cal-cell.weekend .cal-daynum {
  color: var(--t-text3);
}
.cal-mon {
  font-size: 12.5px;
  font-weight: 650;
  color: var(--t-text2);
}
.cal-cell:has(.cal-mon) .cal-daynum {
  font-weight: 650;
}
.cal-cell.today .cal-daynum {
  min-width: 22px;
  height: 22px;
  padding: 0 6px;
  box-sizing: border-box;
  border-radius: 11px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--t-accent-grad);
  color: #fff;
  font-weight: 700;
}
.cal-todaytxt {
  margin-left: 2px;
  font-size: 11.5px;
  font-weight: 600;
}
.cal-add {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  padding: 0;
  border: none;
  border-radius: 6px;
  background: var(--t-pill);
  color: var(--t-text2);
  cursor: pointer;
  opacity: 0;
}
.cal-cell:hover .cal-add,
.cal-add:focus-visible {
  opacity: 1;
}
.cal-pills {
  display: flex;
  flex-direction: column;
  gap: 3px;
  min-height: 0;
}
.cal.week .cal-pills {
  overflow-y: auto;
  scrollbar-width: thin;
}
.cal-more {
  height: 20px;
  padding: 0 6px;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: var(--t-text2);
  font: inherit;
  font-size: 12px;
  font-weight: 600;
  text-align: left;
  cursor: pointer;
}
.cal-more:hover {
  background: var(--t-hover);
}
.cal-addrow {
  margin-top: 2px;
}
.cal-dropcap {
  position: absolute;
  left: 6px;
  bottom: 6px;
  font-size: 11px;
  color: var(--t-text2);
  pointer-events: none;
}
.cal-flying {
  position: fixed;
  z-index: 3000;
  pointer-events: none;
}

/* The day list («+N ещё», the phone's tap) and the undated tasks: a popover. */
.cal-pop {
  position: absolute;
  z-index: 20;
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 10px;
  border-radius: 16px;
  background: var(--t-pop);
  box-shadow: var(--t-sh-pop);
}
.cal-undated {
  top: 40px;
  right: 120px;
  width: 300px;
}
.cal-pop-head {
  display: flex;
  align-items: baseline;
  gap: 8px;
  padding: 0 2px;
  font-size: 13px;
  font-weight: 650;
  color: var(--t-text1);
}
.cal-pop-note {
  font-size: 11.5px;
  font-weight: 400;
  color: var(--t-text3);
}
.cal-pop-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
  max-height: 320px;
  overflow-y: auto;
  scrollbar-width: thin;
}

/* ── phone: the month as dots, a tap opens the day ── */
.cal.mobile {
  height: calc(var(--app-vh, 100vh) - var(--m-head-h, 52px) - var(--mnav-h, 0px) - 88px);
  min-height: 420px;
}
.cal.mobile .cal-bar {
  flex-wrap: wrap;
  height: auto;
  row-gap: 6px;
}
.cal.mobile .cal-title {
  font-size: 16px;
}
.cal.mobile .cal-grid {
  gap: 4px;
}
.cal.mobile .cal-cell {
  padding: 4px;
  border-radius: 8px;
}
.cal.mobile .cal-wds {
  gap: 4px;
  padding: 0 4px;
}
.cal-dots {
  display: flex;
  flex-wrap: wrap;
  gap: 3px;
  padding: 2px;
}
.cal-dot {
  width: 6px;
  height: 6px;
  border-radius: 3px;
}
.cal.mobile .cal-mon,
.cal.mobile .cal-meta {
  display: none;
}
</style>
