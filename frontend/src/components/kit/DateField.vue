<script setup>
// A date field on the v2 calendar (#3034): the grid of the due picker
// (due/DueCalendar) in an AnchorLayer at a field-looking trigger. It replaces
// Naive's v1 NDatePicker wherever a plain date is asked for — a meeting's start,
// a stage's dates, a GitLab import cut-off — and keeps its value contract, so a
// host swaps the tag and nothing else:
//   type="date"       epoch ms of the local midnight, or null
//   type="datetime"   epoch ms of the local date and time, or null
//   type="daterange"  [start, end] of local midnights, or null
//
// Keys inside the open layer: arrows / PgUp / PgDn / Home / End / T walk the
// grid (utils/dueModel `gridStep`), ↵ picks the walked day (in datetime: closes),
// Esc closes without touching the value. A range takes two picks; the second one
// earlier than the first swaps them.
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import { CalendarClearOutline, CloseCircle } from '@vicons/ionicons5'
import AnchorLayer from './AnchorLayer.vue'
import './kit.css'
import DueCalendar from '@/components/due/DueCalendar.vue'
import { coordsOf, dateOf, epochOf, gridStep, pad2, sameDay } from '@/utils/dueModel'
import { parseClock } from '@/utils/quickParse'
import { useFormat } from '@/composables/useFormat'
import { useThemeStore } from '@/stores/theme'

const props = defineProps({
  value: { type: [Number, Array], default: null },
  type: { type: String, default: 'date' }, // date | datetime | daterange
  placeholder: { type: String, default: '' },
  clearable: { type: Boolean, default: true },
  disabled: { type: Boolean, default: false },
  // The time a datetime gets when a day is picked before any time was.
  defaultTime: { type: String, default: '09:00' },
  testid: { type: String, default: 'date-field' },
})
const emit = defineEmits(['update:value'])
// Two roots (the trigger and the layer): the host's class and listeners go on
// the trigger. A host's own keydown runs first — `@keydown.enter.prevent` there
// (submit a row) wins over «↵ opens the calendar».
defineOptions({ inheritAttrs: false })
defineExpose({ focus: () => trigger.value?.focus(), open: () => show() })

const { t } = useI18n()
const { formatDate, formatDateTime, formatTime } = useFormat()
const theme = useThemeStore()
const weekStart = computed(() => (theme.weekStart === 0 ? 0 : 1))

const isRange = computed(() => props.type === 'daterange')
const withTime = computed(() => props.type === 'datetime')

// ── the value as calendar coordinates ──
const fromMs = (ms) => (ms === null || ms === undefined ? null : coordsOf(new Date(ms)))
const startMs = computed(() => (isRange.value ? (props.value?.[0] ?? null) : props.value))
const endMs = computed(() => (isRange.value ? (props.value?.[1] ?? null) : null))
const timeOf = (ms) => {
  if (ms === null || ms === undefined) return null
  const d = new Date(ms)
  return `${pad2(d.getHours())}:${pad2(d.getMinutes())}`
}

const label = computed(() => {
  if (isRange.value) {
    if (startMs.value === null) return ''
    const a = formatDate(startMs.value)
    return endMs.value === null ? a : `${a} – ${formatDate(endMs.value)}`
  }
  if (props.value === null || props.value === undefined) return ''
  return withTime.value ? formatDateTime(props.value) : formatDate(props.value)
})

// ── the layer ──
const open = ref(false)
const trigger = ref(null)
const today = ref(coordsOf(new Date()))
const view = ref({ year: today.value.year, month: today.value.month })
const focus = ref(null)
// A range being picked: the first end, waiting for the second.
const pending = ref(null)

function show() {
  if (props.disabled) return
  today.value = coordsOf(new Date())
  const at = fromMs(startMs.value) || today.value
  view.value = { year: at.year, month: at.month }
  focus.value = at
  pending.value = null
  timeDraft.value = clockLabel(timeOf(startMs.value))
  open.value = true
}
function close() {
  open.value = false
  pending.value = null
  nextTick(() => trigger.value?.focus({ preventScroll: true }))
}

// What the grid draws: in a range being picked the pending end is the selection.
const sel = computed(() => pending.value || fromMs(startMs.value))
const end = computed(() => (pending.value ? null : fromMs(endMs.value)))
const span = computed(() => {
  if (!sel.value || !end.value) return null
  return { lo: epochOf(sel.value), hi: epochOf(end.value) }
})

function emitDay(p, hm) {
  const d = dateOf(p)
  if (hm) {
    const [h, m] = hm.split(':').map(Number)
    d.setHours(h, m, 0, 0)
  }
  return d.getTime()
}

function pick(p) {
  focus.value = p
  if (isRange.value) {
    if (!pending.value) {
      pending.value = p
      return
    }
    const [a, b] = epochOf(p) < epochOf(pending.value) ? [p, pending.value] : [pending.value, p]
    emit('update:value', [emitDay(a), emitDay(b)])
    close()
    return
  }
  if (withTime.value) {
    const hm = timeOf(props.value) || parseClock(timeDraft.value) || props.defaultTime
    emit('update:value', emitDay(p, hm))
    timeDraft.value = clockLabel(hm)
    return
  }
  emit('update:value', emitDay(p))
  close()
}

function clear() {
  emit('update:value', null)
  if (open.value) close()
}

// ── time (datetime) ──
const TIMES = ['09:00', '12:00', '18:00']
const clockLabel = (hm) => {
  if (!hm) return ''
  const [h, m] = hm.split(':').map(Number)
  return formatTime(new Date(Date.UTC(2026, 0, 1, h, m)), { timeZone: 'UTC' })
}
const timeDraft = ref('')
const activeTime = computed(() => timeOf(props.value))
watch(activeTime, (hm) => (timeDraft.value = clockLabel(hm)))
function setTime(hm) {
  const day = fromMs(props.value) || focus.value || today.value
  emit('update:value', emitDay(day, hm))
  timeDraft.value = clockLabel(hm)
}
function commitTime() {
  const v = timeDraft.value.trim()
  if (!v || v === clockLabel(activeTime.value)) {
    timeDraft.value = clockLabel(activeTime.value)
    return
  }
  const hm = parseClock(v)
  if (hm) setTime(hm)
  else timeDraft.value = clockLabel(activeTime.value)
}

// ── keys ──
function onKey(e) {
  if (e.key === 'Escape') {
    e.preventDefault()
    e.stopPropagation()
    close()
    return
  }
  const inTime = e.target?.classList?.contains('df-time-in')
  if (e.key === 'Enter') {
    e.preventDefault()
    e.stopPropagation()
    if (inTime) {
      commitTime()
      close()
    } else if (withTime.value && focus.value && sameDay(focus.value, fromMs(props.value))) close()
    else if (focus.value) pick(focus.value)
    return
  }
  if (inTime) return
  const next = gridStep(e, focus.value, today.value)
  if (!next) return
  e.preventDefault()
  focus.value = next
  view.value = { year: next.year, month: next.month }
}

function onTriggerKey(e) {
  if (e.defaultPrevented) return
  if (e.key === 'Enter' || e.key === ' ' || e.key === 'ArrowDown') {
    e.preventDefault()
    show()
  } else if ((e.key === 'Backspace' || e.key === 'Delete') && props.clearable && label.value) {
    e.preventDefault()
    clear()
  }
}
</script>

<template>
  <button
    v-bind="$attrs"
    ref="trigger"
    type="button"
    class="kf-input df-trigger"
    :class="{ empty: !label, active: open }"
    :disabled="disabled"
    :aria-expanded="open"
    aria-haspopup="dialog"
    :data-testid="testid"
    @click="open ? close() : show()"
    @keydown="onTriggerKey"
  >
    <span class="df-text">{{ label || placeholder }}</span>
    <span
      v-if="clearable && label && !disabled"
      class="df-clear"
      role="button"
      :aria-label="t('common.date.clear')"
      :data-testid="`${testid}-clear`"
      @click.stop="clear"
    >
      <n-icon :component="CloseCircle" :size="15" />
    </span>
    <n-icon v-else :component="CalendarClearOutline" :size="15" class="df-ic" />
  </button>

  <AnchorLayer
    :show="open"
    :anchor="() => trigger"
    :title="placeholder"
    :testid="`${testid}-layer`"
    focus=".df"
    @update:show="(v) => !v && close()"
  >
    <div class="df" tabindex="-1" @keydown="onKey">
      <div v-if="isRange" class="df-hint">
        {{ pending ? t('common.date.pickEnd') : t('common.date.pickStart') }}
      </div>
      <DueCalendar
        v-model:view="view"
        :week-start="weekStart"
        :sel="sel"
        :end="end"
        :span="span"
        :today="today"
        :focus="focus"
        :testid="`${testid}-cal`"
        @pick="pick"
      />
      <div v-if="withTime" class="df-times">
        <span class="df-times-l">{{ t('task.due.time') }}</span>
        <input
          v-model="timeDraft"
          class="df-time-in"
          :placeholder="t('task.due.noTimeDash')"
          :aria-label="t('task.due.time')"
          :data-testid="`${testid}-time`"
          @blur="commitTime"
        />
        <button
          v-for="tm in TIMES"
          :key="tm"
          type="button"
          class="df-chip"
          :class="{ on: activeTime === tm }"
          :data-time="tm"
          @click="setTime(tm)"
        >
          {{ clockLabel(tm) }}
        </button>
      </div>
      <div class="df-foot">
        <button
          v-if="clearable"
          type="button"
          class="df-act"
          :data-testid="`${testid}-clear-btn`"
          @click="clear"
        >
          {{ t('common.date.clear') }}
        </button>
        <span class="df-grow" />
        <button
          v-if="withTime"
          type="button"
          class="df-act primary"
          :data-testid="`${testid}-done`"
          @click="close"
        >
          {{ t('common.action.done') }}
        </button>
      </div>
    </div>
  </AnchorLayer>
</template>

<style scoped>
.df-trigger {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  text-align: left;
  cursor: pointer;
}
.df-trigger.active {
  box-shadow: 0 0 0 2px var(--t-primary);
}
.df-text {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-variant-numeric: tabular-nums;
}
.df-trigger.empty .df-text {
  color: var(--t-text3);
}
.df-ic,
.df-clear {
  flex: none;
  display: flex;
  color: var(--t-text3);
}
.df-clear:hover {
  color: var(--t-text1);
}
.df {
  width: 320px;
  box-sizing: border-box;
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 8px;
  border-radius: var(--t-r-card, 14px);
  background: var(--t-pop);
  box-shadow: var(--t-sh-pop);
  outline: 0;
}
.df-hint {
  padding: 2px 6px;
  font-size: 12.5px;
  color: var(--t-text3);
}
.df-times {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 4px 2px 0 6px;
}
.df-times-l {
  margin-right: 2px;
  font-size: 12.5px;
  color: var(--t-text3);
}
.df-time-in {
  width: 62px;
  height: 28px;
  box-sizing: border-box;
  padding: 0 9px;
  border: 0;
  outline: 0;
  border-radius: 8px;
  background: var(--t-input-bg);
  color: var(--t-text1);
  font: inherit;
  font-size: 13px;
  font-variant-numeric: tabular-nums;
}
.df-time-in:focus {
  box-shadow: 0 0 0 2px var(--t-primary);
}
.df-chip,
.df-act {
  height: 28px;
  display: flex;
  align-items: center;
  padding: 0 10px;
  border: 0;
  border-radius: 999px;
  background: var(--t-ctl);
  color: var(--t-text1);
  font: inherit;
  font-size: 12.5px;
  white-space: nowrap;
  cursor: pointer;
}
.df-chip {
  padding: 0 8px;
}
.df-chip:hover,
.df-act:hover {
  background: var(--t-hover);
}
.df-chip.on,
.df-act.primary {
  background: linear-gradient(
    to top right,
    color-mix(in srgb, var(--t-primary) 16%, var(--t-pop)),
    color-mix(in srgb, var(--t-primary) 28%, var(--t-pop))
  );
  font-weight: 600;
}
.df-foot {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 2px;
}
.df-grow {
  flex: 1;
}
/* On a phone the layer is a bottom sheet: the card's own box would be a second one. */
:global(.al-sheet) .df {
  width: auto;
  background: transparent;
  box-shadow: none;
}
</style>
