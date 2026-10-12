import { onMounted, onBeforeUnmount, unref } from 'vue'

// A swipe in from the left edge of the screen (v2, #2976 — package G): opens
// the project tree's drawer on a phone, the way the board's name does.
//
// Only a touch that starts within EDGE px of the left edge counts, and only a
// mostly horizontal one — the board's columns scroll sideways under the same
// finger everywhere else, and a vertical scroll must stay a scroll.
const EDGE = 20
const DISTANCE = 48

export function useEdgeSwipe(onSwipe, { enabled = true } = {}) {
  let start = null

  function onStart(e) {
    start = null
    if (!unref(enabled) || e.touches.length !== 1) return
    const t = e.touches[0]
    if (t.clientX > EDGE) return
    start = { x: t.clientX, y: t.clientY }
  }
  function onMove(e) {
    if (!start) return
    const t = e.touches[0]
    const dx = t.clientX - start.x
    const dy = Math.abs(t.clientY - start.y)
    if (dy > 32 && dy > dx) {
      start = null
      return
    }
    if (dx > DISTANCE && dx > dy * 1.5) {
      start = null
      onSwipe()
    }
  }
  function onEnd() {
    start = null
  }

  onMounted(() => {
    window.addEventListener('touchstart', onStart, { passive: true })
    window.addEventListener('touchmove', onMove, { passive: true })
    window.addEventListener('touchend', onEnd, { passive: true })
    window.addEventListener('touchcancel', onEnd, { passive: true })
  })
  onBeforeUnmount(() => {
    window.removeEventListener('touchstart', onStart)
    window.removeEventListener('touchmove', onMove)
    window.removeEventListener('touchend', onEnd)
    window.removeEventListener('touchcancel', onEnd)
  })
}
