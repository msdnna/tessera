<script setup>
// Branded loader: the firmennyy tessera loader — the "t" mark whose corner tile
// grows to cover the glyph, then endlessly spins, unfolding into app views
// (kanban / list / timeline / gantt / matrix) on each turn. Geometry + timings
// live in @/utils/tesseraLoader (shared source of truth with the Android port).
// Drop it into an <n-spin> #icon slot, or use standalone (optionally with a
// label) as a full-area loading state.
import { ref, onMounted, onBeforeUnmount, watch } from 'vue'
import { mountTesseraLoader } from '@/utils/tesseraLoader'

const props = defineProps({
  size: { type: Number, default: 32 },
  label: { type: String, default: '' },
  // 'accent' → follows the active accent (--t-primary); 'white' → white on a
  // coloured backdrop (e.g. the purple splash / connection overlay).
  variant: { type: String, default: 'accent' },
})

const host = ref(null)
let handle = null

function mount() {
  if (!host.value) return
  handle?.destroy()
  // Accent → each tile/glyph carries the brand diagonal accent gradient (theme-
  // reactive via the App.vue #t-accent-grad-svg def). White → flat white for the
  // purple splash / coloured overlays. Mirrors the Android accentGradient/flat fill.
  const paint = props.variant === 'white' ? '#ffffff' : 'url(#t-accent-grad-svg)'
  handle = mountTesseraLoader(host.value, { size: props.size, paint })
}

onMounted(mount)
// Re-mount on size change (the SVG dimensions are baked at mount time).
watch(() => props.size, mount)
onBeforeUnmount(() => handle?.destroy())
</script>

<template>
  <span class="t-loader" :class="[{ 'has-label': label }, `v-${variant}`]">
    <span ref="host" class="t-stage" />
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
/* The loader glyph/tiles fill with `currentColor`; set the colour per variant. */
.t-stage {
  display: inline-flex;
  line-height: 0;
}
.v-accent .t-stage {
  color: var(--t-primary);
}
.v-white .t-stage {
  color: #ffffff;
}
.t-label {
  font-size: 13px;
  color: var(--t-text3);
  letter-spacing: 0.2px;
}
.v-white .t-label {
  color: rgba(255, 255, 255, 0.82);
}
</style>
