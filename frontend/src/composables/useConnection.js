import { reactive } from 'vue'

// Shared connection/liveness state, driven by the axios interceptors (api/index.js)
// and rendered by AppConnectionOverlay.vue. A plain reactive singleton (no Pinia)
// so the api module — imported before any component — can mutate it directly.
//
//   pending — in-flight request count (balanced start/end across retries)
//   slow    — a request has been pending past SLOW_AFTER and is still running
//   offline — the last request could not reach the server (no HTTP response)
export const connection = reactive({
  pending: 0,
  slow: false,
  offline: false,
})

const SLOW_AFTER = 4000 // ms a request may run before we surface the "slow" loader
let slowTimer = null

export function reqStart() {
  connection.pending += 1
  if (connection.pending === 1 && !slowTimer) {
    slowTimer = setTimeout(() => {
      // Only flag slow if something is still actually in flight.
      if (connection.pending > 0) connection.slow = true
    }, SLOW_AFTER)
  }
}

export function reqEnd(reached) {
  connection.pending = Math.max(0, connection.pending - 1)
  if (reached) connection.offline = false
  if (connection.pending === 0) {
    if (slowTimer) {
      clearTimeout(slowTimer)
      slowTimer = null
    }
    connection.slow = false
  }
}

export function setOffline(v) {
  connection.offline = v
}
