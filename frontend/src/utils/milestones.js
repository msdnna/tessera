// Shared milestone helpers (used by the card chip, the task picker and the manager).

// The reserved ?milestone= value for "tasks without a milestone".
export const BACKLOG_SCOPE = 'backlog'

// milestoneKey is what goes into ?milestone= — the slug, so sprint links read
// ?milestone=sprint-1. Falls back to the UUID for rows the startup backfill has
// not reached yet.
export function milestoneKey(m) {
  if (!m) return BACKLOG_SCOPE
  return m.slug || m.id
}

// matchesScope tells whether a ?milestone= value points at this milestone. Both
// forms count: links shared before slugs existed carry the UUID.
export function matchesScope(m, scope) {
  if (!scope) return false
  if (!m) return scope === BACKLOG_SCOPE
  return scope === m.slug || scope === m.id
}

function fmtDate(iso) {
  if (!iso) return ''
  return new Date(iso).toLocaleDateString('ru-RU', {
    day: 'numeric',
    month: 'short',
    year: 'numeric',
  })
}

// Human-readable start–due window for a milestone (either side may be missing).
export function milestoneRange(m) {
  if (!m) return ''
  const s = fmtDate(m.start_date)
  const d = fmtDate(m.due_date)
  if (s && d) return `${s} – ${d}`
  if (d) return `до ${d}`
  if (s) return `с ${s}`
  return ''
}
