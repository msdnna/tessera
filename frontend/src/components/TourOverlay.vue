<script setup>
import { ref, computed, watch, nextTick, onBeforeUnmount } from 'vue'
import { NIcon } from 'naive-ui'
import { SparklesOutline } from '@vicons/ionicons5'
import { useTourStore } from '@/stores/tour'
import { useTourAnchor, anchorSelector } from '@/composables/useTourAnchor'
import { layoutArrow, ARROW_HEAD } from '@/utils/tourArrow'

// The Get Started guide's viewport layer (#2753): a dimming mask with a cut-out
// around the element in play, one curved arrow per anchor, and the popover.
//
// Everything here is pointer-events: none except the popover itself — the whole
// point is that the user keeps clicking the real UI (open the dropdown, fill the
// modal) while the mask only *directs* attention. That's why the cut-out is
// purely visual: nothing is actually blocked, so a wrong turn can't trap anyone.

const POP_W = 260
const GAP = 76 // popover-to-target breathing room, matches SidebarSpotlight
const PAD = 6 // mask cut-out padding around the target
const EDGE = 16 // keep the popover this far from the viewport edge

const tour = useTourStore()
const step = computed(() => tour.current)

const pop = ref(null)
const layer = ref(null)
const arrowEls = ref([])
const playing = ref(false)
const popStyle = ref({})
const arrows = ref([]) // { len, head } per anchor, in `anchors` order

const { rects, els, count } = useTourAnchor(() => tour.anchors, {
  onMissing: () => tour.anchorMissing(step.value?.id),
  countFn: () => step.value?.advanceOn?.count || '',
})

const target = computed(() => rects.value[0] || null)
const masked = computed(() => step.value?.mask !== false)
const isAction = computed(() => step.value?.mode === 'action')

// Outer viewport rect + a rounded hole per anchor. fill-rule="evenodd" turns the
// inner sub-paths into holes.
const maskPath = computed(() => {
  const w = window.innerWidth
  const h = window.innerHeight
  let d = `M 0 0 H ${w} V ${h} H 0 Z`
  for (const r of rects.value) {
    if (!r) continue
    const x = r.left - PAD
    const y = r.top - PAD
    const rw = r.width + PAD * 2
    const rh = r.height + PAD * 2
    const rad = Math.min(10, rw / 2, rh / 2)
    d +=
      ` M ${x + rad} ${y} H ${x + rw - rad} A ${rad} ${rad} 0 0 1 ${x + rw} ${y + rad}` +
      ` V ${y + rh - rad} A ${rad} ${rad} 0 0 1 ${x + rw - rad} ${y + rh}` +
      ` H ${x + rad} A ${rad} ${rad} 0 0 1 ${x} ${y + rh - rad}` +
      ` V ${y + rad} A ${rad} ${rad} 0 0 1 ${x + rad} ${y} Z`
  }
  return d
})

// Park the popover beside the target: to its right when there's room, otherwise
// left, and clamped into the viewport either way.
function placePopover(t) {
  const popH = pop.value?.offsetHeight || 140
  let left = t.right + GAP
  if (left + POP_W > window.innerWidth - EDGE) {
    const leftSide = t.left - GAP - POP_W
    left = leftSide >= EDGE ? leftSide : Math.max(EDGE, window.innerWidth - POP_W - EDGE)
  }
  let top = t.bottom + 4
  if (top + popH > window.innerHeight - EDGE) top = Math.max(EDGE, t.top - popH - 4)
  popStyle.value = { left: Math.round(left) + 'px', top: Math.round(top) + 'px' }
}

// Where the arrow leaves the popover: the corner facing the target, inset so the
// stroke starts on the card rather than in mid-air.
function arrowOrigin(p, t) {
  const x = p.left + p.width / 2 > t.left + t.width / 2 ? p.left + 14 : p.right - 14
  const y = p.top + p.height / 2 > t.top + t.height / 2 ? p.top + 8 : p.bottom - 8
  return { x, y }
}

// Where it lands: just outside the target's edge that faces the popover, so the
// head never covers the thing it's pointing at.
function arrowTip(p, t) {
  const pcx = p.left + p.width / 2
  const pcy = p.top + p.height / 2
  const tcx = t.left + t.width / 2
  const tcy = t.top + t.height / 2
  if (Math.abs(pcy - tcy) > Math.abs(pcx - tcx) * 1.5) {
    return { x: tcx, y: pcy > tcy ? t.bottom + 12 : t.top - 12 }
  }
  return { x: pcx > tcx ? t.right + 12 : t.left - 12, y: tcy }
}

function place() {
  const t = target.value
  if (!t || !pop.value) return
  placePopover(t)
  nextTick(() => {
    if (!pop.value) return
    const p = pop.value.getBoundingClientRect()
    const from = arrowOrigin(p, t)
    arrows.value = rects.value.map((r, i) => {
      const el = arrowEls.value[i]
      if (!r || !el) return { len: 0, head: '' }
      return layoutArrow(el, from, arrowTip(p, r))
    })
  })
}

function replay() {
  playing.value = false
  void layer.value?.offsetWidth // reflow, so the draw animation restarts
  playing.value = true
}

// Action steps advance on the user clicking the anchor the step names. Listened
// for on the document in the capture phase: the element is often replaced (a
// dropdown re-renders, a modal remounts) between arming and the actual click.
function onDocClick(e) {
  const s = step.value
  const key = s?.advanceOn?.click
  if (!key || !(e.target instanceof Element)) return
  const sel = anchorSelector(key === true ? s.anchor : key)
  if (sel && e.target.closest(sel)) tour.clicked(s.id)
}
document.addEventListener('click', onDocClick, true)
onBeforeUnmount(() => document.removeEventListener('click', onDocClick, true))

// Steps that wait for an entity to be created report the match count on every
// mutation pass; re-reporting on a step change too gives the store its baseline
// even when the new step happens to start on the same number.
watch([count, () => step.value?.id], ([n, id]) => id && tour.counted(id, n), {
  immediate: true,
  flush: 'post',
})

// Some anchors are only revealed on hover (the sidebar's per-row buttons). Flag
// the live ones so the rule in main.css can hold them visible while the guide
// points at them — otherwise the arrow lands on an invisible target.
let marked = []
function unmark() {
  marked.forEach((el) => el.removeAttribute('data-tour-active'))
  marked = []
}
watch(
  els,
  (next) => {
    unmark()
    marked = (next || []).filter(Boolean)
    marked.forEach((el) => el.setAttribute('data-tour-active', ''))
  },
  // immediate: the composable resolves the first anchors while it is being set
  // up, i.e. before this watcher exists — without it the opening step of a tour
  // that starts before the overlay mounts is never flagged.
  { immediate: true, flush: 'post' },
)
onBeforeUnmount(unmark)

// Post-flush throughout: the popover is measured from its real box, so every
// layout pass has to run after the DOM is patched (on the step the layer mounts,
// `pop` doesn't exist yet during the pre-flush pass).
watch(rects, place, { deep: true, flush: 'post' })
watch(
  () => (step.value && target.value ? step.value.id : null),
  async (id) => {
    if (!id) return
    arrows.value = []
    await nextTick()
    place()
    replay()
  },
  { immediate: true, flush: 'post' },
)
</script>

<template>
  <teleport to="body">
    <div v-if="step && target" ref="layer" class="tr-layer" :class="{ playing }">
      <svg v-if="masked" class="tr-mask" aria-hidden="true">
        <path :d="maskPath" fill-rule="evenodd" />
      </svg>
      <svg class="tr-arrows" aria-hidden="true">
        <!-- Reuses the app-wide accent gradient def from App.vue (theme-aware). -->
        <g v-for="(a, i) in rects" :key="i">
          <path
            :ref="(el) => (arrowEls[i] = el)"
            class="tr-path"
            :style="{ '--len': arrows[i]?.len || 0, '--gap': ARROW_HEAD }"
            fill="none"
            stroke="url(#t-accent-grad-svg)"
          />
          <polygon class="tr-head" :points="arrows[i]?.head || ''" />
        </g>
      </svg>
      <div
        ref="pop"
        class="tr-pop"
        :style="popStyle"
        role="dialog"
        aria-label="Обучение"
        data-testid="tour-pop"
      >
        <div class="tr-title">
          <n-icon :component="SparklesOutline" :size="15" class="tr-spark" />
          {{ step.title }}
        </div>
        <div class="tr-body">{{ step.body }}</div>
        <div class="tr-actions">
          <button v-if="!isAction" class="tr-btn" data-testid="tour-next" @click="tour.next()">
            Понятно
          </button>
          <button class="tr-skip" data-testid="tour-skip" @click="tour.skip()">Пропустить</button>
        </div>
      </div>
    </div>
  </teleport>
</template>

<style scoped>
.tr-layer {
  position: fixed;
  inset: 0;
  z-index: 7000; /* above content, below the app loader/connection overlay */
  pointer-events: none;
}
.tr-mask,
.tr-arrows {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  overflow: visible;
}
.tr-mask path {
  /* Dim from the app background so the mask reads the same in both themes —
     a hard-coded rgba() would wash out the dark one. */
  fill: color-mix(in srgb, var(--t-bg) 72%, transparent);
}
.tr-path {
  stroke-width: 2.4;
  stroke-linecap: round;
  stroke-dasharray: var(--len);
  stroke-dashoffset: var(--len);
}
.tr-head {
  fill: var(--t-primary);
  opacity: 0;
}
.tr-layer.playing .tr-path {
  animation: tr-draw 0.58s cubic-bezier(0.65, 0, 0.35, 1) 0.26s forwards;
}
.tr-layer.playing .tr-head {
  animation: tr-fade 0.18s ease 0.76s forwards;
}
/* Stop the stroke --gap px short of the tip so the arrowhead's point stays sharp
   (the round cap sits behind the triangle base instead of poking through it). */
@keyframes tr-draw {
  to {
    stroke-dashoffset: var(--gap);
  }
}
@keyframes tr-fade {
  to {
    opacity: 1;
  }
}

.tr-pop {
  position: absolute;
  width: 260px;
  pointer-events: auto;
  background: var(--t-surface);
  border: 1px solid color-mix(in srgb, var(--t-primary) 55%, var(--t-border));
  border-radius: 14px;
  padding: 13px 14px 12px;
  box-shadow:
    0 12px 34px rgba(124, 92, 255, 0.24),
    0 0 0 1px color-mix(in srgb, var(--t-primary) 30%, transparent);
  opacity: 0;
  transform: translateY(8px) scale(0.98);
}
.tr-layer.playing .tr-pop {
  animation: tr-pop 0.42s cubic-bezier(0.22, 1, 0.36, 1) forwards;
}
@keyframes tr-pop {
  to {
    opacity: 1;
    transform: translateY(0) scale(1);
  }
}
.tr-title {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13.5px;
  font-weight: 700;
  color: var(--t-text1);
  margin-bottom: 4px;
}
.tr-spark {
  color: var(--t-primary);
}
.tr-body {
  font-size: 12.5px;
  color: var(--t-text3);
  line-height: 1.45;
  margin-bottom: 10px;
}
.tr-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}
.tr-btn {
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
.tr-skip {
  font: inherit;
  font-size: 12px;
  font-weight: 500;
  padding: 5px 8px;
  border-radius: 8px;
  border: none;
  background: transparent;
  color: var(--t-text3);
  cursor: pointer;
}
.tr-skip:hover {
  color: var(--t-text1);
}
@media (prefers-reduced-motion: reduce) {
  .tr-layer.playing .tr-path,
  .tr-layer.playing .tr-head,
  .tr-layer.playing .tr-pop {
    animation-duration: 0.01s;
  }
}
</style>
