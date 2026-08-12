<script setup>
import { PRIORITY_COLORS } from '@/styles/tokens'

// Tasks with neither a start nor a due date can't be placed on the axis, so they
// sit under the chart as clickable chips. Same strip in both chart views.
defineProps({
  tasks: { type: Array, default: () => [] },
})
defineEmits(['open', 'menu'])
</script>

<template>
  <div v-if="tasks.length" class="tl-unsched">
    <span class="us-label">Без дат</span>
    <button
      v-for="t in tasks"
      :key="t.id"
      type="button"
      class="us-chip"
      :class="{ done: t.completed_at }"
      :style="{ '--chip': PRIORITY_COLORS[t.priority || 0] }"
      :title="t.title"
      @click="$emit('open', t.id)"
      @contextmenu.prevent.stop="$emit('menu', $event, t)"
    >
      {{ t.title }}
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
