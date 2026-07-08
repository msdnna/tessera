<script setup>
import { computed } from 'vue'

// Custom Tessera icon pack (design/icons, ionicons-5 style, 512 grid, currentColor).
// Covers view layouts (layout-*) and kanban column statuses (status-*), each in
// outline / sharp / filled. Rendered inline so `currentColor` + the column-status
// gradient selectors keep working; sized in `em` like n-icon.
const mods = import.meta.glob('../assets/view-icons/*.svg', {
  query: '?raw',
  import: 'default',
  eager: true,
})
const REG = {}
for (const p in mods) {
  const m = p.match(/([^/]+)\.svg$/)
  if (m) REG[m[1]] = mods[p]
}

const props = defineProps({
  name: { type: String, required: true }, // e.g. 'layout-kanban' | 'status-done'
  variant: { type: String, default: 'outline' }, // 'outline' | 'sharp' | 'filled'
  size: { type: [Number, String], default: null }, // px; omit to inherit font-size
})
const svg = computed(
  () => REG[`${props.name}-${props.variant}`] || REG[`${props.name}-outline`] || '',
)
const sizeStyle = computed(() => (props.size ? { fontSize: `${parseFloat(props.size)}px` } : {}))
</script>

<template>
  <!-- eslint-disable-next-line vue/no-v-html -- bundled static SVG assets, not user input -->
  <span class="t-vicon" :style="sizeStyle" v-html="svg" />
</template>

<style scoped>
.t-vicon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 1em;
  height: 1em;
  line-height: 0;
}
.t-vicon :deep(svg) {
  width: 1em;
  height: 1em;
  display: block;
}
</style>
