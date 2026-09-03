import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount, flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'

// The shared conference session lifted above the router (#2888): the store that
// keeps a call alive across route changes, plus the floating mini-window that is
// the call while it is minimised.

const api = {
  get: vi.fn(),
  leave: vi.fn(() => Promise.resolve({ data: {} })),
  token: vi.fn(() => Promise.reject(new Error('no media in jsdom'))),
}
vi.mock('@/api', () => ({
  conferences: api,
  getAccessToken: () => 'jwt',
  apiBaseURL: () => '/api',
}))

// A stand-in room socket, so start()/stop() can be observed opening and closing it
// without a real server. No livekit is needed: with no navigator.mediaDevices the
// transport stops at UNAVAILABLE before it would ever import the SDK.
const sockets = []
class FakeSocket {
  static OPEN = 1
  constructor(url, protocols) {
    this.url = url
    this.protocols = protocols
    this.readyState = FakeSocket.OPEN
    this.sent = []
    this.closed = false
    sockets.push(this)
  }
  send(raw) {
    this.sent.push(JSON.parse(raw))
  }
  close() {
    this.readyState = 3
    this.closed = true
    this.onclose?.()
  }
  deliver(msg) {
    this.onmessage?.({ data: JSON.stringify(msg) })
  }
}

const { useConferenceSession } = await import('@/stores/conference')
const { default: ConferenceMiniWindow } = await import(
  '@/components/conference/ConferenceMiniWindow.vue'
)
const { useAuthStore } = await import('@/stores/auth')

beforeEach(() => {
  setActivePinia(createPinia())
  sockets.length = 0
  sessionStorage.clear()
  localStorage.clear()
  for (const fn of Object.values(api)) fn.mockClear()
  vi.stubGlobal('WebSocket', FakeSocket)
  // No secure-context media: the transport reports UNAVAILABLE and never touches
  // the SFU, which is exactly what we want for testing the session bookkeeping.
  if ('mediaDevices' in navigator) delete navigator.mediaDevices
})
afterEach(() => {
  useConferenceSession().stop()
  vi.unstubAllGlobals()
})

describe('useConferenceSession — lifecycle', () => {
  it('starts a call: sets the active id, remembers it, and opens the room socket', () => {
    const s = useConferenceSession()
    s.start('c1', 'Летучка')
    expect(s.activeId).toBe('c1')
    expect(s.title).toBe('Летучка')
    expect(s.active).toBe(true)
    expect(sessionStorage.getItem('tessera_conf_active')).toBe('c1')
    // The room socket is opened for the conference.
    expect(sockets.at(-1)?.url).toContain('/conferences/c1/ws')
  })

  it('is idempotent for the same call and only refreshes the title', () => {
    const s = useConferenceSession()
    s.start('c1', 'Летучка')
    const opened = sockets.length
    s.start('c1', 'Летучка (обновлённая)')
    expect(sockets.length).toBe(opened) // no reconnect
    expect(s.title).toBe('Летучка (обновлённая)')
  })

  it('switching to another call drops the first', () => {
    const s = useConferenceSession()
    s.start('c1', 'A')
    const first = sockets.at(-1)
    s.start('c2', 'B')
    expect(first.closed).toBe(true)
    expect(s.activeId).toBe('c2')
    expect(sockets.at(-1).url).toContain('/conferences/c2/ws')
  })

  it('stop clears the session and forgets it, without hitting the leave endpoint', async () => {
    const s = useConferenceSession()
    s.start('c1', 'A')
    await s.stop()
    expect(s.active).toBe(false)
    expect(s.activeId).toBe('')
    expect(sessionStorage.getItem('tessera_conf_active')).toBe(null)
    expect(api.leave).not.toHaveBeenCalled()
  })

  it('hangup drops the server seat, then tears the session down', async () => {
    const s = useConferenceSession()
    s.start('c1', 'A')
    await s.hangup()
    expect(api.leave).toHaveBeenCalledWith('c1')
    expect(s.active).toBe(false)
    expect(sessionStorage.getItem('tessera_conf_active')).toBe(null)
  })
})

describe('useConferenceSession — restore after reload', () => {
  it('walks back into a still-live call remembered in sessionStorage', async () => {
    sessionStorage.setItem('tessera_conf_active', 'c9')
    api.get.mockResolvedValue({ data: { conference: { id: 'c9', title: 'Ретро', status: 'live' } } })
    const s = useConferenceSession()
    await s.restore()
    expect(api.get).toHaveBeenCalledWith('c9')
    expect(s.activeId).toBe('c9')
    expect(s.title).toBe('Ретро')
  })

  it('forgets a call that has since ended rather than rejoining it', async () => {
    sessionStorage.setItem('tessera_conf_active', 'c9')
    api.get.mockResolvedValue({ data: { conference: { id: 'c9', title: 'Ретро', status: 'ended' } } })
    const s = useConferenceSession()
    await s.restore()
    expect(s.active).toBe(false)
    expect(sessionStorage.getItem('tessera_conf_active')).toBe(null)
  })

  it('forgets a deleted call (404) instead of looping against it', async () => {
    sessionStorage.setItem('tessera_conf_active', 'c9')
    api.get.mockRejectedValue({ response: { status: 404 } })
    const s = useConferenceSession()
    await s.restore()
    expect(s.active).toBe(false)
    expect(sessionStorage.getItem('tessera_conf_active')).toBe(null)
  })
})

describe('ConferenceMiniWindow', () => {
  function makeRouter(start = '/tasks') {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/tasks', component: { template: '<div />' } },
        { path: '/conferences/:id?', component: { template: '<div />' } },
      ],
    })
    router.push(start)
    return router
  }

  async function mountMini(start = '/tasks') {
    const router = makeRouter(start)
    await router.isReady()
    useAuthStore().user = { id: 'u1' }
    const w = mount(ConferenceMiniWindow, { global: { plugins: [router] } })
    await flushPromises()
    return { w, router }
  }

  it('paints nothing without a live session', async () => {
    const { w } = await mountMini()
    expect(w.find('[data-testid="conference-mini"]').exists()).toBe(false)
  })

  it('appears once a call is live and the open route is not its room', async () => {
    const s = useConferenceSession()
    s.start('c1', 'Летучка')
    const { w } = await mountMini('/tasks')
    expect(w.find('[data-testid="conference-mini"]').exists()).toBe(true)
  })

  it('stays hidden on the call’s own room route', async () => {
    const s = useConferenceSession()
    s.start('c1', 'Летучка')
    const { w } = await mountMini('/conferences/c1')
    expect(w.find('[data-testid="conference-mini"]').exists()).toBe(false)
  })

  it('expands back to the full room', async () => {
    const s = useConferenceSession()
    s.start('c1', 'Летучка')
    const { w, router } = await mountMini('/tasks')
    await w.find('[data-testid="conference-mini-expand"]').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/conferences/c1')
  })

  it('hangs up the whole session from the mini toolbar', async () => {
    const s = useConferenceSession()
    s.start('c1', 'Летучка')
    const { w } = await mountMini('/tasks')
    await w.find('[data-testid="conference-mini-hangup"]').trigger('click')
    await flushPromises()
    expect(api.leave).toHaveBeenCalledWith('c1')
  })
})
