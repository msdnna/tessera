<script setup>
import { ref, computed, watch } from 'vue'
import { NSelect, NInputNumber, NCheckbox, NTimePicker, NIcon } from 'naive-ui'
import {
  ChevronBackOutline,
  ChevronForwardOutline,
  PlayBackOutline,
  PlayForwardOutline,
  RepeatOutline,
} from '@vicons/ionicons5'
import { useThemeStore } from '@/stores/theme'
import {
  FREQ_OPTIONS,
  TRIGGER_OPTIONS,
  unitLabel,
  occurrenceKeys,
} from '@/utils/recurrence'

const props = defineProps({
  due: { type: Number, default: null }, // ms
  recurrence: { type: Object, default: null },
  notify: { type: Object, default: () => ({ enabled: 'inherit', lead: -1, repeat: -1 }) },
  columns: { type: Array, default: () => [] }, // [{id,name}]
})
// `apply` carries due + recurrence together (one PATCH, no clobber); `notify`
// is separate (different endpoint).
const emit = defineEmits(['apply', 'notify'])

// commit sends the current due date + recurrence rule as one patch.
function commit() {
  emit('apply', {
    due_date: localDue.value != null ? new Date(localDue.value).toISOString() : null,
    recurrence: buildRule(),
  })
}

const theme = useThemeStore()
const weekStart = computed(() => (theme.weekStart === 0 ? 0 : 1)) // 0=Sun, 1=Mon

// ── due date + time ──
const localDue = ref(props.due)
watch(
  () => props.due,
  (v) => {
    localDue.value = v
    if (v != null) {
      const d = new Date(v)
      viewY.value = d.getFullYear()
      viewM.value = d.getMonth()
    }
  },
)
function emitDue(ms) {
  localDue.value = ms
  commit()
}
function setTime(ms) {
  // n-time-picker gives a ms whose date part may be today; keep our date, take time.
  if (ms == null) return
  const t = new Date(ms)
  const base = localDue.value != null ? new Date(localDue.value) : new Date()
  base.setHours(t.getHours(), t.getMinutes(), 0, 0)
  emitDue(base.getTime())
}

// ── calendar view ──
const today = new Date()
const viewY = ref((props.due ? new Date(props.due) : today).getFullYear())
const viewM = ref((props.due ? new Date(props.due) : today).getMonth())
function stepMonth(delta) {
  const m = viewM.value + delta
  viewY.value += Math.floor(m / 12)
  viewM.value = ((m % 12) + 12) % 12
}
const monthLabel = computed(() =>
  new Date(viewY.value, viewM.value, 1).toLocaleDateString(
    theme.language === 'en' ? 'en-GB' : 'ru-RU',
    { month: 'long', year: 'numeric' },
  ),
)
const WD_RU = ['вс', 'пн', 'вт', 'ср', 'чт', 'пт', 'сб']
const weekdayHeaders = computed(() => {
  const order = weekStart.value === 0 ? [0, 1, 2, 3, 4, 5, 6] : [1, 2, 3, 4, 5, 6, 0]
  return order.map((d) => WD_RU[d])
})
const dayKey = (d) =>
  `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
const todayKey = dayKey(today)
const selectedKey = computed(() => (localDue.value != null ? dayKey(new Date(localDue.value)) : ''))

// 42-cell grid starting on the week-start on/before the 1st.
const gridDays = computed(() => {
  const first = new Date(viewY.value, viewM.value, 1)
  const firstDow = first.getDay() // 0=Sun..6=Sat
  const lead = (firstDow - weekStart.value + 7) % 7
  const start = new Date(viewY.value, viewM.value, 1 - lead)
  const cells = []
  for (let i = 0; i < 42; i++) {
    const d = new Date(start.getFullYear(), start.getMonth(), start.getDate() + i)
    cells.push(d)
  }
  return cells
})

// Highlighted occurrences (the rule's upcoming dates), excluding the selected one.
const occKeys = computed(() => occurrenceKeys(rule.value, localDue.value, 24))

function onDayClick(d) {
  if (freq.value === 'custom') {
    const k = dayKey(d)
    const set = new Set(dates.value)
    if (set.has(k)) set.delete(k)
    else set.add(k)
    dates.value = [...set].sort()
    // Due = earliest selected custom date (keep time of day).
    if (dates.value.length) {
      const [yy, mm, dd] = dates.value[0].split('-').map(Number)
      const base = localDue.value != null ? new Date(localDue.value) : new Date()
      localDue.value = new Date(yy, mm - 1, dd, base.getHours(), base.getMinutes(), 0, 0).getTime()
    }
    commit()
    return
  }
  const base = localDue.value != null ? new Date(localDue.value) : new Date(0, 0, 0, 9, 0)
  emitDue(new Date(d.getFullYear(), d.getMonth(), d.getDate(), base.getHours(), base.getMinutes(), 0, 0).getTime())
}

// ── recurrence rule (local mirror of props.recurrence) ──
const freq = ref('')
const interval = ref(1)
const weekdays = ref([]) // [0..6]
const dates = ref([]) // ['YYYY-MM-DD']
const trigger = ref('complete')
const triggerColumn = ref(null)
const targetColumn = ref(null)
const createNew = ref(false)
const recurForever = ref(true)
const skipWeekends = ref(false)

watch(
  () => props.recurrence,
  (r) => {
    freq.value = r?.freq || ''
    interval.value = r?.interval || 1
    weekdays.value = r?.weekdays ? [...r.weekdays] : []
    dates.value = r?.dates ? [...r.dates] : []
    trigger.value = r?.trigger || 'complete'
    triggerColumn.value = r?.trigger_column || null
    targetColumn.value = r?.target_column || null
    createNew.value = !!r?.create_new
    recurForever.value = !r?.once
    skipWeekends.value = !!r?.skip_weekends
  },
  { immediate: true },
)

const rule = computed(() => buildRule())
function buildRule() {
  if (!freq.value) return null
  const r = { freq: freq.value, interval: Math.max(1, Math.round(interval.value || 1)) }
  if (freq.value === 'weekly' && weekdays.value.length) r.weekdays = [...weekdays.value].sort((a, b) => a - b)
  if (freq.value === 'custom') r.dates = [...dates.value].sort()
  if (trigger.value && trigger.value !== 'complete') r.trigger = trigger.value
  if (trigger.value === 'column' && triggerColumn.value) r.trigger_column = triggerColumn.value
  if (targetColumn.value) r.target_column = targetColumn.value
  if (createNew.value) r.create_new = true
  if (!recurForever.value) r.once = true
  if (skipWeekends.value && (freq.value === 'daily' || freq.value === 'weekly')) r.skip_weekends = true
  return r
}
function setFreq(v) {
  freq.value = v
  // A recurrence needs a due date to advance from — anchor to now if unset.
  if (v && localDue.value == null) emitDue(Date.now())
  commit()
}
function toggleWeekday(d) {
  const set = new Set(weekdays.value)
  if (set.has(d)) set.delete(d)
  else set.add(d)
  weekdays.value = [...set].sort((a, b) => a - b)
  commit()
}

const columnOptions = computed(() => props.columns.map((c) => ({ label: c.name, value: c.id })))
const targetOptions = computed(() => [{ label: 'Первая колонка', value: null }, ...columnOptions.value])

// weekday chips, ordered by week-start; 0=Sun..6=Sat
const weekdayChips = computed(() => {
  const order = weekStart.value === 0 ? [0, 1, 2, 3, 4, 5, 6] : [1, 2, 3, 4, 5, 6, 0]
  return order.map((d) => ({ d, label: WD_RU[d] }))
})

// ── notify ──
const DUE_ENABLED_OPTS = [
  { label: 'По умолчанию', value: 'inherit' },
  { label: 'Включены', value: 'on' },
  { label: 'Выключены', value: 'off' },
]
const DUE_LEAD_OPTS = [
  { label: 'По умолчанию', value: -1 },
  { label: 'В срок', value: 0 },
  { label: 'За 15 мин', value: 15 },
  { label: 'За час', value: 60 },
  { label: 'За 3 часа', value: 180 },
  { label: 'За день', value: 1440 },
]
const DUE_REPEAT_OPTS = [
  { label: 'По умолчанию', value: -1 },
  { label: 'Однократно', value: 0 },
  { label: 'Каждый час', value: 60 },
  { label: 'Каждые 3 часа', value: 180 },
  { label: 'Каждый день', value: 1440 },
]
</script>

<template>
  <div class="due-editor" @click.stop>
    <div class="de-main">
      <!-- recurrence side-panel (left) -->
      <div class="de-recur">
        <div class="de-recur-head">
          <n-icon :component="RepeatOutline" :size="15" />
          <span>Повтор</span>
        </div>
        <n-select size="small" :value="freq" :options="FREQ_OPTIONS" @update:value="setFreq" />

        <template v-if="freq">
          <div v-if="freq !== 'custom'" class="de-row">
            <span class="muted">каждые</span>
            <n-input-number
              size="small"
              class="de-num"
              :value="interval"
              :min="1"
              :max="99"
              @update:value="(v) => { interval = v || 1; commit() }"
            />
            <span class="muted">{{ unitLabel(freq, interval) }}</span>
          </div>

          <div v-if="freq === 'weekly'" class="de-weekdays">
            <button
              v-for="w in weekdayChips"
              :key="w.d"
              type="button"
              class="wd-chip"
              :class="{ on: weekdays.includes(w.d) }"
              @click="toggleWeekday(w.d)"
            >
              {{ w.label }}
            </button>
          </div>

          <div v-if="freq === 'custom'" class="de-hint muted">
            Отметьте даты повтора в календаре →
          </div>

          <label class="de-field">
            <span class="muted">Событие</span>
            <n-select
              size="small"
              :value="trigger"
              :options="TRIGGER_OPTIONS"
              @update:value="(v) => { trigger = v; commit() }"
            />
          </label>

          <label v-if="trigger === 'column'" class="de-field">
            <span class="muted">Колонка-триггер</span>
            <n-select
              size="small"
              :value="triggerColumn"
              :options="columnOptions"
              placeholder="Выберите колонку"
              @update:value="(v) => { triggerColumn = v; commit() }"
            />
          </label>

          <label class="de-field">
            <span class="muted">Переносить в</span>
            <n-select
              size="small"
              :value="targetColumn"
              :options="targetOptions"
              @update:value="(v) => { targetColumn = v; commit() }"
            />
          </label>

          <n-checkbox
            size="small"
            :checked="createNew"
            @update:checked="(v) => { createNew = v; commit() }"
          >
            Создавать дубликат
          </n-checkbox>
          <n-checkbox
            size="small"
            :checked="recurForever"
            @update:checked="(v) => { recurForever = v; commit() }"
          >
            Повторять всегда
          </n-checkbox>
          <n-checkbox
            v-if="freq === 'daily' || freq === 'weekly'"
            size="small"
            :checked="skipWeekends"
            @update:checked="(v) => { skipWeekends = v; commit() }"
          >
            Пропускать выходные
          </n-checkbox>
        </template>
      </div>

      <!-- calendar (right) -->
      <div class="de-cal">
        <div class="de-cal-head">
          <button class="nav" type="button" @click="viewY--"><n-icon :component="PlayBackOutline" :size="13" /></button>
          <button class="nav" type="button" @click="stepMonth(-1)"><n-icon :component="ChevronBackOutline" :size="15" /></button>
          <span class="de-month">{{ monthLabel }}</span>
          <button class="nav" type="button" @click="stepMonth(1)"><n-icon :component="ChevronForwardOutline" :size="15" /></button>
          <button class="nav" type="button" @click="viewY++"><n-icon :component="PlayForwardOutline" :size="13" /></button>
        </div>
        <div class="de-grid de-wd">
          <span v-for="(w, i) in weekdayHeaders" :key="i" class="de-wd-h">{{ w }}</span>
        </div>
        <div class="de-grid">
          <button
            v-for="(d, i) in gridDays"
            :key="i"
            type="button"
            class="de-day"
            :class="{
              out: d.getMonth() !== viewM,
              sel: dayKey(d) === selectedKey,
              occ: dayKey(d) !== selectedKey && occKeys.has(dayKey(d)),
              today: dayKey(d) === todayKey,
            }"
            @click="onDayClick(d)"
          >
            {{ d.getDate() }}
          </button>
        </div>
        <div class="de-cal-foot">
          <n-time-picker
            size="small"
            format="HH:mm"
            :value="localDue"
            placeholder="Время"
            @update:value="setTime"
          />
          <button class="de-link" type="button" @click="emitDue(null)">Очистить</button>
        </div>
      </div>
    </div>

    <!-- notifications (shared) -->
    <div v-if="localDue != null" class="de-notify">
      <div class="dn-row">
        <span>Уведомления</span>
        <n-select
          size="tiny"
          :value="notify.enabled"
          :options="DUE_ENABLED_OPTS"
          @update:value="(v) => emit('notify', { enabled: v })"
        />
      </div>
      <div class="dn-row">
        <span>Напоминать</span>
        <n-select
          size="tiny"
          :value="notify.lead"
          :options="DUE_LEAD_OPTS"
          @update:value="(v) => emit('notify', { lead: v })"
        />
      </div>
      <div class="dn-row">
        <span>Повтор уведомления</span>
        <n-select
          size="tiny"
          :value="notify.repeat"
          :options="DUE_REPEAT_OPTS"
          @update:value="(v) => emit('notify', { repeat: v })"
        />
      </div>
    </div>
  </div>
</template>

<style scoped>
.due-editor {
  background: var(--t-surface);
  border-radius: 10px;
}
.de-main {
  display: flex;
  align-items: stretch;
}
/* recurrence panel left of the calendar */
.de-recur {
  width: 210px;
  flex: 0 0 210px;
  padding: 12px;
  border-right: 1px solid var(--t-border);
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.de-recur-head {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  font-weight: 600;
  color: var(--t-text2);
}
.de-row {
  display: flex;
  align-items: center;
  gap: 8px;
}
.de-num {
  width: 76px;
}
.de-field {
  display: flex;
  flex-direction: column;
  gap: 3px;
}
.de-field .muted {
  font-size: 12px;
}
.de-weekdays {
  display: flex;
  gap: 3px;
  flex-wrap: wrap;
}
.wd-chip {
  flex: 1 1 0;
  min-width: 24px;
  padding: 4px 0;
  border-radius: 6px;
  border: 1px solid var(--t-border);
  background: var(--t-surface);
  color: var(--t-text2);
  font-size: 11px;
  cursor: pointer;
}
.wd-chip.on {
  background: var(--t-accent-grad-subtle);
  border-color: transparent;
  color: var(--t-primary);
  font-weight: 600;
}
.de-hint {
  font-size: 12px;
  line-height: 1.35;
}
.muted {
  color: var(--t-text3);
  font-size: 13px;
}

/* calendar */
.de-cal {
  flex: 1;
  padding: 12px;
  min-width: 248px;
}
.de-cal-head {
  display: flex;
  align-items: center;
  gap: 2px;
  margin-bottom: 8px;
}
.de-month {
  flex: 1;
  text-align: center;
  font-size: 13px;
  font-weight: 600;
  color: var(--t-text1);
  text-transform: capitalize;
}
.nav {
  width: 26px;
  height: 26px;
  display: grid;
  place-items: center;
  border: none;
  background: transparent;
  border-radius: 6px;
  color: var(--t-text2);
  cursor: pointer;
}
.nav:hover {
  background: var(--t-hover);
}
.de-grid {
  display: grid;
  grid-template-columns: repeat(7, 1fr);
  gap: 2px;
}
.de-wd {
  margin-bottom: 2px;
}
.de-wd-h {
  text-align: center;
  font-size: 11px;
  color: var(--t-text3);
  padding: 2px 0;
}
.de-day {
  aspect-ratio: 1;
  border: none;
  background: transparent;
  border-radius: 7px;
  color: var(--t-text1);
  font-size: 12px;
  cursor: pointer;
  position: relative;
}
.de-day:hover {
  background: var(--t-hover);
}
.de-day.out {
  color: var(--t-text3);
}
.de-day.occ {
  background: var(--t-accent-grad-subtle);
  color: var(--t-primary);
}
.de-day.sel {
  background: var(--t-accent-grad);
  color: #fff;
  font-weight: 600;
}
.de-day.today::after {
  content: '';
  position: absolute;
  top: 4px;
  right: 5px;
  width: 4px;
  height: 4px;
  border-radius: 50%;
  background: var(--t-primary);
}
.de-day.sel.today::after {
  background: #fff;
}
.de-cal-foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-top: 10px;
}
.de-link {
  border: none;
  background: transparent;
  color: var(--t-text3);
  font-size: 13px;
  cursor: pointer;
}
.de-link:hover {
  color: var(--t-text1);
}

/* notify */
.de-notify {
  border-top: 1px solid var(--t-border);
  padding: 10px 12px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.dn-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  font-size: 12px;
  color: var(--t-text2);
}
.dn-row :deep(.n-select) {
  width: 150px;
}
</style>
