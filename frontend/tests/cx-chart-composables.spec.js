import { describe, it, expect, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { ref, computed, defineComponent, h } from 'vue'
import { mount } from '@vue/test-utils'

import { useThemeStore } from '@/stores/theme'
import { useChartLanes, MILESTONE_COLORS } from '@/composables/useChartLanes'
import { useChartRows, ROW_H, SUB_STEP } from '@/composables/useChartRows'
import { useChartTimeline, ZOOM, DEFAULT_ZOOM_IDX } from '@/composables/useChartTimeline'
import { DAY_MS, startOfDay, spanOf, parseDate } from '@/utils/timeAxis'

// The Timeline and Gantt board views render the same chart from these three
// composables; before the extraction each kept its own copy of this logic, which is
// exactly where the two could drift apart. Lane membership is the part both views
// must agree on — the ONLY intended difference is the order they feed in (the Gantt
// passes the dependency-DFS order when «Авто» is on), so that's asserted explicitly.

// Mount a composable inside a real component so its lifecycle hooks run.
function host(setup) {
  let api
  const wrapper = mount(
    defineComponent({
      setup() {
        api = setup()
        return () => h('div')
      },
    }),
  )
  return { api, wrapper }
}

const day = (n) => new Date(2026, 0, n).toISOString()

function task(id, extra = {}) {
  return { id, title: id, start_date: day(10), due_date: day(12), ...extra }
}

function lanesFor(source, opts = {}) {
  return host(() =>
    useChartLanes({
      source: computed(() => source),
      statusColumns: ref(opts.statusColumns || []),
      membersMap: ref(opts.membersMap || {}),
      tagsMap: ref(opts.tagsMap || {}),
      groupMode: ref(opts.groupMode || 'none'),
      tagPrefix: ref(opts.tagPrefix || ''),
      estCfg: ref(opts.estCfg || { unit: 'hours' }),
      milestones: ref(opts.milestones || []),
    }),
  ).api
}

// These panels render dates through useFormat(), which reads the theme store —
// so a Pinia instance has to be active before they mount (#2798).
beforeEach(() => setActivePinia(createPinia()))

describe('useChartLanes', () => {
  it('groups by assignee and pushes the unassigned lane last', () => {
    const { lanes } = lanesFor(
      [
        task('a', { assignee_ids: [] }),
        task('b', { assignee_ids: ['u1'] }),
        task('c', { assignee_ids: ['u1'] }),
      ],
      { groupMode: 'assignee', membersMap: { u1: { name: 'Ира' } } },
    )
    expect(lanes.value.map((l) => l.key)).toEqual(['u1', '∅'])
    expect(lanes.value[0].label).toBe('Ира')
    expect(lanes.value[0].tasks.map((t) => t.id)).toEqual(['b', 'c'])
    expect(lanes.value[1].label).toBe('Не назначено')
  })

  it('seeds every status column in board order, empty ones included', () => {
    const statusColumns = [
      { id: 'c1', name: 'К работе', color: '#111' },
      { id: 'c2', name: 'В процессе', color: '#222' },
    ]
    const { lanes } = lanesFor([task('a', { column_id: 'c2' })], {
      groupMode: 'status',
      statusColumns,
    })
    expect(lanes.value.map((l) => l.key)).toEqual(['c1', 'c2'])
    expect(lanes.value[0].tasks).toEqual([])
    expect(lanes.value[1].tasks.map((t) => t.id)).toEqual(['a'])
  })

  it('honours the tag prefix when grouping by a tag namespace', () => {
    const tagsMap = {
      t1: { name: 'area::be', color: '#0f0' },
      t2: { name: 'prio::high', color: '#f00' },
    }
    const { lanes } = lanesFor([task('a', { tag_ids: ['t2', 't1'] })], {
      groupMode: 'tag',
      tagPrefix: 'area::',
      tagsMap,
    })
    // t2 is filtered out by the prefix, so the lane is the area:: tag, not the first id.
    expect(lanes.value.map((l) => l.key)).toEqual(['t1'])
    // Captioned like the board's tag column: the namespace the grouping is already
    // narrowed to is not repeated in every lane (see cx-tag-lane-caption).
    expect(lanes.value[0].label).toBe('be')
  })

  // v2 (#2959): milestone lanes in the project's milestone order, coloured by
  // position (milestones have no colour), «Без этапа» last, «X из N» counted.
  it('groups by milestone in milestone order, the rest in «Без этапа» last', () => {
    const milestones = [
      { id: 'm1', title: 'Релиз 1', due_date: day(20) },
      { id: 'm2', title: 'Релиз 2' },
    ]
    const { lanes } = lanesFor(
      [
        task('a', { milestone_id: 'm2' }),
        task('b'),
        task('c', { milestone_id: 'm1', completed_at: day(11) }),
        task('d', { milestone_id: 'm1' }),
      ],
      { groupMode: 'milestone', milestones },
    )
    expect(lanes.value.map((l) => l.key)).toEqual(['m1', 'm2', '∅'])
    expect(lanes.value[0].milestone.title).toBe('Релиз 1')
    expect(lanes.value[0].color).toBe(MILESTONE_COLORS[0])
    expect(lanes.value[1].color).toBe(MILESTONE_COLORS[1])
    expect(lanes.value[0].done).toBe(1)
    expect(lanes.value[2].label).toBe('Без этапа')
    expect(lanes.value[2].tasks.map((x) => x.id)).toEqual(['b'])
  })

  it('keeps the incoming order inside a lane — this is what carries the Gantt «Авто» order', () => {
    const a = task('a', { assignee_ids: ['u1'] })
    const b = task('b', { assignee_ids: ['u1'], start_date: day(1), due_date: day(2) })
    // `b` starts earlier, but the caller ordered `a` first: lanes must not re-sort.
    const { lanes } = lanesFor([a, b], { groupMode: 'assignee' })
    expect(lanes.value[0].tasks.map((t) => t.id)).toEqual(['a', 'b'])
  })

  it('sums lane effort with the project estimation config', () => {
    const cfg = { unit: 'hours', hours_per_day: 8 }
    const { lanes, laneEffort, laneEffortFull } = lanesFor(
      [task('a', { estimate: 4 }), task('b', { estimate: 2 })],
      { estCfg: cfg },
    )
    expect(laneEffort(lanes.value[0])).toBeTruthy()
    expect(laneEffortFull(lanes.value[0])).toBeTruthy()
    // No estimates at all → no lane total rendered.
    const empty = lanesFor([task('c')], { estCfg: cfg })
    expect(empty.laneEffort(empty.lanes.value[0])).toBe('')
  })
})

function rowsFor(lanes, opts = {}) {
  return host(() =>
    useChartRows({
      lanes: computed(() => lanes),
      tasks: ref(opts.tasks || []),
      subtasksByParent: ref(opts.subtasksByParent || {}),
      scrollY: ref(opts.scrollY ?? 0),
      viewH: ref(opts.viewH ?? 800),
      bodyTop: ref(0),
      bodyEl: ref(null),
      ...(opts.laneH0 != null ? { laneH0: opts.laneH0 } : {}),
      ...(opts.collapsed ? { collapsedLanes: ref(new Set(opts.collapsed)) } : {}),
    }),
  ).api
}

describe('useChartRows', () => {
  it('flattens lanes into header + task rows in document order', () => {
    const lanes = [
      { key: 'l1', tasks: [task('a'), task('b')] },
      { key: 'l2', tasks: [task('c')] },
    ]
    const { flatRows } = rowsFor(lanes)
    expect(flatRows.value.map((r) => r.key)).toEqual(['Ll1', 'a', 'b', 'Ll2', 'c'])
  })

  // v2 (#2959): a milestone lane closes with the milestone's own row; a folded
  // lane is its header alone.
  it('closes a milestone lane with its row and folds a collapsed lane', () => {
    const lanes = [
      { key: 'm1', milestone: { id: 'm1', due_date: day(20) }, tasks: [task('a')] },
      { key: 'm2', milestone: { id: 'm2' }, tasks: [task('b')] },
      { key: 'l3', tasks: [task('c')] },
    ]
    expect(rowsFor(lanes).flatRows.value.map((r) => r.key)).toEqual([
      'Lm1',
      'a',
      'Mm1',
      'Lm2',
      'b',
      'Ll3',
      'c',
    ])
    const folded = rowsFor(lanes, { collapsed: ['m1'] })
    expect(folded.flatRows.value.map((r) => r.key)).toEqual(['Lm1', 'Lm2', 'b', 'Ll3', 'c'])
    expect(folded.rowH({ t: 'ms' })).toBe(ROW_H)
  })

  it('grows a row by SUB_STEP per scheduled subtask, ignoring undated ones', () => {
    const parent = task('p')
    const subtasksByParent = {
      p: [task('s1'), { id: 's2', start_date: null, due_date: null }, task('s3')],
    }
    const { rowHeight, subBars } = rowsFor([{ key: 'l', tasks: [parent] }], { subtasksByParent })
    expect(subBars(parent).map((s) => s.id)).toEqual(['s1', 's3'])
    expect(rowHeight(parent)).toBe(ROW_H + 2 * SUB_STEP)
  })

  it('finds a task by id across top-level tasks and subtasks', () => {
    const tasks = [task('p')]
    const subtasksByParent = { p: [task('s1')] }
    const { findTask } = rowsFor([{ key: 'l', tasks }], { tasks, subtasksByParent })
    expect(findTask('p').id).toBe('p')
    expect(findTask('s1').id).toBe('s1')
    expect(findTask('nope')).toBeNull()
  })

  it('windows rows to the viewport and keeps spacers summing to the full height', () => {
    const many = Array.from({ length: 200 }, (_, i) => task(`t${i}`))
    const lanes = [{ key: 'l', tasks: many }]
    const { vwindow, flatRows } = rowsFor(lanes, { viewH: 400, scrollY: 2000 })
    const total = flatRows.value.length
    expect(vwindow.value.rows.length).toBeGreaterThan(0)
    expect(vwindow.value.rows.length).toBeLessThan(total)
    // Spacers + rendered rows must reconstruct the untruncated scroll height, or the
    // scrollbar jumps as rows swap in and out.
    const rendered = vwindow.value.rows.reduce((sum, r) => sum + (r.t === 'lane' ? 30 : ROW_H), 0)
    expect(vwindow.value.top + rendered + vwindow.value.bottom).toBe(30 + 200 * ROW_H)
  })

  it('renders everything when the content is shorter than the viewport', () => {
    const lanes = [{ key: 'l', tasks: [task('a'), task('b')] }]
    const { vwindow } = rowsFor(lanes, { viewH: 800 })
    expect(vwindow.value.rows).toHaveLength(3)
    expect(vwindow.value.top).toBe(0)
    expect(vwindow.value.bottom).toBe(0)
  })

  it('seeds the lane-header height from laneH0 (the two views differ until measured)', () => {
    expect(rowsFor([]).laneH.value).toBe(30) // timeline default
    expect(rowsFor([], { laneH0: 32 }).laneH.value).toBe(32) // gantt passes LANE_H
  })
})

function timelineFor(scheduled, opts = {}) {
  return host(() =>
    useChartTimeline({
      scheduled: computed(() => scheduled),
      milestones: ref(opts.milestones || []),
      estCfg: ref(opts.estCfg || { unit: 'hours' }),
      leftW: ref(opts.leftW ?? 224),
      ...opts.extra,
    }),
  ).api
}

describe('useChartTimeline', () => {
  it('pads the range around the scheduled span and always covers today', () => {
    const { range } = timelineFor([task('a', { start_date: day(10), due_date: day(12) })])
    const lo = startOfDay(parseDate(day(10))) - 3 * DAY_MS
    expect(range.value.start).toBe(lo)
    // today is included even when it sits outside the task span
    const todayIdx = Math.round((startOfDay(Date.now()) - range.value.start) / DAY_MS)
    expect(todayIdx).toBeGreaterThanOrEqual(0)
    expect(todayIdx).toBeLessThan(range.value.days)
  })

  it('clamps zoom to the ZOOM table and tracks the tier', () => {
    const { zoomIdx, dayW, tier, zoomIn, zoomOut, applyZoom } = timelineFor([task('a')])
    expect(zoomIdx.value).toBe(DEFAULT_ZOOM_IDX)
    expect(dayW.value).toBe(ZOOM[DEFAULT_ZOOM_IDX])
    expect(tier.value).toBe('days')
    applyZoom(999)
    expect(zoomIdx.value).toBe(ZOOM.length - 1)
    expect(tier.value).toBe('hours')
    zoomIn() // already at the top — must not overflow
    expect(zoomIdx.value).toBe(ZOOM.length - 1)
    applyZoom(-5)
    expect(zoomIdx.value).toBe(0)
    expect(tier.value).toBe('months')
    zoomOut()
    expect(zoomIdx.value).toBe(0)
  })

  // v2 (#2959): the toolbar's «Дни / Недели / Месяцы» presets.
  it('jumps to a scale preset and reports the scale the zoom sits in', async () => {
    const { scale, setScale, dayW, zoomIn } = timelineFor([task('a')])
    expect(scale.value).toBe('days')
    expect(dayW.value).toBe(24)
    setScale('weeks')
    expect(dayW.value).toBe(10)
    expect(scale.value).toBe('weeks')
    setScale('months')
    expect(scale.value).toBe('months')
    zoomIn()
    expect(scale.value).toBe('months') // 6px/day still groups by months
    setScale('days')
    expect(scale.value).toBe('days')
  })

  it('breaks the week bands on the preferred first day of the week', () => {
    // The weeks tier is the only zoom level that draws them; band `key` is the
    // band's first day, so its weekday is what the preference has to move.
    const firstDays = () => {
      const { weekBands, setScale } = timelineFor([
        task('a', { start_date: day(5), due_date: day(40) }),
      ])
      setScale('weeks') // the weeks tier (v2 preset; zoom 0 is months now)
      return weekBands.value.slice(1).map((b) => new Date(b.key).getDay())
    }
    expect(new Set(firstDays())).toEqual(new Set([1])) // default: Monday
    useThemeStore().weekStart = 0
    expect(new Set(firstDays())).toEqual(new Set([0])) // Sunday
  })

  it('breaks the days tier’s week ranges on the same preferred day (v2)', () => {
    const firstDays = () => {
      const { weekRangeBands, setScale } = timelineFor([
        task('a', { start_date: day(5), due_date: day(40) }),
      ])
      setScale('days')
      return weekRangeBands.value.slice(1).map((b) => new Date(b.key).getDay())
    }
    expect(new Set(firstDays())).toEqual(new Set([1]))
    useThemeStore().weekStart = 0
    expect(new Set(firstDays())).toEqual(new Set([0]))
  })

  it('keeps only milestones whose due date lands inside the axis range', () => {
    const { milestoneMarkers, range, dayW } = timelineFor(
      [task('a', { start_date: day(10), due_date: day(12) })],
      {
        milestones: [
          { id: 'm1', title: 'in', due_date: day(11) },
          { id: 'm2', title: 'far', due_date: new Date(2027, 5, 1).toISOString() },
          { id: 'm3', title: 'undated', due_date: null },
        ],
      },
    )
    expect(milestoneMarkers.value.map((m) => m.id)).toEqual(['m1'])
    const di = Math.round((startOfDay(parseDate(day(11))) - range.value.start) / DAY_MS)
    expect(milestoneMarkers.value[0].left).toBe(di * dayW.value + dayW.value / 2)
  })

  it('hides the cursor guide while another gesture owns the pointer', () => {
    const blocked = ref(false)
    const { cursor, onHoverMove } = timelineFor([task('a')], {
      extra: { cursorBlocked: () => blocked.value },
    })
    blocked.value = true
    onHoverMove({ clientX: 500, clientY: 100 })
    expect(cursor.value).toBeNull()
  })
})

describe('spanOf (shared axis math)', () => {
  it('collapses a one-ended task to a single day and reports which end is real', () => {
    expect(spanOf({ start_date: day(5), due_date: null })).toEqual({
      a: startOfDay(parseDate(day(5))),
      b: startOfDay(parseDate(day(5))),
      hasStart: true,
      hasDue: false,
    })
    expect(spanOf({ start_date: null, due_date: day(7) })).toEqual({
      a: startOfDay(parseDate(day(7))),
      b: startOfDay(parseDate(day(7))),
      hasStart: false,
      hasDue: true,
    })
  })
})
