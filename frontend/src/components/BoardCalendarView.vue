<script setup>
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NButtonGroup, NDropdown, NPopconfirm } from 'naive-ui'
import { useTaskMenu } from '@/composables/useTaskMenu'
import { useFormat } from '@/composables/useFormat'
import { capitalizeFirst } from '@/utils/format'
import { PRIORITY_COLORS } from '@/styles/tokens'

const props = defineProps({
  tasks: { type: Array, default: () => [] },
  // Real board status columns [{ id, name }] for the "move to column" menu.
  statusColumns: { type: Array, default: () => [] },
  // Archive view (?archived=1): look, don't touch — the right-click menu is the
  // only thing here that writes, and every item on it edits the live task.
  readonly: { type: Boolean, default: false },
})
const emit = defineEmits(['open', 'changed'])

const { t } = useI18n()

const menu = useTaskMenu({
  onOpen: (id) => emit('open', id),
  onChanged: () => emit('changed'),
  columns: () => props.statusColumns,
  readonly: () => props.readonly,
})

// Month and weekday names are dates, not interface text: they come from Intl in
// the user's language (#2798's formatting layer) rather than from the locale
// files — twelve more hand-written names per language is exactly the duplication
// that layer exists to avoid. The grid cells are calendar coordinates rather
// than instants, so their own labels pass `timeZone: null` — a cell built on
// 1 March must read "March" whatever zone the reader is in. What goes INTO a
// cell is a different matter; see `dueKey`.
const { formatDate, formatters } = useFormat()

// Where the week starts, as a JS getDay() value. Settings → Локализация offers
// the choice, and the calendar in the due editor, the Naive pickers and the
// Gantt week bands all follow it — a month grid that didn't would be a week out
// of step with the picker the user opens from the same card.
const firstDow = computed(() => (formatters.value.prefs.weekStart === 0 ? 0 : 1))

// 1 Jan 2024 was a Monday — any week would do, only the weekday names are read
// off this one, rotated to start on the user's first day.
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

// The user's today — the calendar day in their timezone preference, not the
// browser's. `new Date()` was both the highlighted cell and the month the view
// opens on, so a profile zone a day ahead of the device (or behind it) lit the
// neighbouring cell at the edges of the day.
const todayParts = computed(() => formatters.value.today())
// Cursor points at the first day of the displayed month.
const cursor = ref(new Date(todayParts.value.year, todayParts.value.month, 1))

const monthLabel = computed(() =>
  capitalizeFirst(formatDate(cursor.value, { month: 'long', year: 'numeric', timeZone: null })),
)

// The grid cells are calendar coordinates, built from local fields — so their
// key is read off the same fields. Month is 0-based here to match the `parts()`
// shape below, which is what a due date's key is built from.
function cellKey(d) {
  return `${d.getFullYear()}-${d.getMonth()}-${d.getDate()}`
}

// Which day a DUE DATE belongs to is not the same question, and it was answered
// here with the browser's calendar (#2932). It has two rules, both of which the
// rest of the app already follows through the formatter set (utils/taskFilter
// `dueDayOf`, the due pill on the card, the «Сегодня»/«Просрочено» facets):
//
//   • a due carrying a time of day is read in the user's TIMEZONE preference;
//   • a pure UTC midnight is a date-only value (a GitLab issue due, an all-day
//     due) and is read in UTC in every zone, or it slides to the previous day
//     for every reader west of it.
//
// Reading them off `new Date(...)` instead put the very same task on one day in
// the calendar layout and on another in the due pill of its own chip.
function dueKey(value) {
  const p = formatters.value.parts(value)
  return p ? `${p.year}-${p.month}-${p.day}` : ''
}

// Index tasks that have a due date by the day the user sees it on.
const tasksByDay = computed(() => {
  const map = {}
  for (const task of props.tasks) {
    if (!task.due_date) continue
    ;(map[dueKey(task.due_date)] ||= []).push(task)
  }
  return map
})

const todayKey = computed(
  () => `${todayParts.value.year}-${todayParts.value.month}-${todayParts.value.day}`,
)

const noDate = computed(() => props.tasks.filter((task) => !task.due_date))

// 6 weeks × 7 days covering the displayed month, starting on the user's first
// day of the week (same source as the header row above).
const weeks = computed(() => {
  const first = cursor.value
  const offset = (first.getDay() - firstDow.value + 7) % 7
  const start = new Date(first.getFullYear(), first.getMonth(), 1 - offset)
  const cells = []
  for (let i = 0; i < 42; i++) {
    const d = new Date(start.getFullYear(), start.getMonth(), start.getDate() + i)
    cells.push({
      date: d,
      inMonth: d.getMonth() === first.getMonth(),
      isToday: cellKey(d) === todayKey.value,
      tasks: tasksByDay.value[cellKey(d)] || [],
    })
  }
  const rows = []
  for (let i = 0; i < 42; i += 7) rows.push(cells.slice(i, i + 7))
  return rows
})

function shift(months) {
  cursor.value = new Date(cursor.value.getFullYear(), cursor.value.getMonth() + months, 1)
}
function goToday() {
  cursor.value = new Date(todayParts.value.year, todayParts.value.month, 1)
}
</script>

<template>
  <div class="cal">
    <div class="cal-bar">
      <n-button-group size="small">
        <n-button @click="shift(-1)">‹</n-button>
        <n-button @click="goToday">{{ t('board.calendar.today') }}</n-button>
        <n-button @click="shift(1)">›</n-button>
      </n-button-group>
      <span class="cal-month">{{ monthLabel }}</span>
    </div>

    <div class="cal-grid">
      <div v-for="w in weekdays" :key="w" class="cal-wd">{{ w }}</div>
      <template v-for="(row, ri) in weeks" :key="ri">
        <div
          v-for="cell in row"
          :key="cell.date.toISOString()"
          class="cal-cell"
          :class="{ out: !cell.inMonth, today: cell.isToday }"
        >
          <div class="cal-daynum">{{ cell.date.getDate() }}</div>
          <button
            v-for="task in cell.tasks"
            :key="task.id"
            type="button"
            class="cal-chip"
            :class="{ done: task.completed_at }"
            :style="{ '--chip': PRIORITY_COLORS[task.priority || 0] }"
            :title="task.title"
            @click="$emit('open', task.id)"
            @contextmenu.prevent.stop="menu.open($event, task)"
          >
            {{ task.title }}
          </button>
        </div>
      </template>
    </div>

    <div v-if="noDate.length" class="cal-nodate">
      <span class="nd-label">{{ t('board.calendar.noDue') }}</span>
      <button
        v-for="task in noDate"
        :key="task.id"
        type="button"
        class="cal-chip"
        :class="{ done: task.completed_at }"
        :style="{ '--chip': PRIORITY_COLORS[task.priority || 0] }"
        :title="task.title"
        @click="$emit('open', task.id)"
        @contextmenu.prevent.stop="menu.open($event, task)"
      >
        {{ task.title }}
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
  padding: 4px 2px 40px;
}
.cal-bar {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 10px;
}
.cal-month {
  font-weight: 600;
  font-size: 16px;
  color: var(--t-text1);
}
.cal-grid {
  display: grid;
  /* minmax(0, 1fr) lets columns shrink below their content so a long task
     title doesn't blow the grid past the screen (it gets clipped instead). */
  grid-template-columns: repeat(7, minmax(0, 1fr));
  gap: 1px;
  background: var(--t-border);
  border: 1px solid var(--t-border);
  border-radius: 8px;
  overflow: hidden;
}
.cal-wd {
  background: var(--t-surface-alt, var(--t-hover));
  padding: 6px 8px;
  font-size: 12px;
  font-weight: 600;
  color: var(--t-text3);
  text-align: center;
}
.cal-cell {
  background: var(--t-surface);
  min-width: 0;
  min-height: 96px;
  padding: 4px;
  display: flex;
  flex-direction: column;
  gap: 3px;
}
.cal-cell.out {
  background: var(--t-surface-alt, var(--t-hover));
}
.cal-cell.out .cal-daynum {
  color: var(--t-text3);
}
.cal-cell.today .cal-daynum {
  background: var(--t-accent-grad);
  color: #fff;
}
.cal-daynum {
  align-self: flex-start;
  font-size: 12px;
  min-width: 20px;
  height: 20px;
  line-height: 20px;
  text-align: center;
  border-radius: 50%;
  color: var(--t-text2);
}
.cal-chip {
  display: block;
  width: 100%;
  text-align: left;
  border: none;
  border-left: 3px solid var(--chip, var(--t-primary));
  border-radius: 4px;
  background: var(--t-hover);
  color: var(--t-text1);
  font-size: 12px;
  padding: 2px 6px;
  cursor: pointer;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.cal-chip:hover {
  filter: brightness(0.96);
}
.cal-chip.done {
  text-decoration: line-through;
  color: var(--t-text3);
}
.cal-nodate {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  margin-top: 14px;
}
.nd-label {
  font-size: 12px;
  font-weight: 600;
  color: var(--t-text3);
  margin-right: 4px;
}
.cal-nodate .cal-chip {
  width: auto;
  max-width: 220px;
}
</style>
