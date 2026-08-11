// Kanban drop-persistence rules.
//
// When a card or a column is dropped, `vuedraggable` hands us a Sortable change
// event (`{ added | moved | removed: { element, newIndex } }`). Turning that into
// the right API mutation carries seven fiddly rules — neighbour math, a collapsed
// destination, a subtask dragged out, milestone being single-value, tag add/remove,
// column reorder only in status mode. That logic used to live inline in the
// 2.5k-line KanbanBoard and was untestable without mounting the whole board
// (#2670). It moves here as pure functions: the component keeps only the thin
// `await api…` dispatch, and every rule is characterised by a unit test.
//
// These functions never touch the network or reactive state — callers pass the
// current list snapshot and collapsed flag in, and get back a plain description
// of what to persist.

// planColumnReorder resolves a Sortable column-reorder event into the move it
// implies: which column key moved, and its new left/right neighbour keys. Column
// reorder is meaningful only in status mode; any other mode (tag/milestone
// grouping) returns null, as does an event with no moved/added payload.
export function planColumnReorder(evt, colModel, groupMode) {
  if (groupMode !== 'status') return null
  const info = evt.moved || evt.added
  if (!info) return null
  const before = colModel[info.newIndex - 1]
  const after = colModel[info.newIndex + 1]
  return {
    key: info.element.key,
    before_id: before ? before.key : null,
    after_id: after ? after.key : null,
  }
}

// planColDrop turns a Sortable change event on a destination column into the
// ordered list of task mutations it implies. `groupMode` selects the rule set:
//
//   status    — reposition the card. `before`/`after` come from newIndex, EXCEPT
//               on a collapsed column where newIndex is meaningless (its cards are
//               hidden behind the drop overlay): pin to the top instead
//               (before = nothing, after = the first card that isn't the dropped
//               one). A subtask dragged out onto a column is promoted to
//               top-level (setParent → null) before the move.
//   milestone — single-value: the destination's `added` sets or clears the
//               milestone; the source's `removed` is ignored (the new value wins).
//   tag       — `added` adds the column's tag, `removed` removes it.
//
// Returns an array of intents (possibly empty). Each intent is `{ op, ... }`; the
// component maps ops to API calls. Keeping this a list (not direct calls) is what
// makes the setParent-then-move ordering and the collapsed/subtask branches
// assertable in isolation.
export function planColDrop({ groupMode, evt, dcol, list = [], collapsed = false }) {
  const intents = []
  if (groupMode === 'status') {
    const info = evt.added || evt.moved
    if (!info) return intents
    const before = collapsed ? null : list[info.newIndex - 1]
    const after = collapsed
      ? list.find((t) => t.id !== info.element.id)
      : list[info.newIndex + 1]
    // A subtask dragged out onto a column becomes top-level again — before moving.
    if (evt.added && info.element.parent_id) {
      intents.push({ op: 'setParent', id: info.element.id, parentId: null })
    }
    intents.push({
      op: 'move',
      id: info.element.id,
      columnId: dcol.key,
      beforeId: before ? before.id : null,
      afterId: after ? after.id : null,
    })
  } else if (groupMode === 'milestone') {
    if (evt.added) {
      const id = evt.added.element.id
      if (dcol.milestone) intents.push({ op: 'setMilestone', id, milestoneId: dcol.milestone.id })
      else intents.push({ op: 'clearMilestone', id })
    }
  } else {
    if (evt.added && dcol.tag) intents.push({ op: 'addTag', id: evt.added.element.id, tagId: dcol.tag.id })
    if (evt.removed && dcol.tag)
      intents.push({ op: 'removeTag', id: evt.removed.element.id, tagId: dcol.tag.id })
  }
  return intents
}
