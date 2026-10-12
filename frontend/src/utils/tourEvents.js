// The checklist's client-side events (#3000, #3102): things the user did that
// never reach the server as a socket event — a window opened, a card driven by
// keys, a stage picked on the board. Components call tourEvent('<name>'); the
// shell (AppLayout) points the sink at the onboarding store, which turns names
// into checklist results (stores/onboarding.js EVENT_ITEMS).
//
// A plain function rather than a store import so low-level modules (the
// shortcut dispatcher, the inbox store) can report without depending on Pinia
// — and in tests, with no sink set, it is a no-op.
let sink = null

export function setTourEventSink(fn) {
  sink = typeof fn === 'function' ? fn : null
}

export function tourEvent(name) {
  if (!sink || !name) return
  try {
    sink(name)
  } catch {
    /* the checklist is a nicety — never let it break the action itself */
  }
}
