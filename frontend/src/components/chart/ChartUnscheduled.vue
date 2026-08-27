<script setup>
import { useI18n } from 'vue-i18n'
import { PRIORITY_COLORS } from '@/styles/tokens'

const { t } = useI18n()

// Tasks with neither a start nor a due date can't be placed on the axis, so they
// sit under the chart as clickable chips. Same strip in both chart views.
defineProps({
  tasks: { type: Array, default: () => [] },
})
defineEmits(['open', 'menu'])
</script>

<template>
  <div v-if="tasks.length" class="tl-unsched">
    <span class="us-label">{{ t('board.chart.unscheduled') }}</span>
    <button
      v-for="task in tasks"
      :key="task.id"
      type="button"
      class="us-chip"
      :class="{ done: task.completed_at }"
      :style="{ '--chip': PRIORITY_COLORS[task.priority || 0] }"
      :title="task.title"
      @click="$emit('open', task.id)"
      @contextmenu.prevent.stop="$emit('menu', $event, task)"
    >
      {{ task.title }}
    </button>
  </div>
</template>

<style scoped>
.tl-unsched {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  margin-top: 14px;
}
.us-label {
  font-size: 12px;
  font-weight: 600;
  color: var(--t-text3);
  margin-right: 4px;
}
.us-chip {
  max-width: 220px;
  text-align: left;
  border: none;
  border-left: 3px solid var(--chip, var(--t-primary));
  border-radius: 4px;
  background: var(--t-hover);
  color: var(--t-text1);
  font-size: 12px;
  padding: 3px 8px;
  cursor: pointer;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.us-chip.done {
  text-decoration: line-through;
  color: var(--t-text3);
}
</style>
