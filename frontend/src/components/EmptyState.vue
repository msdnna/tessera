<script setup>
// Branded empty placeholder — a soft ionicons5 glyph over a tinted disc with a
// caption under it. Replaces Naive's <n-empty> (whose default broken-image icon
// reads as an error). Pass any ionicons5 component via `icon`; the optional
// default slot renders an action below the text.
import { NIcon } from 'naive-ui'
import { FileTrayOutline } from '@vicons/ionicons5'

defineProps({
  icon: { type: [Object, Function], default: () => FileTrayOutline },
  text: { type: String, default: '' },
  // 'small' for inline panels (sidebar/search), 'medium' for full views.
  size: { type: String, default: 'medium' },
})
</script>

<template>
  <div class="empty-state" :class="size">
    <span class="es-disc">
      <n-icon :component="icon" class="es-icon" />
    </span>
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
  gap: 10px;
  padding: 28px 16px;
  text-align: center;
  color: var(--t-text3);
}
.empty-state.small {
  padding: 18px 12px;
  gap: 7px;
}
.es-disc {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 56px;
  height: 56px;
  border-radius: 50%;
  /* Faint accent tint — keeps the placeholder on-brand without shouting. */
  background: color-mix(in srgb, var(--t-primary) 12%, transparent);
  color: color-mix(in srgb, var(--t-primary) 72%, var(--t-text3));
}
.empty-state.small .es-disc {
  width: 40px;
  height: 40px;
}
.es-icon {
  font-size: 28px;
}
.empty-state.small .es-icon {
  font-size: 20px;
}
.es-text {
  font-size: 13px;
  line-height: 1.4;
  max-width: 240px;
}
.empty-state.small .es-text {
  font-size: 12px;
}
.es-slot:empty {
  display: none;
}
</style>
