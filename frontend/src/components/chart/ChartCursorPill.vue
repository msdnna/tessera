<script setup>
// Date/time chip riding the cursor guide-line, pinned to the chart's top edge.
// Teleported and position:fixed, so vertical scrolling never hides it.
defineProps({
  x: { type: Number, required: true },
  y: { type: Number, required: true },
  label: { type: String, default: '' },
})
</script>

<template>
  <Teleport to="body">
    <div class="tl-cursor-pill" :style="{ left: `${x}px`, top: `${y}px` }">{{ label }}</div>
  </Teleport>
</template>

<style scoped>
/* a muted neutral chip, not an accent element */
.tl-cursor-pill {
  position: fixed;
  z-index: 3500;
  transform: translateX(-50%);
  background: var(--t-surface);
  color: var(--t-text2);
  border: 1px solid var(--t-border);
  font-size: 11px;
  font-weight: 500;
  padding: 2px 7px;
  border-radius: 6px;
  white-space: nowrap;
  pointer-events: none;
  box-shadow: 0 2px 6px rgba(0, 0, 0, 0.16);
}
</style>
