import { describe, it, expect } from 'vitest'
import { createFormatters } from '@/utils/format'
import { snoozedDue, taskSnoozeDays } from '@/utils/taskSnooze'

const fmt = createFormatters({ language: 'ru', timezone: 'UTC', timeFormat: '24h', weekStart: 1 })

describe('task snooze (#3108)', () => {
  it('offers tomorrow, the day after, the coming Monday and a week', () => {
    // Wednesday 14 Oct 2026, 10:00 UTC.
    const days = taskSnoozeDays(fmt, new Date(Date.UTC(2026, 9, 14, 10)))
    expect(days.map((d) => [d.key, d.day.day])).toEqual([
      ['tomorrow', 15],
      ['dayAfter', 16],
      ['monday', 19],
      ['week', 21],
    ])
  })

  it('a Monday puts it off to the next Monday, not today', () => {
    const days = taskSnoozeDays(fmt, new Date(Date.UTC(2026, 9, 12, 10)))
    expect(days.find((d) => d.key === 'monday').day.day).toBe(19)
  })

  it('a date-only due stays date-only; a timed due keeps its clock', () => {
    const day = { year: 2026, month: 9, day: 20 }
    expect(snoozedDue('2026-10-12T00:00:00Z', day, fmt, 'UTC')).toBe('2026-10-20T00:00:00.000Z')
    expect(snoozedDue(null, day, fmt, 'UTC')).toBe('2026-10-20T00:00:00.000Z')
    expect(snoozedDue('2026-10-12T15:30:00Z', day, fmt, 'UTC')).toBe('2026-10-20T15:30:00.000Z')
  })
})
