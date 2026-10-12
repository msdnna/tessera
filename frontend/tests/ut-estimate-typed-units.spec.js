// Оценка, набранная руками: что делает разбор с числом БЕЗ единицы (#2932).
//
// Поле оценки — свободный текст, и «1ч 30» / «2ч30» — обычный способ записать час
// с половиной. Разбор читал КАЖДОЕ число без суффикса как часы, поэтому «1ч 30»
// молча складывалось в 1860 минут (31 час) и уезжало на сервер: ни ошибки, ни
// подсветки — карточка просто начинала показывать «7д 3ч».
//
// Что «число без единицы = часы» задумывалось только для ОДИНОЧНОГО числа, видно
// по бэкендовому кейсу `{in: "1d 4", want: 720}` (quickact_test.go): там ждут
// «1 день 4 часа», то есть шаг вниз от предыдущей единицы. Для дня это совпало с
// «всегда часы» случайно — для часа даёт тридцатикратный промах.
//
// Правило, которое этот спек закрепляет (одинаковое во всех трёх разборах —
// frontend utils/estimation.js, android util/Estimation.kt, backend
// internal/quickact/args.go):
//   • число без единицы ПЕРВЫМ токеном — часы (как было);
//   • число без единицы сразу после токена с единицей — на шаг меньшая единица
//     (н→д, д→ч, ч→м, м→м);
//   • во всех остальных случаях (второе безъединичное подряд, неизвестный
//     суффикс) — отказ, а не догадка.
//
// Отказ обязан быть ВИДИМЫМ: `parseEstimate` возвращает null, а попап звал
// `setEstimate(null)`, то есть непонятый ввод СТИРАЛ уже выставленную оценку.
// Отсюда второй экспорт — `parseEstimateInput`, который отличает «пусто = снять
// оценку» от «не разобрал = не трогать и сказать».
import { describe, it, expect } from 'vitest'
import { DEFAULT_ESTIMATION, parseEstimate, parseEstimateInput } from '@/utils/estimation'

const cfg = DEFAULT_ESTIMATION // время, 8ч/день, 5дн/неделя → 480 мин/день, 2400 мин/неделя

describe('parseEstimate: число без единицы после единицы', () => {
  it('читает хвостовое число как единицу на шаг меньше', () => {
    expect(parseEstimate('1ч 30', cfg)).toBe(90)
    expect(parseEstimate('1h 30', cfg)).toBe(90)
    expect(parseEstimate('2ч30', cfg)).toBe(150)
    // Шаг вниз от дня — часы (это и есть бэкендовый кейс «1d 4» = 720).
    expect(parseEstimate('1д 4', cfg)).toBe(480 + 240)
    // Шаг вниз от недели — дни.
    expect(parseEstimate('1н 2', cfg)).toBe(2400 + 960)
    // Ниже минуты шага нет: остаётся минутами.
    expect(parseEstimate('30м 15', cfg)).toBe(45)
  })

  it('оставляет одиночное число часами', () => {
    expect(parseEstimate('3', cfg)).toBe(180)
    expect(parseEstimate('1.5', cfg)).toBe(90)
  })

  it('отказывается угадывать второе безъединичное число подряд', () => {
    expect(parseEstimate('2 3', cfg)).toBeNull()
    expect(parseEstimate('1:30', cfg)).toBeNull()
  })

  it('отказывается от неизвестного суффикса вместо тихих часов', () => {
    // «5 попугаев» бэкенд отвергал с самого начала, а клиенты сохраняли пять
    // часов — теперь все три читают ввод одинаково.
    expect(parseEstimate('5 попугаев', cfg)).toBeNull()
    expect(parseEstimate('2s', cfg)).toBeNull()
    expect(parseEstimate('2 сек', cfg)).toBeNull()
  })

  it('не задевает разбор с явными единицами', () => {
    expect(parseEstimate('1w 2d 3h 30m', cfg)).toBe(2400 + 960 + 180 + 30)
    expect(parseEstimate('1н 2д 3ч 30м', cfg)).toBe(2400 + 960 + 180 + 30)
    expect(parseEstimate('90м', cfg)).toBe(90)
    expect(parseEstimate('1,5ч', cfg)).toBe(90)
    expect(parseEstimate('2 часа 30 минут', cfg)).toBe(150)
    expect(parseEstimate('1 день 4 часа', cfg)).toBe(720)
  })

  it('не задевает нечисловые единицы (points / custom)', () => {
    expect(parseEstimate('5', { unit: 'points' })).toBe(5)
    expect(parseEstimate('m', { unit: 'points', points_scale: 'tshirt' })).toBe(3)
    expect(parseEstimate('2.5 тикетов', { unit: 'custom', custom_label: 'тикетов' })).toBe(2.5)
  })
})

describe('parseEstimateInput: пусто ≠ не разобрал', () => {
  it('снимает оценку только на пустой строке', () => {
    expect(parseEstimateInput('', cfg)).toEqual({ value: null, ok: true })
    expect(parseEstimateInput('   ', cfg)).toEqual({ value: null, ok: true })
    expect(parseEstimateInput(null, cfg)).toEqual({ value: null, ok: true })
  })

  it('сообщает об отказе, а не выдаёт «снять оценку»', () => {
    expect(parseEstimateInput('абракадабра', cfg)).toEqual({ value: null, ok: false })
    expect(parseEstimateInput('1:30', cfg)).toEqual({ value: null, ok: false })
    expect(parseEstimateInput('0', cfg)).toEqual({ value: null, ok: false })
  })

  it('пропускает разобранное значение', () => {
    expect(parseEstimateInput('1ч 30', cfg)).toEqual({ value: 90, ok: true })
    expect(parseEstimateInput('3д 4ч', cfg)).toEqual({ value: 1440 + 240, ok: true })
  })
})
