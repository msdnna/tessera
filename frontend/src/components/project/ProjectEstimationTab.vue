<script setup>
// «Оценка» of the «Проект» window (#2989, mockup ProjectWin «Оценка»): the
// project's estimate unit and the picker's scale. Laid out as in the mockup —
// a unit segment and a row of values — while keeping every mode the code has
// (decision in #2971): time with the working day and week, story points on a
// Fibonacci / linear scale, T-shirt sizes, a custom unit, and «inherit from
// the workspace». The scale's values are the project's own (#3029): ✕ drops
// one, «+» adds one typed in the unit («3ч», «5»). Saves at once.
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon, NSwitch } from 'naive-ui'
import { useMessage } from '@/composables/useMessage'
import { AddOutline, CloseOutline } from '@vicons/ionicons5'
import { projects as projApi } from '@/api'
import { useWorkspacesStore } from '@/stores/workspaces'
import {
  DEFAULT_ESTIMATION,
  resolveEstimation,
  unitName,
  pickerValues,
  pickerLabel,
  parseEstimateInput,
  estimatePlaceholder,
} from '@/utils/estimation'
import { humanizeError } from '@/utils/errors'
import '../kit/kit.css'

const props = defineProps({
  project: { type: Object, required: true },
})
const emit = defineEmits(['changed'])
const { t } = useI18n()
const message = useMessage()
const store = useWorkspacesStore()

// The unit segment of the mockup, with the code's two extra modes appended.
const KINDS = ['time', 'points', 'tshirt', 'custom']
const stored = computed(() => props.project.estimation || null)
const inherited = computed(() => resolveEstimation(null, store.current) || DEFAULT_ESTIMATION)
const inherit = computed(() => !stored.value)
const cfg = computed(() => stored.value || inherited.value)
const kind = computed(() => {
  const c = cfg.value
  if (c.unit === 'points') return c.points_scale === 'tshirt' ? 'tshirt' : 'points'
  return c.unit || 'time'
})

const hours = ref(8)
const days = ref(5)
const label = ref('')
watch(
  cfg,
  (c) => {
    hours.value = c.hours_per_day || 8
    days.value = c.days_per_week || 5
    label.value = c.custom_label || ''
  },
  { immediate: true },
)

const saving = ref(false)
async function save(next) {
  saving.value = true
  try {
    const res = await projApi.setEstimation(props.project.id, next)
    store.setProjectEstimation(props.project.id, res.data?.estimation ?? null)
    emit('changed')
  } catch (e) {
    message.error(humanizeError(e))
  } finally {
    saving.value = false
  }
}
// The stored config with a change applied; switching the unit drops the values
// (a scale in hours means nothing in points).
function withPatch(patch, { keepValues = true } = {}) {
  const base = { ...cfg.value }
  const out = { ...base, ...patch }
  if (!keepValues) delete out.values
  return out
}
function setKind(k) {
  if (k === kind.value && !inherit.value) return
  if (k === 'time') {
    save({ unit: 'time', hours_per_day: hours.value || 8, days_per_week: days.value || 5 })
  } else if (k === 'points') save({ unit: 'points', points_scale: 'fibonacci' })
  else if (k === 'tshirt') save({ unit: 'points', points_scale: 'tshirt' })
  else save({ unit: 'custom', custom_label: label.value.trim() })
}
function setScale(scale) {
  save(withPatch({ points_scale: scale }, { keepValues: false }))
}
function setInherit(on) {
  if (on) save(null)
  else save({ ...inherited.value })
}
const clamp = (v, lo, hi, d) => {
  const n = Math.round(Number(v))
  return Number.isFinite(n) && n >= lo && n <= hi ? n : d
}
function commitTime() {
  const h = clamp(hours.value, 1, 24, 8)
  const d = clamp(days.value, 1, 7, 5)
  hours.value = h
  days.value = d
  if (h === cfg.value.hours_per_day && d === cfg.value.days_per_week) return
  save(withPatch({ hours_per_day: h, days_per_week: d }))
}
function commitLabel() {
  const l = label.value.trim()
  if (l === (cfg.value.custom_label || '')) return
  save(withPatch({ custom_label: l }))
}

// ── the picker's scale ──
const values = computed(() => pickerValues(cfg.value))
const tshirt = computed(() => kind.value === 'tshirt')
function removeValue(v) {
  const next = values.value.filter((x) => x !== v)
  save(withPatch({ values: next.length ? next : [] }))
}
const adding = ref(false)
const addText = ref('')
const addErr = ref('')
const addEl = ref(null)
function startAdd() {
  adding.value = true
  addText.value = ''
  addErr.value = ''
  nextTick(() => addEl.value?.focus())
}
function commitAdd() {
  const raw = addText.value.trim()
  if (!raw) {
    adding.value = false
    return
  }
  const { value, ok } = parseEstimateInput(raw, cfg.value)
  if (!ok || value == null) {
    addErr.value = t('task.estimate.unreadable', { value: raw })
    return
  }
  adding.value = false
  if (values.value.includes(value)) return
  save(withPatch({ values: [...values.value, value].sort((a, b) => a - b) }))
}
const customized = computed(() => Array.isArray(cfg.value.values) && cfg.value.values.length > 0)
function resetValues() {
  save(withPatch({}, { keepValues: false }))
}

const inheritLabel = computed(() => {
  const c = inherited.value
  if (c.unit === 'time')
    return t('task.estimate.config.inheritTime', {
      unit: unitName(c),
      hours: c.hours_per_day || 8,
      days: c.days_per_week || 5,
    })
  return unitName(c)
})

const firstEl = ref(null)
function focusFirst() {
  nextTick(() => firstEl.value?.querySelector('button')?.focus())
}
function escape() {
  if (adding.value) {
    adding.value = false
    return true
  }
  return false
}
defineExpose({ focusFirst, escape })
</script>

<template>
  <div class="pe" data-testid="project-estimation">
    <div class="pe-title">
      <h2>{{ t('project.window.tab.estimation') }}</h2>
      <span class="pe-note">{{ t('project.window.general.autosave') }}</span>
    </div>

    <div class="pe-row">
      <div class="pe-lbl">{{ t('project.window.estimation.source') }}</div>
      <label class="pe-inherit">
        <n-switch :value="inherit" size="small" :disabled="saving" @update:value="setInherit" />
        <span>{{ t('task.estimate.config.inherit', { value: inheritLabel }) }}</span>
      </label>
    </div>

    <div class="pe-row" :class="{ dim: inherit }">
      <div class="pe-lbl">{{ t('task.estimate.config.unitField') }}</div>
      <div ref="firstEl" class="pe-seg" role="radiogroup" data-testid="estimation-unit">
        <button
          v-for="k in KINDS"
          :key="k"
          type="button"
          class="pe-seg-btn"
          :class="{ on: kind === k }"
          role="radio"
          :aria-checked="kind === k"
          :disabled="saving"
          :data-kind="k"
          @click="setKind(k)"
        >
          {{ t(`project.window.estimation.kind.${k}`) }}
        </button>
      </div>
    </div>

    <div v-if="kind === 'time'" class="pe-row" :class="{ dim: inherit }">
      <div class="pe-lbl">{{ t('project.window.estimation.workday') }}</div>
      <div class="pe-pair">
        <label class="pe-num">
          <input
            v-model.number="hours"
            class="kf-input"
            type="number"
            min="1"
            max="24"
            data-testid="estimation-hours"
            @keydown.enter.prevent="$event.target.blur()"
            @blur="commitTime"
          />
          <span>{{ t('project.window.estimation.hoursPerDay') }}</span>
        </label>
        <label class="pe-num">
          <input
            v-model.number="days"
            class="kf-input"
            type="number"
            min="1"
            max="7"
            @keydown.enter.prevent="$event.target.blur()"
            @blur="commitTime"
          />
          <span>{{ t('project.window.estimation.daysPerWeek') }}</span>
        </label>
      </div>
    </div>

    <div v-if="kind === 'points'" class="pe-row" :class="{ dim: inherit }">
      <div class="pe-lbl">{{ t('task.estimate.config.scale') }}</div>
      <div class="pe-seg">
        <button
          v-for="s in ['fibonacci', 'linear']"
          :key="s"
          type="button"
          class="pe-seg-btn"
          :class="{ on: cfg.points_scale === s || (s === 'fibonacci' && !cfg.points_scale) }"
          @click="setScale(s)"
        >
          {{ t(`task.estimate.config.scaleOption.${s}`) }}
        </button>
      </div>
    </div>

    <div v-if="kind === 'custom'" class="pe-row" :class="{ dim: inherit }">
      <div class="pe-lbl">{{ t('task.estimate.config.customLabel') }}</div>
      <input
        v-model="label"
        class="kf-input pe-narrow"
        :placeholder="t('task.estimate.config.customPlaceholder')"
        @keydown.enter.prevent="$event.target.blur()"
        @blur="commitLabel"
      />
    </div>

    <div class="pe-row" :class="{ dim: inherit }">
      <div>
        <div class="pe-lbl">{{ t('task.estimate.config.scale') }}</div>
        <div class="pe-sub">{{ t('project.window.estimation.scaleHint') }}</div>
      </div>
      <div class="pe-scale-wrap">
        <div class="pe-scale" data-testid="estimation-values">
          <span v-for="v in values" :key="v" class="pe-chip">
            {{ pickerLabel(v, cfg) }}
            <button
              v-if="!tshirt"
              type="button"
              class="pe-chip-x"
              :title="t('common.action.delete')"
              @click="removeValue(v)"
            >
              <n-icon :component="CloseOutline" :size="11" />
            </button>
          </span>
          <input
            v-if="adding"
            ref="addEl"
            v-model="addText"
            class="pe-add-in"
            :class="{ err: !!addErr }"
            :placeholder="estimatePlaceholder(cfg)"
            data-testid="estimation-value-input"
            @input="addErr = ''"
            @keydown.enter.prevent="commitAdd"
            @keydown.esc.stop.prevent="adding = false"
            @blur="commitAdd"
          />
          <button
            v-else-if="!tshirt"
            type="button"
            class="pe-add"
            :title="t('project.window.estimation.addValue')"
            data-testid="estimation-value-add"
            @click="startAdd"
          >
            <n-icon :component="AddOutline" :size="14" />
          </button>
        </div>
        <div v-if="addErr" class="pe-err">{{ addErr }}</div>
        <button v-else-if="customized" type="button" class="pe-reset" @click="resetValues">
          {{ t('project.window.estimation.resetValues') }}
        </button>
        <div v-if="kind === 'custom' && !values.length" class="pe-sub">
          {{ t('project.window.estimation.customEmpty') }}
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.pe {
  display: flex;
  flex-direction: column;
  gap: 26px;
  overflow-y: auto;
  padding: 6px 8px 24px 0;
}
.pe-title {
  display: flex;
  align-items: baseline;
  gap: 10px;
}
.pe-title h2 {
  margin: 0;
  font-size: 20px;
  font-weight: 700;
}
.pe-note {
  font-size: 13px;
  color: var(--t-text3);
}
.pe-row {
  display: grid;
  grid-template-columns: 200px minmax(0, 1fr);
  gap: 24px;
  align-items: start;
}
.pe-row.dim {
  opacity: 0.55;
  pointer-events: none;
}
.pe-lbl {
  font-size: 13.5px;
  font-weight: 650;
  padding-top: 8px;
}
.pe-sub {
  margin-top: 4px;
  font-size: 12.5px;
  color: var(--t-text3);
}
.pe-inherit {
  display: flex;
  align-items: center;
  gap: 10px;
  padding-top: 6px;
  font-size: 13.5px;
  color: var(--t-text2);
  cursor: pointer;
}
.pe-seg {
  display: inline-flex;
  flex-wrap: wrap;
  gap: 2px;
  width: fit-content;
  padding: 3px;
  border-radius: var(--t-r-ctl);
  background: var(--t-ctl);
}
.pe-seg-btn {
  height: 30px;
  padding: 0 12px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: var(--t-text2);
  font: inherit;
  font-size: 13.5px;
  cursor: pointer;
}
.pe-seg-btn:hover {
  color: var(--t-text1);
}
.pe-seg-btn.on {
  background: var(--t-card);
  box-shadow: var(--t-sh-card);
  color: var(--t-text1);
  font-weight: 650;
}
.pe-seg-btn:focus-visible {
  outline: 2px solid var(--t-primary);
  outline-offset: 1px;
}
.pe-pair {
  display: flex;
  flex-wrap: wrap;
  gap: 16px;
}
.pe-num {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: var(--t-text2);
}
.pe-num .kf-input {
  width: 72px;
}
.pe-narrow {
  max-width: 260px;
}
.pe-scale {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
}
.pe-chip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  height: 30px;
  padding: 0 6px 0 12px;
  border-radius: var(--t-r-ctl);
  background: var(--t-ctl);
  font-size: 13.5px;
  font-variant-numeric: tabular-nums;
}
.pe-chip-x {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  padding: 0;
  border: 0;
  border-radius: 5px;
  background: transparent;
  color: var(--t-text3);
  cursor: pointer;
  opacity: 0;
}
.pe-chip:hover .pe-chip-x,
.pe-chip-x:focus-visible {
  opacity: 1;
}
.pe-chip-x:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.pe-add {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  border: 0;
  border-radius: var(--t-r-ctl);
  background: transparent;
  color: var(--t-text2);
  cursor: pointer;
}
.pe-add:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.pe-add-in {
  width: 110px;
  height: 30px;
  padding: 0 10px;
  border: 0;
  outline: 0;
  border-radius: var(--t-r-ctl);
  background: var(--t-input-bg);
  color: var(--t-text1);
  font: inherit;
  font-size: 13.5px;
  box-shadow: 0 0 0 2px var(--t-primary);
}
.pe-add-in.err {
  box-shadow: 0 0 0 2px var(--t-error);
}
.pe-err {
  margin-top: 6px;
  font-size: 12.5px;
  color: var(--t-error);
}
.pe-reset {
  margin-top: 8px;
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--t-primary);
  font: inherit;
  font-size: 12.5px;
  cursor: pointer;
}
@media (max-width: 768px) {
  .pe-row {
    grid-template-columns: minmax(0, 1fr);
    gap: 8px;
  }
}
</style>
