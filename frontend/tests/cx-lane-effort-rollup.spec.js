// A lane's effort total counts the work its cards actually show (#2932).
//
// A card prints its OWN estimate, or — when it has none — the rollup of its
// subtasks (TaskCardPills: `own ?? sumEstimates(subtasks)`, with the «Σ …» rollup
// tooltip). The totals above those cards read only the own estimates: the kanban
// column's «Σ» header (milestone grouping) and the Timeline/Gantt swimlane effort
// chip both went through `sumEstimates(lane.tasks)`.
//
// Lanes hold top-level rows only — `buildLists` buckets the filtered top-level
// tasks, and the charts' `source` is the scheduled top-level tasks with their
// children drawn as sub-bars inside the parent's row. So a task planned entirely in
// its subtasks (no own estimate, every child estimated) showed «Σ 5д» on its own
// card and contributed exactly zero to the total above it. Sprint load is read off
// that total, and the tasks it silently dropped were the decomposed ones.
//
// Three halves are pinned, because any of them alone can be green while the header
// still disagrees with the cards under it: the shared rule, that the chart lanes go
// through it, and that a card's own estimate still wins over its children's (a
// parent that estimated itself must not be counted twice).
import { describe, it, expect } from 'vitest'
import { ref } from 'vue'

import { effectiveEstimate, sumEstimates, sumLaneEffort } from '@/utils/estimation'
import { useChartLanes } from '@/composables/useChartLanes'

// Minutes, the canonical unit for `time` estimates.
const PARENT = { id: 'p1', column_id: 'c1', estimate: null }
const KIDS = [{ id: 's1', estimate: 120 }, { id: 's2', estimate: 180 }, { id: 's3' }]
const SOLO = { id: 'p2', column_id: 'c1', estimate: 60 }
// A parent that estimated ITSELF and also has estimated children.
const BOTH = { id: 'p3', column_id: 'c1', estimate: 90 }
const BOTH_KIDS = [{ id: 's4', estimate: 999 }]

const CFG = { unit: 'time', hours_per_day: 8, days_per_week: 5 }

describe('effectiveEstimate — what one card is worth', () => {
  it('falls back to the subtask rollup when the card has no estimate of its own', () => {
    expect(effectiveEstimate(PARENT, KIDS)).toBe(300)
  })

  it('prefers the card own estimate over its children (no double counting)', () => {
    expect(effectiveEstimate(BOTH, BOTH_KIDS)).toBe(90)
  })

  it('is null when neither the card nor its children are estimated', () => {
    expect(effectiveEstimate({ id: 'x' }, [{ id: 'y' }])).toBeNull()
    expect(effectiveEstimate({ id: 'x' }, undefined)).toBeNull()
  })

  it('reads a card the same way its own chip does', () => {
    // TaskCardPills: `ownEstimate ?? rollupEstimate`, rollup = sumEstimates(subtasks).
    const asCardPrints = PARENT.estimate ?? sumEstimates(KIDS)
    expect(effectiveEstimate(PARENT, KIDS)).toBe(asCardPrints)
  })
})

describe('sumLaneEffort — the total above those cards', () => {
  const byParent = { p1: KIDS, p3: BOTH_KIDS }

  it('counts a decomposed card through its subtasks', () => {
    // 300 (rolled up) + 60 (own) + 90 (own, children ignored).
    expect(sumLaneEffort([PARENT, SOLO, BOTH], byParent)).toBe(450)
  })

  it('agrees with the sum of what each card prints', () => {
    const lane = [PARENT, SOLO, BOTH]
    const perCard = lane.reduce((acc, t) => acc + effectiveEstimate(t, byParent[t.id]), 0)
    expect(sumLaneEffort(lane, byParent)).toBe(perCard)
  })

  it('is null for a lane where nothing is estimated at all', () => {
    expect(sumLaneEffort([{ id: 'a' }, { id: 'b' }], { a: [{ id: 'c' }] })).toBeNull()
    expect(sumLaneEffort([], {})).toBeNull()
    expect(sumLaneEffort(null)).toBeNull()
  })

  it('works with no child map at all (an undecomposed board)', () => {
    expect(sumLaneEffort([SOLO])).toBe(60)
  })

  it('ignores children of a card that is not in the lane', () => {
    expect(sumLaneEffort([SOLO], byParent)).toBe(60)
  })
})

describe('chart swimlanes use the same rule', () => {
  function lanesFor({ subtasksByParent } = {}) {
    return useChartLanes({
      source: ref([PARENT, SOLO]),
      statusColumns: ref([{ id: 'c1', name: 'В процессе', color: '' }]),
      membersMap: ref({}),
      tagsMap: ref({}),
      groupMode: ref('status'),
      tagPrefix: ref(''),
      prefixNames: ref({}),
      estCfg: ref(CFG),
      subtasksByParent: subtasksByParent ? ref(subtasksByParent) : undefined,
    })
  }

  it('rolls a decomposed card up into the lane effort', () => {
    const { lanes, laneEffort } = lanesFor({ subtasksByParent: { p1: KIDS } })
    const lane = lanes.value.find((l) => l.key === 'c1')
    // 300 + 60 = 360 min = 6h; without the rollup it would be the bare 60 min = 1h.
    expect(laneEffort(lane)).toBe('6ч')
  })

  it('spells the same total out in the tooltip', () => {
    const { lanes, laneEffortFull } = lanesFor({ subtasksByParent: { p1: KIDS } })
    const lane = lanes.value.find((l) => l.key === 'c1')
    expect(laneEffortFull(lane)).toContain('6')
  })

  it('still works when the caller passes no child map', () => {
    const { lanes, laneEffort } = lanesFor()
    const lane = lanes.value.find((l) => l.key === 'c1')
    expect(laneEffort(lane)).toBe('1ч')
  })
})
