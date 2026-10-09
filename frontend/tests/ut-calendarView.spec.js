// «Календарь» (#3019): the pure half — whole-week grids, the due a drop on a day
// writes (a date-only due stays date-only in every zone, a timed one keeps its
// wall-clock time in the profile zone), how many pills a cell shows.
import { describe, it, expect } from 'vitest'
import {
  monthGrid,
  weekGrid,
  addDays,
  addMonths,
  movedDates,
  shiftInstant,
  dateOnlyISO,
  pillsShown,
  dayOrder,
} from '@/utils/calendarView'
import { createFormatters } from '@/utils/format'

describe('grids', () => {
  it('covers the month in whole weeks from the chosen first day', () => {
    // October 2026 starts on a Thursday and ends on a Saturday.
    const mon = monthGrid({ year: 2026, month: 9, day: 8 }, 1)
    expect(mon.rows).toBe(5)
    expect(mon.cells[0]).toMatchObject({ year: 2026, month: 8, day: 28, inMonth: false })
    expect(mon.cells.at(-1)).toMatchObject({ month: 10, day: 1 })
    const sun = monthGrid({ year: 2026, month: 9, day: 8 }, 0)
    expect(sun.cells[0].day).toBe(27)
    // February 2026 starts on a Sunday: four weeks with Sunday first.
    expect(monthGrid({ year: 2026, month: 1, day: 1 }, 0).rows).toBe(4)
  })

  it('marks weekends and gives the week from its first day', () => {
    const w = weekGrid({ year: 2026, month: 9, day: 8 }, 1)
    expect(w.map((c) => c.day)).toEqual([5, 6, 7, 8, 9, 10, 11])
    expect(w.filter((c) => c.weekend).map((c) => c.day)).toEqual([10, 11])
  })

  it('steps days and months on calendar fields', () => {
    expect(addDays({ year: 2026, month: 11, day: 31 }, 1)).toEqual({ year: 2027, month: 0, day: 1 })
    expect(addMonths({ year: 2026, month: 0, day: 31 }, 1)).toEqual({
      year: 2026,
      month: 1,
      day: 28,
    })
  })
})

describe('movedDates', () => {
  const msk = createFormatters({ timezone: 'Europe/Moscow', language: 'ru' })
  const la = createFormatters({ timezone: 'America/Los_Angeles', language: 'ru' })

  it('keeps a date-only due date-only, the same day for every reader', () => {
    const task = { due_date: '2026-10-28T00:00:00Z' }
    for (const fmt of [msk, la]) {
      expect(movedDates(task, { year: 2026, month: 9, day: 23 }, fmt)).toEqual({
        due_date: '2026-10-23T00:00:00.000Z',
        start_date: null,
      })
    }
  })

  it('reads the prod form of a date-only due (+03:00 offset) the same way', () => {
    const task = { due_date: '2026-10-28T03:00:00+03:00' }
    expect(movedDates(task, { year: 2026, month: 9, day: 23 }, la).due_date).toBe(
      '2026-10-23T00:00:00.000Z',
    )
  })

  it('moves a timed due by days in the profile zone, keeping its time', () => {
    // 28 Oct 10:00 in Moscow.
    const task = { due_date: '2026-10-28T07:00:00Z', start_date: '2026-10-26T07:00:00Z' }
    expect(movedDates(task, { year: 2026, month: 9, day: 23 }, msk)).toEqual({
      due_date: '2026-10-23T07:00:00.000Z',
      start_date: '2026-10-21T07:00:00.000Z',
    })
  })

  it('keeps the wall-clock time across a clock change', () => {
    // Los Angeles leaves DST on 1 Nov 2026: 09:00 PDT = 16:00Z, 09:00 PST = 17:00Z.
    expect(shiftInstant('2026-10-30T16:00:00Z', 3, la)).toBe('2026-11-02T17:00:00.000Z')
  })

  it('gives an undated task the day as a date-only due, and does nothing on its own day', () => {
    expect(movedDates({ due_date: null }, { year: 2026, month: 9, day: 5 }, msk).due_date).toBe(
      dateOnlyISO({ year: 2026, month: 9, day: 5 }),
    )
    expect(
      movedDates({ due_date: '2026-10-05T00:00:00Z' }, { year: 2026, month: 9, day: 5 }, msk),
    ).toBeNull()
  })
})

describe('cells', () => {
  it('shows every task that fits, else three at most and «+N»', () => {
    expect(pillsShown(2, 138)).toBe(2)
    expect(pillsShown(3, 138)).toBe(3)
    expect(pillsShown(5, 138)).toBe(3)
    expect(pillsShown(5, 104)).toBe(2)
    expect(pillsShown(4, 60)).toBe(0)
  })

  it('reads a day in time order, the all-day tasks after the timed ones', () => {
    const list = [
      { id: 'a', due_date: '2026-10-08T00:00:00Z' },
      { id: 'b', due_date: '2026-10-08T12:00:00Z' },
      { id: 'c', due_date: '2026-10-08T07:00:00Z' },
    ]
    expect(dayOrder(list).map((t) => t.id)).toEqual(['c', 'b', 'a'])
  })
})
