import { describe, it, expect } from 'vitest'
import {
  DATE_PRESETS,
  createFormatters,
  datePattern,
  datePresetSamples,
  dateTimePattern,
  isDateOnly,
  localeTag,
  normalizeDatePreset,
  normalizePrefs,
  timePattern,
  toDate,
} from '@/utils/format'

// The pure formatting core (#2798). Every case pins preferences explicitly, so
// nothing here depends on the machine's timezone or language.
const RU_MSK = { language: 'ru', timezone: 'Europe/Moscow', timeFormat: '24h', dateFormat: 'short' }
const EN_NY = {
  language: 'en',
  timezone: 'America/New_York',
  timeFormat: '12h',
  dateFormat: 'medium',
}

// An instant late enough in UTC that Moscow (+3) has already rolled over to the
// next day while New York (-5) is still on the previous one — the whole point of
// honouring the timezone preference.
const LATE = '2026-01-01T23:30:00Z'

describe('preference normalization', () => {
  it('maps legacy date-fns patterns onto presets', () => {
    expect(normalizeDatePreset('dd.MM.yyyy')).toBe('short')
    expect(normalizeDatePreset('dd/MM/yyyy')).toBe('short')
    expect(normalizeDatePreset('MM/dd/yyyy')).toBe('short')
    expect(normalizeDatePreset('yyyy-MM-dd')).toBe('iso')
  })

  it('keeps a known preset and falls back for anything else', () => {
    for (const p of DATE_PRESETS) expect(normalizeDatePreset(p)).toBe(p)
    expect(normalizeDatePreset('nonsense')).toBe('short')
    expect(normalizeDatePreset(undefined)).toBe('short')
  })

  it('fills the gaps of a partial preference object', () => {
    const p = normalizePrefs({ language: 'en' })
    expect(p).toEqual({
      language: 'en',
      timezone: '',
      timeFormat: '24h',
      dateFormat: 'short',
      weekStart: 1,
    })
    // week_start = 0 (Sunday) is a real value, not a missing one.
    expect(normalizePrefs({ weekStart: 0 }).weekStart).toBe(0)
  })

  it('picks the BCP-47 tag from the language preference', () => {
    expect(localeTag('en')).toBe('en-GB')
    expect(localeTag('ru')).toBe('ru-RU')
    expect(localeTag(undefined)).toBe('ru-RU')
  })

  it('parses dates and rejects junk', () => {
    expect(toDate('')).toBeNull()
    expect(toDate(null)).toBeNull()
    expect(toDate('garbage')).toBeNull()
    expect(toDate('2026-06-15T00:00:00Z')).toBeInstanceOf(Date)
  })
})

describe('formatDate / formatDateTime honour the timezone preference', () => {
  it('renders the same instant on different days east and west of UTC', () => {
    // 23:30 UTC is already 02:30 the next day in Moscow, still 18:30 in New York.
    expect(createFormatters(RU_MSK).formatDate(LATE)).toBe('02.01.2026')
    expect(createFormatters(EN_NY).formatDate(LATE)).toBe('1 Jan 2026')
  })

  it('renders the clock in the chosen timezone, in the chosen 12/24h form', () => {
    expect(createFormatters(RU_MSK).formatTime(LATE)).toBe('02:30')
    // hour: '2-digit' keeps the leading zero in the 12h form too, as before #2798.
    expect(createFormatters(EN_NY).formatTime(LATE)).toBe('06:30 pm')
  })

  it('lets an explicit timeZone override the preference', () => {
    const f = createFormatters(RU_MSK)
    expect(f.formatDate(LATE, { timeZone: 'UTC' })).toBe('01.01.2026')
  })

  it('keeps date and clock in one timezone for the iso preset', () => {
    const f = createFormatters({ ...RU_MSK, dateFormat: 'iso' })
    expect(f.formatDate(LATE)).toBe('2026-01-02')
    expect(f.formatDateTime(LATE)).toBe('2026-01-02 02:30')
  })
})

describe('date-only values', () => {
  const DAY = '2026-06-15T00:00:00Z'

  it('recognizes a pure UTC midnight as date-only', () => {
    expect(isDateOnly(new Date(DAY))).toBe(true)
    expect(isDateOnly(new Date('2026-06-15T00:00:01Z'))).toBe(false)
  })

  it('stays on the same calendar day in every timezone', () => {
    // Without the UTC rule a user west of UTC would read this as the 14th.
    expect(createFormatters(RU_MSK).formatDate(DAY)).toBe('15.06.2026')
    expect(createFormatters(EN_NY).formatDate(DAY)).toBe('15 Jun 2026')
    expect(createFormatters({ ...RU_MSK, timezone: 'Pacific/Kiritimati' }).formatDate(DAY)).toBe(
      '15.06.2026',
    )
  })
})

describe('presets', () => {
  it('renders each preset in the current language', () => {
    const d = '2026-12-31T12:00:00Z'
    const ru = createFormatters({ ...RU_MSK, timezone: 'UTC' })
    expect(ru.formatDate(d, { preset: 'short' })).toBe('31.12.2026')
    expect(ru.formatDate(d, { preset: 'medium' })).toBe('31 дек. 2026 г.')
    expect(ru.formatDate(d, { preset: 'long' })).toBe('31 декабря 2026 г.')
    expect(ru.formatDate(d, { preset: 'iso' })).toBe('2026-12-31')

    const en = createFormatters({ ...EN_NY, timezone: 'UTC' })
    expect(en.formatDate(d, { preset: 'short' })).toBe('31/12/2026')
    expect(en.formatDate(d, { preset: 'long' })).toBe('31 December 2026')
  })

  it('lets an explicit field set replace the preset instead of merging into it', () => {
    const f = createFormatters({ ...RU_MSK, dateFormat: 'long', timezone: 'UTC' })
    // "day + short month" means exactly that — no year smuggled in from `long`.
    expect(f.formatDate('2026-12-31T12:00:00Z', { day: '2-digit', month: 'short' })).toBe('31 дек.')
  })

  it('offers a labelled sample of every preset for the settings picker', () => {
    const samples = datePresetSamples(RU_MSK)
    expect(samples.map((s) => s.value)).toEqual(DATE_PRESETS)
    expect(samples[0].label).toBe('31.12.2026')
    expect(samples.at(-1).label).toBe('2026-12-31')
    expect(datePresetSamples({ ...EN_NY }).at(1).label).toBe('31 Dec 2026')
  })
})

describe('date-fns patterns for the Naive UI pickers', () => {
  it('derives the pattern from the preset and the language', () => {
    expect(datePattern(RU_MSK)).toBe('dd.MM.yyyy')
    expect(datePattern({ ...RU_MSK, dateFormat: 'iso' })).toBe('yyyy-MM-dd')
    expect(datePattern({ ...EN_NY, dateFormat: 'short' })).toBe('dd/MM/yyyy')
  })

  it('takes the token width from the preset, not from the reference date', () => {
    // `medium`/`long` ask for a numeric day, so the picker must print "5 дек.",
    // not "05 дек." — the 31st of the reference date must not widen the token.
    // ICU separates the ru year from " г." with a narrow no-break space, so the
    // literal is matched loosely.
    expect(datePattern({ language: 'ru', dateFormat: 'medium' })).toMatch(/^d MMM yyyy'\s?г\.'$/u)
    expect(datePattern({ language: 'en', dateFormat: 'long' })).toBe('d MMMM yyyy')
  })

  it('quotes literal words so date-fns does not read them as tokens', () => {
    // ru long dates end in " г." — bare letters would parse as pattern tokens.
    const p = datePattern({ ...RU_MSK, dateFormat: 'long' })
    expect(p).toContain("'")
    expect(p).toContain('MMMM')
  })

  it('accepts a legacy pattern as input and still yields a valid one', () => {
    expect(datePattern({ language: 'ru', dateFormat: 'dd.MM.yyyy' })).toBe('dd.MM.yyyy')
  })

  it('follows the 12/24h preference for the time half', () => {
    expect(timePattern(RU_MSK)).toBe('HH:mm')
    expect(timePattern(EN_NY)).toBe('hh:mm a')
    expect(dateTimePattern(RU_MSK)).toBe('dd.MM.yyyy HH:mm')
  })
})

describe('formatRelative', () => {
  // A fixed "now" so the test never straddles midnight.
  const NOW = new Date('2026-06-15T09:00:00Z')
  const day = (n) => new Date(Date.UTC(2026, 5, 15 + n)).toISOString()

  it('names today, tomorrow and yesterday in the current language', () => {
    const ru = createFormatters(RU_MSK)
    expect(ru.formatRelative(day(0), { now: NOW })).toBe('сегодня')
    expect(ru.formatRelative(day(1), { now: NOW })).toBe('завтра')
    expect(ru.formatRelative(day(-1), { now: NOW })).toBe('вчера')

    const en = createFormatters(EN_NY)
    expect(en.formatRelative(day(0), { now: NOW })).toBe('Today')
    expect(en.formatRelative(day(1), { now: NOW })).toBe('Tomorrow')
  })

  it('capitalizes the Russian label only in the long form', () => {
    const ru = createFormatters(RU_MSK)
    expect(ru.formatRelative(day(0), { now: NOW, long: true })).toBe('Сегодня')
  })

  it('uses a weekday inside the current week and nothing beyond it', () => {
    // 2026-06-15 is a Monday, so Thursday the 18th is still this week.
    const ru = createFormatters(RU_MSK)
    expect(ru.formatRelative(day(3), { now: NOW })).toBe('чт')
    expect(ru.formatRelative(day(3), { now: NOW, long: true })).toBe('Четверг')
    // Next Monday is a different week — no relative label at all.
    expect(ru.formatRelative(day(7), { now: NOW })).toBe('')
  })

  it('moves the week boundary with the week-start preference', () => {
    // Sunday the 21st: inside the week for a Monday start, the next week's first
    // day for a Sunday start.
    const mon = createFormatters({ ...RU_MSK, weekStart: 1 })
    const sun = createFormatters({ ...RU_MSK, weekStart: 0 })
    expect(mon.formatRelative(day(6), { now: NOW })).toBe('вс')
    expect(sun.formatRelative(day(6), { now: NOW })).toBe('')
  })

  it('reads "today" in the user timezone, not the browser one', () => {
    // 22:00 UTC on the 15th is already the 16th in Moscow, so a due on the 16th
    // is "today" there and "tomorrow" for a UTC user.
    const now = new Date('2026-06-15T22:00:00Z')
    expect(createFormatters(RU_MSK).formatRelative(day(1), { now })).toBe('сегодня')
    expect(createFormatters({ ...RU_MSK, timezone: 'UTC' }).formatRelative(day(1), { now })).toBe(
      'завтра',
    )
  })
})

describe('formatDue', () => {
  const NOW = new Date('2026-06-15T09:00:00Z')

  it('appends the clock only when the due carries a time of day', () => {
    const f = createFormatters(RU_MSK)
    expect(f.formatDue('2026-06-15T00:00:00Z', { now: NOW })).toBe('сегодня')
    // 12:30 UTC = 15:30 in Moscow.
    expect(f.formatDue('2026-06-15T12:30:00Z', { now: NOW })).toBe('сегодня, 15:30')
  })

  it('falls back to day + month, adding the year outside the current one', () => {
    const f = createFormatters(RU_MSK)
    expect(f.formatDue('2026-09-01T00:00:00Z', { now: NOW })).toBe('01 сент.')
    expect(f.formatDue('2027-09-01T00:00:00Z', { now: NOW })).toBe('01 сент. 2027 г.')
  })

  it('returns "" for an empty or unparsable value', () => {
    const f = createFormatters(RU_MSK)
    expect(f.formatDue('')).toBe('')
    expect(f.formatDue(null)).toBe('')
    expect(f.formatDue('garbage')).toBe('')
  })
})

describe('numbers and lists', () => {
  it('formats numbers in the current locale', () => {
    // ru groups with a non-breaking space, en-GB with a comma.
    expect(createFormatters(RU_MSK).formatNumber(1234567.5)).toMatch(/^1\s?234\s?567,5$/)
    expect(createFormatters(EN_NY).formatNumber(1234567.5)).toBe('1,234,567.5')
    expect(createFormatters(RU_MSK).formatNumber(null)).toBe('')
    expect(createFormatters(RU_MSK).formatNumber('nope')).toBe('')
  })

  it('joins a list with the locale conjunction', () => {
    const ru = createFormatters(RU_MSK)
    expect(ru.formatList(['название', 'срок', 'приоритет'])).toBe('название, срок и приоритет')
    expect(createFormatters(EN_NY).formatList(['title', 'due'])).toBe('title and due')
    expect(ru.formatList([])).toBe('')
    expect(ru.formatList(['срок', '', null])).toBe('срок')
  })
})

describe('calendar fields', () => {
  it('reports the year the user sees, not the browser one', () => {
    // 22:00 UTC on 31 December is already next year in Moscow.
    const eve = '2026-12-31T22:00:00Z'
    expect(createFormatters(RU_MSK).parts(eve).year).toBe(2027)
    expect(createFormatters({ ...RU_MSK, timezone: 'UTC' }).parts(eve).year).toBe(2026)
    expect(createFormatters(RU_MSK).parts('garbage')).toBeNull()
  })
})
