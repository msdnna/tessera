<script setup>
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import { CloudDownloadOutline, CloseOutline, ReloadOutline } from '@vicons/ionicons5'
import { useAppUpdate } from '@/composables/useAppUpdate'

// A single, global "new version available" toast, styled after the board
// activity toasts (#2748). Bottom-right so it never collides with the
// bottom-left board-activity stack. Prompt mode: shows only when the waiting
// service worker signals a fresh deploy; "update" reloads onto it, "later"
// hides it for now (it returns if a further deploy lands, or on next load).
const { t } = useI18n()
const { needRefresh, updateServiceWorker } = useAppUpdate()

const dismissed = ref(false)
const reloading = ref(false)

// A later deploy re-arms the prompt even if this session postponed the previous
// one: reset the local dismissal whenever needRefresh rises again.
watch(needRefresh, (v) => {
  if (v) dismissed.value = false
})

function reload() {
  reloading.value = true
  updateServiceWorker(true)
}
function postpone() {
  dismissed.value = true
}
</script>

<template>
  <transition name="update-toast">
    <div v-if="needRefresh && !dismissed" class="upd-toast" role="alert">
      <div class="u-ico">
        <n-icon :component="CloudDownloadOutline" :size="22" />
      </div>
      <div class="u-body">
        <div class="u-title">{{ t('app.update.title') }}</div>
        <div class="u-text">{{ t('app.update.text') }}</div>
        <div class="u-actions">
          <button class="u-btn primary" :disabled="reloading" @click="reload">
            <n-icon :component="ReloadOutline" :size="13" />
            {{ reloading ? t('app.update.reloading') : t('app.update.reload') }}
          </button>
          <button class="u-btn" :disabled="reloading" @click="postpone">
            {{ t('app.update.postpone') }}
          </button>
        </div>
      </div>
      <button class="u-close" :title="t('app.update.postpone')" @click="postpone">
        <n-icon :component="CloseOutline" :size="15" />
      </button>
    </div>
  </transition>
</template>

<style scoped>
.upd-toast {
  position: fixed;
  right: 16px;
  bottom: 16px;
  /* Same band as the board-activity stack (above Naive overlays, below the
     blocking app loader/connection overlay). */
  z-index: 8000;
  display: flex;
  gap: 10px;
  width: 320px;
  max-width: calc(100vw - 32px);
  padding: 10px 12px;
  border-radius: 12px;
  background: var(--t-surface);
  border: 1px solid var(--t-border);
  box-shadow: 0 6px 24px rgba(0, 0, 0, 0.16);
}
.u-ico {
  flex: none;
  display: inline-flex;
  align-items: flex-start;
  padding-top: 1px;
  color: var(--t-primary);
}
.u-body {
  flex: 1;
  min-width: 0;
}
.u-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--t-text1);
}
.u-text {
  margin: 1px 0 7px;
  font-size: 12px;
  color: var(--t-text3);
}
.u-actions {
  display: flex;
  gap: 6px;
}
.u-btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  padding: 3px 10px;
  border-radius: 7px;
  border: 1px solid var(--t-border);
  background: transparent;
  color: var(--t-text2);
  cursor: pointer;
}
/* Only the neutral "later" button greys on hover — the primary one keeps its
   accent (a blanket .u-btn:hover would paint it grey and leave white text on it,
   unreadable; #2748 rework). */
.u-btn:not(.primary):hover:not(:disabled) {
  background: var(--t-hover);
}
.u-btn:disabled {
  opacity: 0.6;
  cursor: default;
}
.u-btn.primary {
  border-color: transparent;
  background: var(--t-accent-grad, var(--t-primary));
  color: var(--t-on-primary, #fff);
}
.u-btn.primary:hover:not(:disabled) {
  filter: brightness(1.06);
}
.u-close {
  position: absolute;
  top: 6px;
  right: 6px;
  display: inline-flex;
  border: none;
  background: transparent;
  color: var(--t-text3);
  cursor: pointer;
  border-radius: 6px;
  padding: 1px;
}
.u-close:hover {
  color: var(--t-text1);
  background: var(--t-hover);
}
.update-toast-enter-active,
.update-toast-leave-active {
  transition:
    transform 0.24s cubic-bezier(0.22, 1, 0.36, 1),
    opacity 0.24s ease;
}
.update-toast-enter-from,
.update-toast-leave-to {
  transform: translate3d(0, 20px, 0);
  opacity: 0;
}
@media (prefers-reduced-motion: reduce) {
  .update-toast-enter-active,
  .update-toast-leave-active {
    transition: none;
  }
}
</style>
