// Which cards stand in each kanban lane (#2932).
//
// Pulled out of KanbanBoard.vue as a pure function so the rule that matters here can
// be pinned without mounting the board: EVERY lane the board draws must come back
// with a bucket, even an empty one. `lists[dcol.key]` is handed straight to
// `<draggable :list="…">`, and vuedraggable's `list` prop defaults to null — an
// undefined bucket therefore turns into a draggable whose `realList` is null, and
// its `_manage` wrapper drops every drag callback on the floor. The column still
// accepts the card visually (it is a live drop target), no `change` event fires, no
// request is sent, and the next reload puts the card back where it was: a drop that
// silently did nothing.
//
// That is what the board actually did with a lane born between task fetches. The
// buckets were rebuilt only when the TASKS changed, while the lanes themselves come
// from the columns, the project's tags and its milestones — all three of which the
// realtime layer re-reads on their own (a `column.*` event refetches just the
// columns, a `tag.*` event just the workspace meta). A colleague adding a column, or
// a tag in the tag grouping — the board's killer feature — drew the new lane at once
// and left it undroppable until something unrelated touched the cards.

// The catch-all lane: no milestone, or no tag from the grouping set. Same string as
// facetKeys' NO_MILESTONE — the filter chips and the lanes have to agree on it.
export const NO_LANE = '__none__'

/**
 * Key under which a lane's explicit collapsed/expanded override is stored.
 *
 * Lane keys are ids and unique across the board — except NO_LANE, which every
 * non-status grouping reuses for its catch-all lane: «Без этапа» when grouping by
 * milestone, «Без тегов» when grouping by tag. Those are different lanes holding
 * different cards, and the collapse overrides live in ONE per-board blob that
 * switching the grouping does not reset. Sharing the key therefore leaks the
 * gesture across modes: collapsing the noisy «Без этапа» lane leaves «Без тегов»
 * — usually the fullest lane on a tag-grouped board, the killer feature's own
 * fallback bucket — drawn as a narrow strip the moment the reader flips the
 * grouping, with no gesture of their own. Only the catch-all is namespaced: real
 * lane ids already say which lane they are, and prefixing them too would throw
 * away every collapse a reader has saved.
 *
 * @param {string} groupMode 'status' | 'milestone' | anything else = tag grouping
 * @param {string} laneKey the lane's `dcol.key`
 * @returns {string} storage key for that lane's override
 */
export function collapseKey(groupMode, laneKey) {
  return laneKey === NO_LANE ? `${groupMode}:${NO_LANE}` : laneKey
}

/**
 * Bucket the board's cards by lane key.
 *
 * @param {Object} o
 * @param {string} o.groupMode 'status' | 'milestone' | anything else = tag grouping
 * @param {Array} [o.columns] board columns (status grouping)
 * @param {Array} [o.milestones] project milestones (milestone grouping)
 * @param {Array} [o.groupTags] the tags that become lanes (tag grouping)
 * @param {Array} [o.tasks] the cards to place, already filtered
 * @returns {Object} lane key → cards
 */
export function buildLists({
  groupMode,
  columns = [],
  milestones = [],
  groupTags = [],
  tasks = [],
}) {
  const map = {}
  if (groupMode === 'status') {
    for (const col of columns) map[col.id] = []
    // A card whose column is not among the board's own gets a bucket of its own
    // rather than being folded into a lane it doesn't belong to. Nothing draws that
    // bucket, which is the point: it is the card that is wrong, not the lane.
    for (const t of tasks) (map[t.column_id] ||= []).push(t)
    return map
  }
  if (groupMode === 'milestone') {
    const known = new Set(milestones.map((m) => m.id))
    for (const m of milestones) map[m.id] = []
    map[NO_LANE] = []
    for (const t of tasks) {
      if (t.milestone_id && known.has(t.milestone_id)) map[t.milestone_id].push(t)
      else map[NO_LANE].push(t)
    }
    return map
  }
  // Tag grouping: a card carrying two of the grouping tags stands in both lanes.
  const known = new Set(groupTags.map((tg) => tg.id))
  for (const tg of groupTags) map[tg.id] = []
  map[NO_LANE] = []
  for (const t of tasks) {
    const own = (t.tag_ids || []).filter((id) => known.has(id))
    if (!own.length) map[NO_LANE].push(t)
    else for (const id of own) map[id].push(t)
  }
  return map
}
