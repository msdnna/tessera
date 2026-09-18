import { describe, it, expect } from 'vitest'
import {
  sortedColumns,
  columnById,
  nextColumn,
  divergedColumn,
  doneTarget,
  siblingNeighbors,
  moveNeighbors,
} from '@/utils/status'

const COLS = [
  { id: 'c3', name: 'На рассмотрении', color: '#7c5cff', position: 3 },
  { id: 'c1', name: 'К работе', color: '#9aa0aa', position: 1 },
  { id: 'c4', name: 'Готово', color: '#18a058', position: 4 },
  { id: 'c2', name: 'В процессе', color: '#2f80ed', position: 2 },
]

describe('sortedColumns', () => {
  it('orders by position, not by array index', () => {
    expect(sortedColumns(COLS).map((c) => c.id)).toEqual(['c1', 'c2', 'c3', 'c4'])
  })
  it('tolerates empty/missing input', () => {
    expect(sortedColumns([])).toEqual([])
    expect(sortedColumns(null)).toEqual([])
  })
  it('does not mutate the source list', () => {
    const src = [...COLS]
    sortedColumns(src)
    expect(src.map((c) => c.id)).toEqual(['c3', 'c1', 'c4', 'c2'])
  })
})

describe('columnById', () => {
  it('finds a column', () => {
    expect(columnById(COLS, 'c2').name).toBe('В процессе')
  })
  it('returns null for unknown/empty ids', () => {
    expect(columnById(COLS, 'nope')).toBeNull()
    expect(columnById(COLS, null)).toBeNull()
    expect(columnById([], 'c1')).toBeNull()
  })
})

describe('nextColumn', () => {
  it('walks rightwards by position', () => {
    expect(nextColumn(COLS, 'c1').id).toBe('c2')
    expect(nextColumn(COLS, 'c3').id).toBe('c4')
  })
  it('returns null in the last column (shift disabled)', () => {
    expect(nextColumn(COLS, 'c4')).toBeNull()
  })
  it('returns null for an unknown column or an empty board', () => {
    expect(nextColumn(COLS, 'nope')).toBeNull()
    expect(nextColumn([], 'c1')).toBeNull()
  })
})

describe('divergedColumn', () => {
  it('returns the column when the subtask ran ahead of its parent', () => {
    expect(divergedColumn({ column_id: 'c2' }, 'c1', COLS).name).toBe('В процессе')
  })
  it('returns null when subtask and parent agree', () => {
    expect(divergedColumn({ column_id: 'c1' }, 'c1', COLS)).toBeNull()
  })
  it('returns null when either side is unknown', () => {
    expect(divergedColumn({}, 'c1', COLS)).toBeNull()
    expect(divergedColumn({ column_id: 'c2' }, null, COLS)).toBeNull()
    expect(divergedColumn(null, 'c1', COLS)).toBeNull()
  })
  it('returns null when the column was deleted / not loaded', () => {
    expect(divergedColumn({ column_id: 'gone' }, 'c1', COLS)).toBeNull()
    expect(divergedColumn({ column_id: 'c2' }, 'c1', [])).toBeNull()
  })
})

describe('doneTarget', () => {
  it('resolves the board done column', () => {
    expect(doneTarget(COLS, 'c4').name).toBe('Готово')
  })
  it('falls back to null when the board has none', () => {
    expect(doneTarget(COLS, null)).toBeNull()
    expect(doneTarget(COLS, 'gone')).toBeNull()
  })
})

describe('siblingNeighbors', () => {
  const subs = [{ id: 's1' }, { id: 's2' }, { id: 's3' }]
  it('keeps a middle item between its neighbours', () => {
    expect(siblingNeighbors(subs, 's2')).toEqual({ before_id: 's1', after_id: 's3' })
  })
  it('keeps the first item first', () => {
    expect(siblingNeighbors(subs, 's1')).toEqual({ before_id: null, after_id: 's2' })
  })
  it('keeps the last item last', () => {
    expect(siblingNeighbors(subs, 's3')).toEqual({ before_id: 's2', after_id: null })
  })
  it('never uses the moved task as its own neighbour', () => {
    expect(siblingNeighbors([{ id: 's1' }], 's1')).toEqual({ before_id: null, after_id: null })
  })
  it('returns nulls for an unknown id or empty list', () => {
    expect(siblingNeighbors(subs, 'nope')).toEqual({ before_id: null, after_id: null })
    expect(siblingNeighbors([], 's1')).toEqual({ before_id: null, after_id: null })
    expect(siblingNeighbors(null, 's1')).toEqual({ before_id: null, after_id: null })
  })
})

describe('moveNeighbors', () => {
  const subs = [{ id: 's1' }, { id: 's2' }, { id: 's3' }]

  it('names no neighbour for a top-level card — the server appends it', () => {
    expect(moveNeighbors({ id: 't1', column_id: 'c1' }, [])).toEqual({
      before_id: null,
      after_id: null,
    })
  })
  // The regression this replaced: the tail was picked from the board's loaded
  // cards, which under a sprint scope are a slice of the column. Anything the
  // client names here is one card out of that slice, and before_id alone lands
  // the card at prev + 65536 — the next card's slot on an untouched ladder.
  it('names no neighbour even when sibling cards are loaded', () => {
    const loaded = [
      { id: 't2', column_id: 'c2', position: 65536 },
      { id: 't3', column_id: 'c2', position: 131072 },
    ]
    expect(moveNeighbors({ id: 't1', column_id: 'c1' }, loaded)).toEqual({
      before_id: null,
      after_id: null,
    })
  })
  it('pins a subtask to its row in the parent list', () => {
    expect(moveNeighbors({ id: 's2', parent_id: 'p1' }, subs)).toEqual({
      before_id: 's1',
      after_id: 's3',
    })
  })
  it('tolerates a missing task', () => {
    expect(moveNeighbors(null, subs)).toEqual({ before_id: null, after_id: null })
  })
})
