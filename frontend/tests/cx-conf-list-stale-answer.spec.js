import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount, flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'

// An answer that arrives out of order in the «Созвоны» section (#2932).
//
// Four things ask ConferencesView for the list — the mount, the status radio, the
// workspace switcher and the socket — and one more asks for a lobby. None of them
// were held apart, so what ended up on screen was decided by ARRIVAL order, not by
// what the reader last asked for: a slow "все" page landing after a fast "ended"
// one leaves live calls under the «Завершённые» radio, with nothing left to
// refetch them.
//
// The lobby is worse than cosmetic: on the way in it calls `ensureWorkspace` with
// the answer's own workspace (a call opened from an invitation may live outside
// the remembered one), so a late reply about the call the reader has left drags
// the entire app into that call's workspace.
//
// These tests pin the comparison-after-await for both. Android has done it since
// the rail was written; the web side had nothing.

const toast = { error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }
vi.mock('naive-ui', async () => {
  const actual = await vi.importActual('naive-ui')
  return { ...actual, useMessage: () => toast }
})

const api = {
  list: vi.fn(),
  create: vi.fn(),
  get: vi.fn(),
  join: vi.fn(),
  leave: vi.fn(),
  end: vi.fn(),
  remove: vi.fn(),
  participants: vi.fn(),
  recordings: vi.fn(() => Promise.resolve({ data: [] })),
  token: vi.fn(() => Promise.reject(new Error('no media in jsdom'))),
}
const wsApi = {
  members: vi.fn(() => Promise.resolve({ data: [] })),
  groups: vi.fn(() => Promise.resolve({ data: [] })),
  projects: vi.fn(() => Promise.resolve({ data: [] })),
  commands: vi.fn(() => Promise.resolve({ data: { builtin: [], custom: [] } })),
}
vi.mock('@/api', () => ({ conferences: api, workspaces: wsApi, getAccessToken: () => null }))

const { default: ConferencesView } = await import('@/views/ConferencesView.vue')
const { useWorkspacesStore } = await import('@/stores/workspaces')
const { useAuthStore } = await import('@/stores/auth')
const { useConferenceSession } = await import('@/stores/conference')

const conf = (over = {}) => ({
  id: 'c1',
  workspace_id: 'ws1',
  title: 'Ежедневная летучка',
  description: '',
  status: 'scheduled',
  scheduled_at: '2026-09-04T09:00:00Z',
  started_at: null,
  ended_at: null,
  created_by: 'u1',
  created_by_name: 'Аня',
  participant_count: 3,
  active_count: 0,
  ...over,
})

/** A promise plus the handles that settle it, so a page can be held in flight. */
function deferred() {
  let resolve, reject
  const promise = new Promise((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

function makeRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/conferences/:id?', component: ConferencesView },
      { path: '/:rest(.*)*', component: { template: '<div />' } },
    ],
  })
}

const mounted = []
async function mountAt(path) {
  const router = makeRouter()
  router.push(path)
  await router.isReady()
  const w = mount(ConferencesView, { global: { plugins: [router] } })
  mounted.push(w)
  await flushPromises()
  return { w, router }
}

beforeEach(() => {
  setActivePinia(createPinia())
  localStorage.clear()
  for (const fn of Object.values(api)) fn.mockReset()
  for (const fn of Object.values(toast)) fn.mockReset()
  api.list.mockResolvedValue({ data: [] })
  api.participants.mockResolvedValue({ data: [] })
  api.recordings.mockResolvedValue({ data: [] })
  api.token.mockRejectedValue(new Error('no media in jsdom'))
  useWorkspacesStore().currentId = 'ws1'
  useAuthStore().user = { id: 'u1', is_admin: false }
})

afterEach(() => {
  while (mounted.length) mounted.pop().unmount()
  useConferenceSession().stop()
  document.body.innerHTML = ''
})

describe('the list ignores an answer it has stopped waiting for', () => {
  it('keeps the newest filter when the previous filter answers last', async () => {
    const { w } = await mountAt('/conferences')

    // «Живые» — held in flight.
    const live = deferred()
    api.list.mockReturnValueOnce(live.promise)
    w.vm.filter = 'live'
    await flushPromises()

    // «Завершённые» — answers first.
    api.list.mockResolvedValueOnce({
      data: [conf({ id: 'c9', title: 'Разбор инцидента', status: 'ended' })],
    })
    w.vm.filter = 'ended'
    await flushPromises()
    expect(w.text()).toContain('Разбор инцидента')

    // The «Живые» page finally lands. It describes a filter nobody is on.
    live.resolve({ data: [conf({ id: 'c8', title: 'Идущий созвон', status: 'live' })] })
    await flushPromises()

    expect(w.vm.filter).toBe('ended')
    expect(w.text()).toContain('Разбор инцидента')
    expect(w.text()).not.toContain('Идущий созвон')
  })

  it('keeps the new workspace when the previous workspace answers last', async () => {
    const store = useWorkspacesStore()
    const { w } = await mountAt('/conferences')

    // A page asked of ws1, still out when the reader switches away.
    const oldWs = deferred()
    api.list.mockReturnValueOnce(oldWs.promise)
    w.vm.filter = 'live'
    await flushPromises()
    expect(api.list).toHaveBeenLastCalledWith('ws1', 'live')

    // The switch re-asks, against the new workspace; that answer is the one
    // the reader is owed.
    api.list.mockResolvedValueOnce({
      data: [conf({ id: 'c7', workspace_id: 'ws2', title: 'Созвон второго пространства' })],
    })
    store.currentId = 'ws2'
    await flushPromises()

    oldWs.resolve({ data: [conf({ id: 'c6', title: 'Созвон первого пространства' })] })
    await flushPromises()

    expect(w.text()).toContain('Созвон второго пространства')
    expect(w.text()).not.toContain('Созвон первого пространства')
  })

  it('does not blame the current filter for a superseded request failing', async () => {
    const { w } = await mountAt('/conferences')

    const live = deferred()
    api.list.mockReturnValueOnce(live.promise)
    w.vm.filter = 'live'
    await flushPromises()

    api.list.mockResolvedValueOnce({ data: [conf({ id: 'c9', status: 'ended' })] })
    w.vm.filter = 'ended'
    await flushPromises()

    live.reject(Object.assign(new Error('Сеть недоступна'), { status: 0 }))
    await flushPromises()

    expect(toast.error).not.toHaveBeenCalled()
    // The spinner belongs to the request that lit it, so a stale rejection must
    // not leave the list looking settled while the real answer is still out —
    // and here the real answer has landed, so it must be off.
    expect(w.vm.loading).toBe(false)
  })

  it('clears the list without letting the in-flight page refill it', async () => {
    const store = useWorkspacesStore()
    const { w } = await mountAt('/conferences')

    const pending = deferred()
    api.list.mockReturnValueOnce(pending.promise)
    w.vm.filter = 'live'
    await flushPromises()

    // No workspace: the view clears the list and asks nothing.
    store.currentId = ''
    await flushPromises()

    pending.resolve({ data: [conf({ id: 'c5', title: 'Созвон ниоткуда' })] })
    await flushPromises()

    expect(w.text()).not.toContain('Созвон ниоткуда')
    expect(w.text()).toContain('Выберите пространство')
  })
})

describe('the lobby ignores an answer it has stopped waiting for', () => {
  it('does not draw the previous call over the one that is open', async () => {
    const first = deferred()
    api.get.mockReturnValueOnce(first.promise)
    const { router, w } = await mountAt('/conferences/c1')

    api.get.mockResolvedValueOnce({
      data: {
        conference: conf({ id: 'c2', title: 'Вторая летучка', workspace_id: 'ws1' }),
        participants: [],
      },
    })
    await router.push('/conferences/c2')
    await flushPromises()
    expect(w.text()).toContain('Вторая летучка')

    first.resolve({
      data: {
        conference: conf({ id: 'c1', title: 'Первая летучка', workspace_id: 'ws1' }),
        participants: [],
      },
    })
    await flushPromises()

    expect(w.text()).toContain('Вторая летучка')
    expect(w.text()).not.toContain('Первая летучка')
  })

  it('does not drag the app into the workspace of the call the reader left', async () => {
    const store = useWorkspacesStore()
    const ensure = vi.spyOn(store, 'ensureWorkspace').mockResolvedValue(undefined)

    const first = deferred()
    api.get.mockReturnValueOnce(first.promise)
    const { router } = await mountAt('/conferences/c1')

    api.get.mockResolvedValueOnce({
      data: {
        conference: conf({ id: 'c2', title: 'Вторая летучка', workspace_id: 'ws1' }),
        participants: [],
      },
    })
    await router.push('/conferences/c2')
    await flushPromises()
    ensure.mockClear()

    // The abandoned call lives somewhere else entirely.
    first.resolve({
      data: {
        conference: conf({ id: 'c1', title: 'Первая летучка', workspace_id: 'ws-elsewhere' }),
        participants: [],
      },
    })
    await flushPromises()

    expect(ensure).not.toHaveBeenCalled()
  })

  it('does not reopen a lobby over the list after going back', async () => {
    const pending = deferred()
    api.get.mockReturnValueOnce(pending.promise)
    const { router, w } = await mountAt('/conferences/c1')

    api.list.mockResolvedValueOnce({ data: [conf({ id: 'c3', title: 'Строка списка' })] })
    await router.push('/conferences')
    await flushPromises()

    pending.resolve({
      data: {
        conference: conf({ id: 'c1', title: 'Первая летучка', workspace_id: 'ws1' }),
        participants: [],
      },
    })
    await flushPromises()

    expect(w.text()).not.toContain('Первая летучка')
    // The list is what the route says, so the assertion above holds either way;
    // what the guard decides is whether the lobby state came back behind it —
    // one «Назад→Открыть» later, that is what the reader would have seen.
    expect(w.vm.detail).toBeNull()
    expect(w.vm.detailLoading).toBe(false)
  })
})
