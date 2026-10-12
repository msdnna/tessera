<script setup>
// «Потрачено» (#2965): log time on a task and see what was logged. A tier-1
// popover form at the estimate row of the task modal — «1ч 30м» and, if you
// like, what it went on; the entries under it, newest day first, each removable
// by its author (or a workspace admin — the server decides). The field reads
// like the estimate in time units, on the project's working day.
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { tasks as tasksApi } from '@/api'
import { useMessage } from '@/composables/useMessage'
import { useFormat } from '@/composables/useFormat'
import { humanizeError } from '@/utils/errors'
import { formatEstimate, parseEstimateInput } from '@/utils/estimation'
import PopoverForm from '../kit/PopoverForm.vue'
import FormField from '../kit/FormField.vue'
import ArmButton from '../kit/ArmButton.vue'

const props = defineProps({
  show: { type: Boolean, default: false },
  taskId: { type: String, default: null },
  // The project's estimation config: its working day reads «1д».
  cfg: { type: Object, default: null },
})
const emit = defineEmits(['update:show', 'changed'])
const { t } = useI18n()
const message = useMessage()
const { formatDate } = useFormat()

const timeCfg = computed(() => ({ ...(props.cfg || {}), unit: 'time' }))
const spentLabel = (m) => formatEstimate(m, timeCfg.value)

const entries = ref([])
const total = ref({ spent: 0, spent_total: 0 })
const amount = ref('')
const note = ref('')
const error = ref('')
const busy = ref(false)
const field = ref(null)

async function load() {
  if (!props.taskId) return
  try {
    const { data } = await tasksApi.time(props.taskId)
    entries.value = data.entries || []
    total.value = { spent: data.spent || 0, spent_total: data.spent_total || 0 }
    emit('changed', total.value)
  } catch (e) {
    message.error(humanizeError(e))
  }
}
watch(
  () => props.show,
  async (on) => {
    if (!on) return
    amount.value = ''
    note.value = ''
    error.value = ''
    load()
    await nextTick()
    field.value?.focus()
  },
  { immediate: true },
)

const parsed = computed(() => parseEstimateInput(amount.value, timeCfg.value))
async function submit() {
  const { value, ok } = parsed.value
  if (!ok || !value) {
    error.value = t('task.time.unreadable')
    return
  }
  busy.value = true
  error.value = ''
  try {
    await tasksApi.addTime(props.taskId, { minutes: Math.round(value), note: note.value })
    amount.value = ''
    note.value = ''
    await load()
    field.value?.focus()
  } catch (e) {
    error.value = humanizeError(e)
  } finally {
    busy.value = false
  }
}
async function remove(e) {
  try {
    await tasksApi.deleteTime(props.taskId, e.id)
    await load()
  } catch (err) {
    message.error(humanizeError(err))
  }
}
const subtitle = computed(() => {
  const { spent, spent_total: all } = total.value
  if (!all) return t('task.time.none')
  return all > spent
    ? t('task.time.totalWithSub', { own: spentLabel(spent) || '0', all: spentLabel(all) })
    : t('task.time.total', { all: spentLabel(all) })
})
</script>

<template>
  <PopoverForm
    inline
    :show="show"
    :width="380"
    :title="t('task.time.title')"
    :subtitle="subtitle"
    :primary="t('task.time.log')"
    :busy="busy"
    :disabled="!amount.trim()"
    testid="time-log"
    @update:show="emit('update:show', $event)"
    @submit="submit"
  >
    <div class="tl-row">
      <FormField :label="t('task.time.amount')" :error="error">
        <input
          ref="field"
          v-model="amount"
          class="kf-input"
          :placeholder="t('task.time.amountPlaceholder')"
          autocomplete="off"
          data-testid="time-log-amount"
          @input="error = ''"
        />
      </FormField>
      <FormField :label="t('task.time.note')">
        <input
          v-model="note"
          class="kf-input"
          :placeholder="t('task.time.notePlaceholder')"
          maxlength="500"
          data-testid="time-log-note"
        />
      </FormField>
    </div>
    <div v-if="entries.length" class="tl-list" data-testid="time-log-entries">
      <div
        v-for="e in entries.slice(0, 8)"
        :key="e.id"
        class="tl-entry"
        data-testid="time-log-entry"
      >
        <span class="tl-day">{{ formatDate(e.spent_on, { day: 'numeric', month: 'short' }) }}</span>
        <span class="tl-who">{{ e.user_name }}</span>
        <span class="tl-what">{{ e.note }}</span>
        <span class="tl-min">{{ spentLabel(e.minutes) }}</span>
        <ArmButton
          :title="t('task.time.remove')"
          :confirm="t('task.time.removeConfirm')"
          testid="time-log-remove"
          @confirm="remove(e)"
        />
      </div>
      <div v-if="entries.length > 8" class="tl-more">
        {{ t('task.time.more', { n: entries.length - 8 }) }}
      </div>
    </div>
  </PopoverForm>
</template>

<style scoped>
.tl-row {
  display: grid;
  grid-template-columns: 120px minmax(0, 1fr);
  gap: 10px;
}
.tl-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin-top: 4px;
}
.tl-entry {
  display: grid;
  grid-template-columns: 52px minmax(0, auto) minmax(0, 1fr) auto 28px;
  align-items: center;
  gap: 8px;
  min-height: 30px;
  padding: 0 4px 0 6px;
  border-radius: 8px;
  font-size: 12.5px;
}
.tl-entry:hover {
  background: var(--t-hover);
}
.tl-day,
.tl-what {
  color: var(--t-text3);
}
.tl-who,
.tl-what {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.tl-min {
  font-variant-numeric: tabular-nums;
  font-weight: 600;
}
.tl-more {
  padding: 4px 6px;
  font-size: 12px;
  color: var(--t-text3);
}
</style>
