import { describe, it, expect } from 'vitest'
import { filterBoardTasks, matchesDue, matchesTask, hasSubtaskFacets } from '@/utils/taskFilter'

const NOW = new Date('2026-08-03T12:00:00')

const EMPTY = {
  q: '',
  priorities: [],
  assignees: [],
  tags: [],
  statuses: [],
  milestones: [],
  due: '',
}
const f = (over) => ({ ...EMPTY, ...over })

// One parent in column c1 with two children; only the first is assigned to u1.
const parent = {
  id: 'p1',
  title: 'Родитель',
  number: 10,
  column_id: 'c1',
  priority: 0,
  assignee_ids: [],
}
const subMine = {
  id: 's1',
  parent_id: 'p1',
  title: 'Моя подзадача',
  number: 11,
  column_id: 'c1',
  priority: 3,
  assignee_ids: ['u1'],
  tag_ids: ['tagA'],
  due_date: '2026-08-03T00:00:00+03:00',
}
const subOther = {
  id: 's2',
  parent_id: 'p1',
  title: 'Чужая подзадача',
  number: 12,
  column_id: 'c2',
  priority: 0,
  assignee_ids: ['u2'],
}
const board = { tasks: [parent], subtasksByParent: { p1: [subMine, subOther] }, now: NOW }

describe('filterBoardTasks — подзадача поднимает родителя', () => {
  it('фильтр по исполнителю показывает родителя и только совпавшую подзадачу', () => {
    const r = filterBoardTasks({ ...board, filters: f({ assignees: ['u1'] }) })
    expect(r.tasks.map((t) => t.id)).toEqual(['p1'])
    expect(r.subtasksByParent.p1.map((s) => s.id)).toEqual(['s1'])
    expect(r.narrowedParents.has('p1')).toBe(true)
  })

  it('поиск по заголовку подзадачи поднимает родителя', () => {
    const r = filterBoardTasks({ ...board, filters: f({ q: 'моя подз' }) })
    expect(r.tasks.map((t) => t.id)).toEqual(['p1'])
    expect(r.subtasksByParent.p1.map((s) => s.id)).toEqual(['s1'])
  })

  it('поиск по #номеру подзадачи поднимает родителя', () => {
    const r = filterBoardTasks({ ...board, filters: f({ q: '#11' }) })
    expect(r.tasks.map((t) => t.id)).toEqual(['p1'])
  })

  it('теги / приоритет / срок подзадачи тоже поднимают родителя', () => {
    for (const flt of [{ tags: ['tagA'] }, { priorities: [3] }, { due: 'today' }]) {
      const r = filterBoardTasks({ ...board, filters: f(flt) })
      expect(
        r.tasks.map((t) => t.id),
        JSON.stringify(flt),
      ).toEqual(['p1'])
      expect(r.subtasksByParent.p1.map((s) => s.id)).toEqual(['s1'])
    }
  })

  it('gl:<login> подзадачи поднимает родителя', () => {
    const gl = { ...subMine, assignee_ids: [], gitlab_assignee_logins: ['msdnna'] }
    const r = filterBoardTasks({
      tasks: [parent],
      subtasksByParent: { p1: [gl, subOther] },
      filters: f({ assignees: ['gl:msdnna'] }),
      now: NOW,
    })
    expect(r.tasks.map((t) => t.id)).toEqual(['p1'])
    expect(r.subtasksByParent.p1.map((s) => s.id)).toEqual(['s1'])
  })

  it('ни родитель, ни дети не совпали — родителя нет', () => {
    const r = filterBoardTasks({ ...board, filters: f({ assignees: ['u9'] }) })
    expect(r.tasks).toEqual([])
  })
})

describe('filterBoardTasks — колонка (statuses) не пробрасывается', () => {
  it('подзадача из другой колонки НЕ затягивает родителя в этот фильтр', () => {
    // s2 живёт в c2, родитель — в c1: фильтр по колонке c2 не должен показать родителя.
    const r = filterBoardTasks({ ...board, filters: f({ statuses: ['c2'] }) })
    expect(r.tasks).toEqual([])
  })

  it('колонка родителя учитывается и при подъёме подзадачей', () => {
    const ok = filterBoardTasks({ ...board, filters: f({ statuses: ['c1'], assignees: ['u1'] }) })
    expect(ok.tasks.map((t) => t.id)).toEqual(['p1'])
    const no = filterBoardTasks({ ...board, filters: f({ statuses: ['c2'], assignees: ['u1'] }) })
    expect(no.tasks).toEqual([])
  })
})

describe('filterBoardTasks — родитель совпал сам', () => {
  it('все подзадачи остаются, родитель не помечен усечённым', () => {
    const r = filterBoardTasks({ ...board, filters: f({ q: 'родитель' }) })
    expect(r.subtasksByParent.p1.map((s) => s.id)).toEqual(['s1', 's2'])
    expect(r.narrowedParents.size).toBe(0)
  })

  it('без фильтров доска не меняется', () => {
    const r = filterBoardTasks({ ...board, filters: f({}) })
    expect(r.tasks).toEqual([parent])
    expect(r.subtasksByParent.p1).toHaveLength(2)
    expect(r.narrowedParents.size).toBe(0)
  })

  it('совпали все дети — усечения нет', () => {
    const r = filterBoardTasks({
      tasks: [parent],
      subtasksByParent: { p1: [subMine, { ...subOther, assignee_ids: ['u1'] }] },
      filters: f({ assignees: ['u1'] }),
      now: NOW,
    })
    expect(r.subtasksByParent.p1).toHaveLength(2)
    expect(r.narrowedParents.size).toBe(0)
  })
})

describe('matchesDue', () => {
  const t = (d) => ({ due_date: d })
  it('none / has', () => {
    expect(matchesDue(t(null), 'none', NOW)).toBe(true)
    expect(matchesDue(t('2026-08-03'), 'none', NOW)).toBe(false)
    expect(matchesDue(t('2026-08-03'), 'has', NOW)).toBe(true)
    expect(matchesDue(t(null), 'has', NOW)).toBe(false)
  })
  it('today / week / overdue', () => {
    expect(matchesDue(t('2026-08-03T09:00:00'), 'today', NOW)).toBe(true)
    expect(matchesDue(t('2026-08-06T09:00:00'), 'week', NOW)).toBe(true)
    expect(matchesDue(t('2026-08-20T09:00:00'), 'week', NOW)).toBe(false)
    expect(matchesDue(t('2026-08-01T09:00:00'), 'overdue', NOW)).toBe(true)
    expect(matchesDue({ due_date: '2026-08-01', completed_at: '2026-08-02' }, 'overdue', NOW)).toBe(
      false,
    )
  })
})

describe('matchesTask / hasSubtaskFacets', () => {
  it('facets ограничивают набор проверяемых фильтров', () => {
    const flt = f({ statuses: ['c9'], assignees: ['u1'] })
    expect(matchesTask(subMine, flt, { now: NOW })).toBe(false) // колонка не та
    expect(matchesTask(subMine, flt, { facets: ['assignees'], now: NOW })).toBe(true)
  })
  it('без «подзадачных» фасетов проход по детям не нужен', () => {
    expect(hasSubtaskFacets(f({}))).toBe(false)
    expect(hasSubtaskFacets(f({ statuses: ['c1'] }))).toBe(false)
    expect(hasSubtaskFacets(f({ q: '  ' }))).toBe(false)
    expect(hasSubtaskFacets(f({ q: 'x' }))).toBe(true)
    expect(hasSubtaskFacets(f({ tags: ['tagA'] }))).toBe(true)
  })
})
