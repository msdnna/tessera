import { describe, it, expect } from 'vitest'
import {
  FREQ_OPTIONS,
  TRIGGER_OPTIONS,
  pluralRu,
  unitLabel,
  nextOccurrence,
  occurrenceKeys,
} from '@/utils/recurrence'

// Local dayKey mirror for assertions (avoids TZ drift from ISO strings).
const key = (d) =>
  `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`

describe('option lists', () => {
  it('expose the expected frequency and trigger values', () => {
    expect(FREQ_OPTIONS.map((o) => o.value)).toEqual([
      '',
      'daily',
      'weekly',
      'monthly',
      'yearly',
      'custom',
    ])
    expect(TRIGGER_OPTIONS.map((o) => o.value)).toEqual(['complete', 'column', 'schedule'])
  })
})

describe('pluralRu / unitLabel', () => {
  it('picks russian plural forms', () => {
    const f = ['день', 'дня', 'дней']
    expect(pluralRu(1, f)).toBe('день')
    expect(pluralRu(2, f)).toBe('дня')
    expect(pluralRu(5, f)).toBe('дней')
    expect(pluralRu(11, f)).toBe('дней') // teens → many
    expect(pluralRu(21, f)).toBe('день')
  })

  it('labels a frequency unit, empty for unknown', () => {
    expect(unitLabel('weekly', 1)).toBe('неделю')
    expect(unitLabel('monthly', 3)).toBe('месяца')
    expect(unitLabel('bogus', 1)).toBe('')
  })
})

describe('nextOccurrence', () => {
  it('returns null without a rule/freq', () => {
    expect(nextOccurrence(null, new Date())).toBeNull()
    expect(nextOccurrence({}, new Date())).toBeNull()
    expect(nextOccurrence({ freq: 'unknown' }, new Date())).toBeNull()
  })

  it('daily advances by the interval and skips weekends when asked', () => {
    const wed = new Date(2026, 6, 1, 9, 0, 0) // Wed
    expect(key(nextOccurrence({ freq: 'daily' }, wed))).toBe('2026-07-02')
    expect(key(nextOccurrence({ freq: 'daily', interval: 3 }, wed))).toBe('2026-07-04')
    // Fri +1 lands on Sat → pushed to Mon.
    const fri = new Date(2026, 6, 3, 9, 0, 0)
    expect(key(nextOccurrence({ freq: 'daily', skip_weekends: true }, fri))).toBe('2026-07-06')
  })

  it('weekly without weekdays advances 7*interval days', () => {
    const wed = new Date(2026, 6, 1, 9)
    expect(key(nextOccurrence({ freq: 'weekly' }, wed))).toBe('2026-07-08')
    expect(key(nextOccurrence({ freq: 'weekly', interval: 2 }, wed))).toBe('2026-07-15')
  })

  it('weekly with weekdays picks the next matching day in the week', () => {
    const wed = new Date(2026, 6, 1, 9) // Wed = day 3
    // next Friday (5) this week
    expect(key(nextOccurrence({ freq: 'weekly', weekdays: [5] }, wed))).toBe('2026-07-03')
    // only Monday (1): wraps to next week's Monday
    expect(key(nextOccurrence({ freq: 'weekly', weekdays: [1] }, wed))).toBe('2026-07-06')
  })

  it('monthly advances months and clamps the target day to month length', () => {
    const jan31 = new Date(2026, 0, 31, 9)
    // day defaults to from's date (31) → clamped to Feb length
    expect(key(nextOccurrence({ freq: 'monthly' }, jan31))).toBe('2026-02-28')
    expect(key(nextOccurrence({ freq: 'monthly', interval: 2, day: 15 }, jan31))).toBe('2026-03-15')
  })

  it('yearly advances a year, honouring month/day', () => {
    const d = new Date(2026, 5, 10, 9)
    expect(key(nextOccurrence({ freq: 'yearly' }, d))).toBe('2027-06-10')
    expect(key(nextOccurrence({ freq: 'yearly', month: 12, day: 25 }, d))).toBe('2027-12-25')
  })

  it('custom returns the next explicit date after "from", else null', () => {
    const from = new Date(2026, 6, 1, 9)
    const rule = { freq: 'custom', dates: ['2026-06-01', '2026-07-05', '2026-08-01'] }
    expect(key(nextOccurrence(rule, from))).toBe('2026-07-05')
    const late = new Date(2026, 8, 1, 9)
    expect(nextOccurrence(rule, late)).toBeNull()
  })
})

describe('occurrenceKeys', () => {
  it('returns empty set without ts/rule', () => {
    expect(occurrenceKeys(null, null).size).toBe(0)
    expect(occurrenceKeys({ freq: 'daily' }, null).size).toBe(0)
  })

  it('custom returns exactly the listed dates', () => {
    const keys = occurrenceKeys({ freq: 'custom', dates: ['2026-01-01', '2026-02-02'] }, Date.now())
    expect([...keys].sort()).toEqual(['2026-01-01', '2026-02-02'])
  })

  it('includes the start day plus n upcoming occurrences', () => {
    const from = new Date(2026, 6, 1, 9).getTime()
    const keys = occurrenceKeys({ freq: 'daily' }, from, 3)
    expect(keys.has('2026-07-01')).toBe(true)
    expect(keys.has('2026-07-04')).toBe(true) // start + 3 daily steps
    expect(keys.size).toBe(4)
  })

  it('a "once" rule shows only the current due plus one next', () => {
    const from = new Date(2026, 6, 1, 9).getTime()
    const keys = occurrenceKeys({ freq: 'daily', once: true }, from, 24)
    expect(keys.size).toBe(2)
  })
})
