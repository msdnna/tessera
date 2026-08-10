import { describe, it, expect, vi, beforeEach } from 'vitest'
import { ref, computed, defineComponent, h } from 'vue'
import { mount } from '@vue/test-utils'

// The chart parts the Timeline and Gantt views now share. Before the extraction
// each view carried its own copy of this markup, and neither view was mounted by
// any test — so the pieces most likely to drift (the bar geometry, the link-only
// affordances, the tier-dependent header) had no coverage at all. These mount the
// shared components directly and assert what each view relies on.

const update = vi.fn()
vi.mock('@/api', () => ({
  tasks: {
    update: (...a) => update(...a),
    addRelation: vi.fn(),
    removeRelation: vi.fn(),
  },
  boards: { dependencies: vi.fn() },
}))

import ChartTaskRow from '@/components/chart/ChartTaskRow.vue'
import ChartLaneHeader from '@/components/chart/ChartLaneHeader.vue'
import ChartToolbar from '@/components/chart/ChartToolbar.vue'
import ChartAxisHeader from '@/components/chart/ChartAxisHeader.vue'
import ChartUnscheduled from '@/components/chart/ChartUnscheduled.vue'
import { useChartBars } from '@/composables/useChartBars'
import { SUB_STEP, SUB_TOP0 } from '@/composables/useChartRows'
import { DAY_MS } from '@/utils/timeAxis'

const stubs = {
  NIcon: { template: '<i class="n-icon-stub" />' },
  NTooltip: { template: '<div class="n-tooltip-stub"><slot name="trigger" /><slot /></div>' },
}

const day = (n) => new Date(2026, 0, n).toISOString()
const geom = (left, width, extra = {}) => ({
  left,
  width,
  hasStart: true,
  hasDue: true,
  ...extra,
})

function rowOf(extra = {}) {
  return {
    t: 'task',
    key: 't1',
    task: { id: 't1', title: 'Задача', priority: 2 },
    height: 36,
    geom: geom(100, 68),
    ghost: null,
    ghostEst: '',
    ghostTitle: '',
    subs: [],
    ...extra,
  }
}

function mountRow(props = {}, rowExtra = {}) {
  return mount(ChartTaskRow, {
    props: { row: rowOf(rowExtra), leftW: 224, axisW: 900, tier: 'days', ...props },
    global: { stubs },
  })
}

// ── ChartTaskRow ──────────────────────────────────────────────────────────────
describe('ChartTaskRow.vue', () => {
  it('places the bar at the precomputed geometry', () => {
    const bar = mountRow().find('.bar')
    expect(bar.attributes('style')).toContain('left: 100px')
    expect(bar.attributes('style')).toContain('width: 68px')
  })

  it('marks a one-ended task as a point bar', () => {
    const w = mountRow({}, { geom: geom(10, 34, { hasDue: false }) })
    expect(w.find('.bar').classes()).toContain('point')
    expect(mountRow().find('.bar').classes()).not.toContain('point')
  })

  it('stacks sub-bars at their own tops and keeps them draggable', async () => {
    const subs = [
      { task: { id: 's1', title: 'a', priority: 0 }, geom: geom(20, 30), top: SUB_TOP0 },
      {
        task: { id: 's2', title: 'b', priority: 1 },
        geom: geom(50, 30),
        top: SUB_TOP0 + SUB_STEP,
      },
    ]
    const w = mountRow({}, { subs })
    const bars = w.findAll('.tl-subbar')
    expect(bars).toHaveLength(2)
    expect(bars[0].attributes('style')).toContain(`top: ${SUB_TOP0}px`)
    expect(bars[1].attributes('style')).toContain(`top: ${SUB_TOP0 + SUB_STEP}px`)

    await bars[1].find('.handle.r').trigger('pointerdown')
    const [, task, mode] = w.emitted('bar-down').at(-1)
    expect(task.id).toBe('s2')
    expect(mode).toBe('due')
  })

  // The Gantt adds dependencies on top of the same row; the Timeline must not grow
  // link affordances (a knob there would drag nothing and the drop-target attribute
  // would make elementFromPoint resolve rows in a view that never links).
  it('renders link knobs and drop targets only when linkable', () => {
    const plain = mountRow(
      {},
      { subs: [{ task: { id: 's1', title: 'a' }, geom: geom(1, 2), top: 0 }] },
    )
    expect(plain.find('.link-knob').exists()).toBe(false)
    expect(plain.find('[data-task-id]').exists()).toBe(false)

    const gantt = mountRow(
      { linkable: true },
      { subs: [{ task: { id: 's1', title: 'a' }, geom: geom(1, 2), top: 0 }] },
    )
    expect(gantt.findAll('.link-knob')).toHaveLength(2)
    expect(gantt.findAll('[data-task-id]').map((e) => e.attributes('data-task-id'))).toEqual([
      't1',
      's1',
    ])
  })

  it('highlights the bar a link is being dragged from', () => {
    expect(mountRow({ linkable: true, linkFromId: 't1' }).find('.bar').classes()).toContain(
      'linksrc',
    )
    expect(mountRow({ linkable: true, linkFromId: 'other' }).find('.bar').classes()).not.toContain(
      'linksrc',
    )
  })

  it('emits the edge being resized for each handle', async () => {
    const w = mountRow()
    await w.find('.handle.l').trigger('pointerdown')
    expect(w.emitted('bar-down').at(-1)[2]).toBe('start')
    await w.find('.bar').trigger('pointerdown')
    expect(w.emitted('bar-down').at(-1)[2]).toBe('move')
  })

  it('shows the estimate ghost only when there is one', () => {
    expect(mountRow().find('.ghost').exists()).toBe(false)
    const w = mountRow({}, { ghost: { left: 90, width: 200 }, ghostEst: '3 д' })
    expect(w.find('.ghost').attributes('style')).toContain('width: 200px')
    expect(w.text()).toContain('3 д')
  })

  it('carries the chart-level state its scoped CSS keys on', () => {
    const w = mountRow({ tier: 'weeks', collapsed: true, animate: true })
    const cls = w.find('.tl-row').classes()
    expect(cls).toEqual(expect.arrayContaining(['chart-part', 'weeks', 'collapsed', 'animate']))
  })
})

// ── ChartLaneHeader ───────────────────────────────────────────────────────────
describe('ChartLaneHeader.vue', () => {
  const lane = { label: 'Иван', color: '#7c5cff', tasks: [{ id: 'a' }, { id: 'b' }] }

  it('renders the lane name and task count', () => {
    const w = mount(ChartLaneHeader, {
      props: { lane, leftW: 224, axisW: 800, tier: 'days' },
      global: { stubs },
    })
    expect(w.find('.lane-name').text()).toBe('Иван')
    expect(w.find('.lane-count').text()).toBe('2')
  })

  it('hides the effort chip when the lane has no estimate', () => {
    const bare = mount(ChartLaneHeader, {
      props: { lane, leftW: 224, axisW: 800, tier: 'days' },
      global: { stubs },
    })
    expect(bare.find('.lane-effort').exists()).toBe(false)

    const withEffort = mount(ChartLaneHeader, {
      props: { lane, leftW: 224, axisW: 800, tier: 'days', effort: '5 д', effortFull: '5 дней' },
      global: { stubs },
    })
    expect(withEffort.find('.lane-effort').text()).toContain('5 д')
  })
})

// ── ChartToolbar ──────────────────────────────────────────────────────────────
describe('ChartToolbar.vue', () => {
  const base = { zoomIdx: 2, zoomCount: 5 }

  it('disables each zoom button at its end of the range', () => {
    const atMin = mount(ChartToolbar, { props: { ...base, zoomIdx: 0 }, global: { stubs } })
    const [out, inn] = atMin.findAll('.tl-zoom-btn')
    expect(out.attributes('disabled')).toBeDefined()
    expect(inn.attributes('disabled')).toBeUndefined()

    const atMax = mount(ChartToolbar, { props: { ...base, zoomIdx: 4 }, global: { stubs } })
    expect(atMax.findAll('.tl-zoom-btn')[1].attributes('disabled')).toBeDefined()
  })

  it('renders counters and flags the overdue one', () => {
    const w = mount(ChartToolbar, {
      props: {
        ...base,
        counters: [
          { key: 'overdue', text: '3 просрочено', overdue: true },
          { key: 'unsched', text: '2 без дат' },
        ],
      },
      global: { stubs },
    })
    const chips = w.findAll('.tl-counter')
    expect(chips.map((c) => c.text())).toEqual(['3 просрочено', '2 без дат'])
    expect(chips[0].classes()).toContain('overdue')
    expect(chips[1].classes()).not.toContain('overdue')
  })

  it('shows the hint only when one is given (the Gantt has it, the Timeline does not)', () => {
    expect(mount(ChartToolbar, { props: base, global: { stubs } }).find('.tl-hint').exists()).toBe(
      false,
    )
    const w = mount(ChartToolbar, { props: { ...base, hint: 'Тяните' }, global: { stubs } })
    expect(w.find('.tl-hint').text()).toBe('Тяните')
  })

  it('emits today / toggle-left', async () => {
    const w = mount(ChartToolbar, { props: base, global: { stubs } })
    await w.find('.tl-today-btn').trigger('click')
    expect(w.emitted('today')).toHaveLength(1)
    await w.findAll('.tl-zoom-btn')[2].trigger('click')
    expect(w.emitted('toggle-left')).toHaveLength(1)
  })
})

// ── ChartAxisHeader ───────────────────────────────────────────────────────────
describe('ChartAxisHeader.vue', () => {
  const bands = {
    leftW: 224,
    dayW: 34,
    monthBands: [{ label: 'январь', span: 31 }],
    days: [
      { day: 1, dow: 'чт', weekend: false, isToday: true },
      { day: 2, dow: 'пт', weekend: false, isToday: false },
    ],
    weekBands: [{ key: 'w1', label: '1 янв', span: 7 }],
    hourTicks: [{ key: 'h6', label: '06', left: 8 }],
  }

  it('swaps the day band for the week band in the weeks tier', () => {
    const days = mount(ChartAxisHeader, { props: { ...bands, tier: 'days' }, global: { stubs } })
    expect(days.findAll('.tl-dayh')).toHaveLength(2)
    expect(days.find('.tl-weeksrow').exists()).toBe(false)

    const weeks = mount(ChartAxisHeader, { props: { ...bands, tier: 'weeks' }, global: { stubs } })
    expect(weeks.find('.tl-weeksrow').exists()).toBe(true)
    expect(weeks.find('.tl-daysrow').exists()).toBe(false)
  })

  it('adds hour ticks only in the hours tier', () => {
    const days = mount(ChartAxisHeader, { props: { ...bands, tier: 'days' }, global: { stubs } })
    expect(days.find('.tl-hoursrow').exists()).toBe(false)
    const hours = mount(ChartAxisHeader, { props: { ...bands, tier: 'hours' }, global: { stubs } })
    expect(hours.findAll('.tl-hourtick')).toHaveLength(1)
  })

  it('marks today and flags the collapsed column', () => {
    const w = mount(ChartAxisHeader, {
      props: { ...bands, tier: 'days', collapsed: true },
      global: { stubs },
    })
    expect(w.findAll('.tl-dayh')[0].classes()).toContain('today')
    expect(w.find('.tl-head').classes()).toContain('collapsed')
  })
})

// ── ChartUnscheduled ──────────────────────────────────────────────────────────
describe('ChartUnscheduled.vue', () => {
  it('renders nothing when every task is scheduled', () => {
    const w = mount(ChartUnscheduled, { props: { tasks: [] }, global: { stubs } })
    expect(w.find('.tl-unsched').exists()).toBe(false)
  })

  it('opens a chip and raises the context menu', async () => {
    const tasks = [{ id: 'a', title: 'Без даты', priority: 1 }]
    const w = mount(ChartUnscheduled, { props: { tasks }, global: { stubs } })
    await w.find('.us-chip').trigger('click')
    expect(w.emitted('open')[0]).toEqual(['a'])
    await w.find('.us-chip').trigger('contextmenu')
    expect(w.emitted('menu')[0][1]).toEqual(tasks[0])
  })
})

// ── useChartBars ──────────────────────────────────────────────────────────────
describe('useChartBars', () => {
  beforeEach(() => update.mockReset())

  function host(tasks, opts = {}) {
    let api
    const vwindow = ref({
      rows: tasks.map((t) => ({ t: 'task', key: t.id, task: t })),
      top: 0,
      bottom: 0,
    })
    const wrapper = mount(
      defineComponent({
        setup() {
          api = useChartBars({
            tier: ref('days'),
            range: computed(() => ({ start: new Date(2026, 0, 1).getTime() })),
            dayW: ref(34),
            estCfg: computed(() => ({ unit: 'time', hoursPerDay: 8 })),
            vwindow,
            subBars: (t) => opts.subs?.[t.id] || [],
            rowHeight: () => 36,
            findTask: (id) => tasks.find((t) => t.id === id),
            onChanged: opts.onChanged || (() => {}),
          })
          return () => h('div')
        },
      }),
    )
    return { api, wrapper }
  }

  it('precomputes bar geometry for every windowed row', () => {
    const t = { id: 'a', title: 'a', start_date: day(3), due_date: day(5) }
    const { api } = host([t])
    const [row] = api.renderRows.value
    expect(row.t).toBe('task')
    expect(row.geom).toEqual(api.geom(t))
    expect(row.height).toBe(36)
  })

  it('stacks sub-bars at SUB_TOP0 + i * SUB_STEP', () => {
    const t = { id: 'a', title: 'a', start_date: day(3), due_date: day(5) }
    const subs = {
      a: [
        { id: 's1', title: 's1', start_date: day(3), due_date: day(4) },
        { id: 's2', title: 's2', start_date: day(4), due_date: day(5) },
      ],
    }
    const { api } = host([t], { subs })
    expect(api.renderRows.value[0].subs.map((s) => s.top)).toEqual([SUB_TOP0, SUB_TOP0 + SUB_STEP])
  })

  it('leaves the ghost envelope out when the task has no estimate', () => {
    const { api } = host([{ id: 'a', title: 'a', start_date: day(3), due_date: day(5) }])
    expect(api.renderRows.value[0].ghost).toBeNull()
    expect(api.renderRows.value[0].ghostEst).toBe('')
  })

  it('shifts both endpoints by whole days on a move drag and persists them', async () => {
    const t = {
      id: 'a',
      title: 'a',
      priority: 1,
      start_date: day(10),
      due_date: day(12),
      recurrence: null,
    }
    const { api } = host([t])
    api.onBarDown({ button: 0, clientX: 0, preventDefault() {}, stopPropagation() {} }, t, 'move')
    // Two day-widths to the right → both dates move two days on.
    window.dispatchEvent(new PointerEvent('pointermove', { clientX: 68 }))
    expect(api.renderRows.value[0].geom.left).toBe(api.geom({ ...t }).left)
    window.dispatchEvent(new PointerEvent('pointerup'))
    await Promise.resolve()

    expect(update).toHaveBeenCalledTimes(1)
    const [id, body] = update.mock.calls[0]
    expect(id).toBe('a')
    expect(Date.parse(body.start_date) - Date.parse(t.start_date)).toBe(2 * DAY_MS)
    expect(Date.parse(body.due_date) - Date.parse(t.due_date)).toBe(2 * DAY_MS)
    expect(body.completed).toBe(false)
  })

  it('does not PATCH when the drag ends where it started', async () => {
    const t = { id: 'a', title: 'a', start_date: day(10), due_date: day(12) }
    const { api } = host([t])
    api.onBarDown({ button: 0, clientX: 40, preventDefault() {}, stopPropagation() {} }, t, 'move')
    window.dispatchEvent(new PointerEvent('pointermove', { clientX: 44 })) // < half a day
    window.dispatchEvent(new PointerEvent('pointerup'))
    await Promise.resolve()
    expect(update).not.toHaveBeenCalled()
  })

  it('never lets a resized start cross its due date', async () => {
    const t = { id: 'a', title: 'a', start_date: day(10), due_date: day(12) }
    const { api } = host([t])
    api.onBarDown({ button: 0, clientX: 0, preventDefault() {}, stopPropagation() {} }, t, 'start')
    window.dispatchEvent(new PointerEvent('pointermove', { clientX: 34 * 9 })) // way past the due
    window.dispatchEvent(new PointerEvent('pointerup'))
    await Promise.resolve()
    const [, body] = update.mock.calls[0]
    expect(Date.parse(body.start_date)).toBe(Date.parse(t.due_date))
  })
})
