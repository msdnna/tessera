import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'
import { createRouter, createMemoryHistory } from 'vue-router'
import { nextTick } from 'vue'

// Server-side recording, the client half (#2877, parent #2864).
//
// The invariant this file exists to hold: **the red dot is not the button's**.
// Whoever pressed record learns nothing the room does not learn at the same
// moment, through the same snapshot — because the people who did NOT press it
// are precisely the ones the indicator is for. Every test below is a way for the
// button and the dot to come apart: a start that fails, a stop mid-flight, a
// socket that drops, a call that is left. In each one the question is the same —
// does the screen still say what the server says?

// ── fake SDK ─────────────────────────────────────────────────────────────
// No media at all: the transport reports UNAVAILABLE and never reaches the SFU,
// which is what we want — recording is bookkeeping, not pixels.
const lk = vi.hoisted(() => {
  const RoomEvent = {
    ParticipantConnected: 'participantConnected',
    TrackSubscribed: 'trackSubscribed',
    LocalTrackPublished: 'localTrackPublished',
    LocalTrackUnpublished: 'localTrackUnpublished',
    ConnectionStateChanged: 'connectionState',
    Disconnected: 'disconnected',
  }
  class Room {
    constructor() {
      this.handlers = {}
      this.remoteParticipants = new Map()
      this.canPlaybackAudio = true
    }
    on(event, fn) {
      ;(this.handlers[event] ||= []).push(fn)
      return this
    }
    async connect() {}
    async disconnect() {}
    static async getLocalDevices() {
      return []
    }
  }
  return { RoomEvent, Room }
})
vi.mock('livekit-client', () => ({ Room: lk.Room, RoomEvent: lk.RoomEvent }))

const api = {
  token: vi.fn(() => Promise.reject(new Error('no media in jsdom'))),
  startRecording: vi.fn(() => Promise.resolve({ data: {} })),
  stopRecording: vi.fn(() => Promise.resolve({ data: {} })),
  recordings: vi.fn(() => Promise.resolve({ data: [] })),
  recording: vi.fn(),
  removeRecording: vi.fn(() => Promise.resolve({ data: {} })),
}
vi.mock('@/api', () => ({
  conferences: api,
  getAccessToken: () => 'jwt',
  apiBaseURL: () => '/api',
}))

// ── fake room socket ─────────────────────────────────────────────────────
const sockets = []
class FakeSocket {
  static OPEN = 1
  constructor(url, protocols) {
    this.url = url
    this.protocols = protocols
    this.readyState = FakeSocket.OPEN
    this.sent = []
    sockets.push(this)
  }
  send(raw) {
    this.sent.push(JSON.parse(raw))
  }
  close() {
    this.readyState = 3
    this.onclose?.()
  }
  drop() {
    // A transport failure, not a deliberate close: what a backend restart or a
    // laptop lid looks like from here.
    this.readyState = 3
    this.onclose?.()
  }
  deliver(msg) {
    this.onmessage?.({ data: JSON.stringify(msg) })
  }
}

const { useConfRoom } = await import('@/composables/useConfRoom')
const { useConferenceSession } = await import('@/stores/conference')
const { useAuthStore } = await import('@/stores/auth')
const { default: ConferenceRoom } = await import('@/components/conference/ConferenceRoom.vue')
const { default: ConferenceRecordings } =
  await import('@/components/conference/ConferenceRecordings.vue')
const { default: ConferenceMiniWindow } =
  await import('@/components/conference/ConferenceMiniWindow.vue')

const REC = { id: 'r1', started_at: '2026-09-04T10:00:00Z', started_by: 'Аня' }

beforeEach(() => {
  setActivePinia(createPinia())
  sockets.length = 0
  sessionStorage.clear()
  localStorage.clear()
  for (const fn of Object.values(api)) fn.mockClear()
  api.startRecording.mockResolvedValue({ data: {} })
  api.stopRecording.mockResolvedValue({ data: {} })
  api.recordings.mockResolvedValue({ data: [] })
  api.removeRecording.mockResolvedValue({ data: {} })
  vi.stubGlobal('WebSocket', FakeSocket)
  // A secure context with devices: without it ConferenceRoom renders only its
  // «браузер не даёт доступ» notice and the toolbar — the button under test —
  // never exists. The fake SDK above is what keeps that from reaching a real SFU.
  api.token.mockResolvedValue({ data: { url: 'wss://x/livekit', token: 'jwt', expires_in: 900 } })
  Object.defineProperty(navigator, 'mediaDevices', {
    value: { getUserMedia: vi.fn() },
    configurable: true,
    writable: true,
  })
})
afterEach(async () => {
  // Awaited, unlike the older conference specs. `stop()` is async — it leaves the
  // SFU — and letting it settle after the hook returns means the previous test's
  // teardown lands in the middle of the next one, resetting the transport to
  // "no media" while a freshly mounted room is being asserted on. That showed up
  // as one test in three going red with no change to the code under test.
  await useConferenceSession().stop()
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

describe('useConfRoom — where the red dot comes from', () => {
  // Every room opened here is closed again, and that is not tidiness. A dropped
  // socket schedules a reconnect on a real timer with a random delay, so a room
  // left open in one test opens a *second* socket somewhere inside a later one —
  // which then hands `sockets.at(-1)` the stray instead of the socket under
  // test. That was one test in three going red, a different one each run.
  const opened = []
  afterEach(() => {
    for (const r of opened.splice(0)) r.close()
  })

  function open() {
    const room = useConfRoom()
    opened.push(room)
    room.open('c1')
    const ws = sockets.at(-1)
    ws.onopen()
    ws.deliver({ type: 'welcome', conn_id: 'c-1', user_id: 'me', role: 'member' })
    return { room, ws }
  }
  const snapshot = (over = {}) => ({
    type: 'state',
    participants: [],
    stage: null,
    queue: [],
    ...over,
  })

  it('reads the recording out of the snapshot, and lets the snapshot take it away', () => {
    const { room, ws } = open()
    expect(room.recording.value).toBeNull()

    ws.deliver(snapshot({ recording: REC }))
    expect(room.recording.value.started_by).toBe('Аня')

    // A snapshot without one is the server saying it stopped — the only thing
    // that may clear the dot in the ordinary course of a call.
    ws.deliver(snapshot())
    expect(room.recording.value).toBeNull()
  })

  it('keeps the dot lit when the socket drops, because the recording did not stop', () => {
    const { room, ws } = open()
    ws.deliver(snapshot({ recording: REC }))
    ws.drop()

    // The roster is cleared — a stale one would offer a mute button for someone
    // who may have left. The dot is not: this socket carries bookkeeping, the
    // media goes to the SFU on its own connection, and losing this one stops us
    // hearing about the recording rather than stopping the recording. Clearing
    // it would tell the room it is off the record when it is not.
    expect(room.participants.value).toEqual([])
    expect(room.recording.value).not.toBeNull()
  })

  it('drops the dot when we leave the call, so it cannot follow us into the next one', () => {
    const { room, ws } = open()
    ws.deliver(snapshot({ recording: REC }))
    room.close()
    expect(room.recording.value).toBeNull()
  })
})

describe('ConferenceRoom — the button and the dot', () => {
  async function room({ role = 'member', canModerate = false } = {}) {
    useConferenceSession().start('c1', 'Тест')
    // Let the session's own join settle before mounting. `start()` kicks off an
    // async SFU join; a continuation of it landing mid-assertion re-renders the
    // room from under the test, and the toolbar this describe is about lives
    // behind that state. Settling it here is what makes the mount deterministic.
    await flushPromises()
    const w = mount(ConferenceRoom, {
      props: { conferenceId: 'c1', active: true, ended: false },
      global: {
        stubs: {
          ConferenceChat: { template: '<div/>' },
          ParticipantsPanel: { template: '<div/>' },
        },
      },
    })
    await flushPromises()
    const ws = sockets.at(-1)
    ws.onopen()
    ws.deliver({
      type: 'welcome',
      conn_id: 'c-1',
      user_id: 'me',
      role,
      can_moderate: canModerate,
      stage_ttl_ms: 3000,
    })
    await flushPromises()
    return { w, ws }
  }
  const snapshot = (over = {}) => ({
    type: 'state',
    participants: [{ user_id: 'me', name: 'Я', role: 'member', mic: false, cam: false }],
    stage: null,
    queue: [],
    ...over,
  })

  it('shows the indicator to a plain participant, who has no button to press', async () => {
    const { w, ws } = await room()
    ws.deliver(snapshot({ recording: REC }))
    await nextTick()

    // This is the whole feature: being recorded is told to the room, not to the
    // room's moderators.
    const dot = w.find('[data-testid="conference-recording-dot"]')
    expect(dot.exists()).toBe(true)
    expect(dot.text()).toContain('Аня')
    expect(w.find('[data-testid="conference-record"]').exists()).toBe(false)
    w.unmount()
  })

  it('gives the button to a moderator only', async () => {
    const { w } = await room({ canModerate: true })
    expect(w.find('[data-testid="conference-record"]').exists()).toBe(true)
    w.unmount()
  })

  it('does not light the dot on its own click — it waits for the server to say so', async () => {
    const { w, ws } = await room({ canModerate: true })
    await w.find('[data-testid="conference-record"]').trigger('click')
    await flushPromises()

    expect(api.startRecording).toHaveBeenCalledWith('c1')
    // The HTTP call returned, and still nothing claims to be recording: the
    // egress worker may yet fail to start, and a screen that said "идёт запись"
    // here would be lying to the people who trust it most.
    expect(w.find('[data-testid="conference-recording-dot"]').exists()).toBe(false)

    ws.deliver(snapshot({ recording: REC }))
    await nextTick()
    expect(w.find('[data-testid="conference-recording-dot"]').exists()).toBe(true)
    w.unmount()
  })

  it('stops what the snapshot says is running, not what it started itself', async () => {
    const { w, ws } = await room({ canModerate: true })
    // The recording was started by somebody else, in another tab. The button is
    // still a stop button, because it acts on the call rather than on this
    // client's memory of what it did.
    ws.deliver(snapshot({ recording: { ...REC, started_by: 'Борис' } }))
    await nextTick()

    await w.find('[data-testid="conference-record"]').trigger('click')
    await flushPromises()
    expect(api.stopRecording).toHaveBeenCalledWith('c1')
    expect(api.startRecording).not.toHaveBeenCalled()
    w.unmount()
  })

  it('prints the server refusal verbatim instead of a shrug', async () => {
    // What a server without the recording profile running actually answers.
    api.startRecording.mockRejectedValue({
      response: { data: { error: 'recording is not configured on this server' } },
    })
    const { w } = await room({ canModerate: true })
    await w.find('[data-testid="conference-record"]').trigger('click')
    await flushPromises()

    const err = w.find('[data-testid="conference-recording-error"]')
    expect(err.exists()).toBe(true)
    expect(err.text()).toContain('not configured')
    // And nothing pretends to be recording.
    expect(w.find('[data-testid="conference-recording-dot"]').exists()).toBe(false)
    w.unmount()
  })
})

describe('ConferenceMiniWindow — the dot follows the call out of sight', () => {
  async function mountMini() {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/tasks', component: { template: '<div />' } },
        { path: '/conferences/:id?', component: { template: '<div />' } },
      ],
    })
    router.push('/tasks')
    await router.isReady()
    useAuthStore().user = { id: 'u1' }
    const s = useConferenceSession()
    s.start('c1', 'Летучка')
    const w = mount(ConferenceMiniWindow, { global: { plugins: [router] } })
    await flushPromises()
    return { w, ws: sockets.at(-1) }
  }

  it('carries the indicator into the minimised window', async () => {
    const { w, ws } = await mountMini()
    expect(w.find('[data-testid="conference-mini-recording"]').exists()).toBe(false)

    ws.onopen()
    ws.deliver({ type: 'welcome', conn_id: 'c-1', user_id: 'u1', role: 'member' })
    ws.deliver({
      type: 'state',
      participants: [],
      stage: null,
      queue: [],
      recording: REC,
    })
    await nextTick()

    // Minimised is the state in which a recording is easiest to forget about,
    // which makes this the last place the indicator may be dropped.
    expect(w.find('[data-testid="conference-mini-recording"]').exists()).toBe(true)
    w.unmount()
  })
})

describe('ConferenceRecordings — the list under the room', () => {
  const done = (over = {}) => ({
    id: 'r1',
    status: 'completed',
    error: '',
    file_name: 'rec-2026-09-04-1000.mp4',
    size_bytes: 5 * 1024 * 1024,
    duration_sec: 90,
    started_at: '2026-09-04T10:00:00Z',
    ended_at: '2026-09-04T10:01:30Z',
    expires_at: '2026-10-04T10:01:30Z',
    started_by_name: 'Аня',
    ...over,
  })

  async function panel(rows, props = {}) {
    api.recordings.mockResolvedValue({ data: rows })
    const w = mount(ConferenceRecordings, {
      props: { conferenceId: 'c1', canModerate: false, nudge: 0, ...props },
    })
    await flushPromises()
    return w
  }

  it('renders nothing at all when the call has never been recorded', async () => {
    const w = await panel([])
    // An empty frame under every conference would be a permanent reminder of a
    // feature most calls never use.
    expect(w.find('[data-testid="conference-recordings"]').exists()).toBe(false)
    w.unmount()
  })

  it('shows length, size, who started it and when it self-destructs', async () => {
    const w = await panel([done()])
    const row = w.find('[data-testid="conference-recording-row"]')
    expect(row.text()).toContain('1:30')
    expect(row.text()).toContain('5 MB')
    expect(row.text()).toContain('Аня')
    // The expiry is said out loud: a file that deletes itself on a date is not
    // something to find out about after it is gone.
    expect(row.text()).toContain('удалится')
    w.unmount()
  })

  it('saves the mp4 under its own name, fetched with our credential', async () => {
    const blob = new Blob(['x'], { type: 'video/mp4' })
    api.recording.mockResolvedValue({ data: blob })
    const createURL = vi.fn(() => 'blob:rec')
    vi.stubGlobal('URL', { createObjectURL: createURL, revokeObjectURL: vi.fn() })
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})

    const w = await panel([done()])
    await w.find('[data-testid="conference-recording-download"]').trigger('click')
    await flushPromises()

    // Through the API with a bearer header, not an <a href> at a public path:
    // an anchor carries no credential, and an unguessable URL is not a guard a
    // private meeting should rest on.
    expect(api.recording).toHaveBeenCalledWith('r1')
    expect(createURL).toHaveBeenCalledWith(blob)
    expect(click).toHaveBeenCalled()
    w.unmount()
  })

  it('offers no download for a recording that has no file yet', async () => {
    const w = await panel([done({ status: 'active', size_bytes: 0, duration_sec: 0 })])
    // The worker is still writing it; a download here would hand over a
    // truncated file that looks like the meeting and is not.
    expect(w.find('[data-testid="conference-recording-download"]').exists()).toBe(false)
    expect(w.find('[data-testid="conference-recording-row"]').text()).toContain('Идёт запись')
    w.unmount()
  })

  it('keeps a running recording out of reach of the delete button', async () => {
    const w = await panel([done({ status: 'active' })], { canModerate: true })
    // The row is the only handle on a live egress worker: dropping it would
    // leave the worker recording into a file nothing points at.
    expect(w.find('[data-testid="conference-recording-delete"]').exists()).toBe(false)
    w.unmount()
  })

  it('hides deletion from someone who may not moderate the call', async () => {
    const w = await panel([done()], { canModerate: false })
    expect(w.find('[data-testid="conference-recording-delete"]').exists()).toBe(false)
    w.unmount()
  })

  it('says why a recording is unplayable instead of showing an empty row', async () => {
    const w = await panel([done({ status: 'failed', error: 'egress vanished' })])
    const row = w.find('[data-testid="conference-recording-row"]')
    expect(row.text()).toContain('egress vanished')
    expect(w.find('[data-testid="conference-recording-download"]').exists()).toBe(false)
    w.unmount()
  })

  it('refetches on a nudge rather than patching itself from an event', async () => {
    const w = await panel([])
    api.recordings.mockResolvedValue({ data: [done()] })
    await w.setProps({ nudge: 1 })
    await flushPromises()

    // The list endpoint is the one place that knows the whole truth; a panel
    // patched from a broadcast drifts from it after the first missed frame.
    expect(api.recordings).toHaveBeenCalledTimes(2)
    expect(w.find('[data-testid="conference-recording-row"]').exists()).toBe(true)
    w.unmount()
  })
})
