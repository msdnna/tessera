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

function boxOf(el) {
  const r = el.getBoundingClientRect()
  // Elements that are in the DOM but not laid out (display:none, a collapsed
  // drawer) measure 0×0 — treat them as "not there yet" so the step waits for
  // the real thing instead of drawing an arrow into the corner.
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

/**
 * Track one or more anchors.
 *
 * @param keysFn    () => string[]  reactive getter for the keys of the current step
 * @param onMissing (keys) => void  called once when the *first* key never resolved
 *                                  within `timeout` ms, so the caller can skip the
 *                                  step rather than hang on an arrow to nothing
 */
export function useTourAnchor(keysFn, { timeout = 8000, onMissing } = {}) {
  const rects = ref([])
  const els = ref([])
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
    keysFn,
    (keys) => {
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

  return { rects, els, refresh }
}
