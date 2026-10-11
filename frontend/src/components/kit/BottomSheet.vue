<script setup>
// The phone form of tier 1 (v2, #2969): a popover form becomes a sheet sliding
// up from the bottom, with a handle, over a scrim. No key caps on touch.
// `full` is tier 2 — a sheet that takes the whole height under the status bar.
//
// On the touch (#2976 — package G «BottomSheet»): two snaps, «половина» (56%
// of the screen) and «полный» (up to the status bar). The handle and anything
// marked `data-sheet-grab` (a sheet's own header) drag it: a flick up opens it
// in full, down drops it back to half, and from half a pull past 30% of its
// height — or a flick down — closes it. A sheet with a field rides above the
// on-screen keyboard (--kb-h, useVisualViewport).
//
// `page` is the phone's page-like sheet («Входящие»): from the status bar down
// to the bottom navigation, which stays live under it — no scrim, no handle.
//
// `bare` is a sheet over a live screen (the tour on a phone, #3000): no scrim of
// its own — the screen around it keeps taking taps — and `z` lifts it over a
// layer that draws its own dim (the tour's cutout). `lift` stands it on the
// bottom navigation instead of over it; `top` docks it under the status bar —
// for when what it talks about sits where the sheet would be.
import { computed, ref, toRef, watch, onBeforeUnmount } from 'vue'
import { useOverlayBack } from '@/composables/useOverlayBack'
import { vLayer } from '@/utils/layerZ'

const props = defineProps({
  show: { type: Boolean, default: false },
  title: { type: String, default: '' },
  subtitle: { type: String, default: '' },
  full: { type: Boolean, default: false },
  page: { type: Boolean, default: false },
  bare: { type: Boolean, default: false },
  z: { type: Number, default: 0 },
  lift: { type: Boolean, default: false },
  top: { type: Boolean, default: false },
  testid: { type: String, default: 'bottom-sheet' },
  // Where the sheet is mounted: <body>, or the body of the Naive modal it was
  // opened from — that modal's focus trap rejects any focus outside it (#3113).
  to: { type: [String, Object], default: 'body' },
})
const emit = defineEmits(['update:show', 'close'])

function close() {
  emit('update:show', false)
  emit('close')
}
const open = computed(() => props.show)
useOverlayBack(toRef(props, 'show'), close)

// ── snaps and the drag ──
const expanded = ref(false)
watch(open, (v) => !v && (expanded.value = false))
const sheet = ref(null)
const dragY = ref(0)
const dragging = ref(false)
let track = null

const MOVE_MIN = 6
function onDown(e) {
  if (props.page || e.button > 0) return
  if (!e.target.closest?.('[data-sheet-grab], .bs-grab')) return
  track = { y: e.clientY, t: performance.now(), id: e.pointerId }
  window.addEventListener('pointermove', onMove, { passive: false })
  window.addEventListener('pointerup', onUp)
  window.addEventListener('pointercancel', onCancel)
}
function onMove(e) {
  if (!track || e.pointerId !== track.id) return
  const dy = e.clientY - track.y
  if (!dragging.value && Math.abs(dy) < MOVE_MIN) return
  dragging.value = true
  e.preventDefault()
  // Upwards it only resists: the snap above is reached on release.
  dragY.value = dy > 0 ? dy : dy * 0.3
}
function finish() {
  window.removeEventListener('pointermove', onMove)
  window.removeEventListener('pointerup', onUp)
  window.removeEventListener('pointercancel', onCancel)
  track = null
  dragY.value = 0
  // A drag that moved swallows the click that follows it (a header button
  // under the finger must not fire on release).
  if (dragging.value) {
    const swallow = (ev) => {
      ev.stopPropagation()
      ev.preventDefault()
    }
    window.addEventListener('click', swallow, { capture: true, once: true })
    setTimeout(() => window.removeEventListener('click', swallow, { capture: true }), 0)
  }
  dragging.value = false
}
function onUp(e) {
  if (!track || !dragging.value) return finish()
  const dy = e.clientY - track.y
  const v = dy / Math.max(1, performance.now() - track.t) // px per ms
  const h = sheet.value?.offsetHeight || 1
  if (dy < -40 || v < -0.5) {
    if (!props.full) expanded.value = true
  } else if (dy > h * 0.3 || v > 0.6) {
    if (expanded.value && !props.full && dy < h * 0.6) expanded.value = false
    else close()
  }
  finish()
}
function onCancel() {
  finish()
}
onBeforeUnmount(finish)

const sheetStyle = computed(() =>
  dragY.value ? { transform: `translateY(${Math.round(dragY.value)}px)` } : null,
)
</script>

<template>
  <Teleport :to="to">
    <transition name="bs">
      <div
        v-if="open"
        v-layer="{ zIndex: z || undefined }"
        class="bs-scrim"
        :class="{ page, bare }"
        @click.self="!page && !bare && close()"
      >
        <div
          ref="sheet"
          class="bs kit-sheet"
          :class="{ full: full || expanded, page, dragging, lift, top }"
          :style="sheetStyle"
          role="dialog"
          aria-modal="true"
          :data-testid="testid"
          @pointerdown="onDown"
        >
          <div v-if="!page" class="bs-grab" aria-hidden="true">
            <span class="bs-handle" />
          </div>
          <div v-if="title || subtitle" class="bs-head" data-sheet-grab>
            <div class="bs-title">{{ title }}</div>
            <div v-if="subtitle" class="bs-sub">{{ subtitle }}</div>
          </div>
          <div class="bs-body"><slot /></div>
          <div v-if="$slots.actions" class="bs-actions"><slot name="actions" /></div>
        </div>
      </div>
    </transition>
  </Teleport>
</template>

<style scoped>
.bs-scrim {
  position: fixed;
  inset: 0;
  z-index: 2100;
  background: var(--t-scrim);
}
.bs {
  position: absolute;
  left: 0;
  right: 0;
  /* Over the on-screen keyboard when a field inside has brought it up. */
  bottom: var(--kb-h, 0px);
  display: flex;
  flex-direction: column;
  gap: 14px;
  /* «Половина»: 56% of the screen; the content scrolls inside, the handle
     opens the rest. */
  max-height: min(56%, calc(100% - var(--kb-h, 0px) - var(--safe-top) - 8px));
  /* With the keyboard up the gesture bar is under it: no room kept for it. */
  padding: 0 16px max(14px, calc(30px + var(--safe-bottom) - var(--kb-h, 0px)));
  box-sizing: border-box;
  border-radius: 18px 18px 0 0;
  background: var(--t-pop);
  box-shadow: var(--t-sh-drag);
  overflow-y: auto;
  overscroll-behavior: contain;
  transition:
    transform 0.2s ease,
    max-height 0.2s ease;
}
.bs.dragging {
  transition: none;
}
/* «Полный»: up to the status bar. */
.bs.full {
  max-height: calc(100% - var(--kb-h, 0px) - var(--safe-top) - 8px);
}
.bs.full:not(.page) {
  top: calc(var(--safe-top) + 8px);
}
.bs-grab {
  flex: none;
  display: flex;
  justify-content: center;
  height: 22px;
  margin: 0 -16px -8px;
  padding-top: 8px;
  box-sizing: border-box;
  touch-action: none;
  cursor: grab;
}
.bs-handle {
  width: 36px;
  height: 5px;
  border-radius: 3px;
  background: var(--t-kbd);
}
.bs-head {
  display: flex;
  flex-direction: column;
  gap: 2px;
  touch-action: none;
}
.bs-title {
  font-size: 17px;
  font-weight: 650;
  color: var(--t-text1);
}
.bs-sub {
  font-size: 13px;
  color: var(--t-text3);
}
.bs-body {
  display: flex;
  flex-direction: column;
  gap: 14px;
}
.bs.full .bs-body {
  flex: 1;
}
.bs-actions {
  display: flex;
  gap: 10px;
  margin-top: 4px;
}
/* Standing on the bottom navigation; or docked at the top of the screen. */
.bs.lift {
  bottom: var(--mnav-h, 0px);
  padding-bottom: 16px;
  border-radius: 24px 24px 0 0;
}
.bs.top {
  top: calc(var(--safe-top) + 8px);
  bottom: auto;
  margin: 0 8px;
  padding-top: 16px;
  padding-bottom: 16px;
  border-radius: 24px;
}
.bs.top .bs-grab {
  display: none;
}
/* A sheet over a live screen: the scrim is see-through and lets taps through;
   only the sheet itself takes them. */
.bs-scrim.bare {
  background: transparent;
  pointer-events: none;
}
.bs-scrim.bare .bs {
  pointer-events: auto;
}
/* The page-like sheet: under the status bar down to the bottom navigation,
   which stays live (#2976). Below the FAB (60) and the tab bar (90). */
.bs-scrim.page {
  z-index: 55;
  bottom: var(--mnav-h, 0px);
  background: transparent;
}
.bs.page {
  top: 0;
  bottom: 0;
  max-height: none;
  padding: var(--safe-top) 14px 12px;
  border-radius: 0;
  background: var(--t-bg);
  box-shadow: none;
}
.bs-enter-active,
.bs-leave-active {
  transition: opacity 0.18s ease;
}
.bs-enter-active .bs,
.bs-leave-active .bs {
  transition: transform 0.22s ease;
}
.bs-enter-from,
.bs-leave-to {
  opacity: 0;
}
.bs-enter-from .bs,
.bs-leave-to .bs {
  transform: translateY(24px);
}
@media (prefers-reduced-motion: reduce) {
  .bs-enter-active,
  .bs-leave-active,
  .bs-enter-active .bs,
  .bs-leave-active .bs {
    transition: none;
  }
}
</style>
