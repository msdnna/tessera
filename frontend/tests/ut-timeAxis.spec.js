import { describe, it, expect } from 'vitest'
import {
  DAY_MS,
  HOUR_MS,
  MONTHS,
  WD,
  startOfDay,
  isAllDayMs,
  tierFor,
  WEEKS_MAX_DAYW,
  HOURS_MIN_DAYW,
  xAt,
  barSpan,
  anchorMs,
  buildDays,
  buildMonthBands,
  buildWeekBands,
  hourStepFor,
  hourTicksInWindow,
} from '@/utils/timeAxis'

describe('constants', () => {
  it('exposes ms/label tables', () => {
    expect(DAY_MS).toBe(86400000)
    expect(HOUR_MS).toBe(3600000)
    expect(MONTHS).toHaveLength(12)
    expect(WD).toEqual(['вс', 'пн', 'вт', 'ср', 'чт', 'пт', 'сб'])
  })
})

describe('startOfDay / isAllDayMs', () => {
  it('startOfDay strips the time in local tz', () => {
    const ms = new Date(2026, 5, 15, 13, 45).getTime()
    const s = new Date(startOfDay(ms))
    expect(s.getHours()).toBe(0)
    expect(s.getDate()).toBe(15)
  })

  it('isAllDayMs is true only at UTC midnight', () => {
    expect(isAllDayMs(Date.UTC(2026, 5, 15, 0, 0, 0))).toBe(true)
    expect(isAllDayMs(Date.UTC(2026, 5, 15, 9, 30, 0))).toBe(false)
  })
})

describe('tierFor', () => {
  it('maps px/day to a zoom tier at the thresholds', () => {
    expect(tierFor(WEEKS_MAX_DAYW - 1)).toBe('weeks')
    expect(tierFor(WEEKS_MAX_DAYW)).toBe('days')
    expect(tierFor(HOURS_MIN_DAYW - 1)).toBe('days')
    expect(tierFor(HOURS_MIN_DAYW)).toBe('hours')
  })
})

describe('xAt', () => {
  it('maps time offset to pixels', () => {
    const start = 0
    expect(xAt(2 * DAY_MS, start, 40)).toBe(80)
    expect(xAt(start, start, 40)).toBe(0)
  })
})

describe('barSpan', () => {
  const rangeStart = startOfDay(new Date(2026, 5, 1).getTime())
  const dayW = 40

  it('null when there is neither start nor due', () => {
    expect(barSpan({ start: null, due: null, tier: 'days', rangeStart, dayW })).toBeNull()
  })

  it('day tier: a due-only all-day task fills its whole day', () => {
    const due = startOfDay(new Date(2026, 5, 3).getTime())
    const s = barSpan({ start: null, due, tier: 'days', rangeStart, dayW })
    expect(s.hasStart).toBe(false)
    expect(s.hasDue).toBe(true)
    expect(s.width).toBe(dayW) // day start → day end
    expect(s.left).toBe(2 * dayW)
  })

  it('enforces the minimum width', () => {
    const start = startOfDay(new Date(2026, 5, 1).getTime())
    const s = barSpan({ start, due: start, tier: 'hours', rangeStart, dayW, minW: 6 })
    // hours tier honours clock time; identical start=due (all-day midnight) still spans a day
    expect(s.width).toBeGreaterThanOrEqual(6)
  })

  it('hours tier honours a timed endpoint', () => {
    const start = Date.UTC(2026, 5, 1, 6, 0, 0)
    const due = Date.UTC(2026, 5, 1, 12, 0, 0)
    const rs = startOfDay(start)
    const s = barSpan({ start, due, tier: 'hours', rangeStart: rs, dayW: 240 })
    // 6h span at 240px/day = 10px/hr → 60px
    expect(Math.round(s.width)).toBe(60)
  })
})

describe('anchorMs', () => {
  it('honours clock time only in the hours tier for timed values', () => {
    const timed = Date.UTC(2026, 5, 1, 9, 0, 0)
    expect(anchorMs(timed, 'hours')).toBe(timed)
    expect(anchorMs(timed, 'days')).toBe(startOfDay(timed))
    const allDay = Date.UTC(2026, 5, 1, 0, 0, 0)
    expect(anchorMs(allDay, 'hours')).toBe(startOfDay(allDay))
  })
})

describe('buildDays', () => {
  it('builds day cells with weekend/today flags', () => {
    const rangeStart = startOfDay(new Date(2026, 5, 1).getTime()) // Jun 1 2026 = Mon
    const today = startOfDay(new Date(2026, 5, 2).getTime())
    const days = buildDays(rangeStart, 7, today)
    expect(days).toHaveLength(7)
    expect(days[0].day).toBe(1)
    expect(days[1].isToday).toBe(true)
    // Jun 6/7 2026 = Sat/Sun
    expect(days[5].weekend).toBe(true)
    expect(days[6].weekend).toBe(true)
  })
})

describe('buildMonthBands', () => {
  it('groups consecutive same-month days into bands', () => {
    const rangeStart = startOfDay(new Date(2026, 5, 28).getTime())
    const days = buildDays(rangeStart, 6, 0) // Jun 28-30 + Jul 1-3
    const bands = buildMonthBands(days)
    expect(bands).toHaveLength(2)
    expect(bands[0].span).toBe(3)
    expect(bands[1].span).toBe(3)
    expect(bands[0].label).toContain('июн')
  })
})

describe('buildWeekBands', () => {
  it('breaks bands on Monday and labels day ranges', () => {
    const rangeStart = startOfDay(new Date(2026, 5, 3).getTime()) // Jun 3 2026 = Wed
    const days = buildDays(rangeStart, 8, 0) // Wed..next Wed
    const bands = buildWeekBands(days)
    // first partial band Wed-Sun, then Mon-onwards
    expect(bands.length).toBe(2)
    expect(bands[0].label).toContain('–')
  })

  it('a single-day band labels just the day', () => {
    const rangeStart = startOfDay(new Date(2026, 5, 8).getTime()) // Mon
    const days = buildDays(rangeStart, 1, 0)
    const bands = buildWeekBands(days)
    expect(bands[0].label).toBe('8')
  })
})

describe('hourStepFor', () => {
  it('picks a step keeping labels ~34px apart, 0 when too tight', () => {
    expect(hourStepFor(240)).toBe(4) // 10px/hr → 4h*10=40 ≥34
    expect(hourStepFor(24 * 34)).toBe(1) // exactly 34px/hr
    expect(hourStepFor(20)).toBe(0) // even 12h step < 34px
  })
})

describe('hourTicksInWindow', () => {
  it('returns [] when the step is 0', () => {
    expect(hourTicksInWindow(3, 20, 0, 60)).toEqual([])
  })

  it('emits ticks in the visible window, skipping the day boundary', () => {
    const dayW = 240 // step = 4
    const ticks = hourTicksInWindow(1, dayW, 0, dayW)
    // hours 4,8,12,16,20 within one day
    expect(ticks.map((t) => t.label)).toEqual(['04', '08', '12', '16', '20'])
    expect(ticks[0].left).toBe((4 / 24) * dayW)
  })
})
