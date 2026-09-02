import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount, flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'

// Conferences section (#2864, subtask #2870): the list, the schedule dialog and
// one conference's lobby.

// useMessage() throws without an <n-message-provider>; stub only that, as the
// other component specs do.
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
  // The embedded room (#2871) reaches for a media token. jsdom has no
  // navigator.mediaDevices, so the transport stops before ever calling it —
  // stubbed anyway so a change in that order fails loudly instead of throwing.
  token: vi.fn(() => Promise.reject(new Error('no media in jsdom'))),
}
// getAccessToken is what useRealtime reads on mount; null keeps the socket from
// ever opening in jsdom (it retries on a timer, which unmount clears).
vi.mock('@/api', () => ({ conferences: api, getAccessToken: () => null }))

const { default: ConferencesView } = await import('@/views/ConferencesView.vue')
const { useWorkspacesStore } = await import('@/stores/workspaces')
const { useAuthStore } = await import('@/stores/auth')

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

const part = (over = {}) => ({
  conference_id: 'c1',
  user_id: 'u1',
  user_name: 'Аня',
  user_email: 'anya@example.com',
  role: 'host',
  invited_at: '2026-09-03T10:00:00Z',
  joined_at: null,
  left_at: null,
  force_muted: false,
  ...over,
})

// A router with the real shape of the section: ONE record carrying an optional
// param. The view reads route.params.id to decide list vs. lobby.
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
  api.list.mockResolvedValue({ data: [conf()] })
  api.participants.mockResolvedValue({ data: [] })
  useWorkspacesStore().currentId = 'ws1'
  useAuthStore().user = { id: 'u1', is_admin: false }
})

afterEach(() => {
  // A wrapper left mounted keeps re-rendering on later mounts and then throws
  // inside the scheduler, which silently drops the render under test.
  while (mounted.length) mounted.pop().unmount()
  document.body.innerHTML = ''
})

describe('conferences list', () => {
  it('lists the workspace conferences with status, time and counts', async () => {
    const { w } = await mountAt('/conferences')
    expect(api.list).toHaveBeenCalledWith('ws1', '')
    const rows = w.findAll('[data-testid="conference-row"]')
    expect(rows).toHaveLength(1)
    const text = rows[0].text()
    expect(text).toContain('Ежедневная летучка')
    expect(text).toContain('Запланирована')
    expect(text).toContain('Приглашено: 3')
    expect(text).toContain('Аня')
  })

  it('describes a finished call by its end and a live one by its start', async () => {
    api.list.mockResolvedValue({
      data: [
        conf({ id: 'c2', status: 'ended', ended_at: '2026-09-01T10:00:00Z' }),
        conf({ id: 'c3', status: 'live', started_at: '2026-09-03T08:00:00Z', active_count: 2 }),
      ],
    })
    const { w } = await mountAt('/conferences')
    const rows = w.findAll('[data-testid="conference-row"]')
    expect(rows[0].text()).toContain('Завершилась')
    expect(rows[1].text()).toContain('Началась')
    // active_count is only shown when somebody is actually in the room.
    expect(rows[1].text()).toContain('В комнате: 2')
    expect(rows[0].text()).not.toContain('В комнате')
  })

  it('refetches with the status filter and shows the filtered empty state', async () => {
    const { w } = await mountAt('/conferences')
    api.list.mockResolvedValue({ data: [] })
    // The radio group is naive-ui's; drive the filter the way the template binds it.
    w.vm.filter = 'live'
    await flushPromises()
    expect(api.list).toHaveBeenLastCalledWith('ws1', 'live')
    expect(w.text()).toContain('В этом фильтре пусто')
  })

  it('says which workspace to pick instead of an empty list with no workspace', async () => {
    useWorkspacesStore().currentId = ''
    api.list.mockResolvedValue({ data: [] })
    const { w } = await mountAt('/conferences')
    expect(api.list).not.toHaveBeenCalled()
    expect(w.text()).toContain('Выберите пространство')
  })
})

describe('scheduling', () => {
  it('sends a null scheduled_at when no time was picked', async () => {
    api.create.mockResolvedValue({ data: conf({ id: 'new1' }) })
    const { w } = await mountAt('/conferences')
    w.vm.openDialog()
    w.vm.dlg.title = '  Ретро  '
    await flushPromises()
    await w.vm.submit()
    expect(api.create).toHaveBeenCalledWith('ws1', {
      title: 'Ретро',
      description: '',
      scheduled_at: null,
    })
  })

  it('refuses an empty title without calling the API', async () => {
    const { w } = await mountAt('/conferences')
    w.vm.openDialog()
    w.vm.dlg.title = '   '
    await w.vm.submit()
    expect(api.create).not.toHaveBeenCalled()
    expect(toast.warning).toHaveBeenCalled()
  })
})

describe('one conference', () => {
  it('opens the lobby from the route param and offers to join', async () => {
    api.get.mockResolvedValue({ data: { conference: conf(), participants: [part()] } })
    const { w } = await mountAt('/conferences/c1')
    expect(api.get).toHaveBeenCalledWith('c1')
    expect(w.find('[data-testid="conference-row"]').exists()).toBe(false)
    expect(w.text()).toContain('Ежедневная летучка')
    expect(w.text()).toContain('Ведущий')
    expect(w.find('[data-testid="conference-join"]').exists()).toBe(true)
    expect(w.find('[data-testid="conference-leave"]').exists()).toBe(false)
  })

  it('swaps join for leave once the server confirms the join', async () => {
    api.get.mockResolvedValue({ data: { conference: conf(), participants: [part()] } })
    const joined = part({ joined_at: '2026-09-03T09:00:00Z' })
    api.join.mockResolvedValue({
      data: { conference: conf({ status: 'live' }), participant: joined },
    })
    api.participants.mockResolvedValue({ data: [joined] })
    const { w } = await mountAt('/conferences/c1')
    await w.vm.join()
    await flushPromises()
    expect(w.find('[data-testid="conference-leave"]').exists()).toBe(true)
    expect(w.find('[data-testid="conference-join"]').exists()).toBe(false)
    // join answers the refreshed conference — the first arrival starts the call.
    expect(w.text()).toContain('Идёт сейчас')
  })

  it('cannot join a conference that has ended', async () => {
    api.get.mockResolvedValue({
      data: {
        conference: conf({ status: 'ended', ended_at: '2026-09-01T10:00:00Z' }),
        participants: [],
      },
    })
    const { w } = await mountAt('/conferences/c1')
    expect(w.find('[data-testid="conference-join"]').attributes('disabled')).toBeDefined()
  })

  it('offers «Завершить» to the creator and withholds it from a plain member', async () => {
    api.get.mockResolvedValue({
      data: { conference: conf({ status: 'live' }), participants: [part({ role: 'member' })] },
    })
    const { w } = await mountAt('/conferences/c1')
    expect(w.vm.canModerate).toBe(true) // created_by === u1

    useAuthStore().user = { id: 'u9', is_admin: false }
    await flushPromises()
    expect(w.vm.canModerate).toBe(false)
  })

  it('reports a deleted conference instead of leaving the previous one on screen', async () => {
    api.get.mockRejectedValue({ response: { status: 404 }, message: 'not found' })
    const { w } = await mountAt('/conferences/c1')
    expect(w.text()).toContain('Конференция не найдена')
    // A 404 is the expected answer for a deleted meeting — not a toast.
    expect(toast.error).not.toHaveBeenCalled()
  })
})

describe('routing', () => {
  it('registers the section as one record with an optional param (#2727)', async () => {
    // Two sibling records (`conferences` + `conferences/:id`) would take the
    // sidebar item dark the moment a conference is opened, because vue-router
    // marks a link active only when the open route shares its record.
    const { default: router } = await import('@/router')
    const paths = router
      .getRoutes()
      .map((r) => r.path)
      .filter((p) => p.includes('conferences'))
    expect(paths).toEqual(['/conferences/:id?'])
  })
})
