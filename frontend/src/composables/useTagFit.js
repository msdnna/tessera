import { ref, watch, nextTick, onBeforeUnmount } from 'vue'

// Shared "fit as many whole tag chips on one line as possible, rest → +N" logic
// for the tag value (task modal + stacked card row). An invisible measurement row
// (natural chip widths, never sliced) is measured against the trigger's width,
// reserving room for the +N chip when not everything fits.
//
// Two subtleties this handles:
//  1. The trigger can be REPLACED (the modal unmounts its content on close and
//     remounts on open → a fresh `valEl`), so the ResizeObserver is (re)created
//     whenever `valEl` changes — watching a stale detached node was one bug.
//  2. A content-sized trigger (e.g. the modal's inline-flex button) shrinks to its
//     visible chips, so measuring it directly feeds back: once collapsed to one
//     chip it reports a narrow width and stays collapsed (the "reopen after another
//     task shows 1 chip + N" bug). We first EXPAND to all chips so the trigger
//     reaches its true container width, read that, then apply the fit — guarding
//     against the self-induced resize so it doesn't oscillate.
export function useTagFit(valEl, measureEl, tags, opts = {}) {
  const pad = opts.pad ?? 20 // trigger horizontal padding to discount
  const gap = opts.gap ?? 6
  const plusW = opts.plusW ?? 46 // reserved px for the "+N" chip (incl. gap)
  const visibleCount = ref(99)
  let ro = null
  let selfResize = false
  let clearT = 0

  function fitCount(avail, widths, reserve) {
    let used = 0
    let n = 0
    for (let i = 0; i < widths.length; i++) {
      const add = widths[i] + (i > 0 ? gap : 0)
      if (used + add + reserve > avail) break
      used += add
      n++
    }
    return n
  }
  function setCount(n) {
    if (n === visibleCount.value) return
    // Mark the resize as self-induced so the observer ignores it (else shrinking
    // the trigger to the fitted chips would re-trigger a measure → oscillation).
    selfResize = true
    visibleCount.value = n
    clearTimeout(clearT)
    clearT = setTimeout(() => (selfResize = false), 60)
  }
  function compute() {
    const val = valEl.value
    const m = measureEl.value
    if (!val || !m) return
    const avail = val.clientWidth - pad
    const widths = [...m.children].map((ch) => ch.offsetWidth)
    if (!widths.length || avail <= 0) {
      setCount(widths.length)
      return
    }
    let n = fitCount(avail, widths, 0)
    if (n < widths.length) n = fitCount(avail, widths, plusW) // make room for +N
    setCount(Math.max(n, 1)) // always show at least one
  }
  function measure() {
    const m = measureEl.value
    const total = m ? m.children.length : 0
    if (total && visibleCount.value < total) {
      // Expand to full width first, then read on the next frame.
      selfResize = true
      visibleCount.value = total
      clearTimeout(clearT)
      requestAnimationFrame(() => {
        selfResize = false
        compute()
      })
    } else {
      compute()
    }
  }

  watch(
    [tags, valEl],
    () => {
      nextTick(measure)
      if (valEl.value && typeof ResizeObserver !== 'undefined') {
        ro?.disconnect() // re-observe: valEl may be a freshly-mounted element
        ro = new ResizeObserver(() => {
          if (!selfResize) measure()
        })
        ro.observe(valEl.value)
      }
    },
    { immediate: true },
  )
  onBeforeUnmount(() => {
    ro?.disconnect()
    clearTimeout(clearT)
  })

  return { visibleCount, measure }
}
