<script setup>
// One field of a PopoverForm (v2, #2969): label (+ an aside on the right), the
// control in the default slot, then optionally a path preview, an error with a
// «Свободно: x или ⌥↵» suggestion, and a quiet message.
//
// Controls use the `kf-input` class (kit.css) to get the v2 well.
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import { AlertCircleOutline, LinkOutline } from '@vicons/ionicons5'
import Kbd from '@/components/Kbd.vue'
import { CAPS } from '@/utils/keyCaps'
import './kit.css'

defineProps({
  label: { type: String, default: '' },
  aside: { type: String, default: '' },
  // Path preview under the field: «…/project/<preview>».
  preview: { type: String, default: null },
  previewPrefix: { type: String, default: '…/project/' },
  error: { type: String, default: '' },
  suggestion: { type: String, default: '' },
  message: { type: String, default: '' },
})
const emit = defineEmits(['suggest'])
const { t } = useI18n()
</script>

<template>
  <div class="kf">
    <div v-if="label || aside" class="kf-label">
      <span class="kf-label-text">{{ label }}</span>
      <span v-if="aside" class="kf-aside">{{ aside }}</span>
    </div>
    <slot />
    <div v-if="preview !== null" class="kf-preview">
      <n-icon :component="LinkOutline" :size="13" />
      <span>{{ previewPrefix }}</span>
      <b>{{ preview || '…' }}</b>
    </div>
    <div v-if="error" class="kf-error" role="alert">
      <n-icon :component="AlertCircleOutline" :size="14" class="kf-error-icon" />
      <span>{{ error }}</span>
    </div>
    <div v-if="error && suggestion" class="kf-suggest">
      {{ t('common.form.free') }}
      <button type="button" class="kf-chip" @click="emit('suggest', suggestion)">{{ suggestion }}</button>
      {{ t('common.form.or') }}
      <Kbd>{{ CAPS.altEnter }}</Kbd>
    </div>
    <div v-if="message && !error" class="kf-message">{{ message }}</div>
  </div>
</template>

<style scoped>
.kf {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.kf-label {
  display: flex;
  gap: 6px;
  font-size: 12px;
  font-weight: 600;
  color: var(--t-text3);
}
.kf-label-text {
  flex: 1;
}
.kf-aside {
  font-weight: 500;
}
.kf-preview {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--t-text3);
  white-space: nowrap;
  overflow: hidden;
}
.kf-preview b {
  color: var(--t-text2);
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
}
.kf-error {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  font-size: 12.5px;
  line-height: 17px;
  color: var(--t-text1);
}
.kf-error-icon {
  flex: none;
  margin-top: 1px;
  color: #e0533d;
}
.kf-suggest {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--t-text3);
}
.kf-chip {
  height: 24px;
  padding: 0 9px;
  border: 0;
  border-radius: 999px;
  background: linear-gradient(
    to top right,
    color-mix(in srgb, var(--t-primary) 14%, var(--t-pop)),
    color-mix(in srgb, var(--t-primary) 24%, var(--t-pop))
  );
  color: var(--t-text1);
  font: inherit;
  font-size: 12.5px;
  font-weight: 600;
  cursor: pointer;
}
.kf-message {
  font-size: 12px;
  line-height: 17px;
  color: var(--t-text3);
  text-wrap: pretty;
}
</style>
