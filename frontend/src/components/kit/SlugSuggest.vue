<script setup>
// «Свободно: <chip> или ⌥↵» under a taken address (v2, #3013, mockup PopForm
// state=error). The chip puts the free variant into the field; ⌥↵ does the same
// from the keyboard — that key is the owner's to bind (PopoverForm emits
// `suggest`, a plain field listens itself).
//
// mousedown is swallowed so the click doesn't blur the field first: fields that
// save on blur would otherwise send the taken address again.
import { useI18n } from 'vue-i18n'
import Kbd from '@/components/Kbd.vue'
import { CAPS } from '@/utils/keyCaps'

defineProps({
  suggestion: { type: String, required: true },
})
const emit = defineEmits(['pick'])
const { t } = useI18n()
</script>

<template>
  <div class="kf-suggest" data-testid="slug-suggest">
    {{ t('common.form.free') }}
    <button
      type="button"
      class="kf-chip"
      data-testid="slug-suggest-chip"
      @mousedown.prevent
      @click="emit('pick', suggestion)"
    >
      {{ suggestion }}
    </button>
    {{ t('common.form.or') }}
    <Kbd>{{ CAPS.altEnter }}</Kbd>
  </div>
</template>

<style scoped>
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
</style>
