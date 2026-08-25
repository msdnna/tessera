// Short elapsed-time formatting for background jobs (integration sync runs).
// Deliberately coarse — the journal shows "how long did that take", not a stopwatch.
//
// Called from render code but living outside a setup context, so the units come
// from `i18n.global.t` on every call rather than a frozen module constant.
import { i18n } from '@/i18n'

// formatElapsed renders a millisecond span in short units:
// "12 с" · "1 м 12 с" · "1 ч 3 м". Sub-second spans round up to "1 с" so a
// finished run never reads as taking no time at all.
export function formatElapsed(ms) {
  if (!Number.isFinite(ms) || ms < 0) return ''
  const t = i18n.global.t
  const total = Math.max(1, Math.round(ms / 1000))
  if (total >= 3600) {
    return t('common.duration.hm', {
      h: Math.floor(total / 3600),
      m: Math.floor((total % 3600) / 60),
    })
  }
  if (total >= 60) return t('common.duration.ms', { m: Math.floor(total / 60), s: total % 60 })
  return t('common.duration.s', { s: total })
}

// runDuration formats how long a run took, from its two ISO timestamps. Returns
// '' when either end is missing or unparseable — a run still in flight has no
// duration yet (the caller shows a live "running for N" ticker instead).
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
