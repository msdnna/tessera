<script setup>
// The undo toast stack (v2 interaction model, #2969) — see stores/undo.js.
// Mounted once in AppLayout; centred over the work area (right of the rail and
// sidebar), 24px above its bottom, above the FAB and below popovers and modals.
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import UndoToast from './UndoToast.vue'
import { useUndoStore, UNDO_MS } from '@/stores/undo'
import { useShortcuts } from '@/composables/useShortcuts'

const { t } = useI18n()
const undo = useUndoStore()
const list = computed(() => undo.toasts)

function label(text) {
  if (!text) return ''
  if (typeof text === 'function') return text()
  if (typeof text === 'string') return text
  return t(text.key, text.params || {})
}

function caption(toast) {
  if (toast.status === 'done') return label(toast.doneText) || t('common.undo.done')
  if (toast.status === 'error') return label(toast.errorText) || t('common.undo.failed')
  return label(toast.text)
}

// One clock for the whole stack, running only while something is shown.
const TICK = 100
let timer = null
watch(
  () => list.value.length > 0,
  (any) => {
    if (any && !timer) timer = setInterval(() => undo.tick(TICK), TICK)
    if (!any && timer) {
      clearInterval(timer)
      timer = null
    }
  },
  { immediate: true },
)
onBeforeUnmount(() => timer && clearInterval(timer))

const reduced = ref(
  typeof window !== 'undefined' && window.matchMedia?.('(prefers-reduced-motion: reduce)').matches,
)

const root = ref(null)
function focusNewest() {
  const buttons = root.value?.querySelectorAll('.ut-action, .ut-close')
  if (!buttons?.length) return false
  const items = root.value.querySelectorAll('.ut')
  const last = items[items.length - 1]
  ;(last?.querySelector('.ut-action') || last?.querySelector('.ut-close'))?.focus()
  return true
}

useShortcuts({
  'undo.last': () => undo.undoLast(),
  'toast.focus': () => focusNewest(),
})

function onKey(e, toast) {
  if (e.key === 'Escape') {
    e.preventDefault()
    e.stopPropagation()
    undo.dismiss(toast.id)
  }
}
</script>

<template>
  <Teleport to="body">
    <div
      ref="root"
      class="ut-stack"
      role="region"
      aria-live="polite"
      :aria-label="t('common.undo.region')"
    >
      <transition-group name="ut">
        <UndoToast
          v-for="(toast, i) in list"
          :key="toast.id"
          :caption="caption(toast)"
          :icon="toast.icon"
          :status="toast.status"
          :progress="toast.left / UNDO_MS"
          :paused="toast.paused > 0"
          :reduced="reduced"
          :seconds-left="Math.ceil(toast.left / 1000)"
          :older="i < list.length - 1"
          @undo="undo.undo(toast.id)"
          @retry="undo.undo(toast.id)"
          @close="undo.dismiss(toast.id)"
          @mouseenter="undo.pause(toast.id, true)"
          @mouseleave="undo.pause(toast.id, false)"
          @focusin="undo.pause(toast.id, true)"
          @focusout="undo.pause(toast.id, false)"
          @keydown="onKey($event, toast)"
        />
      </transition-group>
    </div>
  </Teleport>
</template>

<style scoped>
.ut-stack {
  position: fixed;
  left: var(--app-content-left, 0px);
  right: 0;
  bottom: 24px;
  z-index: 1500;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  pointer-events: none;
}
.ut-stack > * {
  pointer-events: auto;
}
.ut-enter-from,
.ut-leave-to {
  opacity: 0;
  transform: translateY(8px);
}
</style>
