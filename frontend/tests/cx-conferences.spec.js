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
  // The recordings panel under the room (#2877); empty here, it has its own spec.
  recordings: vi.fn(() => Promise.resolve({ data: [] })),
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
  // Membership can have started the shared session (#2888); it outlives the view
  // by design, so stop it here to clear its reconnect timer between tests.
  useConferenceSession().stop()
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
      // The retention the dialog defaults to (#2877) — sent explicitly so the
      // number the user saw is the number that applies.
      recording_ttl_days: 30,
    })
  })

  it('falls back to the default retention when the field is cleared', async () => {
    // n-input-number answers null for an empty box. Sending that through as 0
    // would read as «хранить вечно» — a decision an empty field must not make.
    api.create.mockResolvedValue({ data: conf({ id: 'new2' }) })
    const { w } = await mountAt('/conferences')
    w.vm.openDialog()
    w.vm.dlg.title = 'Ретро'
    w.vm.dlg.ttl = null
    await flushPromises()
    await w.vm.submit()
    expect(api.create.mock.calls[0][1].recording_ttl_days).toBe(30)
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
    // The roster now lives in the room's own «В комнате» rail (#2881), not a
    // separate column, so the lobby offers to join rather than listing roles.
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

  it('lets you rejoin an ended conference — the room is reusable (#2879)', async () => {
    api.get.mockResolvedValue({
      data: {
        conference: conf({ status: 'ended', ended_at: '2026-09-01T10:00:00Z' }),
        participants: [],
      },
    })
    const { w } = await mountAt('/conferences/c1')
    const join = w.find('[data-testid="conference-join"]')
    expect(join.exists()).toBe(true)
    // Enabled: a daily standup is started again by joining, not blocked as finished.
    expect(join.attributes('disabled')).toBeUndefined()
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

  // The cold entry — a full page load straight onto a room, which is what the
  // link in an invitation does. Every other test here mounts the view on its
  // own, so none of them ever reproduced it (#2876).
  //
  // The shell renders the topbar slots the room's controls teleport into, and
  // it renders them in the SAME pass as the view. A <teleport> looks its target
  // up once, while rendering — and at that moment the shell's tree is still
  // being built in memory, so `document.getElementById` cannot see it and the
  // teleport binds to null. One tick later `onMounted` finds the slot for real,
  // `inTopbar` flips true, and Vue moves the teleport's children into that null:
  // «Cannot read properties of null (reading 'insertBefore')», thrown from
  // inside the patch. The render tree dies there and everything after it stops
  // updating — the symptom users see is a room that no longer follows the theme.
  it('survives being the first thing rendered under the topbar slots (#2876)', async () => {
    api.get.mockResolvedValue({ data: { conference: conf(), participants: [part()] } })
    const router = makeRouter()
    router.push('/conferences/c1')
    await router.isReady()

    const Shell = {
      components: { ConferencesView },
      template: `<div>
        <div id="tb-slot-left" /><div id="tb-slot-right" />
        <ConferencesView />
      </div>`,
    }
    // attachTo is the whole point: the slots have to be real document nodes by
    // the time onMounted runs, and absent from it while the view renders.
    const w = mount(Shell, { attachTo: document.body, global: { plugins: [router] } })
    mounted.push(w)
    await flushPromises()

    // Rendered, and rendered into the topbar rather than inline.
    expect(document.getElementById('tb-slot-left').textContent).toContain('К списку')
    expect(w.text()).toContain('Ежедневная летучка')
  })
})
