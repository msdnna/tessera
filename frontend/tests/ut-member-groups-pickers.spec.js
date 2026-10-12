// Member groups (#3074) in the two people pickers: the assignee picker offers a
// group as one row that assigns its people; the editor's @-popup writes out the
// handles of everyone in it. Neither grants anything — only people are sent.
import { describe, it, expect, vi } from 'vitest'
import { assigneeItems } from '@/components/task/props/propItems'
import { mentionRows } from '@/utils/edSuggest'

const t = (k, p) => (p && typeof p === 'object' ? `${k}:${JSON.stringify(p)}` : k)

describe('assigneeItems — groups', () => {
  const members = [
    { user_id: 'u1', name: 'Анна' },
    { user_id: 'u2', name: 'Борис' },
  ]
  const groups = [
    { id: 'g1', name: 'Фронтенд', member_ids: ['u1', 'u2', 'gone'] },
    { id: 'g2', name: 'Пустая', member_ids: ['gone'] },
  ]
  it('a group row per group with people on the roster, on when all are assigned', () => {
    const toggleGroup = vi.fn()
    const { items } = assigneeItems({
      t,
      members,
      groups,
      assigned: new Set(['u1', 'u2']),
      ops: { toggle: vi.fn(), toggleGroup },
    })
    const g = items.filter((i) => i.id?.startsWith('g:'))
    expect(g.map((i) => i.label)).toEqual(['Фронтенд'])
    expect(g[0].on).toBe(true)
    g[0].run()
    expect(toggleGroup).toHaveBeenCalledWith(groups[0])
  })
  it('the query filters groups by name', () => {
    const { items } = assigneeItems({
      t,
      members,
      groups,
      query: 'бэк',
      ops: { toggle: vi.fn(), toggleGroup: vi.fn() },
    })
    expect(items.some((i) => i.id?.startsWith('g:'))).toBe(false)
  })
})

describe('mentionRows — groups', () => {
  const items = [
    { id: 'u1', label: 'anna', display: 'Анна' },
    { id: 'u2', label: 'boris', display: 'Борис' },
  ]
  const groups = [{ id: 'g1', name: 'Фронтенд', member_ids: ['u1', 'u2'] }]
  it('a group row carries the handles of its people', () => {
    const out = mentionRows(items, 'фронт', [], { groups: 'Группы' }, groups)
    const row = out.find((r) => r.key === 'grp:g1')
    expect(row.item).toEqual({ group: true, labels: ['anna', 'boris'] })
    expect(out.find((r) => r.head === 'Группы')).toBeTruthy()
  })
  it('no groups — the list is as before', () => {
    expect(mentionRows(items, '', [], {})).toEqual(mentionRows(items, '', [], {}, []))
  })
})
