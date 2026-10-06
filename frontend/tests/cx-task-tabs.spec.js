import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount, flushPromises } from '@vue/test-utils'
import { seedBoardStore } from './helpers/boardStore'

// Characterization tests for the tabs extracted out of TaskModal (#2661). The
// modal still owns each list (the tab labels carry the counts), so what matters
// here is the contract: what the tab renders, and what it publishes back when it
// mutates something.

const api = {
  tasks: {
    relations: vi.fn(),
    addRelation: vi.fn(),
    removeRelation: vi.fn(),
    attachments: vi.fn(),
    uploadAttachment: vi.fn(),
    removeAttachment: vi.fn(),
    move: vi.fn(),
    update: vi.fn(),
  },
  boards: { createTask: vi.fn() },
  workspaces: { tasks: vi.fn() },
  gitlab: { pushChild: vi.fn() },
}
vi.mock('@/api', () => api)

vi.mock('naive-ui', async (importOriginal) => {
  const actual = await importOriginal()
  return {
    ...actual,
    useMessage: () => ({ error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }),
  }
})

// Stub keys are Naive's *internal* component names ('Input'), not the exported
// aliases ('NInput'): these tabs import the components in <script setup>, so Vue
// Test Utils matches a stub against the component's own `name`, and an `N…` key
// silently does nothing — the real Naive component renders instead.
const stubs = {
  Icon: { template: '<i class="n-icon-stub"><slot /></i>' },
  Button: { template: '<button class="n-button-stub"><slot /></button>' },
  Input: { template: '<input class="n-input-stub" />' },
  Select: { template: '<div class="n-select-stub" />' },
  Popover: { template: '<div class="n-popover-stub"><slot name="trigger" /><slot /></div>' },
  // Confirming is Naive's job; the stub turns a click on the trigger straight into
  // the confirmed outcome so the handler under test actually runs.
  Popconfirm: {
    emits: ['positive-click'],
    template:
      '<div class="n-popconfirm-stub" @click="$emit(\'positive-click\')"><slot name="trigger" /><slot /></div>',
  },
}

beforeEach(() => {
  setActivePinia(createPinia())
  // The subtasks tab asks useResponsive whether it is on a phone (the hover
  // preview is off there — #2893); jsdom has no matchMedia, so the mount would
  // throw. Desktop width — the mobile behaviour is covered by the e2e tier.
  vi.clearAllMocks()
  window.matchMedia = vi.fn().mockReturnValue({
    matches: false,
    addEventListener: () => {},
    removeEventListener: () => {},
  })
})

// ── activity: «Всё» (#2958) ───────────────────────────────────────────────────
// The history tab is gone: its journal rows now run between the comment threads,
// by time, in the comments pane's «Всё» mode.
describe('TaskCommentsTab.vue — «Всё»', () => {
  const load = async () => (await import('@/components/task/TaskCommentsTab.vue')).default
  const events = [
    {
      id: 'e1',
      actor_id: 'u1',
      actor_name: 'Аня',
      kind: 'created',
      created_at: '2026-03-07T09:00:00Z',
    },
    // The comment's own journal entry: the thread already shows it.
    {
      id: 'e2',
      actor_id: 'u1',
      actor_name: 'Аня',
      kind: 'comment',
      created_at: '2026-03-07T09:30:00Z',
    },
    {
      id: 'e3',
      actor_id: 'u2',
      actor_name: 'Боря',
      kind: 'moved',
      data: { from: 'К работе', to: 'В процессе' },
      created_at: '2026-03-07T10:00:00Z',
    },
    { id: 'e4', kind: 'recurred', created_at: '2026-03-07T11:00:00Z' },
  ]
  const comments = [
    {
      id: 'c1',
      author_id: 'u1',
      author_name: 'Аня',
      body: 'Привет',
      created_at: '2026-03-07T09:30:00Z',
    },
  ]
  const mountTab = async (mode) =>
    mount(await load(), {
      props: { taskId: 't1', comments, events, mode, readonly: true },
      global: { stubs: { ...stubs, RichContent: true, MarkdownEditor: true } },
    })

  it('interleaves the journal with the threads by time, without the comment entries', async () => {
    const w = await mountTab('all')
    const kinds = w.findAll('.c-items > div').map((n) => n.classes()[0])
    expect(kinds).toEqual(['c-event', 'c-thread', 'c-event', 'c-event'])
  })

  it('draws a move as «from → to» chips and names a system event «Кто-то»', async () => {
    const w = await mountTab('all')
    const rows = w.findAll('.c-event')
    expect(rows[1].findAll('.e-chip').map((n) => n.text())).toEqual(['К работе', 'В процессе'])
    expect(rows[2].text()).toContain('Кто-то')
  })

  it('draws an old move (no source) as one chip and a priority as its pill', async () => {
    const w = mount(await load(), {
      props: {
        taskId: 't1',
        comments: [],
        readonly: true,
        mode: 'all',
        events: [
          { id: 'm', kind: 'moved', data: { to: 'Готово' }, created_at: '2026-03-07T09:00:00Z' },
          { id: 'p', kind: 'priority', data: { to: 3 }, created_at: '2026-03-07T10:00:00Z' },
        ],
      },
      global: { stubs: { ...stubs, RichContent: true, MarkdownEditor: true } },
    })
    const rows = w.findAll('.c-event')
    expect(rows[0].findAll('.e-chip').map((n) => n.text())).toEqual(['Готово'])
    expect(rows[1].find('.e-prio').text()).toBe('Высокий')
  })

  it('shows the threads alone in the «Комментарии» mode', async () => {
    const w = await mountTab('comments')
    expect(w.findAll('.c-event')).toHaveLength(0)
    expect(w.findAll('.c-thread')).toHaveLength(1)
  })
})

// ── files ─────────────────────────────────────────────────────────────────────
describe('TaskFilesTab.vue', () => {
  const load = async () => (await import('@/components/task/TaskFilesTab.vue')).default

  it('scales the size label by magnitude', async () => {
    const C = await load()
    const w = mount(C, {
      props: {
        taskId: 't1',
        attachments: [
          { id: 'a', filename: 'tiny.txt', size: 900 },
          { id: 'b', filename: 'mid.png', size: 2048 },
          { id: 'c', filename: 'big.zip', size: 3 * 1024 * 1024 },
        ],
      },
      global: { stubs },
    })
    const sizes = w.findAll('.f-size').map((n) => n.text())
    expect(sizes).toEqual(['900 Б', '2 КБ', '3.0 МБ'])
  })

  it('publishes the shortened list back to the modal on delete', async () => {
    const C = await load()
    api.tasks.removeAttachment.mockResolvedValue({})
    const w = mount(C, {
      props: {
        taskId: 't1',
        attachments: [
          { id: 'a', filename: 'one.txt', size: 1 },
          { id: 'b', filename: 'two.txt', size: 2 },
        ],
      },
      global: { stubs },
    })
    await w.findAll('button[title="Удалить"]')[0].trigger('click')
    await flushPromises()
    expect(api.tasks.removeAttachment).toHaveBeenCalledWith('a')
    expect(w.emitted('update:attachments')[0][0]).toEqual([
      { id: 'b', filename: 'two.txt', size: 2 },
    ])
  })
})

// ── relations ─────────────────────────────────────────────────────────────────
describe('TaskRelationsTab.vue', () => {
  const load = async () => (await import('@/components/task/TaskRelationsTab.vue')).default

  it('labels the relation kind and marks a completed target', async () => {
    const C = await load()
    const w = mount(C, {
      props: {
        taskId: 't1',
        relations: [
          { id: 'r1', kind: 'blocks', related_number: 7, related_title: 'Семь' },
          {
            id: 'r2',
            kind: 'relates',
            related_number: 8,
            related_title: 'Восемь',
            related_completed_at: '2026-03-07T09:05:00Z',
          },
        ],
      },
      global: { stubs },
    })
    expect(w.findAll('.rel-kind').map((n) => n.text())).toEqual(['блокирует', 'связана с'])
    expect(w.findAll('.rel-link')[1].classes()).toContain('done')
  })

  it('asks the modal to navigate instead of routing itself', async () => {
    const C = await load()
    const rel = { id: 'r1', kind: 'relates', related_number: 7, related_title: 'Семь' }
    const w = mount(C, { props: { taskId: 't1', relations: [rel] }, global: { stubs } })
    await w.find('.rel-link').trigger('click')
    // Deep equality, not identity: the payload is the reactive proxy of `rel`.
    expect(w.emitted('open-related')[0][0]).toEqual(rel)
  })

  it('groups picker results by project/board and hides the task itself', async () => {
    const C = await load()
    api.workspaces.tasks.mockResolvedValue({
      data: [
        { id: 't1', number: 1, title: 'сама задача', project_name: 'P', board_name: 'B' },
        { id: 't2', number: 2, title: 'вторая', project_name: 'P', board_name: 'B' },
        { id: 't3', number: 3, title: 'третья', project_name: 'Q', board_name: 'B' },
        { id: 't4', number: null, title: 'без номера', project_name: 'P', board_name: 'B' },
      ],
    })
    const w = mount(C, { props: { taskId: 't1', relations: [], wsId: 'w1' }, global: { stubs } })
    // v2 (#2958): the form folds behind «+ Связать» in the narrow panel.
    await w.find('.rel-open').trigger('click')
    await w.find('.n-input-stub').trigger('focus')
    await flushPromises()
    const heads = w.findAll('.rp-head').map((n) => n.text())
    expect(heads).toEqual(['P · B', 'Q · B'])
    const titles = w.findAll('.rp-title').map((n) => n.text())
    expect(titles).toEqual(['вторая', 'третья']) // no self-link, no numberless task
  })
})

// ── subtasks ──────────────────────────────────────────────────────────────────
describe('TaskSubtasksTab.vue', () => {
  const load = async () => (await import('@/components/task/TaskSubtasksTab.vue')).default
  const task = {
    id: 't1',
    board_id: 'b1',
    column_id: 'c1',
    subtasks: [
      { id: 's1', title: 'Первая', column_id: 'c1', priority: 2 },
      { id: 's2', title: 'Вторая', column_id: 'c2', completed_at: '2026-03-07T09:05:00Z' },
    ],
  }
  const columns = [
    { id: 'c1', name: 'К работе', color: '#aaa', position: 1 },
    { id: 'c2', name: 'Готово', color: '#bbb', position: 2 },
  ]

  it('resolves each subtask column from the columns prop', async () => {
    seedBoardStore({ columns })
    const C = await load()
    const w = mount(C, { props: { task, columns }, global: { stubs } })
    expect(w.findAll('.subrow')).toHaveLength(2)
    expect(w.text()).toContain('К работе')
    expect(w.text()).toContain('Готово')
    expect(w.findAll('.subrow')[1].classes()).toContain('done')
  })

  it('hides the column picker in readonly mode', async () => {
    seedBoardStore({ columns })
    const C = await load()
    const w = mount(C, { props: { task, columns, readonly: true }, global: { stubs } })
    // The status still shows (v2: glyph + name), but as text, without its menu.
    expect(w.findAll('.col-chip.static')).toHaveLength(task.subtasks.length)
    expect(w.findAll('.col-item')).toHaveLength(0)
  })

  it('reports «changed» after completing a subtask so the modal reloads', async () => {
    seedBoardStore({ columns })
    api.tasks.update.mockResolvedValue({})
    const C = await load()
    const w = mount(C, { props: { task, columns }, global: { stubs } })
    await w.findAll('.check')[0].trigger('click')
    await flushPromises()
    expect(api.tasks.update).toHaveBeenCalledWith(
      's1',
      expect.objectContaining({ completed: true }),
    )
    expect(w.emitted('changed')).toHaveLength(1)
  })

  it('opens a subtask by id rather than handling navigation itself', async () => {
    seedBoardStore({ columns })
    const C = await load()
    const w = mount(C, { props: { task, columns }, global: { stubs } })
    await w.findAll('.subrow')[0].trigger('click')
    expect(w.emitted('open')[0]).toEqual(['s1'])
  })

  // ── GitLab hierarchy column (#2592) ──
  // The distinction that matters: an issue exists (gl_iid) but GitLab did not accept
  // it as a child (empty gl_parent_global_id). That subtask is NOT lost, so the row
  // must offer a retry instead of reading as "in GitLab".
  const glTask = (over = {}) => ({
    ...task,
    gitlab: { iid: 10, is_group: true, ...(over.gitlab || {}) },
    subtasks: [
      {
        id: 's1',
        title: 'В иерархии',
        column_id: 'c1',
        gl_iid: 11,
        gl_parent_global_id: 'gid://x',
      },
      { id: 's2', title: 'Вне иерархии', column_id: 'c1', gl_iid: 12, gl_parent_global_id: '' },
      { id: 's3', title: 'Не выгружена', column_id: 'c1' },
    ],
  })

  it('tells "child work item" apart from "issue exists but is not in the hierarchy"', async () => {
    seedBoardStore({ columns })
    const C = await load()
    const w = mount(C, {
      props: { task: glTask(), columns, gitlabCanGroup: true },
      global: { stubs },
    })
    const chips = w.findAll('.gl-chip')
    expect(chips).toHaveLength(3)
    // Real child → a plain link to the issue, no action.
    expect(chips[0].element.tagName).toBe('A')
    expect(chips[0].text()).toBe('!11')
    // The other two are actionable, and say which of the two problems it is.
    expect(chips[1].text()).toBe('вне иерархии')
    expect(chips[2].text()).toBe('не в GitLab')
  })

  it('queues a push for the subtask that is out of the hierarchy', async () => {
    seedBoardStore({ columns })
    api.gitlab.pushChild.mockResolvedValue({})
    const C = await load()
    const w = mount(C, {
      props: { task: glTask(), columns, gitlabCanGroup: true },
      global: { stubs },
    })
    await w.findAll('.gl-chip')[1].trigger('click')
    await flushPromises()
    expect(api.gitlab.pushChild).toHaveBeenCalledWith('s2')
  })

  it('offers to mark the parent instead of per-row pushes when it is not grouped', async () => {
    seedBoardStore({ columns })
    const C = await load()
    const w = mount(C, {
      props: { task: glTask({ gitlab: { is_group: false } }), columns, gitlabCanGroup: true },
      global: { stubs },
    })
    // Nothing here can reach the hierarchy, so no row offers a push…
    expect(w.findAll('.gl-chip.act')).toHaveLength(0)
    // …and the fix is offered once, at the bottom.
    await w.find('.gl-hint-act').trigger('click')
    expect(w.emitted('group-parent')).toHaveLength(1)
  })

  it('shows nothing GitLab-ish when the integration does not push children', async () => {
    seedBoardStore({ columns })
    const C = await load()
    const w = mount(C, { props: { task: glTask(), columns }, global: { stubs } })
    expect(w.findAll('.gl-chip')).toHaveLength(0)
    expect(w.find('.gl-hint').exists()).toBe(false)
  })
})
