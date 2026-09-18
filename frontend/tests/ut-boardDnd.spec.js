import { describe, it, expect } from 'vitest'
import { planColumnReorder, planColDrop, canSortWithinLane } from '@/utils/boardDnd'

// Characterisation tests for the kanban drop-persistence rules extracted from
// KanbanBoard (#2670). Input = a Sortable change event (+ list snapshot); output =
// the intents the board dispatches to the API. The seven rules the component
// depended on are pinned here so a refactor that moves the board around can't
// silently change them.

const col = (over = {}) => ({ key: 'colX', ...over })

describe('planColumnReorder', () => {
  const model = [{ key: 'a' }, { key: 'b' }, { key: 'c' }]

  it('resolves a moved column to its new neighbours', () => {
    const evt = { moved: { element: { key: 'c' }, newIndex: 1 } }
    expect(planColumnReorder(evt, model, 'status')).toEqual({
      key: 'c',
      before_id: 'a',
      after_id: 'c', // model[newIndex+1] — the snapshot isn't reordered yet
    })
  })

  it('leaves before null at the left edge and after null at the right edge', () => {
    expect(
      planColumnReorder({ moved: { element: { key: 'x' }, newIndex: 0 } }, model, 'status'),
    ).toMatchObject({ before_id: null, after_id: 'b' })
    expect(
      planColumnReorder({ moved: { element: { key: 'x' }, newIndex: 2 } }, model, 'status'),
    ).toMatchObject({ before_id: 'b', after_id: null })
  })

  it('also accepts an `added` payload', () => {
    const evt = { added: { element: { key: 'z' }, newIndex: 1 } }
    expect(planColumnReorder(evt, model, 'status')).toMatchObject({ key: 'z', before_id: 'a' })
  })

  it('does nothing outside status mode', () => {
    const evt = { moved: { element: { key: 'c' }, newIndex: 1 } }
    expect(planColumnReorder(evt, model, 'tag')).toBeNull()
    expect(planColumnReorder(evt, model, 'milestone')).toBeNull()
  })

  it('does nothing when there is no moved/added payload', () => {
    expect(
      planColumnReorder({ removed: { element: { key: 'c' }, newIndex: 0 } }, model, 'status'),
    ).toBeNull()
    expect(planColumnReorder({}, model, 'status')).toBeNull()
  })
})

describe('planColDrop — status mode', () => {
  const list = [{ id: 't1' }, { id: 't2' }, { id: 't3' }]

  it('positions a card between its new neighbours by newIndex', () => {
    const evt = { added: { element: { id: 'tX' }, newIndex: 1 } }
    expect(planColDrop({ groupMode: 'status', evt, dcol: col(), list })).toEqual([
      { op: 'move', id: 'tX', columnId: 'colX', beforeId: 't1', afterId: 't3' },
    ])
  })

  it('pins to the top on a collapsed column (before none, after first non-self)', () => {
    // newIndex is meaningless here; the dropped card itself may appear in the list.
    const withSelf = [{ id: 'tX' }, { id: 't1' }, { id: 't2' }]
    const evt = { added: { element: { id: 'tX' }, newIndex: 5 } }
    expect(
      planColDrop({ groupMode: 'status', evt, dcol: col(), list: withSelf, collapsed: true }),
    ).toEqual([{ op: 'move', id: 'tX', columnId: 'colX', beforeId: null, afterId: 't1' }])
  })

  it('promotes a dragged-out subtask to top-level before moving it', () => {
    const evt = { added: { element: { id: 'sub', parent_id: 'par' }, newIndex: 0 } }
    const intents = planColDrop({ groupMode: 'status', evt, dcol: col(), list })
    expect(intents).toEqual([
      { op: 'setParent', id: 'sub', parentId: null },
      { op: 'move', id: 'sub', columnId: 'colX', beforeId: null, afterId: 't2' },
    ])
    // Order matters: setParent must precede move.
    expect(intents[0].op).toBe('setParent')
  })

  it('does not promote a top-level card that just moves within/into a column', () => {
    const evt = { moved: { element: { id: 't2' }, newIndex: 2 } }
    const intents = planColDrop({ groupMode: 'status', evt, dcol: col(), list })
    expect(intents.every((i) => i.op !== 'setParent')).toBe(true)
    expect(intents[0]).toMatchObject({ op: 'move', beforeId: 't2', afterId: null })
  })

  it('is a no-op when the event carries no added/moved payload', () => {
    expect(planColDrop({ groupMode: 'status', evt: { removed: {} }, dcol: col(), list })).toEqual(
      [],
    )
  })
})

describe('planColDrop — milestone mode (single-value)', () => {
  it('sets the destination milestone on add', () => {
    const evt = { added: { element: { id: 't1' } } }
    const dcol = col({ milestone: { id: 'm9' } })
    expect(planColDrop({ groupMode: 'milestone', evt, dcol })).toEqual([
      { op: 'setMilestone', id: 't1', milestoneId: 'm9' },
    ])
  })

  it('clears the milestone when dropped on the «no milestone» column', () => {
    const evt = { added: { element: { id: 't1' } } }
    expect(planColDrop({ groupMode: 'milestone', evt, dcol: col({ milestone: null }) })).toEqual([
      { op: 'clearMilestone', id: 't1' },
    ])
  })

  it('ignores the source column `removed` (the new value overwrites)', () => {
    const evt = { removed: { element: { id: 't1' } } }
    expect(
      planColDrop({ groupMode: 'milestone', evt, dcol: col({ milestone: { id: 'm9' } }) }),
    ).toEqual([])
  })
})

describe('planColDrop — tag mode', () => {
  const dcol = col({ tag: { id: 'tagRed' } })

  it('adds the column tag on add', () => {
    const evt = { added: { element: { id: 't1' } } }
    expect(planColDrop({ groupMode: 'tag', evt, dcol })).toEqual([
      { op: 'addTag', id: 't1', tagId: 'tagRed' },
    ])
  })

  it('removes the column tag on remove', () => {
    const evt = { removed: { element: { id: 't1' } } }
    expect(planColDrop({ groupMode: 'tag', evt, dcol })).toEqual([
      { op: 'removeTag', id: 't1', tagId: 'tagRed' },
    ])
  })

  it('emits both when a card moves between two tag columns in one event', () => {
    const evt = { added: { element: { id: 't1' } }, removed: { element: { id: 't1' } } }
    expect(planColDrop({ groupMode: 'tag', evt, dcol })).toEqual([
      { op: 'addTag', id: 't1', tagId: 'tagRed' },
      { op: 'removeTag', id: 't1', tagId: 'tagRed' },
    ])
  })

  it('strips the lane tags when dropped on the «no tags» column', () => {
    const evt = { added: { element: { id: 't1', tag_ids: ['tagRed', 'tagBlue'] } } }
    expect(
      planColDrop({
        groupMode: 'tag',
        evt,
        dcol: col({ tag: null }),
        groupTags: [{ id: 'tagRed' }, { id: 'tagBlue' }],
      }),
    ).toEqual([
      { op: 'removeTag', id: 't1', tagId: 'tagRed' },
      { op: 'removeTag', id: 't1', tagId: 'tagBlue' },
    ])
  })

  it('leaves tags that are not lanes on the card', () => {
    // Grouping narrowed to a namespace: the lane means "no сложность:: tag", and
    // the card's other tags are invisible to this grouping.
    const evt = { added: { element: { id: 't1', tag_ids: ['tagRed', 'tagOther'] } } }
    expect(
      planColDrop({
        groupMode: 'tag',
        evt,
        dcol: col({ tag: null }),
        groupTags: [{ id: 'tagRed' }],
      }),
    ).toEqual([{ op: 'removeTag', id: 't1', tagId: 'tagRed' }])
  })

  it('plans nothing for a card that already carries no lane tag', () => {
    const evt = { added: { element: { id: 't1', tag_ids: [] } } }
    expect(
      planColDrop({
        groupMode: 'tag',
        evt,
        dcol: col({ tag: null }),
        groupTags: [{ id: 'tagRed' }],
      }),
    ).toEqual([])
  })

  it('takes nothing off a card LEAVING the «no tags» column', () => {
    // It left because the destination lane gave it a tag — that lane's own `added`
    // persists the change; there is nothing for the catch-all to undo.
    const evt = { removed: { element: { id: 't1', tag_ids: ['tagRed'] } } }
    expect(
      planColDrop({
        groupMode: 'tag',
        evt,
        dcol: col({ tag: null }),
        groupTags: [{ id: 'tagRed' }],
      }),
    ).toEqual([])
  })
})

// Re-ordering a card INSIDE one lane (#2932). Card order is `position`, which is
// per COLUMN — a tag/milestone lane collects cards out of every column, so its
// neighbours are not the card's neighbours in the order `position` defines. There
// is nothing to persist, and the two rule sets say so by producing no intents for
// a bare `moved`. The board therefore must not offer the gesture: it used to leave
// those lanes sortable, so the card landed in its new slot, nothing was sent, and
// the debounced reload put it back a fifth of a second later.
describe('canSortWithinLane — which grouping owns card order', () => {
  it('status columns do', () => {
    expect(canSortWithinLane('status')).toBe(true)
  })

  it('tag and milestone lanes do not', () => {
    expect(canSortWithinLane('tag')).toBe(false)
    expect(canSortWithinLane('milestone')).toBe(false)
  })

  it('and neither do the timeline-only swimlane modes, or an unknown one', () => {
    expect(canSortWithinLane('assignee')).toBe(false)
    expect(canSortWithinLane('none')).toBe(false)
    expect(canSortWithinLane(undefined)).toBe(false)
  })

  it('is exactly the modes whose planner can act on a bare `moved`', () => {
    const evt = { moved: { element: { id: 't2' }, newIndex: 0 } }
    const list = [{ id: 't1' }, { id: 't2' }]
    for (const groupMode of ['status', 'tag', 'milestone']) {
      const dcol = col({ tag: { id: 'tagRed' }, milestone: { id: 'm9' } })
      const intents = planColDrop({ groupMode, evt, dcol, list })
      expect(intents.length > 0).toBe(canSortWithinLane(groupMode))
    }
  })
})
