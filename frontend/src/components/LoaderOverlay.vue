<script setup>
// Reusable branded loading overlay: the TesseraSpinner floating over a pale,
// blurred backdrop with reassuring captions that cross-fade every `interval` and
// settle on the last one. Used by AppConnectionOverlay (server slow) and for
// longer scoped operations (e.g. GitLab sync). Renders fixed/full-screen so it
// sits above whatever triggered it (modals included).
import { ref, watch, onUnmounted } from 'vue'
import TesseraSpinner from '@/components/TesseraSpinner.vue'

const props = defineProps({
  show: { type: Boolean, default: false },
  // Captions to cross-fade through; stays on the last once reached.
  messages: { type: Array, default: () => ['Загружаем…'] },
  size: { type: Number, default: 44 },
  interval: { type: Number, default: 3000 },
  // contained: absolute-fill the nearest positioned ancestor (e.g. dim only a
  // modal body) instead of covering the whole viewport.
  contained: { type: Boolean, default: false },
})

const index = ref(0)
let timer = null
function stop() {
  if (timer) {
    clearInterval(timer)
    timer = null
  }
}
function start() {
  stop()
  index.value = 0
  if (props.messages.length < 2) return
  timer = setInterval(() => {
    if (index.value < props.messages.length - 1) index.value += 1
    else stop()
  }, props.interval)
}
watch(
  () => props.show,
  (v) => (v ? start() : stop()),
  { immediate: true },
)
onUnmounted(stop)
</script>

<template>
  <transition name="lo-fade">
    <div v-if="show" class="lo-overlay" :class="{ contained }">
      <div class="lo-box">
        <tessera-spinner :size="size" />
        <transition name="lo-cap" mode="out-in">
          <div :key="index" class="lo-cap">{{ messages[index] }}</div>
        </transition>
      </div>
    </div>
  </transition>
</template>

<style scoped>
.lo-overlay {
  position: fixed;
  inset: 0;
  z-index: 9000;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  background: color-mix(in srgb, var(--t-bg) 72%, transparent);
  backdrop-filter: blur(6px);
}
/* In-modal: fill the positioned ancestor and dim/blur only that surface. */
.lo-overlay.contained {
  position: absolute;
  z-index: 20;
  background: color-mix(in srgb, var(--t-surface) 64%, transparent);
  border-radius: inherit;
}
.lo-box {
  display: flex;
  flex-direction: column;
  align-items: center;
  text-align: center;
  gap: 18px;
  max-width: 360px;
}
.lo-cap {
  font-size: 13px;
  color: var(--t-text3);
  min-height: 18px;
  max-width: 280px;
}
.lo-fade-enter-active,
.lo-fade-leave-active {
  transition: opacity 0.2s ease;
}
.lo-fade-enter-from,
.lo-fade-leave-to {
  opacity: 0;
}
.lo-cap-enter-active,
.lo-cap-leave-active {
  transition: opacity 0.5s ease;
}
.lo-cap-enter-from,
.lo-cap-leave-to {
  opacity: 0;
}
@media (prefers-reduced-motion: reduce) {
  .lo-cap-enter-active,
  .lo-cap-leave-active {
    transition: none;
  }
}
</style>
