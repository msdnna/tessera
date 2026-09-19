// Устаревший ответ в сторе пространств (#2932).
//
// Счётчик поколений здесь уже есть — но ровно в одном месте: selectWorkspace
// сверяет selectSeq и уступает дерево более позднему переключению. Всё
// остальное, что пишет то же самое состояние, не сверяется ни с чем:
//
//   • refresh() берёт id на входе и после await ПРИСВАИВАЕТ groups/projects.
//     Его зовут после каждого create/rename/delete/move в сайдбаре и на каждом
//     resync — то есть именно тогда, когда связь флапает. Пока он в полёте,
//     читатель успевает уйти в другое пространство, и поздний ответ рисует в
//     сайдбаре проекты покинутого: клик по такому проекту уводит за пределы
//     текущего скоупа.
//   • loadCommands пишет словарь быстрых действий, а в catch его ещё и гасит —
//     упавший запрос покинутого пространства оставляет открытое без команд.
//   • resync/loadWorkspaces присваивают list.value. Список — это не только
//     строки сайдбара, но и my_role, то есть весь canManage.
//   • loadBoards вставляет доски в карту, которую переключение только что
//     очистило.
import { setActivePinia, createPinia } from 'pinia'
import { beforeEach, afterEach, describe, it, expect, vi } from 'vitest'

const apiMock = vi.hoisted(() => ({
  workspaces: {
    list: vi.fn(),
    groups: vi.fn(),
    projects: vi.fn(),
    commands: vi.fn(),
    remove: vi.fn(() => Promise.resolve()),
  },
  projects: { boards: vi.fn() },
}))
vi.mock('@/api', () => apiMock)
const expanded = vi.hoisted(() => ({ value: false }))
vi.mock('@/composables/useTreeExpand', () => ({
  useTreeExpand: () => ({ isExpanded: () => expanded.value }),
}))

import { useWorkspacesStore } from '@/stores/workspaces'

const ws = (id, role = 'member') => ({ id, name: id, my_role: role })
const row = (id, wsId) => ({ id, name: id, position: 0, workspace_id: wsId })
const cmds = (custom, canManage = false) => ({
  data: { builtin: [], custom, can_manage: canManage },
})

// Ответ, которым в тесте управляют вручную.
function deferred() {
  let resolve, reject
  const promise = new Promise((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}
// Прокрутить микрозадачи, чтобы дойти до следующего await внутри стора.
async function flush() {
  for (let i = 0; i < 12; i++) await Promise.resolve()
}

// Дерево отвечает по пространству, которое спросили: так видно, чьё именно
// осталось на экране.
function treePerWorkspace() {
  apiMock.workspaces.groups.mockImplementation((id) =>
    Promise.resolve({ data: [row(`g-${id}`, id)] }),
  )
  apiMock.workspaces.projects.mockImplementation((id) =>
    Promise.resolve({ data: [row(`p-${id}`, id)] }),
  )
  apiMock.workspaces.commands.mockImplementation(() => Promise.resolve(cmds([])))
  apiMock.projects.boards.mockResolvedValue({ data: [] })
}

beforeEach(() => {
  localStorage.clear()
  vi.clearAllMocks()
  expanded.value = false
  setActivePinia(createPinia())
  treePerWorkspace()
})
afterEach(() => localStorage.clear())

// Приложение уже загружено: два пространства, открыто первое.
async function loaded(list = [ws('w1'), ws('w2')]) {
  apiMock.workspaces.list.mockResolvedValue({ data: list })
  const store = useWorkspacesStore()
  await store.loadWorkspaces()
  expect(store.currentId).toBe(list[0].id)
  return store
}

describe('стор пространств: устаревший ответ', () => {
  it('поздний refresh покинутого пространства не подменяет дерево', async () => {
    const store = await loaded()
    // Переименовали проект в w1 — сайдбар перечитывает дерево, ответ подвис.
    const g1 = deferred()
    const p1 = deferred()
    apiMock.workspaces.groups.mockImplementation((id) =>
      id === 'w1' ? g1.promise : Promise.resolve({ data: [row(`g-${id}`, id)] }),
    )
    apiMock.workspaces.projects.mockImplementation((id) =>
      id === 'w1' ? p1.promise : Promise.resolve({ data: [row(`p-${id}`, id)] }),
    )
    const pending = store.refresh()

    // …и читатель ушёл в w2, которое ответило быстрее.
    await store.selectWorkspace('w2')
    expect(store.projects.map((p) => p.id)).toEqual(['p-w2'])

    g1.resolve({ data: [row('g-w1', 'w1')] })
    p1.resolve({ data: [row('p-w1', 'w1')] })
    await pending

    expect(store.currentId).toBe('w2')
    expect(store.projects.map((p) => p.id)).toEqual(['p-w2'])
    expect(store.groups.map((g) => g.id)).toEqual(['g-w2'])
  })

  it('поздний словарь команд покинутого пространства не подменяет открытое', async () => {
    const store = await loaded()
    const c1 = deferred()
    apiMock.workspaces.commands.mockImplementation((id) =>
      id === 'w1' ? c1.promise : Promise.resolve(cmds([{ name: 'w2cmd', body: 'b' }], true)),
    )
    // Переключение в w1: дерево пришло, словарь завис.
    const pending = store.selectWorkspace('w1')
    await flush()

    await store.selectWorkspace('w2')
    expect(store.customCommands.map((c) => c.name)).toEqual(['w2cmd'])

    c1.resolve(cmds([{ name: 'w1cmd', body: 'b' }], false))
    await pending

    expect(store.customCommands.map((c) => c.name)).toEqual(['w2cmd'])
    expect(store.commandsCanManage).toBe(true)
  })

  it('упавший словарь команд покинутого пространства не гасит открытый', async () => {
    const store = await loaded()
    const c1 = deferred()
    apiMock.workspaces.commands.mockImplementation((id) =>
      id === 'w1' ? c1.promise : Promise.resolve(cmds([{ name: 'w2cmd', body: 'b' }], true)),
    )
    const pending = store.selectWorkspace('w1')
    await flush()
    await store.selectWorkspace('w2')

    c1.reject(new Error('сеть'))
    await pending

    expect(store.customCommands.map((c) => c.name)).toEqual(['w2cmd'])
    expect(store.commandsCanManage).toBe(true)
  })

  it('поздний resync не возвращает прежний список пространств', async () => {
    const store = await loaded([ws('w1')])
    // Связь флапнула дважды — два resync в полёте, первый отвечает последним.
    const l1 = deferred()
    apiMock.workspaces.list.mockReturnValueOnce(l1.promise)
    apiMock.workspaces.list.mockResolvedValue({ data: [ws('w1', 'admin'), ws('w3')] })
    const pending = store.resync()
    await store.resync()
    expect(store.list.map((w) => w.id)).toEqual(['w1', 'w3'])

    l1.resolve({ data: [ws('w1')] })
    await pending

    expect(store.list.map((w) => w.id)).toEqual(['w1', 'w3'])
    expect(store.canManage).toBe(true)
  })

  it('поздний список пространств не выкидывает читателя из нового', async () => {
    const store = await loaded([ws('w1'), ws('w2')])
    // Загрузка списка подвисла…
    const l1 = deferred()
    apiMock.workspaces.list.mockReturnValueOnce(l1.promise)
    const pending = store.loadWorkspaces()

    // …а нас за это время добавили в w3: сокет закрыли, resync увидел новое
    // пространство, читатель его открыл.
    apiMock.workspaces.list.mockResolvedValue({ data: [ws('w1'), ws('w2'), ws('w3')] })
    await store.resync()
    await store.selectWorkspace('w3')
    expect(store.currentId).toBe('w3')

    // Снимок списка, снятый ДО добавления, приходит последним.
    l1.resolve({ data: [ws('w1'), ws('w2')] })
    await pending

    expect(store.currentId).toBe('w3')
    expect(store.projects.map((p) => p.id)).toEqual(['p-w3'])
  })

  it('доски покинутого пространства не всплывают после переключения', async () => {
    const store = await loaded()
    const b = deferred()
    apiMock.projects.boards.mockImplementation(() => b.promise)
    const pending = store.loadBoards('p-w1')

    await store.selectWorkspace('w2')
    expect(store.boardsByProject).toEqual({})

    b.resolve({ data: [row('b-w1', 'w1')] })
    await pending

    expect(store.boardsByProject).toEqual({})
  })

  it('обычный refresh по-прежнему обновляет дерево текущего пространства', async () => {
    const store = await loaded()
    apiMock.workspaces.projects.mockResolvedValue({ data: [row('p-new', 'w1')] })
    await store.refresh()
    expect(store.projects.map((p) => p.id)).toEqual(['p-new'])
  })
})
