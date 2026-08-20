import { ref, watch, onBeforeUnmount } from 'vue'

// Resolves the DOM nodes a tour step points at and keeps their boxes fresh
// (#2753).
//
// Unlike the What's-New spotlight, which only ever looked up [data-nav="…"] once
// on `resize`, tour targets appear asynchronously (a dropdown opens, a modal
// mounts, a card is created) and move while the user scrolls a kanban column. So
// we re-resolve on every DOM mutation and re-measure on scroll/resize/element
// resize, all coalesced into one rAF per frame.

// A tour key is either a bare anchor name (`ws-switch` → [data-tour="ws-switch"])
// or, when the element already carries a usable attribute, a raw CSS selector.
export function anchorSelector(key) {
  if (!key) return ''
  return /[[\].#\s>]/.test(key) ? key : `[data-tour="${key}"]`
}

// Currently-open naive floating panels (a date picker, a priority/tags popover,
// a select menu). The guide needs their boxes so the mask can cut them out
// instead of dimming the very control it just told the user to use, and so the
// popover can be parked clear of them (#2753 rework). Tooltips are excluded —
// they're transient hover noise, not something the user interacts with.
export const PANEL_SEL = [
  '.n-popover:not(.n-tooltip)',
  '.n-date-panel',
  '.n-dropdown-menu',
  '.n-base-select-menu',
  '.n-color-picker-panel',
  '.n-cascader-menu',
].join(',')

// Larger overlay surfaces the user may open mid-tour — a create-workspace modal,
// the board-settings drawer. These get cut OUT of the dimming mask so the guide
// never darkens something the user just opened (#2753 rework), but unlike the
// small pickers they are NOT fed to popover placement: the tour popover has to be
// free to sit *inside* a modal (naming a project, filling a task field).
export const SURFACE_SEL = ['.n-modal', '.n-drawer'].join(',')

// The container an advanceOn.moved spec's element currently sits in, read as a
// plain string so the store can compare "before" with "now" without holding on
// to DOM nodes — the board re-renders its card list after every drop, so a node
// reference would go stale exactly when it matters.
export function placeOf(spec) {
  if (!spec?.el || !spec.within || !spec.by) return null
  const el = document.querySelector(anchorSelector(spec.el))
  const box = el?.closest(anchorSelector(spec.within))
  return box?.getAttribute(spec.by) ?? null
}

function boxOf(el) {
  const r = el.getBoundingClientRect()
  // Elements that are in the DOM but not laid out (display:none, a collapsed
  // drawer) measure 0×0 — treat them as "not there yet" so the step waits for
  // the real thing instead of drawing an arrow into the corner.
  if (!r.width && !r.height) return null
  // Fully off-screen (a tab scrolled past the strip's edge, a row below the
  // fold) — don't point at something the user can't see (#2753 rework).
  if (r.right <= 0 || r.bottom <= 0 || r.left >= window.innerWidth || r.top >= window.innerHeight) {
    return null
  }
  return {
    left: r.left,
    top: r.top,
    width: r.width,
    height: r.height,
    right: r.right,
    bottom: r.bottom,
  }
}

/**
 * Track one or more anchors.
 *
 * @param keysFn    () => string[]  reactive getter for the keys of the current step
 * @param onMissing (keys) => void  called once when the *first* key never resolved
 *                                  within `timeout` ms, so the caller can skip the
 *                                  step rather than hang on an arrow to nothing
 * @param countFn   () => string    reactive getter for a key whose *number* of
 *                                  matches the caller wants (the guide advances a
 *                                  step when the entity it asked for appears, and
 *                                  tasks live in component state with no store to
 *                                  watch — the DOM is what there is). Counted on
 *                                  the same mutation pass, so it costs one extra
 *                                  querySelectorAll per frame that already ran.
 * @param placeFn   () => spec     reactive getter for an advanceOn.moved spec
 *                                  ({ el, within, by }, selectors already
 *                                  resolved). Reports the *address* of the
 *                                  container `el` currently sits in, so the
 *                                  caller can tell a drag actually landed
 *                                  somewhere else (#2778). Read on the same
 *                                  pass — one more querySelector per frame.
 */
export function useTourAnchor(keysFn, { timeout = 8000, onMissing, countFn, placeFn } = {}) {
  const rects = ref([])
  const els = ref([])
  const count = ref(0)
  const place = ref(null)
  const panels = ref([])
  const surfaces = ref([])
  let frame = 0
  let missTimer = 0
  let ro = null
  let mo = null

  function measure() {
    frame = 0
    const keys = keysFn() || []
    const nextEls = keys.map((k) => document.querySelector(anchorSelector(k)))
    const nextRects = nextEls.map((el) => (el ? boxOf(el) : null))
    els.value = nextEls
    rects.value = nextRects
    const countSel = anchorSelector(countFn?.())
    count.value = countSel ? document.querySelectorAll(countSel).length : 0
    place.value = placeOf(placeFn?.())
    panels.value = [...document.querySelectorAll(PANEL_SEL)].map(boxOf).filter(Boolean)
    surfaces.value = [...document.querySelectorAll(SURFACE_SEL)].map(boxOf).filter(Boolean)
    if (ro) {
      ro.disconnect()
      nextEls.forEach((el) => el && ro.observe(el))
    }
    if (nextRects[0] && missTimer) {
      clearTimeout(missTimer)
      missTimer = 0
    }
  }

  function refresh() {
    if (frame) return
    frame = requestAnimationFrame(measure)
  }

  function armMissing(keys) {
    clearTimeout(missTimer)
    missTimer = 0
    if (!keys.length || !onMissing) return
    missTimer = setTimeout(() => {
      missTimer = 0
      if (!rects.value[0]) onMissing(keys)
    }, timeout)
  }

  if (typeof ResizeObserver !== 'undefined') ro = new ResizeObserver(refresh)
  if (typeof MutationObserver !== 'undefined') {
    mo = new MutationObserver(refresh)
    mo.observe(document.body, { childList: true, subtree: true, attributes: true })
  }
  window.addEventListener('resize', refresh)
  window.addEventListener('scroll', refresh, true)

  watch(
    // The moved-spec is serialised: it's rebuilt on every read, so passing the
    // object itself would re-measure on every unrelated reactive tick.
    () => [keysFn(), countFn?.(), JSON.stringify(placeFn?.() ?? null)],
    ([keys]) => {
      measure() // synchronous so the first paint already has a box
      armMissing(keys || [])
    },
    { immediate: true, flush: 'post' },
  )

  onBeforeUnmount(() => {
    window.removeEventListener('resize', refresh)
    window.removeEventListener('scroll', refresh, true)
    mo?.disconnect()
    ro?.disconnect()
    clearTimeout(missTimer)
    if (frame) cancelAnimationFrame(frame)
  })

  return { rects, els, count, place, panels, surfaces, refresh }
}
