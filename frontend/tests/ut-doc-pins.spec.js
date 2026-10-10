import { describe, it, expect } from 'vitest'
import { RECENT_ENDED, defaultAgendaMeeting, meetingChoices, stageChoices } from '@/utils/docPins'
import {
  BUILTIN_TEMPLATES,
  TEMPLATE_CATEGORIES,
  builtinCards,
  inSource,
  isAgendaTemplate,
} from '@/utils/docTemplates'

// A document's stage and meeting pickers (#3044, #3046) and the template
// gallery's categories (#3045).

const ms = (id, project, state = 'active', over = {}) => ({
  id,
  title: `Этап ${id}`,
  project_id: project,
  project_name: `Проект ${project}`,
  state,
  ...over,
})

describe('stageChoices', () => {
  it('lists the open stages, a closed one only while it is the pin', () => {
    const list = [ms('a', 'p'), ms('b', 'p', 'closed'), ms('c', 'p', 'closed')]
    expect(stageChoices(list, null).map((r) => r.key)).toEqual(['ms:a'])
    const rows = stageChoices(list, 'b')
    expect(rows.map((r) => r.key)).toEqual(['ms:a', 'ms:b'])
    expect(rows.find((r) => r.key === 'ms:b').checked).toBe(true)
  })

  it('groups under project headings only when there is more than one project', () => {
    expect(stageChoices([ms('a', 'p'), ms('b', 'p')], null).some((r) => r.heading)).toBe(false)
    const rows = stageChoices([ms('a', 'p'), ms('b', 'q'), ms('c', 'p')], null)
    expect(rows.map((r) => r.heading || r.key)).toEqual([
      'Проект p',
      'ms:a',
      'ms:c',
      'Проект q',
      'ms:b',
    ])
  })

  it('carries the note the caller computes', () => {
    const rows = stageChoices([ms('a', 'p')], null, { noteOf: (m) => m.state })
    expect(rows[0].note).toBe('active')
  })
})

const conf = (id, status, over = {}) => ({ id, title: `Встреча ${id}`, status, ...over })

describe('meetingChoices', () => {
  it('orders live, then planned soonest first, then the recent finished ones', () => {
    const list = [
      conf('e1', 'ended', { ended_at: '2026-10-01T10:00:00Z' }),
      conf('s2', 'scheduled', { scheduled_at: '2026-10-20T10:00:00Z' }),
      conf('l', 'live'),
      conf('s1', 'scheduled', { scheduled_at: '2026-10-12T10:00:00Z' }),
      conf('s0', 'scheduled'),
      conf('e2', 'ended', { ended_at: '2026-10-05T10:00:00Z' }),
    ]
    expect(meetingChoices(list, null).map((r) => r.key)).toEqual([
      'conf:l',
      'conf:s1',
      'conf:s2',
      'conf:s0',
      'conf:e2',
      'conf:e1',
    ])
  })

  it('keeps only a few finished meetings, but always the current pin', () => {
    const ended = Array.from({ length: RECENT_ENDED + 3 }, (_, i) =>
      conf(`e${i}`, 'ended', { ended_at: new Date(Date.UTC(2026, 0, i + 1)).toISOString() }),
    )
    expect(meetingChoices(ended, null)).toHaveLength(RECENT_ENDED)
    const rows = meetingChoices(ended, 'e0')
    expect(rows).toHaveLength(RECENT_ENDED + 1)
    expect(rows.at(-1)).toMatchObject({ key: 'conf:e0', checked: true })
  })
})

describe('defaultAgendaMeeting', () => {
  const now = Date.parse('2026-10-10T12:00:00Z')
  it('prefers the meeting on now', () => {
    const list = [
      conf('s', 'scheduled', { scheduled_at: '2026-10-11T10:00:00Z' }),
      conf('l', 'live'),
    ]
    expect(defaultAgendaMeeting(list, now).id).toBe('l')
  })
  it('else the nearest one still ahead, skipping the missed ones', () => {
    const list = [
      conf('past', 'scheduled', { scheduled_at: '2026-10-09T10:00:00Z' }),
      conf('later', 'scheduled', { scheduled_at: '2026-10-14T10:00:00Z' }),
      conf('soon', 'scheduled', { scheduled_at: '2026-10-11T10:00:00Z' }),
    ]
    expect(defaultAgendaMeeting(list, now).id).toBe('soon')
  })
  it('none when nothing is ahead', () => {
    expect(defaultAgendaMeeting([conf('e', 'ended')], now)).toBeNull()
    expect(defaultAgendaMeeting(null, now)).toBeNull()
  })
})

describe('template categories', () => {
  it('files every built-in under a known category', () => {
    for (const tpl of BUILTIN_TEMPLATES) {
      expect(TEMPLATE_CATEGORIES).toContain(tpl.category)
      expect(tpl.category).not.toBe('')
    }
    expect(builtinCards().every((c) => c.category)).toBe(true)
  })

  it('«Мои» is the reader’s saved templates, not a category', () => {
    const mine = { id: '1', author_id: 'u1', category: 'sprint' }
    const theirs = { id: '2', author_id: 'u2', category: '' }
    const builtin = builtinCards()[0]
    expect(inSource(mine, 'mine', 'u1')).toBe(true)
    expect(inSource(theirs, 'mine', 'u1')).toBe(false)
    expect(inSource(builtin, 'mine', 'u1')).toBe(false)
    expect(inSource(mine, 'mine', '')).toBe(false)
    expect(inSource(mine, 'sprint', 'u1')).toBe(true)
    expect(inSource(theirs, 'sprint', 'u1')).toBe(false)
    expect(inSource(theirs, 'all', 'u1')).toBe(true)
  })

  it('an agenda is the built-in meeting template or a saved one under «Встречи»', () => {
    expect(isAgendaTemplate(builtinCards().find((c) => c.key === 'meeting'))).toBe(true)
    expect(isAgendaTemplate(builtinCards().find((c) => c.key === 'retro'))).toBe(false)
    expect(isAgendaTemplate({ id: 'x', category: 'meetings' })).toBe(true)
    expect(isAgendaTemplate({ id: 'x', category: '' })).toBe(false)
    expect(isAgendaTemplate(null)).toBe(false)
  })
})
