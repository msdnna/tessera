import { describe, it, expect } from 'vitest'
import {
  detectTrigger,
  mentionRows,
  taskRows,
  commandRows,
  rowsOf,
  SUGGEST_LIMIT,
} from '@/utils/edSuggest'

const L = {
  inTask: 'В задаче',
  members: 'Участники',
  gitlab: 'GitLab',
  gitlabAside: 'GitLab',
  role: (r) => ({ assignee: 'исполнитель', author: 'автор' })[r] || '',
}

describe('detectTrigger (#3118)', () => {
  it('opens @ and # after a space or a line start', () => {
    expect(detectTrigger('отступы в тосте @ан')).toEqual({
      kind: 'mention',
      start: 16,
      query: 'ан',
    })
    expect(detectTrigger('#эксп')).toEqual({ kind: 'task', start: 0, query: 'эксп' })
    expect(detectTrigger('(#12')).toEqual({ kind: 'task', start: 1, query: '12' })
  })
  it('stays shut inside a word, after a space and on a heading', () => {
    expect(detectTrigger('user@mail')).toBeNull()
    expect(detectTrigger('C#')).toBeNull()
    expect(detectTrigger('@ан ')).toBeNull()
    expect(detectTrigger('##')).toBeNull()
  })
  it('honours what the editor offers', () => {
    expect(detectTrigger('@a', { mentions: false })).toBeNull()
    expect(detectTrigger('#1', { tasks: false })).toBeNull()
  })
})

describe('mentionRows', () => {
  const items = [
    { id: 'u1', label: 'a.belova', display: 'Анастасия Белова', hint: '@a.belova' },
    { id: 'u2', label: 'anna', display: 'Анна Ким', hint: '@anna' },
    { id: 'u3', label: 'Пётр', display: 'Пётр' },
    { id: null, label: 'a.shilov', display: 'Антон Шилов', gitlab: true },
  ]
  it('puts the task people first, with their part in it', () => {
    const rows = mentionRows(items, '', [{ id: 'u2', role: 'assignee' }], L)
    expect(rows[0]).toEqual({ head: 'В задаче' })
    expect(rows[1]).toMatchObject({ name: 'Анна Ким', login: '@anna', aside: 'исполнитель' })
    expect(rows.filter((r) => r.head).map((r) => r.head)).toEqual([
      'В задаче',
      'Участники',
      'GitLab',
    ])
    expect(rows.at(-1)).toMatchObject({ name: 'Антон Шилов', login: '@a.shilov', aside: 'GitLab' })
  })
  it('filters by name and login', () => {
    const rows = rowsOf(mentionRows(items, 'ан', [], L))
    expect(rows.map((r) => r.name)).toEqual(['Анастасия Белова', 'Анна Ким', 'Антон Шилов'])
  })
  it('caps the list', () => {
    const many = Array.from({ length: 20 }, (_, i) => ({ id: `x${i}`, label: `u${i}` }))
    expect(rowsOf(mentionRows(many, '', [], L))).toHaveLength(SUGGEST_LIMIT)
  })
})

describe('taskRows', () => {
  it('drops the task itself and reads the stage before the board', () => {
    const rows = taskRows(
      [
        {
          id: 't1',
          number: 2958,
          title: 'Экспорт',
          project_name: 'Web',
          milestone_title: 'Спринт',
        },
        { id: 'self', number: 1, title: 'Я' },
        { id: 't2', number: 2903, title: 'PDF', project_name: 'Web', board_name: 'Бэклог' },
        { id: 't3', number: 7, completed_at: '2026-01-01', column_name_key: 'in_progress' },
      ],
      'self',
    )
    expect(rows.map((r) => r.number)).toEqual([2958, 2903, 7])
    expect(rows[0].sub).toBe('Web · Спринт')
    expect(rows[1].sub).toBe('Web · Бэклог')
    expect(rows[2].icon).toBe('status-done')
  })
})

describe('commandRows', () => {
  const cmd = (key, arg = 'none', builtin = true) => ({ key, description: key, arg, builtin })
  const items = [
    cmd('assign', 'user'),
    cmd('due', 'date'),
    cmd('estimate', 'estimate'),
    cmd('priority', 'priority'),
    cmd('move', 'column'),
    cmd('close'),
    cmd('reopen'),
    cmd('blocks', 'task_ref'),
    cmd('subtask', 'text'),
    cmd('deploy', 'none', false),
  ]
  const labels = { group: (g) => g, arg: (k) => `<${k}>` }
  it('offers the featured eight in groups, then the workspace own', () => {
    const rows = commandRows(items, '', labels)
    expect(rows.filter((r) => r.head).map((r) => r.head)).toEqual([
      'fields',
      'status',
      'links',
      'subtasks',
      'custom',
    ])
    expect(rowsOf(rows).map((r) => r.cmd)).toEqual([
      '/due',
      '/assign',
      '/estimate',
      '/priority',
      '/move',
      '/close',
      '/blocks',
      '/subtask',
      '/deploy',
    ])
    expect(rows[1]).toMatchObject({ cmd: '/due', aside: '<date>' })
    expect(rows.at(-1)).toMatchObject({ custom: true })
  })
  it('matches a typed query anywhere in the registry', () => {
    expect(rowsOf(commandRows(items, 'reo', labels)).map((r) => r.cmd)).toEqual(['/reopen'])
    expect(commandRows(items, 'zzz', labels)).toEqual([])
  })
})
