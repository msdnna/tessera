<script setup>
// «Этап · M» (#3118, mockups PropPicker f=stage / stageGl): a search over the
// project's stages, «Без этапа» (0), «Активные» with their progress and days
// left, «Запланированные», «Закрытые» folded behind a row, and «Создать этап…»
// (⌘↵) — whose second step is a popover form with the dates at the same
// anchor. A stage that comes from GitLab carries a lock: the stage itself is
// changed in GitLab only (#3010), a task can still be put on it.
//
// The host sets the stage (`set` — and journals it: every path that changes
// milestone_id must, see journalMilestoneActor); a new stage is created here
// and handed over through `created` + `set`.
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useMessage } from 'naive-ui'
import PropPicker from '../../kit/PropPicker.vue'
import AnchorLayer from '../../kit/AnchorLayer.vue'
import PopoverForm from '../../kit/PopoverForm.vue'
import FormField from '../../kit/FormField.vue'
import { projects as projApi } from '@/api'
import { useFormat } from '@/composables/useFormat'
import { hasModKey } from '@/utils/keys'
import { humanizeError } from '@/utils/errors'
import { footerFor, milestoneItems, WIDTHS } from './propItems'

const props = defineProps({
  show: { type: Boolean, default: false },
  anchor: { type: Function, default: () => null },
  placement: { type: String, default: 'below-or-side' },
  milestones: { type: Array, default: () => [] },
  current: { type: String, default: null },
  projectId: { type: String, default: '' },
  readonly: { type: String, default: '' },
  keyId: { type: String, default: 'task.milestone' },
  testid: { type: String, default: 'milestone-picker' },
})
const emit = defineEmits(['update:show', 'set', 'created'])
const { t } = useI18n()
const message = useMessage()
const { formatters } = useFormat()

const query = ref('')
const showClosed = ref(false)
// 'list' → the picker; 'form' → «Создать этап» with its dates.
const step = ref('list')
watch(
  () => props.show,
  (on) => {
    if (!on) return
    query.value = ''
    showClosed.value = false
    step.value = 'list'
  },
)

const built = computed(() =>
  milestoneItems({
    t,
    query: query.value,
    milestones: props.milestones,
    current: props.current,
    showClosed: showClosed.value,
    formatters: formatters.value,
    ops: {
      set: (id) => emit('set', id),
      toggleClosed: () => (showClosed.value = !showClosed.value),
      create: props.projectId ? (name) => openForm(name) : null,
    },
  }),
)
const footer = computed(() =>
  props.readonly ? footerFor('readonly', t) : footerFor('milestone', t),
)

function onKey(e) {
  if (props.readonly) return false
  if (e.key === 'Enter' && hasModKey(e) && props.projectId) {
    e.preventDefault()
    openForm(query.value.trim())
    return true
  }
  return false
}

// ── the second step ──
const form = ref({ title: '', start: '', due: '' })
const busy = ref(false)
const formError = ref('')
function openForm(name) {
  form.value = { title: name || '', start: '', due: '' }
  formError.value = ''
  step.value = 'form'
}
const listShow = computed(() => props.show && step.value === 'list')
const formShow = computed({
  get: () => props.show && step.value === 'form',
  set: (v) => !v && emit('update:show', false),
})
// A native date field gives the day as «YYYY-MM-DD»; stage dates are date-only
// — the UTC midnight of that day (utils/milestones toDateOnlyISO).
const day = (s) => (s ? `${s}T00:00:00.000Z` : null)
async function submit() {
  const title = form.value.title.trim()
  if (!title || busy.value || !props.projectId) return
  busy.value = true
  formError.value = ''
  try {
    const { data } = await projApi.createMilestone(props.projectId, {
      title,
      start_date: day(form.value.start),
      due_date: day(form.value.due),
    })
    emit('created', data)
    emit('set', data.id)
    emit('update:show', false)
  } catch (e) {
    formError.value = humanizeError(e)
    message.error(formError.value)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <PropPicker
    v-model:query="query"
    :show="listShow"
    :anchor="anchor"
    :placement="placement"
    :width="WIDTHS.milestone"
    :title="t('props.milestone.title')"
    :header="false"
    :key-id="keyId"
    search
    :placeholder="t('props.milestone.search')"
    :items="built.items"
    :cursor="built.cursor"
    :empty="{ title: t('props.milestone.nothing') }"
    :footer="footer"
    :readonly="readonly"
    :on-key="onKey"
    :testid="testid"
    @update:show="!$event && step === 'list' && emit('update:show', false)"
  />
  <AnchorLayer
    v-model:show="formShow"
    :anchor="anchor"
    :placement="placement"
    :offset="{ x: 0, y: 6 }"
    focus=".kf-input"
    :testid="`${testid}-form`"
  >
    <PopoverForm
      inline
      :show="formShow"
      :width="WIDTHS.milestone"
      :title="t('props.milestone.formTitle')"
      :primary="t('props.milestone.createBtn')"
      :busy="busy"
      :disabled="!form.title.trim()"
      hint="tab"
      :testid="`${testid}-form-card`"
      @update:show="formShow = $event"
      @submit="submit"
    >
      <FormField :label="t('props.milestone.name')" :error="formError">
        <input
          v-model="form.title"
          class="kf-input"
          :placeholder="t('props.milestone.namePlaceholder')"
          :data-testid="`${testid}-form-title`"
          autocomplete="off"
        />
      </FormField>
      <div class="mp-dates">
        <FormField :label="t('props.milestone.start')">
          <input
            v-model="form.start"
            type="date"
            class="kf-input"
            :data-testid="`${testid}-form-start`"
          />
        </FormField>
        <FormField :label="t('props.milestone.due')">
          <input
            v-model="form.due"
            type="date"
            class="kf-input"
            :min="form.start || undefined"
            :data-testid="`${testid}-form-due`"
          />
        </FormField>
      </div>
    </PopoverForm>
  </AnchorLayer>
</template>

<style scoped>
.mp-dates {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;
}
.mp-dates input[type='date'] {
  color-scheme: inherit;
}
</style>
