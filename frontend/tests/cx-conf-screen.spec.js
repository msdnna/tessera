import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'
import { nextTick } from 'vue'

// Screen share with a server-side queue (#2864, subtask #2874).
//
// The invariant this file is here to hold: the *stage* is the server's, the
// *pixels* are the SFU's, and the client is allowed to publish a screen only
// while the two agree. Everything below is a way for them to disagree — a host
// preempting a member, a hold expiring, a picker dismissed, the queue reaching
// someone who is no longer waiting — and the assertion is always the same: at
// most one screen is being published, and it belongs to whoever the server says.

// ── fake SDK ─────────────────────────────────────────────────────────────
const lk = vi.hoisted(() => {
  const RoomEvent = {
    ParticipantConnected: 'participantConnected',
    TrackSubscribed: 'trackSubscribed',
    LocalTrackPublished: 'localTrackPublished',
    LocalTrackUnpublished: 'localTrackUnpublished',
    ConnectionStateChanged: 'connectionState',
    Disconnected: 'disconnected',
  }
  const state = { rooms: [], grantScreen: true }

  function track(id) {
    return { id, attach: vi.fn(), detach: vi.fn() }
  }

  function participant(identity, over = {}) {
    const pubs = {}
    if (over.screen) pubs.screen_share = { isSubscribed: true, track: over.screen }
    if (over.screenAudio) pubs.screen_share_audio = { isSubscribed: true, track: over.screenAudio }
    const p = {
      identity,
      sid: `sid-${identity}`,
      name: over.name || identity,
      isSpeaking: !!over.speaking,
      isMicrophoneEnabled: over.mic !== false,
      isCameraEnabled: false,
      isScreenShareEnabled: !!over.screen,
      getTrackPublication: (source) => pubs[source],
    }
    p.setMicrophoneEnabled = vi.fn(async (on) => {
      p.isMicrophoneEnabled = on
    })
    p.setCameraEnabled = vi.fn(async (on) => {
      p.isCameraEnabled = on
    })
    p.screenCalls = []
    p.setScreenShareEnabled = vi.fn(async (on, opts) => {
      p.screenCalls.push({ on, opts })
      if (on && !state.grantScreen) {
        // What a dismissed picker actually throws.
        const err = new Error('Permission denied')
        err.name = 'NotAllowedError'
        throw err
      }
      p.isScreenShareEnabled = on
      pubs.screen_share = on
        ? { isSubscribed: true, track: track(`screen-${identity}`) }
        : undefined
    })
    return p
  }

  class Room {
    constructor() {
      this.handlers = {}
      this.remoteParticipants = new Map()
      this.canPlaybackAudio = true
      this.localParticipant = participant('me', { name: 'Я', mic: false })
      state.rooms.push(this)
    }
    on(event, fn) {
      ;(this.handlers[event] ||= []).push(fn)
      return this
    }
    fire(event) {
      for (const fn of this.handlers[event] || []) fn()
    }
    async connect() {}
    async disconnect() {
      this.fire('disconnected')
    }
    async switchActiveDevice() {}
    async startAudio() {}
    static async getLocalDevices() {
      return []
    }
  }
  return { RoomEvent, Room, state, participant, track }
})

vi.mock('livekit-client', () => ({ Room: lk.Room, RoomEvent: lk.RoomEvent }))

const api = { token: vi.fn() }
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
  deliver(msg) {
    this.onmessage?.({ data: JSON.stringify(msg) })
  }
}

const { useConfTransport } = await import('@/composables/useConfTransport')
const { useConfRoom } = await import('@/composables/useConfRoom')
const { useConferenceSession } = await import('@/stores/conference')
const { default: ParticipantTile } = await import('@/components/conference/ParticipantTile.vue')
const { default: ConferenceRoom } = await import('@/components/conference/ConferenceRoom.vue')

beforeEach(() => {
  // The rail's chat reads the auth and theme stores through useFormat.
  setActivePinia(createPinia())
  lk.state.rooms = []
  lk.state.grantScreen = true
  sockets.length = 0
  vi.stubGlobal('WebSocket', FakeSocket)
  api.token.mockReset()
  api.token.mockResolvedValue({ data: { url: 'wss://x/livekit', token: 'jwt', expires_in: 900 } })
  Object.defineProperty(navigator, 'mediaDevices', {
    value: { getUserMedia: vi.fn() },
    configurable: true,
    writable: true,
  })
})
afterEach(() => {
  // The session lives in the store now (#2888), so it no longer stops when a
  // ConferenceRoom unmounts — tear it down explicitly so its media loop and
  // socket do not leak into the next test.
  useConferenceSession().stop()
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

describe('useConfRoom — the stage', () => {
  function opened(role = 'member') {
    const room = useConfRoom()
    room.open('c1')
    const ws = sockets.at(-1)
    ws.onopen()
    ws.deliver({
      type: 'welcome',
      conn_id: 'c-1',
      user_id: 'me',
      role,
      stage_ttl_ms: 30000,
    })
    return { room, ws }
  }

  it('sends the three stage commands in the shape the server reads', () => {
    const { room, ws } = opened()
    room.requestScreen()
    room.refreshScreen()
    room.releaseScreen()

    expect(ws.sent).toEqual([
      { type: 'screen.request' },
      { type: 'screen.refresh' },
      { type: 'screen.release' },
    ])
  })

  it('takes the hold TTL from the server rather than assuming one', () => {
    const room = useConfRoom()
    room.open('c1')
    const ws = sockets.at(-1)
    ws.onopen()
    ws.deliver({ type: 'welcome', conn_id: 'c-1', user_id: 'me', role: 'host', stage_ttl_ms: 9000 })
    // A heartbeat derived from a hardcoded TTL would silently drift out of step
    // the day the server's constant changes.
    expect(room.stageTtlMs.value).toBe(9000)
  })

  it('reads the stage by connection, not by user', () => {
    const { room, ws } = opened()
    ws.deliver({
      type: 'state',
      participants: [],
      // Same person, their *other* tab. A stop-sharing button here would stop
      // nothing and say it did.
      stage: { user_id: 'me', conn_id: 'c-2', name: 'Я', since: '2026-09-03T03:00:00Z' },
      queue: [],
    })
    expect(room.presenting.value).toBe(false)

    ws.deliver({
      type: 'state',
      participants: [],
      stage: { user_id: 'me', conn_id: 'c-1', name: 'Я', since: '2026-09-03T03:00:00Z' },
      queue: [],
    })
    expect(room.presenting.value).toBe(true)
  })

  it('reports our 1-based place in the queue, and zero when we are not in it', () => {
    const { room, ws } = opened()
    ws.deliver({
      type: 'state',
      participants: [],
      stage: { user_id: 'u-1', conn_id: 'c-9', name: 'Ира' },
      queue: [
        { user_id: 'u-2', conn_id: 'c-8', name: 'Боря', role: 'member' },
        { user_id: 'me', conn_id: 'c-1', name: 'Я', role: 'member' },
      ],
    })
    expect(room.queuePos.value).toBe(2)

    ws.deliver({ type: 'state', participants: [], stage: null, queue: [] })
    expect(room.queuePos.value).toBe(0)
  })

  it('counts snapshots so an unanswered request can be told from a refusal', () => {
    const { room, ws } = opened()
    const before = room.stateSeq.value
    ws.deliver({ type: 'state', participants: [], stage: null, queue: [] })
    expect(room.stateSeq.value).toBe(before + 1)
  })
})

describe('useConfTransport — publishing a screen', () => {
  async function joined() {
    const t = useConfTransport()
    await t.join('c1')
    return { t, room: lk.state.rooms[0] }
  }

  it('publishes the screen with its audio and reports it as ours', async () => {
    const { t, room } = await joined()
    expect(await t.startScreen()).toBe(true)

    // Audio too: a shared tab playing a video silently for everyone else is the
    // classic screen-share disappointment. Plus the quality knobs (#2885): the
    // 'detail' content hint keeps a shared screen sharp for the remote viewers.
    const call = room.localParticipant.screenCalls[0]
    expect(call.on).toBe(true)
    expect(call.opts.audio).toBe(true)
    expect(call.opts.contentHint).toBe('detail')
    expect(t.screenOn.value).toBe(true)
    expect(t.screenPeer.value.id).toBe('me')
  })

  it('treats a dismissed picker as a no, not as an error', async () => {
    lk.state.grantScreen = false
    const { t } = await joined()

    expect(await t.startScreen()).toBe(false)
    expect(t.screenOn.value).toBe(false)
    // The user changed their mind; an error banner would be noise.
    expect(t.error.value).toBe('')
  })

  it('follows the browser stopping the capture behind our back', async () => {
    const { t, room } = await joined()
    await t.startScreen()

    // The browser's own "stop sharing" bar ends the capture without asking us.
    room.localParticipant.isScreenShareEnabled = false
    room.localParticipant.getTrackPublication = () => undefined
    room.fire('localTrackUnpublished')

    expect(t.screenOn.value).toBe(false)
    expect(t.screenPeer.value).toBeNull()
  })

  it('finds a remote presenter and keeps their camera tile separate', async () => {
    const { t, room } = await joined()
    const screen = lk.track('scr')
    room.remoteParticipants.set('a', lk.participant('u-a', { name: 'Аня', screen }))
    room.fire('participantConnected')

    const peer = t.peers.value.find((p) => p.id === 'u-a')
    expect(peer.screenTrack).toBe(screen)
    expect(peer.videoTrack).toBeNull()
    expect(t.screenPeer.value.name).toBe('Аня')
  })

  it('never plays our own shared audio back into the room', async () => {
    const { t, room } = await joined()
    room.localParticipant.getTrackPublication = (s) =>
      s === 'screen_share_audio' ? { isSubscribed: true, track: lk.track('mine') } : undefined
    room.fire('localTrackPublished')

    // Attaching it would route the room's speakers through the room again.
    expect(t.peers.value[0].screenAudioTrack).toBeNull()
  })

  it('drops the sharing flag when the SFU disconnects us', async () => {
    const { t, room } = await joined()
    await t.startScreen()
    room.fire('disconnected')
    expect(t.screenOn.value).toBe(false)
  })
})

describe('ParticipantTile — screen mode', () => {
  const peer = (over = {}) => ({
    id: 'u-a',
    sid: 'sid-a',
    name: 'Аня',
    local: false,
    speaking: false,
    micOn: true,
    camOn: false,
    videoTrack: null,
    audioTrack: null,
    screenTrack: null,
    screenAudioTrack: null,
    ...over,
  })

  it('attaches the shared screen instead of the camera', async () => {
    const cam = lk.track('cam')
    const scr = lk.track('scr')
    const w = mount(ParticipantTile, {
      props: { peer: peer({ videoTrack: cam, screenTrack: scr }), screen: true },
    })
    await nextTick()

    expect(scr.attach).toHaveBeenCalledTimes(1)
    expect(cam.attach).not.toHaveBeenCalled()
    w.unmount()
    expect(scr.detach).toHaveBeenCalledTimes(1)
  })

  it('does not fall back to the camera while the first frame is in flight', async () => {
    const cam = lk.track('cam')
    const w = mount(ParticipantTile, {
      props: { peer: peer({ videoTrack: cam, screenTrack: null }), screen: true },
    })
    await nextTick()

    // Showing their webcam where their screen belongs would be a lie about what
    // is being presented.
    expect(cam.attach).not.toHaveBeenCalled()
    expect(w.find('.face').exists()).toBe(true)
    expect(w.findComponent({ name: 'UserAvatar' }).exists()).toBe(false)
    w.unmount()
  })

  it('carries the screen through to a distinct testid and letterboxes it', () => {
    const w = mount(ParticipantTile, {
      props: { peer: peer({ screenTrack: lk.track('scr') }), screen: true },
    })
    expect(w.attributes('data-testid')).toBe('conference-screen-tile')
    // `cover` would crop exactly the spreadsheet rows being pointed at.
    expect(w.classes()).toContain('screen')
    w.unmount()
  })

  it('shows no mute glyph or speaking ring on a screen tile', () => {
    const w = mount(ParticipantTile, {
      props: {
        peer: peer({ screenTrack: lk.track('scr'), micOn: false, speaking: true }),
        screen: true,
      },
    })
    // Those belong to the person, and the person still has their own tile.
    expect(w.find('.muted-icon').exists()).toBe(false)
    expect(w.classes()).not.toContain('speaking')
    w.unmount()
  })
})

describe('ConferenceRoom — the queue in practice', () => {
  async function room(props = {}) {
    // The store owns the session now (#2888): it opens the socket and joins the
    // SFU, and ConferenceRoom is the view over it. Start it before mounting, the
    // way ConferencesView does when membership is confirmed.
    useConferenceSession().start('c1', 'Тест')
    const w = mount(ConferenceRoom, {
      props: { conferenceId: 'c1', active: true, ended: false, ...props },
      global: {
        // The rail is not what this file is about, and the chat half of it wants
        // a message provider and its own half of the API.
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
      role: 'member',
      stage_ttl_ms: 3000,
    })
    await nextTick()
    return { w, ws, sfu: lk.state.rooms[0] }
  }

  const snapshot = (over = {}) => ({
    type: 'state',
    participants: [{ user_id: 'me', name: 'Я', role: 'member', mic: true, cam: false }],
    stage: null,
    queue: [],
    ...over,
  })

  it('asks the server before capturing, then shares once the stage is free', async () => {
    const { w, ws, sfu } = await room()
    await w.find('[data-testid="conference-screen"]').trigger('click')
    await flushPromises()

    // The room is the arbiter and is told first; the capture starts in the same
    // click because the browser only grants the picker inside a gesture.
    expect(ws.sent).toContainEqual({ type: 'screen.request' })
    expect(sfu.localParticipant.screenCalls.map((c) => c.on)).toEqual([true])
    expect(w.find('[data-testid="conference-screen-tile"]').exists()).toBe(true)
    w.unmount()
  })

  it('queues instead of opening a picker that would be stopped a moment later', async () => {
    const { w, ws, sfu } = await room()
    ws.deliver(snapshot({ stage: { user_id: 'u-1', conn_id: 'c-9', name: 'Ира' } }))
    await nextTick()

    await w.find('[data-testid="conference-screen"]').trigger('click')
    await flushPromises()

    expect(ws.sent).toContainEqual({ type: 'screen.request' })
    // Making the user pick a window only to have it taken away is worse than
    // telling them they are in the queue.
    expect(sfu.localParticipant.screenCalls).toHaveLength(0)
    w.unmount()
  })

  it('stops our capture the moment a host takes the stage away', async () => {
    const { w, ws, sfu } = await room()
    await w.find('[data-testid="conference-screen"]').trigger('click')
    await flushPromises()
    ws.deliver(snapshot({ stage: { user_id: 'me', conn_id: 'c-1', name: 'Я' } }))
    await nextTick()
    expect(sfu.localParticipant.isScreenShareEnabled).toBe(true)

    // A host preempts: the server moved us to the head of the queue.
    ws.deliver(
      snapshot({
        stage: { user_id: 'u-1', conn_id: 'c-9', name: 'Ира' },
        queue: [{ user_id: 'me', conn_id: 'c-1', name: 'Я', role: 'member' }],
      }),
    )
    await flushPromises()

    // Two screens published at once is exactly what the queue exists to prevent.
    expect(sfu.localParticipant.screenCalls.map((c) => c.on)).toEqual([true, false])
    w.unmount()
  })

  it('does not stop a share before the server has answered the request', async () => {
    const { w, sfu } = await room()
    await w.find('[data-testid="conference-screen"]').trigger('click')
    await flushPromises()

    // No snapshot has arrived yet: `stage` is still null, which must not be read
    // as "the stage is not yours".
    expect(sfu.localParticipant.isScreenShareEnabled).toBe(true)
    expect(sfu.localParticipant.screenCalls.map((c) => c.on)).toEqual([true])
    w.unmount()
  })

  it('releases the stage when the picker is dismissed', async () => {
    lk.state.grantScreen = false
    const { w, ws } = await room()
    await w.find('[data-testid="conference-screen"]').trigger('click')
    await flushPromises()

    // Holding a stage for a share that never starts parks the whole queue.
    expect(ws.sent).toContainEqual({ type: 'screen.release' })
    w.unmount()
  })

  it('drops out of the queue on a second click while waiting', async () => {
    const { w, ws } = await room()
    ws.deliver(snapshot({ stage: { user_id: 'u-1', conn_id: 'c-9', name: 'Ира' } }))
    await nextTick()
    await w.find('[data-testid="conference-screen"]').trigger('click')
    await flushPromises()

    await w.find('[data-testid="conference-screen"]').trigger('click')
    await flushPromises()
    expect(ws.sent.filter((m) => m.type === 'screen.release')).toHaveLength(1)
    w.unmount()
  })

  it('prompts for a click when the queue reaches us, instead of a silent picker', async () => {
    const { w, ws } = await room()
    ws.deliver(snapshot({ stage: { user_id: 'u-1', conn_id: 'c-9', name: 'Ира' } }))
    await nextTick()
    await w.find('[data-testid="conference-screen"]').trigger('click')
    await flushPromises()

    // Our turn: the stage is ours but nothing is being captured.
    ws.deliver(snapshot({ stage: { user_id: 'me', conn_id: 'c-1', name: 'Я' } }))
    await nextTick()

    const turn = w.find('[data-testid="conference-screen-turn"]')
    expect(turn.exists()).toBe(true)
    // getDisplayMedia outside a gesture is refused, so this has to be a button.
    await turn.find('button').trigger('click')
    await flushPromises()
    expect(lk.state.rooms[0].localParticipant.isScreenShareEnabled).toBe(true)
    w.unmount()
  })

  it('refreshes the hold while sharing and stops when the share does', async () => {
    vi.useFakeTimers()
    try {
      const { w, ws, sfu } = await room()
      await w.find('[data-testid="conference-screen"]').trigger('click')
      await flushPromises()
      ws.deliver(snapshot({ stage: { user_id: 'me', conn_id: 'c-1', name: 'Я' } }))
      await nextTick()

      // TTL 3000 from the welcome above → a beat every second.
      vi.advanceTimersByTime(2500)
      expect(ws.sent.filter((m) => m.type === 'screen.refresh')).toHaveLength(2)

      sfu.localParticipant.isScreenShareEnabled = false
      sfu.localParticipant.getTrackPublication = () => undefined
      sfu.fire('localTrackUnpublished')
      await nextTick()
      const beats = ws.sent.filter((m) => m.type === 'screen.refresh').length
      vi.advanceTimersByTime(5000)
      // A heartbeat outliving the capture would hold the stage against the queue.
      expect(ws.sent.filter((m) => m.type === 'screen.refresh')).toHaveLength(beats)
      w.unmount()
    } finally {
      vi.useRealTimers()
    }
  })

  it('shows the presenter and the length of the line behind them', async () => {
    const { w, ws } = await room()
    ws.deliver(
      snapshot({
        stage: { user_id: 'u-1', conn_id: 'c-9', name: 'Ира' },
        queue: [{ user_id: 'u-2', conn_id: 'c-8', name: 'Боря', role: 'member' }],
      }),
    )
    await nextTick()

    const note = w.find('[data-testid="conference-stage-note"]')
    expect(note.text()).toContain('Экран показывает Ира')
    expect(note.text()).toContain('Ждут очереди: 1')
    w.unmount()
  })

  it('does not announce a share that has not started yet', async () => {
    const { w, ws } = await room()
    ws.deliver(snapshot({ stage: { user_id: 'u-1', conn_id: 'c-9', name: 'Ира' } }))
    await nextTick()
    await w.find('[data-testid="conference-screen"]').trigger('click')
    await flushPromises()

    // The queue reaches us: the stage is ours, the picker has not been answered.
    ws.deliver(snapshot({ stage: { user_id: 'me', conn_id: 'c-1', name: 'Я' } }))
    await nextTick()

    // Claiming a share here contradicts the "start sharing" prompt right below.
    const note = w.find('[data-testid="conference-stage-note"]')
    expect(w.find('[data-testid="conference-screen-turn"]').exists()).toBe(true)
    expect(note.text()).not.toContain('Вы показываете экран')
    expect(note.text()).toContain('Очередь дошла до вас')

    // …and once the capture is running, it says so.
    await w.find('[data-testid="conference-screen-turn"]').find('button').trigger('click')
    await flushPromises()
    expect(w.find('[data-testid="conference-stage-note"]').text()).toContain('Вы показываете экран')
    w.unmount()
  })

  it('shows our own place in the line rather than the crowd size', async () => {
    const { w, ws } = await room()
    ws.deliver(
      snapshot({
        stage: { user_id: 'u-1', conn_id: 'c-9', name: 'Ира' },
        queue: [
          { user_id: 'u-2', conn_id: 'c-8', name: 'Боря', role: 'member' },
          { user_id: 'me', conn_id: 'c-1', name: 'Я', role: 'member' },
        ],
      }),
    )
    await nextTick()

    expect(w.find('[data-testid="conference-screen-queuepos"]').text()).toBe('Вы в очереди: 2')
    w.unmount()
  })

  it('gives the stage to a shared screen and moves the speaker into the strip', async () => {
    const { w, sfu } = await room()
    const remote = lk.participant('u-a', { name: 'Аня', screen: lk.track('scr'), speaking: true })
    sfu.remoteParticipants.set('a', remote)
    sfu.fire('connectionState')
    await nextTick()

    // A screen is always what people are looking at — that is why it was shared.
    expect(w.find('[data-testid="conference-screen-tile"]').exists()).toBe(true)
    // The presenter keeps their own tile in the strip alongside everyone else.
    expect(w.findAll('[data-testid="conference-tile"]')).toHaveLength(2)
    w.unmount()
  })
})
