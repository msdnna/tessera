import { describe, it, expect } from 'vitest'
import { mineOf, buildSections, reviewCount, weekBars, dueDay } from '@/utils/homeSections'

// The start screen's lists and counters (#2960), counted per user on the client.
const now = new Date(2026, 9, 7, 15, 0) // Wed 7 Oct 2026
const at = (d, h = 0) => new Date(2026, 9, d, h, 30).toISOString() // a timed due
const allDay = (d) => new Date(Date.UTC(2026, 9, d)).toISOString() // UTC midnight
const task = (id, extra = {}) => ({ id, assignee_ids: ['me'], ...extra })

describe('homeSections', () => {
  it('keeps my open tasks only', () => {
    const list = [task('a'), task('b', { completed_at: at(6) }), task('c', { assignee_ids: ['x'] })]
    expect(mineOf(list, 'me').map((t) => t.id)).toEqual(['a'])
  })

  it('sorts by due date into overdue / today / this week / later / none', () => {
    const s = buildSections(
      [
        task('later', { due_date: allDay(20) }),
        task('over', { due_date: at(5, 10) }),
        task('today', { due_date: at(7, 18) }),
        task('sun', { due_date: allDay(11) }),
        task('none'),
      ],
      { now, firstDayOfWeek: 1 },
    )
    expect(Object.fromEntries(Object.entries(s).map(([k, v]) => [k, v.map((t) => t.id)]))).toEqual({
      overdue: ['over'],
      today: ['today'],
      week: ['sun'],
      later: ['later'],
      nodate: ['none'],
    })
  })

  it('reads an all-day due by its UTC calendar day', () => {
    expect(new Date(dueDay(allDay(9))).getDate()).toBe(9)
  })

  it('counts what waits for my review: mine, open, in a review column', () => {
    const list = [
      { id: 'a', created_by: 'me', column_name_key: 'review' },
      { id: 'b', created_by: 'me', column_name: 'На рассмотрении' },
      { id: 'c', created_by: 'me', column_name_key: 'review', completed_at: at(6) },
      { id: 'd', created_by: 'x', column_name_key: 'review' },
    ]
    expect(reviewCount(list, 'me')).toBe(2)
  })

  it('stacks the week from Monday: done and open per day', () => {
    const w = weekBars(
      [
        task('a', { due_date: at(5), completed_at: at(5) }),
        task('b', { due_date: at(7) }),
        task('c', { due_date: allDay(11) }),
        task('d', { due_date: at(12) }), // next week
      ],
      'me',
      { now, firstDayOfWeek: 1 },
    )
    expect(w.days.map((d) => [d.done, d.open])).toEqual([
      [1, 0],
      [0, 0],
      [0, 1],
      [0, 0],
      [0, 0],
      [0, 0],
      [0, 1],
    ])
    expect(w.days[2].today).toBe(true)
    expect([w.done, w.total]).toEqual([1, 3])
  })
})
