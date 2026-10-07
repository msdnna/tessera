<script setup>
// Tier 1 of the v2 interaction model (#2969): a small form at its anchor —
// create or change something with one to three fields — instead of a centred
// modal. No scrim; focus is kept inside; on a phone it becomes a BottomSheet.
//
//   <PopoverForm v-model:show="open" :anchor="() => plusBtn" :title="…"
//                :primary="t('…create')" :busy="saving" :disabled="!name"
//                @submit="({ open }) => create(open)">
//     <FormField :label="…"><input class="kf-input" v-model="name" /></FormField>
//   </PopoverForm>
//
// Keys: ↵ submit (in a single-line field), ⌘↵ submit-and-open, ⌥↵ «подставить
// свободный» (emits `suggest`), Tab / ⇧Tab cycle inside, Esc closes — or, while
// `busy`, emits `abort` so the owner can cancel the request. An owner opened
// from a menu listens to `escape` to step back to it instead of closing.
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Kbd from '@/components/Kbd.vue'
import { CAPS } from '@/utils/keyCaps'
import TesseraSpinner from '@/components/TesseraSpinner.vue'
import BottomSheet from './BottomSheet.vue'
import './kit.css'
import { useAnchoredPanel } from '@/composables/useAnchoredPanel'
import { useResponsive } from '@/composables/useResponsive'

const props = defineProps({
  show: { type: Boolean, default: false },
  anchor: { type: Function, default: () => null },
  placement: { type: String, default: 'right-start' },
  offset: { type: Object, default: () => ({ x: 4, y: -6 }) },
  width: { type: Number, default: 340 },
  title: { type: String, default: '' },
  subtitle: { type: String, default: '' },
  primary: { type: String, default: '' },
  busyLabel: { type: String, default: '' },
  busy: { type: Boolean, default: false },
  disabled: { type: Boolean, default: false },
  danger: { type: Boolean, default: false },
  // Footer hint on the left: 'tab' («Tab поле»), 'list' («↑↓ выбрать»),
  // 'suggest' («⌥↵ подставить свободный») or free text.
  hint: { type: String, default: '' },
  // When the owner handles Esc itself (back to a menu), set this.
  escapeBack: { type: Boolean, default: false },
  testid: { type: String, default: 'popover-form' },
  // data-tour of the primary button, for the Get Started guide.
  submitTour: { type: String, default: undefined },
})
const emit = defineEmits(['update:show', 'submit', 'cancel', 'abort', 'suggest', 'escape'])

const { t } = useI18n()
const { isMobile } = useResponsive()
const panel = ref(null)

function close() {
  emit('update:show', false)
  emit('cancel')
}

const showDesktop = computed(() => props.show && !isMobile.value)
const { style } = useAnchoredPanel({
  show: showDesktop,
  anchor: () => props.anchor(),
  panel,
  placement: props.placement,
  offset: props.offset,
  onOutside: () => {
    if (!props.busy) close()
  },
})

// Focus the first field on open.
watch(
  () => props.show,
  async (on) => {
    if (!on) return
    await nextTick()
    await nextTick()
    const root = isMobile.value ? document.querySelector(`[data-testid="${props.testid}"]`) : panel.value
    const first = root?.querySelector('input:not([disabled]), textarea:not([disabled]), [tabindex="0"]')
    first?.focus()
    if (first?.select && first.dataset.noSelect === undefined) first.select()
  },
)

const canSubmit = computed(() => !props.busy && !props.disabled)
function submit(open = false) {
  if (!canSubmit.value) return
  emit('submit', { open })
}

function focusables() {
  return [
    ...(panel.value?.querySelectorAll(
      'input:not([disabled]), textarea:not([disabled]), button:not([disabled]), [tabindex="0"]',
    ) || []),
  ].filter((el) => el.offsetParent !== null)
}

function onKeydown(e) {
  if (e.isComposing) return
  const inText = e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement
  if (e.key === 'Escape') {
    e.preventDefault()
    e.stopPropagation()
    if (props.busy) emit('abort')
    else if (props.escapeBack) emit('escape')
    else close()
    return
  }
  if (e.key === 'Enter' && e.altKey) {
    e.preventDefault()
    emit('suggest')
    return
  }
  if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
    e.preventDefault()
    submit(true)
    return
  }
  if (e.key === 'Enter' && inText && !(e.target instanceof HTMLTextAreaElement)) {
    e.preventDefault()
    submit(false)
    return
  }
  if (e.key === 'Tab') {
    const items = focusables()
    if (!items.length) return
    const i = items.indexOf(document.activeElement)
    const next = e.shiftKey ? (i <= 0 ? items.length - 1 : i - 1) : i === items.length - 1 ? 0 : i + 1
    e.preventDefault()
    items[next].focus()
  }
}

const hintParts = computed(() => {
  switch (props.hint) {
    case 'tab':
      return { cap: CAPS.tab, text: t('common.form.field') }
    case 'list':
      return { cap: CAPS.arrows, text: t('common.form.pick') }
    case 'suggest':
      return { cap: CAPS.altEnter, text: t('common.form.suggest') }
    case '':
      return null
    default:
      return { cap: '', text: props.hint }
  }
})

const sheetShow = computed({
  get: () => props.show && isMobile.value,
  set: (v) => {
    if (!v) close()
  },
})
</script>

<template>
  <Teleport to="body">
    <div
      v-if="showDesktop"
      ref="panel"
      class="pf kit-form"
      role="dialog"
      :aria-label="title"
      :style="{ ...style, width: `${width}px` }"
      :data-testid="testid"
      @keydown="onKeydown"
    >
      <div class="pf-head">
        <div class="pf-title">{{ title }}</div>
        <div v-if="subtitle" class="pf-sub">{{ subtitle }}</div>
      </div>
      <div class="pf-fields" :class="{ busy }">
        <slot />
      </div>
      <div class="pf-foot">
        <span v-if="hintParts" class="pf-hint">
          <Kbd v-if="hintParts.cap">{{ hintParts.cap }}</Kbd>
          {{ hintParts.text }}
        </span>
        <span class="pf-spacer" />
        <button type="button" class="pf-cancel" @click="busy ? emit('abort') : close()">
          {{ t('common.form.cancel') }}
          <Kbd>{{ CAPS.escape }}</Kbd>
        </button>
        <button
          type="button"
          class="pf-primary"
          :class="{ danger, disabled: !canSubmit && !busy, busy }"
          :disabled="!canSubmit"
          :data-testid="`${testid}-submit`"
          :data-tour="submitTour"
          @click="submit(false)"
        >
          <template v-if="busy">
            <TesseraSpinner :size="12" variant="white" />
            {{ busyLabel || primary }}
          </template>
          <template v-else>
            {{ primary }}
            <Kbd class="pf-cap">{{ CAPS.enter }}</Kbd>
          </template>
        </button>
      </div>
    </div>
  </Teleport>

  <BottomSheet
    v-if="isMobile"
    v-model:show="sheetShow"
    :title="title"
    :subtitle="subtitle"
    :testid="testid"
  >
    <div class="pf-fields sheet" :class="{ busy }" @keydown="onKeydown">
      <slot />
    </div>
    <template #actions>
      <button type="button" class="bs-btn" @click="close">{{ t('common.form.cancel') }}</button>
      <button
        type="button"
        class="bs-btn primary"
        :class="{ danger }"
        :disabled="!canSubmit"
        :data-testid="`${testid}-submit`"
        @click="submit(false)"
      >
        {{ busy ? busyLabel || primary : primary }}
      </button>
    </template>
  </BottomSheet>
</template>

<style scoped>
.pf {
  position: fixed;
  z-index: 2050;
  overflow: hidden;
  border-radius: var(--t-r-pop);
  background: var(--t-pop);
  box-shadow: var(--t-sh-pop);
  color: var(--t-text1);
  --t-surface: var(--t-pop);
}
.pf-head {
  display: flex;
  flex-direction: column;
  gap: 3px;
  padding: 14px 16px 2px;
}
.pf-title {
  font-size: 14.5px;
  font-weight: 650;
  line-height: 20px;
  text-wrap: pretty;
}
.pf-sub {
  font-size: 12.5px;
  line-height: 17px;
  color: var(--t-text3);
}
.pf-fields {
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding: 12px 16px 0;
}
.pf-fields.sheet {
  padding: 0;
}
.pf-foot {
  display: flex;
  align-items: center;
  gap: 8px;
  height: 48px;
  margin-top: 16px;
  padding: 0 10px 0 16px;
  background: color-mix(in srgb, var(--t-pop) 55%, var(--t-bg));
  font-size: 12px;
  color: var(--t-text3);
  white-space: nowrap;
}
.pf-hint {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  overflow: hidden;
  text-overflow: ellipsis;
}
.pf-spacer {
  flex: 1;
}
.pf-cancel,
.pf-primary {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  height: 30px;
  border: 0;
  border-radius: 8px;
  font: inherit;
  font-size: 13px;
  cursor: pointer;
}
.pf-cancel {
  padding: 0 5px 0 10px;
  background: transparent;
  color: var(--t-text2);
}
.pf-cancel:hover {
  background: var(--t-hover);
}
.pf-primary {
  padding: 0 5px 0 12px;
  background: var(--t-accent-grad);
  color: #fff;
  font-weight: 600;
}
.pf-primary:not(:disabled):hover {
  box-shadow: 0 4px 14px color-mix(in srgb, var(--t-primary) 45%, transparent);
}
.pf-primary:focus-visible,
.pf-cancel:focus-visible {
  outline: 2px solid var(--t-primary);
  outline-offset: 2px;
}
.pf-primary.danger {
  background: linear-gradient(
    to top right,
    color-mix(in srgb, #e0533d 86%, #000),
    #e0533d 50%,
    color-mix(in srgb, #e0533d 86%, #fff)
  );
}
.pf-primary.disabled {
  background: var(--t-pill);
  color: var(--t-placeholder);
  cursor: not-allowed;
}
.pf-primary.disabled .pf-cap {
  background: var(--t-kbd);
  color: var(--t-placeholder);
  box-shadow: none;
}
.pf-primary.busy {
  padding: 0 12px 0 10px;
  background: var(--t-accent-grad);
  opacity: 0.8;
  cursor: progress;
}
.pf-cap {
  background: rgba(255, 255, 255, 0.22);
  color: #fff;
  box-shadow: none;
}
.bs-btn {
  flex: 1;
  height: 46px;
  border: 0;
  border-radius: 12px;
  background: var(--t-pill);
  color: var(--t-text2);
  font: inherit;
  font-size: 15px;
  font-weight: 600;
}
.bs-btn.primary {
  flex: 2;
  background: var(--t-accent-grad);
  color: #fff;
}
.bs-btn.primary.danger {
  background: #e0533d;
}
.bs-btn.primary:disabled {
  background: var(--t-pill);
  color: var(--t-placeholder);
}
</style>
