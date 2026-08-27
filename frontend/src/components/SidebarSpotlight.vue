<script setup>
import { ref, computed, watch, nextTick, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import { SparklesOutline } from '@vicons/ionicons5'
import { useWhatsNewStore } from '@/stores/whatsNew'
import { useResponsive } from '@/composables/useResponsive'
import { layoutArrow, ARROW_HEAD } from '@/utils/tourArrow'

// One-at-a-time sidebar spotlight hint (#2749), shown after the What's-New modal:
// a glowing popover with a curved gradient arrow that "draws" itself from the
// popover to the target sidebar item, which then nods toward it (variant D).
//
// The overlay is fixed to the viewport and teleported to <body>. It anchors to
// the nav item carrying data-nav="<navKey>" (see Sidebar.vue). Skipped on mobile,
// where the sidebar lives in a closed drawer.
const { t } = useI18n()
const store = useWhatsNewStore()
const { isMobile } = useResponsive()

const spot = computed(() => store.currentSpotlight)

const active = ref(false)
const playing = ref(false)
const pop = ref(null)
const layer = ref(null)
const arrowPath = ref(null)
const arrowHead = ref(null)
const popStyle = ref({})
const pathLen = ref(0)
const headGap = ref(0) // px of the tip left undrawn by the stroke (the arrowhead)
let targetEl = null
let nudgeTimer = null

function clearNudge() {
  if (targetEl) targetEl.classList.remove('sl-nudge')
  clearTimeout(nudgeTimer)
}

// Measure the target + popover and lay out the arrow. Runs after the popover is
// in the DOM so we can read its real box.
function place() {
  if (!targetEl || !pop.value) return
  const t = targetEl.getBoundingClientRect()
  const gap = 76
  const popW = pop.value.offsetWidth || 240
  const popH = pop.value.offsetHeight || 96
  let left = t.right + gap
  // Keep the popover on-screen if the rail is wide / viewport is narrow.
  left = Math.min(left, window.innerWidth - popW - 16)
  // Default: hang the popover just below the target's bottom. For a target low in
  // the viewport (the footer «Помощь» button) that would run off the bottom edge,
  // so flip it above the target instead — the arrow then draws downward to it.
  const below = t.bottom + 4 + popH <= window.innerHeight - 8
  const top = below ? Math.round(t.bottom + 4) : Math.round(t.top - 4 - popH)
  popStyle.value = { left: left + 'px', top: top + 'px' }

  nextTick(() => {
    const p = pop.value.getBoundingClientRect()
    // Aim the arrow at the item's *content*, not the full-width nav row: the tip
    // lands just past the label/badge (close to the text) rather than at the far
    // right of the sidebar — but never on the "alpha" badge itself.
    const anchor =
      targetEl.querySelector('.nav-badge') || targetEl.querySelector('span') || targetEl
    const a = anchor.getBoundingClientRect()
    // Arrow: from the popover's near-left corner to just right of the label/badge.
    // When the popover sits above the target, leave from its bottom edge so the
    // line curves down onto the item instead of doubling back over itself.
    const x1 = p.left + 14
    const y1 = below ? p.top + 8 : p.bottom - 8
    // Nav rows: land the tip just past the label/badge. A flipped (footer) target is
    // a compact icon button, not a wide row — aim at its centre so the arrowhead sits
    // on the icon itself rather than in the gap toward the next control.
    const x2 = below ? a.right + 12 : a.left + a.width / 2
    const y2 = t.top + t.height / 2
    // Tighter arc for the narrow gap (smaller "radius" than the mockup) and a
    // sharp head that the stroke stops ARROW_HEAD px short of — shared with the
    // Get Started tour, see utils/tourArrow.js (#2753).
    const { len, head } = layoutArrow(arrowPath.value, { x: x1, y: y1 }, { x: x2, y: y2 })
    pathLen.value = len
    headGap.value = ARROW_HEAD
    arrowHead.value.setAttribute('points', head)

    // Kick off: draw the arrow, then nod the target as the arrowhead lands.
    playing.value = false
    void layer.value.offsetWidth
    playing.value = true
    clearNudge()
    nudgeTimer = setTimeout(() => {
      targetEl?.classList.add('sl-nudge')
      nudgeTimer = setTimeout(() => targetEl?.classList.remove('sl-nudge'), 2400)
    }, 820)
  })
}

async function show() {
  if (isMobile.value || !spot.value) {
    active.value = false
    return
  }
  targetEl = document.querySelector(`[data-nav="${spot.value.navKey}"]`)
  if (!targetEl) {
    // The section this hint points at isn't in the sidebar right now — skip it so
    // we never draw an arrow to nothing (the ack still happens on dismiss).
    active.value = false
    return
  }
  active.value = true
  await nextTick()
  place()
}

function dismiss() {
  const key = spot.value?.navKey
  clearNudge()
  active.value = false
  playing.value = false
  if (key) store.dismissSpotlight(key) // advances to the next queued spotlight
}

function onResize() {
  if (active.value) place()
}
watch(spot, show, { immediate: true })
window.addEventListener('resize', onResize)
onBeforeUnmount(() => {
  window.removeEventListener('resize', onResize)
  clearNudge()
})
</script>

<template>
  <teleport to="body">
    <div v-if="active" ref="layer" class="sl-layer" :class="{ playing }">
      <svg class="sl-arrow" aria-hidden="true">
        <!-- Reuses the app-wide accent gradient def from App.vue (theme-aware). -->
        <path
          ref="arrowPath"
          class="sl-path"
          :style="{ '--len': pathLen, '--gap': headGap }"
          fill="none"
          stroke="url(#t-accent-grad-svg)"
        />
        <polygon ref="arrowHead" class="sl-head" />
      </svg>
      <div
        ref="pop"
        class="sl-pop"
        :style="popStyle"
        role="dialog"
        :aria-label="t('shell.spotlight.aria')"
      >
        <div class="sl-title">
          <n-icon :component="SparklesOutline" :size="15" class="sl-spark" />
          {{ spot?.titleKey ? t(spot.titleKey) : '' }}
        </div>
        <div class="sl-body">{{ spot?.bodyKey ? t(spot.bodyKey) : '' }}</div>
        <button class="sl-btn" @click="dismiss">{{ t('shell.spotlight.dismiss') }}</button>
      </div>
    </div>
  </teleport>
</template>

<style scoped>
.sl-layer {
  position: fixed;
  inset: 0;
  z-index: 7000; /* above content, below the app loader/connection overlay */
  pointer-events: none;
}
.sl-arrow {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  overflow: visible;
}
.sl-path {
  stroke-width: 2.4;
  stroke-linecap: round;
  stroke-dasharray: var(--len);
  stroke-dashoffset: var(--len);
}
.sl-head {
  fill: var(--t-primary);
  opacity: 0;
}
.sl-layer.playing .sl-path {
  animation: sl-draw 0.58s cubic-bezier(0.65, 0, 0.35, 1) 0.26s forwards;
}
.sl-layer.playing .sl-head {
  animation: sl-head 0.18s ease 0.76s forwards;
}
/* Stop the stroke --gap px short of the tip so the arrowhead's point stays sharp
   (the round cap sits behind the triangle base instead of poking through it). */
@keyframes sl-draw {
  to {
    stroke-dashoffset: var(--gap);
  }
}
@keyframes sl-head {
  to {
    opacity: 1;
  }
}

.sl-pop {
  position: absolute;
  width: 240px;
  pointer-events: auto;
  background: var(--t-surface);
  border: 1px solid color-mix(in srgb, var(--t-primary) 55%, var(--t-border));
  border-radius: 14px;
  padding: 13px 14px 12px;
  /* The "glowing" accent popover the user picked. */
  box-shadow:
    0 12px 34px rgba(124, 92, 255, 0.24),
    0 0 0 1px color-mix(in srgb, var(--t-primary) 30%, transparent);
  opacity: 0;
  transform: translateY(8px) scale(0.98);
}
.sl-layer.playing .sl-pop {
  animation: sl-pop 0.42s cubic-bezier(0.22, 1, 0.36, 1) forwards;
}
@keyframes sl-pop {
  to {
    opacity: 1;
    transform: translateY(0) scale(1);
  }
}
.sl-title {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13.5px;
  font-weight: 700;
  color: var(--t-text1);
  margin-bottom: 4px;
}
.sl-spark {
  color: var(--t-primary);
}
.sl-body {
  font-size: 12.5px;
  color: var(--t-text3);
  line-height: 1.45;
  margin-bottom: 10px;
}
.sl-btn {
  font: inherit;
  font-size: 12px;
  font-weight: 600;
  padding: 5px 12px;
  border-radius: 8px;
  border: none;
  background: var(--t-accent-grad, var(--t-primary));
  color: var(--t-on-primary, #fff);
  cursor: pointer;
}
@media (prefers-reduced-motion: reduce) {
  .sl-layer.playing .sl-path,
  .sl-layer.playing .sl-head,
  .sl-layer.playing .sl-pop {
    animation-duration: 0.01s;
  }
}
</style>
