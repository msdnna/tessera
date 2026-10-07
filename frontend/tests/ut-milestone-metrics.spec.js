// Stage page maths (#2993) and stage state helpers (#2971).
import { describe, it, expect } from 'vitest'
import {
  dayMs,
  daysRange,
  isWeekend,
  forecast,
  niceAxis,
  burndown,
  ringDash,
  percent,
  paceAverage,
} from '@/utils/milestoneMetrics'
import {
  stageState,
  daysLeft,
  stageProgress,
  sortStages,
  stagePath,
  todayUTC,
} from '@/utils/milestones'

const pt = (date, scope, done, scopeEst = scope * 60, doneEst = done * 60) => ({
  date,
  scope,
  done,
  scope_est: scopeEst,
  done_est: doneEst,
})

describe('milestoneMetrics', () => {
  it('walks UTC days and knows weekends', () => {
    expect(daysRange('2026-10-12', '2026-10-14')).toEqual([
      '2026-10-12',
      '2026-10-13',
      '2026-10-14',
    ])
    expect(daysRange('2026-10-14', '2026-10-12')).toEqual([])
    expect(isWeekend(dayMs('2026-10-17'))).toBe(true) // Saturday
    expect(isWeekend(dayMs('2026-10-19'))).toBe(false)
  })

  it('projects the pace of the last working days to the end', () => {
    // Mon 12 … Wed 21 Oct: 2 done per working day over the last five.
    const series = [
      pt('2026-10-12', 30, 0),
      pt('2026-10-13', 30, 2),
      pt('2026-10-14', 30, 4),
      pt('2026-10-15', 30, 6),
      pt('2026-10-16', 30, 8),
      pt('2026-10-17', 30, 8),
      pt('2026-10-18', 30, 8),
      pt('2026-10-19', 30, 10),
      pt('2026-10-20', 30, 12),
      pt('2026-10-21', 30, 14),
    ]
    const f = forecast(series, '2026-10-26')
    expect(f.pace).toBe(2)
    // Thu 22, Fri 23, Mon 26 are left → 14 + 3×2 = 20.
    expect(f.value).toBe(20)
    // Capped at the scope.
    expect(forecast(series, '2026-11-30').value).toBe(30)
    expect(forecast([], '2026-10-26')).toBe(null)
    expect(forecast(series, '2026-10-26', 'estimate').pace).toBe(120)
  })

  it('picks a nice axis and the burn-down', () => {
    expect(niceAxis(31)).toEqual({ top: 40, ticks: [0, 10, 20, 30, 40] })
    expect(niceAxis(0).top).toBeGreaterThan(0)
    expect(burndown([pt('2026-10-12', 10, 3)])).toEqual([{ date: '2026-10-12', left: 7 }])
  })

  it('draws rings and averages the pace', () => {
    const r = ringDash(50, 10)
    expect(parseFloat(r.dasharray)).toBeCloseTo(Math.PI * 10)
    expect(percent(18, 31)).toBe(58)
    expect(percent(0, 0)).toBe(0)
    expect(
      paceAverage([{ done: 21 }, { done: 24 }, { done: 19 }, { done: 18, current: true }]),
    ).toBe(21)
    expect(paceAverage([{ done: 5, current: true }])).toBe(null)
  })
})

describe('stage state', () => {
  const today = Date.UTC(2026, 9, 21)
  const active = {
    state: 'active',
    start_date: '2026-10-12T00:00:00Z',
    due_date: '2026-10-26T00:00:00Z',
  }
  const planned = { state: 'active', start_date: '2026-10-27T00:00:00Z' }
  const closed = { state: 'closed', closed_at: '2026-10-11T09:00:00Z' }

  it('derives «planned» from a start ahead', () => {
    expect(stageState(active, today)).toBe('active')
    expect(stageState(planned, today)).toBe('planned')
    expect(stageState(closed, today)).toBe('closed')
    expect(stageState({ state: 'active' }, today)).toBe('active')
  })

  it('counts the days left and the progress', () => {
    expect(daysLeft(active, today)).toBe(5)
    expect(daysLeft({ due_date: '2026-10-19T00:00:00Z' }, today)).toBe(-2)
    expect(daysLeft({}, today)).toBe(null)
    expect(stageProgress({ task_count: 31, done_count: 18 })).toEqual({
      done: 18,
      total: 31,
      pct: 58,
    })
  })

  it('orders active → planned → closed', () => {
    const list = sortStages(
      [
        { id: 'c', ...closed },
        { id: 'p', ...planned },
        { id: 'a', ...active },
        { id: 'a2', state: 'active', due_date: '2026-10-30T00:00:00Z' },
      ],
      today,
    )
    expect(list.map((m) => m.id)).toEqual(['a', 'a2', 'p', 'c'])
  })

  it('builds the page address from slugs', () => {
    expect(stagePath('tessera-web', { slug: 'sprint-42', id: 'x' })).toBe(
      '/project/tessera-web/stage/sprint-42',
    )
    expect(typeof todayUTC()).toBe('number')
  })
})
