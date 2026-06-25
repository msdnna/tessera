<script setup>
import { computed } from 'vue'
import { NIcon } from 'naive-ui'
import { iconComponent, iconKind, sanitizeIconSvg } from '@/utils/projectIcons'

const props = defineProps({
  icon: { type: String, default: '' },
  initials: { type: String, default: '?' },
  size: { type: Number, default: 14 },
  // When set, tints the glyph itself (currentColor-based icons + initials) — used
  // by "icon" colour mode, where the badge box is transparent. Raster/own-fill
  // SVGs keep their colours.
  color: { type: String, default: '' },
})

const kind = computed(() => iconKind(props.icon))
const comp = computed(() => iconComponent(props.icon))
const svg = computed(() => (kind.value === 'svg' ? sanitizeIconSvg(props.icon) : ''))
const px = computed(() => ({ width: props.size + 'px', height: props.size + 'px' }))
const tint = computed(() => (props.color ? { color: props.color } : {}))
</script>

<template>
  <n-icon v-if="kind === 'curated'" :component="comp" :size="size" :color="color || undefined" />
  <!-- eslint-disable-next-line vue/no-v-html -->
  <span v-else-if="kind === 'svg'" class="pi-svg" :style="{ ...px, ...tint }" v-html="svg" />
  <img v-else-if="kind === 'img'" :src="icon" class="pi-img" :style="px" alt="" />
  <span v-else :style="tint">{{ initials }}</span>
</template>

<style scoped>
.pi-svg {
  display: inline-flex;
  align-items: center;
  justify-content: center;
}
.pi-svg :deep(svg) {
  width: 100%;
  height: 100%;
}
.pi-img {
  object-fit: contain;
  display: block;
}
</style>
