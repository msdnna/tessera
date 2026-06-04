// Distinguishes a long-press (→ context menu) from a press-and-drag (→ move).
// Sortable starts a drag ~160ms into any touch hold even without movement, so
// "is a drag active" can't tell the two apart. Instead we track whether the
// finger actually moved during the current press; the context-menu handlers
// open the menu only when it stayed put.
const THRESHOLD = 10 // px of movement that counts as a drag, not a long-press

let startX = 0
let startY = 0
let moved = false
let tracking = false

function onTouchStart(e) {
  const t = e.touches && e.touches[0]
  if (!t) return
  startX = t.clientX
  startY = t.clientY
  moved = false
  tracking = true
}
function onTouchMove(e) {
  if (!tracking) return
  const t = e.touches && e.touches[0]
  if (!t) return
  if (Math.hypot(t.clientX - startX, t.clientY - startY) > THRESHOLD) moved = true
}
function onTouchEnd() {
  tracking = false
}

if (typeof window !== 'undefined') {
  window.addEventListener('touchstart', onTouchStart, { passive: true, capture: true })
  window.addEventListener('touchmove', onTouchMove, { passive: true, capture: true })
  window.addEventListener('touchend', onTouchEnd, { passive: true, capture: true })
  window.addEventListener('touchcancel', onTouchEnd, { passive: true, capture: true })
  // A mouse interaction clears any stale touch-move flag (right-click always
  // opens the menu on desktop).
  window.addEventListener('mousedown', () => {
    moved = false
    tracking = false
  })
}

// True if the current/last press involved a drag-like movement → the context
// menu should be suppressed (the user is dragging, not long-pressing).
export function pressMoved() {
  return moved
}
