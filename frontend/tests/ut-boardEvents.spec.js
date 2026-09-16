import { describe, it, expect } from 'vitest'
import {
  classifyEvent,
  partialKinds,
  isFullTask,
  mergeTaskRow,
  applyTaskPatch,
  applySubtaskPatch,
  boardGone,
} from '@/utils/boardEvents'

const BOARD = 'b1'
const ctx = { boardId: BOARD }
const ev = (type, data) => classifyEvent({ type, data }, ctx)

// A full task payload, shaped like backend db.Task (handlers/tasks.go:727).
const full = (over = {}) => ({
  id: 't1',
  board_id: BOARD,
  column_id: 'c1',
  parent_id: null,
  title: 'Задача',
  description: 'много markdown',
  priority: 2,
  due_date: null,
  position: 100,
  created_by: 'u1',
  completed_at: null,
  milestone_id: null,
  ...over,
})

// A board row, shaped like ListBoardTasksWithMetaRow after the slim DTO —
// no `description`, but with the aggregates the payload never carries.
const row = (over = {}) => ({
  id: 't1',
  board_id: BOARD,
  column_id: 'c1',
  title: 'Задача',
  priority: 0,
  position: 100,
  has_description: false,
  tag_ids: ['tagA'],
  assignee_ids: ['u2'],
  gitlab_iid: 42,
  ...over,
})

describe('classifyEvent — таблица маршрутизации', () => {
  it('всё, что не про эту доску, не трогает её вовсе', () => {
    for (const t of [
      'note.created',
      'note.updated',
      'note.deleted',
      'notification',
      'workspace.updated',
      'workspace.estimation',
      'group.created',
      'group.updated',
      'group.deleted',
      'group.moved',
      'project.created',
      'project.updated',
      'project.deleted',
      'project.moved',
      'project.estimation',
      'board.created',
      'board.deleted',
      'workspace_commands.updated',
      'gitlab.conflict',
      'milestone.created',
      'milestone.updated',
      'milestone.deleted',
    ]) {
      expect(ev(t, {}), t).toBe('ignore')
    }
  })

  it('колонки, доска и мета — каждая своим запросом', () => {
    expect(ev('column.created', {})).toBe('columns')
    expect(ev('column.updated', {})).toBe('columns')
    expect(ev('column.moved', {})).toBe('columns')
    expect(ev('board.updated', {})).toBe('board')
    expect(ev('tag.created', {})).toBe('meta')
    expect(ev('tag.updated', {})).toBe('meta')
    expect(ev('tag.deleted', {})).toBe('meta')
    expect(ev('tag_prefixes.updated', {})).toBe('meta')
    expect(ev('integration.sync', {})).toBe('meta')
  })

  // Удаление колонки уносит с собой все карточки в ней (сервер каскадит через
  // tasks.column_id), а в WS летит ОДНО событие column.deleted — ни одного
  // task.deleted. Перечитать только список колонок мало: осиротевшие задачи
  // остаются в allTasks и в группировке по тегам/вехам (и в list/gantt/calendar)
  // раскладываются как живые — видимые и кликабельные. Плюс done_column_id
  // доски мог обнулиться (FK ON DELETE SET NULL). #2932
  it('удаление колонки перечитывает и задачи, и доску, не только колонки', () => {
    expect(ev('column.deleted', {})).toBe('column-purge')
    const kinds = partialKinds('column-purge')
    expect(kinds).toContain('columns')
    expect(kinds).toContain('tasks')
    expect(kinds).toContain('board')
  })

  // Колонки и board.updated летят в ВЕСЬ воркспейс, а не только на свою доску:
  // коллега переименовал колонку на соседней доске — и у всех открытых досок был
  // лишний GET /columns, а его удаление колонки стоило каждой трёх запросов
  // (column-purge). Оба вида событий называют свою доску — board.updated самим
  // board-рядом (поле id), колоночные — board_id, — так что чужое отсеивается. #2932
  it('колоночные события и board.updated с ЧУЖОЙ доски не трогают эту', () => {
    expect(ev('column.created', { board_id: 'b2' })).toBe('ignore')
    expect(ev('column.updated', { board_id: 'b2' })).toBe('ignore')
    expect(ev('column.moved', { board_id: 'b2' })).toBe('ignore')
    expect(ev('column.deleted', { id: 'c9', board_id: 'b2' })).toBe('ignore')
    expect(ev('board.updated', { id: 'b2' })).toBe('ignore')
  })

  it('те же события со СВОЕЙ доски обрабатываются как раньше', () => {
    expect(ev('column.created', { board_id: BOARD })).toBe('columns')
    expect(ev('column.updated', { board_id: BOARD })).toBe('columns')
    expect(ev('column.moved', { board_id: BOARD })).toBe('columns')
    expect(ev('column.deleted', { id: 'c9', board_id: BOARD })).toBe('column-purge')
    expect(ev('board.updated', { id: BOARD })).toBe('board')
  })

  // Удаление ОТКРЫТОЙ доски: сервер уносит её колонки, карточки и вложения и
  // шлёт голое board.deleted {id}. Перечитывать нечего — доски больше нет, — так
  // что событие остаётся 'ignore', а распознаёт его отдельный предикат: без него
  // читатель (и сам удаливший — сайдбар после удаления никуда не уводит)
  // продолжает таскать карточки, которых уже нет, и каждый вызов идёт в 404. #2932
  it('удаление доски не перечитывается, а распознаётся предикатом', () => {
    expect(ev('board.deleted', { id: BOARD })).toBe('ignore')
    expect(boardGone({ type: 'board.deleted', data: { id: BOARD } }, BOARD)).toBe(true)
    // Чужая доска того же пространства — не наше дело.
    expect(boardGone({ type: 'board.deleted', data: { id: 'b2' } }, BOARD)).toBe(false)
    // Ни другое событие про ту же доску, ни пустой id не считаются уходом.
    expect(boardGone({ type: 'board.updated', data: { id: BOARD } }, BOARD)).toBe(false)
    expect(boardGone({ type: 'board.deleted', data: {} }, BOARD)).toBe(false)
    expect(boardGone({ type: 'board.deleted', data: { id: BOARD } }, null)).toBe(false)
  })

  it('обычные классификации остаются одним запросом', () => {
    for (const k of ['tasks', 'columns', 'board', 'meta']) {
      expect(partialKinds(k), k).toEqual([k])
    }
  })

  it('task-события с полным объектом патчатся, id-only идут в ре-фетч', () => {
    expect(ev('task.updated', full())).toBe('patch')
    expect(ev('task.created', full())).toBe('patch')
    expect(ev('task.moved', full())).toBe('patch')
    // id-only формы, которые реально шлёт бэкенд
    expect(ev('task.updated', { id: 't1' })).toBe('tasks')
    expect(ev('task.updated', { task_id: 't1' })).toBe('tasks')
    expect(ev('task.deleted', { id: 't1' })).toBe('tasks')
    expect(ev('task.restored', { id: 't1' })).toBe('tasks')
    expect(ev('task.archived', { id: 't1' })).toBe('tasks')
    expect(ev('task.commented', { task_id: 't1' })).toBe('tasks')
    expect(ev('task.assigned', { task_id: 't1', user_id: 'u1' })).toBe('tasks')
    expect(ev('task.unassigned', { task_id: 't1', user_id: 'u1' })).toBe('tasks')
    expect(ev('task.tagged', { task_id: 't1', tag_id: 'g1' })).toBe('tasks')
    expect(ev('task.untagged', { task_id: 't1', tag_id: 'g1' })).toBe('tasks')
  })

  it('событие с чужим board_id до доски не доходит', () => {
    expect(ev('task.updated', full({ board_id: 'other' }))).toBe('ignore')
    expect(ev('task.synced', { board_id: 'other', created: 1, updated: 2 })).toBe('ignore')
    expect(ev('task.synced', { board_id: BOARD, created: 1, updated: 2 })).toBe('tasks')
  })

  // Уход карточки на другую доску объявляется задачей ПОСЛЕ перенода: board_id в
  // событии — уже целевой, от чужого его отличает только то, что карточку мы всё
  // ещё держим. Обе двери на другую доску (перенос из хлебной крошки модалки и
  // setParent на карточку соседней доски) шлют именно такой task.moved (#2932).
  describe('карточка уехала с этой доски', () => {
    const mine = (type, data) =>
      classifyEvent({ type, data }, { boardId: BOARD, hasTask: (id) => id === 't1' })

    it('task.moved с чужим board_id про НАШУ карточку перечитывает списки', () => {
      expect(mine('task.moved', full({ id: 't1', board_id: 'other' }))).toBe('tasks')
    })

    it('та же карточка на своей доске по-прежнему патчится без запросов', () => {
      expect(mine('task.moved', full({ id: 't1' }))).toBe('patch')
    })

    it('чужая карточка на чужой доске остаётся чужой', () => {
      expect(mine('task.moved', full({ id: 't9', board_id: 'other' }))).toBe('ignore')
      expect(mine('task.updated', full({ id: 't9', board_id: 'other' }))).toBe('ignore')
    })

    it('не-task событие чужой доски молчит, даже совпав по id', () => {
      expect(mine('gitlab.something', { id: 't1', board_id: 'other' })).toBe('ignore')
    })

    it('без hasTask поведение прежнее — чужая доска молчит', () => {
      expect(ev('task.moved', full({ id: 't1', board_id: 'other' }))).toBe('ignore')
    })
  })

  it('неизвестный тип перезагружает задачи, а не молча теряется', () => {
    expect(ev('task.somethingNew', { id: 't1' })).toBe('tasks')
    expect(ev('totally.unknown', {})).toBe('tasks')
  })

  it('мусорные кадры не роняют классификатор', () => {
    expect(classifyEvent(null, ctx)).toBe('ignore')
    expect(classifyEvent({}, ctx)).toBe('ignore')
    expect(classifyEvent({ type: 42 }, ctx)).toBe('ignore')
  })
})

describe('isFullTask', () => {
  it('отличает полный объект от id-only', () => {
    expect(isFullTask(full())).toBe(true)
    expect(isFullTask({ id: 't1' })).toBe(false)
    expect(isFullTask({ task_id: 't1' })).toBe(false)
    expect(isFullTask(null)).toBe(false)
  })
})

describe('mergeTaskRow', () => {
  it('не втаскивает description в строку карточки, но чинит has_description', () => {
    const out = mergeTaskRow(row(), full({ description: 'много markdown' }))
    expect('description' in out).toBe(false)
    expect(out.has_description).toBe(true)
    expect(
      mergeTaskRow(row({ has_description: true }), full({ description: '' })).has_description,
    ).toBe(false)
  })

  it('merge, а не replace: агрегаты строки доски переживают патч', () => {
    const out = mergeTaskRow(row(), full({ title: 'Новое имя', priority: 3 }))
    expect(out.title).toBe('Новое имя')
    expect(out.priority).toBe(3)
    expect(out.tag_ids).toEqual(['tagA'])
    expect(out.assignee_ids).toEqual(['u2'])
    expect(out.gitlab_iid).toBe(42)
  })

  it('возвращает новый объект, исходную строку не трогает', () => {
    const r = row()
    const out = mergeTaskRow(r, full({ title: 'Новое имя' }))
    expect(out).not.toBe(r)
    expect(r.title).toBe('Задача')
  })
})

describe('applyTaskPatch', () => {
  it('патчит задачу на месте и отдаёт новый массив', () => {
    const rows = [row(), row({ id: 't2', position: 200 })]
    const out = applyTaskPatch(rows, full({ title: 'Правка' }))
    expect(out).not.toBe(rows)
    expect(out[0].title).toBe('Правка')
    expect(out[1]).toBe(rows[1])
    expect(rows[0].title).toBe('Задача')
  })

  it('null (→ ре-фетч), когда карточку надо переставить или её ещё нет', () => {
    const rows = [row()]
    expect(applyTaskPatch(rows, full({ id: 'нет-такой' }))).toBe(null)
    expect(applyTaskPatch(rows, full({ column_id: 'c2' }))).toBe(null)
    expect(applyTaskPatch(rows, full({ position: 999 }))).toBe(null)
  })
})

describe('applySubtaskPatch', () => {
  const byParent = () => ({ p1: [row({ id: 's1', parent_id: 'p1' })] })

  it('патчит подзадачу внутри списка родителя', () => {
    const src = byParent()
    const out = applySubtaskPatch(src, full({ id: 's1', parent_id: 'p1', title: 'Правка' }))
    expect(out.p1[0].title).toBe('Правка')
    expect(out).not.toBe(src)
    expect(src.p1[0].title).toBe('Задача')
  })

  it('null, когда родитель неизвестен, ребёнка нет в списке или он переехал', () => {
    expect(applySubtaskPatch(byParent(), full({ id: 's1', parent_id: null }))).toBe(null)
    expect(applySubtaskPatch(byParent(), full({ id: 's1', parent_id: 'p9' }))).toBe(null)
    expect(applySubtaskPatch(byParent(), full({ id: 'нет', parent_id: 'p1' }))).toBe(null)
    expect(applySubtaskPatch(byParent(), full({ id: 's1', parent_id: 'p1', position: 999 }))).toBe(
      null,
    )
  })
})
