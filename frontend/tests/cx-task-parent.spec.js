// Parent row (TaskModal, #2861): the row shows the parent's title next to
// «Открепить», the title opens the parent, and the title comes from the GET the
// modal already makes for sibling order — so mount it for real and count calls.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount } from '@vue/test-utils'
import { seedBoardStore } from './helpers/boardStore'

const PARENT = {
  id: 'p1',
  board_id: 'b1',
  column_id: 'c1',
  title: 'Очень длинный заголовок родительской задачи, который не влезет в строку',
  description: '',
  priority: 0,
  subtasks: [{ id: 't1', title: 'Подзадача', column_id: 'c1', parent_id: 'p1' }],
}
const TASK = {
  id: 't1',
  board_id: 'b1',
  column_id: 'c1',
  parent_id: 'p1',
  title: 'Подзадача',
  description: '',
  priority: 0,
  subtasks: [],
}

const get = vi.fn((id) => Promise.resolve({ data: id === 'p1' ? PARENT : TASK }))
const setParent = vi.fn(() => Promise.resolve({ data: {} }))

vi.mock('@/api', () => ({
  tasks: {
    get: (...a) => get(...a),
    move: vi.fn(() => Promise.resolve({ data: {} })),
    update: vi.fn(() => Promise.resolve({ data: TASK })),
    setParent: (...a) => setParent(...a),
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
    columns: vi.fn(() => Promise.resolve({ data: [] })),
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

// Key every Naive stub by both the import alias and the N-prefixed name — see the
// note in cx-task-status.spec.js; missing one leaves the real popover mounted.
const naive = {
  Icon: { template: '<i class="n-icon-stub"><slot /></i>' },
  Button: { template: '<button class="n-button-stub"><slot /></button>' },
  Input: { inheritAttrs: false, template: '<input class="n-input-stub" />' },
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

// The detail loader hangs off a non-immediate watcher on `show` — mount closed,
// then open, or the modal renders its empty shell forever.
const openModal = async (TaskModal, props = {}) => {
  const w = mount(TaskModal, {
    props: { show: false, taskId: 't1', ...props },
    global: { stubs },
  })
  await w.setProps({ show: true })
  await flush(w)
  return w
}

describe('TaskModal parent row', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    seedBoardStore({ board: null, columns: [] })
    window.matchMedia = vi.fn().mockReturnValue({
      matches: false,
      addEventListener: () => {},
      removeEventListener: () => {},
    })
    get.mockClear()
    setParent.mockClear()
  })

  it('shows the parent title before «Открепить» and opens the parent on click', async () => {
    const TaskModal = (await import('@/components/TaskModal.vue')).default
    const w = await openModal(TaskModal)

    const chip = w.find('.parent-chip')
    expect(chip.exists()).toBe(true)
    expect(chip.text()).toBe(PARENT.title)
    // Full title in the native tooltip — the visible text is ellipsised by CSS.
    expect(chip.attributes('title')).toBe(PARENT.title)
    expect(w.find('.parent-row .detach').text()).toBe('Открепить')

    await chip.trigger('click')
    expect(w.emitted('open')?.[0]).toEqual(['p1'])
    // The title rides along with the sibling-order GET: one request for the parent.
    expect(get.mock.calls.filter((c) => c[0] === 'p1')).toHaveLength(1)
  })

  it('still detaches from the parent', async () => {
    const TaskModal = (await import('@/components/TaskModal.vue')).default
    const w = await openModal(TaskModal)

    await w.find('.parent-row .detach').trigger('click')
    expect(setParent).toHaveBeenCalledWith('t1', null)
  })

  it('hides «Открепить» on a read-only task but keeps the parent clickable', async () => {
    const TaskModal = (await import('@/components/TaskModal.vue')).default
    const w = await openModal(TaskModal, { readonly: true })

    expect(w.find('.parent-row .detach').exists()).toBe(false)
    expect(w.find('.parent-chip').text()).toBe(PARENT.title)
  })
})
