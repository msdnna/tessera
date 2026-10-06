// Шапка оси «Таймлайн»/«Гантт» и перевод часов.
//
// Ось раскладывается по ЛОКАЛЬНЫМ календарным дням: startOfDay() снимает время в
// зоне браузера, dayIndex() округляет разницу до суток, ячейка i стоит на x=i*dayW.
// А buildDays() набирал момент ячейки как `rangeStart + i * DAY_MS` — то есть
// ровно 24 часа, которых в дне с переводом часов не бывает.
//
// Осенний день длится 25 часов, поэтому начиная с перевода каждая ячейка получает
// момент 23:00 ПРЕДЫДУЩЕГО дня: номер дня повторяется, а дальше число, день недели,
// выходные, полосы месяцев/недель и подсветка «сегодня» уезжают на сутки. Полосы
// задач считаются честными локальными полуночами и остаются на месте — значит
// задача со сроком 27.10 оказывается под заголовком «26».
//
// Зона прогона зафиксирована: на UTC-раннере (и в МСК) перевода часов нет вовсе,
// и спека была бы зелёной. Восстанавливаем в afterAll — пул vitest переиспользует
// процесс между файлами.
const TZ_BEFORE = process.env.TZ
process.env.TZ = 'Europe/Berlin' // перевод часов: 29.03.2026 (23 ч) и 25.10.2026 (25 ч)

import { describe, it, expect, afterAll } from 'vitest'
import {
  DAY_MS,
  startOfDay,
  buildDays,
  buildMonthBands,
  buildWeekBands,
  msAtDayOffset,
} from '@/utils/timeAxis'
import { createFormatters } from '@/utils/format'

afterAll(() => {
  if (TZ_BEFORE === undefined) delete process.env.TZ
  else process.env.TZ = TZ_BEFORE
})

const at = (y, m, d) => startOfDay(new Date(y, m, d).getTime())
const numbers = (days) => days.map((d) => d.day)

describe('buildDays через осенний перевод часов', () => {
  it('не повторяет день и не сдвигает числа после 25.10', () => {
    const days = buildDays(at(2026, 9, 23), 6, 0) // 23..28 октября
    expect(numbers(days)).toEqual([23, 24, 25, 26, 27, 28])
  })

  it('каждая ячейка — настоящая локальная полночь', () => {
    const days = buildDays(at(2026, 9, 23), 6, 0)
    for (const d of days) {
      const dt = new Date(d.ms)
      expect([dt.getHours(), dt.getMinutes()]).toEqual([0, 0])
      // Тот же момент, что берут полосы задач.
      expect(d.ms).toBe(startOfDay(d.ms))
    }
  })

  it('выходные и дни недели не уезжают на сутки', () => {
    const days = buildDays(at(2026, 9, 23), 6, 0, createFormatters({ language: 'ru' }))
    // 24/25 октября 2026 — суббота и воскресенье, 26-е уже понедельник.
    expect(days.map((d) => d.weekend)).toEqual([false, true, true, false, false, false])
    expect(days[3].dow).toBe('пн')
  })

  // Полосы задач стоят на ячейке dayIndex() = (startOfDay(срок) − начало оси)/сутки
  // с округлением, то есть на 4-й от 23 октября. Подсветка «сегодня» обязана попасть
  // в ту же ячейку, иначе она светит соседний столбец.
  it('«сегодня» подсвечивает ту же ячейку, где стоят полосы этого дня', () => {
    const rangeStart = at(2026, 9, 23)
    const todayMs = at(2026, 9, 27)
    const days = buildDays(rangeStart, 6, todayMs)
    const barCell = Math.round((todayMs - rangeStart) / DAY_MS)
    expect(days.map((d) => d.isToday)).toEqual(days.map((_, i) => i === barCell))
    expect(days[barCell].day).toBe(27)
  })
})

describe('полосы месяцев и недель через перевод часов', () => {
  it('граница месяца стоит на 1 ноября', () => {
    const days = buildDays(at(2026, 9, 23), 12, 0) // 23..31 окт (9) + 1,2,3 ноя (3)
    const bands = buildMonthBands(days, createFormatters({ language: 'en' }))
    expect(bands.map((b) => [b.label, b.span])).toEqual([
      ['Oct 2026', 9],
      ['Nov 2026', 3],
    ])
  })

  it('неделя ломается на понедельнике 26 октября', () => {
    const days = buildDays(at(2026, 9, 23), 6, 0) // пт..ср
    const bands = buildWeekBands(days)
    expect(bands).toHaveLength(2)
    expect(bands[0].label).toBe('23–25')
    expect(bands.map((b) => b.span)).toEqual([3, 3])
    expect(bands[1].startDay).toBe(26)
  })
})

describe('весенний перевод часов (негативный контроль)', () => {
  // 29.03.2026 длится 23 часа — мс-арифметика здесь попадала внутрь нужных суток,
  // так что этот случай был зелёным и до правки. Проверяем, что он им и остался.
  it('числа идут подряд через 29.03', () => {
    const days = buildDays(at(2026, 2, 27), 6, 0)
    expect(numbers(days)).toEqual([27, 28, 29, 30, 31, 1])
  })
})

describe('msAtDayOffset', () => {
  it('целое смещение даёт локальную полночь этого дня', () => {
    const start = at(2026, 9, 23)
    expect(msAtDayOffset(start, 4)).toBe(at(2026, 9, 27))
    expect(msAtDayOffset(start, 0)).toBe(start)
  })

  it('дробная часть — время внутри суток, а не внутри 24 часов от начала оси', () => {
    const start = at(2026, 9, 23)
    const dt = new Date(msAtDayOffset(start, 4.5))
    expect(dt.getDate()).toBe(27)
    expect(dt.getHours()).toBe(12)
  })

  it('без перевода часов совпадает с прежней мс-арифметикой', () => {
    const start = at(2026, 5, 1)
    expect(msAtDayOffset(start, 3.25)).toBe(start + 3.25 * DAY_MS)
  })
})
