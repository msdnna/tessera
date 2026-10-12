// Перетаскивание в матрице Эйзенхауэра не останавливало доску (#2932).
//
// Пока карточка в воздухе, SortableJS владеет DOM списка, поэтому доска намеренно
// замирает: `dragging` из useBoardDragScroll мьютит realtime-перезагрузки. Но
// поднимают этот флаг только draggable'ы самого канбана — у BoardMatrixView был свой
// ЛОКАЛЬНЫЙ флаг, наружу он не отдавался. Событие коллеги, пришедшее во время жеста
// в матрице, шло обычным маршрутом: доска перечитывала задачи, watcher матрицы звал
// rebuild() и подменял все четыре ведра НОВЫМИ массивами прямо под Sortable. Карточка
// возвращалась на место, а дроп считал индекс по перерисованному списку — onQuadChange
// берёт evt.added.element и мог закрепить в квадранте чужую задачу.
//
// Прибито здесь: матрица сообщает о жесте наружу (в т.ч. снимает флаг, если её
// размонтировали посреди жеста — `end` от Sortable тогда не придёт и realtime замолчал
// бы до конца сессии), а доска включает этот флаг в тот же гвард и сливает копилку
// пропущенного по окончании.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount } from '@vue/test-utils'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import process from 'node:process'
import draggable from 'vuedraggable'

vi.mock('naive-ui', async () => {
  const actual = await vi.importActual('naive-ui')
  return {
    ...actual,
    useMessage: () => ({ error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }),
  }
})

vi.mock('@/api', () => ({
  tasks: { eisenhower: vi.fn(() => Promise.resolve({})) },
}))

const { default: BoardMatrixView } = await import('@/components/BoardMatrixView.vue')

const TASK = { id: 't1', title: 'Карточка', eisenhower_quadrant: 0, priority: 1 }

// useFormat() читает таймзону из стора (ось срочности считается в календарных днях),
// так что гриду нужна живая pinia.
beforeEach(() => setActivePinia(createPinia()))

const mountMatrix = () =>
  mount(BoardMatrixView, {
    props: { tasks: [TASK] },
    global: { stubs: { TaskCard: true, TesseraIcon: true } },
  })

// Первый квадрант: именно в его списке лежит карточка.
const firstList = (w) => w.findAllComponents(draggable)[0]

describe('BoardMatrixView сообщает о своём перетаскивании', () => {
  it('поднимает флаг на start и снимает на end', async () => {
    const w = mountMatrix()
    const list = firstList(w)

    list.vm.$emit('start')
    await w.vm.$nextTick()
    expect(w.emitted('drag-state')).toEqual([[true]])

    list.vm.$emit('end')
    await w.vm.$nextTick()
    expect(w.emitted('drag-state')).toEqual([[true], [false]])
    w.unmount()
  })

  it('снимает флаг, если её размонтировали посреди жеста', () => {
    const w = mountMatrix()
    firstList(w).vm.$emit('start')
    expect(w.emitted('drag-state')).toEqual([[true]])

    w.unmount()
    // Последнее, что доска услышала от размонтированного грида — «жест окончен».
    expect(w.emitted('drag-state').at(-1)).toEqual([false])
  })

  it('на обычном размонтировании лишнего не эмитит', () => {
    const w = mountMatrix()
    w.unmount()
    expect(w.emitted('drag-state')).toBeUndefined()
  })

  it('локальный флаг по-прежнему доезжает до карточки', async () => {
    const w = mountMatrix()
    firstList(w).vm.$emit('start')
    await w.vm.$nextTick()

    // TaskCard застабан, поэтому смотрим на проп, который ему передан.
    expect(w.findComponent({ name: 'TaskCard' }).props('dragging')).toBe(true)
    w.unmount()
  })
})

// KanbanBoard тянет за собой весь стек доски и здесь не монтируется — проводку
// читаем по исходнику (vitest запускается с cwd = frontend/).
const kanban = readFileSync(resolve(process.cwd(), 'src/components/KanbanBoard.vue'), 'utf8')

describe('проводка в KanbanBoard', () => {
  it('слушает жест матрицы', () => {
    const view = kanban.slice(kanban.indexOf('<BoardMatrixView'))
    expect(view.slice(0, view.indexOf('/>'))).toContain('@drag-state="onLayoutDragState"')
  })

  it('включает флаг макета в тот же гвард, что и свой', () => {
    expect(kanban).toMatch(/if \(dragging\.value \|\| layoutDragging\.value\) \{/)
  })

  it('по окончании жеста сливает копилку пропущенного', () => {
    const fn = kanban.match(/function onLayoutDragState\(active\) \{[\s\S]*?\n\}/)
    expect(fn, 'onLayoutDragState is gone').toBeTruthy()
    expect(fn[0]).toContain('layoutDragging.value = !!active')
    expect(fn[0]).toContain('if (!active) flushMissed()')
  })
})
