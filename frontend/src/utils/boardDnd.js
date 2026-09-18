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

// May a card be re-ordered INSIDE a lane, or only dragged between lanes?
//
// Only status columns own their cards' order. Card order comes from `position`,
// which is per COLUMN — and a tag/milestone lane collects cards out of every
// column of the board, so the neighbours a card lands between there are not its
// neighbours in the column `position` orders it in. There is no `move` those two
// indices could be turned into: `planColDrop` has no rule for a bare `moved`
// outside status mode, and returns no intents at all for one.
//
// The lanes stayed sortable anyway, so the gesture *looked* like it worked:
// Sortable re-ordered the lane on drop, `onColChange` dispatched nothing, and the
// debounced reload it schedules rebuilt the lists ~200 ms later and put the card
// back where it started — no request, no error, no explanation, and the same drag
// one grouping over (status) persists. Cross-lane drops are untouched: those carry
// `added`/`removed`, which DO mean something here (set the milestone, add the tag).
export function canSortWithinLane(groupMode) {
  return groupMode === 'status'
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
//   tag       — `added` adds the column's tag, `removed` removes it. The catch-all
//               lane («Без тегов») carries no tag of its own, so a drop there means
//               the mirror image: strip the GROUPING tags the card holds — see
//               clearLaneTags.
//
// Neither non-status rule set has anything to say about a bare `moved` (a re-order
// inside one lane) — see canSortWithinLane above, which is what keeps the board
// from offering that gesture in the first place.
//
// Returns an array of intents (possibly empty). Each intent is `{ op, ... }`; the
// component maps ops to API calls. Keeping this a list (not direct calls) is what
// makes the setParent-then-move ordering and the collapsed/subtask branches
// assertable in isolation.
export function planColDrop({
  groupMode,
  evt,
  dcol,
  list = [],
  collapsed = false,
  groupTags = [],
}) {
  const intents = []
  if (groupMode === 'status') {
    const info = evt.added || evt.moved
    if (!info) return intents
    const before = collapsed ? null : list[info.newIndex - 1]
    const after = collapsed ? list.find((t) => t.id !== info.element.id) : list[info.newIndex + 1]
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
    if (evt.added) {
      if (dcol.tag) intents.push({ op: 'addTag', id: evt.added.element.id, tagId: dcol.tag.id })
      else intents.push(...clearLaneTags(evt.added.element, groupTags))
    }
    // The catch-all lane has no tag to take off a card LEAVING it: the card left
    // because the destination lane gave it one, which that lane's `added` persists.
    if (evt.removed && dcol.tag)
      intents.push({ op: 'removeTag', id: evt.removed.element.id, tagId: dcol.tag.id })
  }
  return intents
}

// Dropping a card into the tag grouping's catch-all lane («Без тегов»).
//
// That lane holds the cards carrying none of the lane tags (utils/boardLists'
// NO_LANE bucket), and `dcol.tag` is null for it — so the rule above, which only
// knew the lane's own tag, produced NO intent at all. The lane is a live drop
// target all the same (every lane renders the same `group="tasks"` draggable), so
// the card visibly moved there, nothing was requested, and the debounced reload put
// it back ~200 ms later: the phantom gesture canSortWithinLane was written for, in
// the one grouping where the drop does mean something. Milestone grouping's own
// catch-all has always persisted the mirror («Без этапа» → clearMilestone).
//
// Only tags that ARE lanes come off. With the grouping narrowed to a namespace
// («Тег · Сложность») the lane means "carries no сложность:: tag", so the card's
// other tags — invisible to this grouping — have to survive the drop.
//
// The lane a card is dragged OUT of emits its own `removed`, so a card standing in
// two tag lanes has one of its removals planned twice. That is harmless: the API's
// detach is a bare DELETE and takes a missing row as done.
function clearLaneTags(element, groupTags) {
  const lane = new Set((groupTags || []).map((tg) => tg && tg.id).filter(Boolean))
  return (element.tag_ids || [])
    .filter((id) => lane.has(id))
    .map((tagId) => ({ op: 'removeTag', id: element.id, tagId }))
}
