<script setup>
// The undo toast stack (v2 interaction model, #2969) — see stores/undo.js.
// Mounted once in AppLayout; centred over the work area (right of the rail and
// sidebar), 24px above its bottom, above the FAB and below popovers and modals.
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import {
  AlertCircleOutline,
  ArchiveOutline,
  CheckmarkOutline,
  CloseOutline,
  TrashOutline,
} from '@vicons/ionicons5'
import Kbd from '@/components/Kbd.vue'
import { CAPS } from '@/utils/keyCaps'
import ShortcutKeys from '@/components/ShortcutKeys.vue'
import TesseraSpinner from '@/components/TesseraSpinner.vue'
import { useUndoStore, UNDO_MS } from '@/stores/undo'
import { useShortcuts } from '@/composables/useShortcuts'

const { t } = useI18n()
const undo = useUndoStore()
const list = computed(() => undo.toasts)

const ICONS = { trash: TrashOutline, archive: ArchiveOutline }

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
    <div ref="root" class="ut-stack" role="region" aria-live="polite" :aria-label="t('common.undo.region')">
      <transition-group name="ut">
        <div
          v-for="(toast, i) in list"
          :key="toast.id"
          class="ut"
          :class="{ older: i < list.length - 1 }"
          data-testid="undo-toast"
          @mouseenter="undo.pause(toast.id, true)"
          @mouseleave="undo.pause(toast.id, false)"
          @focusin="undo.pause(toast.id, true)"
          @focusout="undo.pause(toast.id, false)"
          @keydown="onKey($event, toast)"
        >
          <span
            class="ut-icon"
            :class="{ ok: toast.status === 'done', bad: toast.status === 'error' }"
          >
            <n-icon
              :component="
                toast.status === 'done'
                  ? CheckmarkOutline
                  : toast.status === 'error'
                    ? AlertCircleOutline
                    : ICONS[toast.icon] || TrashOutline
              "
              :size="16"
            />
          </span>
          <span class="ut-text">{{ caption(toast) }}</span>

          <span v-if="toast.status === 'busy'" class="ut-busy">
            <TesseraSpinner :size="14" />
            {{ t('common.undo.undoing') }}
          </span>
          <button
            v-else-if="toast.status === 'live'"
            type="button"
            class="ut-action"
            data-testid="undo-toast-undo"
            @click="undo.undo(toast.id)"
          >
            {{ t('common.undo.undo') }}
            <ShortcutKeys id="undo.last" />
          </button>
          <button
            v-else-if="toast.status === 'error'"
            type="button"
            class="ut-action"
            data-testid="undo-toast-retry"
            @click="undo.undo(toast.id)"
          >
            {{ t('common.undo.retry') }}
            <Kbd>{{ CAPS.enter }}</Kbd>
          </button>

          <span v-if="reduced && toast.status === 'live'" class="ut-secs">
            {{ t('common.undo.seconds', { n: Math.ceil(toast.left / 1000) }) }}
          </span>
          <button
            type="button"
            class="ut-close"
            :title="t('common.undo.close')"
            :aria-label="t('common.undo.close')"
            @click="undo.dismiss(toast.id)"
          >
            <n-icon :component="CloseOutline" :size="16" />
          </button>

          <span
            v-if="toast.status === 'live' && !reduced"
            class="ut-bar"
            :class="{ paused: toast.paused > 0 }"
            :style="{ width: `${Math.max(0, (toast.left / UNDO_MS) * 100)}%` }"
          />
        </div>
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
.ut {
  position: relative;
  overflow: hidden;
  display: flex;
  align-items: center;
  gap: 12px;
  height: 50px;
  padding: 0 8px 0 10px;
  box-sizing: border-box;
  border-radius: var(--t-r-card);
  background: var(--t-pop);
  box-shadow: var(--t-sh-drag);
  color: var(--t-text1);
  white-space: nowrap;
  max-width: calc(100vw - 32px);
  pointer-events: auto;
  transform-origin: center;
  transition:
    transform 0.18s ease,
    opacity 0.18s ease;
}
.ut.older {
  transform: scale(0.96);
  opacity: 0.9;
}
.ut-icon {
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  border-radius: 9px;
  background: var(--t-pill);
  color: var(--t-text2);
}
.ut-icon.ok {
  background: linear-gradient(
    to top right,
    color-mix(in srgb, #18a058 16%, var(--t-pop)),
    color-mix(in srgb, #18a058 28%, var(--t-pop))
  );
  color: #18a058;
}
.ut-icon.bad {
  background: linear-gradient(
    to top right,
    color-mix(in srgb, #e0533d 16%, var(--t-pop)),
    color-mix(in srgb, #e0533d 28%, var(--t-pop))
  );
  color: #e0533d;
}
.ut-text {
  font-size: 13.5px;
  padding-right: 4px;
  overflow: hidden;
  text-overflow: ellipsis;
}
.ut-action {
  flex: none;
  display: inline-flex;
  align-items: center;
  gap: 8px;
  height: 32px;
  padding: 0 5px 0 11px;
  border: 0;
  border-radius: 9px;
  background: linear-gradient(
    to top right,
    color-mix(in srgb, var(--t-primary) 16%, var(--t-pop)),
    color-mix(in srgb, var(--t-primary) 26%, var(--t-pop))
  );
  color: var(--t-text1);
  font: inherit;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
}
.ut-action:hover {
  background: linear-gradient(
    to top right,
    color-mix(in srgb, var(--t-primary) 24%, var(--t-pop)),
    color-mix(in srgb, var(--t-primary) 36%, var(--t-pop))
  );
}
.ut-action:focus-visible,
.ut-close:focus-visible {
  outline: 2px solid var(--t-primary);
  outline-offset: 2px;
}
.ut-busy {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  height: 32px;
  padding: 0 11px;
  font-size: 13px;
  color: var(--t-text2);
}
.ut-secs {
  font-size: 12px;
  color: var(--t-text3);
}
.ut-close {
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: var(--t-text3);
  cursor: pointer;
}
.ut-close:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.ut-bar {
  position: absolute;
  left: 0;
  bottom: 0;
  height: 3px;
  border-radius: 0 2px 2px 0;
  background: var(--t-accent-grad);
  transition: width 0.1s linear;
}
.ut-bar.paused {
  background: var(--t-text3);
  opacity: 0.6;
}
.ut-enter-from,
.ut-leave-to {
  opacity: 0;
  transform: translateY(8px);
}
@media (prefers-reduced-motion: reduce) {
  .ut,
  .ut-bar {
    transition: none;
  }
}
</style>
