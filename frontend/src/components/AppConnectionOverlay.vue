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
//
// v2 (#3107 — PackageE «Нет соединения»): the offline state is a card on the
// scrim over the current screen — nothing under it is torn down, and it closes
// by itself the moment any request reaches the server again (useConnection).
// «Повторить сейчас» (↵) reloads.
import { computed, watch, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import { CloudOfflineOutline } from '@vicons/ionicons5'
import { connection } from '@/composables/useConnection'
import { CAPS } from '@/utils/keyCaps'
import Kbd from './Kbd.vue'
import './kit/kit.css'

const { t } = useI18n()
const active = computed(() => connection.active && !connection.offline)

function retry() {
  window.location.reload()
}

// ↵ retries while the card is up — it is the only thing on screen to do.
function onKey(e) {
  if (e.key !== 'Enter' || e.defaultPrevented) return
  e.preventDefault()
  retry()
}
watch(
  () => connection.offline,
  (off) => {
    if (off) window.addEventListener('keydown', onKey, true)
    else window.removeEventListener('keydown', onKey, true)
  },
  { immediate: true },
)
onBeforeUnmount(() => window.removeEventListener('keydown', onKey, true))
</script>

<template>
  <!-- Server reachable but a call is taking a beat — non-blocking top bar. -->
  <transition name="tp-fade">
    <div
      v-if="active"
      class="top-progress"
      role="progressbar"
      :aria-label="t('app.connection.loading')"
    >
      <div class="top-progress__bar" />
    </div>
  </transition>

  <!-- Server unreachable — error + retry. -->
  <transition name="conn-fade">
    <div v-if="connection.offline" class="conn-overlay" data-testid="connection-offline">
      <div class="conn-box" role="alertdialog" :aria-label="t('app.connection.offlineTitle')">
        <span class="conn-tile">
          <n-icon :component="CloudOfflineOutline" class="conn-icon" />
        </span>
        <div class="conn-title">{{ t('app.connection.offlineTitle') }}</div>
        <div class="conn-sub">{{ t('app.connection.offlineSub') }}</div>
        <button type="button" class="kit-btn conn-retry" @click="retry">
          {{ t('app.connection.retry') }}<Kbd>{{ CAPS.enter }}</Kbd>
        </button>
      </div>
    </div>
  </transition>
</template>

<style scoped>
/* Thin indeterminate progress bar, top of the viewport, non-interactive. */
.top-progress {
  position: fixed;
  top: 0;
  /* Start past the desktop server rail (#2930) so the hotbar stays visible; 0 on
     web, where --app-rail-w is unset. */
  left: var(--app-rail-w, 0px);
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
  /* Leave the desktop server rail (#2930) uncovered and clickable — so a stuck
     server can be switched away from without dismissing this overlay. 0 on web. */
  left: var(--app-rail-w, 0px);
  z-index: 9001;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  background: var(--t-scrim);
}
/* The card: popover level, the large-surface radius, the popover shadow. */
.conn-box {
  display: flex;
  flex-direction: column;
  align-items: center;
  text-align: center;
  gap: 8px;
  width: 100%;
  max-width: 340px;
  box-sizing: border-box;
  padding: 26px 24px 22px;
  border-radius: 18px;
  background: var(--t-pop);
  box-shadow: var(--t-sh-pop);
}
.conn-tile {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 56px;
  height: 56px;
  margin-bottom: 6px;
  border-radius: 18px;
  background: color-mix(in srgb, var(--t-primary) 16%, transparent);
  color: var(--t-primary);
}
.conn-icon {
  font-size: 26px;
}
.conn-title {
  font-size: 17px;
  font-weight: 700;
  color: var(--t-text1);
}
.conn-sub {
  font-size: 13px;
  line-height: 1.5;
  color: var(--t-text2);
}
.conn-retry {
  margin-top: 10px;
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
