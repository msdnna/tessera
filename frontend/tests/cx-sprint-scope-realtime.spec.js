import { describe, it, expect } from 'vitest'
import { classifyEvent, applyTaskPatch, inBoardScope } from '@/utils/boardEvents'

// Смена этапа, увиденная с доски, открытой в спринте (#2932).
//
// `?milestone=<slug|uuid|backlog>` сужает доску СЕРВЕРНО: handlers/tasks.go отдаёт
// ListBoardTasksWithMeta с `t.milestone_id = @milestone_id` (или `IS NULL` для
// backlog). Значит задача, у которой этап сменили, со спринт-доски обязана исчезнуть.
//
// Событие, которым это приходит, — обычный полный `task.updated`: бэкендовый
// `applyMilestone` (task_ops.go) пишет только milestone_id, а column_id, position и
// parent_id оставляет как были. Поэтому все прежние проверки applyTaskPatch проходят,
// и мерж на месте оставлял карточку нарисованной на чужом спринте у всех, кроме того,
// кто правку сделал.
//
// Тест идёт ровно тем маршрутом, которым событие идёт в KanbanBoard.onRealtime:
// classifyEvent → (patch) applyTaskPatch(rows, payload, patchScope()).
const BOARD = 'b1'
const COL = 'c1'
const SPRINT = 'm-sprint-3'
const OTHER = 'm-sprint-4'

// Строка доски, как её отдаёт слим-DTO.
const boardRow = (over = {}) => ({
  id: 't1',
  board_id: BOARD,
  column_id: COL,
  parent_id: null,
  title: 'Задача',
  position: 100,
  milestone_id: SPRINT,
  has_description: false,
  tag_ids: ['tagA'],
  assignee_ids: ['u2'],
  ...over,
})

// Полезная нагрузка `task.updated` — полный db.Task (GetTask).
const payload = (over = {}) => ({
  id: 't1',
  board_id: BOARD,
  column_id: COL,
  parent_id: null,
  title: 'Задача',
  description: 'текст описания',
  position: 100,
  milestone_id: SPRINT,
  ...over,
})

// Что сделает доска с событием: 'merge' — тихий мерж, 'refetch' — ре-фетч списков.
function route(rows, ev, scope) {
  const kind = classifyEvent(ev, { boardId: BOARD, hasTask: (id) => rows.some((r) => r.id === id) })
  if (kind !== 'patch') return { kind, outcome: 'refetch' }
  const patched = applyTaskPatch(rows, ev.data, scope)
  return { kind, outcome: patched ? 'merge' : 'refetch', rows: patched }
}

describe('спринт-скоуп доски и realtime-патч', () => {
  it('задача, уехавшая в другой спринт, снимается со спринт-доски через ре-фетч', () => {
    const rows = [boardRow()]
    const ev = { type: 'task.updated', data: payload({ milestone_id: OTHER }) }
    expect(route(rows, ev, { milestone: SPRINT }).outcome).toBe('refetch')
  })

  it('снятый этап тоже уводит карточку со спринт-доски', () => {
    const rows = [boardRow()]
    const ev = { type: 'task.updated', data: payload({ milestone_id: null }) }
    expect(route(rows, ev, { milestone: SPRINT }).outcome).toBe('refetch')
  })

  it('на доске ?milestone=backlog назначенный этап уводит карточку', () => {
    const rows = [boardRow({ milestone_id: null })]
    const ev = { type: 'task.updated', data: payload({ milestone_id: SPRINT }) }
    // backlog-скоуп резолвится в `milestone: null` — «задачи совсем без этапа».
    expect(route(rows, ev, { milestone: null }).outcome).toBe('refetch')
  })

  it('обычная правка внутри того же спринта по-прежнему мержится без запросов', () => {
    const rows = [boardRow()]
    const ev = { type: 'task.updated', data: payload({ title: 'Новое имя' }) }
    const r = route(rows, ev, { milestone: SPRINT })
    expect(r.outcome).toBe('merge')
    expect(r.rows[0].title).toBe('Новое имя')
    // Мерж, а не замена: агрегаты строки доски на месте, описание не подмешано.
    expect(r.rows[0].tag_ids).toEqual(['tagA'])
    expect(r.rows[0].description).toBeUndefined()
    expect(r.rows[0].has_description).toBe(true)
  })

  it('на доске без скоупа смена этапа остаётся тихим мержем', () => {
    const rows = [boardRow()]
    const ev = { type: 'task.updated', data: payload({ milestone_id: OTHER }) }
    const r = route(rows, ev, {})
    expect(r.outcome).toBe('merge')
    expect(r.rows[0].milestone_id).toBe(OTHER)
  })

  it('нерезолвнутый скоуп (слуг ещё не загружен) всегда уходит в ре-фетч', () => {
    const rows = [boardRow()]
    const ev = { type: 'task.updated', data: payload({ title: 'Новое имя' }) }
    expect(route(rows, ev, { milestone: '' }).outcome).toBe('refetch')
  })

  it('обратная ветка: задача, ВЪЕХАВШАЯ в спринт, и так шла ре-фетчем', () => {
    const rows = [boardRow({ id: 'other' })]
    const ev = { type: 'task.updated', data: payload({ milestone_id: SPRINT }) }
    expect(route(rows, ev, { milestone: SPRINT }).outcome).toBe('refetch')
  })
})

describe('inBoardScope', () => {
  it('доска без скоупа принимает что угодно', () => {
    expect(inBoardScope({ milestone_id: SPRINT }, {})).toBe(true)
    expect(inBoardScope({ milestone_id: null }, {})).toBe(true)
    expect(inBoardScope({ milestone_id: SPRINT })).toBe(true)
    expect(inBoardScope({ milestone_id: SPRINT }, null)).toBe(true)
  })

  it('скоуп по этапу сверяет id', () => {
    expect(inBoardScope({ milestone_id: SPRINT }, { milestone: SPRINT })).toBe(true)
    expect(inBoardScope({ milestone_id: OTHER }, { milestone: SPRINT })).toBe(false)
    expect(inBoardScope({ milestone_id: null }, { milestone: SPRINT })).toBe(false)
  })

  it('backlog-скоуп пускает только задачи без этапа; отсутствующее поле = без этапа', () => {
    expect(inBoardScope({ milestone_id: null }, { milestone: null })).toBe(true)
    expect(inBoardScope({}, { milestone: null })).toBe(true)
    expect(inBoardScope({ milestone_id: SPRINT }, { milestone: null })).toBe(false)
  })
})
