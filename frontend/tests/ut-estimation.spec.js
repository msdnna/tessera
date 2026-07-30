import { describe, it, expect } from 'vitest'
import {
  DEFAULT_ESTIMATION,
  POINTS_SCALES,
  resolveEstimation,
  parseEstimate,
  formatEstimate,
  scaleOptions,
  unitName,
  estimatePlaceholder,
  formatEstimateFull,
  estimateRangeShort,
  estimateTooltip,
  estimateToDays,
  sumEstimates,
} from '@/utils/estimation'

const cfg = DEFAULT_ESTIMATION // time, 8h day, 5d week → 480 min/day, 2400 min/week

describe('resolveEstimation', () => {
  it('prefers project, then workspace, then the built-in default', () => {
    const p = { estimation: { unit: 'points' } }
    const w = { estimation: { unit: 'custom' } }
    expect(resolveEstimation(p, w)).toEqual({ unit: 'points' })
    expect(resolveEstimation({}, w)).toEqual({ unit: 'custom' })
    expect(resolveEstimation(null, null)).toBe(DEFAULT_ESTIMATION)
  })
})

describe('parseEstimate (time)', () => {
  it('parses mixed english tokens into canonical minutes', () => {
    expect(parseEstimate('1w 2d 3h 30m', cfg)).toBe(2400 + 960 + 180 + 30)
  })

  it('parses russian units', () => {
    expect(parseEstimate('1н 2д 3ч 30м', cfg)).toBe(2400 + 960 + 180 + 30)
    expect(parseEstimate('90м', cfg)).toBe(90)
  })

  it('treats a bare number as hours', () => {
    expect(parseEstimate('3', cfg)).toBe(180)
  })

  it('accepts decimals and a comma separator', () => {
    expect(parseEstimate('1.5h', cfg)).toBe(90)
    expect(parseEstimate('1,5ч', cfg)).toBe(90)
  })

  it('honours a custom working day/week', () => {
    const c = { unit: 'time', hours_per_day: 6, days_per_week: 4 }
    expect(parseEstimate('1д', c)).toBe(360)
    expect(parseEstimate('1w', c)).toBe(1440)
  })

  it('returns null for empty / unparseable / zero input', () => {
    expect(parseEstimate(null, cfg)).toBeNull()
    expect(parseEstimate('   ', cfg)).toBeNull()
    expect(parseEstimate('abc', cfg)).toBeNull()
    expect(parseEstimate('0', cfg)).toBeNull()
  })
})

describe('parseEstimate (points / custom)', () => {
  it('parses a plain positive number', () => {
    expect(parseEstimate('5', { unit: 'points' })).toBe(5)
    expect(parseEstimate('4.5', { unit: 'custom' })).toBe(4.5)
  })

  it('maps t-shirt size labels to their point values (case-insensitive)', () => {
    const c = { unit: 'points', points_scale: 'tshirt' }
    expect(parseEstimate('m', c)).toBe(3)
    expect(parseEstimate('XL', c)).toBe(8)
  })

  it('rejects non-positive numbers', () => {
    expect(parseEstimate('0', { unit: 'points' })).toBeNull()
    expect(parseEstimate('-2', { unit: 'points' })).toBeNull()
  })
})

describe('formatEstimate', () => {
  it('compresses minutes into working weeks/days/hours', () => {
    expect(formatEstimate(1680, cfg)).toBe('3д 4ч') // 3.5 working days
    expect(formatEstimate(2400, cfg)).toBe('1н')
    expect(formatEstimate(30, cfg)).toBe('30м')
    expect(formatEstimate(1500, cfg)).toBe('3д 1ч') // 1500 / 480 = 3.125 working days
  })

  it('returns empty for null/zero and "0м" for a sub-minute value', () => {
    expect(formatEstimate(null, cfg)).toBe('')
    expect(formatEstimate(0, cfg)).toBe('')
    expect(formatEstimate(0.4, cfg)).toBe('0м')
  })

  it('formats points with SP suffix and t-shirt with the size label', () => {
    expect(formatEstimate(5, { unit: 'points' })).toBe('5 SP')
    expect(formatEstimate(3, { unit: 'points', points_scale: 'tshirt' })).toBe('M')
    // off-scale value falls back to the number
    expect(formatEstimate(4, { unit: 'points', points_scale: 'tshirt' })).toBe('4')
  })

  it('formats custom with the label and trims trailing .0', () => {
    expect(formatEstimate(2.5, { unit: 'custom', custom_label: 'шт' })).toBe('2.5 шт')
    expect(formatEstimate(5, { unit: 'custom' })).toBe('5')
  })
})

describe('scaleOptions', () => {
  it('is empty for non-points units', () => {
    expect(scaleOptions(cfg)).toEqual([])
    expect(scaleOptions({ unit: 'custom' })).toEqual([])
  })

  it('defaults to fibonacci and supports t-shirt labels', () => {
    expect(scaleOptions({ unit: 'points' }).map((o) => o.value)).toEqual(POINTS_SCALES.fibonacci)
    const ts = scaleOptions({ unit: 'points', points_scale: 'tshirt' })
    expect(ts[0]).toEqual({ label: 'XS', value: 1 })
    expect(ts).toHaveLength(6)
  })
})

describe('unitName / estimatePlaceholder', () => {
  it('names the unit in russian', () => {
    expect(unitName(cfg)).toBe('Время')
    expect(unitName({ unit: 'points' })).toBe('Стори-поинты')
    expect(unitName({ unit: 'custom', custom_label: ' шт ' })).toBe('шт')
    expect(unitName({ unit: 'custom' })).toBe('Единицы')
  })

  it('hints the accepted syntax', () => {
    expect(estimatePlaceholder(cfg)).toContain('3д')
    expect(estimatePlaceholder({ unit: 'points' })).toBe('напр. 5')
    expect(estimatePlaceholder({ unit: 'custom' })).toBe('напр. 8')
  })
})

describe('formatEstimateFull (russian plurals)', () => {
  it('spells out the compressed time', () => {
    expect(formatEstimateFull(1800, cfg)).toBe('3 дня 6 часов') // 30h @ 8h/day
    expect(formatEstimateFull(480, cfg)).toBe('1 день')
    expect(formatEstimateFull(2400, cfg)).toBe('1 неделя')
  })

  it('picks the right plural form for minutes', () => {
    expect(formatEstimateFull(21, cfg)).toBe('21 минута')
    expect(formatEstimateFull(22, cfg)).toBe('22 минуты')
    expect(formatEstimateFull(11, cfg)).toBe('11 минут')
    expect(formatEstimateFull(25, cfg)).toBe('25 минут')
  })

  it('falls back to compact formatting for points and returns "" for empty', () => {
    expect(formatEstimateFull(5, { unit: 'points' })).toBe('5 SP')
    expect(formatEstimateFull(null, cfg)).toBe('')
  })
})

describe('estimateToDays', () => {
  it('maps a working week onto 7 calendar days', () => {
    expect(estimateToDays(2400, cfg)).toBe(7)
    expect(estimateToDays(1200, cfg)).toBe(3.5)
  })

  it('is null for points/custom and empty values', () => {
    expect(estimateToDays(5, { unit: 'points' })).toBeNull()
    expect(estimateToDays(null, cfg)).toBeNull()
    expect(estimateToDays(0, cfg)).toBeNull()
  })
})

describe('estimateRangeShort / estimateTooltip', () => {
  const y = new Date().getFullYear()

  it('formats a start → end window without the year for current-year dates', () => {
    const s = estimateRangeShort(`${y}-06-15T00:00:00`, 2400, cfg)
    expect(s).toContain('→')
    expect(s).not.toContain(String(y))
  })

  it('appends the year for non-current years', () => {
    const s = estimateRangeShort(`${y - 1}-06-15T00:00:00`, 2400, cfg)
    expect(s).toContain(String(y - 1))
  })

  it('returns "" without a start date or with an invalid one', () => {
    expect(estimateRangeShort(null, 2400, cfg)).toBe('')
    expect(estimateRangeShort('garbage', 2400, cfg)).toBe('')
    expect(estimateRangeShort(`${y}-06-15`, null, cfg)).toBe('')
  })

  it('tooltip = full estimate, with the window in parens when a start exists', () => {
    expect(estimateTooltip(null, 2400, cfg)).toBe('1 неделя')
    const t = estimateTooltip(`${y}-06-15T00:00:00`, 2400, cfg)
    expect(t.startsWith('1 неделя (')).toBe(true)
    expect(t.endsWith(')')).toBe(true)
    expect(estimateTooltip(null, null, cfg)).toBe('')
  })
})

describe('sumEstimates', () => {
  it('sums positive estimates and skips empty ones', () => {
    expect(sumEstimates([{ estimate: 10 }, { estimate: null }, { estimate: 5 }, {}])).toBe(15)
  })

  it('returns null when nothing is estimated', () => {
    expect(sumEstimates([])).toBeNull()
    expect(sumEstimates(null)).toBeNull()
    expect(sumEstimates([{ estimate: 0 }, { estimate: null }])).toBeNull()
  })
})
