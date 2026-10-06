// "Авто" ordering for the Gantt view: order tasks by their blocking-dependency
// graph instead of the incoming list order.
//
// It's a DFS pre-order over the edges `blocker → blocked`. Roots (tasks with no
// blocker) are visited in the incoming order; each task is emitted before its
// dependents, so a chain A→Б prints A, Б, then the next root (e.g. В) — even
// when В originally sat right after A. Children of a node follow the incoming
// order; anything left over (a cycle) is appended in incoming order, so every
// task appears exactly once.
//
// A task with more than one blocker waits for the LAST of them rather than
// riding out on the first: the walk reaches it once per blocker, and all but the
// final visit turn back. Emitting on the first would put it above a blocker it
// has not met yet (A→В, Б→В gives A, В, Б), and the Gantt draws finish→start
// arrows between rows — so that ordering is read off the chart as an arrow
// running back up it, which is exactly what «Авто» exists to avoid. Cycle
// members have a blocker that is never emitted, so the sweep at the end forces
// the first one out and lets the rest follow normally.
//
// `tasks` — array of objects with an `id`. `deps` — array of { blocker, blocked }
// (the normalised shape the Gantt view already builds from /dependencies). Edges
// touching a task not present in `tasks` are ignored.
export function topoByDeps(tasks, deps) {
  if (!Array.isArray(tasks) || tasks.length === 0) return tasks
  if (!Array.isArray(deps) || deps.length === 0) return tasks

  const order = new Map(tasks.map((t, i) => [t.id, i]))
  const byId = new Map(tasks.map((t) => [t.id, t]))
  const adj = new Map(tasks.map((t) => [t.id, []]))
  const blockedBy = new Map(tasks.map((t) => [t.id, []]))

  for (const e of deps) {
    if (!byId.has(e.blocker) || !byId.has(e.blocked)) continue
    adj.get(e.blocker).push(e.blocked)
    blockedBy.get(e.blocked).push(e.blocker)
  }
  for (const children of adj.values()) children.sort((a, b) => order.get(a) - order.get(b))

  const visited = new Set()
  const out = []
  // `force` bypasses the readiness gate for the leftover sweep only — without it
  // a cycle waits on itself and its members would drop out of the list entirely.
  const visit = (id, force) => {
    if (visited.has(id)) return
    if (!force && !blockedBy.get(id).every((b) => visited.has(b))) return
    visited.add(id)
    out.push(byId.get(id))
    for (const c of adj.get(id)) visit(c, false)
  }
  for (const t of tasks) if (blockedBy.get(t.id).length === 0) visit(t.id, false)
  for (const t of tasks) visit(t.id, true) // remaining cycle members, in incoming order
  return out
}
