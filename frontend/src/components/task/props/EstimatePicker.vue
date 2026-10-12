<script setup>
// «Оценка · O» (#3118, mockups PropPicker f=estP / estT / estTerr).
//
// Points: «Не задана» (0) and the project's scale with digits 1–7, the current
// value ticked. Time (and a custom unit): a field that reads «3д 4ч», the
// reading spelled out under it («= 1 день 2 часа · 10 ч при 8-часовом дне») or
// what went wrong — under the field, not as a toast — and chips of quick
// values. Both end with «Σ подзадач» (puts the sum in) and «Шкала проекта…»
// (the «Проект» window on «Оценка»).
//
// Keys of the field: ↵ applies what is typed (an empty field clears), ⌫ in an
// empty field clears, Tab goes to the chips (←→ walk them, ↵ applies, Tab
// back), ↓ to the rows under them.
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import { AlertCircleOutline, ArrowForwardOutline, TimeOutline } from '@vicons/ionicons5'
import PropPicker from '../../kit/PropPicker.vue'
import { hasModKey } from '@/utils/keys'
import {
  estimatePlaceholder,
  formatEstimate,
  formatEstimateFull,
  parseEstimateInput,
  pickerLabel,
  quickValues,
} from '@/utils/estimation'
import { estimateItems, footerFor, WIDTHS } from './propItems'

const props = defineProps({
  show: { type: Boolean, default: false },
  anchor: { type: Function, default: () => null },
  placement: { type: String, default: 'below-or-side' },
  // The project's estimation config (utils/estimation resolveEstimation).
  cfg: { type: Object, default: null },
  value: { type: Number, default: null },
  // Σ of the direct subtasks' estimates, or null.
  rollup: { type: Number, default: null },
  readonly: { type: String, default: '' },
  // The project the «Шкала проекта…» door opens; none — no door.
  canScale: { type: Boolean, default: true },
  keyId: { type: String, default: 'task.estimate' },
  testid: { type: String, default: 'estimate-picker' },
})
const emit = defineEmits(['update:show', 'set', 'scale'])
const { t } = useI18n()

const points = computed(() => props.cfg?.unit === 'points')
const draft = ref('')
const error = ref('')
const chipAt = ref(-1)
const inputEl = ref(null)
const chipsEl = ref(null)
watch(
  () => props.show,
  (on) => {
    if (!on) return
    draft.value = formatEstimate(props.value, props.cfg)
    error.value = ''
    chipAt.value = -1
  },
)
watch(draft, () => (error.value = ''))

const built = computed(() =>
  estimateItems({
    t,
    cfg: props.cfg,
    value: props.value,
    rollup: props.rollup,
    ops: {
      set: (v) => emit('set', v),
      openScale: props.canScale ? () => emit('scale') : null,
    },
  }),
)
const items = computed(() =>
  points.value
    ? built.value.items
    : [{ kind: 'slot', id: 'field', name: 'field' }, ...built.value.items],
)

const scaleName = computed(() => {
  const c = props.cfg
  if (!points.value) return ''
  if (Array.isArray(c?.values) && c.values.length) return t('props.estimate.scales.custom')
  return t(`props.estimate.scales.${c?.points_scale || 'fibonacci'}`)
})
const title = computed(() =>
  points.value
    ? t('props.estimate.titleScale', { scale: scaleName.value })
    : t('props.estimate.title'),
)

// What the field reads as, spelled out.
const parsed = computed(() => parseEstimateInput(draft.value, props.cfg))
const preview = computed(() => {
  const v = parsed.value.value
  if (!parsed.value.ok || v == null) return ''
  const full = formatEstimateFull(v, props.cfg)
  if ((props.cfg?.unit || 'time') !== 'time') return t('props.estimate.preview', { full })
  const day = props.cfg?.hours_per_day || 8
  const hours = Math.round((v / 60) * 10) / 10
  const tail = v >= day * 60 ? ` · ${t('props.estimate.previewDay', { hours, day })}` : ''
  return `${t('props.estimate.preview', { full })}${tail}`
})
const chips = computed(() =>
  quickValues(props.cfg).map((v) => ({ value: v, label: pickerLabel(v, props.cfg) })),
)

function apply() {
  const { value, ok } = parsed.value
  if (!ok) {
    error.value = t('props.estimate.unreadable', { value: draft.value.trim() })
    return false
  }
  emit('set', value)
  return true
}
function focusChip(i) {
  chipAt.value = i
  nextTick(() => chipsEl.value?.querySelectorAll('.ep-chip')[i]?.focus({ preventScroll: true }))
}
function backToField() {
  chipAt.value = -1
  inputEl.value?.focus({ preventScroll: true })
}

function onKey(e, api) {
  if (props.readonly || points.value) return false
  if (api.navigated && chipAt.value < 0) return false
  const mod = hasModKey(e)
  if (e.key === 'Tab' && chips.value.length) {
    e.preventDefault()
    if (chipAt.value < 0) focusChip(e.shiftKey ? chips.value.length - 1 : 0)
    else backToField()
    return true
  }
  if (chipAt.value >= 0) {
    if (e.key === 'ArrowRight' || e.key === 'ArrowLeft') {
      e.preventDefault()
      const n = chips.value.length
      focusChip((chipAt.value + (e.key === 'ArrowRight' ? 1 : -1) + n) % n)
      return true
    }
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault()
      emit('set', chips.value[chipAt.value].value)
      api.close()
      return true
    }
    if (e.key === 'ArrowDown') {
      chipAt.value = -1
      return false
    }
    return false
  }
  if (e.key === 'Enter' && !mod && !e.altKey) {
    e.preventDefault()
    if (apply()) api.close()
    return true
  }
  if (e.key === 'Backspace' && !draft.value) {
    e.preventDefault()
    emit('set', null)
    api.close()
    return true
  }
  return false
}
const footer = computed(() => {
  if (props.readonly) return footerFor('readonly', t)
  if (points.value) return footerFor('estimatePoints', t)
  if (error.value) return [{ caps: ['Tab'], label: t('props.foot.chips') }, '|', { caps: ['Esc'] }]
  return footerFor('estimateTime', t)
})
</script>

<template>
  <PropPicker
    :show="show"
    :anchor="anchor"
    :placement="placement"
    :width="points ? WIDTHS.estimate : WIDTHS.estimateTime"
    :title="title"
    :key-id="keyId"
    :items="items"
    :cursor="built.cursor"
    :footer="footer"
    :readonly="readonly"
    :focus="points ? '' : '.ep-input'"
    :on-key="onKey"
    :testid="testid"
    @update:show="emit('update:show', $event)"
  >
    <template #field>
      <div class="ep">
        <label class="ep-field" :class="{ bad: !!error }">
          <n-icon :component="TimeOutline" :size="14" class="ep-dim" />
          <input
            ref="inputEl"
            v-model="draft"
            class="ep-input"
            :placeholder="estimatePlaceholder(cfg)"
            :aria-label="t('props.estimate.title')"
            :aria-invalid="!!error"
            :disabled="!!readonly"
            :data-testid="`${testid}-input`"
            spellcheck="false"
            autocomplete="off"
            @focus="chipAt = -1"
          />
        </label>
        <div v-if="error" class="ep-err" role="alert" :data-testid="`${testid}-error`">
          <n-icon :component="AlertCircleOutline" :size="14" />
          <span>{{ error }}</span>
        </div>
        <div v-else-if="preview" class="ep-prev" :data-testid="`${testid}-preview`">
          <n-icon :component="ArrowForwardOutline" :size="12" class="ep-arrow" />
          <span>{{ preview }}</span>
        </div>
        <div v-if="chips.length" ref="chipsEl" class="ep-chips">
          <button
            v-for="(c, i) in chips"
            :key="c.value"
            type="button"
            class="ep-chip"
            :class="{ on: value === c.value, kf: chipAt === i }"
            tabindex="-1"
            :data-testid="`${testid}-chip`"
            :disabled="!!readonly"
            @mousedown.prevent
            @click="(emit('set', c.value), emit('update:show', false))"
          >
            {{ c.label }}
          </button>
        </div>
      </div>
    </template>
  </PropPicker>
</template>

<style scoped>
.ep {
  display: flex;
  flex-direction: column;
}
.ep-field {
  display: flex;
  align-items: center;
  gap: 8px;
  height: 36px;
  margin: 2px 2px 4px;
  padding: 0 6px 0 10px;
  border-radius: 9px;
  background: var(--t-input-bg);
  box-shadow: 0 0 0 2px var(--t-primary);
  cursor: text;
}
.ep-field.bad {
  box-shadow: 0 0 0 2px #e0533d;
}
.ep-dim {
  flex: none;
  color: var(--t-text3);
}
.ep-input {
  flex: 1;
  min-width: 0;
  height: 100%;
  padding: 0;
  border: 0;
  outline: 0;
  background: transparent;
  color: var(--t-text1);
  font: inherit;
  font-size: 13.5px;
}
.ep-input::placeholder {
  color: var(--t-placeholder);
}
.ep-prev,
.ep-err {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  padding: 0 10px 6px;
  font-size: 12.5px;
  line-height: 1.4;
}
.ep-prev {
  align-items: center;
  color: var(--t-text2);
}
.ep-arrow {
  flex: none;
  color: var(--t-primary);
}
.ep-err {
  color: #e0533d;
  text-wrap: pretty;
}
.ep-err .n-icon {
  flex: none;
  margin-top: 1px;
}
.ep-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  padding: 2px 8px 6px;
}
.ep-chip {
  height: 26px;
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
  outline: none;
}
.ep-chip:hover {
  background: var(--t-hover);
}
.ep-chip.on {
  background: linear-gradient(
    to top right,
    color-mix(in srgb, var(--t-primary) 16%, var(--t-pop)),
    color-mix(in srgb, var(--t-primary) 28%, var(--t-pop))
  );
  font-weight: 600;
}
.ep-chip.kf {
  box-shadow: 0 0 0 2px var(--t-primary);
}
/* A phone: a 44px field, bigger chips. */
.kit-layer:not(.al) .ep-field {
  height: 44px;
}
.kit-layer:not(.al) .ep-input {
  font-size: 16px;
}
.kit-layer:not(.al) .ep-chip {
  height: 34px;
  padding: 0 14px;
  font-size: 14px;
}
</style>
