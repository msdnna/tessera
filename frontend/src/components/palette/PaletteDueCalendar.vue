<script setup>
// The palette's «Срок» after Tab (#3118 — mockup PackageJ palDcal): the
// calendar under the field. The preview row says what ↵ will set (the keyboard's
// day, or what is typed in the field), the grid walks with the arrows,
// PgUp / PgDn, T; the clock chips set the time at once. The keys arrive from the
// palette's input — the focus never leaves it — through `onKey`.
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import { ArrowForwardOutline } from '@vicons/ionicons5'
import Kbd from '@/components/Kbd.vue'
import DueCalendar from '@/components/due/DueCalendar.vue'
import { useFormat } from '@/composables/useFormat'
import { useThemeStore } from '@/stores/theme'
import { parseDue } from '@/utils/quickParse'
import { coordsOf, gridStep, relDays, wallLabel, wallNow } from '@/utils/dueModel'
import { CAPS } from '@/utils/keyCaps'

const props = defineProps({
  // The task's due as a wall reading ({ day, time } | null), and the field's text.
  current: { type: Object, default: null },
  query: { type: String, default: '' },
})
// `pick` — { day, time } to write.
const emit = defineEmits(['pick'])
const { t } = useI18n()
const { formatters, formatTime, locale } = useFormat()
const theme = useThemeStore()
const fmt = () => formatters.value

const today = computed(() => coordsOf(wallNow(fmt())))
// The field's words, when they read: the grid opens on that day.
const typed = computed(() => {
  const p = parseDue(props.query, { now: wallNow(fmt()) })
  return p.ok && !p.clear ? p : null
})
const time = ref(typed.value?.time || props.current?.time || null)
const kf = ref(typed.value?.day ? coordsOf(typed.value.day) : props.current?.day || today.value)
const view = ref({ year: kf.value.year, month: kf.value.month })

const label = computed(() => wallLabel({ day: kf.value, time: time.value }, fmt()))
const rel = computed(() => relDays(kf.value, today.value, locale.value))

const TIMES = ['09:00', '12:00', '18:00']
const clockLabel = (hm) => {
  const [h, m] = hm.split(':').map(Number)
  return formatTime(new Date(Date.UTC(2026, 0, 1, h, m)), { timeZone: 'UTC' })
}

function moveTo(d) {
  kf.value = d
  if (d.year !== view.value.year || d.month !== view.value.month)
    view.value = { year: d.year, month: d.month }
}
function pick(d) {
  moveTo(d)
  emit('pick', { day: d, time: time.value })
}
// The palette hands every key of its input over; true — taken.
function onKey(e) {
  if (e.key === 'Enter') {
    e.preventDefault()
    pick(kf.value)
    return true
  }
  // T only while the field is empty — otherwise it is a letter being typed.
  if (e.code === 'KeyT' && props.query) return false
  const next = gridStep(e, kf.value, today.value)
  if (!next) return false
  e.preventDefault()
  moveTo(next)
  return true
}
defineExpose({ onKey })
</script>

<template>
  <div class="pdc" data-testid="palette-due-cal">
    <div class="pdc-prev">
      <n-icon :component="ArrowForwardOutline" :size="14" class="pdc-prev-i" />
      <b class="pdc-prev-l">{{ label }}</b>
      <span class="pdc-prev-s">· {{ rel }}</span>
      <span class="pdc-grow" />
      <Kbd>{{ CAPS.enter }}</Kbd>
    </div>
    <div class="pdc-body">
      <div class="pdc-cal">
        <DueCalendar
          v-model:view="view"
          :week-start="theme.weekStart === 0 ? 0 : 1"
          :sel="current?.day || null"
          :today="today"
          :focus="kf"
          :cell="36"
          testid="palette-due-grid"
          @pick="pick"
        />
        <div class="pdc-times">
          <span class="pdc-times-l">{{ t('task.due.time') }}</span>
          <span class="pdc-time">{{ time ? clockLabel(time) : t('task.due.noTimeDash') }}</span>
          <button
            v-for="tm in TIMES"
            :key="tm"
            type="button"
            class="pdc-chip"
            :class="{ on: time === tm }"
            :data-time="tm"
            @mousedown.prevent
            @click="time = tm"
          >
            {{ clockLabel(tm) }}
          </button>
          <button
            type="button"
            class="pdc-chip"
            :class="{ on: !time }"
            data-time="none"
            @mousedown.prevent
            @click="time = null"
          >
            {{ t('task.due.noTime') }}
          </button>
        </div>
      </div>
      <div class="pdc-help">
        <span class="pdc-help-t">{{ t('shell.palette.due.calHelpTitle') }}</span>
        <span>{{ t('shell.palette.due.calHelpKeys') }}</span>
        <span>{{ t('shell.palette.due.calHelpMore') }}</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.pdc {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 0 14px 12px;
  overflow-y: auto;
}
.pdc-prev {
  display: flex;
  align-items: center;
  gap: 8px;
  height: 44px;
  padding: 0 12px;
  border-radius: 10px;
  background: var(--t-hover);
  font-size: 14px;
  white-space: nowrap;
}
.pdc-prev-i {
  flex: none;
  color: var(--t-primary);
}
.pdc-prev-l {
  font-weight: 600;
}
.pdc-prev-s {
  color: var(--t-text3);
}
.pdc-grow {
  flex: 1;
}
.pdc-body {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 220px;
  gap: 16px;
}
.pdc-times {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px;
  padding: 8px 2px 2px 6px;
}
.pdc-times-l {
  margin-right: 2px;
  font-size: 12.5px;
  color: var(--t-text3);
}
.pdc-time {
  width: 62px;
  height: 28px;
  box-sizing: border-box;
  display: flex;
  align-items: center;
  padding: 0 9px;
  border-radius: 8px;
  background: var(--t-input-bg);
  font-size: 13px;
  font-variant-numeric: tabular-nums;
}
.pdc-chip {
  height: 28px;
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
.pdc-chip:hover {
  background: var(--t-hover);
}
.pdc-chip.on {
  background: linear-gradient(
    to top right,
    color-mix(in srgb, var(--t-primary) 16%, var(--t-pop)),
    color-mix(in srgb, var(--t-primary) 28%, var(--t-pop))
  );
  font-weight: 600;
}
.pdc-help {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding-top: 4px;
  font-size: 12.5px;
  line-height: 1.5;
  color: var(--t-text3);
}
.pdc-help-t {
  font-weight: 600;
  color: var(--t-text2);
}
@media (max-width: 768px) {
  .pdc-body {
    grid-template-columns: minmax(0, 1fr);
  }
  .pdc-help {
    display: none;
  }
}
</style>
