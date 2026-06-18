<script setup>
// Full-screen connection state, mirroring the Android LoadingState/ErrorState:
//   • slow    — server reachable but a request has dragged on: the shared branded
//               LoaderOverlay (TesseraSpinner + cross-fading captions).
//   • offline — a request couldn't reach the server: an error glyph, a line,
//               and a single retry that reloads the app.
// Driven by the shared `connection` reactive (api interceptors feed it). Offline
// wins over slow. Nothing renders on the common fast path.
import { computed } from 'vue'
import { NIcon, NButton } from 'naive-ui'
import { CloudOfflineOutline, RefreshOutline } from '@vicons/ionicons5'
import { connection } from '@/composables/useConnection'
import LoaderOverlay from '@/components/LoaderOverlay.vue'

const CAPTIONS = [
  'Пытаемся связаться с сервером…',
  'Это занимает чуть больше времени, чем обычно…',
  'Всё ещё ждём ответ сервера…',
]

const slow = computed(() => connection.slow && !connection.offline)

function retry() {
  window.location.reload()
}
</script>

<template>
  <!-- Server reachable but slow. -->
  <loader-overlay :show="slow" :messages="CAPTIONS" :interval="3500" />

  <!-- Server unreachable — error + retry. -->
  <transition name="conn-fade">
    <div v-if="connection.offline" class="conn-overlay">
      <div class="conn-box">
        <span class="conn-disc">
          <n-icon :component="CloudOfflineOutline" class="conn-icon" />
        </span>
        <div class="conn-title">Нет связи с сервером</div>
        <div class="conn-sub">Проверьте подключение к интернету и попробуйте снова.</div>
        <n-button type="primary" class="conn-retry" @click="retry">
          <template #icon><n-icon :component="RefreshOutline" /></template>
          Попробовать ещё раз
        </n-button>
      </div>
    </div>
  </transition>
</template>

<style scoped>
.conn-overlay {
  position: fixed;
  inset: 0;
  z-index: 9001;
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
</style>
