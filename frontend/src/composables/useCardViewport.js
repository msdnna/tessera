import { reactive, onMounted, onBeforeUnmount } from 'vue'

// Card-list virtualization (IntersectionObserver windowing).
//
// Every column item keeps its wrapper <div> so vuedraggable's child count, indices,
// drop targets and the before/after math in onColChange stay identical (DnD
// untouched). Cards more than ~800px outside the viewport collapse to a cheap
// placeholder of their last-measured height; only near-viewport cards mount the
// heavy TaskCard. Visibility is driven by each card's *real* viewport position (one
// IO, root = viewport), so there's no model-vs-DOM divergence to thrash the
// scrollbar and the bottom is always reachable. Parity with Android's LazyColumn.
// Measuring before collapsing keeps placeholder height exact → no jump.
//
// `frozen` (the board's drag flag) suspends visibility swaps so SortableJS sees a
// stable DOM for the duration of a drag.
export const VCARD_EST = 190 // placeholder px until a card has been measured

// Keyed by "columnKey::taskId", NOT taskId alone: under tag grouping one task can
// appear in several columns at once, and a bare-id key would make those instances
// share visibility/height — collapsing/leaking cards across columns while scrolling.
export function cardKey(columnKey, taskId) {
  return `${columnKey}::${taskId}`
}

export function useCardViewport({
  frozen,
  rootMargin = '800px 0px',
  placeholderClass = 'card-ph',
} = {}) {
  const vis = reactive({}) // card key → in/near viewport (undefined = not yet known)
  const cardH = reactive({}) // card key → last measured px (from the rendered card)
  let io = null // toggles visibility by real viewport position
  let ro = null // measures rendered cards (settles after content layout)

  // Observe each card wrapper (stable per key via item-key) once. Off-screen cards
  // collapse to a placeholder; near-viewport cards mount the real TaskCard.
  function regCard(el, key) {
    if (!el || !io) return
    el.dataset.cardId = key
    io.observe(el)
    ro.observe(el)
  }

  // Drop the previous board's windowing state so its measured heights / visibility
  // don't bleed in (re-seeded fresh by the IO/RO on render).
  function reset() {
    for (const k of Object.keys(vis)) delete vis[k]
    for (const k of Object.keys(cardH)) delete cardH[k]
  }

  onMounted(() => {
    io = new IntersectionObserver(
      (entries) => {
        if (frozen?.value) return
        for (const en of entries) {
          const id = en.target.dataset.cardId
          if (id) vis[id] = en.isIntersecting
        }
      },
      { rootMargin },
    )
    // Measure rendered cards (re-fires as TaskCard content settles, unlike a
    // one-shot read), so a collapsed card's placeholder gets its exact height.
    // Skip wrappers showing a placeholder to avoid feeding back a stale height.
    ro = new ResizeObserver((entries) => {
      for (const en of entries) {
        const el = en.target
        const id = el.dataset.cardId
        if (!id || el.firstElementChild?.classList.contains(placeholderClass)) continue
        const h = Math.round(en.contentRect.height)
        if (h > 0 && cardH[id] !== h) cardH[id] = h
      }
    })
  })

  onBeforeUnmount(() => {
    io?.disconnect()
    ro?.disconnect()
  })

  return { vis, cardH, regCard, reset }
}
