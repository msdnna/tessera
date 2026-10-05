<script setup>
// The half-typed sequence chip (v2, #2956): after «G» the user sees «G …» at
// the bottom of the screen until the second key lands or the wait runs out.
import Kbd from './Kbd.vue'
import { pendingCaps } from '@/shortcuts/dispatcher'
</script>

<template>
  <Teleport to="body">
    <transition name="sk-pend">
      <div v-if="pendingCaps.length" class="sk-pending" data-testid="shortcut-pending">
        <Kbd v-for="(cap, i) in pendingCaps" :key="i">{{ cap }}</Kbd>
        <span class="dots">…</span>
      </div>
    </transition>
  </Teleport>
</template>

<style scoped>
.sk-pending {
  position: fixed;
  left: 50%;
  bottom: 24px;
  transform: translateX(-50%);
  z-index: 2100;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 34px;
  padding: 0 12px;
  border-radius: var(--t-r-tip);
  background: var(--t-pop);
  box-shadow: var(--t-sh-pop);
  color: var(--t-text3);
  pointer-events: none;
}
.dots {
  font-size: 14px;
  letter-spacing: 1px;
}
.sk-pend-enter-active,
.sk-pend-leave-active {
  transition:
    opacity 0.12s ease,
    transform 0.12s ease;
}
.sk-pend-enter-from,
.sk-pend-leave-to {
  opacity: 0;
  transform: translate(-50%, 6px);
}
</style>
