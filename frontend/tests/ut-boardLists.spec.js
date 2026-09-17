// Kanban lane buckets (#2932).
//
// The regression this pins: `lists[dcol.key]` is handed to `<draggable :list="…">`,
// and vuedraggable treats a missing list as null — the lane stays a live drop target
// but every drag callback is swallowed, so a card dropped there moves on screen,
// sends nothing and snaps back on the next reload. The board rebuilt its buckets
// only when the CARDS changed, while lanes are born from the columns, the tags and
// the milestones, each of which realtime re-reads on its own. Hence the rule below:
// every lane that is drawn gets a bucket, empty or not.
import { describe, it, expect } from 'vitest'
import { buildLists, collapseKey, NO_LANE } from '@/utils/boardLists'

const task = (id, over = {}) => ({ id, column_id: 'c1', tag_ids: [], ...over })

describe('buildLists — status grouping', () => {
  it('gives every column a bucket, including one with no cards', () => {
    const map = buildLists({
      groupMode: 'status',
      columns: [{ id: 'c1' }, { id: 'c2' }],
      tasks: [task('t1')],
    })
    expect(Object.keys(map).sort()).toEqual(['c1', 'c2'])
    expect(map.c1.map((t) => t.id)).toEqual(['t1'])
    expect(map.c2).toEqual([])
  })

  it('a column added without the cards changing is droppable at once', () => {
    // The colleague's `column.created` re-reads only the columns; `tasks` is the
    // same array it was. The new lane must still come back with a bucket.
    const tasks = [task('t1')]
    const before = buildLists({ groupMode: 'status', columns: [{ id: 'c1' }], tasks })
    const after = buildLists({
      groupMode: 'status',
      columns: [{ id: 'c1' }, { id: 'c2' }],
      tasks,
    })
    expect(before.c2).toBeUndefined()
    expect(after.c2).toEqual([])
  })

  it('keeps a card whose column is not on the board out of the drawn lanes', () => {
    const map = buildLists({
      groupMode: 'status',
      columns: [{ id: 'c1' }],
      tasks: [task('t1'), task('stray', { column_id: 'gone' })],
    })
    expect(map.c1.map((t) => t.id)).toEqual(['t1'])
    expect(map.gone.map((t) => t.id)).toEqual(['stray'])
  })
})

describe('buildLists — milestone grouping', () => {
  it('buckets by milestone and keeps a catch-all lane', () => {
    const map = buildLists({
      groupMode: 'milestone',
      milestones: [{ id: 'm1' }, { id: 'm2' }],
      tasks: [
        task('t1', { milestone_id: 'm1' }),
        task('t2'),
        // A milestone the board doesn't list (scoped away) reads as "none".
        task('t3', { milestone_id: 'm9' }),
      ],
    })
    expect(map.m1.map((t) => t.id)).toEqual(['t1'])
    expect(map.m2).toEqual([])
    expect(map[NO_LANE].map((t) => t.id)).toEqual(['t2', 't3'])
  })
})

describe('buildLists — tag grouping', () => {
  it('stands a two-tag card in both lanes', () => {
    const map = buildLists({
      groupMode: 'tag',
      groupTags: [{ id: 'g1' }, { id: 'g2' }],
      tasks: [task('t1', { tag_ids: ['g1', 'g2'] }), task('t2', { tag_ids: [] })],
    })
    expect(map.g1.map((t) => t.id)).toEqual(['t1'])
    expect(map.g2.map((t) => t.id)).toEqual(['t1'])
    expect(map[NO_LANE].map((t) => t.id)).toEqual(['t2'])
  })

  it('a tag lane added without the cards changing is droppable at once', () => {
    // `tag.created` re-reads only the workspace meta — the same gap as a new column,
    // on the feature the board exists for (columns = tags).
    const tasks = [task('t1', { tag_ids: ['g1'] })]
    const after = buildLists({
      groupMode: 'tag',
      groupTags: [{ id: 'g1' }, { id: 'g2' }],
      tasks,
    })
    expect(after.g2).toEqual([])
  })

  it('ignores tags outside the grouping set', () => {
    const map = buildLists({
      groupMode: 'tag',
      groupTags: [{ id: 'g1' }],
      tasks: [task('t1', { tag_ids: ['other'] })],
    })
    expect(map.g1).toEqual([])
    expect(map[NO_LANE].map((t) => t.id)).toEqual(['t1'])
  })

  it('survives a card with no tag_ids field at all', () => {
    const map = buildLists({ groupMode: 'tag', groupTags: [{ id: 'g1' }], tasks: [{ id: 't1' }] })
    expect(map[NO_LANE].map((t) => t.id)).toEqual(['t1'])
  })
})

// ── collapse overrides ──
//
// The second regression this file pins: the catch-all lane's key is the SAME
// string in two groupings, while the collapse overrides live in one per-board blob
// that switching the grouping doesn't reset. Collapsing «Без этапа» therefore came
// back as a collapsed «Без тегов» — on a tag-grouped board the fullest lane there
// is — without the reader touching it.
describe('collapseKey', () => {
  it('keeps a real lane id as its own key', () => {
    expect(collapseKey('status', 'c1')).toBe('c1')
    expect(collapseKey('milestone', 'm1')).toBe('m1')
    expect(collapseKey('tag', 'g1')).toBe('g1')
  })

  it('namespaces the catch-all lane per grouping', () => {
    expect(collapseKey('milestone', NO_LANE)).not.toBe(collapseKey('tag', NO_LANE))
  })

  it('is stable for the same grouping', () => {
    expect(collapseKey('tag', NO_LANE)).toBe(collapseKey('tag', NO_LANE))
  })

  it('collapsing «no milestone» leaves «no tags» expanded', () => {
    // What the board does: colCollapse[key] = true, then the grouping flips.
    const colCollapse = {}
    colCollapse[collapseKey('milestone', NO_LANE)] = true
    expect(collapseKey('tag', NO_LANE) in colCollapse).toBe(false)
  })

  it('a real lane id keeps its override across a grouping switch', () => {
    // Only the catch-all is namespaced — a column collapsed in status grouping is
    // still collapsed when the reader comes back to it.
    const colCollapse = { c1: true }
    expect(collapseKey('status', 'c1') in colCollapse).toBe(true)
  })
})
