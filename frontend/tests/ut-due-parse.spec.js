// Срок словами (#3118): quickParse.parseDue и parseClock — разбор строки поля
// DuePicker'а и палитры. `now` — «стенные» часы читателя (локальный Date с полями
// его зоны), ответ в тех же терминах: день и 'HH:MM'.
import { describe, it, expect } from 'vitest'
import { parseDue, parseClock } from '@/utils/quickParse'

// Суббота, 3 октября 2026, 14:20.
const NOW = new Date(2026, 9, 3, 14, 20)
const parse = (s) => parseDue(s, { now: NOW })
const ymd = (d) => `${d.getFullYear()}-${d.getMonth() + 1}-${d.getDate()}`
const read = (s) => {
  const r = parse(s)
  return r.ok && !r.clear ? `${r.day ? ymd(r.day) : '-'} ${r.time || '-'}` : r
}

describe('parseDue', () => {
  it('reads a weekday and a clock — «пт 18:00»', () => {
    expect(read('пт 18:00')).toBe('2026-10-9 18:00')
  })
  it('counts today for its own weekday, and «след.» skips to the next one', () => {
    expect(read('сб')).toBe('2026-10-3 -')
    expect(read('след. суббота')).toBe('2026-10-10 -')
    expect(read('след. понедельник 9:00')).toBe('2026-10-5 09:00')
    expect(read('next monday')).toBe('2026-10-5 -')
  })
  it('reads relative days', () => {
    expect(read('сегодня')).toBe('2026-10-3 -')
    expect(read('завтра 9:30')).toBe('2026-10-4 09:30')
    expect(read('послезавтра')).toBe('2026-10-5 -')
    expect(read('tomorrow 6pm')).toBe('2026-10-4 18:00')
  })
  it('reads «через …» in days, weeks, months, hours', () => {
    expect(read('через 3 дня')).toBe('2026-10-6 -')
    expect(read('через 3д')).toBe('2026-10-6 -')
    expect(read('через неделю')).toBe('2026-10-10 -')
    expect(read('через 2 недели')).toBe('2026-10-17 -')
    expect(read('через месяц')).toBe('2026-11-3 -')
    expect(read('через 2 ч')).toBe('2026-10-3 16:20')
    expect(read('in 3 days')).toBe('2026-10-6 -')
  })
  it('reads dates — a past «14.10» this year is next year’s', () => {
    expect(read('14.10')).toBe('2026-10-14 -')
    expect(read('1.10')).toBe('2027-10-1 -')
    expect(read('14.10.27 10:00')).toBe('2027-10-14 10:00')
    expect(read('2026-12-31')).toBe('2026-12-31 -')
  })
  it('reads the end of the week and parts of the day', () => {
    expect(read('конец недели')).toBe('2026-10-9 -')
    expect(read('в пятницу вечером')).toBe('2026-10-9 18:00')
    expect(read('завтра утром')).toBe('2026-10-4 09:00')
    expect(read('в 18')).toBe('- 18:00')
    expect(read('11.45')).toBe('- 11:45')
  })
  it('says which words it did not understand', () => {
    expect(parse('послезавтрашний')).toEqual({ ok: false, bad: 'послезавтрашний' })
    expect(parse('пт созвон')).toEqual({ ok: false, bad: 'созвон' })
    expect(parse('31.02')).toEqual({ ok: false, bad: '31.02' })
    expect(parse('25:00')).toEqual({ ok: false, bad: '25:00' })
  })
  it('knows blank and «без срока»', () => {
    expect(parse('  ')).toEqual({ empty: true })
    expect(parse('без срока')).toEqual({ ok: true, clear: true })
    expect(parse('No date')).toEqual({ ok: true, clear: true })
  })
})

describe('parseClock', () => {
  it('reads the clock forms the «Время» field takes', () => {
    expect(parseClock('18:00')).toBe('18:00')
    expect(parseClock('9.30')).toBe('09:30')
    expect(parseClock('18')).toBe('18:00')
    expect(parseClock('6pm')).toBe('18:00')
    expect(parseClock('06:30 PM')).toBe('18:30')
    expect(parseClock('12am')).toBe('00:00')
  })
  it('refuses what is not a clock', () => {
    expect(parseClock('')).toBe(null)
    expect(parseClock('24:00')).toBe(null)
    expect(parseClock('13pm')).toBe(null)
    expect(parseClock('утро')).toBe(null)
  })
})
