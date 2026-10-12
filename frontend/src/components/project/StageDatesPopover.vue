<script setup>
// «Даты…» of a stage as a popover form at its row or its page header (#2971):
// start and due date in one range picker. Stage dates are date-only — what is
// saved is the UTC midnight of the picked days (utils/milestones toDateOnlyISO).
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import DateField from '@/components/kit/DateField.vue'
import PopoverForm from '../kit/PopoverForm.vue'
import FormField from '../kit/FormField.vue'
import { milestones as msApi } from '@/api'
import { toDateOnlyISO, pickerValue } from '@/utils/milestones'
import { humanizeError } from '@/utils/errors'
import { useStageSync } from '@/composables/useStageSync'

const props = defineProps({
  show: { type: Boolean, default: false },
  anchor: { type: Function, default: () => null },
  stage: { type: Object, default: null },
  placement: { type: String, default: 'bottom-start' },
})
const emit = defineEmits(['update:show', 'saved'])
const { t } = useI18n()
// A GitLab stage's dates go to GitLab with the save (#3010).
const sync = useStageSync()

const range = ref(null)
const busy = ref(false)
const error = ref('')
watch(
  () => props.show,
  (on) => {
    if (!on || !props.stage) return
    error.value = ''
    const s = pickerValue(props.stage.start_date)
    const d = pickerValue(props.stage.due_date)
    range.value = s || d ? [s || d, d || s] : null
  },
)

async function submit() {
  if (!props.stage || busy.value) return
  busy.value = true
  error.value = ''
  try {
    const m = props.stage
    const { data } = await msApi.update(m.id, {
      title: m.title,
      description: m.description || '',
      state: m.state,
      start_date: range.value ? toDateOnlyISO(range.value[0]) : null,
      due_date: range.value ? toDateOnlyISO(range.value[1]) : null,
    })
    sync.report(data)
    emit('saved', data)
    emit('update:show', false)
  } catch (e) {
    error.value = humanizeError(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <PopoverForm
    :show="show"
    :anchor="anchor"
    :placement="placement"
    :offset="{ x: 0, y: 6 }"
    :width="340"
    :title="t('milestones.stage.datesTitle')"
    :subtitle="stage?.title || ''"
    :primary="t('common.action.save')"
    :busy="busy"
    testid="stage-dates"
    @update:show="emit('update:show', $event)"
    @submit="submit"
  >
    <FormField :label="t('milestones.stage.dates')" :error="error">
      <DateField
        v-model:value="range"
        type="daterange"
        :placeholder="t('milestones.stage.dates')"
        testid="stage-dates-field"
      />
    </FormField>
  </PopoverForm>
</template>
