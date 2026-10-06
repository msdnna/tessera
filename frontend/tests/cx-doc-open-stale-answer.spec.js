import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount, flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'

// A document opened out of order (#2932).
//
// The section keeps one view for the grid and the editor (`documents/:slug?` is a
// single route record, #2727), so opening a document is not a remount but a set of
// assignments made after an await: `selected`, the editor body, the presence room,
// and — the loud one — the URL. Nobody compared the answer with the question.
//
// The reader is free to click the next tile, press Back, or leave for the grid while
// a document is still loading, and then two requests are in flight. The late one
// wins by arriving last: the document the reader abandoned takes the screen, joins
// its collaboration room, and `router.push`es the address bar back to itself. Leaving
// the section is not an escape either — a load that lands after `backToGrid()` puts
// the reader back inside the document they just closed.
//
// `resolveSlug` is worse than the rest: it walks the user's workspaces looking for the
// slug and SWITCHES THE APP'S SCOPE to the one that answers. A stale walk therefore
// drags the whole app into another space behind a document nobody asked for.
//
// Same comparison the rest of the app learned under this task (BoardView,
// cx-ws-switch-stale-answer, TaskModal): an answer is allowed on screen only while it
// is still the answer to the question being asked.

const toast = { error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }
vi.mock('naive-ui', async () => {
  const actual = await vi.importActual('naive-ui')
  return { ...actual, useMessage: () => toast }
})

const docsApi = {
  list: vi.fn(),
  get: vi.fn(),
  bySlug: vi.fn(),
  converterStatus: vi.fn(() => Promise.resolve({ data: { available: false } })),
}
const wsApi = {
  groups: vi.fn(() => Promise.resolve({ data: [] })),
  projects: vi.fn(() => Promise.resolve({ data: [] })),
  commands: vi.fn(() => Promise.resolve({ data: {} })),
}
vi.mock('@/api', () => ({
  documents: docsApi,
  workspaces: wsApi,
  getAccessToken: () => null,
}))

// The presence socket is the one side effect that reaches outside the component,
// and which room it joined is exactly what a stale open gets wrong — so it is
// observed rather than ignored.
const room = { open: vi.fn(), close: vi.fn() }
vi.mock('@/composables/useDocPresence', async () => {
  const { computed, ref, shallowRef } = await vi.importActual('vue')
  return {
    useDocPresence: () => ({
      viewers: ref([]),
      locks: ref([]),
      foreignLocks: computed(() => []),
      connId: ref('c1'),
      userId: ref('u1'),
      connected: ref(true),
      held: ref(''),
      denied: shallowRef(null),
      commentsNudge: ref(0),
      contentNudge: ref(0),
      linksNudge: ref(0),
      remoteSave: shallowRef(null),
      lockedBy: computed(() => () => null),
      open: room.open,
      close: room.close,
      acquire: vi.fn(),
      release: vi.fn(),
    }),
  }
})

const { default: DocumentsView } = await import('@/views/DocumentsView.vue')
const { useWorkspacesStore } = await import('@/stores/workspaces')

/** A promise plus the handles that settle it, so a load can be held in flight. */
function deferred() {
  let resolve, reject
  const promise = new Promise((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

const doc = (slug, over = {}) => ({
  id: `id-${slug}`,
  slug,
  title: `Документ ${slug}`,
  workspace_id: 'ws-1',
  updated_at: '2026-09-19T20:00:00Z',
  content: { type: 'doc', content: [{ type: 'paragraph', attrs: { id: `b-${slug}` } }] },
  ...over,
})

const STUBS = {
  DocEditor: true,
  DocComments: true,
  DocHistory: true,
  DocLinks: true,
  DocTemplates: true,
  DocToc: true,
  EmptyState: true,
  teleport: true,
}

describe('DocumentsView: поздний ответ открытия документа', () => {
  let wrapper, router, wsStore

  beforeEach(async () => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    docsApi.list.mockResolvedValue({ data: [doc('a'), doc('b')] })
    docsApi.converterStatus.mockResolvedValue({ data: { available: false } })
    wsStore = useWorkspacesStore()
    wsStore.currentId = 'ws-1'
    wsStore.list = [{ id: 'ws-1', name: 'Первое' }]
    router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/', redirect: '/documents' },
        { path: '/documents/:slug?', name: 'documents', component: { template: '<div/>' } },
      ],
    })
  })

  afterEach(() => {
    wrapper?.unmount()
    wrapper = null
  })

  async function mountAt(path = '/documents') {
    await router.push(path)
    await router.isReady()
    wrapper = mount(DocumentsView, {
      global: { plugins: [router], stubs: STUBS },
      attachTo: document.body,
    })
    await flushPromises()
    return wrapper.vm
  }

  const openedId = (vm) => vm.selected?.id ?? null

  // The plain race: two tiles clicked in a row. The first answer is the slower
  // one, and it must not take the screen back from the document the reader chose.
  it('поздний ответ первого документа не подменяет открытый второй', async () => {
    const vm = await mountAt()
    const slow = deferred()
    docsApi.get.mockImplementation((id) =>
      id === 'id-a' ? slow.promise : Promise.resolve({ data: doc('b') }),
    )

    // The first request has to be in flight before the second is asked for —
    // that is the race: two answers outstanding, not two calls in one tick.
    const first = vm.open(doc('a'))
    await flushPromises()
    const second = vm.open(doc('b'))
    await flushPromises()
    expect(openedId(vm)).toBe('id-b')

    slow.resolve({ data: doc('a') })
    await Promise.all([first, second])
    await flushPromises()

    expect(openedId(vm)).toBe('id-b')
    expect(vm.title).toBe('Документ b')
  })

  // The same race seen from the address bar. Canonicalization rewrites the URL,
  // so a stale answer does not just repaint — it navigates.
  it('поздний ответ не уводит адресную строку на покинутый документ', async () => {
    const vm = await mountAt()
    const slow = deferred()
    docsApi.get.mockImplementation((id) =>
      id === 'id-a' ? slow.promise : Promise.resolve({ data: doc('b') }),
    )

    // The first request has to be in flight before the second is asked for —
    // that is the race: two answers outstanding, not two calls in one tick.
    const first = vm.open(doc('a'))
    await flushPromises()
    const second = vm.open(doc('b'))
    await flushPromises()

    slow.resolve({ data: doc('a') })
    await Promise.all([first, second])
    await flushPromises()

    expect(router.currentRoute.value.params.slug).toBe('b')
  })

  // The collaboration room is a subscription, not a render: a stale open joins the
  // abandoned document's room and holds locks there for a reader who has left it.
  it('поздний ответ не подключает комнату покинутого документа', async () => {
    const vm = await mountAt()
    const slow = deferred()
    docsApi.get.mockImplementation((id) =>
      id === 'id-a' ? slow.promise : Promise.resolve({ data: doc('b') }),
    )

    // The first request has to be in flight before the second is asked for —
    // that is the race: two answers outstanding, not two calls in one tick.
    const first = vm.open(doc('a'))
    await flushPromises()
    const second = vm.open(doc('b'))
    await flushPromises()
    room.open.mockClear()

    slow.resolve({ data: doc('a') })
    await Promise.all([first, second])
    await flushPromises()

    expect(room.open).not.toHaveBeenCalledWith('id-a')
  })

  // Leaving is a question too. A load that lands after the reader went back to the
  // grid must not re-open the document they closed.
  it('поздний ответ не возвращает читателя в закрытый документ', async () => {
    const vm = await mountAt()
    const slow = deferred()
    docsApi.get.mockImplementation(() => slow.promise)

    const opening = vm.open(doc('a'))
    await flushPromises()
    await vm.backToGrid()
    await flushPromises()

    slow.resolve({ data: doc('a') })
    await opening
    await flushPromises()

    expect(openedId(vm)).toBe(null)
    expect(vm.content).toBe(null)
    expect(router.currentRoute.value.params.slug).toBeFalsy()
  })

  // The deep-link resolver walks every workspace, so its stale answer moves the
  // app's scope as well as the screen.
  it('поздний резолв слага не подменяет документ и не уводит пространство', async () => {
    wsStore.list = [
      { id: 'ws-1', name: 'Первое' },
      { id: 'ws-2', name: 'Второе' },
    ]
    const vm = await mountAt()
    const selectWorkspace = vi.spyOn(wsStore, 'selectWorkspace').mockResolvedValue()

    const slow = deferred()
    docsApi.bySlug.mockImplementation((_ws, slug) =>
      slug === 'a' ? slow.promise : Promise.resolve({ data: doc('b') }),
    )

    const first = vm.resolveSlug('a')
    await flushPromises()
    const second = vm.resolveSlug('b')
    await flushPromises()
    expect(openedId(vm)).toBe('id-b')

    slow.resolve({ data: doc('a', { workspace_id: 'ws-2' }) })
    await Promise.all([first, second])
    await flushPromises()

    expect(openedId(vm)).toBe('id-b')
    expect(selectWorkspace).not.toHaveBeenCalledWith('ws-2')
  })

  // A 404 for the abandoned slug must not blank the document that answered in the
  // meantime — the failure belongs to a question nobody is asking any more.
  it('ошибка покинутого запроса не гасит открывшийся документ', async () => {
    const vm = await mountAt()
    const slow = deferred()
    docsApi.get.mockImplementation((id) =>
      id === 'id-a' ? slow.promise : Promise.resolve({ data: doc('b') }),
    )

    // The first request has to be in flight before the second is asked for —
    // that is the race: two answers outstanding, not two calls in one tick.
    const first = vm.open(doc('a'))
    await flushPromises()
    const second = vm.open(doc('b'))
    await flushPromises()
    toast.error.mockClear()

    slow.reject(new Error('не найден'))
    await Promise.all([first, second])
    await flushPromises()

    expect(openedId(vm)).toBe('id-b')
    expect(vm.loading).toBe(false)
    expect(toast.error).not.toHaveBeenCalled()
  })

  // The grid is scoped too: a listing requested for the space the reader left must
  // not fill the tiles of the one they are in now.
  it('поздний список покинутого пространства не наполняет сетку', async () => {
    const vm = await mountAt()
    const slow = deferred()
    docsApi.list.mockImplementation((ws) =>
      ws === 'ws-1' ? slow.promise : Promise.resolve({ data: [doc('new')] }),
    )

    const first = vm.loadList()
    wsStore.currentId = 'ws-2'
    const second = vm.loadList()
    await flushPromises()

    slow.resolve({ data: [doc('old')] })
    await Promise.all([first, second])
    await flushPromises()

    expect(vm.list.map((d) => d.slug)).toEqual(['new'])
  })

  // The guard is a comparison, not a mute: the ordinary path still opens.
  it('обычное открытие документа работает', async () => {
    const vm = await mountAt()
    docsApi.get.mockImplementation(() => Promise.resolve({ data: doc('a') }))

    await vm.open(doc('a'))
    await flushPromises()

    expect(openedId(vm)).toBe('id-a')
    expect(room.open).toHaveBeenCalledWith('id-a')
    expect(router.currentRoute.value.params.slug).toBe('a')
  })
})
