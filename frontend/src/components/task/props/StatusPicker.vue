<script setup>
// «Статус · S» (#3118, mockup PropPicker f=status): the board's columns with
// their counts and digits 1–9, the done column marked «завершает», and two
// actions — «Сдвинуть в …» (]) and «Выполнить» (E). A search field only on a
// board of 8 columns or more. The host does the moving (`move`, `next`,
// `complete`).
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import PropPicker from '../../kit/PropPicker.vue'
import { keyOf } from '@/shortcuts/keymap'
import { hasModKey } from '@/utils/keys'
import { statusItems, footerFor, WIDTHS } from './propItems'

const props = defineProps({
  show: { type: Boolean, default: false },
  anchor: { type: Function, default: () => null },
  placement: { type: String, default: 'below-or-side' },
  columns: { type: Array, default: () => [] },
  currentId: { type: String, default: '' },
  doneId: { type: String, default: null },
  counts: { type: Object, default: null },
  completed: { type: Boolean, default: false },
  readonly: { type: String, default: '' },
  keyId: { type: String, default: 'task.status' },
  testid: { type: String, default: 'status-picker' },
})
const emit = defineEmits(['update:show', 'move', 'next', 'complete', 'cycle'])
const { t } = useI18n()

const query = ref('')
watch(
  () => props.show,
  (on) => on && (query.value = ''),
)
const search = computed(() => props.columns.length >= 8)
const built = computed(() =>
  statusItems({
    t,
    columns: props.columns,
    currentId: props.currentId,
    doneId: props.doneId,
    counts: props.counts,
    completed: props.completed,
    query: query.value,
    ops: {
      move: (id) => emit('move', id),
      next: () => emit('next'),
      complete: () => emit('complete'),
    },
  }),
)
const footer = computed(() =>
  props.readonly ? footerFor('readonly', t) : footerFor('status', t, { n: props.columns.length }),
)

// ] and E act on the task from the list (in the search field they type).
function onKey(e, api) {
  if (props.readonly || hasModKey(e) || e.altKey) return false
  if (search.value && !api.navigated) return false
  const k = keyOf(e)
  if (k === ']' && !e.shiftKey) {
    e.preventDefault()
    const row = built.value.items.find((i) => i.id === 'act:next')
    if (row) api.pick(row)
    return true
  }
  if (k === 'e' && !e.shiftKey) {
    e.preventDefault()
    const row = built.value.items.find((i) => i.id === 'act:complete')
    if (row) api.pick(row)
    return true
  }
  return false
}
</script>

<template>
  <PropPicker
    v-model:query="query"
    :show="show"
    :anchor="anchor"
    :placement="placement"
    :width="WIDTHS.status"
    :title="t('props.status.title')"
    :key-id="keyId"
    :search="search"
    :placeholder="t('props.status.search')"
    :items="built.items"
    :cursor="built.cursor"
    :footer="footer"
    :readonly="readonly"
    cycle-key="s"
    :on-key="onKey"
    :testid="testid"
    @update:show="emit('update:show', $event)"
    @cycle="emit('cycle')"
  />
</template>
