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
// `tasks` — array of objects with an `id`. `deps` — array of { blocker, blocked }
// (the normalised shape the Gantt view already builds from /dependencies). Edges
// touching a task not present in `tasks` are ignored.
export function topoByDeps(tasks, deps) {
  if (!Array.isArray(tasks) || tasks.length === 0) return tasks
  if (!Array.isArray(deps) || deps.length === 0) return tasks

  const order = new Map(tasks.map((t, i) => [t.id, i]))
  const byId = new Map(tasks.map((t) => [t.id, t]))
  const adj = new Map(tasks.map((t) => [t.id, []]))
  const indeg = new Map(tasks.map((t) => [t.id, 0]))

  for (const e of deps) {
    if (!byId.has(e.blocker) || !byId.has(e.blocked)) continue
    adj.get(e.blocker).push(e.blocked)
    indeg.set(e.blocked, indeg.get(e.blocked) + 1)
  }
  for (const children of adj.values()) children.sort((a, b) => order.get(a) - order.get(b))

  const visited = new Set()
  const out = []
  const visit = (id) => {
    if (visited.has(id)) return
    visited.add(id)
    out.push(byId.get(id))
    for (const c of adj.get(id)) visit(c)
  }
  for (const t of tasks) if (indeg.get(t.id) === 0) visit(t.id)
  for (const t of tasks) visit(t.id) // remaining cycle members, in incoming order
  return out
}
