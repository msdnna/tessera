// The pickers behind a document's stage and meeting chips (#3044, #3046) —
// pure, so the order and the grouping are tested without a menu.

// Stages a document can be pinned to: the open ones (active and planned), in
// the order sortStages gave them, grouped under their project when there is
// more than one. A closed stage is listed only while it is the current pin, so
// the menu can still show the check mark and the way back out.
//
// `list` is GET /workspaces/:id/milestones, already sorted; rows are ActionMenu
// items — { key: 'ms:<id>', label, checked, note }.
export function stageChoices(list, currentId, { noteOf = () => '' } = {}) {
  const open = (list || []).filter((m) => m.state !== 'closed' || m.id === currentId)
  const projects = []
  const byProject = new Map()
  for (const m of open) {
    if (!byProject.has(m.project_id)) {
      byProject.set(m.project_id, [])
      projects.push({ id: m.project_id, name: m.project_name || '' })
    }
    byProject.get(m.project_id).push(m)
  }
  const out = []
  for (const p of projects) {
    if (projects.length > 1) out.push({ heading: p.name })
    for (const m of byProject.get(p.id)) {
      out.push({ key: `ms:${m.id}`, label: m.title, checked: m.id === currentId, note: noteOf(m) })
    }
  }
  return out
}

const at = (iso) => (iso ? Date.parse(iso) || 0 : 0)

// Meetings a document can be the agenda of: what is on now, then what is
// planned (soonest first), then the few most recent finished ones — a protocol
// is written after the call. The current pin is always there.
export const RECENT_ENDED = 5

export function meetingChoices(list, currentId, { noteOf = () => '' } = {}) {
  const rows = list || []
  const live = rows.filter((c) => c.status === 'live')
  const planned = rows
    .filter((c) => c.status === 'scheduled')
    .sort((a, b) => (at(a.scheduled_at) || Infinity) - (at(b.scheduled_at) || Infinity))
  const ended = rows
    .filter((c) => c.status === 'ended')
    .sort((a, b) => at(b.ended_at || b.scheduled_at) - at(a.ended_at || a.scheduled_at))
  const recent = ended.slice(0, RECENT_ENDED)
  const current = ended.find((c) => c.id === currentId)
  if (current && !recent.includes(current)) recent.push(current)
  return [...live, ...planned, ...recent].map((c) => ({
    key: `conf:${c.id}`,
    label: c.title,
    checked: c.id === currentId,
    note: noteOf(c),
  }))
}

// The meeting a new agenda is offered for by default: the one on now, else the
// nearest planned one still ahead (or with no time), else none.
export function defaultAgendaMeeting(list, now = Date.now()) {
  const rows = list || []
  const live = rows.find((c) => c.status === 'live')
  if (live) return live
  const ahead = rows
    .filter((c) => c.status === 'scheduled' && (!c.scheduled_at || at(c.scheduled_at) >= now))
    .sort((a, b) => (at(a.scheduled_at) || Infinity) - (at(b.scheduled_at) || Infinity))
  return ahead[0] || null
}
