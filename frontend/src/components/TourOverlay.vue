<script setup>
import { ref, computed, watch, nextTick, onBeforeUnmount } from 'vue'
import { NIcon } from 'naive-ui'
import { SparklesOutline } from '@vicons/ionicons5'
import { useTourStore } from '@/stores/tour'
import { useTourAnchor, anchorSelector } from '@/composables/useTourAnchor'
import { sidebarDragging } from '@/composables/useSidebarDnd'
import { boardDragging } from '@/composables/useBoardDragScroll'
import { layoutArrow, tourArc, ARROW_HEAD, unionRect, choosePlacement } from '@/utils/tourArrow'

// The Get Started guide's viewport layer (#2753): a dimming mask with a cut-out
// around the element in play, one curved arrow per anchor, and the popover.
//
// Everything here is pointer-events: none except the popover itself — the whole
// point is that the user keeps clicking the real UI (open the dropdown, fill the
// modal) while the mask only *directs* attention. That's why the cut-out is
// purely visual: nothing is actually blocked, so a wrong turn can't trap anyone.

const POP_W = 260
// Distance from the group of anchors to the popover. Kept generous so the arrow
// is a real curved *line* reaching across, not a stub — the author preferred the
// long lined arrows (#2753 rework). Shrunk down this ladder only when nothing
// fits at the preferred distance.
const GAP_LADDER = [92, 60, 32]
const PAD = 6 // mask cut-out padding around the target
const EDGE = 16 // keep the popover this far from the viewport edge

const tour = useTourStore()
const step = computed(() => tour.current)

const pop = ref(null)
const layer = ref(null)
const arrowEls = ref([])
const playing = ref(false)
const popStyle = ref({})
const popSide = ref('right') // which side of the group the popover sits on
const arrows = ref([]) // { len, head } per anchor, in `anchors` order

// `place` is already the name of the popover's layout pass below, hence movedAt.
const {
  rects,
  els,
  count,
  place: movedAt,
  panels,
  surfaces,
} = useTourAnchor(() => tour.anchors, {
  onMissing: () => tour.anchorMissing(step.value?.id),
  countFn: () => step.value?.advanceOn?.count || step.value?.advanceOn?.set || '',
  // resolve() expands the {project}/{board} tokens here too, so a moved-step can
  // track the row the guide itself created rather than the first one in the tree.
  placeFn: () => {
    const m = step.value?.advanceOn?.moved
    return m ? { el: tour.resolve(m.el), within: tour.resolve(m.within), by: m.by } : null
  },
})

// The tour dims itself almost away while a drag is in progress (#2778): the
// overlay never blocked the pointer (`pointer-events: none`), but an arrow drawn
// over the drag ghost, and a mask shading the column the card is heading for,
// both read as "the guide is in the way".
const dragging = computed(() => sidebarDragging.value || boardDragging.value)

const target = computed(() => rects.value[0] || null)

// A surface the user opened *off-script* — a modal/drawer that holds none of the
// step's anchors (the create-workspace modal, the board-settings drawer). The
// tour's dim is switched off entirely while one of these is up, so it looks
// exactly as it would outside the tour. A *guided* modal (the project/task modal
// the step points into) is NOT off-script — it keeps the dim, and the mask's
// holes highlight the row the user has to act on (#2753 rework).
function rectContains(outer, inner, m = 4) {
  return (
    inner.left >= outer.left - m &&
    inner.right <= outer.right + m &&
    inner.top >= outer.top - m &&
    inner.bottom <= outer.bottom + m
  )
}
const offScriptSurface = computed(() =>
  surfaces.value.some((s) => !rects.value.some((a) => a && rectContains(s, a))),
)
const masked = computed(() => step.value?.mask !== false && !offScriptSurface.value)
const isAction = computed(() => step.value?.mode === 'action')
// A picker (calendar, priority/tags/assignee popover) is open. Used to keep the
// popover clear of it and to hold a `set`-step until it's closed (#2753 rework).
const hasPanel = computed(() => panels.value.length > 0)

function readRect(el) {
  const r = el.getBoundingClientRect()
  if (!r.width && !r.height) return null
  return {
    left: r.left,
    top: r.top,
    width: r.width,
    height: r.height,
    right: r.right,
    bottom: r.bottom,
  }
}

// Rects for the step's declared `cut` selectors (a modal's «Создать» button and
// the like): bright in the mask AND kept clear of the popover, so the user is
// never left with a greyed-out or covered control they're meant to press.
function getCutRects() {
  const out = []
  for (const sel of step.value?.cut || []) {
    // resolve() as everywhere else: a cut scoped to the entity the user just
    // created ({group}) has to land on that row, and an unexpanded token would
    // silently punch the hole over the first matching one instead (#2778 rework).
    const el = document.querySelector(anchorSelector(tour.resolve(sel)))
    const r = el && readRect(el)
    if (r) out.push(r)
  }
  return out
}

// Everything the mask punches a bright hole in: the step's anchors, any open
// picker panel (so the guide never dims the control it just pointed at), and the
// step's declared `cut` extras. (Modals/drawers don't need a hole — the mask is
// switched off entirely while one is open.)
const holes = computed(() => {
  const out = []
  for (const r of rects.value) if (r) out.push(r)
  if (masked.value) {
    for (const p of panels.value) out.push(p)
    out.push(...getCutRects())
  }
  return out
})

const clampN = (v, lo, hi) => Math.max(lo, Math.min(hi, v))

// Park the popover beside the whole group of anchors. The side/gap decision is a
// pure function (choosePlacement) so it can be unit-tested against real layouts;
// here we just feed it the live boxes and the things it must not cover.
function placePopover() {
  const anchors = rects.value.filter(Boolean)
  const u = unionRect(anchors)
  if (!u) return
  const p = choosePlacement(u, anchors, [...anchors, ...panels.value, ...getCutRects()], {
    // Measured, not the CSS number (#2807): .tr-pop is content-box, so its 260px
    // width plus padding and border renders 290 — placing it by 260 let the last
    // step of the guide hang 30px past the right edge of the viewport.
    popW: pop.value?.offsetWidth || POP_W,
    popH: pop.value?.offsetHeight || 140,
    vw: window.innerWidth,
    vh: window.innerHeight,
    gaps: GAP_LADDER,
    edge: EDGE,
    panels: panels.value,
  })
  popSide.value = p.side
  popStyle.value = { left: Math.round(p.left) + 'px', top: Math.round(p.top) + 'px' }
}

// Where each arrow leaves the popover: the edge facing the group, at the point
// nearest *that* target — so several arrows fan out cleanly instead of crossing
// from one shared corner.
function arrowOrigin(p, t) {
  const tcx = t.left + t.width / 2
  const tcy = t.top + t.height / 2
  if (popSide.value === 'bottom') return { x: clampN(tcx, p.left + 14, p.right - 14), y: p.top + 2 }
  if (popSide.value === 'top') return { x: clampN(tcx, p.left + 14, p.right - 14), y: p.bottom - 2 }
  if (popSide.value === 'left') return { x: p.right - 2, y: clampN(tcy, p.top + 10, p.bottom - 10) }
  return { x: p.left + 2, y: clampN(tcy, p.top + 10, p.bottom - 10) }
}

// Where it lands: just outside the target's edge that faces the popover, so the
// head never covers the thing it's pointing at.
function arrowTip(t) {
  const tcx = t.left + t.width / 2
  const tcy = t.top + t.height / 2
  if (popSide.value === 'bottom') return { x: tcx, y: t.bottom + 12 }
  if (popSide.value === 'top') return { x: tcx, y: t.top - 12 }
  if (popSide.value === 'left') return { x: t.left - 12, y: tcy }
  return { x: t.right + 12, y: tcy }
}

function place() {
  if (!target.value || !pop.value) return
  placePopover()
  nextTick(() => {
    if (!pop.value) return
    const p = pop.value.getBoundingClientRect()
    arrows.value = rects.value.map((r, i) => {
      const el = arrowEls.value[i]
      if (!r || !el) return { len: 0, head: '' }
      return layoutArrow(el, arrowOrigin(p, r), arrowTip(r), ARROW_HEAD, (f, t) =>
        tourArc(f, t, popSide.value),
      )
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
  // resolve() expands the {project}/{board} tokens the scoped anchors carry.
  const sel = anchorSelector(tour.resolve(key === true ? s.anchor : key))
  if (sel && e.target.closest(sel)) tour.clicked(s.id)
}
document.addEventListener('click', onDocClick, true)
onBeforeUnmount(() => document.removeEventListener('click', onDocClick, true))

// Steps that wait for an entity to be created report the match count on every
// mutation pass; re-reporting on a step change too gives the store its baseline
// even when the new step happens to start on the same number.
//
// A `set` step (fill a field in the task modal) is held while a picker is still
// open: reporting 0 keeps it from advancing the instant a date is picked, so the
// user closes the calendar first, then it moves on (#2753 rework).
watch(
  [count, () => step.value?.id, hasPanel],
  ([n, id, panel]) => {
    if (!id) return
    const gated = !!step.value?.advanceOn?.set && panel
    tour.counted(id, gated ? 0 : n)
  },
  { immediate: true, flush: 'post' },
)

// Drag-and-drop steps report the address of the element they track on every
// mutation pass, same shape as the counter above; re-reporting on a step change
// gives the store its baseline even when the address happens to be unchanged.
watch(
  [movedAt, () => step.value?.id],
  ([p, id]) => {
    if (id) tour.located(id, p)
  },
  { immediate: true, flush: 'post' },
)

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
// A picker/surface opening or closing shifts the free space around the group, so
// re-place when either set changes.
watch(panels, place, { deep: true, flush: 'post' })
watch(surfaces, place, { deep: true, flush: 'post' })
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
    <div v-if="step && target" ref="layer" class="tr-layer" :class="{ playing, dragging }">
      <!-- Dim the page with a bright hole per anchor/panel. An SVG <mask> (black
           holes on a white field) is used instead of an evenodd path so that
           holes for adjacent elements can overlap and still read as one clean
           opening — with evenodd the overlap XORs back to dim (#2753 rework). -->
      <svg v-if="masked" class="tr-mask" aria-hidden="true">
        <defs>
          <mask id="tr-hole" maskUnits="userSpaceOnUse" x="0" y="0" width="100000" height="100000">
            <rect x="0" y="0" width="100000" height="100000" fill="white" />
            <rect
              v-for="(h, i) in holes"
              :key="i"
              :x="h.left - PAD"
              :y="h.top - PAD"
              :width="h.width + PAD * 2"
              :height="h.height + PAD * 2"
              :rx="Math.min(10, (h.width + PAD * 2) / 2, (h.height + PAD * 2) / 2)"
              fill="black"
            />
          </mask>
        </defs>
        <rect class="tr-dim" x="0" y="0" width="100000" height="100000" mask="url(#tr-hole)" />
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
/* Out of the way while the user is actually dragging — see `dragging` above.
   Not display:none: the layer keeps measuring, so the step still ends the moment
   the card lands, and the popover fades back in where it was. */
.tr-layer.dragging {
  opacity: 0.15;
  transition: opacity 0.12s ease;
}
.tr-mask,
.tr-arrows {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
}
.tr-arrows {
  overflow: visible;
}
.tr-mask {
  /* Clip the deliberately-oversized dim rect back to the viewport. */
  overflow: hidden;
}
.tr-dim {
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
