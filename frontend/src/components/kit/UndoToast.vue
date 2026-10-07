<script setup>
// One undo toast (v2 interaction model, #2969, mockup UndoToast) — the look
// only. The stack, the clock and the keys live in UndoToastHost + stores/undo;
// the design system shows this one on its own.
//
// status: 'live' (counting down, «Отменить ⌘Z») · 'busy' («Отменяем…») ·
// 'done' (green check, no button) · 'error' («Повторить ↵»).
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import {
  AlertCircleOutline,
  ArchiveOutline,
  CheckmarkDoneOutline,
  CheckmarkOutline,
  CloseOutline,
  MoonOutline,
  TrashOutline,
} from '@vicons/ionicons5'
import Kbd from '@/components/Kbd.vue'
import ShortcutKeys from '@/components/ShortcutKeys.vue'
import TesseraSpinner from '@/components/TesseraSpinner.vue'
import { CAPS } from '@/utils/keyCaps'

defineProps({
  caption: { type: String, default: '' },
  // 'trash' | 'archive' | 'check' (completed, #2997) | 'moon' (snoozed, #2997)
  icon: { type: String, default: 'trash' },
  status: { type: String, default: 'live' },
  // Time left, 0..1 — the 3px bar along the bottom edge.
  progress: { type: Number, default: 1 },
  paused: { type: Boolean, default: false },
  // prefers-reduced-motion: the bar becomes a «7 с» text.
  reduced: { type: Boolean, default: false },
  secondsLeft: { type: Number, default: 7 },
  older: { type: Boolean, default: false },
})
const emit = defineEmits(['undo', 'retry', 'close'])
const { t } = useI18n()
const ICONS = {
  trash: TrashOutline,
  archive: ArchiveOutline,
  check: CheckmarkDoneOutline,
  moon: MoonOutline,
}
</script>

<template>
  <div class="ut" :class="{ older }" data-testid="undo-toast">
    <span class="ut-icon" :class="{ ok: status === 'done', bad: status === 'error' }">
      <n-icon
        :component="
          status === 'done'
            ? CheckmarkOutline
            : status === 'error'
              ? AlertCircleOutline
              : ICONS[icon] || TrashOutline
        "
        :size="16"
      />
    </span>
    <span class="ut-text">{{ caption }}</span>

    <span v-if="status === 'busy'" class="ut-busy">
      <TesseraSpinner :size="14" />
      {{ t('common.undo.undoing') }}
    </span>
    <button
      v-else-if="status === 'live'"
      type="button"
      class="ut-action"
      data-testid="undo-toast-undo"
      @click="emit('undo')"
    >
      {{ t('common.undo.undo') }}
      <ShortcutKeys id="undo.last" />
    </button>
    <button
      v-else-if="status === 'error'"
      type="button"
      class="ut-action"
      data-testid="undo-toast-retry"
      @click="emit('retry')"
    >
      {{ t('common.undo.retry') }}
      <Kbd>{{ CAPS.enter }}</Kbd>
    </button>

    <span v-if="reduced && status === 'live'" class="ut-secs">
      {{ t('common.undo.seconds', { n: secondsLeft }) }}
    </span>
    <button
      type="button"
      class="ut-close"
      :title="t('common.undo.close')"
      :aria-label="t('common.undo.close')"
      @click="emit('close')"
    >
      <n-icon :component="CloseOutline" :size="16" />
    </button>

    <span
      v-if="status === 'live' && !reduced"
      class="ut-bar"
      :class="{ paused }"
      :style="{ width: `${Math.max(0, Math.min(1, progress)) * 100}%` }"
    />
  </div>
</template>

<style scoped>
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
@media (prefers-reduced-motion: reduce) {
  .ut,
  .ut-bar {
    transition: none;
  }
}
</style>
