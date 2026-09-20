import { describe, it, expect } from 'vitest'
import { classifyEvent, missedRefetchKinds, partialKinds } from '@/utils/boardEvents'

// `task.commented` — единственное событие семейства `task.*`, которое на доске
// не меняет ничего (#2932).
//
// Строка карточки его содержимого не несёт: в board-списках нет ни счётчика
// комментариев, ни признака «комментарии есть» (backend handlers/task_list_dto.go —
// там только has_description), и TaskCard ничего такого не рисует. Потребителей у
// события нет ни одного — ни доска, ни модалка задачи его не слушают.
//
// Быстрые команды внутри коммента («/close», «/assign @msdnna») доску меняют —
// но рассылают СВОИ события через общий слой apply* (task.updated / task.moved /
// task.tagged / task.assigned …), каждое из которых здесь классифицируется само.
// Поэтому пропуск `task.commented` ничего не теряет, а раньше стоил перечитки
// списков задач на КАЖДЫЙ коммент в пространстве — включая каждый пинг бота.
const BOARD = 'b1'
const ev = (type, data) => classifyEvent({ type, data }, { boardId: BOARD })

describe('task.commented не перезагружает доску', () => {
  it('коммент проходит мимо доски', () => {
    expect(ev('task.commented', { task_id: 't1' })).toBe('ignore')
  })

  // Событие шлётся в ПРОСТРАНСТВО, а не на доску, и нагрузка у него — голое
  // `{task_id}`: ни board_id, ни id, так что отсеять его «по чужой доске»
  // (ветка evBoard) нельзя в принципе — до правки оно доходило до всех открытых
  // досок пространства без исключения.
  it('нагрузка не называет ни доску, ни задачу в поле id', () => {
    expect(ev('task.commented', { task_id: 't1' })).toBe('ignore')
    // Даже если экран держит эту карточку — держит он её без комментариев.
    expect(
      classifyEvent(
        { type: 'task.commented', data: { task_id: 't1' } },
        { boardId: BOARD, hasTask: (id) => id === 't1' },
      ),
    ).toBe('ignore')
  })

  // Пропущенное событие не должно возвращаться через чёрный ход: доска глушит
  // realtime на время перетаскивания и потом догоняет через missedRefetchKinds.
  it('пропущенный во время перетаскивания коммент не требует догоняющих чтений', () => {
    expect(missedRefetchKinds(ev('task.commented', { task_id: 't1' }))).toEqual([])
  })

  // ── контроли: соседи по семейству доску менять ОБЯЗАНЫ ──

  it('остальные id-only события задач по-прежнему перечитывают списки', () => {
    expect(ev('task.updated', { id: 't1' })).toBe('tasks')
    expect(ev('task.deleted', { id: 't1' })).toBe('tasks')
    expect(ev('task.restored', { id: 't1' })).toBe('tasks')
    expect(ev('task.archived', { id: 't1' })).toBe('tasks')
    expect(ev('task.assigned', { task_id: 't1', user_id: 'u1' })).toBe('tasks')
    expect(ev('task.unassigned', { task_id: 't1', user_id: 'u1' })).toBe('tasks')
    expect(ev('task.tagged', { task_id: 't1', tag_id: 'g1' })).toBe('tasks')
    expect(ev('task.untagged', { task_id: 't1', tag_id: 'g1' })).toBe('tasks')
  })

  // То, что делают команды из коммента, приезжает отдельными событиями — именно
  // они и перезагружают доску.
  it('быстрые команды из коммента доходят своими событиями', () => {
    expect(ev('task.moved', { id: 't1', board_id: BOARD, column_id: 'c2', title: 'Задача' })).toBe(
      'patch',
    )
    expect(ev('task.tagged', { task_id: 't1', tag_id: 'g1' })).toBe('tasks')
  })

  // Неизвестный тип по-прежнему перечитывает списки: новое событие на бэкенде не
  // должно молча переставать обновлять доску. Глушим ровно один тип, не класс.
  it('незнакомое событие задач всё ещё перечитывает списки', () => {
    expect(ev('task.reacted', { task_id: 't1' })).toBe('tasks')
    expect(partialKinds('tasks')).toEqual(['tasks'])
  })
})
