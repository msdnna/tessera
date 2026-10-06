// Saved board views: the bit of naming logic the board's own save path needs, kept
// out of KanbanBoard.vue so it stays unit-testable.
//
// A view is presented as belonging to one visualization — both the load popover and
// the "overwrite:" chip row are filtered by layout, so the two sets read as separate.
// The server key is not: board_views is UNIQUE (board_id, user_id, name) with the
// layout buried in the opaque config blob. A name that looks free in the timeline
// list is therefore still taken by the board view of the same name, and the upsert
// overwrites it — the destroyed view is the one the popover never offered.

const DEFAULT_LAYOUT = 'board'

// The visualization a saved view belongs to. Views predating per-layout sets carry
// no layout in their config and count as board views (same default the board's own
// list filter uses).
export function viewLayout(v) {
  return v?.config?.layout || DEFAULT_LAYOUT
}

// The layout of an existing view that already owns `name` under a *different*
// visualization, or '' when the name is free or merely a same-layout overwrite.
export function conflictingViewLayout(views, name, layout) {
  const wanted = (name || '').trim()
  if (!wanted) return ''
  const here = layout || DEFAULT_LAYOUT
  const clash = (views || []).find((v) => v.name === wanted && viewLayout(v) !== here)
  return clash ? viewLayout(clash) : ''
}
