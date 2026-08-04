// Board-column helpers shared by the task modal (status row, subtask rows) and
// the kanban card (divergence chip). Pure functions — no DOM, no API.
//
// A board column is `{ id, name, color, position }`; `position` is a float8
// midpoint (see backend positionBetween), so ordering is always by `position`
// and never by array index — the caller's list may come unsorted.

// Columns sorted by their float8 position (stable for equal/missing values).
export function sortedColumns(columns) {
  return [...(columns || [])].sort((a, b) => (a?.position ?? 0) - (b?.position ?? 0))
}

export function columnById(columns, id) {
  if (!id) return null
  return (columns || []).find((c) => c && c.id === id) || null
}

// The column right of `id`, or null when `id` is the last one / unknown.
export function nextColumn(columns, id) {
  const list = sortedColumns(columns)
  const i = list.findIndex((c) => c && c.id === id)
  if (i < 0) return null
  return list[i + 1] || null
}

// The column to show on a subtask that sits in a different column than its
// parent — null when they agree (a chip on every row would be pure noise) or
// when the column is unknown (deleted/not loaded → render nothing, don't throw).
export function divergedColumn(sub, parentColumnId, columns) {
  const id = sub?.column_id
  if (!id || !parentColumnId || id === parentColumnId) return null
  return columnById(columns, id)
}

// Target column for the "close now" checkmark: the board's configured done
// column. Null when the board has none — the caller falls back to the plain
// `completed` flag.
export function doneTarget(columns, doneColumnId) {
  return columnById(columns, doneColumnId)
}

// Neighbours to send with PATCH /tasks/:id/move so the moved task keeps its
// place in the sibling order. Without them the backend's positionBetween(nil,
// nil) returns the constant 65536 and the task jumps within its parent's list.
// `siblings` is the current ordering (as rendered); the moved task itself is
// skipped, so its own position never becomes its own neighbour.
export function siblingNeighbors(siblings, id) {
  const list = (siblings || []).filter((s) => s && s.id !== id)
  const i = (siblings || []).findIndex((s) => s && s.id === id)
  if (i < 0) return { before_id: null, after_id: null }
  const before = list[i - 1] || null
  const after = list[i] || null
  return { before_id: before?.id || null, after_id: after?.id || null }
}
