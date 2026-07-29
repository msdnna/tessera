<script setup>
import { ref, computed, watch } from 'vue'
import {
  NModal,
  NCard,
  NSelect,
  NInputNumber,
  NInput,
  NSwitch,
  NButton,
  useMessage,
} from 'naive-ui'
import { workspaces as wsApi, projects as projApi } from '@/api'
import { useWorkspacesStore } from '@/stores/workspaces'
import { DEFAULT_ESTIMATION, unitName, formatEstimate } from '@/utils/estimation'

// Editor for the two-level task-estimation config. `scope` is 'workspace' (the
// default every project inherits) or 'project' (an override). "Наследовать"
// stores null — the workspace falls back to the built-in default, a project to
// its workspace.
const props = defineProps({
  show: { type: Boolean, default: false },
  scope: { type: String, default: 'project' }, // 'workspace' | 'project'
  targetId: { type: String, default: '' },
  name: { type: String, default: '' },
  // The currently-stored config for this scope (null = inherit).
  value: { type: Object, default: null },
  // The config this scope falls back to when inheriting (for the preview line).
  inherited: { type: Object, default: () => DEFAULT_ESTIMATION },
})
const emit = defineEmits(['update:show'])

const store = useWorkspacesStore()
const message = useMessage()
const saving = ref(false)

const inherit = ref(true)
const unit = ref('time')
const hoursPerDay = ref(8)
const daysPerWeek = ref(5)
const pointsScale = ref('fibonacci')
const customLabel = ref('')

const unitOptions = [
  { label: 'Время', value: 'time' },
  { label: 'Стори-поинты', value: 'points' },
  { label: 'Свои единицы', value: 'custom' },
]
const scaleOptionList = [
  { label: 'Фибоначчи (1, 2, 3, 5, 8, 13)', value: 'fibonacci' },
  { label: 'Футболки (XS…XXL)', value: 'tshirt' },
  { label: 'Линейная (1…10)', value: 'linear' },
]

// Seed the form from the stored value whenever the modal opens.
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
  },
  { immediate: true },
)

const inheritLabel = computed(() => {
  const cfg = props.inherited || DEFAULT_ESTIMATION
  const u = unitName(cfg)
  if (cfg.unit === 'time')
    return `${u} · ${cfg.hours_per_day || 8}ч/день · ${cfg.days_per_week || 5}дн/неделя`
  return u
})

const title = computed(() =>
  props.scope === 'workspace' ? 'Оценка задач — по умолчанию' : `Оценка задач — ${props.name}`,
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

function buildConfig() {
  if (inherit.value) return null
  if (unit.value === 'points') return { unit: 'points', points_scale: pointsScale.value }
  if (unit.value === 'custom') return { unit: 'custom', custom_label: customLabel.value.trim() }
  return {
    unit: 'time',
    hours_per_day: hoursPerDay.value || 8,
    days_per_week: daysPerWeek.value || 5,
  }
}

async function save() {
  saving.value = true
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
    message.error(e.message)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <n-modal :show="show" @update:show="$emit('update:show', $event)">
    <n-card class="est-modal" :title="title" :bordered="false" role="dialog">
      <div class="est-form">
        <label class="est-inherit">
          <n-switch :value="inherit" @update:value="inherit = $event" />
          <span>Наследовать ({{ inheritLabel }})</span>
        </label>

        <template v-if="!inherit">
          <div class="est-field">
            <span class="est-flabel">Единица</span>
            <n-select v-model:value="unit" :options="unitOptions" />
          </div>

          <template v-if="unit === 'time'">
            <div class="est-field">
              <span class="est-flabel">Часов в рабочем дне</span>
              <n-input-number v-model:value="hoursPerDay" :min="1" :max="24" />
            </div>
            <div class="est-field">
              <span class="est-flabel">Дней в рабочей неделе</span>
              <n-input-number v-model:value="daysPerWeek" :min="1" :max="7" />
            </div>
          </template>

          <div v-else-if="unit === 'points'" class="est-field">
            <span class="est-flabel">Шкала</span>
            <n-select v-model:value="pointsScale" :options="scaleOptionList" />
          </div>

          <div v-else class="est-field">
            <span class="est-flabel">Название единицы</span>
            <n-input v-model:value="customLabel" placeholder="напр. у.е." :maxlength="24" />
          </div>

          <div class="est-preview">
            Пример: <b>{{ preview }}</b>
          </div>
        </template>
      </div>

      <template #footer>
        <div class="est-foot">
          <n-button tertiary @click="$emit('update:show', false)">Отмена</n-button>
          <n-button type="primary" :loading="saving" @click="save">Сохранить</n-button>
        </div>
      </template>
    </n-card>
  </n-modal>
</template>

<style scoped>
.est-modal {
  width: 420px;
  max-width: calc(100vw - 32px);
}
.est-form {
  display: flex;
  flex-direction: column;
  gap: 14px;
}
.est-inherit {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 13px;
  color: var(--t-text2);
  cursor: pointer;
}
.est-field {
  display: flex;
  flex-direction: column;
  gap: 5px;
}
.est-flabel {
  font-size: 12px;
  color: var(--t-text3);
}
.est-preview {
  font-size: 13px;
  color: var(--t-text2);
}
.est-foot {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
</style>
