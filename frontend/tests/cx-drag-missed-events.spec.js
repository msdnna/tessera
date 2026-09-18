import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import process from 'node:process'
import { classifyEvent, missedRefetchKinds, partialKinds } from '@/utils/boardEvents'

// Чужие правки, пришедшие во время перетаскивания карточки (#2932).
//
// Доска на время жеста замирает: `dragging` мьютит realtime, чтобы SortableJS видел
// стабильный DOM (useBoardDragScroll). Событие в этот момент просто выбрасывалось —
// и если жест кончился ничем (подняли карточку и вернули на место, бросили мимо
// колонок, нажали Esc), vuedraggable не эмитит `change`, onColChange не зовётся,
// перезагрузки нет: всё, что коллега успел сделать за эти секунды, не доезжало до
// доски до следующего постороннего события.
//
// Окно suppress такой бухгалтерии не требует — оно взводится нашими СОБСТВЕННЫМИ
// мутациями, а за каждой из них идёт свой load()/scheduleReload().

const BOARD = 'b1'
const ev = (type, data = {}) => ({ type, data })

// Модель ровно тех трёх вещей, которыми KanbanBoard распоряжается на этом маршруте:
// флага перетаскивания, окна подавления и копилки пропущенного. Запросы не шлём —
// записываем, какие частичные фетчи (и полные перезагрузки) доска бы заказала.
function board() {
  const fetched = []
  const missed = new Set()
  let dragging = false
  let suppressUntil = 0
  const now = () => 1_000_000

  const schedulePartial = (kind) => fetched.push(kind)
  const scheduleReload = () => {
    missed.clear()
    fetched.push('full-reload')
  }
  return {
    fetched,
    missedSize: () => missed.size,
    startDrag() {
      dragging = true
    },
    suppress() {
      suppressUntil = now() + 1500
    },
    onEvent(e) {
      if (dragging) {
        for (const k of missedRefetchKinds(classifyEvent(e, { boardId: BOARD }))) missed.add(k)
        return
      }
      if (now() < suppressUntil) return
      const kind = classifyEvent(e, { boardId: BOARD })
      if (kind === 'ignore') return
      for (const k of partialKinds(kind)) schedulePartial(k)
    },
    // Перетащили и отпустили с настоящим перемещением: vuedraggable сначала эмитит
    // `change` (→ suppress + scheduleReload), и только потом `end`.
    dropWithChange() {
      this.suppress()
      scheduleReload()
      this.endDrag()
    },
    endDrag() {
      dragging = false
      if (!missed.size) return
      const kinds = [...missed]
      missed.clear()
      for (const k of kinds) schedulePartial(k)
    },
  }
}

describe('события, пропущенные во время перетаскивания', () => {
  it('жест, не изменивший ничего: пропущенное всё равно доезжает', () => {
    const b = board()
    b.startDrag()
    b.onEvent(ev('task.created', { id: 't9', board_id: BOARD }))
    expect(b.fetched).toEqual([]) // пока карточка в воздухе — ни одного запроса
    b.endDrag() // бросили на месте: `change` не было, своей перезагрузки нет
    expect(b.fetched).toEqual(['tasks'])
  })

  it('полная правка карточки во время жеста тоже не теряется — но перечитыванием, не мержем', () => {
    const b = board()
    b.startDrag()
    // Полный payload: вне драга это был бы тихий мерж на месте.
    b.onEvent(ev('task.updated', { id: 't1', board_id: BOARD, column_id: 'c1', title: 'Правка' }))
    b.endDrag()
    expect(b.fetched).toEqual(['tasks'])
  })

  it('удаление колонки во время жеста уносит с собой и её карточки', () => {
    const b = board()
    b.startDrag()
    b.onEvent(ev('column.deleted', { id: 'c2', board_id: BOARD }))
    b.endDrag()
    expect(b.fetched).toEqual(['columns', 'tasks', 'board'])
  })

  it('пачка событий схлопывается в по одному фетчу на вид', () => {
    const b = board()
    b.startDrag()
    b.onEvent(ev('task.created', { id: 't1', board_id: BOARD }))
    b.onEvent(ev('task.moved', { id: 't2', board_id: BOARD }))
    b.onEvent(ev('tag.updated', { id: 'tag1' }))
    b.onEvent(ev('column.updated', { id: 'c1', board_id: BOARD }))
    b.endDrag()
    expect(b.fetched.sort()).toEqual(['columns', 'meta', 'tasks'])
  })

  it('дроп с перемещением не платит лишним фетчем — его перезагрузка покрывает всё', () => {
    const b = board()
    b.startDrag()
    b.onEvent(ev('task.created', { id: 't9', board_id: BOARD }))
    b.dropWithChange()
    expect(b.fetched).toEqual(['full-reload'])
    expect(b.missedSize()).toBe(0)
  })

  it('посторонние события копилку не наполняют', () => {
    const b = board()
    b.startDrag()
    b.onEvent(ev('note.created', { id: 'n1' }))
    b.onEvent(ev('project.updated', { id: 'p1' }))
    b.onEvent(ev('task.moved', { id: 't5', board_id: 'b2' }))
    expect(b.missedSize()).toBe(0)
    b.endDrag()
    expect(b.fetched).toEqual([])
  })

  it('окно подавления по-прежнему глушит эхо собственной правки', () => {
    const b = board()
    b.suppress()
    b.onEvent(ev('task.updated', { id: 't1', board_id: BOARD, column_id: 'c1', title: 'Своя' }))
    expect(b.fetched).toEqual([])
  })
})

describe('missedRefetchKinds', () => {
  it('патч догоняется перечитыванием задач: мержить задним числом нечего', () => {
    expect(missedRefetchKinds('patch')).toEqual(['tasks'])
  })

  it('неинтересное событие не стоит ничего', () => {
    expect(missedRefetchKinds('ignore')).toEqual([])
    expect(missedRefetchKinds('')).toEqual([])
    expect(missedRefetchKinds(undefined)).toEqual([])
  })

  it('остальные виды совпадают с обычной маршрутизацией', () => {
    for (const kind of ['tasks', 'columns', 'board', 'meta', 'column-purge']) {
      expect(missedRefetchKinds(kind)).toEqual(partialKinds(kind))
    }
  })
})

describe('проводка в KanbanBoard', () => {
  // KanbanBoard тянет за собой весь стек доски и здесь не монтируется — проводку
  // читаем по исходнику (vitest запускается с cwd = frontend/).
  const src = readFileSync(resolve(process.cwd(), 'src/components/KanbanBoard.vue'), 'utf8')

  it('гвард перетаскивания запоминает событие, а не выбрасывает его', () => {
    expect(src).toMatch(
      /if \(dragging\.value \|\| layoutDragging\.value\) \{\s*\n\s*rememberMissed\(ev\)/,
    )
  })

  it('оба draggable сливают копилку по окончании жеста', () => {
    expect(src.match(/@end="onBoardDragEnd"/g)).toHaveLength(2)
    expect(src).not.toMatch(/@end="onDragEnd"/)
  })

  it('полная перезагрузка снимает копилку, чтобы не фетчить дважды', () => {
    expect(src).toMatch(/function scheduleReload\(\) \{[\s\S]*?missedKinds\.clear\(\)/)
  })
})
