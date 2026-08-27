<script setup>
import { useI18n } from 'vue-i18n'
import { NDropdown, NPopconfirm } from 'naive-ui'

const { t } = useI18n()

// The chart's right-click menu and its two confirmations. Presentational only: the
// useTaskMenu state stays owned by the view, which feeds it in and applies every
// close back — a child must not write through its props.
defineProps({
  show: { type: Boolean, default: false },
  x: { type: Number, default: 0 },
  y: { type: Number, default: 0 },
  options: { type: Array, default: () => [] },
  deleteShow: { type: Boolean, default: false },
  archiveShow: { type: Boolean, default: false },
})
defineEmits([
  'select',
  'close',
  'update:deleteShow',
  'delete-confirm',
  'update:archiveShow',
  'archive-confirm',
])
</script>

<template>
  <n-dropdown
    trigger="manual"
    placement="bottom-start"
    :show="show"
    :x="x"
    :y="y"
    :options="options"
    @select="$emit('select', $event)"
    @clickoutside="$emit('close')"
  />
  <n-popconfirm
    :show="deleteShow"
    :x="x"
    :y="y"
    :positive-button-props="{ type: 'error' }"
    :positive-text="t('task.confirm.deleteYes')"
    @update:show="$emit('update:deleteShow', $event)"
    @positive-click="$emit('delete-confirm')"
    @clickoutside="$emit('update:deleteShow', false)"
  >
    <template #trigger><span /></template>
    {{ t('task.confirm.delete') }}
  </n-popconfirm>
  <n-popconfirm
    :show="archiveShow"
    :x="x"
    :y="y"
    :positive-text="t('task.confirm.archiveYes')"
    @update:show="$emit('update:archiveShow', $event)"
    @positive-click="$emit('archive-confirm')"
    @clickoutside="$emit('update:archiveShow', false)"
  >
    <template #trigger><span /></template>
    {{ t('task.confirm.archive') }}
  </n-popconfirm>
</template>
