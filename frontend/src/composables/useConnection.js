import { reactive } from 'vue'

// Shared connection/liveness state, driven by the axios interceptors (api/index.js)
// and rendered by AppConnectionOverlay.vue. A plain reactive singleton (no Pinia)
// so the api module — imported before any component — can mutate it directly.
//
//   pending — in-flight request count (balanced start/end across retries)
//   active  — show the thin top progress bar: a request has been pending long
//             enough to be worth surfacing (short delay skips the flash on fast
//             calls), and the bar lingers a beat so bursts don't strobe it
//   offline — the last request could not reach the server (no HTTP response)
//
// Note the deliberate design change (task #2616): a slow-but-live request no
// longer raises a full-screen blocking overlay. On a remote install every call
// takes a beat, and blocking the UI for each one made the app feel broken. A
// non-blocking top bar communicates "working" without stealing the board.
export const connection = reactive({
  pending: 0,
  active: false,
  offline: false,
})

const SHOW_AFTER = 250 // ms a request may run before the bar appears (skips fast calls)
const MIN_VISIBLE = 450 // ms the bar stays up once shown, so bursts don't strobe it
let showTimer = null
let hideTimer = null
let shownAt = 0

function now() {
  return Date.now()
}

export function reqStart() {
  connection.pending += 1
  // A fresh request cancels any pending hide so the bar stays continuous across
  // a burst of calls (a board open fires ~10 at once).
  if (hideTimer) {
    clearTimeout(hideTimer)
    hideTimer = null
  }
  if (!connection.active && !showTimer) {
    showTimer = setTimeout(() => {
      showTimer = null
      if (connection.pending > 0) {
        connection.active = true
        shownAt = now()
      }
    }, SHOW_AFTER)
  }
}

export function reqEnd(reached) {
  connection.pending = Math.max(0, connection.pending - 1)
  if (reached) connection.offline = false
  if (connection.pending > 0) return
  // Everything settled: drop a not-yet-shown bar immediately; hold a shown one
  // for the rest of its minimum visible window so it doesn't blink out.
  if (showTimer) {
    clearTimeout(showTimer)
    showTimer = null
  }
  if (connection.active && !hideTimer) {
    const wait = Math.max(0, MIN_VISIBLE - (now() - shownAt))
    hideTimer = setTimeout(() => {
      hideTimer = null
      if (connection.pending === 0) connection.active = false
    }, wait)
  }
}

export function setOffline(v) {
  connection.offline = v
}
