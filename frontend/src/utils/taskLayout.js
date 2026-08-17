// Task presentation modes (#2716): the modal can be shown as the classic centred
// dialog, edge-to-edge fullscreen, or a right-hand side panel that leaves the board
// underneath live. Which one is in force is a device-level choice, so it lives in
// localStorage rather than the server prefs — a 27" screen wants the side panel, a
// laptop the modal, and syncing the two would let one screen dictate the other.
// Same reasoning (and storage) as `tagPrefixMode` (#2604).

export const TASK_LAYOUTS = ['modal', 'fullscreen', 'sidebar']
const STORE_KEY = 'tessera_task_layout'

// Narrow screens have exactly one sane presentation — the stacked sheet the modal
// already is. A 560px side panel on a 400px phone is a fullscreen sheet with worse
// ergonomics, so the saved preference is honoured only when there's room for it to
// mean anything.
export function effectiveTaskLayout(saved, narrow) {
  if (narrow) return 'modal'
  return TASK_LAYOUTS.includes(saved) ? saved : 'modal'
}

export function loadTaskLayout() {
  try {
    const v = localStorage.getItem(STORE_KEY)
    return TASK_LAYOUTS.includes(v) ? v : 'modal'
  } catch {
    /* storage disabled — non-fatal, fall back to the default */
    return 'modal'
  }
}

// ── Dismissing the side panel (#2716). Naive wires its own click-outside even with
// the mask off, but it only fires when the event target is the modal container —
// and with `show-mask=false` that container is `pointer-events: none`, so a click on
// the board never lands on it and the panel could only be closed from its header.
// The caller listens itself and asks this predicate, which carves out the two cases
// where a click outside the card means something other than "dismiss".
const FLOATING_SEL = [
  '.v-binder-follower-container', // popovers, dropdowns, selects, date pickers
  '.n-modal-container', // nested modals / confirm dialogs
  '.n-drawer-container',
  '.n-message-container',
  '.n-notification-container',
  '.n-image-preview-container',
].join(',')

export function dismissesSidebar(target, cardEl) {
  if (!target || typeof target.closest !== 'function') return false
  // Inside the panel itself.
  if (cardEl?.contains(target)) return false
  // Floating UI: Naive teleports popovers, menus, date pickers and nested modals to
  // <body>, so "outside the card" is not the same as "outside the panel's own UI".
  if (target.closest(FLOATING_SEL)) return false
  // A board card re-points the panel at that task instead of closing it — that's the
  // whole reason the panel forgoes a mask.
  if (target.closest('[data-testid="task-card"]')) return false
  return true
}

export function saveTaskLayout(v) {
  if (!TASK_LAYOUTS.includes(v)) return
  try {
    localStorage.setItem(STORE_KEY, v)
  } catch {
    /* storage disabled — non-fatal */
  }
}
