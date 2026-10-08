<script setup>
// The «nothing here» block (v2, #3107 — PackageE «Пустые состояния и ошибки»):
// an ionicons5 glyph on a 56px tile in a soft tone, an optional title (17/700),
// a line of text, and up to two actions in the default slot (each may carry its
// key). Replaces Naive's <n-empty>, whose broken-image glyph reads as an error.
//
// `size="small"` is the inline variant for panels and lists (a 40px tile, no
// big title); `medium` is a whole view or pane.
import { computed } from 'vue'
import { NIcon } from 'naive-ui'
import { FileTrayOutline } from '@vicons/ionicons5'

const props = defineProps({
  icon: { type: [Object, Function], default: () => FileTrayOutline },
  title: { type: String, default: '' },
  text: { type: String, default: '' },
  // 'small' for inline panels (sidebar/search), 'medium' for full views.
  size: { type: String, default: 'medium' },
  // The tile's hue: a hex or a CSS colour; the accent when empty.
  tone: { type: String, default: '' },
})

const tileStyle = computed(() => {
  const hue = props.tone || 'var(--t-primary)'
  return {
    background: `color-mix(in srgb, ${hue} 16%, transparent)`,
    color: `color-mix(in srgb, ${hue} 82%, var(--t-text1))`,
  }
})
</script>

<template>
  <div class="empty-state" :class="size">
    <span class="es-tile" :style="tileStyle">
      <n-icon :component="icon" class="es-icon" />
    </span>
    <div v-if="title" class="es-title">{{ title }}</div>
    <div v-if="text" class="es-text">{{ text }}</div>
    <div class="es-slot"><slot /></div>
  </div>
</template>

<style scoped>
.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 32px 16px;
  text-align: center;
  color: var(--t-text2);
}
.empty-state.small {
  padding: 18px 12px;
  gap: 6px;
}
/* A rounded tile, not a disc: the radius of the large surfaces (18), the soft
   wash of the tone under a glyph in that tone. */
.es-tile {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 56px;
  height: 56px;
  margin-bottom: 8px;
  border-radius: 18px;
}
.empty-state.small .es-tile {
  width: 40px;
  height: 40px;
  margin-bottom: 4px;
  border-radius: 13px;
}
.es-icon {
  font-size: 26px;
}
.empty-state.small .es-icon {
  font-size: 19px;
}
.es-title {
  font-size: 17px;
  font-weight: 700;
  line-height: 1.3;
  color: var(--t-text1);
}
.empty-state.small .es-title {
  font-size: 14px;
  font-weight: 650;
}
.es-text {
  max-width: 340px;
  font-size: 13px;
  line-height: 1.5;
  color: var(--t-text2);
}
.empty-state.small .es-text {
  max-width: 260px;
  font-size: 12px;
  color: var(--t-text3);
}
/* Up to two actions side by side, under the text. */
.es-slot {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: center;
  gap: 8px;
  margin-top: 8px;
}
.es-slot:empty {
  display: none;
}
</style>
