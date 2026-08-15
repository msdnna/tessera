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

export function saveTaskLayout(v) {
  if (!TASK_LAYOUTS.includes(v)) return
  try {
    localStorage.setItem(STORE_KEY, v)
  } catch {
    /* storage disabled — non-fatal */
  }
}
