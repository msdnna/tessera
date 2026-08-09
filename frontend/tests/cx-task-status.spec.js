// Status row (TaskModal) + subtask divergence marker (TaskCard): these render
// paths carry the move calls, so mount them for real instead of trusting the
// pure helpers in ut-status.spec.js alone.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount } from '@vue/test-utils'
import { seedBoardStore } from './helpers/boardStore'

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

vi.mock('vue-router', () => ({ useRouter: () => ({ push: vi.fn(), replace: vi.fn() }) }))

// Naive stubs. TaskModal is <script setup>, so its `<n-popover>` resolves straight
// to the imported binding — there is no local registration for VTU to match, and it
// falls back to the component's own `name` ("Popover", not "NPopover"). Keying only
// by the import alias silently leaves the real Naive components mounted, and n-modal
// then teleports the body out of the wrapper while n-popover's follower throws.
// So key every stub by BOTH names.
const naive = {
  Icon: { template: '<i class="n-icon-stub"><slot /></i>' },
  Button: { template: '<button class="n-button-stub"><slot /></button>' },
  // inheritAttrs:false — Naive passes size="small"/"tiny", which a bare <input>
  // rejects and jsdom reports as a Vue warning on every mount.
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

// The detail loader hangs off a non-immediate watcher on `show`, so mounting with
// show:true loads nothing — KanbanBoard keeps the modal mounted and flips the prop.
// Mount closed and open it, or the modal renders its empty shell forever.
const openModal = async (TaskModal) => {
  const w = mount(TaskModal, {
    props: { show: false, taskId: 't1' },
    global: { stubs },
  })
  await w.setProps({ show: true })
  await flush(w)
  return w
}

describe('TaskModal status row', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    // Workspace/project context comes from the store now. Board + columns stay
    // empty on purpose: that is the cross-board / deep-link open, where the modal
    // falls back to fetching the board itself (what these cases exercise).
    seedBoardStore({ board: null, columns: [] })
    // TaskModal uses useResponsive (matchMedia) for its wide/stacked layout; jsdom
    // has none — stub a desktop-width match so the modal mounts.
    window.matchMedia = vi.fn().mockReturnValue({
      matches: false,
      addEventListener: () => {},
      removeEventListener: () => {},
    })
    move.mockClear()
    update.mockClear()
  })

  it('shows the task column and both actions, and moves right on shift', async () => {
    const TaskModal = (await import('@/components/TaskModal.vue')).default
    const w = await openModal(TaskModal)

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
    // Wait out the first move: the row disables itself while `moving` is set, and
    // a click on a disabled button never reaches the handler.
    await flush(w)
    move.mockClear()
    await close.trigger('click')
    await flush(w)
    expect(move).toHaveBeenCalledWith('t1', expect.objectContaining({ column_id: 'c3' }))
    expect(update).not.toHaveBeenCalled()
  })

  it('moves a subtask from its row, pinned between its siblings', async () => {
    const TaskModal = (await import('@/components/TaskModal.vue')).default
    const w = await openModal(TaskModal)

    const rows = w.findAll('.subrow')
    expect(rows.length).toBe(2)
    // Each row carries its own column chip; the first subtask sits in «В процессе».
    expect(rows[0].find('.col-chip.mini').text()).toContain('В процессе')

    // Scope to the first row — every subrow renders its own column menu.
    const items = rows[0].findAll('.col-item')
    await items[items.length - 1].trigger('click') // → «Готово»
    await flush(w)
    expect(move).toHaveBeenCalledWith('s1', {
      column_id: 'c3',
      before_id: null,
      after_id: 's2',
    })
  })
})

describe('TaskCard subtask column marker', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    // The card reads the board's status columns from the store, not from props.
    seedBoardStore({ columns: COLS })
  })

  it('marks only the child that diverged from its parent', async () => {
    const TaskCard = (await import('@/components/TaskCard.vue')).default
    const w = mount(TaskCard, {
      props: { task: TASK, subtasks: TASK.subtasks, subtasksTotal: 2 },
      global: { stubs },
    })
    await w.vm.$nextTick()

    const marks = w.findAll('.col-mark')
    expect(marks.length).toBe(1)
    expect(marks[0].attributes('title')).toBe('Колонка: В процессе')
  })
})

// The card takes tags, assignees, milestones and the customize-view display
// settings from the board store instead of props (#2664). Mount it with nothing
// but the task itself and check the context still reaches the pills — otherwise a
// missed binding degrades silently into an empty pill rather than an error.
describe('TaskCard board context from the store', () => {
  const TAGGED = {
    id: 't2',
    board_id: 'b1',
    column_id: 'c1',
    title: 'С контекстом',
    number: 42,
    priority: 3,
    tag_ids: ['tag1'],
    assignee_ids: ['u1'],
    milestone_id: 'm1',
  }
  const seed = (over) =>
    seedBoardStore({
      columns: COLS,
      tags: [{ id: 'tag1', name: 'backend', color: '#7c5cff' }],
      members: [{ user_id: 'u1', name: 'Аня' }],
      milestones: [{ id: 'm1', title: 'Спринт 1' }],
      // 'large' allows every field; the medium preset deliberately omits the
      // milestone pill, which would make the assertions below test the preset.
      cardSize: 'large',
      ...over,
    })
  const mountCard = async () => {
    const TaskCard = (await import('@/components/TaskCard.vue')).default
    const w = mount(TaskCard, { props: { task: TAGGED }, global: { stubs } })
    await w.vm.$nextTick()
    return w
  }

  beforeEach(() => setActivePinia(createPinia()))

  it('resolves tag, assignee and milestone through the store maps', async () => {
    seed()
    const w = await mountCard()
    expect(w.text()).toContain('backend')
    expect(w.text()).toContain('Спринт 1')
    expect(w.html()).toContain('Аня')
  })

  it('honours the store fieldVis toggle', async () => {
    seed({ fieldVis: { milestone: false } })
    const w = await mountCard()
    expect(w.text()).toContain('backend')
    expect(w.text()).not.toContain('Спринт 1')
  })

  it('honours the store cardSize preset (compact drops the pills)', async () => {
    seed({ cardSize: 'compact' })
    const w = await mountCard()
    expect(w.text()).toContain('С контекстом')
    expect(w.text()).not.toContain('backend')
  })
})
