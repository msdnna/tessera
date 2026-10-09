<script setup>
// «Повтор» — the due picker's sub-panel behind its door (v2, #3118, mockup
// DuePicker pane=rep): frequency, «каждые N», the weekdays (or, «По датам», a
// grid of its own to tick dates in), when the next one is made, where it lands,
// three switches, the soft warnings and the rule in words. Every change saves
// at once, like the rest of the picker.
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import {
  AddOutline,
  ChevronDownOutline,
  InformationCircleOutline,
  RemoveOutline,
  RepeatOutline,
} from '@vicons/ionicons5'
import ActionMenu from '../kit/ActionMenu.vue'
import DueCalendar from './DueCalendar.vue'
import { useFormat } from '@/composables/useFormat'
import { everyLabel, unitLabel } from '@/utils/recurrence'
import { columnName } from '@/utils/defaultNames'
import { sortedColumns } from '@/utils/status'
import { capitalizeFirst } from '@/utils/format'
import { weekOrder } from '@/utils/dueModel'

const props = defineProps({
  model: { type: Object, required: true },
  columns: { type: Array, default: () => [] },
  // The phone's sheet page: bigger targets.
  mobile: { type: Boolean, default: false },
})
const { t } = useI18n()
const { formatDate } = useFormat()
const m = props.model

const FREQS = ['daily', 'weekly', 'monthly', 'yearly', 'custom']
const TRIGGERS = ['complete', 'column', 'schedule']

const cols = computed(() => sortedColumns(props.columns))
const weekdayChips = computed(() =>
  weekOrder(m.weekStart).map((d) => ({
    d,
    label: formatDate(new Date(Date.UTC(2026, 2, 1 + d)), { weekday: 'short', timeZone: 'UTC' }),
  })),
)
const everyText = computed(() => capitalizeFirst(everyLabel(m.freq, m.interval)))
const unitText = computed(() => {
  const u = unitLabel(m.freq, m.interval)
  return m.freq === 'weekly' ? t('task.due.rep.unitByDays', { unit: u }) : u
})

// ── column selects ──
const menuOpen = ref('') // 'trigger' | 'target'
const triggerBtn = ref(null)
const targetBtn = ref(null)
const colName = (id) => columnName(cols.value.find((c) => c.id === id))
const triggerLabel = computed(() =>
  m.triggerColumn && colName(m.triggerColumn)
    ? colName(m.triggerColumn)
    : m.triggerColumn
      ? t('task.due.rep.columnGoneShort')
      : t('task.recur.columnPlaceholder'),
)
const targetLabel = computed(() =>
  m.targetColumn && colName(m.targetColumn)
    ? colName(m.targetColumn)
    : t('task.due.rep.firstColumn', { name: columnName(cols.value[0]) || '—' }),
)
const menuItems = computed(() => {
  const cur = menuOpen.value === 'trigger' ? m.triggerColumn : m.targetColumn
  const items = cols.value.map((c) => ({
    key: c.id,
    label: columnName(c),
    dot: c.color || undefined,
    checked: c.id === cur,
  }))
  if (menuOpen.value === 'target')
    items.unshift({
      key: '',
      label: t('task.due.rep.firstColumn', { name: columnName(cols.value[0]) || '—' }),
      checked: !cur,
    })
  return items
})
function pickColumn(id) {
  if (menuOpen.value === 'trigger') m.setTriggerColumn(id || null)
  else m.setTargetColumn(id || '')
  menuOpen.value = ''
}

// ── «По датам»: its own grid ──
const dview = ref(null)
const datesView = computed({
  get: () => dview.value || m.view,
  set: (v) => (dview.value = v),
})

const SWITCHES = computed(() => [
  {
    key: 'dup',
    on: m.createNew,
    set: m.setCreateNew,
    title: t('task.recur.createDuplicate'),
    sub: t('task.due.rep.dupSub'),
  },
  {
    key: 'forever',
    on: m.recurForever,
    set: m.setForever,
    title: t('task.recur.forever'),
    sub: t('task.due.rep.foreverSub'),
  },
  ...(m.skipWeekendsOffered
    ? [
        {
          key: 'weekends',
          on: m.skipWeekends,
          set: m.setSkipWeekends,
          title: t('task.recur.skipWeekends'),
          sub: t('task.due.rep.weekendsSub'),
        },
      ]
    : []),
])
const warning = computed(() => {
  if (m.deadNoDue) return t('task.due.rep.warnNoDue')
  if (m.deadTriggerColumn)
    return m.previewRule?.trigger_column ? t('task.recur.columnGone') : t('task.recur.columnUnset')
  return ''
})
</script>

<template>
  <div class="drp" :class="{ mobile }" data-testid="due-repeat">
    <div class="drp-sec">{{ t('task.due.rep.freq') }}</div>
    <div class="drp-seg" role="radiogroup" :aria-label="t('task.due.rep.freq')">
      <button
        v-for="f in FREQS"
        :key="f"
        type="button"
        class="drp-sego"
        :class="{ on: m.freq === f }"
        role="radio"
        :aria-checked="m.freq === f"
        :data-freq="f"
        @click="m.setFreq(f)"
      >
        {{ t(`task.due.rep.freqs.${f}`) }}
      </button>
    </div>

    <template v-if="m.freq">
      <div v-if="m.freq !== 'custom'" class="drp-every">
        <span class="drp-muted">{{ everyText }}</span>
        <span class="drp-step">
          <button
            type="button"
            class="drp-stepb"
            :aria-label="t('task.due.rep.less')"
            data-testid="due-rep-less"
            :disabled="m.interval <= 1"
            @click="m.setInterval(m.interval - 1)"
          >
            <n-icon :component="RemoveOutline" :size="13" />
          </button>
          <span class="drp-n" data-testid="due-rep-interval">{{ m.interval }}</span>
          <button
            type="button"
            class="drp-stepb"
            :aria-label="t('task.due.rep.more')"
            data-testid="due-rep-more"
            @click="m.setInterval(m.interval + 1)"
          >
            <n-icon :component="AddOutline" :size="13" />
          </button>
        </span>
        <span class="drp-muted">{{ unitText }}</span>
      </div>

      <div v-if="m.freq === 'weekly'" class="drp-chips">
        <button
          v-for="w in weekdayChips"
          :key="w.d"
          type="button"
          class="drp-chip"
          :class="{ on: m.weekdays.includes(w.d) }"
          :aria-pressed="m.weekdays.includes(w.d)"
          :data-wd="w.d"
          @click="m.toggleWeekday(w.d)"
        >
          {{ w.label }}
        </button>
      </div>

      <template v-if="m.freq === 'custom'">
        <div class="drp-hint">{{ t('task.due.rep.datesHint') }}</div>
        <DueCalendar
          v-model:view="datesView"
          class="drp-cal"
          :cell="mobile ? 40 : 30"
          :week-start="m.weekStart"
          :sel="m.dueEp?.day || null"
          :rep="m.pickedKeys"
          :today="m.today()"
          testid="due-rep-cal"
          @pick="m.toggleCustomDate"
        />
      </template>

      <div class="drp-sec">{{ t('task.due.rep.when') }}</div>
      <div class="drp-seg" role="radiogroup" :aria-label="t('task.due.rep.when')">
        <button
          v-for="tr in TRIGGERS"
          :key="tr"
          type="button"
          class="drp-sego"
          :class="{ on: m.trigger === tr }"
          role="radio"
          :aria-checked="m.trigger === tr"
          :data-trigger="tr"
          @click="m.setTrigger(tr)"
        >
          {{ t(`task.due.rep.triggers.${tr}`) }}
        </button>
      </div>
      <button
        v-if="m.trigger === 'column'"
        ref="triggerBtn"
        type="button"
        class="drp-select"
        data-testid="due-rep-trigger-col"
        @click="menuOpen = menuOpen === 'trigger' ? '' : 'trigger'"
      >
        <span class="drp-sel-l">{{ t('task.recur.triggerColumn') }}</span>
        <span class="drp-sel-v" :class="{ muted: !m.triggerColumn }">{{ triggerLabel }}</span>
        <n-icon :component="ChevronDownOutline" :size="13" class="drp-chev" />
      </button>

      <div class="drp-sec">{{ t('task.due.rep.where') }}</div>
      <button
        ref="targetBtn"
        type="button"
        class="drp-select"
        data-testid="due-rep-target-col"
        @click="menuOpen = menuOpen === 'target' ? '' : 'target'"
      >
        <span class="drp-sel-v">{{ targetLabel }}</span>
        <n-icon :component="ChevronDownOutline" :size="13" class="drp-chev" />
      </button>
      <ActionMenu
        :show="!!menuOpen"
        :anchor="() => (menuOpen === 'trigger' ? triggerBtn : targetBtn)"
        placement="bottom-start"
        :offset="{ x: 0, y: 4 }"
        :width="260"
        :items="menuItems"
        in-place
        testid="due-rep-col-menu"
        @update:show="(v) => !v && (menuOpen = '')"
        @select="pickColumn"
      />

      <div class="drp-switches">
        <div
          v-for="s in SWITCHES"
          :key="s.key"
          class="drp-sw"
          :data-switch="s.key"
          @click="s.set(!s.on)"
        >
          <span class="drp-sw-t">
            <span>{{ s.title }}</span>
            <span class="drp-sw-sub">{{ s.sub }}</span>
          </span>
          <button
            type="button"
            role="switch"
            class="drp-toggle"
            :class="{ on: s.on }"
            :aria-checked="s.on"
            :aria-label="s.title"
            @click.stop="s.set(!s.on)"
          >
            <span class="drp-knob" />
          </button>
        </div>
      </div>

      <div class="drp-warn" :class="{ dead: !!warning }" data-testid="due-rep-warn">
        <n-icon :component="InformationCircleOutline" :size="15" class="drp-warn-i" />
        <span>{{ warning || t('task.due.rep.countsFromDue') }}</span>
      </div>
    </template>
    <div v-else class="drp-hint">{{ t('task.due.rep.pickFreq') }}</div>

    <div class="drp-sum" data-testid="due-rep-summary">
      <n-icon :component="RepeatOutline" :size="15" class="drp-sum-i" />
      <span class="drp-sum-t">{{ m.summary }}</span>
      <span v-if="m.nextLabel" class="drp-sum-n">{{
        t('task.due.rep.nextSuffix', { date: m.nextLabel })
      }}</span>
    </div>
  </div>
</template>

<style scoped>
.drp {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 0 6px 4px;
}
.drp-sec {
  padding: 8px 4px 4px;
  font-size: 11.5px;
  font-weight: 600;
  letter-spacing: 0.05em;
  text-transform: uppercase;
  color: var(--t-text3);
}
.drp-seg {
  display: flex;
  gap: 2px;
  padding: 3px;
  border-radius: 9px;
  background: var(--t-ctl);
}
.drp-sego {
  flex: 1;
  min-width: 0;
  height: 28px;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0 6px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--t-text2);
  font: inherit;
  font-size: 12.5px;
  white-space: nowrap;
  cursor: pointer;
}
.drp-sego:hover {
  color: var(--t-text1);
}
.drp-sego.on {
  background: var(--t-card);
  box-shadow: var(--t-sh-card);
  color: var(--t-text1);
  font-weight: 600;
}
.drp-every {
  display: flex;
  align-items: center;
  gap: 8px;
  padding-top: 6px;
}
.drp-muted {
  font-size: 13px;
  color: var(--t-text2);
}
.drp-step {
  height: 30px;
  display: flex;
  align-items: center;
  gap: 2px;
  padding: 0 3px;
  border-radius: 8px;
  background: var(--t-input-bg);
}
.drp-stepb {
  width: 24px;
  height: 24px;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: var(--t-text3);
  cursor: pointer;
}
.drp-stepb:hover:not(:disabled) {
  background: var(--t-hover);
  color: var(--t-text1);
}
.drp-stepb:disabled {
  opacity: 0.4;
  cursor: default;
}
.drp-n {
  min-width: 20px;
  text-align: center;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}
.drp-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.drp-chip {
  height: 30px;
  min-width: 30px;
  box-sizing: border-box;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0 11px;
  border: 0;
  border-radius: 999px;
  background: var(--t-ctl);
  color: var(--t-text1);
  font: inherit;
  font-size: 12.5px;
  white-space: nowrap;
  cursor: pointer;
}
.drp-chip:hover {
  background: var(--t-hover);
}
.drp-chip.on {
  background: linear-gradient(
    to top right,
    color-mix(in srgb, var(--t-primary) 16%, var(--t-pop)),
    color-mix(in srgb, var(--t-primary) 28%, var(--t-pop))
  );
  font-weight: 600;
}
.drp-hint {
  padding: 4px 4px 2px;
  font-size: 12.5px;
  line-height: 1.45;
  color: var(--t-text3);
}
.drp-cal {
  padding: 0 2px;
}
.drp-select {
  height: 34px;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 10px;
  border: 0;
  border-radius: 9px;
  background: var(--t-ctl);
  color: var(--t-text1);
  font: inherit;
  font-size: 13px;
  text-align: left;
  cursor: pointer;
}
.drp-select:hover {
  background: var(--t-hover);
}
.drp-sel-l {
  color: var(--t-text3);
  white-space: nowrap;
}
.drp-sel-v {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.drp-sel-v.muted {
  color: var(--t-text3);
}
.drp-chev {
  flex: none;
  color: var(--t-text3);
}
.drp-switches {
  display: flex;
  flex-direction: column;
  padding-top: 4px;
}
.drp-sw {
  display: flex;
  align-items: center;
  gap: 10px;
  min-height: 36px;
  padding: 2px 4px;
  cursor: pointer;
}
.drp-sw-t {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 1px;
  font-size: 13px;
}
.drp-sw-sub {
  font-size: 12px;
  color: var(--t-text3);
}
.drp-toggle {
  flex: 0 0 30px;
  width: 30px;
  height: 18px;
  display: flex;
  align-items: center;
  padding: 0 2px;
  box-sizing: border-box;
  border: 0;
  border-radius: 9px;
  background: var(--t-pill);
  cursor: pointer;
}
.drp-knob {
  width: 14px;
  height: 14px;
  border-radius: 50%;
  background: var(--t-text3);
  transition: transform 0.15s ease;
}
.drp-toggle.on {
  background: var(--t-accent-grad);
}
.drp-toggle.on .drp-knob {
  transform: translateX(12px);
  background: #fff;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.3);
}
.drp-warn {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 9px 10px;
  border-radius: 9px;
  background: linear-gradient(
    to top right,
    color-mix(in srgb, #f0a020 10%, var(--t-pop)),
    color-mix(in srgb, #f0a020 18%, var(--t-pop))
  );
  font-size: 12.5px;
  line-height: 1.45;
}
.drp-warn-i {
  flex: none;
  margin-top: 1px;
  color: #f0a020;
}
.drp-sum {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  padding: 10px;
  border-radius: 9px;
  background: var(--t-sub);
  font-size: 13px;
}
.drp-sum-i {
  flex: none;
  color: var(--t-primary);
}
.drp-sum-t {
  font-weight: 600;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.drp-sum-n {
  flex: none;
  color: var(--t-text3);
  white-space: nowrap;
}

/* The phone's page: 44px targets. */
.drp.mobile {
  padding: 0;
}
.drp.mobile .drp-seg {
  overflow-x: auto;
  scrollbar-width: none;
}
.drp.mobile .drp-sego {
  flex: 1 0 auto;
  height: 36px;
  font-size: 13.5px;
}
.drp.mobile .drp-chip {
  height: 38px;
  min-width: 38px;
  font-size: 14px;
}
.drp.mobile .drp-select {
  height: 44px;
  font-size: 15px;
}
.drp.mobile .drp-sw {
  min-height: 48px;
}
.drp.mobile .drp-sw-t {
  font-size: 15px;
}
.drp.mobile .drp-sum {
  flex-wrap: wrap;
}
</style>
