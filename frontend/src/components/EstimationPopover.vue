<script setup>
// Editor for the two-level task-estimation config — a popover form at its door
// since the v2 interaction model (#2969 / #2983); it replaced EstimationModal.
// `scope` is 'workspace' (the default every project inherits) or 'project' (an
// override). The "inherit" switch stores null — the workspace falls back to the
// built-in default, a project to its workspace.
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NSwitch } from 'naive-ui'
import PopoverForm from './kit/PopoverForm.vue'
import FormField from './kit/FormField.vue'
import { workspaces as wsApi, projects as projApi } from '@/api'
import { useWorkspacesStore } from '@/stores/workspaces'
import { DEFAULT_ESTIMATION, unitName, formatEstimate } from '@/utils/estimation'
import { humanizeError } from '@/utils/errors'

const props = defineProps({
  show: { type: Boolean, default: false },
  scope: { type: String, default: 'project' }, // 'workspace' | 'project'
  targetId: { type: String, default: '' },
  name: { type: String, default: '' },
  // The currently-stored config for this scope (null = inherit).
  value: { type: Object, default: null },
  // The config this scope falls back to when inheriting (for the preview line).
  inherited: { type: Object, default: () => DEFAULT_ESTIMATION },
  anchor: { type: Function, default: () => null },
  placement: { type: String, default: 'right-start' },
  offset: { type: Object, default: () => ({ x: 4, y: -6 }) },
})
const emit = defineEmits(['update:show'])

const store = useWorkspacesStore()
const { t } = useI18n()
const saving = ref(false)
const error = ref('')

const inherit = ref(true)
const unit = ref('time')
const hoursPerDay = ref(8)
const daysPerWeek = ref(5)
const pointsScale = ref('fibonacci')
const customLabel = ref('')

// Computed, not constants — a module-level list of labels would freeze at the
// language of the first render (pitfall 1 of #2799).
const unitOptions = computed(() =>
  ['time', 'points', 'custom'].map((value) => ({ label: t(`task.estimate.config.unitOption.${value}`), value })),
)
const scaleOptions = computed(() =>
  ['fibonacci', 'tshirt', 'linear'].map((value) => ({ label: t(`task.estimate.config.scaleOption.${value}`), value })),
)

// Seed the form from the stored value whenever it opens.
watch(
  () => props.show,
  (s) => {
    if (!s) return
    const v = props.value
    inherit.value = !v
    unit.value = v?.unit || 'time'
    hoursPerDay.value = v?.hours_per_day || 8
    daysPerWeek.value = v?.days_per_week || 5
    pointsScale.value = v?.points_scale || 'fibonacci'
    customLabel.value = v?.custom_label || ''
    error.value = ''
  },
  { immediate: true },
)

const inheritLabel = computed(() => {
  const cfg = props.inherited || DEFAULT_ESTIMATION
  const u = unitName(cfg)
  if (cfg.unit === 'time')
    return t('task.estimate.config.inheritTime', { unit: u, hours: cfg.hours_per_day || 8, days: cfg.days_per_week || 5 })
  return u
})

const title = computed(() =>
  props.scope === 'workspace'
    ? t('task.estimate.config.titleWorkspace')
    : t('task.estimate.config.titleProject', { name: props.name }),
)

// A small live example of how a value renders under the chosen unit.
const previewCfg = computed(() => ({
  unit: unit.value,
  hours_per_day: hoursPerDay.value,
  days_per_week: daysPerWeek.value,
  points_scale: pointsScale.value,
  custom_label: customLabel.value,
}))
const preview = computed(() => {
  if (unit.value === 'time') return formatEstimate(30 * 60, previewCfg.value) // 30h example
  if (unit.value === 'points') return formatEstimate(5, previewCfg.value)
  return formatEstimate(8, previewCfg.value)
})

const clamp = (v, lo, hi, dflt) => {
  const n = Number(v)
  return Number.isFinite(n) && n >= lo && n <= hi ? Math.round(n) : dflt
}
function buildConfig() {
  if (inherit.value) return null
  if (unit.value === 'points') return { unit: 'points', points_scale: pointsScale.value }
  if (unit.value === 'custom') return { unit: 'custom', custom_label: customLabel.value.trim() }
  return { unit: 'time', hours_per_day: clamp(hoursPerDay.value, 1, 24, 8), days_per_week: clamp(daysPerWeek.value, 1, 7, 5) }
}

async function save() {
  saving.value = true
  error.value = ''
  try {
    const config = buildConfig()
    if (props.scope === 'workspace') {
      const res = await wsApi.setEstimation(props.targetId, config)
      store.setWorkspaceEstimation(props.targetId, res.data?.estimation ?? null)
    } else {
      const res = await projApi.setEstimation(props.targetId, config)
      store.setProjectEstimation(props.targetId, res.data?.estimation ?? null)
    }
    emit('update:show', false)
  } catch (e) {
    error.value = humanizeError(e)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <PopoverForm
    :show="show"
    :anchor="anchor"
    :placement="placement"
    :offset="offset"
    :width="380"
    :title="title"
    :primary="t('common.action.save')"
    :busy-label="t('common.form.saving')"
    :busy="saving"
    testid="estimation-form"
    @update:show="emit('update:show', $event)"
    @submit="save"
  >
    <FormField :error="error">
      <label class="est-inherit">
        <n-switch :value="inherit" size="small" @update:value="inherit = $event" />
        <span>{{ t('task.estimate.config.inherit', { value: inheritLabel }) }}</span>
      </label>
    </FormField>

    <template v-if="!inherit">
      <FormField :label="t('task.estimate.config.unitField')">
        <div class="est-seg" role="radiogroup">
          <button
            v-for="o in unitOptions"
            :key="o.value"
            type="button"
            class="est-seg-btn"
            :class="{ on: unit === o.value }"
            role="radio"
            :aria-checked="unit === o.value"
            @click="unit = o.value"
          >
            {{ o.label }}
          </button>
        </div>
      </FormField>

      <div v-if="unit === 'time'" class="est-pair">
        <FormField :label="t('task.estimate.config.hoursPerDay')">
          <input v-model.number="hoursPerDay" class="kf-input" type="number" min="1" max="24" />
        </FormField>
        <FormField :label="t('task.estimate.config.daysPerWeek')">
          <input v-model.number="daysPerWeek" class="kf-input" type="number" min="1" max="7" />
        </FormField>
      </div>

      <FormField v-else-if="unit === 'points'" :label="t('task.estimate.config.scale')">
        <div class="est-seg" role="radiogroup">
          <button
            v-for="o in scaleOptions"
            :key="o.value"
            type="button"
            class="est-seg-btn"
            :class="{ on: pointsScale === o.value }"
            role="radio"
            :aria-checked="pointsScale === o.value"
            @click="pointsScale = o.value"
          >
            {{ o.label }}
          </button>
        </div>
      </FormField>

      <FormField v-else :label="t('task.estimate.config.customLabel')">
        <input
          v-model="customLabel"
          class="kf-input"
          maxlength="24"
          :placeholder="t('task.estimate.config.customPlaceholder')"
        />
      </FormField>

      <FormField :message="`${t('task.estimate.config.example')} ${preview}`" />
    </template>
  </PopoverForm>
</template>

<style scoped>
.est-inherit {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 13px;
  color: var(--t-text2);
  cursor: pointer;
}
.est-seg {
  display: flex;
  gap: 2px;
  padding: 3px;
  border-radius: 10px;
  background: var(--t-input-bg);
}
.est-seg-btn {
  flex: 1;
  height: 30px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--t-text2);
  font: inherit;
  font-size: 12.5px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  cursor: pointer;
}
.est-seg-btn:hover {
  background: var(--t-hover);
}
.est-seg-btn.on {
  background: var(--t-pop);
  box-shadow: var(--t-sh-card);
  color: var(--t-text1);
  font-weight: 600;
}
.est-pair {
  display: grid;
  grid-template-columns: 1fr 1fr;
  align-items: end;
  gap: 10px;
}
</style>
