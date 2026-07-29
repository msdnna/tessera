<script setup>
import { ref, computed } from 'vue'
import { NButton, NButtonGroup, NDropdown, NPopconfirm } from 'naive-ui'
import { useTaskMenu } from '@/composables/useTaskMenu'
import { PRIORITY_COLORS } from '@/styles/tokens'

const props = defineProps({
  tasks: { type: Array, default: () => [] },
  // Real board status columns [{ id, name }] for the "move to column" menu.
  statusColumns: { type: Array, default: () => [] },
})
const emit = defineEmits(['open', 'changed'])

const menu = useTaskMenu({
  onOpen: (id) => emit('open', id),
  onChanged: () => emit('changed'),
  columns: () => props.statusColumns,
})

const WEEKDAYS = ['Пн', 'Вт', 'Ср', 'Чт', 'Пт', 'Сб', 'Вс']
const MONTHS = [
  'Январь',
  'Февраль',
  'Март',
  'Апрель',
  'Май',
  'Июнь',
  'Июль',
  'Август',
  'Сентябрь',
  'Октябрь',
  'Ноябрь',
  'Декабрь',
]

const today = new Date()
today.setHours(0, 0, 0, 0)
// Cursor points at the first day of the displayed month.
const cursor = ref(new Date(today.getFullYear(), today.getMonth(), 1))

const monthLabel = computed(
  () => `${MONTHS[cursor.value.getMonth()]} ${cursor.value.getFullYear()}`,
)

function dayKey(d) {
  return `${d.getFullYear()}-${d.getMonth()}-${d.getDate()}`
}

// Index tasks that have a due date by local day key.
const tasksByDay = computed(() => {
  const map = {}
  for (const t of props.tasks) {
    if (!t.due_date) continue
    const d = new Date(t.due_date)
    ;(map[dayKey(d)] ||= []).push(t)
  }
  return map
})

const noDate = computed(() => props.tasks.filter((t) => !t.due_date))

// 6 weeks × 7 days, Monday-first, covering the displayed month.
const weeks = computed(() => {
  const first = cursor.value
  const offset = (first.getDay() + 6) % 7 // Monday = 0
  const start = new Date(first.getFullYear(), first.getMonth(), 1 - offset)
  const cells = []
  for (let i = 0; i < 42; i++) {
    const d = new Date(start.getFullYear(), start.getMonth(), start.getDate() + i)
    cells.push({
      date: d,
      inMonth: d.getMonth() === first.getMonth(),
      isToday: d.getTime() === today.getTime(),
      tasks: tasksByDay.value[dayKey(d)] || [],
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
  cursor.value = new Date(today.getFullYear(), today.getMonth(), 1)
}
</script>

<template>
  <div class="cal">
    <div class="cal-bar">
      <n-button-group size="small">
        <n-button @click="shift(-1)">‹</n-button>
        <n-button @click="goToday">Сегодня</n-button>
        <n-button @click="shift(1)">›</n-button>
      </n-button-group>
      <span class="cal-month">{{ monthLabel }}</span>
    </div>

    <div class="cal-grid">
      <div v-for="w in WEEKDAYS" :key="w" class="cal-wd">{{ w }}</div>
      <template v-for="(row, ri) in weeks" :key="ri">
        <div
          v-for="cell in row"
          :key="cell.date.toISOString()"
          class="cal-cell"
          :class="{ out: !cell.inMonth, today: cell.isToday }"
        >
          <div class="cal-daynum">{{ cell.date.getDate() }}</div>
          <button
            v-for="t in cell.tasks"
            :key="t.id"
            type="button"
            class="cal-chip"
            :class="{ done: t.completed_at }"
            :style="{ '--chip': PRIORITY_COLORS[t.priority || 0] }"
            :title="t.title"
            @click="$emit('open', t.id)"
            @contextmenu.prevent.stop="menu.open($event, t)"
          >
            {{ t.title }}
          </button>
        </div>
      </template>
    </div>

    <div v-if="noDate.length" class="cal-nodate">
      <span class="nd-label">Без срока</span>
      <button
        v-for="t in noDate"
        :key="t.id"
        type="button"
        class="cal-chip"
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
