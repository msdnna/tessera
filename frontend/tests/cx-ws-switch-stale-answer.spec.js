import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount, flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'

// A workspace switch answered out of order (#2932).
//
// Every workspace-scoped section refetches on `wsStore.currentId`, and the answer
// to the request it just abandoned is still on its way. Nobody compared it after
// the await, so what ended up on screen was decided by ARRIVAL order rather than
// by the workspace the reader is actually in: the previous space's tasks,
// milestones and notes land under the new space's name, and nothing refetches
// them — the switch has already happened.
//
// It is not only a wrong list. On «Главной» those rows are links: clicking one
// routes to a board of a workspace the reader has left. In «Заметках» the answer
// also re-opens a note through `?note=`, so the editor fills with a foreign note
// and Сохранить writes to it. The conflict badge in the shell counts someone
// else's conflicts.
//
// «Созвоны» has compared since cx-conf-list-stale-answer; these tests pin the
// same comparison for the sections that were left behind.

const toast = { error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }
vi.mock('naive-ui', async () => {
  const actual = await vi.importActual('naive-ui')
  return { ...actual, useMessage: () => toast }
})

const wsApi = {
  summary: vi.fn(),
  tasks: vi.fn(),
  tags: vi.fn(() => Promise.resolve({ data: [] })),
  tagPrefixes: vi.fn(() => Promise.resolve({ data: [] })),
  members: vi.fn(() => Promise.resolve({ data: [] })),
  milestones: vi.fn(),
}
const notesApi = {
  list: vi.fn(),
  create: vi.fn(),
  update: vi.fn(),
  remove: vi.fn(),
}
const glApi = { conflicts: vi.fn() }
// «Входящие» (#2972): the feed replaced the «Заметки» section.
const inboxApi = { list: vi.fn() }
// v2 «Мои задачи» (#2960) also reads the reminders.
const remApi = { list: vi.fn(() => Promise.resolve({ data: [] })) }
vi.mock('@/api', () => ({
  workspaces: wsApi,
  notes: notesApi,
  gitlab: glApi,
  reminders: remApi,
  inbox: inboxApi,
  tasks: { update: vi.fn() },
  getAccessToken: () => null,
}))

const { default: HomeView } = await import('@/views/HomeView.vue')
const { default: MilestonesView } = await import('@/views/MilestonesView.vue')
const { useInboxStore } = await import('@/stores/inbox')
const { useWorkspacesStore } = await import('@/stores/workspaces')
const { useAuthStore } = await import('@/stores/auth')
const { useConflictsStore } = await import('@/stores/conflicts')

/** A promise plus the handles that settle it, so a section can be held in flight. */
function deferred() {
  let resolve, reject
  const promise = new Promise((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

const task = (over = {}) => ({
  id: 't1',
  number: 1,
  title: 'Задача',
  board_id: 'b1',
  column_name: 'К работе',
  priority: 'medium',
  assignees: [],
  tags: [],
  due_at: null,
  completed_at: null,
  ...over,
})

const milestone = (over = {}) => ({
  id: 'm1',
  project_id: 'p1',
  project_name: 'Проект',
  title: 'Веха',
  state: 'active',
  task_count: 0,
  done_count: 0,
  ...over,
})

const note = (over = {}) => ({
  id: 'n1',
  slug: 'zametka',
  title: 'Заметка',
  body: 'Текст',
  ...over,
})

const emptySummary = {
  overdue: 0,
  today: 0,
  week: 0,
  completed: 0,
  assigned: 0,
  total: 0,
}

const mounted = []
async function mountAt(component, path) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', component },
      { path: '/:rest(.*)*', component: { template: '<div />' } },
    ],
  })
  router.push(path)
  await router.isReady()
  const w = mount(component, { global: { plugins: [router] } })
  mounted.push(w)
  await flushPromises()
  return { w, router }
}

beforeEach(() => {
  setActivePinia(createPinia())
  localStorage.clear()
  // useResponsive reads matchMedia on mount; jsdom has none.
  window.matchMedia = vi.fn().mockReturnValue({
    matches: false,
    addEventListener: () => {},
    removeEventListener: () => {},
  })
  for (const fn of [
    ...Object.values(wsApi),
    ...Object.values(notesApi),
    glApi.conflicts,
    inboxApi.list,
  ]) {
    fn.mockReset()
  }
  for (const fn of Object.values(toast)) fn.mockReset()
  wsApi.summary.mockResolvedValue({ data: emptySummary })
  wsApi.tasks.mockResolvedValue({ data: [] })
  wsApi.tags.mockResolvedValue({ data: [] })
  wsApi.tagPrefixes.mockResolvedValue({ data: [] })
  wsApi.members.mockResolvedValue({ data: [] })
  wsApi.milestones.mockResolvedValue({ data: [] })
  notesApi.list.mockResolvedValue({ data: [] })
  glApi.conflicts.mockResolvedValue({ data: [] })
  useWorkspacesStore().currentId = 'ws1'
  useAuthStore().user = { id: 'u1', is_admin: false }
})

afterEach(() => {
  while (mounted.length) mounted.pop().unmount()
  document.body.innerHTML = ''
})

describe('«Главная» ignores the workspace the reader has left', () => {
  it('keeps the new workspace when the previous one answers last', async () => {
    const store = useWorkspacesStore()
    const { w } = await mountAt(HomeView, '/')
    // v2 lists the reader's own open tasks, by due date (#2960): assigned to u1
    // and due today, so the rows sit in an unfolded section.
    const mine = (over) =>
      task({ assignee_ids: ['u1'], due_date: new Date().toISOString(), ...over })

    // The page asked of ws1, still out when the reader switches away.
    const oldWs = deferred()
    wsApi.tasks.mockReturnValueOnce(oldWs.promise)
    store.currentId = 'ws2'
    // The switch's own request is the one the reader is owed.
    await flushPromises()
    wsApi.tasks.mockResolvedValue({ data: [mine({ id: 't2', title: 'Задача второго' })] })
    store.currentId = 'ws3'
    await flushPromises()
    expect(w.text()).toContain('Задача второго')

    oldWs.resolve({ data: [mine({ id: 't1', title: 'Задача первого' })] })
    await flushPromises()

    expect(w.text()).toContain('Задача второго')
    expect(w.text()).not.toContain('Задача первого')
  })

  it('leaves the spinner up while the newest workspace is still loading', async () => {
    const store = useWorkspacesStore()
    const { w } = await mountAt(HomeView, '/')

    const first = deferred()
    wsApi.tasks.mockReturnValueOnce(first.promise)
    store.currentId = 'ws2'
    await flushPromises()

    const second = deferred()
    wsApi.tasks.mockReturnValueOnce(second.promise)
    store.currentId = 'ws3'
    await flushPromises()

    // The abandoned request settles first: it must not put out a spinner lit
    // for the page that is still on its way.
    first.resolve({ data: [] })
    await flushPromises()
    expect(w.vm.loading).toBe(true)

    second.resolve({ data: [] })
    await flushPromises()
    expect(w.vm.loading).toBe(false)
  })
})

describe('«Вехи» ignore the workspace the reader has left', () => {
  it('keeps the new workspace when the previous one answers last', async () => {
    const store = useWorkspacesStore()
    const { w } = await mountAt(MilestonesView, '/')

    const oldWs = deferred()
    wsApi.milestones.mockReturnValueOnce(oldWs.promise)
    store.currentId = 'ws2'
    await flushPromises()

    wsApi.milestones.mockResolvedValueOnce({
      data: [milestone({ id: 'm2', title: 'Веха второго' })],
    })
    store.currentId = 'ws3'
    await flushPromises()
    expect(w.text()).toContain('Веха второго')

    oldWs.resolve({ data: [milestone({ id: 'm1', title: 'Веха первого' })] })
    await flushPromises()

    expect(w.text()).toContain('Веха второго')
    expect(w.text()).not.toContain('Веха первого')
  })

  it('does not blame the current workspace for a superseded request failing', async () => {
    const store = useWorkspacesStore()
    await mountAt(MilestonesView, '/')

    const oldWs = deferred()
    wsApi.milestones.mockReturnValueOnce(oldWs.promise)
    store.currentId = 'ws2'
    await flushPromises()

    wsApi.milestones.mockResolvedValueOnce({ data: [] })
    store.currentId = 'ws3'
    await flushPromises()

    oldWs.reject(new Error('пространство недоступно'))
    await flushPromises()

    expect(toast.error).not.toHaveBeenCalled()
  })
})

describe('«Входящие» ignore the workspace the reader has left', () => {
  it('keeps the new workspace when the previous one answers last', async () => {
    const inbox = useInboxStore()
    const oldWs = deferred()
    inboxApi.list.mockReturnValueOnce(oldWs.promise)
    const first = inbox.load('ws2')

    inboxApi.list.mockResolvedValueOnce({
      data: { reminders: [], notes: [note({ id: 'n2', title: 'Заметка второго' })] },
    })
    await inbox.load('ws3')
    expect(inbox.notes.map((n) => n.title)).toEqual(['Заметка второго'])

    oldWs.resolve({
      data: { reminders: [], notes: [note({ id: 'n1', title: 'Заметка первого' })] },
    })
    await first
    expect(inbox.wsId).toBe('ws3')
    expect(inbox.notes.map((n) => n.title)).toEqual(['Заметка второго'])
  })

  it('drops a note event from another workspace', async () => {
    const inbox = useInboxStore()
    inboxApi.list.mockResolvedValueOnce({ data: { reminders: [], notes: [] } })
    await inbox.load('ws3')
    inbox.onEvent({ scope: 'ws2', type: 'note.created', data: note({ workspace_id: 'ws2' }) })
    expect(inbox.notes).toEqual([])
    inbox.onEvent({
      scope: 'ws3',
      type: 'note.created',
      data: note({ id: 'n3', workspace_id: 'ws3' }),
    })
    expect(inbox.notes.map((n) => n.id)).toEqual(['n3'])
  })
})

describe('the conflict badge ignores the workspace the reader has left', () => {
  it('keeps the new workspace when the previous one answers last', async () => {
    const store = useConflictsStore()

    const oldWs = deferred()
    glApi.conflicts.mockReturnValueOnce(oldWs.promise)
    const first = store.load('ws1')

    glApi.conflicts.mockResolvedValueOnce({ data: [{ id: 'c2', task_id: 't2' }] })
    await store.load('ws2')
    expect(store.list.length).toBe(1)

    oldWs.resolve({ data: [{ id: 'c1' }, { id: 'c1b' }] })
    await first
    await flushPromises()

    expect(store.list.map((c) => c.id)).toEqual(['c2'])
  })
})
