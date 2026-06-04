<script setup>
import { computed } from 'vue'
import { NIcon } from 'naive-ui'
import { iconComponent, iconKind, sanitizeIconSvg } from '@/utils/projectIcons'

const props = defineProps({
  icon: { type: String, default: '' },
  initials: { type: String, default: '?' },
  size: { type: Number, default: 14 },
})

const kind = computed(() => iconKind(props.icon))
const comp = computed(() => iconComponent(props.icon))
const svg = computed(() => (kind.value === 'svg' ? sanitizeIconSvg(props.icon) : ''))
const px = computed(() => ({ width: props.size + 'px', height: props.size + 'px' }))
</script>

<template>
  <n-icon v-if="kind === 'curated'" :component="comp" :size="size" />
  <!-- eslint-disable-next-line vue/no-v-html -->
  <span v-else-if="kind === 'svg'" class="pi-svg" :style="px" v-html="svg" />
  <img v-else-if="kind === 'img'" :src="icon" class="pi-img" :style="px" alt="" />
  <template v-else>{{ initials }}</template>
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
