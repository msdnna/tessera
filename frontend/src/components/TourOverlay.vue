<script setup>
import { ref, computed, watch, nextTick, onBeforeUnmount, getCurrentInstance } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import {
  SparklesOutline,
  CheckmarkOutline,
  CheckmarkCircleOutline,
  EyeOffOutline,
  PauseOutline,
} from '@vicons/ionicons5'
import ShortcutKeys from './ShortcutKeys.vue'
import BottomSheet from './kit/BottomSheet.vue'
import { useTourStore } from '@/stores/tour'
import { useTourAnchor, anchorSelector } from '@/composables/useTourAnchor'
import { useResponsive } from '@/composables/useResponsive'
import { useShortcuts } from '@/composables/useShortcuts'
import { useSidebarSize } from '@/composables/useSidebarSize'
import { sidebarDragging } from '@/composables/useSidebarDnd'
import { boardDragging } from '@/composables/useBoardDragScroll'
import { layoutArrow, tourArc, ARROW_HEAD, unionRect, choosePlacement } from '@/utils/tourArrow'

// The tour's viewport layer (#2753, v2 #3000 — mockups TourPop / TourLayer): a
// dim with a cutout around the element in play, one curved arrow per anchor,
// and the step's popover. On a phone the popover is a bottom sheet and there
// are no arrows — the cutout says where.
//
// Everything here is pointer-events: none except the popover itself — the user
// keeps clicking the real UI (open the menu, fill the form) while the dim only
// *directs* attention. Nothing is actually blocked, so a wrong turn can't trap
// anyone.
//
// States of the popover (TourPop): an info step («Понятно» ↵, «Назад» ←), an
// action step (a plate saying what will count it), «Засчитано» for a moment
// after an action, «Ждём…» while the anchor hasn't shown up, «Не видим …» with
// a 5-second countdown once it hasn't for 8 s (no more silent skipping), the
// «Прервать обучение?» question in place (Esc), and the chapter's final step.

const POP_W = 320
const POP_W_LONG = 340
const POP_W_CONFIRM = 360
// Distance from the group of anchors to the popover, shrunk down the ladder
// only when nothing fits — the arrow stays a real curved line, not a stub.
const GAP_LADDER = [72, 44, 16]
const PAD = 6 // dim cutout padding around the target
const EDGE = 12 // keep the popover this far from the viewport edge
const LOADING_MS = 1200
const MISSING_MS = 8000
const COUNTDOWN = 5
const DRAW_MS = 420 // arrow drawn (320 ms after an 80 ms delay) + a beat

const { t, te } = useI18n()
const tour = useTourStore()
const { isMobile } = useResponsive()
const { collapsed, toggle: toggleSidebar } = useSidebarSize()
const router = getCurrentInstance()?.proxy?.$router
const step = computed(() => tour.current)

// The guide runs on a board seeded moments ago, so the columns it names are the
// four default ones — captioned from the reader's own catalogue (#2800), handed
// to every step as params (#2932).
const columnCaptions = computed(() => ({
  todo: t('board.column.defaultName.todo'),
  inProgress: t('board.column.defaultName.in_progress'),
  review: t('board.column.defaultName.review'),
  done: t('board.column.defaultName.done'),
}))
// Wording is looked up by id on every render rather than carried on the step:
// the store snapshots the scenario, so text stored there would keep the
// language the tour started in (#2799).
const stepText = (part) => {
  if (!step.value) return ''
  const key = `tour.steps.${step.value.id}.${part}`
  return part === 'wait' && !te(key) ? '' : t(key, columnCaptions.value)
}

const chapterName = computed(() =>
  tour.chapter ? t(`tour.chapters.${tour.chapter}.title`) : t('tour.aria'),
)
const chapLine = computed(() =>
  tour.total > 1
    ? t('tour.position', { chapter: chapterName.value, n: tour.number, of: tour.total })
    : chapterName.value,
)

const pop = ref(null)
const layer = ref(null)
const primaryBtn = ref(null)
const arrowEls = ref([])
const playing = ref(false)
const drawing = ref(false)
const popStyle = ref({})
const popSide = ref('right')
const arrows = ref([]) // { len, head, from, to } per anchor

const {
  rects,
  els,
  count,
  place: movedAt,
  panels,
  surfaces,
} = useTourAnchor(() => tour.anchors, {
  // The overlay counts a missing anchor down itself (below), visibly.
  timeout: 0,
  countFn: () => {
    const on = step.value?.advanceOn || {}
    return tour.resolve(on.count || on.set || on.appear || on.vanish || '')
  },
  placeFn: () => {
    const m = step.value?.advanceOn?.moved
    return m ? { el: tour.resolve(m.el), within: tour.resolve(m.within), by: m.by } : null
  },
})

// Out of the way while a drag is in progress (#2778): the layer never blocked
// the pointer, but an arrow over the drag ghost reads as "in the way".
const dragging = computed(() => sidebarDragging.value || boardDragging.value)

const target = computed(() => rects.value[0] || null)
const isAction = computed(() => step.value?.mode === 'action')
const isFinal = computed(() => !!step.value?.final && !isAction.value)
const flashing = computed(() => !!step.value && tour.flash === step.value.id)

// ── the anchor that isn't there ──
// 0–1.2 s nothing (most anchors land within a frame or two), then «Ждём…» at
// the edge of the screen, then at 8 s «Не видим …» counting 5 s down to the
// next step. Paused while «Прервать?» is asked.
const phase = ref('ok') // ok | loading | missing
const countdown = ref(COUNTDOWN)
let tLoad = 0
let tMiss = 0
let tTick = 0
function clearTimers() {
  clearTimeout(tLoad)
  clearTimeout(tMiss)
  clearInterval(tTick)
  tLoad = tMiss = tTick = 0
}
watch(
  () => [step.value?.id, !!target.value, tour.confirming, flashing.value],
  ([id, has, confirming, flash]) => {
    clearTimers()
    if (!id || has || flash) {
      phase.value = 'ok'
      return
    }
    if (confirming) return
    phase.value = 'ok'
    tLoad = setTimeout(() => (phase.value = 'loading'), LOADING_MS)
    tMiss = setTimeout(() => {
      phase.value = 'missing'
      countdown.value = COUNTDOWN
      tTick = setInterval(() => {
        countdown.value -= 1
        if (countdown.value <= 0) {
          clearTimers()
          tour.anchorMissing(id)
        }
      }, 1000)
    }, MISSING_MS)
  },
  { immediate: true },
)
onBeforeUnmount(clearTimers)

function skipStep() {
  clearTimers()
  if (step.value) tour.anchorMissing(step.value.id)
}

// The popover is on screen: at its anchor, or at the edge while waiting.
const shown = computed(
  () =>
    !!step.value && (!!target.value || flashing.value || phase.value !== 'ok' || tour.confirming),
)
const docked = computed(() => !target.value) // no anchor: parked at the edge

// The popover's look (TourPop `kind`).
const kind = computed(() => {
  if (tour.confirming) return 'confirm'
  if (flashing.value) return 'done'
  if (phase.value === 'missing') return 'missing'
  if (phase.value === 'loading') return 'loading'
  if (isFinal.value) return 'final'
  return isAction.value ? 'action' : 'info'
})
const title = computed(() => {
  if (kind.value === 'confirm') return t('tour.confirm.title')
  if (kind.value === 'missing') return t('tour.missing.title')
  return stepText('title')
})
const body = computed(() => {
  if (kind.value === 'confirm') return t('tour.confirm.body')
  if (kind.value === 'missing') return t('tour.missing.body')
  return stepText('body')
})
const waitText = computed(() => stepText('wait') || t('tour.waitDefault'))
const popW = computed(() => {
  if (kind.value === 'confirm') return POP_W_CONFIRM
  return (body.value || '').length > 150 ? POP_W_LONG : POP_W
})
const segsOn = computed(() => Math.min(tour.number, tour.total))
const hasPrimary = computed(() => ['info', 'final', 'confirm'].includes(kind.value))
const primaryLabel = computed(() => {
  if (kind.value === 'confirm') return t('tour.confirm.later')
  if (kind.value === 'final') return t('tour.done')
  return t('tour.gotIt')
})

// The dim: off for an off-script surface — a modal/drawer that holds none of
// the step's anchors (the create-workspace dialog, the board drawer) — so it
// looks exactly as it would outside the tour. A *guided* surface (the project
// form, the task window) keeps it, with a bright hole over the field in play.
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
const masked = computed(
  () => !!target.value && step.value?.mask !== false && !offScriptSurface.value,
)
// A picker is open (calendar, tags…): kept clear of the popover, and a
// `set`-step holds until it closes (#2753 rework).
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
// The step's declared `cut` extras (a form's submit, the target column): bright
// in the dim AND kept clear of the popover.
function getCutRects() {
  const out = []
  for (const sel of step.value?.cut || []) {
    const el = document.querySelector(anchorSelector(tour.resolve(sel)))
    const r = el && readRect(el)
    if (r) out.push(r)
  }
  return out
}
const holes = computed(() => {
  const out = []
  for (const r of rects.value) if (r) out.push(r)
  if (masked.value) {
    for (const p of panels.value) out.push(p)
    out.push(...getCutRects())
  }
  return out
})
const holeBox = (h) => ({
  x: h.left - PAD,
  y: h.top - PAD,
  width: h.width + PAD * 2,
  height: h.height + PAD * 2,
  rx: Math.min(10, (h.width + PAD * 2) / 2, (h.height + PAD * 2) / 2),
})

const clampN = (v, lo, hi) => Math.max(lo, Math.min(hi, v))

function placePopover() {
  if (isMobile.value) return
  const anchors = rects.value.filter(Boolean)
  const u = unionRect(anchors)
  const w = pop.value?.offsetWidth || popW.value
  const h = pop.value?.offsetHeight || 180
  if (!u) {
    // «Засчитано» stays where the step was, though its anchor is gone (the
    // window it asked to close, the form it asked to submit).
    if (flashing.value && popStyle.value.left) return
    // Parked at the bottom-right edge while the anchor is awaited.
    popStyle.value = {
      left: Math.round(window.innerWidth - w - 24) + 'px',
      top: Math.round(window.innerHeight - h - 24) + 'px',
    }
    return
  }
  const p = choosePlacement(u, anchors, [...anchors, ...panels.value, ...getCutRects()], {
    popW: w,
    popH: h,
    vw: window.innerWidth,
    vh: window.innerHeight,
    gaps: GAP_LADDER,
    edge: EDGE,
    panels: panels.value,
    prefer: step.value?.sidebar ? 'right' : null,
  })
  popSide.value = p.side
  popStyle.value = { left: Math.round(p.left) + 'px', top: Math.round(p.top) + 'px' }
}

// Where each arrow leaves the popover: the edge facing the group, at the point
// nearest *that* target — several arrows fan out instead of crossing.
function arrowOrigin(p, tr) {
  const tcx = tr.left + tr.width / 2
  const tcy = tr.top + tr.height / 2
  if (popSide.value === 'bottom') return { x: clampN(tcx, p.left + 16, p.right - 16), y: p.top + 2 }
  if (popSide.value === 'top') return { x: clampN(tcx, p.left + 16, p.right - 16), y: p.bottom - 2 }
  if (popSide.value === 'left') return { x: p.right - 2, y: clampN(tcy, p.top + 14, p.bottom - 14) }
  return { x: p.left + 2, y: clampN(tcy, p.top + 14, p.bottom - 14) }
}
// Where it lands: just outside the target's edge facing the popover.
function arrowTip(tr) {
  const tcx = tr.left + tr.width / 2
  const tcy = tr.top + tr.height / 2
  if (popSide.value === 'bottom') return { x: tcx, y: tr.bottom + 10 }
  if (popSide.value === 'top') return { x: tcx, y: tr.top - 10 }
  if (popSide.value === 'left') return { x: tr.left - 10, y: tcy }
  return { x: tr.right + 10, y: tcy }
}

function place() {
  if (!shown.value || !pop.value) return
  placePopover()
  nextTick(() => {
    if (!pop.value || isMobile.value) return
    const p = pop.value.getBoundingClientRect()
    arrows.value = rects.value.map((r, i) => {
      const el = arrowEls.value[i]
      if (!r || !el || flashing.value) return { len: 0, head: '' }
      const from = arrowOrigin(p, r)
      const to = arrowTip(r)
      return {
        ...layoutArrow(el, from, to, ARROW_HEAD, (f, tt) => tourArc(f, tt, popSide.value)),
        from,
        to,
      }
    })
  })
}

let drawTimer = 0
function replay() {
  playing.value = false
  void layer.value?.offsetWidth // reflow, so the draw animation restarts
  playing.value = true
}
// «Понятно» waits for the arrows to land (PackageH «Затемнение и стрелки») —
// only when there is an arrow being drawn at all.
function holdWhileDrawing() {
  clearTimeout(drawTimer)
  drawing.value = arrows.value.some((a) => a.len)
  if (drawing.value) drawTimer = setTimeout(() => (drawing.value = false), DRAW_MS)
}
onBeforeUnmount(() => clearTimeout(drawTimer))

// Action steps that advance on a click on the real element. Capture phase: the
// element is often replaced (a menu re-renders) between arming and the click.
function onDocClick(e) {
  const s = step.value
  const key = s?.advanceOn?.click
  if (!key || !(e.target instanceof Element)) return
  const sel = anchorSelector(tour.resolve(key === true ? s.anchor : key))
  if (sel && e.target.closest(sel)) tour.clicked(s.id)
}
document.addEventListener('click', onDocClick, true)
onBeforeUnmount(() => document.removeEventListener('click', onDocClick, true))

// Steps waiting on an entity report the match count on every mutation pass. A
// `set` step is held while a picker is open (reporting 0), so the user closes
// the calendar first and then the step moves on (#2753 rework).
watch(
  [count, () => step.value?.id, hasPanel],
  ([n, id, panel]) => {
    if (!id) return
    const gated = !!step.value?.advanceOn?.set && panel
    tour.counted(id, gated ? 0 : n)
  },
  { immediate: true, flush: 'post' },
)
watch(
  [movedAt, () => step.value?.id],
  ([p, id]) => {
    if (id) tour.located(id, p)
  },
  { immediate: true, flush: 'post' },
)

// A step whose purpose is already met (a task window already open for «Откройте
// задачу») is passed over without a word.
watch(
  () => step.value?.id,
  async (id) => {
    const sel = step.value?.skipIf
    if (!id || !sel) return
    await nextTick()
    if (document.querySelector(anchorSelector(tour.resolve(sel)))) tour.anchorMissing(id)
  },
  { immediate: true, flush: 'post' },
)

// An anchor in the DOM but scrolled out of sight (a card low in a long column,
// a row under the fold) is brought into view once per step.
let scrolledFor = null
watch(
  () => [step.value?.id, els.value[0], target.value],
  ([id, el, box]) => {
    if (!id || !el || box || scrolledFor === id) return
    scrolledFor = id
    el.scrollIntoView?.({ block: 'center', inline: 'nearest', behavior: 'smooth' })
  },
  { flush: 'post' },
)

// Hover-only anchors (the «+» of a tree row, «Создать задачу» at the foot of a
// column) are held visible while the tour points at them — main.css.
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
  { immediate: true, flush: 'post' },
)
onBeforeUnmount(unmark)

// A collapsed sidebar hides the tree: the tour opens it for steps that point
// into it and folds it back when the tour is over (PackageH «Якорь не найден»).
let openedSidebar = false
watch(
  () => step.value?.sidebar,
  (needs) => {
    if (needs && collapsed.value && !isMobile.value) {
      toggleSidebar()
      openedSidebar = true
    }
  },
  { immediate: true },
)
watch(
  () => tour.active,
  (on) => {
    if (on || !openedSidebar) return
    openedSidebar = false
    if (!collapsed.value) toggleSidebar()
  },
)

// Focus: on an info step — «Понятно» (↵ works anyway); on an action step —
// the field the user is asked to fill, if the anchor is one.
function focusFor() {
  if (!shown.value || isMobile.value) return
  if (hasPrimary.value) {
    primaryBtn.value?.focus?.({ preventScroll: true })
    return
  }
  const el = els.value[0]
  if (el && el.matches?.('input, textarea')) el.focus({ preventScroll: true })
  else el?.querySelector?.('input, textarea')?.focus({ preventScroll: true })
}

// Post-flush throughout: the popover is measured from its real box.
watch(rects, place, { deep: true, flush: 'post' })
watch(panels, place, { deep: true, flush: 'post' })
watch(surfaces, place, { deep: true, flush: 'post' })
watch([kind, shown], place, { flush: 'post' })
watch(
  () => (shown.value && step.value ? `${step.value.id}:${!!target.value}` : null),
  async (key) => {
    if (!key) return
    arrows.value = []
    await nextTick()
    place()
    replay()
    focusFor()
    await nextTick()
    holdWhileDrawing()
  },
  { immediate: true, flush: 'post' },
)
watch(
  () => tour.confirming,
  async (on) => {
    await nextTick()
    if (on) primaryBtn.value?.focus?.({ preventScroll: true })
  },
)

// ── buttons and keys ──
function onPrimary() {
  if (kind.value === 'confirm') return tour.later()
  if (drawing.value) return
  tour.next()
}
function toChecklist() {
  tour.next()
  router?.push?.('/')
}
const keyLive = () => tour.active && shown.value
useShortcuts({ 'tour.next': () => onPrimary() }, { when: () => keyLive() && hasPrimary.value })
useShortcuts(
  { 'tour.back': () => tour.back() },
  { when: () => keyLive() && tour.canBack && !tour.confirming && hasPrimary.value },
)
useShortcuts(
  { 'tour.abort': () => (tour.confirming ? tour.resume() : tour.abort()) },
  { when: keyLive },
)

// Phone: the sheet stands on the bottom navigation, so the tabs it talks about
// stay in sight; an anchor in the lower half that isn't a tab (the «+» above
// the navigation, a card) would be covered — the sheet goes to the top then.
const inTabbar = computed(() => !!els.value[0]?.closest?.('[data-testid="mobile-tabbar"]'))
const sheetTop = computed(() => {
  const r = target.value
  if (!r || inTabbar.value) return false
  return r.bottom > window.innerHeight * 0.5
})

// The sheet's own Back / swipe down means the same as Esc.
function onSheetClose() {
  if (tour.confirming) tour.resume()
  else tour.abort()
}

const tileIcon = computed(() => {
  if (kind.value === 'done' || kind.value === 'final') return CheckmarkOutline
  if (kind.value === 'missing') return EyeOffOutline
  if (kind.value === 'confirm') return PauseOutline
  return SparklesOutline
})
</script>

<template>
  <teleport to="body">
    <div
      v-if="shown"
      ref="layer"
      class="tr-layer"
      :class="{ playing, dragging, mobile: isMobile }"
      data-testid="tour-layer"
    >
      <!-- The dim with a bright hole per anchor/panel/cut. An SVG <mask> (black
           holes on a white field) rather than evenodd, so holes of adjacent
           elements overlap into one clean opening (#2753 rework). The ring is
           the cutout's focus ring (PackageH «Затемнение»). -->
      <svg v-if="masked" class="tr-mask" aria-hidden="true">
        <defs>
          <mask id="tr-hole" maskUnits="userSpaceOnUse" x="0" y="0" width="100000" height="100000">
            <rect x="0" y="0" width="100000" height="100000" fill="white" />
            <rect v-for="(h, i) in holes" :key="i" v-bind="holeBox(h)" fill="black" />
          </mask>
        </defs>
        <rect class="tr-dim" x="0" y="0" width="100000" height="100000" mask="url(#tr-hole)" />
        <rect v-for="(h, i) in holes" :key="'r' + i" class="tr-ring" v-bind="holeBox(h)" />
      </svg>
      <svg v-if="!isMobile" class="tr-arrows" aria-hidden="true">
        <defs>
          <linearGradient
            v-for="(a, i) in arrows"
            :id="`tr-g-${i}`"
            :key="i"
            gradientUnits="userSpaceOnUse"
            :x1="a.from?.x || 0"
            :y1="a.from?.y || 0"
            :x2="a.to?.x || 1"
            :y2="a.to?.y || 1"
          >
            <stop offset="0" class="tr-stop-a" />
            <stop offset="1" class="tr-stop-b" />
          </linearGradient>
        </defs>
        <g v-for="(a, i) in rects" :key="i">
          <path
            :ref="(el) => (arrowEls[i] = el)"
            class="tr-path"
            :style="{ '--len': arrows[i]?.len || 0, '--gap': ARROW_HEAD }"
            fill="none"
            :stroke="`url(#tr-g-${i})`"
          />
          <polygon class="tr-head" :points="arrows[i]?.head || ''" />
        </g>
      </svg>

      <!-- Desktop: the popover (TourPop). -->
      <div
        v-if="!isMobile"
        ref="pop"
        class="tp"
        :class="[`k-${kind}`, { docked }]"
        :style="{ ...popStyle, width: popW + 'px' }"
        role="dialog"
        :aria-label="t('tour.aria')"
        data-testid="tour-pop"
        :data-kind="kind"
      >
        <div class="tp-head">
          <span class="tp-tile" :class="`t-${kind}`">
            <n-icon :component="tileIcon" :size="15" />
          </span>
          <span class="tp-ht">
            <span class="tp-title">{{ title }}</span>
            <span class="tp-chap">{{ chapLine }}</span>
          </span>
          <span v-if="kind === 'confirm'" class="tp-tostep">
            {{ t('tour.confirm.toStep') }} <ShortcutKeys id="tour.abort" />
          </span>
        </div>
        <div v-if="body" class="tp-text">{{ body }}</div>
        <div v-if="kind === 'action'" class="tp-plate wait" data-testid="tour-wait">
          <span class="tp-dot" /><span class="tp-pl">{{ waitText }}</span>
        </div>
        <div v-else-if="kind === 'done'" class="tp-plate ok" data-testid="tour-done">
          <n-icon :component="CheckmarkCircleOutline" :size="15" />
          <span class="tp-pl">{{ t('tour.counted') }}</span>
        </div>
        <div v-else-if="kind === 'missing'" class="tp-plate miss" data-testid="tour-missing">
          <span class="tp-pl">{{ t('tour.missing.auto') }}</span>
          <span class="tp-count">{{ t('tour.missing.seconds', { n: countdown }) }}</span>
        </div>
        <div v-else-if="kind === 'loading'" class="tp-plate load" data-testid="tour-loading">
          <span class="tp-spin" /><span class="tp-pl">{{ t('tour.loading') }}</span>
        </div>
        <div v-if="tour.total > 1 && kind !== 'confirm'" class="tp-segs" aria-hidden="true">
          <span v-for="i in tour.total" :key="i" class="tp-seg" :class="{ on: i <= segsOn }" />
        </div>
        <div class="tp-actions">
          <button
            v-if="kind !== 'confirm' && kind !== 'final'"
            type="button"
            class="tp-abort"
            data-testid="tour-skip"
            @click="tour.abort()"
          >
            {{ t('tour.abort') }} <ShortcutKeys id="tour.abort" />
          </button>
          <button
            v-if="kind === 'confirm'"
            type="button"
            class="tp-ctl"
            data-testid="tour-finish"
            @click="tour.skip()"
          >
            {{ t('tour.confirm.finish') }}
          </button>
          <span class="tp-fill" />
          <button
            v-if="tour.canBack && kind === 'info'"
            type="button"
            class="tp-back"
            data-testid="tour-back"
            @click="tour.back()"
          >
            {{ t('tour.back') }} <ShortcutKeys id="tour.back" />
          </button>
          <button
            v-if="kind === 'missing'"
            type="button"
            class="tp-ctl"
            data-testid="tour-skip-step"
            @click="skipStep"
          >
            {{ t('tour.missing.skip') }}
          </button>
          <button
            v-if="kind === 'final'"
            type="button"
            class="tp-ctl"
            data-testid="tour-checklist"
            @click="toChecklist"
          >
            {{ t('tour.toChecklist') }}
          </button>
          <button
            v-if="hasPrimary"
            ref="primaryBtn"
            type="button"
            class="tp-pri"
            :class="{ off: drawing && kind !== 'confirm' }"
            :aria-disabled="drawing && kind !== 'confirm' ? 'true' : null"
            :data-testid="kind === 'confirm' ? 'tour-later' : 'tour-next'"
            @click="onPrimary"
          >
            {{ primaryLabel }} <ShortcutKeys id="tour.next" />
          </button>
        </div>
      </div>
    </div>
  </teleport>

  <!-- Phone: the same step as a bottom sheet over the live screen (#3000,
       mockup TourLayer «mt»). It docks over the bottom navigation, so the tabs it
       talks about stay in sight; an anchor the sheet would cover (the «+»
       above the navigation) sends it to the top of the screen. -->
  <BottomSheet
    v-if="isMobile"
    :show="shown"
    bare
    :lift="!sheetTop"
    :top="sheetTop"
    :z="7100"
    testid="tour-sheet"
    @close="onSheetClose"
  >
    <div class="ts" :data-kind="kind" data-testid="tour-pop">
      <div class="ts-head">
        <span class="tp-tile big" :class="`t-${kind}`">
          <n-icon :component="tileIcon" :size="17" />
        </span>
        <span class="tp-ht">
          <span class="ts-title">{{ title }}</span>
          <span class="ts-chap">{{ chapLine }}</span>
        </span>
      </div>
      <div v-if="body" class="ts-text">{{ body }}</div>
      <div v-if="kind === 'missing'" class="tp-plate miss">
        <span class="tp-pl">{{ t('tour.missing.auto') }}</span>
        <span class="tp-count">{{ t('tour.missing.seconds', { n: countdown }) }}</span>
      </div>
      <div v-if="tour.total > 1 && kind !== 'confirm'" class="ts-segs" aria-hidden="true">
        <span v-for="i in tour.total" :key="i" class="tp-seg" :class="{ on: i <= segsOn }" />
      </div>
    </div>
    <template #actions>
      <button
        v-if="kind !== 'confirm'"
        type="button"
        class="ts-btn ghost"
        data-testid="tour-skip"
        @click="tour.abort()"
      >
        {{ t('tour.abort') }}
      </button>
      <button
        v-else
        type="button"
        class="ts-btn ctl wide"
        data-testid="tour-finish"
        @click="tour.skip()"
      >
        {{ t('tour.confirm.finish') }}
      </button>
      <span v-if="kind !== 'confirm'" class="tp-fill" />
      <button
        v-if="tour.canBack && kind === 'info'"
        type="button"
        class="ts-btn ctl"
        data-testid="tour-back"
        @click="tour.back()"
      >
        {{ t('tour.back') }}
      </button>
      <button
        v-if="kind === 'missing'"
        type="button"
        class="ts-btn ctl"
        data-testid="tour-skip-step"
        @click="skipStep"
      >
        {{ t('tour.missing.skip') }}
      </button>
      <button
        v-if="hasPrimary"
        type="button"
        class="ts-btn pri"
        :class="{ wide: kind === 'confirm' }"
        :data-testid="kind === 'confirm' ? 'tour-later' : 'tour-next'"
        @click="onPrimary"
      >
        {{ primaryLabel }}
      </button>
    </template>
  </BottomSheet>
</template>

<style scoped>
.tr-layer {
  position: fixed;
  inset: 0;
  z-index: 7000; /* above content, below the app loader/connection overlay */
  pointer-events: none;
}
/* Out of the way while the user is actually dragging — see `dragging`. Not
   display:none: the layer keeps measuring, so the step ends the moment the
   card lands, and the popover fades back in where it was. */
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
  overflow: hidden;
}
.tr-dim {
  fill: var(--t-scrim);
}
.tr-ring {
  fill: none;
  stroke: color-mix(in srgb, var(--t-primary) 55%, transparent);
  stroke-width: 1.5;
}
.tr-stop-a {
  stop-color: color-mix(in srgb, var(--t-primary) 80%, #000);
}
.tr-stop-b {
  stop-color: color-mix(in srgb, var(--t-primary) 72%, #fff);
}
.tr-path {
  stroke-width: 2.25;
  stroke-linecap: round;
  stroke-dasharray: var(--len);
  stroke-dashoffset: var(--len);
}
.tr-head {
  fill: color-mix(in srgb, var(--t-primary) 72%, #fff);
  opacity: 0;
}
.tr-layer.playing .tr-path {
  animation: tr-draw 0.32s cubic-bezier(0.65, 0, 0.35, 1) 0.08s forwards;
}
.tr-layer.playing .tr-head {
  animation: tr-fade 0.12s ease 0.36s forwards;
}
/* Stop the stroke --gap px short of the tip so the head's point stays sharp. */
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

/* ── the popover (TourPop): 320–340, radius 16, --t-pop, the pop shadow plus
   the accent's glow as a shadow ── */
.tp {
  position: absolute;
  box-sizing: border-box;
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 14px 16px 12px;
  border-radius: var(--t-r-pop, 16px);
  background: var(--t-pop);
  box-shadow:
    var(--t-sh-pop),
    0 10px 40px color-mix(in srgb, var(--t-primary) 26%, transparent);
  color: var(--t-text1);
  pointer-events: auto;
  opacity: 0;
  transform: translateY(6px);
}
.tr-layer.playing .tp {
  animation: tp-in 0.16s ease-out forwards;
}
@keyframes tp-in {
  to {
    opacity: 1;
    transform: none;
  }
}
.tp-head {
  display: flex;
  align-items: flex-start;
  gap: 10px;
}
.tp-tile {
  flex: 0 0 28px;
  width: 28px;
  height: 28px;
  border-radius: 9px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--t-accent-grad);
  color: #fff;
  box-shadow: 0 2px 8px color-mix(in srgb, var(--t-primary) 35%, transparent);
}
.tp-tile.big {
  flex-basis: 32px;
  width: 32px;
  height: 32px;
  border-radius: 10px;
}
.tp-tile.t-done,
.tp-tile.t-final {
  background: linear-gradient(
    to top right,
    color-mix(in srgb, #18a058 86%, #000),
    #18a058 50%,
    color-mix(in srgb, #18a058 86%, #fff)
  );
  box-shadow: none;
}
.tp-tile.t-missing {
  background: linear-gradient(
    to top right,
    color-mix(in srgb, #f0a020 86%, #000),
    #f0a020 50%,
    color-mix(in srgb, #f0a020 86%, #fff)
  );
  box-shadow: none;
}
.tp-tile.t-confirm {
  background: var(--t-pill);
  color: var(--t-text2);
  box-shadow: none;
}
.tp-ht {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.tp-title {
  font-size: 15px;
  font-weight: 650;
  line-height: 1.3;
  text-wrap: pretty;
}
.tp-chap {
  font-size: 12px;
  color: var(--t-text3);
  font-variant-numeric: tabular-nums;
}
.tp-tostep {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  font-size: 12px;
  color: var(--t-text3);
  white-space: nowrap;
}
.tp-text {
  font-size: 13.5px;
  line-height: 1.5;
  color: var(--t-text2);
  text-wrap: pretty;
}
.tp-plate {
  display: flex;
  align-items: center;
  gap: 9px;
  padding: 8px 10px;
  border-radius: var(--t-r-ctl, 10px);
  font-size: 12.5px;
  line-height: 1.4;
  color: var(--t-text1);
}
.tp-plate.wait {
  background: linear-gradient(
    to top right,
    color-mix(in srgb, var(--t-primary) 9%, var(--t-pop)),
    color-mix(in srgb, var(--t-primary) 17%, var(--t-pop))
  );
}
.tp-plate.ok {
  gap: 8px;
  font-weight: 600;
  color: #18a058;
  background: linear-gradient(
    to top right,
    color-mix(in srgb, #18a058 11%, var(--t-pop)),
    color-mix(in srgb, #18a058 19%, var(--t-pop))
  );
}
.tp-plate.miss {
  background: linear-gradient(
    to top right,
    color-mix(in srgb, #f0a020 10%, var(--t-pop)),
    color-mix(in srgb, #f0a020 18%, var(--t-pop))
  );
}
.tp-plate.load {
  background: var(--t-sub);
  color: var(--t-text2);
}
.tp-pl {
  flex: 1;
  min-width: 0;
}
.tp-dot {
  flex: 0 0 8px;
  width: 8px;
  height: 8px;
  border-radius: 4px;
  background: var(--t-accent-grad);
  box-shadow: 0 0 0 4px color-mix(in srgb, var(--t-primary) 22%, transparent);
}
.tp-count {
  font-size: 12px;
  font-weight: 600;
  color: #f0a020;
  font-variant-numeric: tabular-nums;
}
.tp-spin {
  flex: 0 0 12px;
  width: 12px;
  height: 12px;
  box-sizing: border-box;
  border-radius: 50%;
  border: 2px solid var(--t-pill);
  border-top-color: var(--t-primary);
  animation: tp-spin 0.8s linear infinite;
}
@keyframes tp-spin {
  to {
    transform: rotate(360deg);
  }
}
.tp-segs,
.ts-segs {
  display: flex;
  gap: 3px;
}
.ts-segs {
  gap: 4px;
}
.tp-seg {
  flex: 1;
  height: 3px;
  border-radius: 2px;
  background: color-mix(in srgb, var(--t-text3) 28%, transparent);
}
.ts-segs .tp-seg {
  height: 4px;
}
.tp-seg.on {
  background: var(--t-accent-grad);
}
.tp-actions {
  display: flex;
  align-items: center;
  gap: 6px;
}
.tp-fill {
  flex: 1;
}
.tp-actions button {
  font: inherit;
  border: none;
  cursor: pointer;
  white-space: nowrap;
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.tp-abort {
  height: 28px;
  padding: 0 6px 0 8px;
  margin-left: -8px;
  border-radius: 8px;
  background: transparent;
  font-size: 12.5px;
  color: var(--t-text3);
}
.tp-abort:hover {
  color: var(--t-text1);
}
.tp-ctl {
  height: 30px;
  padding: 0 10px;
  border-radius: 9px;
  background: var(--t-ctl);
  font-size: 13px;
  color: var(--t-text1);
}
.tp-ctl:hover {
  background: color-mix(in srgb, var(--t-text1) 12%, var(--t-ctl));
}
.tp-back {
  height: 30px;
  padding: 0 5px 0 9px;
  border-radius: 9px;
  background: transparent;
  font-size: 13px;
  color: var(--t-text2);
}
.tp-back:hover {
  background: color-mix(in srgb, var(--t-text1) 9%, transparent);
  color: var(--t-text1);
}
.tp-pri {
  height: 30px;
  padding: 0 5px 0 12px;
  border-radius: 9px;
  background: var(--t-accent-grad);
  color: #fff;
  font-size: 13px;
  font-weight: 600;
  box-shadow: 0 2px 8px color-mix(in srgb, var(--t-primary) 30%, transparent);
  transition: opacity 0.12s ease;
}
.tp-pri.off {
  opacity: 0.42;
  cursor: default;
}
.tp-pri :deep(.kbd) {
  background: rgba(255, 255, 255, 0.22);
  color: #fff;
  box-shadow: none;
}
.tp-actions button:focus {
  outline: none;
}
.tp-actions button:focus-visible {
  box-shadow:
    0 0 0 2px var(--t-pop),
    0 0 0 4px color-mix(in srgb, var(--t-primary) 75%, transparent);
}

/* ── the phone's sheet (TourLayer «mt»): radius 24 on top, 48px buttons ── */
.ts {
  display: flex;
  flex-direction: column;
  gap: 12px;
  margin-top: 4px;
}
.ts-head {
  display: flex;
  align-items: center;
  gap: 12px;
}
.ts-title {
  font-size: 17px;
  font-weight: 650;
  color: var(--t-text1);
}
.ts-chap {
  font-size: 13px;
  color: var(--t-text3);
}
.ts-text {
  font-size: 15px;
  line-height: 1.5;
  color: var(--t-text2);
  text-wrap: pretty;
}
.ts-btn {
  height: 48px;
  padding: 0 18px;
  border: none;
  border-radius: 14px;
  font: inherit;
  font-size: 15px;
  cursor: pointer;
  white-space: nowrap;
}
.ts-btn.ghost {
  padding: 0 14px;
  background: transparent;
  color: var(--t-text3);
}
.ts-btn.ctl {
  background: var(--t-ctl);
  color: var(--t-text1);
}
.ts-btn.wide,
.ts-btn.pri.wide {
  flex: 1 1 0;
  min-width: 0;
  padding: 0 10px;
  font-size: 14px;
  white-space: normal;
  line-height: 1.2;
}
.ts-btn.pri {
  padding: 0 22px;
  background: var(--t-accent-grad);
  color: #fff;
  font-weight: 600;
}

@media (prefers-reduced-motion: reduce) {
  .tr-layer.playing .tr-path,
  .tr-layer.playing .tr-head,
  .tr-layer.playing .tp,
  .tp-spin {
    animation-duration: 0.01s;
  }
}
</style>
