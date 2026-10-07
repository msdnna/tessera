<script setup>
// The phone form of tier 1 (v2, #2969): a popover form becomes a sheet sliding
// up from the bottom, with a handle, over a scrim. No key caps on touch.
// `full` is tier 2 — a sheet that takes the whole height under the top bar.
import { computed, toRef } from 'vue'
import { useOverlayBack } from '@/composables/useOverlayBack'

const props = defineProps({
  show: { type: Boolean, default: false },
  title: { type: String, default: '' },
  subtitle: { type: String, default: '' },
  full: { type: Boolean, default: false },
  testid: { type: String, default: 'bottom-sheet' },
})
const emit = defineEmits(['update:show', 'close'])

function close() {
  emit('update:show', false)
  emit('close')
}
const open = computed(() => props.show)
useOverlayBack(toRef(props, 'show'), close)
</script>

<template>
  <Teleport to="body">
    <transition name="bs">
      <div v-if="open" class="bs-scrim" @click.self="close">
        <div class="bs kit-sheet" :class="{ full }" role="dialog" aria-modal="true" :data-testid="testid">
          <span class="bs-handle" aria-hidden="true" />
          <div v-if="title || subtitle" class="bs-head">
            <div class="bs-title">{{ title }}</div>
            <div v-if="subtitle" class="bs-sub">{{ subtitle }}</div>
          </div>
          <div class="bs-body"><slot /></div>
          <div v-if="$slots.actions" class="bs-actions"><slot name="actions" /></div>
        </div>
      </div>
    </transition>
  </Teleport>
</template>

<style scoped>
.bs-scrim {
  position: fixed;
  inset: 0;
  z-index: 2100;
  background: var(--t-scrim);
}
.bs {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 0;
  display: flex;
  flex-direction: column;
  gap: 14px;
  max-height: calc(100% - 52px);
  padding: 8px 16px calc(30px + env(safe-area-inset-bottom, 0px));
  box-sizing: border-box;
  border-radius: 18px 18px 0 0;
  background: var(--t-pop);
  box-shadow: var(--t-sh-drag);
  overflow-y: auto;
}
.bs.full {
  top: 52px;
}
.bs-handle {
  align-self: center;
  width: 36px;
  height: 5px;
  border-radius: 3px;
  background: var(--t-kbd);
  flex: none;
}
.bs-head {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.bs-title {
  font-size: 17px;
  font-weight: 650;
  color: var(--t-text1);
}
.bs-sub {
  font-size: 13px;
  color: var(--t-text3);
}
.bs-body {
  display: flex;
  flex-direction: column;
  gap: 14px;
}
.bs.full .bs-body {
  flex: 1;
}
.bs-actions {
  display: flex;
  gap: 10px;
  margin-top: 4px;
}
.bs-enter-active,
.bs-leave-active {
  transition: opacity 0.18s ease;
}
.bs-enter-active .bs,
.bs-leave-active .bs {
  transition: transform 0.22s ease;
}
.bs-enter-from,
.bs-leave-to {
  opacity: 0;
}
.bs-enter-from .bs,
.bs-leave-to .bs {
  transform: translateY(24px);
}
@media (prefers-reduced-motion: reduce) {
  .bs-enter-active,
  .bs-leave-active,
  .bs-enter-active .bs,
  .bs-leave-active .bs {
    transition: none;
  }
}
</style>
