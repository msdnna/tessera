<script setup>
// «Приоритет · P» (#3118, mockup PropPicker f=prio): Срочный … Без приоритета
// with the digits 4…0 — the same digits as P+digit in the modal. No search.
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import PropPicker from '../../kit/PropPicker.vue'
import { priorityItems, footerFor, WIDTHS } from './propItems'

const props = defineProps({
  show: { type: Boolean, default: false },
  anchor: { type: Function, default: () => null },
  placement: { type: String, default: 'below-or-side' },
  current: { type: Number, default: 0 },
  readonly: { type: String, default: '' },
  keyId: { type: String, default: 'task.priorityPick' },
  testid: { type: String, default: 'priority-picker' },
})
const emit = defineEmits(['update:show', 'set', 'cycle'])
const { t } = useI18n()

const built = computed(() =>
  priorityItems({ current: props.current, ops: { set: (p) => emit('set', p) } }),
)
const footer = computed(() =>
  props.readonly ? footerFor('readonly', t) : footerFor('priority', t),
)
</script>

<template>
  <PropPicker
    :show="show"
    :anchor="anchor"
    :placement="placement"
    :width="WIDTHS.priority"
    :title="t('props.priority.title')"
    :key-id="keyId"
    :items="built.items"
    :cursor="built.cursor"
    :footer="footer"
    :readonly="readonly"
    cycle-key="p"
    :testid="testid"
    @update:show="emit('update:show', $event)"
    @cycle="emit('cycle')"
  />
</template>
