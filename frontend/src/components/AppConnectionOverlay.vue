<script setup>
// Global connection feedback:
//   • active  — a request is in flight and slow enough to surface: a thin,
//               non-blocking accent progress bar pinned to the top of the
//               viewport. It never captures pointer events or covers the board
//               (task #2616 — a remote install makes every call take a beat, so
//               blocking the UI each time made the app feel broken).
//   • offline — a request couldn't reach the server: a real error state, so it
//               keeps the full-screen glyph + retry.
// Driven by the shared `connection` reactive (api interceptors feed it). Offline
// wins over the bar. Nothing renders on the common fast path.
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon, NButton } from 'naive-ui'
import { CloudOfflineOutline, RefreshOutline } from '@vicons/ionicons5'
import { connection } from '@/composables/useConnection'

const { t } = useI18n()
const active = computed(() => connection.active && !connection.offline)

function retry() {
  window.location.reload()
}
</script>

<template>
  <!-- Server reachable but a call is taking a beat — non-blocking top bar. -->
  <transition name="tp-fade">
    <div v-if="active" class="top-progress" role="progressbar" :aria-label="t('app.connection.loading')">
      <div class="top-progress__bar" />
    </div>
  </transition>

  <!-- Server unreachable — error + retry. -->
  <transition name="conn-fade">
    <div v-if="connection.offline" class="conn-overlay">
      <div class="conn-box">
        <span class="conn-disc">
          <n-icon :component="CloudOfflineOutline" class="conn-icon" />
        </span>
        <div class="conn-title">{{ t('app.connection.offlineTitle') }}</div>
        <div class="conn-sub">{{ t('app.connection.offlineSub') }}</div>
        <n-button type="primary" class="conn-retry" @click="retry">
          <template #icon><n-icon :component="RefreshOutline" /></template>
          {{ t('app.connection.retry') }}
        </n-button>
      </div>
    </div>
  </transition>
</template>

<style scoped>
/* Thin indeterminate progress bar, top of the viewport, non-interactive. */
.top-progress {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  height: 3px;
  z-index: 9002;
  pointer-events: none;
  overflow: hidden;
  background: color-mix(in srgb, var(--t-primary) 12%, transparent);
}
.top-progress__bar {
  position: absolute;
  top: 0;
  bottom: 0;
  width: 40%;
  border-radius: 0 2px 2px 0;
  background: var(--t-accent-grad, var(--t-primary));
  animation: top-progress-slide 1.15s cubic-bezier(0.4, 0, 0.2, 1) infinite;
}
@keyframes top-progress-slide {
  0% {
    left: -45%;
    width: 35%;
  }
  50% {
    width: 50%;
  }
  100% {
    left: 100%;
    width: 35%;
  }
}
.tp-fade-enter-active,
.tp-fade-leave-active {
  transition: opacity 0.25s ease;
}
.tp-fade-enter-from,
.tp-fade-leave-to {
  opacity: 0;
}

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
