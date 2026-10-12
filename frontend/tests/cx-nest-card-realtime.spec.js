import { describe, it, expect } from 'vitest'
import {
  classifyEvent,
  isFullTask,
  applyTaskPatch,
  applySubtaskPatch,
  mergeTaskRow,
} from '@/utils/boardEvents'

// Вкладывание карточки в родителя, увиденное с чужой открытой доски (#2932).
//
// Бэкенд (handlers/task_ops.go `applyParent` → db.SetTaskParent) при вложении пишет
// parent_id, board_id и column_id — и НЕ трогает position, — после чего шлёт один
// полный `task.moved`. А доска рисует только те задачи, у которых в этом же виде не
// нарисован родитель (ListBoardTasksWithMeta, `AND NOT EXISTS (… p.id = t.parent_id …)`).
// Значит вложение обязано снимать карточку с доски у всех остальных — то есть кончаться
// ре-фетчем списков, а не тихим мержем на месте.
//
// Тест повторяет ровно тот маршрут, которым событие идёт в KanbanBoard.onRealtime:
// classifyEvent → (patch) applyTaskPatch → applySubtaskPatch → schedulePartial('tasks').
const BOARD = 'b1'
const COL = 'c1'

// Строка доски, как её отдаёт слим-DTO: без description, зато с агрегатами.
const boardRow = (over = {}) => ({
  id: 't1',
  board_id: BOARD,
  column_id: COL,
  parent_id: null,
  title: 'Задача',
  position: 100,
  has_description: false,
  tag_ids: ['tagA'],
  assignee_ids: ['u2'],
  ...over,
})

// Полезная нагрузка `task.moved` — полный db.Task.
const movedPayload = (over = {}) => ({
  id: 't1',
  board_id: BOARD,
  column_id: COL,
  parent_id: null,
  title: 'Задача',
  description: 'много markdown',
  position: 100,
  completed_at: null,
  ...over,
})

// Что сделает доска, получив событие: 'patch-in-place' — карточку смержили и
// запросов не будет; 'refetch-tasks' — пойдёт schedulePartial('tasks').
function route(rows, byParent, payload) {
  const kind = classifyEvent({ type: 'task.moved', data: payload }, { boardId: BOARD })
  if (kind !== 'patch') return kind
  if (applyTaskPatch(rows, payload)) return 'patch-in-place'
  if (applySubtaskPatch(byParent, payload)) return 'patch-subtask'
  return 'refetch-tasks'
}

describe('вложение карточки в родителя по realtime', () => {
  it('родитель в той же колонке: доска перечитывает списки, а не мержит на месте', () => {
    const rows = [boardRow(), boardRow({ id: 'p1', position: 50 })]
    // Единственное, что изменилось — parent_id: колонка та же, position не трогали.
    const payload = movedPayload({ parent_id: 'p1' })
    expect(isFullTask(payload)).toBe(true)
    expect(route(rows, {}, payload)).toBe('refetch-tasks')
  })

  it('родитель в соседней колонке — тоже ре-фетч (эта ветка ловилась и раньше)', () => {
    const rows = [boardRow()]
    expect(route(rows, {}, movedPayload({ parent_id: 'p1', column_id: 'c2' }))).toBe(
      'refetch-tasks',
    )
  })

  it('отцепление подзадачи обратно на доску — ре-фетч', () => {
    const byParent = { p1: [boardRow({ id: 's1', parent_id: 'p1' })] }
    expect(route([], byParent, movedPayload({ id: 's1', parent_id: null }))).toBe('refetch-tasks')
  })

  it('переезд подзадачи к другому родителю — ре-фетч', () => {
    const byParent = { p1: [boardRow({ id: 's1', parent_id: 'p1' })], p2: [] }
    expect(route([], byParent, movedPayload({ id: 's1', parent_id: 'p2' }))).toBe('refetch-tasks')
  })

  it('вложение на другой доске приходит с чужим board_id и нас не касается', () => {
    const kind = classifyEvent(
      { type: 'task.moved', data: movedPayload({ board_id: 'b2', parent_id: 'p1' }) },
      { boardId: BOARD, hasTask: () => false },
    )
    expect(kind).toBe('ignore')
  })
})

describe('обычная правка карточки по-прежнему мержится без запросов', () => {
  it('смена заголовка/приоритета внутри той же колонки', () => {
    const rows = [boardRow()]
    expect(route(rows, {}, movedPayload({ title: 'Правка' }))).toBe('patch-in-place')
  })

  it('карточка под заархивированным родителем: parent_id не null, но он не менялся', () => {
    // Такую строку доска рисует как карточку (NOT EXISTS смотрит на видимого
    // родителя, а не на сам parent_id) — её правки обязаны мержиться как обычно.
    const rows = [boardRow({ parent_id: 'arch' })]
    expect(route(rows, {}, movedPayload({ parent_id: 'arch', title: 'Правка' }))).toBe(
      'patch-in-place',
    )
  })

  it('правка подзадачи внутри списка родителя', () => {
    const byParent = { p1: [boardRow({ id: 's1', parent_id: 'p1' })] }
    expect(route([], byParent, movedPayload({ id: 's1', parent_id: 'p1', title: 'Правка' }))).toBe(
      'patch-subtask',
    )
  })

  it('мерж не тащит description в строку доски, но правит has_description', () => {
    const out = applyTaskPatch([boardRow()], movedPayload({ description: 'текст' }))
    expect(out[0]).not.toHaveProperty('description')
    expect(out[0].has_description).toBe(true)
    expect(out[0].tag_ids).toEqual(['tagA'])
  })

  it('mergeTaskRow сам по себе parent_id не теряет', () => {
    const merged = mergeTaskRow(boardRow(), movedPayload({ parent_id: 'p1' }))
    expect(merged.parent_id).toBe('p1')
  })
})
