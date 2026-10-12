// Переполнение в разборе оценки (#2932).
//
// `parseEstimateInput` существует ровно затем, чтобы непонятый ввод НЕ стирал
// уже выставленную оценку: `ok: false` — «не трогать и сказать», а `null` при
// `ok: true` — «поле пустое, снять оценку». Различие держится на одном
// возвращаемом значении `parseEstimate`, и арифметика проходила мимо него.
//
// `total` в ветке времени — сумма произведений, а JS переполняется в Infinity
// молча. Достаточно длинной череды цифр (вставленный id, зажатая клавиша), и
// разбор возвращал Infinity: `Math.round` его не трогает, проверка `mins > 0`
// пропускает. Наружу это уходило как `{ ok: true, value: Infinity }`, попап
// применял, а `JSON.stringify` превращал Infinity в `null` — то есть в PATCH
// «снять оценку». Тот самый худший из трёх исходов, но через арифметику.
import { describe, it, expect } from 'vitest'
import { DEFAULT_ESTIMATION, parseEstimate, parseEstimateInput } from '@/utils/estimation'

const cfg = DEFAULT_ESTIMATION // время, 8ч/день, 5дн/неделя
const points = { unit: 'points', points_scale: 'linear' }

// 3e306 часов: само число читается, а ×60 выходит за пределы double.
const overflowing = '3' + '0'.repeat(306) + 'ч'

describe('разбор оценки не переполняется', () => {
  it('переполнение — это отказ, а не Infinity', () => {
    expect(parseEstimate(overflowing, cfg)).toBe(null)
  })

  it('отказ виден вызывающему, а не выглядит как «снять оценку»', () => {
    const r = parseEstimateInput(overflowing, cfg)
    expect(r.ok).toBe(false)
    expect(r.value).toBe(null)
    // Контроль: пустое поле — по-прежнему законный способ снять оценку.
    expect(parseEstimateInput('', cfg)).toEqual({ value: null, ok: true })
  })

  it('потолок общий с бэкендом: 1e9 берём, 1e9+1 отказываем', () => {
    expect(parseEstimate('1000000000м', cfg)).toBe(1e9)
    expect(parseEstimate('1000000001м', cfg)).toBe(null)
    expect(parseEstimate('1000000000', points)).toBe(1e9)
    expect(parseEstimate('1000000001', points)).toBe(null)
  })

  it('обычные оценки не задеты', () => {
    expect(parseEstimate('2ч', cfg)).toBe(120)
    expect(parseEstimate('1ч 30', cfg)).toBe(90)
    expect(parseEstimate('1д', cfg)).toBe(480)
    expect(parseEstimate('1w', cfg)).toBe(2400)
    expect(parseEstimate('5', points)).toBe(5)
  })
})
