import { describe, it, expect } from 'vitest'
import {
  sortedColumns,
  columnById,
  nextColumn,
  divergedColumn,
  doneTarget,
  siblingNeighbors,
  columnTail,
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

describe('columnTail', () => {
  const tasks = [
    { id: 't1', column_id: 'c1', position: 65536 },
    { id: 't2', column_id: 'c2', position: 131072 },
    { id: 't3', column_id: 'c2', position: 196608 },
    { id: 't4', column_id: 'c2', position: 98304 },
  ]
  it('picks the highest position in the target column, not the last in the array', () => {
    expect(columnTail(tasks, 'c2', 't1')).toBe('t3')
  })
  it('ignores the moved task itself', () => {
    expect(columnTail(tasks, 'c2', 't3')).toBe('t2')
  })
  it('returns null for an empty target column (task lands first)', () => {
    expect(columnTail(tasks, 'c3', 't1')).toBeNull()
    expect(columnTail([{ id: 't1', column_id: 'c1' }], 'c1', 't1')).toBeNull()
  })
  it('tolerates missing input', () => {
    expect(columnTail(null, 'c1', 't1')).toBeNull()
    expect(columnTail(tasks, null, 't1')).toBeNull()
  })
  it('treats a missing position as the lowest', () => {
    const partial = [
      { id: 'a', column_id: 'c1' },
      { id: 'b', column_id: 'c1', position: 5 },
    ]
    expect(columnTail(partial, 'c1', null)).toBe('b')
  })
})
