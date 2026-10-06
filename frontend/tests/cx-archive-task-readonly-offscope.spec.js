// Архивная задача, открытая МИМО архивного скоупа, обязана быть read-only (#2932).
//
// Модалка выводила readonly из пропа, а проп на доске — из `?archived=1`
// (`archivedMode`). Это ответ про ЭКРАН, а не про задачу, и половина дверей в
// модалку идёт мимо экрана: связь (`openRelated` пушит `?task=` без
// `archived=1`), уведомление (ListNotifications архив не фильтрует), ссылка из
// документа (ListDocumentTaskLinks тоже), чип `#N` в описании, «копировать
// ссылку». Гварда на бэкенде нет — архив клиентская конвенция, — так что
// архивная задача, открытая любой из них, правилась молча и успешно.
//
// Здесь проверяется, что read-only теперь = скоуп ИЛИ собственный `archived_at`
// задачи, и что вторая половина не съела первую: живая задача в архивном скоупе
// остаётся запертой, живая на живой доске — нет.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount } from '@vue/test-utils'
import { seedBoardStore } from './helpers/boardStore'

const COLS = [
  { id: 'c1', name: 'К работе', color: '#9aa0aa', position: 1 },
  { id: 'c2', name: 'В процессе', color: '#2f80ed', position: 2 },
  { id: 'c3', name: 'Готово', color: '#18a058', position: 3 },
]
const BASE = {
  id: 't1',
  board_id: 'b1',
  column_id: 'c1',
  title: 'Задача',
  description: '',
  priority: 0,
  parent_id: 'p9',
  subtasks: [],
  archived_at: null,
}

// Что отдаст GET /tasks/:id в текущем кейсе — правится внутри теста до открытия.
let detail = BASE
const get = vi.fn(() => Promise.resolve({ data: detail }))
const update = vi.fn(() => Promise.resolve({ data: detail }))
const move = vi.fn(() => Promise.resolve({ data: {} }))

vi.mock('@/api', () => ({
  tasks: {
    get: (...a) => get(...a),
    update: (...a) => update(...a),
    move: (...a) => move(...a),
    comments: vi.fn(() => Promise.resolve({ data: [] })),
    relations: vi.fn(() => Promise.resolve({ data: [] })),
    attachments: vi.fn(() => Promise.resolve({ data: [] })),
    events: vi.fn(() => Promise.resolve({ data: [] })),
    documents: vi.fn(() => Promise.resolve({ data: [] })),
  },
  boards: {
    get: vi.fn(() =>
      Promise.resolve({ data: { name: 'Доска', project_id: 'p1', done_column_id: 'c3' } }),
    ),
    columns: vi.fn(() => Promise.resolve({ data: COLS })),
    tasks: vi.fn(() => Promise.resolve({ data: [] })),
    createTask: vi.fn(),
  },
  workspaces: { members: vi.fn(() => Promise.resolve({ data: [] })) },
  projects: { createMilestone: vi.fn() },
  gitlab: { issueTemplates: vi.fn(() => Promise.resolve({ data: [] })) },
  columns: { update: vi.fn(), remove: vi.fn() },
  users: { updatePrefs: vi.fn() },
}))

vi.mock('naive-ui', async (importOriginal) => {
  const actual = await importOriginal()
  return {
    ...actual,
    useMessage: () => ({ error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }),
  }
})

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
  useRoute: () => ({ path: '/', params: {}, query: {} }),
}))

// Стабы Naive — по ОБОИМ именам (импортный алиас и собственное `name`): TaskModal
// на `<script setup>`, локальной регистрации нет, и ключ только по алиасу молча
// оставляет живой компонент (см. cx-task-status.spec.js).
const naive = {
  Icon: { template: '<i class="n-icon-stub"><slot /></i>' },
  Button: { template: '<button class="n-button-stub"><slot /></button>' },
  // props: ['readonly'] — иначе `:readonly="ro"` доедет сюда атрибутом, а
  // inheritAttrs:false его выбросит, и поле «заголовок» в снимке всегда читалось
  // бы как редактируемое.
  Input: {
    inheritAttrs: false,
    props: ['readonly'],
    template: '<input class="n-input-stub" :readonly="readonly || null" />',
  },
  Modal: { template: '<div class="n-modal-stub"><slot /></div>' },
  // Оба слота: подвал модалки (read-only-заметка и «Сохранить») живёт в #footer
  // n-card, и стаб с одним дефолтным слотом молча выбрасывает ровно то, что здесь
  // проверяется.
  Card: { template: '<div class="n-card-stub"><slot /><slot name="footer" /></div>' },
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
const stubs = {
  ...naive,
  ...Object.fromEntries(Object.entries(naive).map(([k, v]) => ['N' + k, v])),
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

// Загрузка висит на НЕ-immediate вотчере `show`: монтируем закрытой и открываем,
// иначе модалка навсегда останется пустой оболочкой.
const openModal = async (readonly) => {
  const TaskModal = (await import('@/components/TaskModal.vue')).default
  const w = mount(TaskModal, {
    props: { show: false, taskId: 't1', readonly },
    global: { stubs },
  })
  await w.setProps({ show: true })
  await flush(w)
  return w
}

// Единый снимок «что модалка предлагает сделать».
const affordances = (w) => ({
  // v2 (#2958): the read-only note is the banner over the body, and there is no
  // «Сохранить» at all — fields save as they change, so the guard is per field.
  roNote: w.find('.tm-ro-banner').exists(),
  statusBtns: w.findAll('.st-btn').length,
  colPicker: w.find('.col-chip.static').exists() ? 'static' : 'picker',
  detachParent: w.find('.val.detach').exists(),
  // Заголовок и прочие текстовые поля: `:readonly="ro"` доходит только до них,
  // так что один запертый input = запертое имя задачи.
  // v2: the title is a <textarea> (#2958), the other text fields inputs.
  lockedInputs: w.findAll('input[readonly], textarea[readonly]').length > 0,
})

describe('TaskModal: read-only идёт от задачи, а не только от экрана', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    seedBoardStore({ board: { id: 'b1', name: 'Доска', done_column_id: 'c3' }, columns: COLS })
    window.matchMedia = vi.fn().mockReturnValue({
      matches: false,
      addEventListener: () => {},
      removeEventListener: () => {},
    })
    detail = BASE
    get.mockClear()
    update.mockClear()
    move.mockClear()
  })

  it('архивная задача с ЖИВОЙ доски заперта и предлагает восстановление', async () => {
    // Ровно то, что делает клик по связи/уведомлению: `?task=<id>` без
    // `archived=1`, значит проп readonly = false.
    detail = { ...BASE, archived_at: '2026-09-01T10:00:00Z' }
    const w = await openModal(false)

    expect(affordances(w)).toEqual({
      roNote: true,
      statusBtns: 0,
      colPicker: 'static',
      detachParent: false,
      lockedInputs: true,
    })
    // Подвал read-only несёт «Восстановить» — единственный осмысленный выход из
    // архивной задачи, открытой с живой доски.
    expect(w.text()).toContain('Вернуть из архива')
    expect(w.text()).toContain('Архивная задача — только просмотр')
    w.unmount()
  })

  it('архивная задача не уходит в API даже мимо кнопок', async () => {
    detail = { ...BASE, archived_at: '2026-09-01T10:00:00Z' }
    const w = await openModal(false)

    // Второй слой, как на android (`mutate`): кнопок не нарисовано, но если до
    // обработчика всё же дошли — тапаемые поля шлют PATCH немедленно, не дожидаясь
    // «Сохранить», — он обязан развернуться.
    w.vm.setPriority(3)
    w.vm.setEstimate(5)
    await w.vm.moveToColumn('c2')
    await flush(w)

    expect(update).not.toHaveBeenCalled()
    expect(move).not.toHaveBeenCalled()
    w.unmount()
  })

  it('живая задача в архивном скоупе остаётся запертой', async () => {
    // Контроль: вторая половина условия не должна была съесть первую — архивная
    // ОБЛАСТЬ доски запирает и неархивную задачу (её показывает та же вкладка,
    // когда карточку восстановили в соседней сессии).
    const w = await openModal(true)

    expect(affordances(w).roNote).toBe(true)
    w.unmount()
  })

  it('живая задача с живой доски по-прежнему правится', async () => {
    // Контроль: гвард обязан ловить ровно архив и ничего больше.
    const w = await openModal(false)

    expect(affordances(w)).toEqual({
      roNote: false,
      statusBtns: 2,
      colPicker: 'picker',
      detachParent: true,
      lockedInputs: false,
    })

    w.vm.setPriority(3)
    await flush(w)
    expect(update).toHaveBeenCalled()
    w.unmount()
  })
})
