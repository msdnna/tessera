// Фильтры доски и ВЛОЖЕННЫЕ подзадачи (#2932).
//
// Правило #2602 — «родителя видно, когда совпал он сам ИЛИ хотя бы одна его
// подзадача» — работало ровно на один уровень: filterBoardTasks читал только
// прямых детей, хотя доска грузит всё дерево разом (ListBoardSubtasksWithMeta
// отдаёт каждую задачу с parent_id, любого уровня вложенности). Внук,
// назначенный на читателя, уносил с доски всю ветку: карточки-предка нет,
// значит нет и пути к нему — ровно та беда, ради которой правило заводили.
import { describe, it, expect } from 'vitest'
import { filterBoardTasks } from '@/utils/taskFilter'

const NOW = new Date('2026-08-03T12:00:00')

const EMPTY = {
  q: '',
  priorities: [],
  assignees: [],
  authors: [],
  tags: [],
  statuses: [],
  milestones: [],
  due: '',
}
const f = (over) => ({ ...EMPTY, ...over })
const ids = (arr) => (arr || []).map((t) => t.id)

// Дерево в три уровня: карточка p1 → «середина» m1 → внуки g1 (мой) и g2 (чужой).
// Ни p1, ни m1 сами по себе ни под один фильтр ниже не подходят — только внук.
const top = {
  id: 'p1',
  title: 'Родитель',
  number: 10,
  column_id: 'c1',
  priority: 0,
  assignee_ids: [],
}
const mid = {
  id: 'm1',
  parent_id: 'p1',
  title: 'Середина',
  number: 11,
  column_id: 'c1',
  priority: 0,
  assignee_ids: [],
}
const grandMine = {
  id: 'g1',
  parent_id: 'm1',
  title: 'Внук по делу',
  number: 12,
  column_id: 'c1',
  priority: 3,
  assignee_ids: ['u1'],
  tag_ids: ['tagA'],
  // Наивно-локальная дата, как и NOW: matchesDue сравнивает календарные дни.
  due_date: '2026-08-03T00:00:00',
}
const grandOther = {
  id: 'g2',
  parent_id: 'm1',
  title: 'Чужой внук',
  number: 13,
  column_id: 'c1',
  priority: 0,
  assignee_ids: ['u2'],
}

const deep = {
  tasks: [top],
  subtasksByParent: { p1: [mid], m1: [grandMine, grandOther] },
  now: NOW,
}

describe('filterBoardTasks — внук поднимает всю ветку', () => {
  it('фильтр по исполнителю: карточка видна, путь к внуку сохранён', () => {
    const r = filterBoardTasks({ ...deep, filters: f({ assignees: ['u1'] }) })
    expect(ids(r.tasks)).toEqual(['p1'])
    expect(ids(r.subtasksByParent.p1)).toEqual(['m1'])
    expect(ids(r.subtasksByParent.m1)).toEqual(['g1'])
  })

  it('список внуков усечён до совпавших, промежуточный родитель помечен', () => {
    const r = filterBoardTasks({ ...deep, filters: f({ assignees: ['u1'] }) })
    expect(r.narrowedParents.has('m1')).toBe(true)
    // p1 отдал единственного ребёнка целиком — усечения на этом уровне нет.
    expect(r.narrowedParents.has('p1')).toBe(false)
  })

  it('поиск по заголовку внука поднимает карточку', () => {
    const r = filterBoardTasks({ ...deep, filters: f({ q: 'внук по делу' }) })
    expect(ids(r.tasks)).toEqual(['p1'])
    expect(ids(r.subtasksByParent.m1)).toEqual(['g1'])
  })

  it('поиск по #номеру внука поднимает карточку', () => {
    const r = filterBoardTasks({ ...deep, filters: f({ q: '#12' }) })
    expect(ids(r.tasks)).toEqual(['p1'])
  })

  it('тег / приоритет / срок внука тоже поднимают карточку', () => {
    for (const flt of [{ tags: ['tagA'] }, { priorities: [3] }, { due: 'today' }]) {
      const r = filterBoardTasks({ ...deep, filters: f(flt) })
      expect(ids(r.tasks), JSON.stringify(flt)).toEqual(['p1'])
      expect(ids(r.subtasksByParent.m1), JSON.stringify(flt)).toEqual(['g1'])
    }
  })

  it('правнук работает так же — глубина не ограничена', () => {
    const great = { ...grandMine, id: 'gg1', parent_id: 'g1', number: 14 }
    const r = filterBoardTasks({
      tasks: [top],
      subtasksByParent: {
        p1: [mid],
        m1: [{ ...grandMine, assignee_ids: [], tag_ids: [], due_date: null, priority: 0 }],
        g1: [great],
      },
      filters: f({ assignees: ['u1'] }),
      now: NOW,
    })
    expect(ids(r.tasks)).toEqual(['p1'])
    expect(ids(r.subtasksByParent.g1)).toEqual(['gg1'])
  })

  it('колонка ветки по-прежнему не пробрасывается наверх', () => {
    // statuses не входит в SUBTASK_FACETS: карточку нельзя затащить в колонку,
    // в которой она не стоит, — ни ребёнком, ни внуком.
    const r = filterBoardTasks({
      tasks: [top],
      subtasksByParent: { p1: [mid], m1: [{ ...grandMine, column_id: 'c2' }, grandOther] },
      filters: f({ statuses: ['c2'] }),
      now: NOW,
    })
    expect(r.tasks).toEqual([])
  })

  it('колонка самой карточки учитывается и при подъёме внуком', () => {
    const ok = filterBoardTasks({
      ...deep,
      filters: f({ statuses: ['c1'], assignees: ['u1'] }),
    })
    expect(ids(ok.tasks)).toEqual(['p1'])
    const no = filterBoardTasks({
      ...deep,
      filters: f({ statuses: ['c2'], assignees: ['u1'] }),
    })
    expect(no.tasks).toEqual([])
  })
})

describe('filterBoardTasks — вложенность: контроли', () => {
  it('никто в ветке не совпал — карточки нет', () => {
    const r = filterBoardTasks({ ...deep, filters: f({ assignees: ['u9'] }) })
    expect(r.tasks).toEqual([])
    expect(r.subtasksByParent.m1).toBeUndefined()
  })

  it('совпал промежуточный родитель — его поддерево остаётся целиком', () => {
    const r = filterBoardTasks({ ...deep, filters: f({ q: 'середина' }) })
    expect(ids(r.tasks)).toEqual(['p1'])
    expect(ids(r.subtasksByParent.p1)).toEqual(['m1'])
    // m1 совпал сам, значит его дети не усекаются — как и у совпавшей карточки.
    expect(ids(r.subtasksByParent.m1)).toEqual(['g1', 'g2'])
    expect(r.narrowedParents.size).toBe(0)
  })

  it('совпала сама карточка — дерево не трогаем', () => {
    const r = filterBoardTasks({ ...deep, filters: f({ q: 'родитель' }) })
    expect(ids(r.subtasksByParent.p1)).toEqual(['m1'])
    expect(r.narrowedParents.size).toBe(0)
  })

  it('пустой композер: дерево возвращается как есть', () => {
    const r = filterBoardTasks({ ...deep, filters: f({}) })
    expect(r.tasks).toEqual([top])
    expect(ids(r.subtasksByParent.p1)).toEqual(['m1'])
    expect(ids(r.subtasksByParent.m1)).toEqual(['g1', 'g2'])
    expect(r.narrowedParents.size).toBe(0)
  })

  it('совпали оба внука — усечения нет', () => {
    const r = filterBoardTasks({
      tasks: [top],
      subtasksByParent: { p1: [mid], m1: [grandMine, { ...grandOther, assignee_ids: ['u1'] }] },
      filters: f({ assignees: ['u1'] }),
      now: NOW,
    })
    expect(r.subtasksByParent.m1).toHaveLength(2)
    expect(r.narrowedParents.size).toBe(0)
  })

  it('зацикленная цепочка родителей не вешает фильтр', () => {
    // Карта приходит с сервера плоской (parent_id → дети); битая цепочка,
    // замкнутая на себя, не должна уводить обход в бесконечность — тот же
    // повод, по которому страхуется countWithSubtasks.
    const r = filterBoardTasks({
      tasks: [top],
      subtasksByParent: { p1: [mid], m1: [grandMine], g1: [top] },
      filters: f({ assignees: ['u1'] }),
      now: NOW,
    })
    expect(ids(r.tasks)).toEqual(['p1'])
  })
})
