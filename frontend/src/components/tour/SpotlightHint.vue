<script setup>
import { ref, computed, watch, nextTick, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import { SparklesOutline } from '@vicons/ionicons5'
import ShortcutKeys from '../ShortcutKeys.vue'
import { useHintsStore, MAX_PER_SESSION } from '@/stores/hints'
import { useTourStore } from '@/stores/tour'
import { useWhatsNewStore } from '@/stores/whatsNew'
import { usePaletteStore } from '@/stores/palette'
import { useResponsive } from '@/composables/useResponsive'
import { useShortcuts } from '@/composables/useShortcuts'
import { sidebarDragging } from '@/composables/useSidebarDnd'
import { boardDragging } from '@/composables/useBoardDragScroll'
import { layoutArrow, tourArc, ARROW_HEAD, choosePlacement } from '@/utils/tourArrow'
import { isTypingTarget } from '@/utils/keys'

// The spotlight card (#2749 → v2 #3000, mockups SpotCard / TourLayer «sp»): one
// hint at a time at ANY element — a top-bar button, the board's sub-bar, the
// stage strip — not only sidebar items. No dim: the target gets a soft halo
// and a short nudge once the arrow lands.
//
// What is eligible is stores/hints.js's call; here we find a hint whose element
// is on screen right now and a moment when the screen is free: no tour, no
// window or form or menu, no palette, no drag, no typing. A hint whose element
// isn't there doesn't block the next one, and comes back when it appears.
//
// Closing: «Понятно», ↵ or Esc (while the focus isn't in a field), or a click
// on the target itself — each retires the hint for good. A click elsewhere
// doesn't. The card never takes the focus.
//
// Phone: a compact card over the bottom navigation, left of the «+», with a
// tail at the tab it is about; a swipe down = «Понятно».

const CARD_W = 270
const GAPS = [56, 32, 16]
const POLL_MS = 700

const { t, te } = useI18n()
const hints = useHintsStore()
const tour = useTourStore()
const wn = useWhatsNewStore()
const palette = usePaletteStore()
const { isMobile } = useResponsive()

const hint = ref(null)
const card = ref(null)
const arrowPath = ref(null)
const cardStyle = ref({})
const arrow = ref({ len: 0, head: '', from: null, to: null })
const halo = ref(null)
const tail = ref(null)
const playing = ref(false)
let targetEl = null

const BUSY_SEL = [
  '.n-modal-container .n-modal',
  '.n-drawer',
  '.kit-form',
  '.kit-menu',
  '.n-dropdown-menu',
  '.n-popover:not(.n-tooltip)',
  '[role="dialog"][aria-modal="true"]',
  '.lw-card',
].join(',')

// Something owns the screen: the hint waits.
function busy() {
  if (tour.active || wn.showV2Intro || wn.historyOpen || wn.pending.length) return true
  if (palette.open) return true
  if (sidebarDragging.value || boardDragging.value) return true
  if (typeof document === 'undefined') return true
  if (isTypingTarget(document.activeElement)) return true
  return !!document.querySelector(BUSY_SEL)
}

function selectorOf(h) {
  return isMobile.value ? h.mobile || null : h.anchor
}
function visibleBox(el) {
  const r = el?.getBoundingClientRect?.()
  if (!r || (!r.width && !r.height)) return null
  if (r.right <= 0 || r.bottom <= 0 || r.left >= window.innerWidth || r.top >= window.innerHeight)
    return null
  return {
    left: r.left,
    top: r.top,
    width: r.width,
    height: r.height,
    right: r.right,
    bottom: r.bottom,
  }
}
function findTarget(h) {
  const sel = selectorOf(h)
  if (!sel) return null
  const el = document.querySelector(sel)
  return el && visibleBox(el) ? el : null
}

function hideCard() {
  hint.value = null
  targetEl = null
  playing.value = false
  hints.hide()
}

function tick() {
  if (hint.value) {
    if (busy() || !targetEl?.isConnected || !visibleBox(targetEl)) hideCard()
    else layout()
    return
  }
  if (busy()) return
  for (const h of hints.queue) {
    if (!hints.canShow(h.key)) continue
    const el = findTarget(h)
    if (!el) continue
    show(h, el)
    return
  }
}
const timer = setInterval(tick, POLL_MS)
onBeforeUnmount(() => clearInterval(timer))
// A hint dismissed elsewhere (Settings) or switched off drops off the screen.
watch(
  () => hints.queue.map((h) => h.key).join(','),
  () => {
    if (hint.value && !hints.queue.some((h) => h.key === hint.value.key)) hideCard()
  },
)

async function show(h, el) {
  hint.value = h
  targetEl = el
  hints.shown(h)
  await nextTick()
  layout()
  playing.value = false
  void card.value?.offsetWidth
  playing.value = true
}

function layout() {
  if (!hint.value || !targetEl || !card.value) return
  const r = visibleBox(targetEl)
  if (!r) return
  halo.value = r
  if (isMobile.value) {
    // Over the bottom navigation, left of the «+»; a tail at a tab.
    const inBar = !!targetEl.closest?.('[data-testid="mobile-tabbar"]')
    const cardBox = card.value.getBoundingClientRect()
    tail.value = inBar
      ? {
          left: Math.max(16, Math.min(cardBox.width - 30, r.left + r.width / 2 - cardBox.left - 7)),
        }
      : null
    return
  }
  const w = card.value.offsetWidth || CARD_W
  const h = card.value.offsetHeight || 150
  const p = choosePlacement(r, [r], [r], {
    popW: w,
    popH: h,
    vw: window.innerWidth,
    vh: window.innerHeight,
    gaps: GAPS,
    edge: 12,
  })
  cardStyle.value = { left: Math.round(p.left) + 'px', top: Math.round(p.top) + 'px' }
  nextTick(() => {
    if (!arrowPath.value || !card.value) return
    const c = card.value.getBoundingClientRect()
    const cx = r.left + r.width / 2
    const cy = r.top + r.height / 2
    const clamp = (v, lo, hi) => Math.max(lo, Math.min(hi, v))
    let from, to
    if (p.side === 'bottom') {
      from = { x: clamp(cx, c.left + 20, c.right - 20), y: c.top + 2 }
      to = { x: cx, y: r.bottom + 9 }
    } else if (p.side === 'top') {
      from = { x: clamp(cx, c.left + 20, c.right - 20), y: c.bottom - 2 }
      to = { x: cx, y: r.top - 9 }
    } else if (p.side === 'left') {
      from = { x: c.right - 2, y: clamp(cy, c.top + 16, c.bottom - 16) }
      to = { x: r.left - 9, y: cy }
    } else {
      from = { x: c.left + 2, y: clamp(cy, c.top + 16, c.bottom - 16) }
      to = { x: r.right + 9, y: cy }
    }
    arrow.value = {
      ...layoutArrow(arrowPath.value, from, to, ARROW_HEAD, (f, tt) => tourArc(f, tt, p.side)),
      from,
      to,
    }
  })
}
function onViewport() {
  if (hint.value) layout()
}
window.addEventListener('resize', onViewport)
window.addEventListener('scroll', onViewport, true)
onBeforeUnmount(() => {
  window.removeEventListener('resize', onViewport)
  window.removeEventListener('scroll', onViewport, true)
})

function dismiss() {
  const h = hint.value
  if (!h) return
  hideCard()
  hints.dismiss(h)
}

// A click on the target itself also retires the hint — and still does what
// the click was for.
function onDocClick(e) {
  if (!hint.value || !targetEl || !(e.target instanceof Node)) return
  if (targetEl.contains(e.target)) dismiss()
}
document.addEventListener('click', onDocClick, true)
onBeforeUnmount(() => document.removeEventListener('click', onDocClick, true))

useShortcuts({ 'hint.ok': dismiss, 'hint.close': dismiss }, { when: () => !!hint.value })

// Phone: a swipe down on the card = «Понятно».
let swipe = null
function onDown(e) {
  swipe = { y: e.clientY, id: e.pointerId }
}
function onUp(e) {
  if (swipe && e.pointerId === swipe.id && e.clientY - swipe.y > 40) dismiss()
  swipe = null
}

const isRelease = computed(() => !!hint.value?.release)
// On a phone a hint may say it differently (no keys to press there).
const bodyText = computed(() => {
  const h = hint.value
  if (!h) return ''
  const phone = `hints.items.${h.key}.mobile`
  return isMobile.value && !h.release && te(phone) ? t(phone) : t(h.bodyKey)
})
const pill = computed(() => {
  if (!hint.value) return ''
  if (hint.value.kind === 'ctx') return t('hints.kind.ctx')
  return t(isRelease.value ? 'hints.kind.release' : 'hints.kind.new')
})
const meta = computed(() => {
  const h = hint.value
  if (!h) return ''
  if (h.metaKey) return t(h.metaKey)
  const more = hints.queue.filter((x) => x.kind === 'new' && x.key !== h.key).length
  if (more && hints.budgetLeft > 0) return t('hints.card.more', { n: more })
  return t('hints.card.session', { n: MAX_PER_SESSION - hints.budgetLeft, of: MAX_PER_SESSION })
})
const haloStyle = computed(() => {
  const r = halo.value
  if (!r) return null
  return {
    left: r.left - 4 + 'px',
    top: r.top - 4 + 'px',
    width: r.width + 8 + 'px',
    height: r.height + 8 + 'px',
  }
})
</script>

<template>
  <teleport to="body">
    <div
      v-if="hint"
      class="sh-layer"
      :class="{ playing, mobile: isMobile }"
      data-testid="spotlight"
    >
      <!-- The target's halo: a soft accent fill, a 2px ring and a wide faint one;
           it nudges (scale 1.06) once the arrow lands. -->
      <span v-if="haloStyle" class="sh-halo" :style="haloStyle" aria-hidden="true" />
      <svg v-if="!isMobile" class="sh-arrow" aria-hidden="true">
        <defs>
          <linearGradient
            id="sh-g"
            gradientUnits="userSpaceOnUse"
            :x1="arrow.from?.x || 0"
            :y1="arrow.from?.y || 0"
            :x2="arrow.to?.x || 1"
            :y2="arrow.to?.y || 1"
          >
            <stop offset="0" class="sh-stop-a" />
            <stop offset="1" class="sh-stop-b" />
          </linearGradient>
        </defs>
        <path
          ref="arrowPath"
          class="sh-path"
          :style="{ '--len': arrow.len, '--gap': ARROW_HEAD }"
          fill="none"
          stroke="url(#sh-g)"
        />
        <polygon class="sh-head" :points="arrow.head" />
      </svg>
      <div
        ref="card"
        class="sh-card"
        :style="isMobile ? null : cardStyle"
        role="dialog"
        :aria-label="t('shell.spotlight.aria')"
        data-testid="spotlight-card"
        :data-hint="hint.key"
        :data-kind="hint.kind"
        @pointerdown="onDown"
        @pointerup="onUp"
      >
        <span v-if="isMobile && tail" class="sh-tail" :style="{ left: tail.left + 'px' }" />
        <div class="sh-top">
          <span class="sh-tile"><n-icon :component="SparklesOutline" :size="13" /></span>
          <span class="sh-pill" :class="hint.kind">{{ pill }}</span>
          <span class="sh-fill" />
          <ShortcutKeys v-if="!isMobile" id="hint.close" />
        </div>
        <div class="sh-txt">
          <span class="sh-title">{{ t(hint.titleKey) }}</span>
          <span class="sh-body">{{ bodyText }}</span>
        </div>
        <div class="sh-foot">
          <span v-if="!isMobile" class="sh-meta">{{ meta }}</span>
          <button type="button" class="sh-ok" data-testid="spotlight-ok" @click="dismiss">
            {{ t('shell.spotlight.dismiss') }}
            <ShortcutKeys v-if="!isMobile" id="hint.ok" />
          </button>
        </div>
      </div>
    </div>
  </teleport>
</template>

<style scoped>
.sh-layer {
  position: fixed;
  inset: 0;
  z-index: 6900; /* under the tour's layer, over the page */
  pointer-events: none;
}
.sh-arrow {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  overflow: visible;
}
.sh-stop-a {
  stop-color: color-mix(in srgb, var(--t-primary) 80%, #000);
}
.sh-stop-b {
  stop-color: color-mix(in srgb, var(--t-primary) 72%, #fff);
}
.sh-path {
  stroke-width: 2.25;
  stroke-linecap: round;
  stroke-dasharray: var(--len);
  stroke-dashoffset: var(--len);
}
.sh-head {
  fill: color-mix(in srgb, var(--t-primary) 72%, #fff);
  opacity: 0;
}
.sh-layer.playing .sh-path {
  animation: sh-draw 0.32s cubic-bezier(0.65, 0, 0.35, 1) 0.16s forwards;
}
.sh-layer.playing .sh-head {
  animation: sh-fade 0.12s ease 0.44s forwards;
}
@keyframes sh-draw {
  to {
    stroke-dashoffset: var(--gap);
  }
}
@keyframes sh-fade {
  to {
    opacity: 1;
  }
}
.sh-halo {
  position: absolute;
  box-sizing: border-box;
  border-radius: 10px;
  background: color-mix(in srgb, var(--t-primary) 8%, transparent);
  box-shadow:
    0 0 0 2px color-mix(in srgb, var(--t-primary) 62%, transparent),
    0 0 0 8px color-mix(in srgb, var(--t-primary) 16%, transparent);
  opacity: 0;
}
.sh-layer.playing .sh-halo {
  animation:
    sh-fade 0.16s ease 0.1s forwards,
    sh-nudge 0.18s ease-out 0.48s 2 alternate;
}
@keyframes sh-nudge {
  to {
    transform: scale(1.06);
  }
}

/* The card (SpotCard): 260–290, radius 16, --t-pop, the pop shadow with the
   accent's glow. Appears in 160 ms, rising 6px. */
.sh-card {
  position: absolute;
  box-sizing: border-box;
  width: 270px;
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 12px 12px 10px 14px;
  border-radius: var(--t-r-pop, 16px);
  background: var(--t-pop);
  box-shadow:
    var(--t-sh-pop),
    0 8px 32px color-mix(in srgb, var(--t-primary) 24%, transparent);
  color: var(--t-text1);
  pointer-events: auto;
  opacity: 0;
  transform: translateY(6px);
}
.sh-layer.playing .sh-card {
  animation: sh-in 0.16s ease-out forwards;
}
@keyframes sh-in {
  to {
    opacity: 1;
    transform: none;
  }
}
.sh-layer.mobile .sh-card {
  left: 12px;
  right: 96px;
  width: auto;
  bottom: calc(var(--mnav-h, 0px) + 10px);
  touch-action: pan-x;
}
.sh-tail {
  position: absolute;
  bottom: -6px;
  width: 14px;
  height: 14px;
  border-radius: 3px;
  transform: rotate(45deg);
  background: var(--t-pop);
}
.sh-top {
  display: flex;
  align-items: center;
  gap: 8px;
}
.sh-tile {
  flex: 0 0 22px;
  width: 22px;
  height: 22px;
  border-radius: 7px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--t-accent-grad);
  color: #fff;
}
.sh-pill {
  height: 20px;
  display: inline-flex;
  align-items: center;
  padding: 0 8px;
  border-radius: 999px;
  font-size: 11.5px;
  font-weight: 650;
  background: linear-gradient(
    to top right,
    color-mix(in srgb, var(--t-primary) 14%, var(--t-pop)),
    color-mix(in srgb, var(--t-primary) 26%, var(--t-pop))
  );
  color: var(--t-text1);
}
.sh-pill.ctx {
  background: var(--t-pill);
  color: var(--t-text2);
  font-weight: 600;
}
.sh-fill {
  flex: 1;
}
.sh-txt {
  display: flex;
  flex-direction: column;
  gap: 3px;
}
.sh-title {
  font-size: 14px;
  font-weight: 650;
  line-height: 1.3;
}
.sh-body {
  font-size: 13px;
  line-height: 1.45;
  color: var(--t-text2);
  text-wrap: pretty;
}
.sh-foot {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 2px;
}
.sh-meta {
  flex: 1;
  min-width: 0;
  font-size: 12px;
  color: var(--t-text3);
}
.sh-ok {
  height: 28px;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 0 4px 0 10px;
  border: none;
  border-radius: 8px;
  font: inherit;
  font-size: 12.5px;
  font-weight: 600;
  white-space: nowrap;
  cursor: pointer;
  color: var(--t-text1);
  background: linear-gradient(
    to top right,
    color-mix(in srgb, var(--t-primary) 14%, var(--t-pop)),
    color-mix(in srgb, var(--t-primary) 24%, var(--t-pop))
  );
}
.sh-ok:hover {
  background: var(--t-accent-grad);
  color: #fff;
  box-shadow: 0 2px 8px color-mix(in srgb, var(--t-primary) 30%, transparent);
}
.sh-ok:hover :deep(.kbd) {
  background: rgba(255, 255, 255, 0.22);
  color: #fff;
  box-shadow: none;
}
.sh-ok:focus {
  outline: none;
}
.sh-ok:focus-visible {
  box-shadow:
    0 0 0 2px var(--t-pop),
    0 0 0 4px color-mix(in srgb, var(--t-primary) 75%, transparent);
}
.sh-layer.mobile .sh-ok {
  flex: 1;
  height: 44px;
  justify-content: center;
  padding: 0;
  border-radius: 12px;
  font-size: 14px;
}
@media (prefers-reduced-motion: reduce) {
  .sh-layer.playing .sh-path,
  .sh-layer.playing .sh-head,
  .sh-layer.playing .sh-card,
  .sh-layer.playing .sh-halo {
    animation-duration: 0.01s;
  }
}
</style>
