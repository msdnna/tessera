import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount, flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'

// A workspace load that FAILS, not one that answers late (#2932).
//
// cx-ws-switch-stale-answer settled the ordering half: every workspace-scoped
// section claims a generation and drops the answer of the space the reader has
// left. But the guard only decides who may WRITE — nobody asks what stays on
// screen when the new space's request never answers with data at all. A refused
// load (403 after being dropped from the space, 404 for one just deleted, a 500
// from the summary query) leaves the previous workspace's rows exactly where they
// were, now under the new workspace's name:
//
//   • «Главная» is the worst of the three, and twice over. It has no `catch` at
//     all — `try { … } finally { loading = false }` — so the rejection is not even
//     reported: the page lands on «Здесь пока пусто» with every tile at 0 and not
//     a word about the failure. That empty state is a lie in the other direction
//     too: on a switch the previous space's tasks stay listed, and each row is a
//     link that routes to a board outside the current scope.
//   • «Вехи» and «Заметки» do toast, but they keep the abandoned list as well; in
//     «Заметках» clicking one of those rows opens a foreign note in the editor and
//     Сохранить writes into it.
//
// The rule these tests pin: data on screen belongs to the workspace it was loaded
// for. A failure to load a DIFFERENT workspace clears it; a failure to refetch the
// SAME one (a save/delete refetch in «Заметках») must not, since those rows are
// still current — only their refresh went missing.

const toast = { error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }
vi.mock('naive-ui', async () => {
  const actual = await vi.importActual('naive-ui')
  return { ...actual, useMessage: () => toast }
})

const wsApi = {
  summary: vi.fn(),
  tasks: vi.fn(),
  tags: vi.fn(),
  tagPrefixes: vi.fn(),
  members: vi.fn(),
  milestones: vi.fn(),
}
const notesApi = {
  list: vi.fn(),
  create: vi.fn(),
  update: vi.fn(),
  remove: vi.fn(),
}
const glApi = { conflicts: vi.fn() }
// «Входящие» (#2972) replaced the «Заметки» section.
const inboxApi = { list: vi.fn() }
// v2 «Мои задачи» (#2960) also reads the reminders and completes tasks in place.
const remApi = { list: vi.fn() }
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

/** An HTTP error the way the api layer hands it on: a message, and a status that
 *  says the server answered (so it is not the offline overlay's business). */
function refused(message = 'Нет доступа', status = 403) {
  const e = new Error(message)
  e.status = status
  e.offline = false
  return e
}

const task = (over = {}) => ({
  id: 't1',
  number: 1,
  title: 'Задача',
  board_id: 'b1',
  column_name: 'К работе',
  priority: 'medium',
  assignee_ids: [],
  tag_ids: [],
  due_date: null,
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
  due_today: 0,
  due_week: 0,
  completed: 0,
  assigned: 0,
  active: 0,
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
    remApi.list,
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
  remApi.list.mockResolvedValue({ data: [] })
  useWorkspacesStore().currentId = 'ws1'
  useAuthStore().user = { id: 'u1', is_admin: false }
})

afterEach(() => {
  while (mounted.length) mounted.pop().unmount()
  document.body.innerHTML = ''
})

describe('«Главная» reports a refused load instead of «пусто» (#2932)', () => {
  // v2 «Мои задачи» (#2960) reads the task list itself (no summary tiles) and
  // lists only open tasks assigned to the reader.
  // Due today: undated tasks sit in the folded «Без срока» section.
  const mine = (over = {}) =>
    task({ assignee_ids: ['u1'], due_date: new Date().toISOString(), ...over })
  const EMPTY = 'Открытых задач на вас нет'

  it('says what went wrong when the first load is refused', async () => {
    wsApi.tasks.mockRejectedValue(refused('Нет доступа к пространству'))
    const { w } = await mountAt(HomeView, '/')

    expect(toast.error).toHaveBeenCalledWith('Нет доступа к пространству')
    // «Нет задач» is a claim about the workspace; nothing was read.
    expect(w.text()).not.toContain(EMPTY)
    expect(w.text()).toContain('Нет доступа к пространству')
    expect(w.text()).toContain('Повторить')
    expect(w.vm.loading).toBe(false)
  })

  it('a refused load does not leave the previous workspace listed', async () => {
    wsApi.tasks.mockResolvedValue({ data: [mine({ title: 'Задача первого' })] })
    const { w } = await mountAt(HomeView, '/')
    expect(w.text()).toContain('Задача первого')

    wsApi.tasks.mockRejectedValue(refused())
    useWorkspacesStore().currentId = 'ws2'
    await flushPromises()

    expect(w.text()).not.toContain('Задача первого')
    expect(toast.error).toHaveBeenCalled()
  })

  it('a refused load drops the counts it can no longer vouch for', async () => {
    wsApi.tasks.mockResolvedValue({
      data: [mine({ due_date: '2020-01-01T00:00:00Z', title: 'Давняя' })],
    })
    const { w } = await mountAt(HomeView, '/')
    expect(w.find('[data-pill="overdue"]').exists()).toBe(true)

    wsApi.tasks.mockRejectedValue(refused())
    useWorkspacesStore().currentId = 'ws2'
    await flushPromises()

    expect(w.find('[data-pill="overdue"]').exists()).toBe(false)
    expect(w.vm.tasks).toEqual([])
  })

  it('retry re-reads the workspace the failure was about', async () => {
    wsApi.tasks.mockRejectedValueOnce(refused())
    const { w } = await mountAt(HomeView, '/')
    expect(w.vm.loadError).toBeTruthy()

    wsApi.tasks.mockResolvedValue({ data: [mine({ title: 'Снова тут' })] })
    await w.vm.load()
    await flushPromises()

    expect(w.vm.loadError).toBe('')
    expect(w.text()).toContain('Снова тут')
  })

  it('a superseded failure neither shouts nor clears the open workspace', async () => {
    const { w } = await mountAt(HomeView, '/')
    const store = useWorkspacesStore()

    // ws2's load is refused, but only after the reader has moved on to ws3.
    let rejectOld
    wsApi.tasks.mockReturnValueOnce(new Promise((_, rej) => (rejectOld = rej)))
    store.currentId = 'ws2'
    await flushPromises()
    wsApi.tasks.mockResolvedValue({ data: [mine({ title: 'Задача третьего' })] })
    store.currentId = 'ws3'
    await flushPromises()
    expect(w.text()).toContain('Задача третьего')

    rejectOld(refused())
    await flushPromises()

    expect(toast.error).not.toHaveBeenCalled()
    expect(w.text()).toContain('Задача третьего')
    expect(w.vm.loadError).toBe('')
  })

  it('a successful load shows the empty state and no error (control)', async () => {
    const { w } = await mountAt(HomeView, '/')

    expect(toast.error).not.toHaveBeenCalled()
    expect(w.text()).toContain(EMPTY)
  })
})

describe('«Вехи» drop the workspace they failed to leave (#2932)', () => {
  it('a refused switch does not keep the previous space’s milestones', async () => {
    wsApi.milestones.mockResolvedValue({ data: [milestone({ title: 'Веха первого' })] })
    const { w } = await mountAt(MilestonesView, '/')
    expect(w.text()).toContain('Веха первого')

    wsApi.milestones.mockRejectedValue(refused())
    useWorkspacesStore().currentId = 'ws2'
    await flushPromises()

    expect(toast.error).toHaveBeenCalled()
    expect(w.text()).not.toContain('Веха первого')
  })

  it('a superseded failure leaves the open workspace alone (control)', async () => {
    const store = useWorkspacesStore()
    const { w } = await mountAt(MilestonesView, '/')

    let rejectOld
    wsApi.milestones.mockReturnValueOnce(new Promise((_, rej) => (rejectOld = rej)))
    store.currentId = 'ws2'
    await flushPromises()
    wsApi.milestones.mockResolvedValue({ data: [milestone({ title: 'Веха третьего' })] })
    store.currentId = 'ws3'
    await flushPromises()
    expect(w.text()).toContain('Веха третьего')

    rejectOld(refused())
    await flushPromises()

    expect(toast.error).not.toHaveBeenCalled()
    expect(w.text()).toContain('Веха третьего')
  })
})

describe('«Входящие» drop the workspace they failed to leave (#2932)', () => {
  it('a refused switch does not keep the previous space’s notes', async () => {
    const inbox = useInboxStore()
    inboxApi.list.mockResolvedValueOnce({
      data: { reminders: [], notes: [note({ title: 'Заметка первого' })] },
    })
    await inbox.load('ws1')
    expect(inbox.notes).toHaveLength(1)

    inboxApi.list.mockRejectedValueOnce(refused())
    await inbox.load('ws2')
    // Opening one of these would edit a foreign note.
    expect(inbox.notes).toEqual([])
    expect(inbox.error).toBeTruthy()
  })

  it('a failed refetch of the SAME workspace keeps the list (control)', async () => {
    const inbox = useInboxStore()
    inboxApi.list.mockResolvedValueOnce({
      data: { reminders: [], notes: [note({ title: 'Заметка первого' })] },
    })
    await inbox.load('ws1')
    inboxApi.list.mockRejectedValueOnce(refused('Сервер не ответил', 500))
    await inbox.load('ws1')
    expect(inbox.notes.map((n) => n.title)).toEqual(['Заметка первого'])
  })
})
