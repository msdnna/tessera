<script setup>
// Full-screen connection state, mirroring the Android LoadingState/ErrorState:
//   • slow    — server reachable but a request has dragged on: the branded
//               TesseraSpinner with reassuring captions that cross-fade.
//   • offline — a request couldn't reach the server: an error glyph, a line,
//               and a single retry that reloads the app.
// Driven by the shared `connection` reactive (api interceptors feed it). Offline
// wins over slow. Nothing renders on the common fast path.
import { ref, computed, watch, onUnmounted } from 'vue'
import { NIcon, NButton } from 'naive-ui'
import { CloudOfflineOutline, RefreshOutline } from '@vicons/ionicons5'
import { connection } from '@/composables/useConnection'
import TesseraSpinner from '@/components/TesseraSpinner.vue'

const CAPTIONS = [
  'Пытаемся связаться с сервером…',
  'Это занимает чуть больше времени, чем обычно…',
  'Всё ещё ждём ответ сервера…',
]

const show = computed(() => connection.offline || connection.slow)
const offline = computed(() => connection.offline)

// Captions advance on a timer while the slow loader is visible, settling on the
// last one. Reset whenever the loader (re)appears.
const captionIndex = ref(0)
let timer = null
function startCaptions() {
  stopCaptions()
  captionIndex.value = 0
  timer = setInterval(() => {
    if (captionIndex.value < CAPTIONS.length - 1) captionIndex.value += 1
    else stopCaptions()
  }, 3500)
}
function stopCaptions() {
  if (timer) {
    clearInterval(timer)
    timer = null
  }
}
watch(
  () => connection.slow && !connection.offline,
  (slow) => (slow ? startCaptions() : stopCaptions()),
)
onUnmounted(stopCaptions)

function retry() {
  window.location.reload()
}
</script>

<template>
  <transition name="conn-fade">
    <div v-if="show" class="conn-overlay">
      <div class="conn-box">
        <template v-if="offline">
          <span class="conn-disc">
            <n-icon :component="CloudOfflineOutline" class="conn-icon" />
          </span>
          <div class="conn-title">Нет связи с сервером</div>
          <div class="conn-sub">Проверьте подключение к интернету и попробуйте снова.</div>
          <n-button type="primary" class="conn-retry" @click="retry">
            <template #icon><n-icon :component="RefreshOutline" /></template>
            Попробовать ещё раз
          </n-button>
        </template>
        <template v-else>
          <tessera-spinner :size="44" />
          <transition name="conn-cap" mode="out-in">
            <div :key="captionIndex" class="conn-cap">{{ CAPTIONS[captionIndex] }}</div>
          </transition>
        </template>
      </div>
    </div>
  </transition>
</template>

<style scoped>
.conn-overlay {
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
.conn-box {
  display: flex;
  flex-direction: column;
  align-items: center;
  text-align: center;
  gap: 18px;
  max-width: 360px;
}
.conn-disc {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 64px;
  height: 64px;
  border-radius: 50%;
  background: color-mix(in srgb, var(--t-primary) 14%, transparent);
  color: var(--t-primary);
}
.conn-icon {
  font-size: 32px;
}
.conn-title {
  font-size: 16px;
  font-weight: 600;
  color: var(--t-text1);
}
.conn-sub {
  font-size: 13px;
  line-height: 1.5;
  color: var(--t-text3);
}
.conn-cap {
  font-size: 13px;
  color: var(--t-text3);
  min-height: 18px;
  max-width: 280px;
}
.conn-retry {
  margin-top: 4px;
}

.conn-fade-enter-active,
.conn-fade-leave-active {
  transition: opacity 0.2s ease;
}
.conn-fade-enter-from,
.conn-fade-leave-to {
  opacity: 0;
}
.conn-cap-enter-active,
.conn-cap-leave-active {
  transition: opacity 0.5s ease;
}
.conn-cap-enter-from,
.conn-cap-leave-to {
  opacity: 0;
}
@media (prefers-reduced-motion: reduce) {
  .conn-cap-enter-active,
  .conn-cap-leave-active {
    transition: none;
  }
}
</style>
