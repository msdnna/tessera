// Правило «не ставить на выходные» и перевод часов.
//
// recurrence.js объявлен зеркалом бэкендового `internal/recur`, а тот шагает
// календарём (`AddDate(0, 0, n)`). applySkip же двигал дату миллисекундами:
// +48 ч от субботы и +24 ч от воскресенья. В неделю осеннего перевода часов
// сутки длятся 25 ч, поэтому оба шага не доходят до понедельника и остаются на
// ВОСКРЕСЕНЬЕ 23:00 — календарь в DueEditor подсвечивал ровно тот выходной,
// который правило запрещает, и день, на который сервер никогда не сработает.
// Сдвинутое время (23:00 вместо 00:00) дальше тянулось по всей цепочке
// occurrenceKeys.
//
// Зона прогона зафиксирована: на UTC-раннере (и в МСК) перевода часов нет вовсе,
// и спека была бы зелёной. Восстанавливаем в afterAll — пул vitest переиспользует
// процесс между файлами.
const TZ_BEFORE = process.env.TZ
process.env.TZ = 'Europe/Berlin' // перевод часов: 29.03.2026 (23 ч) и 25.10.2026 (25 ч)

import { describe, it, expect, afterAll } from 'vitest'
import { nextOccurrence, occurrenceKeys } from '@/utils/recurrence'

afterAll(() => {
  if (TZ_BEFORE === undefined) delete process.env.TZ
  else process.env.TZ = TZ_BEFORE
})

const key = (d) =>
  `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
const clock = (d) =>
  `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`

describe('skip_weekends через осенний перевод часов', () => {
  it('суббота уезжает на понедельник, а не на воскресенье', () => {
    // Пт 23.10 → сб 24.10 → +2 календарных дня = пн 26.10 (миллисекунды давали вс 25.10 23:00).
    const fri = new Date(2026, 9, 23, 0, 0, 0, 0)
    const next = nextOccurrence({ freq: 'daily', skip_weekends: true }, fri)
    expect(key(next)).toBe('2026-10-26')
    expect(clock(next)).toBe('00:00')
  })

  it('воскресенье уезжает на понедельник, а не остаётся собой', () => {
    // Пт 23.10 с шагом 2 → вс 25.10 → +1 календарный день = пн 26.10.
    const fri = new Date(2026, 9, 23, 0, 0, 0, 0)
    const next = nextOccurrence({ freq: 'daily', interval: 2, skip_weekends: true }, fri)
    expect(key(next)).toBe('2026-10-26')
    expect(clock(next)).toBe('00:00')
  })

  it('еженедельное правило по субботам сдвигается на понедельник', () => {
    // Пн 19.10, повтор по субботам → сб 24.10 → пн 26.10.
    const mon = new Date(2026, 9, 19, 9, 30, 0, 0)
    const next = nextOccurrence({ freq: 'weekly', weekdays: [6], skip_weekends: true }, mon)
    expect(key(next)).toBe('2026-10-26')
    expect(clock(next)).toBe('09:30')
  })

  it('в подсветке календаря нет ни одного выходного', () => {
    const from = new Date(2026, 9, 19, 0, 0, 0, 0).getTime() // пн 19.10, до перевода
    const keys = [...occurrenceKeys({ freq: 'daily', skip_weekends: true }, from, 14)]
    const weekend = keys.filter((k) => {
      const [y, m, d] = k.split('-').map(Number)
      const wd = new Date(y, m - 1, d).getDay()
      return wd === 0 || wd === 6
    })
    expect(weekend).toEqual([])
    expect(keys).toContain('2026-10-26')
  })
})

describe('skip_weekends через весенний перевод часов', () => {
  it('сохраняет время суток на 23-часовой неделе', () => {
    // Сб 28.03 → +2 календарных дня = пн 30.03 00:00 (миллисекунды давали 01:00).
    const fri = new Date(2026, 2, 27, 0, 0, 0, 0)
    const next = nextOccurrence({ freq: 'daily', skip_weekends: true }, fri)
    expect(key(next)).toBe('2026-03-30')
    expect(clock(next)).toBe('00:00')
  })
})
