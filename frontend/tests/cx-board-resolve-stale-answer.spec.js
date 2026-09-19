import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount, flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'

// A board switch answered out of order (#2932).
//
// `/project/<p>/board/<b>` names the board by slug; BoardView turns that pair into
// the UUID KanbanBoard works with. The resolution is a request, and the reader is
// free to click the next board in the tree while it is in flight — at which point
// two resolutions are racing and nobody compared the answers afterwards.
//
// The loser decides the screen. A late answer for the abandoned board publishes its
// id (KanbanBoard is keyed on it, so the whole board remounts), switches the app's
// workspace to that board's, and — worst of the three — `router.replace`s the URL to
// the abandoned board's path. The reader clicks B and lands back on A, with the
// address bar agreeing: nothing on screen says a stale request won, and the way back
// is to click B again and hope it is faster this time.
//
// Same comparison the sections learned in cx-ws-switch-stale-answer / TaskModal
// (cx-task-modal-stale-load): the answer is only allowed on screen while it is still
// the answer to the question being asked.

const toast = { error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }
vi.mock('naive-ui', async () => {
  const actual = await vi.importActual('naive-ui')
  return { ...actual, useMessage: () => toast }
})

const boardsApi = { resolve: vi.fn(), get: vi.fn() }
const projectsApi = { get: vi.fn(), boards: vi.fn(() => Promise.resolve({ data: [] })) }
// Switching the space is a real store call here (ensureWorkspace reaches the
// store's own closure, not the instance property), so its fetches need answers.
const wsApi = {
  groups: vi.fn(() => Promise.resolve({ data: [] })),
  projects: vi.fn(() => Promise.resolve({ data: [] })),
  commands: vi.fn(() => Promise.resolve({ data: {} })),
}
vi.mock('@/api', () => ({
  boards: boardsApi,
  projects: projectsApi,
  workspaces: wsApi,
  getAccessToken: () => null,
}))

const { default: BoardView } = await import('@/views/BoardView.vue')
const { useWorkspacesStore } = await import('@/stores/workspaces')

/** A promise plus the handles that settle it, so a resolution can be held in flight. */
function deferred() {
  let resolve, reject
  const promise = new Promise((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

const board = (slug, over = {}) => ({
  id: `id-${slug}`,
  slug,
  project_id: 'p1',
  workspace_id: 'ws1',
  ...over,
})

// KanbanBoard pulls the whole board stack; this spec is about which id reaches it.
const KanbanStub = {
  name: 'KanbanBoard',
  props: ['boardId'],
  template: '<div class="kb-stub">{{ boardId }}</div>',
}

const mounted = []
async function mountAt(path) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/project/:projectSlug/board/:boardSlug', component: BoardView },
      { path: '/board/:id', component: BoardView },
      { path: '/:rest(.*)*', component: { template: '<div />' } },
    ],
  })
  router.push(path)
  await router.isReady()
  const w = mount(BoardView, {
    global: { plugins: [router], stubs: { KanbanBoard: KanbanStub } },
  })
  mounted.push(w)
  await flushPromises()
  return { w, router }
}

const shownBoard = (w) => w.find('.kb-stub').exists() && w.find('.kb-stub').text()

beforeEach(() => {
  setActivePinia(createPinia())
  localStorage.clear()
  boardsApi.resolve.mockReset()
  boardsApi.get.mockReset()
  projectsApi.get.mockReset()
  for (const fn of Object.values(toast)) fn.mockReset()
  projectsApi.get.mockResolvedValue({ data: { slug: 'p1' } })
  // Canonicalizing a legacy link re-runs the resolution under its new params
  // (the params watcher is deep and vue-router hands it a fresh object), so the
  // slug pair has to answer in every test, not only the ones that navigate.
  boardsApi.resolve.mockImplementation((_p, slug) => Promise.resolve({ data: board(slug) }))
  useWorkspacesStore().currentId = 'ws1'
})

afterEach(() => {
  while (mounted.length) mounted.pop().unmount()
  document.body.innerHTML = ''
})

describe('BoardView ignores the board the reader has left (#2932)', () => {
  it('keeps the board that was clicked last when the abandoned one answers later', async () => {
    const { w, router } = await mountAt('/project/p1/board/alpha')
    expect(shownBoard(w)).toBe('id-alpha')

    // Clicked «beta» — its resolution is still out...
    const beta = deferred()
    boardsApi.resolve.mockReturnValueOnce(beta.promise)
    router.push('/project/p1/board/beta')
    await flushPromises()

    // ...when «gamma» is clicked and answers first. That is the board the reader is on.
    router.push('/project/p1/board/gamma')
    await flushPromises()
    expect(shownBoard(w)).toBe('id-gamma')

    beta.resolve({ data: board('beta') })
    await flushPromises()

    expect(shownBoard(w)).toBe('id-gamma')
  })

  it('does not drag the URL back to the abandoned board', async () => {
    const { router } = await mountAt('/project/p1/board/alpha')

    const beta = deferred()
    boardsApi.resolve.mockReturnValueOnce(beta.promise)
    router.push('/project/p1/board/beta')
    await flushPromises()
    router.push('/project/p1/board/gamma')
    await flushPromises()

    beta.resolve({ data: board('beta') })
    await flushPromises()

    expect(router.currentRoute.value.path).toBe('/project/p1/board/gamma')
  })

  it('does not switch the workspace to the abandoned board', async () => {
    const store = useWorkspacesStore()
    const { router } = await mountAt('/project/p1/board/alpha')

    // The abandoned board lives in another space, so its answer would move the
    // whole app — sidebar, notifications, everything scoped to `currentId`.
    const beta = deferred()
    boardsApi.resolve.mockReturnValueOnce(beta.promise)
    router.push('/project/p1/board/beta')
    await flushPromises()
    router.push('/project/p1/board/gamma')
    await flushPromises()

    beta.resolve({ data: board('beta', { workspace_id: 'ws2' }) })
    await flushPromises()

    expect(store.currentId).toBe('ws1')
    expect(wsApi.groups).not.toHaveBeenCalledWith('ws2')
  })

  it('ignores the legacy /board/<id> answer once the reader has moved on', async () => {
    const legacy = deferred()
    boardsApi.get.mockReturnValueOnce(legacy.promise)
    const { w, router } = await mountAt('/board/id-alpha')
    expect(shownBoard(w)).toBe(false)

    router.push('/project/p1/board/gamma')
    await flushPromises()
    expect(shownBoard(w)).toBe('id-gamma')

    legacy.resolve({ data: board('alpha') })
    await flushPromises()

    expect(shownBoard(w)).toBe('id-gamma')
    expect(router.currentRoute.value.path).toBe('/project/p1/board/gamma')
  })

  it('does not blank the open board when the abandoned one 404s', async () => {
    const { w, router } = await mountAt('/project/p1/board/alpha')

    const beta = deferred()
    boardsApi.resolve.mockReturnValueOnce(beta.promise)
    router.push('/project/p1/board/beta')
    await flushPromises()
    router.push('/project/p1/board/gamma')
    await flushPromises()

    // «beta» was deleted while it was being opened. That is news about a board
    // nobody is looking at any more.
    beta.reject(new Error('404'))
    await flushPromises()

    expect(shownBoard(w)).toBe('id-gamma')
  })

  it('still resolves, canonicalizes and switches space on the ordinary path', async () => {
    const store = useWorkspacesStore()
    const foreign = { workspace_id: 'ws2' }
    boardsApi.get.mockResolvedValue({ data: board('alpha', foreign) })
    // The canonical URL is resolved again under its new params (see beforeEach),
    // and it names the same board — so it answers with the same space.
    boardsApi.resolve.mockImplementation((_p, slug) =>
      Promise.resolve({ data: board(slug, foreign) }),
    )
    const { w, router } = await mountAt('/board/id-alpha')

    expect(shownBoard(w)).toBe('id-alpha')
    expect(router.currentRoute.value.path).toBe('/project/p1/board/alpha')
    expect(store.currentId).toBe('ws2')
  })

  it('keeps the query when canonicalizing a deep link', async () => {
    boardsApi.get.mockResolvedValue({ data: board('alpha') })
    const { router } = await mountAt('/board/id-alpha?task=252')

    expect(router.currentRoute.value.path).toBe('/project/p1/board/alpha')
    expect(router.currentRoute.value.query.task).toBe('252')
  })
})
