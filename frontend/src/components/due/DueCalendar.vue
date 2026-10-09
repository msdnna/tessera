<script setup>
// The month grid of the due picker (v2, #3118 — DuePicker «календарь»): a header
// with the month, ‹ › and «Сегодня T», the weekday row and 4–6 weeks of days.
// Days are calendar coordinates (utils/dueModel), never instants.
//
// What a day can be, strongest first: `sel` (the endpoint being edited — the
// accent gradient), `end` (the other endpoint — a ring), `inr` (between the two),
// `rep` (an upcoming occurrence of the repeat, or a picked custom date), plain.
// `today` gets a dot, the keyboard's day (`focus`) a ring around the cell.
//
// The keys belong to the host — the picker's grid zone, the palette's input —
// which moves `focus` with utils/dueModel `gridStep`; the grid only draws it.
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import { ChevronBackOutline, ChevronForwardOutline } from '@vicons/ionicons5'
import Kbd from '../Kbd.vue'
import { useFormat } from '@/composables/useFormat'
import { addDays as addCalDays, weekStart as weekOf } from '@/utils/calendarView'
import { capitalizeFirst } from '@/utils/format'
import { dayKey, epochOf, weekOrder } from '@/utils/dueModel'
import { cap } from '@/utils/keyCaps'

const props = defineProps({
  view: { type: Object, required: true }, // { year, month }
  weekStart: { type: Number, default: 1 },
  sel: { type: Object, default: null },
  end: { type: Object, default: null },
  // Epoch days strictly between the endpoints are shaded: { lo, hi }.
  span: { type: Object, default: null },
  rep: { type: Object, default: () => new Set() }, // Set of 'YYYY-MM-DD'
  today: { type: Object, required: true },
  focus: { type: Object, default: null },
  // Cell height (32 popover · 36 palette · 44 phone).
  cell: { type: Number, default: 32 },
  testid: { type: String, default: 'due-cal' },
})
const emit = defineEmits(['pick', 'update:view'])
const { t } = useI18n()
const { formatDate } = useFormat()

const monthLabel = computed(() => {
  const d = new Date(Date.UTC(props.view.year, props.view.month, 15))
  const m = formatDate(d, { month: 'long', timeZone: 'UTC' })
  return `${capitalizeFirst(m)} ${props.view.year}`
})
const weekdays = computed(() =>
  weekOrder(props.weekStart).map((d) =>
    formatDate(new Date(Date.UTC(2026, 2, 1 + d)), { weekday: 'short', timeZone: 'UTC' }),
  ),
)
const weeks = computed(() => {
  // Always six weeks: the popover keeps its height from month to month.
  const first = weekOf({ ...props.view, day: 1 }, props.weekStart)
  const cells = Array.from({ length: 42 }, (_, i) => {
    const c = addCalDays(first, i)
    return { ...c, inMonth: c.month === props.view.month }
  })
  const k = (p) => (p ? dayKey(p) : '')
  const selK = k(props.sel)
  const endK = k(props.end)
  const todayK = k(props.today)
  const focusK = k(props.focus)
  const rows = []
  for (let i = 0; i < cells.length; i += 7) {
    rows.push(
      cells.slice(i, i + 7).map((c) => {
        const key = dayKey(c)
        const e = epochOf(c)
        let kind = 'norm'
        if (key === selK) kind = 'sel'
        else if (!c.inMonth) kind = 'norm'
        else if (key === endK) kind = 'end'
        else if (props.span && e > props.span.lo && e < props.span.hi) kind = 'inr'
        else if (props.rep.has(key)) kind = 'rep'
        return {
          key,
          p: { year: c.year, month: c.month, day: c.day },
          n: c.day,
          out: !c.inMonth,
          kind,
          today: key === todayK,
          focus: key === focusK,
        }
      }),
    )
  }
  return rows
})

function step(n) {
  const d = new Date(props.view.year, props.view.month + n, 1)
  emit('update:view', { year: d.getFullYear(), month: d.getMonth() })
}
function toToday() {
  emit('update:view', { year: props.today.year, month: props.today.month })
}
</script>

<template>
  <div class="dc" :style="{ '--dc-cell': `${cell}px` }" :data-testid="testid">
    <div class="dc-head">
      <span class="dc-month" data-testid="due-cal-month">{{ monthLabel }}</span>
      <span class="dc-grow" />
      <button
        type="button"
        class="dc-nav"
        :aria-label="t('task.due.prevMonth')"
        tabindex="-1"
        @mousedown.prevent
        @click="step(-1)"
      >
        <n-icon :component="ChevronBackOutline" :size="14" />
      </button>
      <button
        type="button"
        class="dc-nav"
        :aria-label="t('task.due.nextMonth')"
        tabindex="-1"
        @mousedown.prevent
        @click="step(1)"
      >
        <n-icon :component="ChevronForwardOutline" :size="14" />
      </button>
      <button
        type="button"
        class="dc-today"
        tabindex="-1"
        data-testid="due-cal-today"
        @mousedown.prevent
        @click="toToday"
      >
        {{ t('task.due.today') }}<Kbd class="dc-kbd">{{ cap('t') }}</Kbd>
      </button>
    </div>
    <div class="dc-row dc-wd">
      <span v-for="(w, i) in weekdays" :key="i" class="dc-wdh">{{ w }}</span>
    </div>
    <div class="dc-weeks" role="grid">
      <div v-for="(w, i) in weeks" :key="i" class="dc-row" role="row">
        <button
          v-for="c in w"
          :key="c.key"
          type="button"
          tabindex="-1"
          role="gridcell"
          class="dc-day"
          :class="[c.kind, { out: c.out, kf: c.focus }]"
          :data-day="c.key"
          :aria-selected="c.kind === 'sel'"
          @mousedown.prevent
          @click="emit('pick', c.p)"
        >
          {{ c.n }}<span v-if="c.today" class="dc-dot" />
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.dc {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}
.dc-head {
  display: flex;
  align-items: center;
  gap: 4px;
  height: 30px;
  padding: 0 2px 0 6px;
}
.dc-month {
  font-size: 13.5px;
  font-weight: 650;
  white-space: nowrap;
}
.dc-grow {
  flex: 1;
}
.dc-nav {
  width: 26px;
  height: 26px;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--t-text2);
  cursor: pointer;
}
.dc-nav:hover {
  background: var(--t-hover);
}
.dc-today {
  height: 26px;
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 0 4px 0 8px;
  border: 0;
  border-radius: 7px;
  background: var(--t-ctl);
  color: var(--t-text1);
  font: inherit;
  font-size: 12px;
  cursor: pointer;
}
.dc-today:hover {
  background: var(--t-hover);
}
.dc-row {
  display: flex;
  gap: 2px;
}
.dc-wdh {
  flex: 1;
  padding: 2px 0;
  text-align: center;
  font-size: 11px;
  color: var(--t-text3);
}
.dc-weeks {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.dc-day {
  position: relative;
  flex: 1;
  min-width: 0;
  height: var(--dc-cell);
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: var(--t-text1);
  font: inherit;
  font-size: 13px;
  font-variant-numeric: tabular-nums;
  cursor: pointer;
}
.dc-day:hover {
  background: var(--t-hover);
}
.dc-day.out {
  color: var(--t-text3);
  opacity: 0.45;
}
.dc-day.rep {
  background: color-mix(in srgb, var(--t-primary) 16%, transparent);
}
.dc-day.inr {
  background: color-mix(in srgb, var(--t-primary) 9%, transparent);
}
.dc-day.end {
  box-shadow: inset 0 0 0 1.5px var(--t-primary);
  font-weight: 600;
}
.dc-day.sel {
  background: var(--t-accent-grad);
  color: #fff;
  font-weight: 650;
  opacity: 1;
}
.dc-day.kf::after {
  content: '';
  position: absolute;
  inset: -2px;
  border-radius: 10px;
  box-shadow: 0 0 0 2px var(--t-primary);
  pointer-events: none;
}
.dc-dot {
  position: absolute;
  bottom: 3px;
  left: 50%;
  width: 4px;
  height: 4px;
  margin-left: -2px;
  border-radius: 2px;
  background: var(--t-accent-grad);
}
.dc-day.sel .dc-dot {
  background: #fff;
}
</style>
