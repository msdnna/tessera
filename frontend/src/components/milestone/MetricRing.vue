<script setup>
// A progress ring of the stage page (#2993): 76px, 8px thick, one accent hue
// (the gradient the app draws accents with), the percent in the middle.
import { computed } from 'vue'
import { ringDash } from '@/utils/milestoneMetrics'

const props = defineProps({
  pct: { type: Number, default: 0 },
  size: { type: Number, default: 76 },
  stroke: { type: Number, default: 8 },
})
const r = computed(() => (props.size - props.stroke) / 2)
const dash = computed(() => ringDash(props.pct, r.value))
</script>

<template>
  <div class="ring" :style="{ width: `${size}px`, height: `${size}px` }">
    <svg :width="size" :height="size" :viewBox="`0 0 ${size} ${size}`" aria-hidden="true">
      <circle
        class="ring-track"
        :cx="size / 2"
        :cy="size / 2"
        :r="r"
        :stroke-width="stroke"
        fill="none"
      />
      <circle
        v-if="pct > 0"
        class="ring-fill"
        :cx="size / 2"
        :cy="size / 2"
        :r="r"
        :stroke-width="stroke"
        fill="none"
        stroke-linecap="round"
        :stroke-dasharray="dash.dasharray"
        :transform="`rotate(-90 ${size / 2} ${size / 2})`"
      />
    </svg>
    <span class="ring-n">{{ pct }}%</span>
  </div>
</template>

<style scoped>
.ring {
  position: relative;
  flex: none;
}
.ring-track {
  stroke: var(--t-pill);
}
.ring-fill {
  stroke: url(#t-accent-grad-svg);
}
.ring-n {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 17px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}
</style>
