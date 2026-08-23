import { ref, computed, nextTick, watch, onBeforeUnmount } from 'vue'
import {
  DAY_MS,
  startOfDay,
  parseDate,
  spanOf,
  tierFor,
  xAt,
  buildDays,
  buildMonthBands,
  buildWeekBands,
  hourTicksInWindow,
  hourStepFor,
} from '@/utils/timeAxis'
import { estimateToDays } from '@/utils/estimation'
import { useFormat } from '@/composables/useFormat'

// The horizontal time axis shared by the Timeline and Gantt board views: zoom,
// axis range, header bands, the today line, milestone markers, and everything
// that reads or moves the scroll viewport (scroll-to-today, pan, ctrl+wheel zoom,
// the hover cursor guide).
//
// Both views render the same chart and differed only in what they draw ON it
// (Timeline: bar layout; Gantt: dependency arrows). Keeping the axis in one place
// is what stops the two from drifting — `utils/timeAxis` already holds the pure
// geometry, this holds the stateful half.
//
// Everything below is expressed in three coordinate systems, so read carefully:
//   * axis/content px — 0 at the range start, what bars and gridlines use;
//   * container px    — scroll container, offset from axis px by leftW + scrollLeft;
//   * viewport px     — clientX/clientY, used for the fixed-position date pill.

// px per day. The range spans week-grouping (zoomed out) through hour-precision
// (zoomed in); stops land inside each tier: weeks ≤18, days 24–112, hours ≥140
// (≥3 hour ticks fit).
export const ZOOM = [6, 10, 14, 18, 24, 32, 44, 60, 84, 112, 140, 180, 230]
export const DEFAULT_ZOOM_IDX = 5 // → 32px/day (days tier)

export function useChartTimeline({
  scheduled,
  milestones,
  estCfg,
  leftW,
  // Pan starts on a pointerdown that reached empty track. Bars stop propagation
  // themselves, but overlay handles that sit ON the track have to be named here
  // (Gantt's link knob), or dragging one would pan the chart instead.
  panBlockers = ['.bar'],
  // Drag/link gestures own the pointer while active — the cursor guide hides so
  // the two don't fight over the same pointermove.
  cursorBlocked = () => false,
} = {}) {
  const { formatDate, formatTime, formatters } = useFormat()
  const scrollEl = ref(null)
  const bodyEl = ref(null)

  // ── zoom ──
  const zoomIdx = ref(DEFAULT_ZOOM_IDX)
  const dayW = computed(() => ZOOM[zoomIdx.value])
  // While the user is actively zooming (a burst of wheel ticks), bars/today/ghost
  // SNAP instead of CSS-transitioning — animating every intermediate step made the
  // chart jitter on multi-step zoom. `zooming` gates the `.animate` class; a short
  // debounce re-enables transitions once the scale has settled.
  const zooming = ref(false)
  let zoomSettle = 0
  // Re-zoom while keeping the day under `anchorClientX` (a viewport x; omit → centre
  // of the viewport) pinned in place. We read the day at the anchor BEFORE the width
  // changes, then on the next tick (once axisW has reflowed) set scrollLeft so that
  // same day lands back under the anchor — no more snapping to the earliest task.
  function applyZoom(newIdx, anchorClientX) {
    newIdx = Math.max(0, Math.min(ZOOM.length - 1, newIdx))
    if (newIdx === zoomIdx.value) return
    const el = scrollEl.value
    const oldW = ZOOM[zoomIdx.value]
    const newW = ZOOM[newIdx]
    let anchorX = null
    let dayAtAnchor = null
    if (el) {
      const rect = el.getBoundingClientRect()
      anchorX = anchorClientX != null ? anchorClientX - rect.left : el.clientWidth / 2
      dayAtAnchor = (el.scrollLeft + anchorX - leftW.value) / oldW
    }
    zooming.value = true
    clearTimeout(zoomSettle)
    zoomSettle = setTimeout(() => (zooming.value = false), 200)
    zoomIdx.value = newIdx
    if (el && dayAtAnchor != null) {
      nextTick(() => {
        el.scrollLeft = Math.max(0, dayAtAnchor * newW + leftW.value - anchorX)
      })
    }
  }
  function zoomIn(anchorX) {
    applyZoom(zoomIdx.value + 1, anchorX)
  }
  function zoomOut(anchorX) {
    applyZoom(zoomIdx.value - 1, anchorX)
  }

  // ── viewport tracking ──
  const todayMs = startOfDay(Date.now())
  // Current axis granularity tier: 'weeks' | 'days' | 'hours'.
  const tier = computed(() => tierFor(dayW.value))
  // Horizontal scroll position + viewport width, rAF-throttled — drives the windowed
  // hour-tick rendering (only paints the visible slice).
  const scrollX = ref(0)
  const viewW = ref(0)
  // Vertical scroll + viewport height + the body's y-offset within the scroll content
  // (header height), rAF-throttled — drives row virtualization (see useChartRows).
  const scrollY = ref(0)
  const viewH = ref(0)
  const bodyTop = ref(0)

  // ── axis range: covers every scheduled task (incl. its estimate ghost) + today ──
  const range = computed(() => {
    let lo = todayMs
    let hi = todayMs
    for (const t of scheduled.value) {
      const { a, b } = spanOf(t)
      lo = Math.min(lo, a)
      hi = Math.max(hi, b)
      // The estimate "ghost" can reach past the due date — keep its end on-scale.
      const gd = estimateToDays(t.estimate, estCfg.value)
      if (gd != null) hi = Math.max(hi, a + Math.ceil(gd) * DAY_MS)
    }
    lo -= 3 * DAY_MS
    hi += 7 * DAY_MS
    const days = Math.round((hi - lo) / DAY_MS) + 1
    return { start: startOfDay(lo), days }
  })
  const axisW = computed(() => range.value.days * dayW.value)
  const dayIndex = (ms) => Math.round((startOfDay(ms) - range.value.start) / DAY_MS)

  // Axis header bands. Months always show; the second band is per-day (days/hours
  // tiers) or per-week (weeks tier); the hours tier adds an hour-tick row.
  // `formatters.value` is read inside the computed, so the axis header re-renders
  // in the new language the moment the preference changes.
  const days = computed(() =>
    buildDays(range.value.start, range.value.days, todayMs, formatters.value),
  )
  const monthBands = computed(() => buildMonthBands(days.value, formatters.value))
  const weekBands = computed(() => (tier.value === 'weeks' ? buildWeekBands(days.value) : []))
  // Hour ticks only for the visible viewport slice (+ a margin) — "lazy lines" so a
  // wide board doesn't render thousands of header nodes in the hours tier.
  const hourTicks = computed(() => {
    if (tier.value !== 'hours') return []
    const lo = scrollX.value - leftW.value - 200
    const hi = scrollX.value - leftW.value + viewW.value + 200
    return hourTicksInWindow(range.value.days, dayW.value, lo, hi)
  })
  // Minor-gridline period (px): the hour-tick step in the hours tier, else one day.
  const subW = computed(() => {
    const step = hourStepFor(dayW.value)
    return tier.value === 'hours' && step ? (step / 24) * dayW.value : dayW.value
  })

  // Today-line x: the real current time in the hours tier, else today's cell centre.
  const todayLeft = computed(() =>
    tier.value === 'hours'
      ? xAt(Date.now(), range.value.start, dayW.value)
      : dayIndex(todayMs) * dayW.value + dayW.value / 2,
  )

  // Milestone due-markers: dashed vertical lines at each milestone's due date that
  // falls within the current axis range.
  const milestoneMarkers = computed(() =>
    (milestones.value || [])
      .filter((m) => m.due_date)
      .map((m) => {
        const di = dayIndex(parseDate(m.due_date))
        return { id: m.id, title: m.title, di, left: di * dayW.value + dayW.value / 2 }
      })
      .filter((m) => m.di >= 0 && m.di < range.value.days),
  )

  // ── scroll-to-today ──
  // rAF-driven so the glide is reliable: native `scrollTo({behavior:'smooth'})` on
  // this overflow container was a no-op in practice (jumped instantly).
  let scrollRaf = 0
  function animateScrollLeft(target) {
    const el = scrollEl.value
    if (!el) return
    cancelAnimationFrame(scrollRaf)
    const from = el.scrollLeft
    const dist = target - from
    if (Math.abs(dist) < 1) {
      el.scrollLeft = target
      return
    }
    const t0 = performance.now()
    const dur = 340
    const ease = (p) => 1 - Math.pow(1 - p, 3)
    const step = (now) => {
      const p = Math.min(1, (now - t0) / dur)
      el.scrollLeft = from + dist * ease(p)
      if (p < 1) scrollRaf = requestAnimationFrame(step)
    }
    scrollRaf = requestAnimationFrame(step)
  }
  function centerToday(smooth = true) {
    const el = scrollEl.value
    if (!el) return
    const left = Math.max(0, dayIndex(todayMs) * dayW.value - el.clientWidth / 2 + leftW.value)
    if (smooth === false) el.scrollLeft = left
    else animateScrollLeft(left)
  }
  // Centre on the ref appearing rather than on onMounted: the scroll container is
  // the component's template ref, so watching it is what guarantees the element
  // exists (and has a width) when we compute the initial scrollLeft.
  watch(
    scrollEl,
    (el) =>
      el &&
      nextTick(() => {
        centerToday(false)
        syncScroll()
      }),
  )

  // rAF-throttled scroll/resize sync for the windowed hour ticks + row windowing.
  let scrollRaf2 = 0
  function syncScroll() {
    const el = scrollEl.value
    if (!el) return
    scrollX.value = el.scrollLeft
    viewW.value = el.clientWidth
    scrollY.value = el.scrollTop
    viewH.value = el.clientHeight
    if (bodyEl.value) bodyTop.value = bodyEl.value.offsetTop
  }
  function onScroll() {
    if (scrollRaf2) return
    scrollRaf2 = requestAnimationFrame(() => {
      scrollRaf2 = 0
      syncScroll()
    })
  }

  // ── pan: middle-button anywhere, or left-drag on empty chart space ──
  // (bars stop propagation on their own pointerdown, so a pointerdown that reaches
  // the scroll container is on empty space.)
  const pan = ref(null)
  function onPanDown(e) {
    const middle = e.button === 1
    const emptyLeft =
      e.button === 0 &&
      e.target.closest('.tl-track') &&
      !panBlockers.some((sel) => e.target.closest(sel))
    if (!middle && !emptyLeft) return
    const el = scrollEl.value
    if (!el) return
    e.preventDefault()
    pan.value = { x: e.clientX, y: e.clientY, sl: el.scrollLeft, st: el.scrollTop }
    window.addEventListener('pointermove', onPanMove)
    window.addEventListener('pointerup', onPanUp)
  }
  function onPanMove(e) {
    const p = pan.value
    const el = scrollEl.value
    if (!p || !el) return
    el.scrollLeft = p.sl - (e.clientX - p.x)
    el.scrollTop = p.st - (e.clientY - p.y)
  }
  function onPanUp() {
    pan.value = null
    window.removeEventListener('pointermove', onPanMove)
    window.removeEventListener('pointerup', onPanUp)
  }
  // Ctrl/Cmd + wheel zooms, anchored on the day under the cursor.
  function onWheel(e) {
    if (!(e.ctrlKey || e.metaKey)) return
    e.preventDefault()
    if (e.deltaY < 0) zoomIn(e.clientX)
    else zoomOut(e.clientX)
  }

  // ── hover cursor guide ──
  // A neutral vertical line tracking the pointer over the chart, with a date/time
  // pill pinned to the top of the viewport — read off the exact date a bar's edge
  // lands on. Kept separate from the bar hover preview (which sits beside the bar)
  // so the two never collide.
  const cursor = ref(null) // { axisX, ms } in content (axis) coords, or null
  const cursorPill = ref({ x: 0, top: 0 }) // fixed viewport coords for the date pill
  function onHoverMove(e) {
    if (pan.value || cursorBlocked()) {
      cursor.value = null
      return
    }
    const el = scrollEl.value
    if (!el) return
    const rect = el.getBoundingClientRect()
    const xv = e.clientX - rect.left
    // Off the chart (over the sticky left column or past the right edge): hide.
    if (xv < leftW.value || xv > el.clientWidth) {
      cursor.value = null
      return
    }
    const axisX = Math.max(0, Math.min(axisW.value, el.scrollLeft + xv - leftW.value))
    cursor.value = { axisX, ms: range.value.start + (axisX / dayW.value) * DAY_MS }
    cursorPill.value = { x: e.clientX, top: rect.top }
  }
  function onHoverLeave() {
    cursor.value = null
  }
  // Day + short month (+ year off the current one), and the clock time in the hours tier.
  const cursorLabel = computed(() => {
    if (!cursor.value) return ''
    const dt = new Date(cursor.value.ms)
    // The timeline is laid out on local calendar days, so its cursor label reads
    // in the browser timezone (timeZone: null) — see DueEditor for the same rule.
    const o = { day: '2-digit', month: 'short', timeZone: null }
    if (dt.getFullYear() !== new Date().getFullYear()) o.year = 'numeric'
    let s = formatDate(dt, o)
    if (tier.value === 'hours') s += ` ${formatTime(dt, { timeZone: null })}`
    return s
  })

  onBeforeUnmount(() => {
    cancelAnimationFrame(scrollRaf)
    cancelAnimationFrame(scrollRaf2)
    clearTimeout(zoomSettle)
    window.removeEventListener('pointermove', onPanMove)
    window.removeEventListener('pointerup', onPanUp)
  })

  return {
    // elements
    scrollEl,
    bodyEl,
    // zoom
    ZOOM,
    zoomIdx,
    dayW,
    zooming,
    applyZoom,
    zoomIn,
    zoomOut,
    // axis
    todayMs,
    tier,
    range,
    axisW,
    dayIndex,
    days,
    monthBands,
    weekBands,
    hourTicks,
    subW,
    todayLeft,
    milestoneMarkers,
    // viewport
    scrollX,
    viewW,
    scrollY,
    viewH,
    bodyTop,
    syncScroll,
    onScroll,
    centerToday,
    animateScrollLeft,
    // gestures
    pan,
    onPanDown,
    onWheel,
    // cursor guide
    cursor,
    cursorPill,
    cursorLabel,
    onHoverMove,
    onHoverLeave,
  }
}
