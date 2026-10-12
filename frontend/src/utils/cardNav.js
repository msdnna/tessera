// Keyboard selection on the kanban board (v2, #2956). Pure, so the rules are
// testable without mounting the board.
//
// `cols` is the board as rendered, left to right:
//   [{ key, ids: [taskId, …], collapsed }]
// A selection is `{ colKey, taskId }` — not the task alone, because in tag
// grouping one task sits in several columns.

function usable(c) {
  return c && !c.collapsed && c.ids.length > 0
}

export function firstCard(cols) {
  const c = cols.find(usable)
  return c ? { colKey: c.key, taskId: c.ids[0] } : null
}

// moveFocus steps the selection one card in `dir` ('up' | 'down' | 'left' |
// 'right'). Up/down stop at the column's ends; left/right skip collapsed and
// empty columns and land on the same row, or the last card if the column is
// shorter. No selection yet (or a stale one) → the first card on the board.
export function moveFocus(cols, focus, dir) {
  const ci = focus ? cols.findIndex((c) => c.key === focus.colKey) : -1
  const ri = ci >= 0 ? cols[ci].ids.indexOf(focus.taskId) : -1
  if (ci < 0 || ri < 0 || cols[ci].collapsed) return firstCard(cols)
  const col = cols[ci]
  if (dir === 'up') return { colKey: col.key, taskId: col.ids[Math.max(0, ri - 1)] }
  if (dir === 'down')
    return { colKey: col.key, taskId: col.ids[Math.min(col.ids.length - 1, ri + 1)] }
  const step = dir === 'left' ? -1 : 1
  for (let i = ci + step; i >= 0 && i < cols.length; i += step) {
    if (!usable(cols[i])) continue
    return { colKey: cols[i].key, taskId: cols[i].ids[Math.min(ri, cols[i].ids.length - 1)] }
  }
  return focus
}

// neighbourColumn is the display column one step left/right of `colKey`
// (collapsed ones included — moving a card into a collapsed column is fine).
export function neighbourColumn(dcols, colKey, dir) {
  const i = dcols.findIndex((d) => d.key === colKey)
  if (i < 0) return null
  return dcols[i + (dir === 'left' ? -1 : 1)] || null
}

// planShift turns «move the card to the next column» into the same intents a
// drag-and-drop produces (see utils/boardDnd.js), per grouping:
//   status    → move to the end of the target column
//   tag       → drop the source column's tag, add the target's
//   milestone → set the target's milestone, or clear it for «без этапа»
export function planShift({ groupMode, from, to, taskId, tailId }) {
  if (!from || !to || from.key === to.key) return []
  if (groupMode === 'status') {
    return [{ op: 'move', id: taskId, columnId: to.key, beforeId: tailId, afterId: null }]
  }
  if (groupMode === 'tag') {
    const out = []
    if (from.tag) out.push({ op: 'removeTag', id: taskId, tagId: from.tag.id })
    if (to.tag) out.push({ op: 'addTag', id: taskId, tagId: to.tag.id })
    return out
  }
  if (groupMode === 'milestone') {
    return to.milestone
      ? [{ op: 'setMilestone', id: taskId, milestoneId: to.milestone.id }]
      : [{ op: 'clearMilestone', id: taskId }]
  }
  return []
}

// planComplete is the «done» checkmark: reopen a completed task; otherwise
// move it into the board's done column (the backend stamps completed_at and
// logs it); boards without a done column fall back to the completed flag.
export function planComplete({ completed, columnId, doneColumnId }) {
  if (completed) return { op: 'uncomplete' }
  if (doneColumnId && doneColumnId !== columnId) return { op: 'move', columnId: doneColumnId }
  return { op: 'complete' }
}
