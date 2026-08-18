<script setup>
import { ref, computed, watch, nextTick, onBeforeUnmount } from 'vue'
import { NIcon } from 'naive-ui'
import { SparklesOutline } from '@vicons/ionicons5'
import { useWhatsNewStore } from '@/stores/whatsNew'
import { useResponsive } from '@/composables/useResponsive'

// One-at-a-time sidebar spotlight hint (#2749), shown after the What's-New modal:
// a glowing popover with a curved gradient arrow that "draws" itself from the
// popover to the target sidebar item, which then nods toward it (variant D).
//
// The overlay is fixed to the viewport and teleported to <body>. It anchors to
// the nav item carrying data-nav="<navKey>" (see Sidebar.vue). Skipped on mobile,
// where the sidebar lives in a closed drawer.
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
  let left = t.right + gap
  // Keep the popover on-screen if the rail is wide / viewport is narrow.
  left = Math.min(left, window.innerWidth - popW - 16)
  const top = Math.round(t.bottom + 4)
  popStyle.value = { left: left + 'px', top: top + 'px' }

  nextTick(() => {
    const p = pop.value.getBoundingClientRect()
    // Arrow: from the popover's top-left corner up to the target's right edge.
    const x1 = p.left + 14
    const y1 = p.top + 8
    const x2 = t.right + 6
    const y2 = t.top + t.height / 2
    const dx = x1 - x2 // > 0 (popover is right of the item)
    const dy = y1 - y2 // > 0 (popover is below the item)
    // Tighter arc for the narrow gap (smaller "radius" than the mockup): keep the
    // control points close to the endpoints with a shallow upward bow.
    const bow = Math.min(20, dy * 0.35)
    const cx1 = x1 - dx * 0.55
    const cy1 = y1 - bow
    const cx2 = x2 + dx * 0.16
    const cy2 = y2 - bow * 0.7
    const d = `M ${x1} ${y1} C ${cx1} ${cy1}, ${cx2} ${cy2}, ${x2} ${y2}`
    const path = arrowPath.value
    path.setAttribute('d', d)
    const len = path.getTotalLength()
    pathLen.value = len

    // Arrowhead oriented along the final tangent.
    const end = path.getPointAtLength(len)
    const near = path.getPointAtLength(Math.max(0, len - 8))
    const ang = Math.atan2(end.y - near.y, end.x - near.x)
    const s = 7
    const a1 = ang + Math.PI - 0.5
    const a2 = ang + Math.PI + 0.5
    arrowHead.value.setAttribute(
      'points',
      `${end.x},${end.y} ${end.x + s * Math.cos(a1)},${end.y + s * Math.sin(a1)} ${end.x + s * Math.cos(a2)},${end.y + s * Math.sin(a2)}`,
    )

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
          :style="{ '--len': pathLen }"
          fill="none"
          stroke="url(#t-accent-grad-svg)"
        />
        <polygon ref="arrowHead" class="sl-head" />
      </svg>
      <div ref="pop" class="sl-pop" :style="popStyle" role="dialog" aria-label="Подсказка">
        <div class="sl-title">
          <n-icon :component="SparklesOutline" :size="15" class="sl-spark" />
          {{ spot?.title }}
        </div>
        <div class="sl-body">{{ spot?.body }}</div>
        <button class="sl-btn" @click="dismiss">Понятно</button>
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
  animation: sl-head 0.18s ease 0.78s forwards;
}
@keyframes sl-draw {
  to {
    stroke-dashoffset: 0;
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
