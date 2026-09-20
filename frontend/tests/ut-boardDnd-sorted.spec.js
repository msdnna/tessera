// Card drag-reorder under an ACTIVE COMPOSER SORT (#2932).
//
// The same phantom gesture `canSortWithinLane` was written for, one axis over.
// There the lane had no stored order to write back; here the order on screen is
// simply not the stored one: `lists` is built from `filteredTasks`, which is
// `sortByLevels(...)`, so with «Сортировка: Приоритет ↓» the cards in a status
// column are drawn by priority while `position` still orders them the old way.
//
// The board kept offering the drag anyway (`canSortCards` asked only about the
// grouping), and the neighbours handed to PATCH /tasks/:id/move were read off the
// SORTED list. Their `position` values are in arbitrary relative order there, so
// the backend's midpoint (handlers/api.go positionBetween) landed the card at a
// random slot of the stored ladder — and `positionExhausted`, which only fires on
// a collapsed slot, has nothing to say about two neighbours handed over swapped.
// On screen nothing happened: the reload re-sorted and put the card back. The
// damage showed up later, for everyone, the moment the sort was turned off.
//
// A CROSS-COLUMN drop keeps working: the column change is real and has to persist.
// Only the slot is dropped — `before_id: null, after_id: null` is the shape the
// context menu's «Переместить → колонка» already sends, and the server appends
// (handlers/tasks.go positionForUnanchoredMove).
import { describe, it, expect } from 'vitest'
import { planColDrop, canSortWithinLane } from '@/utils/boardDnd'

const col = (over = {}) => ({ key: 'colX', ...over })
const list = [{ id: 't1' }, { id: 't2' }, { id: 't3' }]

describe('canSortWithinLane — a manual sort owns the order too', () => {
  it('refuses re-ordering inside a status column while a sort is active', () => {
    expect(canSortWithinLane('status', true)).toBe(false)
  })

  it('still allows it in a status column with no sort', () => {
    expect(canSortWithinLane('status', false)).toBe(true)
  })

  it('defaults to "no sort" so the existing callers keep their answer', () => {
    expect(canSortWithinLane('status')).toBe(true)
  })

  it('leaves the grouping rule alone — a lane is refused either way', () => {
    expect(canSortWithinLane('tag', false)).toBe(false)
    expect(canSortWithinLane('tag', true)).toBe(false)
    expect(canSortWithinLane('milestone', true)).toBe(false)
  })
})

describe('planColDrop — status mode under an active sort', () => {
  it('plans nothing for a re-order inside the column', () => {
    // Nothing to persist: the two cards it landed between are neighbours in the
    // sort, not in `position`.
    const evt = { moved: { element: { id: 't2' }, newIndex: 2 } }
    expect(planColDrop({ groupMode: 'status', evt, dcol: col(), list, sorted: true })).toEqual([])
  })

  it('but persists it with neighbours when no sort is active', () => {
    const evt = { moved: { element: { id: 't2' }, newIndex: 2 } }
    expect(planColDrop({ groupMode: 'status', evt, dcol: col(), list, sorted: false })).toEqual([
      { op: 'move', id: 't2', columnId: 'colX', beforeId: 't2', afterId: null },
    ])
  })

  it('persists a cross-column drop, but leaves the slot to the server', () => {
    const evt = { added: { element: { id: 'tX' }, newIndex: 1 } }
    expect(planColDrop({ groupMode: 'status', evt, dcol: col(), list, sorted: true })).toEqual([
      { op: 'move', id: 'tX', columnId: 'colX', beforeId: null, afterId: null },
    ])
  })

  it('and names the neighbours for the same drop with no sort', () => {
    const evt = { added: { element: { id: 'tX' }, newIndex: 1 } }
    expect(planColDrop({ groupMode: 'status', evt, dcol: col(), list, sorted: false })).toEqual([
      { op: 'move', id: 'tX', columnId: 'colX', beforeId: 't1', afterId: 't3' },
    ])
  })

  it('still promotes a subtask dragged out onto a column, before the move', () => {
    const evt = { added: { element: { id: 's1', parent_id: 't1' }, newIndex: 1 } }
    expect(planColDrop({ groupMode: 'status', evt, dcol: col(), list, sorted: true })).toEqual([
      { op: 'setParent', id: 's1', parentId: null },
      { op: 'move', id: 's1', columnId: 'colX', beforeId: null, afterId: null },
    ])
  })

  it('drops the collapsed column’s top-pin too — "top" is the sort’s, not the ladder’s', () => {
    const evt = { added: { element: { id: 'tX' }, newIndex: 0 } }
    expect(
      planColDrop({ groupMode: 'status', evt, dcol: col(), list, collapsed: true, sorted: true }),
    ).toEqual([{ op: 'move', id: 'tX', columnId: 'colX', beforeId: null, afterId: null }])
  })

  it('keeps the collapsed top-pin when no sort is active', () => {
    const evt = { added: { element: { id: 'tX' }, newIndex: 0 } }
    expect(
      planColDrop({ groupMode: 'status', evt, dcol: col(), list, collapsed: true, sorted: false }),
    ).toEqual([{ op: 'move', id: 'tX', columnId: 'colX', beforeId: null, afterId: 't1' }])
  })

  it('defaults to "no sort" when the caller says nothing', () => {
    const evt = { added: { element: { id: 'tX' }, newIndex: 1 } }
    expect(planColDrop({ groupMode: 'status', evt, dcol: col(), list })).toEqual([
      { op: 'move', id: 'tX', columnId: 'colX', beforeId: 't1', afterId: 't3' },
    ])
  })
})

describe('planColDrop — the non-status rule sets are untouched by the sort', () => {
  it('a milestone lane still sets the milestone', () => {
    const evt = { added: { element: { id: 'tX' }, newIndex: 0 } }
    const dcol = col({ milestone: { id: 'm1' } })
    expect(planColDrop({ groupMode: 'milestone', evt, dcol, list, sorted: true })).toEqual([
      { op: 'setMilestone', id: 'tX', milestoneId: 'm1' },
    ])
  })

  it('a tag lane still adds its tag', () => {
    const evt = { added: { element: { id: 'tX', tag_ids: [] }, newIndex: 0 } }
    const dcol = col({ tag: { id: 'g1' } })
    expect(planColDrop({ groupMode: 'tag', evt, dcol, list, sorted: true })).toEqual([
      { op: 'addTag', id: 'tX', tagId: 'g1' },
    ])
  })
})
