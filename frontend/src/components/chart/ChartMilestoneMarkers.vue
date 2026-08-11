<script setup>
import { NIcon } from 'naive-ui'
import { RibbonOutline } from '@vicons/ionicons5'

// Project milestones as dashed vertical due-markers across the chart body, each
// with a small sticky label at the top. Positions come from useChartTimeline.
defineProps({
  markers: { type: Array, default: () => [] },
  leftW: { type: Number, required: true },
})
</script>

<template>
  <div v-for="m in markers" :key="m.id" class="tl-ms" :style="{ left: `${leftW + m.left}px` }">
    <span class="tl-ms-label"><n-icon :component="RibbonOutline" :size="11" /> {{ m.title }}</span>
  </div>
</template>

<style scoped>
/* milestone due-marker: dashed vertical line + a small label at the top */
.tl-ms {
  position: absolute;
  top: 0;
  bottom: 0;
  width: 0;
  border-left: 1.5px dashed color-mix(in srgb, var(--t-primary) 55%, transparent);
  z-index: 1;
  pointer-events: none;
}
.tl-ms-label {
  position: sticky;
  top: 2px;
  display: inline-flex;
  align-items: center;
  gap: 2px;
  margin-left: 3px;
  padding: 0 5px;
  height: 16px;
  border-radius: 8px;
  font-size: 10px;
  white-space: nowrap;
  color: var(--t-primary);
  background: color-mix(in srgb, var(--t-primary) 12%, var(--t-surface));
}
</style>
