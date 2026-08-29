// Column header counts (#2850). The header used to show only the number of cards
// in the column; it now also shows the total with every subtask folded in, so the
// reader sees how much work the column really holds.
//
// `subtasksByParent` is the board's flat parent_id → children map (all nesting
// levels at once, as the board loads it), so the walk has to recurse and guard
// against a parent chain that loops back on itself — a cycle would otherwise hang
// the render.

/**
 * Count a column's cards and its full task tree.
 *
 * @param {Array<{id: string}>} tasks top-level cards of the column
 * @param {Object<string, Array<{id: string}>>} subtasksByParent parent id → children
 * @returns {{tasks: number, total: number}} cards, and cards + all their subtasks
 */
export function countWithSubtasks(tasks, subtasksByParent) {
  const list = Array.isArray(tasks) ? tasks : []
  const byParent = subtasksByParent || {}
  const seen = new Set()
  let count = 0
  let total = 0

  const walk = (id) => {
    for (const child of byParent[id] || []) {
      if (!child || !child.id || seen.has(child.id)) continue
      seen.add(child.id)
      total += 1
      walk(child.id)
    }
  }

  for (const t of list) {
    if (!t || !t.id || seen.has(t.id)) continue
    seen.add(t.id)
    count += 1
    total += 1
    walk(t.id)
  }
  return { tasks: count, total }
}
