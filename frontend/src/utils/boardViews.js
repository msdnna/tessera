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

// The list's field columns (#3020), left to right after the title, and their
// widths from the mockup (ListView: приоритет 112, срок 92, люди 92, оценка 62,
// теги 150). The reader drags a column wider or narrower; the widths are part of
// the board's state and of a saved view.
export const LIST_FIELDS = ['priority', 'due', 'people', 'estimate', 'tags']
export const LIST_WIDTHS = Object.freeze({
  priority: 112,
  due: 92,
  people: 92,
  estimate: 62,
  tags: 150,
})
export const LIST_WIDTH_MIN = 48
export const LIST_WIDTH_MAX = 360
export function clampListWidth(w) {
  return Math.round(Math.min(LIST_WIDTH_MAX, Math.max(LIST_WIDTH_MIN, w)))
}
export function listConfig(l) {
  const widths = {}
  for (const k of LIST_FIELDS) {
    const w = l?.widths?.[k]
    widths[k] = Number.isFinite(w) ? clampListWidth(w) : LIST_WIDTHS[k]
  }
  return { widths }
}

// The calendar's mode (#3019): the month grid, or a week of full-height days.
export const CALENDAR_MODES = ['month', 'week']
export function calendarConfig(c) {
  return { mode: CALENDAR_MODES.includes(c?.mode) ? c.mode : 'month' }
}

// The timeline / Gantt «Шкала» (#3021): the scale preset and what the chart
// draws on top of the bars. The defaults keep what the chart has always drawn —
// weekends, the today line, progress, the Gantt's links and the undated strip.
export const CHART_SCALES = ['days', 'weeks', 'months', 'quarters']
export const CHART_TOGGLES = ['weekends', 'today', 'progress', 'links', 'undated']
export const CHART_DEFAULTS = Object.freeze({
  scale: 'days',
  weekends: true,
  today: true,
  progress: true,
  links: true,
  undated: true,
})
export function chartConfig(c) {
  const out = { scale: CHART_SCALES.includes(c?.scale) ? c.scale : CHART_DEFAULTS.scale }
  for (const k of CHART_TOGGLES) out[k] = typeof c?.[k] === 'boolean' ? c[k] : CHART_DEFAULTS[k]
  return out
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
    list: listConfig(c?.list),
    calendar: calendarConfig(c?.calendar),
    chart: chartConfig(c?.chart),
  }
}

export function sameViewConfig(a, b) {
  return JSON.stringify(normalizeViewConfig(a)) === JSON.stringify(normalizeViewConfig(b))
}

// What differs between the loaded view and the board now, as a list of areas
// for the «изменено» note: [{ area, value? }] in a fixed order.
//   area ∈ layout | group | sort | filters | cards | columns | matrix | list |
//          calendar | scale
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
  if (differ('list')) out.push({ area: 'list' })
  if (differ('calendar')) out.push({ area: 'calendar' })
  if (differ('chart')) out.push({ area: 'scale' })
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
