// The rows of the property pickers (#3118, task/props/propItems): one builder
// per field, shared by the modal, the card, the list and the palette — so the
// rows, their order and their keys are pinned here once.
import { describe, it, expect, vi } from 'vitest'
import {
  assigneeItems,
  descendantsOf,
  milestoneItems,
  nextField,
  parentItems,
  priorityItems,
  statusItems,
} from '@/components/task/props/propItems'

const t = (k, p) => (p && typeof p === 'object' ? `${k}:${JSON.stringify(p)}` : k)
const rows = (items) => items.filter((i) => i.kind === 'row')
const sections = (items) => items.filter((i) => i.kind === 'section').map((i) => i.title)

const COLS = [
  { id: 'c1', name: 'К работе', position: 1 },
  { id: 'c2', name: 'В процессе', position: 2 },
  { id: 'c3', name: 'Готово', position: 3 },
]

describe('statusItems', () => {
  const ops = { move: vi.fn(), next: vi.fn(), complete: vi.fn() }
  it('columns with digits, the done one marked, the cursor on the current', () => {
    const { items, cursor } = statusItems({
      t,
      columns: COLS,
      currentId: 'c2',
      doneId: 'c3',
      counts: { c1: 4 },
      ops,
    })
    const r = rows(items)
    expect(r.slice(0, 3).map((x) => x.digit)).toEqual(['1', '2', '3'])
    expect(r[0].meta).toBe('4')
    expect(r[2].badge).toBe('props.status.finishes')
    expect(r[1].on).toBe(true)
    expect(cursor).toBe('col:c2')
    // «Сдвинуть в …» (]) and «Выполнить» (E) under «Действия».
    expect(r.slice(3).map((x) => x.caps)).toEqual([[']'], ['E']])
    r[0].run()
    expect(ops.move).toHaveBeenCalledWith('c1')
  })
  it('no «Сдвинуть» from the last column; a query filters and drops the actions', () => {
    const last = statusItems({ t, columns: COLS, currentId: 'c3', ops })
    expect(rows(last.items).some((x) => x.id === 'act:next')).toBe(false)
    const q = statusItems({ t, columns: COLS, currentId: 'c1', query: 'гот', ops })
    expect(rows(q.items).map((x) => x.id)).toEqual(['col:c3'])
  })
})

describe('priorityItems', () => {
  it('4…0 with the same digits, single choice', () => {
    const { items, cursor } = priorityItems({ current: 3, ops: { set: vi.fn() } })
    expect(items.map((x) => x.digit)).toEqual(['4', '3', '2', '1', '0'])
    expect(items.every((x) => x.close && x.check === 'single')).toBe(true)
    expect(cursor).toBe('prio:3')
  })
})

describe('assigneeItems', () => {
  const members = [
    { user_id: 'u1', name: 'Анна', email: 'anna@x.dev' },
    { user_id: 'u2', name: 'Борис', email: 'boris@x.dev' },
    { user_id: 'me', name: 'Мария', email: 'maria@x.dev' },
  ]
  const ops = { toggle: vi.fn(), toggleGl: vi.fn(), clear: vi.fn(), invite: vi.fn() }
  it('«Назначить себя», then «Назначены» · «Недавние» · «Участники» · «GitLab», «Снять всех»', () => {
    const { items } = assigneeItems({
      t,
      members,
      assigned: new Set(['u1']),
      recent: ['u2'],
      gitlab: [{ gl_username: 'kv', gl_name: 'Константин' }],
      me: 'me',
      load: { u1: 6 },
      ops,
    })
    expect(items[0].id).toBe('self')
    expect(sections(items)).toEqual([
      'props.assignees.assigned',
      'props.assignees.recent',
      'props.assignees.members',
      'props.assignees.gitlab',
    ])
    const anna = items.find((x) => x.id === 'u:u1')
    expect(anna).toMatchObject({ check: 'multi', on: true })
    expect(anna.meta).toContain('props.assignees.load')
    expect(items.at(-1).id).toBe('clear')
  })
  it('nothing found by an email offers the invite', () => {
    const { items, empty } = assigneeItems({ t, query: 'olga@x.dev', members, ops })
    expect(empty.title).toBe('props.assignees.none')
    expect(rows(items).map((x) => x.id)).toEqual(['invite'])
  })
})

describe('milestoneItems', () => {
  const today = new Date()
  const iso = (d) => {
    const x = new Date(Date.UTC(today.getFullYear(), today.getMonth(), today.getDate() + d))
    return x.toISOString()
  }
  const ms = [
    {
      id: 'a',
      title: 'Релиз',
      state: 'active',
      start_date: iso(-5),
      due_date: iso(5),
      task_count: 4,
      done_count: 1,
    },
    { id: 'p', title: 'Спринт', state: 'active', start_date: iso(10), due_date: iso(20) },
    { id: 'g', title: 'Из GitLab', state: 'active', start_date: iso(12), gl_url: 'https://gl' },
    { id: 'c', title: 'Старый', state: 'closed', closed_at: iso(-30) },
  ]
  const ops = { set: vi.fn(), toggleClosed: vi.fn(), create: vi.fn() }
  it('«Без этапа», active with progress, planned, closed folded, «Создать этап»', () => {
    const { items, cursor } = milestoneItems({ t, milestones: ms, current: 'a', ops })
    expect(rows(items)[0]).toMatchObject({ id: 'ms:none', digit: '0' })
    expect(sections(items)).toEqual(['props.milestone.active', 'props.milestone.planned'])
    expect(items.find((x) => x.id === 'ms:a').progress).toEqual({ done: 1, total: 4 })
    expect(items.find((x) => x.id === 'ms:g').lock).toBe(true)
    expect(items.find((x) => x.id === 'ms:closed').meta).toBe('1')
    expect(items.find((x) => x.id === 'ms:create')).toBeTruthy()
    expect(items.some((x) => x.kind === 'note')).toBe(true)
    expect(cursor).toBe('ms:a')
  })
})

describe('parentItems and descendants', () => {
  const all = [
    { id: 'self', board_id: 'b1', title: 'Сама' },
    { id: 'kid', board_id: 'b1', parent_id: 'self', title: 'Дочь' },
    { id: 'grand', board_id: 'b1', parent_id: 'kid', title: 'Внучка' },
    { id: 'x', board_id: 'b1', title: 'Другая', number: 7 },
    { id: 'y', board_id: 'b2', title: 'С другой доски', board_name: 'Бэклог' },
  ]
  it('never offers the task itself or its descendants', () => {
    const excluded = descendantsOf('self', all)
    expect([...excluded].sort()).toEqual(['grand', 'kid', 'self'])
    const { items } = parentItems({
      t,
      boardId: 'b1',
      boardTasks: [],
      projectTasks: all,
      excluded,
      ops: { set: vi.fn(), detach: vi.fn() },
    })
    expect(rows(items).map((x) => x.id)).toEqual(['p:x', 'p:y'])
    expect(sections(items)).toEqual(['props.parent.board', 'props.parent.project'])
  })
  it('a parent set leads with «Открепить» (⌫)', () => {
    const { items, cursor } = parentItems({
      t,
      boardId: 'b1',
      projectTasks: all,
      parent: { id: 'x', title: 'Другая' },
      excluded: new Set(['self']),
      ops: { set: vi.fn(), detach: vi.fn() },
    })
    expect(
      rows(items)
        .slice(0, 2)
        .map((x) => x.id),
    ).toEqual(['p:x', 'p:detach'])
    expect(cursor).toBe('p:detach')
  })
})

describe('nextField', () => {
  it('S A P D L round and round', () => {
    expect(['status', 'assignees', 'priority', 'due', 'tags'].map(nextField)).toEqual([
      'assignees',
      'priority',
      'due',
      'tags',
      'status',
    ])
    expect(nextField('estimate')).toBe(null)
  })
})
