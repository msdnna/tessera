// Board views (#2970): the pure half — the canonical config the «изменено» state
// compares, the one ordered list, the default view, names per list, and the
// neighbours a move or an undone delete lands between.
import { describe, it, expect } from 'vitest'
import {
  viewLayout,
  normalizeViewConfig,
  sameViewConfig,
  viewChanges,
  orderViews,
  defaultView,
  viewNameTaken,
  moveNeighbours,
  neighboursOf,
  matrixConfig,
  MATRIX_DEFAULTS,
  LAYOUTS,
} from '@/utils/boardViews'

describe('viewLayout', () => {
  it('reads the layout a view was saved in, kanban for old views', () => {
    expect(viewLayout({ config: { layout: 'gantt' } })).toBe('gantt')
    expect(viewLayout({ config: {} })).toBe('board')
    expect(viewLayout(null)).toBe('board')
  })

  it('lists the six views in the order of the switch', () => {
    expect(LAYOUTS).toEqual(['board', 'list', 'timeline', 'gantt', 'calendar', 'matrix'])
  })
})

describe('normalizeViewConfig / sameViewConfig', () => {
  it('fills defaults so an old sparse view equals the same full state', () => {
    const sparse = { layout: 'board', groupMode: 'status' }
    const full = normalizeViewConfig({})
    expect(sameViewConfig(sparse, full)).toBe(true)
    expect(full.cardSize).toBe('medium')
    expect(full.showEmpty).toBe(true)
    expect(full.fieldVis.priority).toBe(true)
  })

  it('defaults the grouping per layout: swimlanes by assignee on the timelines', () => {
    expect(normalizeViewConfig({ layout: 'gantt' }).groupMode).toBe('assignee')
    expect(normalizeViewConfig({ layout: 'list' }).groupMode).toBe('status')
  })

  it('ignores the header search and key order', () => {
    const a = { filters: { q: 'релиз', tags: ['t1'] }, colCollapse: { b: true, a: false } }
    const b = { colCollapse: { a: false, b: true }, filters: { tags: ['t1'] } }
    expect(sameViewConfig(a, b)).toBe(true)
  })

  it('reads the legacy single sort as a sort level', () => {
    expect(normalizeViewConfig({ sortBy: 'due', sortDir: 'desc' }).sortLevels).toEqual([
      { field: 'due', dir: 'desc' },
    ])
    expect(normalizeViewConfig({ sortBy: 'position' }).sortLevels).toEqual([])
  })

  it('sees a real difference', () => {
    expect(sameViewConfig({ cardSize: 'medium' }, { cardSize: 'large' })).toBe(false)
    expect(sameViewConfig({ layout: 'board' }, { layout: 'list' })).toBe(false)
  })
})

describe('viewChanges', () => {
  it('names what moved since the view was loaded, in a fixed order', () => {
    const saved = { layout: 'board', groupMode: 'status' }
    const now = {
      layout: 'board',
      groupMode: 'tag',
      filters: { priorities: [3] },
      cardSize: 'compact',
    }
    expect(viewChanges(saved, now)).toEqual([
      { area: 'group', value: 'tag' },
      { area: 'filters' },
      { area: 'cards' },
    ])
  })

  it('reports a layout switch with the new layout', () => {
    expect(viewChanges({ layout: 'board' }, { layout: 'gantt', groupMode: 'assignee' })).toEqual([
      { area: 'layout', value: 'gantt' },
      { area: 'group', value: 'assignee' },
    ])
  })

  it('is empty for an unchanged view', () => {
    expect(viewChanges({ layout: 'list' }, { layout: 'list', filters: { q: 'x' } })).toEqual([])
  })
})

const V = (id, scope, position, extra = {}) => ({ id, name: id, scope, position, ...extra })

describe('orderViews', () => {
  it('puts own views first, then the shared ones, each by position', () => {
    const list = [V('s2', 'board', 2), V('m2', 'user', 2), V('s1', 'board', 1), V('m1', 'user', 1)]
    expect(orderViews(list).map((v) => v.id)).toEqual(['m1', 'm2', 's1', 's2'])
  })
})

describe('defaultView', () => {
  it('prefers the reader’s own star over the shared one', () => {
    const list = [V('s', 'board', 1, { is_default: true }), V('m', 'user', 1, { is_default: true })]
    expect(defaultView(list).id).toBe('m')
  })

  it('falls back to the shared star, else none', () => {
    expect(defaultView([V('s', 'board', 1, { is_default: true })]).id).toBe('s')
    expect(defaultView([V('m', 'user', 1)])).toBeNull()
  })
})

describe('viewNameTaken', () => {
  const list = [V('Спринт', 'user', 1), V('Команда', 'board', 1)]

  it('checks the list the view would join, case-insensitively', () => {
    expect(viewNameTaken(list, ' спринт ', 'user')).toBe(true)
    expect(viewNameTaken(list, 'Спринт', 'board')).toBe(false)
    expect(viewNameTaken(list, 'Команда', 'board')).toBe(true)
  })

  it('lets a view keep its own name', () => {
    expect(viewNameTaken(list, 'Спринт', 'user', 'Спринт')).toBe(false)
  })
})

describe('moveNeighbours / neighboursOf', () => {
  const list = [V('a', 'user', 1), V('b', 'user', 2), V('c', 'user', 3), V('s', 'board', 1)]

  it('moves one step inside its own list', () => {
    expect(moveNeighbours(list, 'b', -1)).toEqual({ before_id: null, after_id: 'a' })
    expect(moveNeighbours(list, 'b', 1)).toEqual({ before_id: 'c', after_id: null })
    expect(moveNeighbours(list, 'a', 1)).toEqual({ before_id: 'b', after_id: 'c' })
  })

  it('stays put at the ends and never crosses into the other list', () => {
    expect(moveNeighbours(list, 'a', -1)).toBeNull()
    expect(moveNeighbours(list, 'c', 1)).toBeNull()
    expect(moveNeighbours(list, 's', -1)).toBeNull()
  })

  it('remembers where a view sat, for an undone delete', () => {
    expect(neighboursOf(list, 'b')).toEqual({ before_id: 'a', after_id: 'c' })
    expect(neighboursOf(list, 's')).toEqual({ before_id: null, after_id: null })
  })
})

describe('matrixConfig (#3018)', () => {
  it('defaults to ≤ 3 days and ≥ high', () => {
    expect(matrixConfig()).toEqual(MATRIX_DEFAULTS)
    expect(MATRIX_DEFAULTS).toEqual({ dueDays: 3, minPriority: 3 })
  })

  it('keeps valid thresholds and drops nonsense', () => {
    expect(matrixConfig({ dueDays: 7, minPriority: 4 })).toEqual({ dueDays: 7, minPriority: 4 })
    expect(matrixConfig({ dueDays: -1, minPriority: 9 })).toEqual(MATRIX_DEFAULTS)
  })
})
