import { describe, it, expect } from 'vitest'
import { moveFocus, firstCard, neighbourColumn, planShift, planComplete } from '@/utils/cardNav'

const COLS = [
  { key: 'a', ids: ['a1', 'a2', 'a3'], collapsed: false },
  { key: 'b', ids: [], collapsed: false },
  { key: 'c', ids: ['c1'], collapsed: true },
  { key: 'd', ids: ['d1', 'd2'], collapsed: false },
]

describe('moveFocus (#2956)', () => {
  it('starts on the first card of the first usable column', () => {
    expect(moveFocus(COLS, null, 'down')).toEqual({ colKey: 'a', taskId: 'a1' })
    expect(firstCard([{ key: 'x', ids: [], collapsed: false }])).toBeNull()
  })

  it('moves up/down inside the column and stops at its ends', () => {
    expect(moveFocus(COLS, { colKey: 'a', taskId: 'a1' }, 'down')).toEqual({
      colKey: 'a',
      taskId: 'a2',
    })
    expect(moveFocus(COLS, { colKey: 'a', taskId: 'a3' }, 'down')).toEqual({
      colKey: 'a',
      taskId: 'a3',
    })
    expect(moveFocus(COLS, { colKey: 'a', taskId: 'a1' }, 'up')).toEqual({
      colKey: 'a',
      taskId: 'a1',
    })
  })

  it('skips empty and collapsed columns sideways, keeping the row when it can', () => {
    expect(moveFocus(COLS, { colKey: 'a', taskId: 'a3' }, 'right')).toEqual({
      colKey: 'd',
      taskId: 'd2',
    })
    expect(moveFocus(COLS, { colKey: 'd', taskId: 'd1' }, 'left')).toEqual({
      colKey: 'a',
      taskId: 'a1',
    })
    expect(moveFocus(COLS, { colKey: 'd', taskId: 'd1' }, 'right')).toEqual({
      colKey: 'd',
      taskId: 'd1',
    })
  })

  it('recovers from a stale selection', () => {
    expect(moveFocus(COLS, { colKey: 'a', taskId: 'gone' }, 'down')).toEqual({
      colKey: 'a',
      taskId: 'a1',
    })
  })
})

describe('planShift / planComplete', () => {
  const tag = (id) => ({ id })
  it('moves to the tail of the next status column', () => {
    expect(
      planShift({
        groupMode: 'status',
        from: { key: 'a' },
        to: { key: 'b' },
        taskId: 't',
        tailId: 'x',
      }),
    ).toEqual([{ op: 'move', id: 't', columnId: 'b', beforeId: 'x', afterId: null }])
    expect(neighbourColumn(COLS, 'a', 'left')).toBeNull()
    expect(neighbourColumn(COLS, 'a', 'right').key).toBe('b')
  })

  it('swaps tags in tag grouping and sets/clears the milestone', () => {
    expect(
      planShift({
        groupMode: 'tag',
        from: { key: '1', tag: tag('t1') },
        to: { key: '__none__' },
        taskId: 't',
      }),
    ).toEqual([{ op: 'removeTag', id: 't', tagId: 't1' }])
    expect(
      planShift({
        groupMode: 'milestone',
        from: { key: '__none__' },
        to: { key: 'm', milestone: tag('m') },
        taskId: 't',
      }),
    ).toEqual([{ op: 'setMilestone', id: 't', milestoneId: 'm' }])
  })

  it('completes through the done column, or by the flag without one', () => {
    expect(planComplete({ completed: true })).toEqual({ op: 'uncomplete' })
    expect(planComplete({ completed: false, columnId: 'a', doneColumnId: 'd' })).toEqual({
      op: 'move',
      columnId: 'd',
    })
    expect(planComplete({ completed: false, columnId: 'd', doneColumnId: 'd' })).toEqual({
      op: 'complete',
    })
    expect(planComplete({ completed: false, columnId: 'a', doneColumnId: null })).toEqual({
      op: 'complete',
    })
  })
})
