<script setup>
// Branded loader: the Tessera tile (accent-gradient rounded square) tumbling
// with a gentle scale pulse. Follows the active accent theme via --t-accent-grad.
// Drop it into an <n-spin> #icon slot, or use standalone (optionally with a
// label) as a full-area loading state.
defineProps({
  size: { type: Number, default: 32 },
  label: { type: String, default: '' },
})
</script>

<template>
  <span class="t-loader" :class="{ 'has-label': label }">
    <span class="t-tile" :style="{ width: size + 'px', height: size + 'px' }" />
    <span v-if="label" class="t-label">{{ label }}</span>
  </span>
</template>

<style scoped>
.t-loader {
  display: inline-flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
}
.t-tile {
  display: block;
  border-radius: 28%;
  background: var(--t-accent-grad);
  box-shadow: 0 4px 16px color-mix(in srgb, var(--t-primary) 38%, transparent);
  animation: t-tile-tumble 1.25s cubic-bezier(0.55, 0.1, 0.45, 0.95) infinite;
  will-change: transform;
}
.t-label {
  font-size: 13px;
  color: var(--t-text3);
  letter-spacing: 0.2px;
}
@keyframes t-tile-tumble {
  0% {
    transform: rotate(0deg) scale(0.86);
  }
  50% {
    transform: rotate(180deg) scale(1);
  }
  100% {
    transform: rotate(360deg) scale(0.86);
  }
}
@media (prefers-reduced-motion: reduce) {
  .t-tile {
    animation-duration: 2.4s;
    animation-timing-function: linear;
  }
}
</style>
