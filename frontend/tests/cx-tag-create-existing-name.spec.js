// Набрать в пикере имя тега, который у проекта УЖЕ есть (#2932).
//
// В пикере тегов (v2, #3113 — модалка и карточка на доске) сверху поле поиска:
// оно фильтрует теги проекта, ↵ отмечает подсвеченный, «Создать тег «…»» (⌘↵)
// появляется, когда так не называется ни один тег. Набрать имя существующего
// тега — обычный способ до него добраться, а не промах: теги проект копит, плюс
// всё, что принёс импорт из GitLab.
//
// Дороги дальше не было: Enter уходил создавать, упирался в UNIQUE(project_id,
// name) и возвращался отказом «тег с таким именем уже есть в проекте» (409 — до
// того вообще «внутренняя ошибка»). Тег на задачу при этом не попадал: читателю
// оставалось искать его в сетке глазами.
//
// Здесь пинится разбор набранного имени ДО запроса: совпало — цепляем существующий
// тег (регистр не в счёт: второй тег, отличающийся только им, разводит по разным
// группам и делит пополам теговые дорожки доски — тот же урок, что у tagInPrefix),
// уже на задаче — молча ничего не шлём. Рядом — обычный путь (новое имя всё так же
// создаётся; гвард, съедающий ЛЮБОЙ ввод, прошёл бы все проверки выше) и второй
// Enter по ещё летящему созданию: поле очищается только по факту, так что нетерпение
// слало второе создание — и оно возвращалось отказом про тег, который первое только
// что завело.
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount } from '@vue/test-utils'
import { h, nextTick, ref } from 'vue'
import * as naive from 'naive-ui'
import { seedBoardStore } from './helpers/boardStore'
import { matchTagByName } from '@/utils/tagGroups'

const createTag = vi.fn(() => Promise.resolve({ data: { id: 'new', name: 'новый' } }))
const addTag = vi.fn(() => Promise.resolve({ data: {} }))
const removeTag = vi.fn(() => Promise.resolve({ data: {} }))

vi.mock('@/api', () => ({
  tasks: {
    get: vi.fn(() => Promise.resolve({ data: {} })),
    addTag: (...a) => addTag(...a),
    removeTag: (...a) => removeTag(...a),
    addAssignee: vi.fn(),
    removeAssignee: vi.fn(),
    update: vi.fn(),
    move: vi.fn(),
    dueNotify: vi.fn(),
    comments: vi.fn(() => Promise.resolve({ data: [] })),
    relations: vi.fn(() => Promise.resolve({ data: [] })),
    attachments: vi.fn(() => Promise.resolve({ data: [] })),
    events: vi.fn(() => Promise.resolve({ data: [] })),
    documents: vi.fn(() => Promise.resolve({ data: [] })),
  },
  projects: {
    createTag: (...a) => createTag(...a),
    createMilestone: vi.fn(),
    tags: vi.fn(() => Promise.resolve({ data: [] })),
    tagPrefixes: vi.fn(() => Promise.resolve({ data: [] })),
  },
  boards: {
    get: vi.fn(() => Promise.resolve({ data: { name: 'Доска', project_id: 'p1' } })),
    columns: vi.fn(() => Promise.resolve({ data: [{ id: 'c1', name: 'К работе', position: 1 }] })),
    tasks: vi.fn(() => Promise.resolve({ data: [] })),
    createTask: vi.fn(),
  },
  workspaces: { members: vi.fn(() => Promise.resolve({ data: [] })) },
  gitlab: {
    issueTemplates: vi.fn(() => Promise.resolve({ data: [] })),
    listIntegrations: vi.fn(() => Promise.resolve({ data: { integrations: [] } })),
  },
  columns: { update: vi.fn(), remove: vi.fn() },
  users: { updatePrefs: vi.fn() },
}))

// Заголовок модалки несёт HelpHint — он выбирает статью по текущему пути.
vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
  useRoute: () => ({ path: '/', params: {}, query: {} }),
}))

const TaskCardPills = (await import('@/components/task/TaskCardPills.vue')).default

// Теги проекта: два обычных и один из GitLab-пространства, которым управляет
// правило «status» — такие пикер прячет намеренно.
const BUG = { id: 'tg-bug', name: 'баг', color: '#e0533d' }
const FRONT = { id: 'tg-front', name: 'Фронт', color: '#2f80ed' }
const META = { id: 'tg-meta', name: 'status::done', color: '' }
const TAGS = [BUG, FRONT, META]

const TASK = { id: 't1', board_id: 'b1', column_id: 'c1', title: 'Задача', number: 7, tag_ids: [] }

let wrapper = null
const shown = ref(false)

// Заглушки Naive для модалки: она на <script setup>, поэтому теги резолвятся к
// собственному имени компонента («Popover», не «NPopover») — ключим оба.
const naiveStubs = {
  Icon: { template: '<i class="n-icon-stub"><slot /></i>' },
  Button: { template: '<button class="n-button-stub"><slot /></button>' },
  // Поле ввода настоящее по поведению (v-model + Enter), но без атрибутов Naive:
  // size="tiny" на голом <input> jsdom не принимает.
  Input: {
    props: ['value'],
    emits: ['update:value'],
    inheritAttrs: false,
    template:
      '<input class="n-input-stub" :value="value" :placeholder="$attrs.placeholder" @input="$emit(\'update:value\', $event.target.value)" @keyup="$attrs.onKeyup?.($event)" />',
  },
  Modal: { template: '<div class="n-modal-stub"><slot /></div>' },
  Card: { template: '<div class="n-card-stub"><slot /></div>' },
  Spin: { template: '<div class="n-spin-stub"><slot /></div>' },
  Tabs: { template: '<div class="n-tabs-stub"><slot /></div>' },
  TabPane: { template: '<div class="n-tab-pane-stub"><slot /></div>' },
  Select: { template: '<div class="n-select-stub" />' },
  Badge: { template: '<span class="n-badge-stub" />' },
  Space: { template: '<div class="n-space-stub"><slot /></div>' },
  Popover: { template: '<div class="n-popover-stub"><slot name="trigger" /><slot /></div>' },
  Popconfirm: { template: '<div class="n-popconfirm-stub"><slot name="trigger" /><slot /></div>' },
  Dropdown: { template: '<div class="n-dropdown-stub" />' },
  Tooltip: { template: '<div class="n-tooltip-stub"><slot name="trigger" /><slot /></div>' },
}
const modalStubs = {
  ...naiveStubs,
  ...Object.fromEntries(Object.entries(naiveStubs).map(([k, v]) => ['N' + k, v])),
  MarkdownEditor: true,
  RichContent: true,
  DueEditor: true,
  draggable: {
    template: '<div><slot name="item" v-for="e in list" :element="e" :index="0" /></div>',
    props: ['list'],
  },
}

const flush = async (w) => {
  for (let i = 0; i < 8; i++) await w.vm.$nextTick()
  await new Promise((r) => setTimeout(r, 0))
  for (let i = 0; i < 8; i++) await w.vm.$nextTick()
}

// Модалка задачи: грузит деталь по невзводящему watcher'у на `show`, поэтому
// монтируем закрытой и открываем (так её держит доска).
async function openModalPicker(task = TASK) {
  seedBoardStore({
    board: { id: 'b1', name: 'Доска', project_id: 'p1' },
    columns: [{ id: 'c1', name: 'К работе', position: 1 }],
    tags: TAGS,
    metaTagPrefixes: ['status::'],
  })
  const { tasks } = await import('@/api')
  tasks.get.mockResolvedValue({ data: task })
  window.matchMedia = vi.fn().mockReturnValue({
    matches: false,
    addEventListener: () => {},
    removeEventListener: () => {},
  })
  const TaskModal = (await import('@/components/TaskModal.vue')).default
  wrapper = mount(
    {
      render: () =>
        h(naive.NMessageProvider, null, {
          default: () => h(TaskModal, { show: shown.value, taskId: task.id }),
        }),
    },
    { global: { stubs: modalStubs } },
  )
  shown.value = true
  await flush(wrapper)
  await wrapper.find('[data-testid="task-tags-open"]').trigger('click')
  await flush(wrapper)
  return document.querySelector('[data-testid="tag-picker-search"]')
}

// Ввод + Enter в поле поиска: пикер уезжает в teleport, поэтому идём через DOM.
async function typeEnter(input, text, mod = false) {
  input.value = text
  input.dispatchEvent(new Event('input', { bubbles: true }))
  await nextTick()
  input.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', ctrlKey: mod, bubbles: true }))
  await nextTick()
  await nextTick()
}

// Карточка на доске: открыть теговый пикер и вернуть его поле ввода.
async function openPillsPicker(task = TASK) {
  seedBoardStore({
    columns: [{ id: 'c1', name: 'К работе', position: 1 }],
    tags: TAGS,
    metaTagPrefixes: ['status::'],
  })
  wrapper = mount(
    {
      render: () => h(naive.NMessageProvider, null, { default: () => h(TaskCardPills, { task }) }),
    },
    { attachTo: document.body },
  )
  await nextTick()
  await wrapper.find('[data-tour="card-tags"]').trigger('click')
  await nextTick()
  await nextTick()
  return document.querySelector('[data-testid="tag-picker-search"]')
}

beforeEach(() => {
  localStorage.clear()
  setActivePinia(createPinia())
  vi.clearAllMocks()
})

afterEach(() => {
  // Оставленный монтаж держит свои наблюдатели на общих сторах и травит планировщик
  // следующему спеку (пилюли смотрят стор доски глубоко).
  wrapper?.unmount()
  wrapper = null
  document.body.innerHTML = ''
})

describe('matchTagByName', () => {
  it('находит тег по точному имени', () => {
    expect(matchTagByName(TAGS, 'баг')).toBe(BUG)
  })

  it('не считается с регистром и лишними пробелами', () => {
    expect(matchTagByName(TAGS, '  фронт ')).toBe(FRONT)
    expect(matchTagByName(TAGS, 'БАГ')).toBe(BUG)
  })

  it('предпочитает точное написание близнецу по регистру', () => {
    const lower = { id: 'x', name: 'фронт' }
    expect(matchTagByName([lower, FRONT], 'Фронт')).toBe(FRONT)
    expect(matchTagByName([FRONT, lower], 'фронт')).toBe(lower)
  })

  it('на незанятом имени и на пустом вводе молчит', () => {
    expect(matchTagByName(TAGS, 'совсем новый')).toBeNull()
    expect(matchTagByName(TAGS, '   ')).toBeNull()
    expect(matchTagByName(null, 'баг')).toBeNull()
  })
})

describe('пикер тегов карточки · набрано занятое имя', () => {
  it('цепляет существующий тег вместо создания', async () => {
    const input = await openPillsPicker()
    await typeEnter(input, 'баг')
    expect(createTag).not.toHaveBeenCalled()
    expect(addTag).toHaveBeenCalledWith('t1', 'tg-bug')
  })

  it('узнаёт имя, набранное в другом регистре', async () => {
    const input = await openPillsPicker()
    await typeEnter(input, 'ФРОНТ')
    expect(createTag).not.toHaveBeenCalled()
    expect(addTag).toHaveBeenCalledWith('t1', 'tg-front')
  })

  it('на теге, который уже на задаче, не шлёт ничего и очищает поле', async () => {
    const input = await openPillsPicker({ ...TASK, tag_ids: ['tg-bug'] })
    await typeEnter(input, 'баг')
    expect(createTag).not.toHaveBeenCalled()
    expect(addTag).not.toHaveBeenCalled()
    // ↵ по набранному имени никогда не СНИМАЕТ тег.
    expect(removeTag).not.toHaveBeenCalled()
    expect(input.value).toBe('')
  })

  it('⌘↵ по занятому имени — тот же тег, а не близнец', async () => {
    const input = await openPillsPicker()
    await typeEnter(input, 'баг', true)
    expect(createTag).not.toHaveBeenCalled()
    expect(addTag).toHaveBeenCalledWith('t1', 'tg-bug')
  })

  it('незанятое имя по-прежнему создаёт тег и цепляет его', async () => {
    const input = await openPillsPicker()
    await typeEnter(input, 'совсем новый')
    expect(createTag).toHaveBeenCalledTimes(1)
    expect(createTag.mock.calls[0][1].name).toBe('совсем новый')
    expect(addTag).toHaveBeenCalledWith('t1', 'new')
  })

  it('второй Enter по летящему созданию не шлёт второй запрос', async () => {
    let release
    createTag.mockImplementationOnce(
      () => new Promise((res) => (release = () => res({ data: { id: 'new', name: 'новый' } }))),
    )
    const input = await openPillsPicker()
    await typeEnter(input, 'совсем новый')
    await typeEnter(input, 'совсем новый')
    expect(createTag).toHaveBeenCalledTimes(1)
    release()
    await nextTick()
    await nextTick()
    expect(createTag).toHaveBeenCalledTimes(1)
  })

  it('тег, спрятанный правилом GitLab, молча не цепляется', async () => {
    // Такие теги ведут поле задачи, ручная привязка их расcинхронизирует — пикер их
    // не показывает, значит и набранное имя не должен разрешать в них за спиной.
    const input = await openPillsPicker()
    await typeEnter(input, 'status::done')
    expect(addTag).not.toHaveBeenCalledWith('t1', 'tg-meta')
    expect(createTag).toHaveBeenCalledTimes(1)
  })
})

describe('пикер тегов модалки задачи · набрано занятое имя', () => {
  beforeEach(() => {
    shown.value = false
  })

  it('цепляет существующий тег вместо создания', async () => {
    const input = await openModalPicker()
    await typeEnter(input, 'баг')
    expect(createTag).not.toHaveBeenCalled()
    expect(addTag).toHaveBeenCalledWith('t1', 'tg-bug')
  })

  it('незанятое имя по-прежнему создаёт тег и цепляет его', async () => {
    const input = await openModalPicker()
    await typeEnter(input, 'совсем новый')
    expect(createTag).toHaveBeenCalledTimes(1)
    expect(createTag.mock.calls[0][1].name).toBe('совсем новый')
    expect(addTag).toHaveBeenCalledWith('t1', 'new')
  })

  it('второй Enter по летящему созданию не шлёт второй запрос', async () => {
    let release
    createTag.mockImplementationOnce(
      () => new Promise((res) => (release = () => res({ data: { id: 'new', name: 'новый' } }))),
    )
    const input = await openModalPicker()
    await typeEnter(input, 'совсем новый')
    await typeEnter(input, 'совсем новый')
    expect(createTag).toHaveBeenCalledTimes(1)
    release()
    await flush(wrapper)
    expect(createTag).toHaveBeenCalledTimes(1)
  })
})

// Таблица тегов окна «Проект» (#2990) — то же поле «Новый тег» и та же
// нетерпеливость: второе создание по второму Enter гасится ровно так же.
describe('таблица тегов проекта · двойной Enter', () => {
  it('не шлёт второе создание, пока первое в полёте', async () => {
    let release
    createTag.mockImplementationOnce(
      () => new Promise((res) => (release = () => res({ data: { id: 'new' } }))),
    )
    const ProjectTagsTab = (await import('@/components/project/ProjectTagsTab.vue')).default
    wrapper = mount(
      {
        render: () =>
          h(naive.NMessageProvider, null, {
            default: () => h(ProjectTagsTab, { project: { id: 'p1', slug: 'p' } }),
          }),
      },
      { attachTo: document.body },
    )
    await nextTick()
    const input = wrapper.find('[data-testid="tag-new-input"]').element
    const enter = async (text) => {
      input.value = text
      input.dispatchEvent(new Event('input', { bubbles: true }))
      await nextTick()
      input.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))
      await nextTick()
    }
    await enter('совсем новый')
    await enter('совсем новый')
    expect(createTag).toHaveBeenCalledTimes(1)
    release()
    await nextTick()
    await nextTick()
    expect(createTag).toHaveBeenCalledTimes(1)
  })
})
