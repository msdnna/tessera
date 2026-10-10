// Small pure helpers of the account screens (v2, #3035).

// safeNext reads a `?next=` into an in-app path. It is a query parameter anyone
// can put into a link they hand out, so only a path of this app counts — an
// absolute URL or a protocol-relative `//host` falls back to the home screen.
export function safeNext(next, fallback = '/') {
  if (typeof next !== 'string' || !next.startsWith('/') || next.startsWith('//')) return fallback
  if (next.startsWith('/\\')) return fallback
  return next
}

// formatCountdown words seconds as the mockup's «0:42».
export function formatCountdown(seconds) {
  const s = Math.max(0, Math.ceil(Number(seconds) || 0))
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`
}

// minutesLeft is how many whole minutes a deadline still has, rounded up, so a
// link with 40 seconds left reads «ещё 1 минуту», never «ещё 0 минут».
export function minutesLeft(deadline, now = Date.now()) {
  const at = deadline instanceof Date ? deadline.getTime() : new Date(deadline).getTime()
  if (!Number.isFinite(at)) return 0
  const ms = at - now
  return ms <= 0 ? 0 : Math.ceil(ms / 60000)
}

// initialsOfSpace makes the two letters of a workspace tile («Студия «Север»» →
// «СС»), skipping punctuation such as guillemets.
export function initialsOfSpace(name) {
  const words = String(name || '')
    .split(/\s+/)
    .map((w) => w.replace(/[^\p{L}\p{N}]/gu, ''))
    .filter(Boolean)
  if (!words.length) return '?'
  const first = words[0][0]
  const second = words.length > 1 ? words[1][0] : words[0][1] || ''
  return (first + second).toUpperCase()
}
