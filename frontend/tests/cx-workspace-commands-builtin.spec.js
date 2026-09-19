// Словарь кастомных команд workspace: сервер считает встроенным не только ключ
// реестра, но и его алиас — quickact.byKey (internal/quickact/registry.go)
// кладёт в карту и Key, и Aliases, а SetWorkspaceCommands зовёт IsBuiltin по
// этой же карте и отбивает ВЕСЬ PUT четырёхсоткой. Модалка же сверялась только
// с .key, так что кастомная «/done» (алиас /close) проходила её проверку молча:
// инлайн-ошибки нет, «Сохранить» активна, а ответ прилетает сырой русской
// строкой в тост — без указания, какая из строк виновата. Ровно то, ради чего
// rowError и написан.
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'
import * as naive from 'naive-ui'

// useMessage() падает без <n-message-provider>; глушим только его, как и
// остальные спеки компонентов.
const toastError = vi.fn()
vi.mock('naive-ui', async () => {
  const actual = await vi.importActual('naive-ui')
  return {
    ...actual,
    useMessage: () => ({
      error: (...a) => toastError(...a),
      success: vi.fn(),
      info: vi.fn(),
      warning: vi.fn(),
    }),
  }
})

const setCommands = vi.fn(() => Promise.resolve({ data: [] }))
const commands = vi.fn()
vi.mock('@/api', () => ({
  workspaces: {
    commands: (...a) => commands(...a),
    setCommands: (...a) => setCommands(...a),
  },
}))

// n-modal телепортирует тело наружу — держим его внутри обёртки.
const stubs = {
  ...naive,
  ...Object.fromEntries(Object.entries(naive).map(([k, v]) => ['N' + k, v])),
  teleport: true,
}

// Кусок настоящего ответа /workspaces/:id/commands: у close есть алиас done,
// у assign — assignee. Форма — как её отдаёт quickact.Registry.
const BUILTIN = [
  { key: 'close', aliases: ['done'], arg: 'none', description: 'Закрыть задачу' },
  { key: 'assign', aliases: ['assignee'], arg: 'user', description: 'Назначить' },
  { key: 'archive', aliases: [], arg: 'none', description: 'Архивировать' },
]

// Реестр приезжает в стор единственным путём — loadCommands по тому же
// эндпоинту, что и в приложении; builtinCommands наружу не выставлен.
const open = async (rows) => {
  const { useWorkspacesStore } = await import('@/stores/workspaces')
  const store = useWorkspacesStore()
  commands.mockResolvedValue({ data: { builtin: BUILTIN, custom: rows, can_manage: true } })
  // Как в приложении: реестр читается только для открытого пространства —
  // ответ для любого другого стор теперь отбрасывает (#2932).
  store.currentId = 'ws-1'
  await store.loadCommands('ws-1')
  const Modal = (await import('@/components/WorkspaceCommandsModal.vue')).default
  const w = mount(Modal, { props: { show: false, workspaceId: 'ws-1' }, global: { stubs } })
  await w.setProps({ show: true })
  await w.vm.$nextTick()
  return w
}

// Инлайн-ошибки модалка рисует в .cmd-err — по одной на проблемную строку.
const inlineErrors = (w) => w.findAll('.cmd-err').map((e) => e.text())
const saveBtn = (w) =>
  w.findAll('button').find((b) => b.text().includes('Сохранить')) ?? w.findAll('button').at(-1)

describe('WorkspaceCommandsModal · алиасы встроенных команд', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    toastError.mockClear()
    setCommands.mockClear()
  })

  it('отбивает ключ, совпавший с алиасом встроенной команды', async () => {
    const w = await open([{ key: 'done', description: 'Готово' }])
    expect(inlineErrors(w)).toEqual(['встроенная команда'])
    expect(saveBtn(w).attributes('disabled')).toBeDefined()
    w.unmount()
  })

  it('не даёт PUT уехать на сервер с таким ключом', async () => {
    const w = await open([{ key: 'assignee', description: 'Исполнитель' }])
    await saveBtn(w).trigger('click')
    await w.vm.$nextTick()
    expect(setCommands).not.toHaveBeenCalled()
    w.unmount()
  })

  it('канонизирует ключ перед сверкой с алиасами', async () => {
    const w = await open([{ key: '/DONE ', description: 'Готово' }])
    expect(inlineErrors(w)).toEqual(['встроенная команда'])
    w.unmount()
  })

  it('показывает, КАКАЯ строка виновата, а не одну ошибку на весь словарь', async () => {
    const w = await open([
      { key: 'approve', description: 'Одобрить' },
      { key: 'done', description: 'Готово' },
      { key: 'ship', description: 'Выкатить' },
    ])
    const rows = w.findAll('.cmd-row')
    expect(rows).toHaveLength(3)
    expect(rows[0].find('.cmd-err').exists()).toBe(false)
    expect(rows[1].find('.cmd-err').text()).toBe('встроенная команда')
    expect(rows[2].find('.cmd-err').exists()).toBe(false)
    w.unmount()
  })

  // Контроли: прежнее поведение не должно поехать.
  it('по-прежнему отбивает сам ключ реестра', async () => {
    const w = await open([{ key: 'close', description: 'Закрыть' }])
    expect(inlineErrors(w)).toEqual(['встроенная команда'])
    w.unmount()
  })

  it('пропускает свободный ключ и шлёт его канонизированным', async () => {
    const w = await open([{ key: '/Approve', description: '  Одобрить  ' }])
    expect(inlineErrors(w)).toEqual([])
    await saveBtn(w).trigger('click')
    await w.vm.$nextTick()
    expect(setCommands).toHaveBeenCalledWith('ws-1', [{ key: 'approve', description: 'Одобрить' }])
    w.unmount()
  })

  it('не путает алиас с похожим свободным ключом', async () => {
    const w = await open([
      { key: 'done_review', description: 'Ревью' },
      { key: 'undone', description: 'Не алиас в этой фикстуре' },
    ])
    expect(inlineErrors(w)).toEqual([])
    w.unmount()
  })
})
