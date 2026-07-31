// Short elapsed-time formatting for background jobs (integration sync runs).
// Deliberately coarse — the journal shows "how long did that take", not a stopwatch.

// formatElapsed renders a millisecond span in short Russian units:
// "12 с" · "1 м 12 с" · "1 ч 3 м". Sub-second spans round up to "1 с" so a
// finished run never reads as taking no time at all.
export function formatElapsed(ms) {
  if (!Number.isFinite(ms) || ms < 0) return ''
  const total = Math.max(1, Math.round(ms / 1000))
  if (total >= 3600) return `${Math.floor(total / 3600)} ч ${Math.floor((total % 3600) / 60)} м`
  if (total >= 60) return `${Math.floor(total / 60)} м ${total % 60} с`
  return `${total} с`
}

// runDuration formats how long a run took, from its two ISO timestamps. Returns
// '' when either end is missing or unparseable — a run still in flight has no
// duration yet (the caller shows a live "идёт N" ticker instead).
export function runDuration(startISO, endISO) {
  if (!startISO || !endISO) return ''
  const start = new Date(startISO).getTime()
  const end = new Date(endISO).getTime()
  if (Number.isNaN(start) || Number.isNaN(end)) return ''
  return formatElapsed(end - start)
}

// elapsedSince formats how long ago startISO was, against `now` (injected so the
// caller's ticker drives it and tests stay deterministic).
export function elapsedSince(startISO, now = Date.now()) {
  if (!startISO) return ''
  const start = new Date(startISO).getTime()
  if (Number.isNaN(start)) return ''
  return formatElapsed(now - start)
}
