// Поле «№ или название» во вкладке «Связи» и Enter по набранному (#2932).
//
// Поле под списком связей обещает два способа адресации — номер ИЛИ название, — и
// пикер под ним честно фильтрует по обоим: строки в нём сгруппированы по проекту
// и доске, потому что задач в пространстве много и номера наизусть никто не
// помнит. А уходил в запрос только `Number(relNumber)`: набранное название
// давало NaN, `if (!n) return` съедал нажатие, и ни Enter, ни «Связать» не делали
// РОВНО НИЧЕГО — ни запроса, ни сообщения, ни открытого списка. Тупик ровно на
// том вводе, который поле и рекламирует. Туда же уезжала вставка «#2932»: «#» в
// поле нарисован префиксом, но скопированный номер несёт его с собой.
//
// Здесь пинится разбор набранного ДО запроса: «#123» это 123, а название,
// совпавшее ровно с одной задачей, связывает именно её (несколько совпадений —
// открываем отфильтрованный список, пустое поле — тоже список, но связывать
// наугад нечего).
//
// Рядом — вторая половина той же дороги: связь с уже связанной задачей ТЕМ ЖЕ
// видом. Ключ связи это (пара, вид), вставка идёт ON CONFLICT DO NOTHING — запрос
// возвращался 201, не сделав ничего: список не менялся, объяснения не было, а в
// историю ОБЕИХ задач падала вторая строка «добавил связь». Такие задачи убраны
// из пикера (как это давно делает DocLinks) и отсекаются при наборе номера — но
// ровно по своему виду: та же пара с другим видом связи это другая связь.
//
// И гвард «запрос в полёте»: поле очищается только по факту ответа, так что
// нетерпеливый второй Enter слал второе создание.
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'

const error = vi.fn()
const info = vi.fn()
const warning = vi.fn()
vi.mock('naive-ui', async () => {
  const actual = await vi.importActual('naive-ui')
  return {
    ...actual,
    useMessage: () => ({ error, info, warning, success: vi.fn() }),
  }
})

const addRelation = vi.fn()
const relations = vi.fn()
const removeRelation = vi.fn()
const wsTasks = vi.fn()
vi.mock('@/api', () => ({
  tasks: {
    addRelation: (...a) => addRelation(...a),
    relations: (...a) => relations(...a),
    removeRelation: (...a) => removeRelation(...a),
  },
  workspaces: { tasks: (...a) => wsTasks(...a) },
}))

const TASK = 't-1'
// Задачи пространства: две с «оплат» в названии и одна уникальная.
const WS_TASKS = [
  { id: 't-2', number: 11, title: 'Оплата картой', project_name: 'P', board_name: 'B' },
  { id: 't-3', number: 12, title: 'Оплата по счёту', project_name: 'P', board_name: 'B' },
  { id: 't-4', number: 13, title: 'Экспорт в PDF', project_name: 'P', board_name: 'B' },
]

const stubs = { EmptyState: true, teleport: true }

const tick = async (w, n = 6) => {
  for (let i = 0; i < n; i++) await w.vm.$nextTick()
}

function deferred() {
  let resolve
  const promise = new Promise((res) => {
    resolve = res
  })
  return { promise, resolve }
}

async function openTab(rels = []) {
  const TaskRelationsTab = (await import('@/components/task/TaskRelationsTab.vue')).default
  const w = mount(TaskRelationsTab, {
    props: { taskId: TASK, relations: rels, wsId: 'ws-1' },
    global: { stubs },
  })
  await tick(w)
  return w
}

// Набрать в поле и нажать Enter (та же дорога, что и кнопка «Связать»).
async function type(w, text) {
  w.vm.relNumber = text
  await tick(w)
  await w.vm.addRelation()
  await tick(w)
}

describe('вкладка «Связи»: набранное в поле «№ или название»', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    addRelation.mockResolvedValue({ data: {} })
    relations.mockResolvedValue({ data: [] })
    wsTasks.mockResolvedValue({ data: WS_TASKS })
  })

  it('название, совпавшее ровно с одной задачей, связывает именно её', async () => {
    const w = await openTab()
    await type(w, 'экспорт')
    expect(addRelation).toHaveBeenCalledWith(TASK, 13, 'relates')
  })

  it('название с несколькими совпадениями не связывает наугад, а открывает список', async () => {
    const w = await openTab()
    await type(w, 'оплат')
    expect(addRelation).not.toHaveBeenCalled()
    expect(w.vm.relPickerOpen).toBe(true)
    // Список под полем уже отфильтрован — выбирать есть из чего.
    expect(w.vm.relGroups.flatMap((g) => g.tasks).map((x) => x.number)).toEqual([11, 12])
  })

  it('название, не совпавшее ни с чем, говорит об этом вместо тишины', async () => {
    const w = await openTab()
    await type(w, 'такого нет')
    expect(addRelation).not.toHaveBeenCalled()
    expect(warning).toHaveBeenCalled()
  })

  it('вставленный «#13» читается как номер 13', async () => {
    const w = await openTab()
    await type(w, '#13')
    expect(addRelation).toHaveBeenCalledWith(TASK, 13, 'relates')
  })

  it('обычный путь: набранный номер по-прежнему связывает', async () => {
    const w = await openTab()
    await type(w, '42')
    expect(addRelation).toHaveBeenCalledWith(TASK, 42, 'relates')
  })

  it('пустое поле не связывает ничего — даже когда совпадение ровно одно', async () => {
    // Единственная задача пространства: «первое совпадение» связало бы её молча.
    wsTasks.mockResolvedValue({ data: [WS_TASKS[2]] })
    const w = await openTab()
    await type(w, '   ')
    expect(addRelation).not.toHaveBeenCalled()
  })
})

describe('вкладка «Связи»: задача, уже связанная этим видом', () => {
  const LINKED = [
    { id: 'r-1', kind: 'relates', related_number: 13, related_title: 'Экспорт в PDF' },
  ]

  beforeEach(() => {
    vi.clearAllMocks()
    addRelation.mockResolvedValue({ data: {} })
    relations.mockResolvedValue({ data: [] })
    wsTasks.mockResolvedValue({ data: WS_TASKS })
  })

  it('набранный номер уже связанной задачи не уходит в запрос', async () => {
    const w = await openTab(LINKED)
    await type(w, '13')
    expect(addRelation).not.toHaveBeenCalled()
    expect(info).toHaveBeenCalled()
    expect(w.vm.relNumber).toBe(null)
  })

  it('та же задача с ДРУГИМ видом связи — это другая связь, запрос уходит', async () => {
    const w = await openTab(LINKED)
    w.vm.relKind = 'blocks'
    await tick(w)
    await type(w, '13')
    expect(addRelation).toHaveBeenCalledWith(TASK, 13, 'blocks')
  })

  it('уже связанной этим видом задачи нет в пикере, с другим видом — есть', async () => {
    const w = await openTab(LINKED)
    w.vm.openRelPicker() // фокус в поле: список задач подгружается лениво
    await tick(w)
    expect(w.vm.relGroups.flatMap((g) => g.tasks).map((x) => x.number)).toEqual([11, 12])
    w.vm.relKind = 'blocks'
    await tick(w)
    expect(w.vm.relGroups.flatMap((g) => g.tasks).map((x) => x.number)).toEqual([11, 12, 13])
  })
})

describe('вкладка «Связи»: второй Enter по летящему запросу', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    relations.mockResolvedValue({ data: [] })
    wsTasks.mockResolvedValue({ data: WS_TASKS })
  })

  it('шлёт ровно одно создание', async () => {
    const sent = deferred()
    addRelation.mockReturnValue(sent.promise)
    const w = await openTab()
    w.vm.relNumber = '13'
    await tick(w)
    w.vm.addRelation()
    await tick(w)
    w.vm.addRelation() // нетерпение: поле ещё не очищено
    await tick(w)
    expect(addRelation).toHaveBeenCalledTimes(1)

    sent.resolve({ data: {} })
    await tick(w)
    // Ответ пришёл — поле снова принимает ввод.
    w.vm.relNumber = '11'
    await tick(w)
    await w.vm.addRelation()
    await tick(w)
    expect(addRelation).toHaveBeenCalledTimes(2)
  })
})
