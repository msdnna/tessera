import { describe, it, expect } from 'vitest'
import { groupConferences, nearestScheduled } from '@/utils/confList'
import { createFormatters } from '@/utils/format'

// The v2 conferences list (#3107): live calls lead as cards, planned ones fall
// into days (soonest first), then the untimed, the plans that never started and
// the finished ones.
const f = createFormatters({ language: 'ru', timezone: 'UTC', timeFormat: '24h' })
const t = (k) => k
const now = new Date('2026-10-08T10:00:00Z')
const conf = (id, over) => ({ id, status: 'scheduled', scheduled_at: null, ...over })

describe('groupConferences', () => {
  const list = [
    conf('end1', { status: 'ended', ended_at: '2026-10-01T10:00:00Z' }),
    conf('end2', { status: 'ended', ended_at: '2026-10-05T10:00:00Z' }),
    conf('tom', { scheduled_at: '2026-10-09T09:00:00Z' }),
    conf('live', { status: 'live', started_at: '2026-10-08T09:30:00Z' }),
    conf('today2', { scheduled_at: '2026-10-08T15:00:00Z' }),
    conf('today1', { scheduled_at: '2026-10-08T12:00:00Z' }),
    conf('old', { scheduled_at: '2026-09-20T12:00:00Z' }),
    conf('free'),
  ]
  const { live, groups } = groupConferences(list, f, { now, t })

  it('puts the live call apart, first', () => {
    expect(live.map((c) => c.id)).toEqual(['live'])
  })

  it('files the rest by day, then untimed, past and finished', () => {
    expect(groups.map((g) => g.rows.map((c) => c.id))).toEqual([
      ['today1', 'today2'],
      ['tom'],
      ['free'],
      ['old'],
      ['end2', 'end1'],
    ])
    expect(groups[2].key).toBe('untimed')
    expect(groups[3].key).toBe('past')
    expect(groups[4].key).toBe('ended')
  })

  it('names today and tomorrow in the day label', () => {
    expect(groups[0].label.toLowerCase()).toContain('сегодня')
    expect(groups[1].label.toLowerCase()).toContain('завтра')
  })
})

describe('nearestScheduled', () => {
  it('is the earliest plan not over by more than a quarter of an hour', () => {
    const list = [
      conf('a', { scheduled_at: '2026-10-08T09:40:00Z' }),
      conf('b', { scheduled_at: '2026-10-08T09:50:00Z' }),
      conf('c', { scheduled_at: '2026-10-08T11:00:00Z' }),
    ]
    expect(nearestScheduled(list, now).id).toBe('b')
  })

  it('is nothing without a timed plan ahead', () => {
    expect(nearestScheduled([conf('x')], now)).toBeNull()
  })
})
