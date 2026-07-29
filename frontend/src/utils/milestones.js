// Shared milestone helpers (used by the card chip, the task picker and the manager).

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
