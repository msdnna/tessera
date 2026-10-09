// The pure half of the due picker (#3118, utils/dueModel): wall readings in and
// out, presets, the grid's keys, the repeat and the notification in words.
import { describe, it, expect } from 'vitest'
import { createFormatters } from '@/utils/format'
import {
  readEndpoint,
  writeEndpoint,
  presetDay,
  gridStep,
  ruleSummary,
  recurrenceHint,
  notifySummary,
  effectiveNotify,
  ruleZone,
  zonedInstant,
} from '@/utils/dueModel'

const fmt = createFormatters({ language: 'ru', timezone: 'Europe/Moscow', weekStart: 1 })
const day = (year, month, d) => ({ year, month, day: d })
const SAT = day(2026, 9, 3) // сб, 3 окт. 2026

describe('reading and writing an endpoint', () => {
  it('a pure UTC midnight is a date without a time, the same day everywhere', () => {
    expect(readEndpoint('2026-10-06T00:00:00.000Z', fmt)).toEqual({
      day: day(2026, 9, 6),
      time: null,
    })
    expect(writeEndpoint(day(2026, 9, 6), null, 'America/New_York')).toBe(
      '2026-10-06T00:00:00.000Z',
    )
  })
  it('a clock is the profile zone’s', () => {
    expect(writeEndpoint(day(2026, 9, 9), '18:00', 'Europe/Moscow')).toBe(
      '2026-10-09T15:00:00.000Z',
    )
    expect(readEndpoint('2026-10-09T15:00:00.000Z', fmt)).toEqual({
      day: day(2026, 9, 9),
      time: '18:00',
    })
  })
  it('a clock landing on a UTC midnight keeps a second, so it is not read as a date', () => {
    const iso = writeEndpoint(day(2026, 9, 9), '03:00', 'Europe/Moscow')
    expect(iso).toBe('2026-10-09T00:00:01.000Z')
    expect(readEndpoint(iso, fmt)).toEqual({ day: day(2026, 9, 9), time: '03:00' })
  })
  it('finds the instant across a clock change', () => {
    // 29 Mar 2026, 10:00 in Berlin is already CEST (UTC+2).
    expect(new Date(zonedInstant(day(2026, 2, 29), 10, 0, 'Europe/Berlin')).toISOString()).toBe(
      '2026-03-29T08:00:00.000Z',
    )
  })
  it('the rule’s zone follows the due: UTC for a date, the profile’s for a clock', () => {
    expect(ruleZone('2026-10-06T00:00:00.000Z', 'Europe/Moscow')).toBe('UTC')
    expect(ruleZone('2026-10-06T15:00:00.000Z', 'Europe/Moscow')).toBe('Europe/Moscow')
    expect(ruleZone(null, 'Europe/Moscow')).toBe('Europe/Moscow')
  })
})

describe('presets', () => {
  it('count from today', () => {
    expect(presetDay('today', SAT)).toEqual(SAT)
    expect(presetDay('tomorrow', SAT)).toEqual(day(2026, 9, 4))
    expect(presetDay('weekEnd', SAT)).toEqual(day(2026, 9, 9))
    expect(presetDay('nextMonday', SAT)).toEqual(day(2026, 9, 5))
    expect(presetDay('nextMonday', day(2026, 9, 5))).toEqual(day(2026, 9, 12))
    expect(presetDay('inWeek', SAT)).toEqual(day(2026, 9, 10))
  })
})

describe('grid keys', () => {
  const k = (key, extra = {}) => ({ key, code: '', ...extra })
  it('arrows walk days and weeks, PgUp/PgDn months, T goes home', () => {
    const f = day(2026, 9, 31)
    expect(gridStep(k('ArrowRight'), f, SAT)).toEqual(day(2026, 10, 1))
    expect(gridStep(k('ArrowUp'), f, SAT)).toEqual(day(2026, 9, 24))
    expect(gridStep(k('PageDown'), f, SAT)).toEqual(day(2026, 10, 30))
    expect(gridStep(k('PageUp', { shiftKey: true }), f, SAT)).toEqual(day(2025, 9, 31))
    expect(gridStep(k('е', { code: 'KeyT' }), f, SAT)).toEqual(SAT)
    expect(gridStep(k('x'), f, SAT)).toBe(null)
  })
})

describe('the repeat in words', () => {
  it('names the cadence and the days', () => {
    expect(ruleSummary({ freq: 'weekly', interval: 1, weekdays: [1, 4] }, { fmt })).toBe(
      'Каждую неделю по пн и чт',
    )
    expect(ruleSummary({ freq: 'daily', interval: 3 }, { fmt })).toBe('Каждые 3 дня')
    expect(ruleSummary({ freq: 'daily', interval: 21 }, { fmt })).toBe('Каждый 21 день')
    expect(ruleSummary({ freq: 'daily', interval: 1, skip_weekends: true }, { fmt })).toBe(
      'Каждый день, кроме выходных',
    )
    expect(ruleSummary(null, { fmt })).toBe('Без повтора')
  })
  it('adds the trigger and «один раз»', () => {
    const columns = [{ id: 'c1', name: 'Готово' }]
    expect(
      ruleSummary(
        { freq: 'monthly', interval: 1, trigger: 'column', trigger_column: 'c1', once: true },
        { fmt, columns, due: { day: day(2026, 9, 6), time: null } },
      ),
    ).toBe('Каждый месяц, 6-го числа · при переходе в «Готово» · один раз')
  })
  it('the tooltip names the next one', () => {
    expect(
      recurrenceHint(
        { freq: 'weekly', interval: 1, weekdays: [1, 4], tz: 'UTC' },
        '2026-10-06T00:00:00.000Z',
        { fmt },
      ),
    ).toBe('Повтор: Каждую неделю по пн и чт · следующая — чт, 8 окт.')
  })
})

describe('the due notification in words', () => {
  const D = { due_enabled: true, due_lead_minutes: 60, due_repeat_minutes: 0 }
  it('says where a value comes from', () => {
    expect(notifySummary({ enabled: 'inherit', lead: -1, repeat: -1 }, D)).toBe(
      'По умолчанию — за час',
    )
    expect(notifySummary({ enabled: 'on', lead: 180, repeat: 60 }, D)).toBe(
      'Включены — за 3 часа, каждый час',
    )
    expect(notifySummary({ enabled: 'off', lead: -1, repeat: -1 }, D)).toBe('Выключены')
  })
  it('falls back to the server defaults when the settings are unknown', () => {
    expect(effectiveNotify({ enabled: 'inherit', lead: -1, repeat: -1 }, null)).toEqual({
      enabled: true,
      lead: 60,
      repeat: 0,
    })
  })
})
