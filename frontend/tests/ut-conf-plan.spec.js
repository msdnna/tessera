import { describe, it, expect } from 'vitest'
import {
  durationLabel,
  planAt,
  planBody,
  recurrenceMenuItems,
  stageHref,
  stageMenuItems,
} from '@/utils/confPlan'
import { groupConferences, nearestScheduled } from '@/utils/confList'
import { createFormatters } from '@/utils/format'

// A meeting's plan (#3034): length and repeat (#3052), stage (#3053).
const t = (k, p) => (p ? `${k}:${JSON.stringify(p)}` : k)

describe('durationLabel', () => {
  it('says minutes, hours, or both — nothing without a length', () => {
    expect(durationLabel(45, t)).toBe('conferences.plan.min:{"n":45}')
    expect(durationLabel(60, t)).toBe('conferences.plan.h:{"h":1}')
    expect(durationLabel(90, t)).toBe('conferences.plan.hm:{"h":1,"m":30}')
    expect(durationLabel(null, t)).toBe('')
    expect(durationLabel(0, t)).toBe('')
  })
})

describe('planAt / stageHref', () => {
  it('files a repeat by its next occurrence', () => {
    expect(planAt({ scheduled_at: 'a', next_at: 'b' })).toBe('b')
    expect(planAt({ scheduled_at: 'a', next_at: null })).toBe('a')
    expect(planAt({})).toBe(null)
  })
  it('links the stage only with both slugs', () => {
    expect(stageHref({ milestone_slug: 's43', milestone_project_slug: 'web' })).toBe(
      '/project/web/stage/s43',
    )
    expect(stageHref({ milestone_slug: 's43' })).toBe(null)
  })
})

describe('planBody', () => {
  it('sends the zone only with a repeat, and drops a repeat without a start', () => {
    expect(
      planBody({ at: 1, duration: 30, recurrence: 'weekdays', milestoneId: 'm1' }, 'Europe/Moscow'),
    ).toEqual({
      duration_minutes: 30,
      recurrence: 'weekdays',
      timezone: 'Europe/Moscow',
      milestone_id: 'm1',
    })
    expect(planBody({ at: null, duration: null, recurrence: 'daily' }, 'UTC')).toEqual({
      duration_minutes: null,
      recurrence: '',
      timezone: '',
      milestone_id: null,
    })
  })
})

describe('menus', () => {
  const ms = [
    { id: 'a', title: 'Спринт 42', project_name: 'Web', state: 'closed' },
    { id: 'b', title: 'Спринт 43', project_name: 'Web', state: 'active' },
    { id: 'c', title: 'Релиз', project_name: 'Android', state: 'active' },
  ]
  it('offers open stages under their projects, plus «Без этапа»', () => {
    const items = stageMenuItems(ms, null, t)
    expect(items[0]).toMatchObject({ key: '', checked: true })
    expect(items.filter((i) => i.heading).map((i) => i.heading)).toEqual(['Web', 'Android'])
    expect(items.filter((i) => i.testid === 'stage-option').map((i) => i.key)).toEqual(['b', 'c'])
  })
  it('keeps a closed stage when it is the current one', () => {
    const keys = stageMenuItems(ms, 'a', t)
      .filter((i) => i.testid === 'stage-option')
      .map((i) => i.key)
    expect(keys).toContain('a')
  })
  it('says so when there are no stages', () => {
    expect(stageMenuItems([], null, t).some((i) => i.hint)).toBe(true)
  })
  it('checks the current repeat', () => {
    const items = recurrenceMenuItems('biweekly', t)
    expect(items.map((i) => i.key)).toEqual([
      'none',
      'daily',
      'weekdays',
      'weekly',
      'biweekly',
      'monthly',
    ])
    expect(items.find((i) => i.checked).key).toBe('biweekly')
  })
})

describe('the list with repeats', () => {
  const f = createFormatters({ language: 'ru', timezone: 'UTC', timeFormat: '24h' })
  const now = new Date('2026-10-08T10:00:00Z')
  const conf = (id, over) => ({ id, status: 'scheduled', scheduled_at: null, ...over })
  it('files a repeat started weeks ago under its next day, not «past»', () => {
    const list = [
      conf('standup', {
        scheduled_at: '2026-09-01T17:30:00Z',
        next_at: '2026-10-08T17:30:00Z',
        recurrence: 'weekdays',
      }),
      conf('old', { scheduled_at: '2026-09-20T12:00:00Z' }),
    ]
    const { groups } = groupConferences(list, f, { now, t })
    expect(groups.map((g) => [g.key === 'past' ? 'past' : 'day', g.rows.map((c) => c.id)])).toEqual(
      [
        ['day', ['standup']],
        ['past', ['old']],
      ],
    )
    expect(nearestScheduled(list, now).id).toBe('standup')
  })
})
