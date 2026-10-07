<script setup>
// A panel docked inside the content area (v2, #2985 — mockup DockPanel): a
// 52px header (title, a quiet subtitle, actions, close), a scrolling body and
// an optional 40px footer of key hints. It is not an overlay: the owner puts it
// into a flex row next to its content, and the content gives up the width.
//
// Two looks, one primitive:
//   variant="pop"     — inside a board, under its sub-toolbar: --t-pop, radius
//                       18, the popover shadow (board customization, #2986);
//   variant="sidebar" — at layout level, under the top bar: --t-sidebar, no
//                       shadow («Входящие», #2972).
// On a phone (≤768px) the same content becomes a bottom sheet (kit/BottomSheet)
// with its handle and without key hints.
//
// Esc inside the panel closes it. The height runs from wherever the panel
// starts down to `gap` px above the bottom of the window, so it never pushes
// the page into scrolling.
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import { CloseOutline } from '@vicons/ionicons5'
import BottomSheet from './BottomSheet.vue'
import ShortcutKeys from '@/components/ShortcutKeys.vue'
import { useResponsive } from '@/composables/useResponsive'

const props = defineProps({
  show: { type: Boolean, default: false },
  title: { type: String, default: '' },
  subtitle: { type: String, default: '' },
  variant: { type: String, default: 'pop' }, // 'pop' | 'sidebar'
  width: { type: Number, default: 340 },
  // Space kept free under the panel (and the right margin of the pop variant).
  gap: { type: Number, default: 12 },
  // Catalog id of the key that toggles the panel — shown next to the close «×».
  toggleKey: { type: String, default: '' },
  // Footer key hints: [{ caps: ['↑↓'] | id: '<catalog id>', text }] — split by
  // `{ spacer: true }`. Desktop only; nothing on a phone.
  hints: { type: Array, default: () => [] },
  testid: { type: String, default: 'dock-panel' },
})
const emit = defineEmits(['update:show', 'close'])
const { t } = useI18n()
const { isMobile } = useResponsive()

function close() {
  emit('update:show', false)
  emit('close')
}

const el = ref(null)
const height = ref(0)
let scroller = null

function scrollParent(node) {
  for (let p = node?.parentElement; p; p = p.parentElement) {
    const oy = getComputedStyle(p).overflowY
    if (oy === 'auto' || oy === 'scroll') return p
  }
  return null
}
function measure() {
  const node = el.value
  if (!node) return
  // A sticky panel reports its stuck position; the scroller's top is the floor.
  const top = Math.max(node.getBoundingClientRect().top, scroller?.getBoundingClientRect().top || 0)
  height.value = Math.max(240, Math.round(window.innerHeight - top - props.gap))
}
function bind() {
  unbind()
  scroller = scrollParent(el.value)
  window.addEventListener('resize', measure)
  scroller?.addEventListener('scroll', measure, { passive: true })
  measure()
}
function unbind() {
  window.removeEventListener('resize', measure)
  scroller?.removeEventListener('scroll', measure)
  scroller = null
}
watch(
  () => props.show && !isMobile.value,
  async (on) => {
    if (!on) return unbind()
    await nextTick()
    bind()
  },
  { immediate: true },
)
onBeforeUnmount(unbind)

function onKeydown(e) {
  if (e.key !== 'Escape' || e.isComposing) return
  // A field or a menu inside the panel answers Esc first (they stop it).
  e.stopPropagation()
  close()
}

const style = computed(() => ({
  width: `${props.width}px`,
  flexBasis: `${props.width}px`,
  height: height.value ? `${height.value}px` : undefined,
  marginRight: props.variant === 'pop' ? `${props.gap}px` : undefined,
}))
defineExpose({ measure, el })
</script>

<template>
  <BottomSheet
    v-if="isMobile"
    :show="show"
    :testid="testid"
    class="dock-sheet"
    @update:show="(v) => (v ? null : close())"
  >
    <div class="dock-sheet-head">
      <span class="dock-title">{{ title }}</span>
      <span v-if="subtitle" class="dock-sub">{{ subtitle }}</span>
      <span class="dock-fill" />
      <slot name="actions" />
      <button type="button" class="dock-x touch" :title="t('common.action.close')" @click="close">
        <n-icon :component="CloseOutline" :size="20" />
      </button>
    </div>
    <div class="dock-sheet-body"><slot /></div>
  </BottomSheet>
  <transition v-else name="dock">
    <aside
      v-if="show"
      ref="el"
      class="dock"
      :class="variant"
      :style="style"
      :data-testid="testid"
      @keydown="onKeydown"
    >
      <header class="dock-head">
        <span class="dock-title">{{ title }}</span>
        <span v-if="subtitle" class="dock-sub">{{ subtitle }}</span>
        <span class="dock-fill" />
        <slot name="actions" />
        <button
          type="button"
          class="dock-x"
          :title="t('common.action.close')"
          data-testid="dock-close"
          @click="close"
        >
          <ShortcutKeys v-if="toggleKey" :id="toggleKey" />
          <n-icon :component="CloseOutline" :size="17" />
        </button>
      </header>
      <div class="dock-body"><slot /></div>
      <footer v-if="hints.length || $slots.footer" class="dock-foot">
        <slot name="footer">
          <template v-for="(h, i) in hints" :key="i">
            <span v-if="h.spacer" class="dock-fill" />
            <span v-else class="dock-hint">
              <ShortcutKeys v-if="h.id" :id="h.id" />
              <template v-else>
                <kbd v-for="(c, j) in h.caps" :key="j" class="dock-kbd">{{ c }}</kbd>
              </template>
              {{ h.text }}
            </span>
          </template>
        </slot>
      </footer>
    </aside>
  </transition>
</template>

<style scoped>
.dock {
  position: sticky;
  top: 0;
  flex: 0 0 auto;
  align-self: flex-start;
  box-sizing: border-box;
  display: flex;
  flex-direction: column;
  min-height: 240px;
  border-radius: var(--t-r-col);
  overflow: hidden;
  color: var(--t-text1);
}
.dock.pop {
  --t-surface: var(--t-pop);
  background: var(--t-pop);
  box-shadow: var(--t-sh-pop);
}
.dock.sidebar {
  --t-surface: var(--t-sidebar);
  background: var(--t-sidebar);
}
.dock-head {
  flex: 0 0 52px;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 8px 0 16px;
  min-width: 0;
}
.dock-title {
  font-size: 15px;
  font-weight: 650;
  white-space: nowrap;
}
.dock-sub {
  min-width: 0;
  font-size: 12.5px;
  color: var(--t-text3);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.dock-fill {
  flex: 1;
}
.dock-x {
  flex: none;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 30px;
  padding: 0 5px 0 8px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: var(--t-text3);
  font: inherit;
  cursor: pointer;
}
.dock-x:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.dock-x:focus-visible {
  outline: 2px solid var(--t-primary);
  outline-offset: -2px;
}
.dock-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 0 6px 8px;
  scrollbar-width: thin;
}
.dock-foot {
  flex: 0 0 40px;
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 0 14px;
  background: var(--t-foot);
  font-size: 11.5px;
  color: var(--t-text3);
  white-space: nowrap;
}
.dock-hint {
  display: inline-flex;
  align-items: center;
  gap: 5px;
}
.dock-kbd {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 20px;
  height: 20px;
  padding: 0 5px;
  box-sizing: border-box;
  border-radius: var(--t-r-kbd);
  background: var(--t-kbd);
  color: var(--t-text2);
  font: inherit;
  font-size: 11px;
  font-weight: 600;
  box-shadow: 0 1px 1px rgba(0, 0, 0, 0.22);
}
.dock-enter-active,
.dock-leave-active {
  transition:
    opacity 0.16s ease,
    transform 0.16s ease;
}
.dock-enter-from,
.dock-leave-to {
  opacity: 0;
  transform: translateX(12px);
}
@media (prefers-reduced-motion: reduce) {
  .dock-enter-active,
  .dock-leave-active {
    transition: none;
  }
}

/* ── phone: the bottom sheet ── */
.dock-sheet-head {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}
.dock-x.touch {
  width: 40px;
  height: 40px;
  padding: 0;
  justify-content: center;
  border-radius: 12px;
  background: var(--t-pill);
  color: var(--t-text2);
}
.dock-sheet-body {
  margin: 0 -10px;
}
</style>
