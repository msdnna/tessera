import { ref, watch, nextTick, onBeforeUnmount } from 'vue'

// Shared "fit as many whole tag chips on one line as possible, rest → +N" logic
// for the tag value (task modal + stacked card row). An invisible measurement row
// (natural chip widths, never sliced) is measured against the trigger's width,
// reserving room for the +N chip when not everything fits.
//
// The element the ResizeObserver watches can be REPLACED (the modal unmounts its
// content on close and remounts on open → a fresh `valEl`). The observer is
// therefore (re)created whenever `valEl` changes; watching a stale detached node
// was the bug where a reopened modal showed only one chip + "+N".
export function useTagFit(valEl, measureEl, tags, opts = {}) {
  const pad = opts.pad ?? 24 // trigger horizontal padding to discount
  const gap = opts.gap ?? 6
  const plusW = opts.plusW ?? 46 // reserved px for the "+N" chip (incl. gap)
  const visibleCount = ref(99)
  let ro = null

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
  function measure() {
    const val = valEl.value
    const m = measureEl.value
    if (!val || !m) return
    const avail = val.clientWidth - pad
    const widths = [...m.children].map((ch) => ch.offsetWidth)
    if (!widths.length || avail <= 0) {
      visibleCount.value = widths.length
      return
    }
    let n = fitCount(avail, widths, 0)
    if (n < widths.length) n = fitCount(avail, widths, plusW) // make room for +N
    visibleCount.value = Math.max(n, 1) // always show at least one
  }

  watch(
    [tags, valEl],
    () => {
      nextTick(measure)
      if (valEl.value && typeof ResizeObserver !== 'undefined') {
        ro?.disconnect() // re-observe: valEl may be a freshly-mounted element
        ro = new ResizeObserver(() => measure())
        ro.observe(valEl.value)
      }
    },
    { immediate: true },
  )
  onBeforeUnmount(() => ro?.disconnect())

  return { visibleCount, measure }
}
