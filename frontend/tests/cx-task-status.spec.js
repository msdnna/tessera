// Status row (TaskModal) + subtask divergence marker (TaskCard): these render
// paths carry the move calls, so mount them for real instead of trusting the
// pure helpers in ut-status.spec.js alone.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount } from '@vue/test-utils'

const COLS = [
  { id: 'c1', name: 'К работе', color: '#9aa0aa', position: 1 },
  { id: 'c2', name: 'В процессе', color: '#2f80ed', position: 2 },
  { id: 'c3', name: 'Готово', color: '#18a058', position: 3 },
]
const TASK = {
  id: 't1',
  board_id: 'b1',
  column_id: 'c1',
  title: 'Родитель',
  description: '',
  priority: 0,
  subtasks: [
    { id: 's1', title: 'Ушла вперёд', column_id: 'c2', parent_id: 't1' },
    { id: 's2', title: 'Как у родителя', column_id: 'c1', parent_id: 't1' },
  ],
}

const move = vi.fn(() => Promise.resolve({ data: {} }))
const update = vi.fn(() => Promise.resolve({ data: TASK }))

vi.mock('@/api', () => ({
  tasks: {
    get: vi.fn(() => Promise.resolve({ data: TASK })),
    move: (...a) => move(...a),
    update: (...a) => update(...a),
    comments: vi.fn(() => Promise.resolve({ data: [] })),
    relations: vi.fn(() => Promise.resolve({ data: [] })),
    attachments: vi.fn(() => Promise.resolve({ data: [] })),
    events: vi.fn(() => Promise.resolve({ data: [] })),
  },
  boards: {
    get: vi.fn(() => Promise.resolve({ data: { name: 'Доска', project_id: 'p1', done_column_id: 'c3' } })),
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

vi.mock('vue-router', () => ({ useRouter: () => ({ push: vi.fn(), replace: vi.fn() }) }))

const stubs = {
  NIcon: { template: '<i class="n-icon-stub"><slot /></i>' },
  NButton: { template: '<button class="n-button-stub"><slot /></button>' },
  NInput: { template: '<input class="n-input-stub" />' },
  NModal: { template: '<div class="n-modal-stub"><slot /></div>' },
  NCard: { template: '<div class="n-card-stub"><slot /></div>' },
  NSpin: { template: '<div class="n-spin-stub"><slot /></div>' },
  NTabs: { template: '<div class="n-tabs-stub"><slot /></div>' },
  NTabPane: { template: '<div class="n-tab-pane-stub"><slot /></div>' },
  NSelect: { template: '<div class="n-select-stub" />' },
  NBadge: { template: '<span class="n-badge-stub" />' },
  NSpace: { template: '<div class="n-space-stub"><slot /></div>' },
  NPopover: { template: '<div class="n-popover-stub"><slot name="trigger" /><slot /></div>' },
  NPopconfirm: { template: '<div class="n-popconfirm-stub"><slot name="trigger" /><slot /></div>' },
  NDropdown: { template: '<div class="n-dropdown-stub" />' },
  NTooltip: { template: '<div class="n-tooltip-stub"><slot name="trigger" /><slot /></div>' },
  MarkdownEditor: true,
  RichContent: true,
  DueEditor: true,
  draggable: { template: '<div><slot name="item" v-for="e in list" :element="e" :index="0" /></div>', props: ['list'] },
}

const flush = async (w) => {
  for (let i = 0; i < 8; i++) await w.vm.$nextTick()
  await new Promise((r) => setTimeout(r, 0))
  for (let i = 0; i < 8; i++) await w.vm.$nextTick()
}

describe('TaskModal status row', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    move.mockClear()
    update.mockClear()
  })

  it('shows the task column and both actions, and moves right on shift', async () => {
    const TaskModal = (await import('@/components/TaskModal.vue')).default
    const w = mount(TaskModal, {
      props: { show: true, taskId: 't1', wsId: 'w1', projectId: 'p1' },
      global: { stubs },
    })
    await flush(w)

    expect(w.text()).toContain('Статус')
    expect(w.find('.col-chip').text()).toContain('К работе')

    const [shift, close] = w.findAll('.st-btn')
    expect(shift.attributes('disabled')).toBeUndefined()
    await shift.trigger('click')
    expect(move).toHaveBeenCalledWith('t1', {
      column_id: 'c2',
      before_id: null,
      after_id: null,
    })

    // The checkmark closes through the board's done column, not the raw flag.
    move.mockClear()
    await close.trigger('click')
    expect(move).toHaveBeenCalledWith('t1', expect.objectContaining({ column_id: 'c3' }))
    expect(update).not.toHaveBeenCalled()
  })

  it('moves a subtask from its row, pinned between its siblings', async () => {
    const TaskModal = (await import('@/components/TaskModal.vue')).default
    const w = mount(TaskModal, {
      props: { show: true, taskId: 't1', wsId: 'w1', projectId: 'p1' },
      global: { stubs },
    })
    await flush(w)

    const rows = w.findAll('.subrow')
    expect(rows.length).toBe(2)
    // Each row carries its own column chip; the first subtask sits in «В процессе».
    expect(rows[0].find('.col-chip.mini').text()).toContain('В процессе')

    const items = w.findAll('.subrow .col-item')
    await items[items.length - 1].trigger('click') // → «Готово»
    expect(move).toHaveBeenCalledWith('s1', {
      column_id: 'c3',
      before_id: null,
      after_id: 's2',
    })
  })
})

describe('TaskCard subtask column marker', () => {
  beforeEach(() => setActivePinia(createPinia()))

  it('marks only the child that diverged from its parent', async () => {
    const TaskCard = (await import('@/components/TaskCard.vue')).default
    const w = mount(TaskCard, {
      props: { task: TASK, subtasks: TASK.subtasks, subtasksTotal: 2, columns: COLS },
      global: { stubs },
    })
    await w.vm.$nextTick()

    const marks = w.findAll('.col-mark')
    expect(marks.length).toBe(1)
    expect(marks[0].attributes('title')).toBe('Колонка: В процессе')
  })
})
