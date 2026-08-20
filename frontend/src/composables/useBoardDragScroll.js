import { ref, onBeforeUnmount } from 'vue'

// Custom edge auto-scroll during a board drag.
//
// Sortable's built-in auto-scroll doesn't reliably scroll a nested horizontal
// container on touch. Rather than chase flaky move events, we read the position of
// Sortable's own drag image (`.sortable-fallback` on touch, `.sortable-drag` on
// desktop) each animation frame, with a dragover fallback for desktop.
//
// `dragging` is also the board's "hold everything" flag: it mutes realtime-driven
// reloads and freezes card windowing, so SortableJS sees a stable DOM.
//
// `boardDragging` mirrors that flag at module scope, the same way the sidebar
// exposes `sidebarDragging`: the Get Started overlay has to fade itself out while
// a card is in the air (#2778) and it is nowhere near the board in the component
// tree. The instance ref stays the one the board itself reads — a module-level
// ref would be shared by two mounted boards, and the tour only needs "is *a* drag
// happening", which is exactly what a shared flag says.
//
// - `scrollEl`  — ref to the horizontally scrolling column strip
// - `colWidth`  — computed column width in px (stride = width + gap)
// - `gap`       — px between columns
export const boardDragging = ref(false)

export function useBoardDragScroll({ scrollEl, colWidth, gap, edge = 72, stepCooldown = 600 }) {
  const dragging = ref(false) // any drag (column OR card): autoscroll, reload guard, reveal
  // Card-only drag: gates the per-card subtask nest dropzone hint. A column drag
  // must NOT flip this, otherwise every childless card flashes a dashed drop hint.
  const draggingCard = ref(false)

  let raf = null
  let pointerX = null // last desktop dragover X (touch uses the drag image)
  let lastStep = 0
  let scrollIdx = null // tracked target column index (avoids reading mid-animation scrollLeft)

  function onDragOver(e) {
    pointerX = e.clientX
  }

  function dragX() {
    // Touch: the moving clone follows the finger. (On desktop `.sortable-drag` is
    // the static original, so there we fall back to the dragover X instead.)
    const clone = document.querySelector('.sortable-fallback')
    if (clone) {
      const r = clone.getBoundingClientRect()
      return r.left + r.width / 2
    }
    return pointerX
  }

  // Scroll exactly one column in `dir` (-1 left / +1 right), snapping to its start.
  function stepColumn(dir) {
    const el = scrollEl.value
    if (!el) return
    const stride = colWidth.value + gap
    const maxIdx = Math.round((el.scrollWidth - el.clientWidth) / stride)
    if (scrollIdx == null) scrollIdx = Math.round(el.scrollLeft / stride)
    scrollIdx = Math.max(0, Math.min(maxIdx, scrollIdx + dir))
    el.scrollTo({ left: scrollIdx * stride, behavior: 'smooth' })
  }

  function tick() {
    const el = scrollEl.value
    const px = dragX()
    if (dragging.value && el && px != null) {
      const rect = el.getBoundingClientRect()
      let dir = 0
      if (px < rect.left + edge) dir = -1
      else if (px > rect.right - edge) dir = 1
      if (dir !== 0) {
        // One column per entry, then one more every cooldown if held at the edge.
        const now = performance.now()
        if (now - lastStep > stepCooldown) {
          stepColumn(dir)
          lastStep = now
        }
      } else {
        // Centre: re-sync the target to where we actually are so the next step
        // moves exactly one column (no skipping from a mid-animation read).
        lastStep = 0
        scrollIdx = Math.round(el.scrollLeft / (colWidth.value + gap))
      }
    }
    raf = requestAnimationFrame(tick)
  }

  function onDragStart() {
    dragging.value = true
    boardDragging.value = true
    pointerX = null
    scrollIdx = null
    lastStep = 0
    // Mobile uses scroll-snap (x mandatory) + smooth scrolling, which both revert
    // our per-frame scrollLeft nudges — disable them for the duration of the drag.
    const el = scrollEl.value
    if (el) {
      el.style.scrollSnapType = 'none'
      el.style.scrollBehavior = 'auto'
    }
    window.addEventListener('dragover', onDragOver, { passive: true })
    if (!raf) raf = requestAnimationFrame(tick)
  }

  // Card drag adds the nest-hint flag on top of the shared drag setup; column drag
  // calls onDragStart directly and leaves draggingCard false.
  function onCardDragStart() {
    draggingCard.value = true
    onDragStart()
  }

  function onDragEnd() {
    dragging.value = false
    boardDragging.value = false
    draggingCard.value = false
    pointerX = null
    const el = scrollEl.value
    if (el) {
      el.style.scrollSnapType = ''
      el.style.scrollBehavior = ''
    }
    window.removeEventListener('dragover', onDragOver)
    if (raf) {
      cancelAnimationFrame(raf)
      raf = null
    }
  }

  // Unmounting mid-drag would otherwise leave the rAF loop and the window listener
  // running against a detached element.
  onBeforeUnmount(onDragEnd)

  return { dragging, draggingCard, onDragStart, onCardDragStart, onDragEnd }
}
