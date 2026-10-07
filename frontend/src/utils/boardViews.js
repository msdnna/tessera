// Board views (v2, #2970): the pure half of the saved-view machinery, kept out
// of KanbanBoard so it stays unit-testable.
//
// A view is one list per board — the reader's own views, then the board's
// shared ones — each carrying the layout it was saved in; applying it switches
// the board to that layout. A loaded view is «изменено» as soon as the board's
// state differs from what the view stores: `normalizeViewConfig` reduces both
// sides to the same canonical shape (defaults filled, the header search and
// legacy keys dropped) so the comparison sees only real differences.
import { defaultFieldVis } from '@/stores/boardView'
import { cloneFilters } from '@/utils/facetKeys'

const DEFAULT_LAYOUT = 'board'

// Every view kind the board draws, in the order of the view switch (#2970: six
// equal segments, V then 1–6).
export const LAYOUTS = ['board', 'list', 'timeline', 'gantt', 'calendar', 'matrix']
export const LAYOUT_ICONS = {
  board: 'layout-kanban',
  list: 'layout-list',
  timeline: 'layout-timeline',
  gantt: 'layout-gantt',
  calendar: 'layout-calendar',
  matrix: 'layout-matrix',
}

// The Eisenhower matrix thresholds (#3018): a card is urgent when due within
// `dueDays` days (or overdue), important from `minPriority` up. Defaults from
// the mockup: ≤ 3 days, ≥ high.
export const MATRIX_DEFAULTS = Object.freeze({ dueDays: 3, minPriority: 3 })
export function matrixConfig(m) {
  const dueDays = Number.isInteger(m?.dueDays) && m.dueDays >= 0 ? m.dueDays : null
  const minPriority =
    Number.isInteger(m?.minPriority) && m.minPriority >= 1 && m.minPriority <= 4
      ? m.minPriority
      : null
  return {
    dueDays: dueDays ?? MATRIX_DEFAULTS.dueDays,
    minPriority: minPriority ?? MATRIX_DEFAULTS.minPriority,
  }
}

// The visualization a saved view belongs to. Views predating per-layout sets carry
// no layout in their config and count as board views.
export function viewLayout(v) {
  return v?.config?.layout || DEFAULT_LAYOUT
}

export function defaultGroupMode(layout) {
  return layout === 'timeline' || layout === 'gantt' ? 'assignee' : 'status'
}

function sortedObject(o) {
  const out = {}
  for (const k of Object.keys(o || {}).sort()) out[k] = o[k]
  return out
}

// The canonical form of a view config — what a view stores and what the board's
// live state is compared against.
export function normalizeViewConfig(c) {
  const layout = c?.layout || DEFAULT_LAYOUT
  let sortLevels = []
  if (Array.isArray(c?.sortLevels)) {
    sortLevels = c.sortLevels.map((l) => ({
      field: l.field,
      dir: l.dir === 'desc' ? 'desc' : 'asc',
    }))
  } else if (c?.sortBy && c.sortBy !== 'position') {
    sortLevels = [{ field: c.sortBy, dir: c.sortDir === 'desc' ? 'desc' : 'asc' }]
  }
  const filters = cloneFilters(c?.filters)
  // The header's title search is the reader's moment, not part of a view.
  filters.q = ''
  return {
    layout,
    groupMode: c?.groupMode || defaultGroupMode(layout),
    tagPrefix: c?.tagPrefix || '',
    sortLevels,
    subtasksExpanded: !!c?.subtasksExpanded,
    autoSort: !!c?.autoSort,
    filters,
    colCollapse: sortedObject(c?.colCollapse),
    autoCollapseEmpty: !!c?.autoCollapseEmpty,
    cardSize: c?.cardSize || 'medium',
    stackFields: !!c?.stackFields,
    showEmpty: c?.showEmpty !== false,
    fieldVis: sortedObject({ ...defaultFieldVis(), ...(c?.fieldVis || {}) }),
    matrix: matrixConfig(c?.matrix),
  }
}

export function sameViewConfig(a, b) {
  return JSON.stringify(normalizeViewConfig(a)) === JSON.stringify(normalizeViewConfig(b))
}

// What differs between the loaded view and the board now, as a list of areas
// for the «изменено» note: [{ area, value? }] in a fixed order.
//   area ∈ layout | group | sort | filters | cards | columns | matrix
export function viewChanges(saved, now) {
  const a = normalizeViewConfig(saved)
  const b = normalizeViewConfig(now)
  const out = []
  const differ = (k) => JSON.stringify(a[k]) !== JSON.stringify(b[k])
  if (a.layout !== b.layout) out.push({ area: 'layout', value: b.layout })
  if (a.groupMode !== b.groupMode || a.tagPrefix !== b.tagPrefix)
    out.push({ area: 'group', value: b.groupMode })
  if (differ('sortLevels')) out.push({ area: 'sort' })
  if (differ('filters')) out.push({ area: 'filters' })
  if (
    a.cardSize !== b.cardSize ||
    a.stackFields !== b.stackFields ||
    a.showEmpty !== b.showEmpty ||
    differ('fieldVis')
  )
    out.push({ area: 'cards' })
  if (
    differ('colCollapse') ||
    a.autoCollapseEmpty !== b.autoCollapseEmpty ||
    a.subtasksExpanded !== b.subtasksExpanded ||
    a.autoSort !== b.autoSort
  )
    out.push({ area: 'columns' })
  if (differ('matrix')) out.push({ area: 'matrix' })
  return out
}

// The board's one list: own views first, then the shared ones, each in its own
// order. Index + 1 is the view's key (1–9).
export function orderViews(views) {
  const list = [...(views || [])]
  const rank = (v) => (v.scope === 'board' ? 1 : 0)
  return list.sort(
    (x, y) =>
      rank(x) - rank(y) ||
      (x.position ?? 0) - (y.position ?? 0) ||
      String(x.name).localeCompare(String(y.name)),
  )
}

// The view that opens by default (#3015): the reader's own star wins over the
// board's shared one.
export function defaultView(views) {
  const list = views || []
  return (
    list.find((v) => v.is_default && v.scope !== 'board') ||
    list.find((v) => v.is_default && v.scope === 'board') ||
    null
  )
}

// Is `name` already taken in the list a view of `scope` would join? Names are
// unique per list (own / shared), not per layout.
export function viewNameTaken(views, name, scope = 'user', exceptId = null) {
  const wanted = (name || '').trim().toLowerCase()
  if (!wanted) return false
  return (views || []).some(
    (v) =>
      v.id !== exceptId &&
      (v.scope || 'user') === scope &&
      String(v.name).trim().toLowerCase() === wanted,
  )
}

// Neighbours for moving a view one step up/down inside its own list:
// { before_id, after_id } in the API's sense (the view lands between them), or
// null when it is already at that end.
export function moveNeighbours(views, id, dir) {
  const me = (views || []).find((v) => v.id === id)
  if (!me) return null
  const list = orderViews(views).filter((v) => (v.scope || 'user') === (me.scope || 'user'))
  const i = list.findIndex((v) => v.id === id)
  const j = i + dir
  if (j < 0 || j >= list.length) return null
  // In the list without the view, it goes in at index j.
  const rest = list.filter((v) => v.id !== id)
  const prev = rest[j - 1] || null
  const next = rest[j] || null
  return { before_id: prev?.id || null, after_id: next?.id || null }
}

// Neighbours of a view inside its list now — where an undone delete goes back.
export function neighboursOf(views, id) {
  const me = (views || []).find((v) => v.id === id)
  if (!me) return { before_id: null, after_id: null }
  const list = orderViews(views).filter((v) => (v.scope || 'user') === (me.scope || 'user'))
  const i = list.findIndex((v) => v.id === id)
  return { before_id: list[i - 1]?.id || null, after_id: list[i + 1]?.id || null }
}
