import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'

// Participants: the panel, local volume, and the two host commands (#2864,
// subtask #2872).
//
// The line this file exists to hold is the one between "I cannot hear them" and
// "nobody may hear them". The first is a number in this browser and reaches
// nothing else; the second is a command our server has to authorise, and the UI
// is not allowed to decide it — a member who forces the button visible still
// gets refused over the socket.

// ── fake SDK ─────────────────────────────────────────────────────────────
// Only what the volume path touches: a remote participant that records the
// levels it was set to, so "the slider moved" and "the audio changed" can be
// told apart.
const lk = vi.hoisted(() => {
  const RoomEvent = { Disconnected: 'disconnected', ConnectionStateChanged: 'connectionState' }
  const state = { rooms: [] }

  function participant(identity, over = {}) {
    const p = {
      identity,
      sid: `sid-${identity}`,
      name: over.name || identity,
      isSpeaking: false,
      isMicrophoneEnabled: over.mic !== false,
      isCameraEnabled: false,
      getTrackPublication: () => undefined,
      volumes: [],
    }
    p.setVolume = vi.fn((v) => p.volumes.push(v))
    p.setMicrophoneEnabled = vi.fn(async (on) => {
      p.isMicrophoneEnabled = on
    })
    p.setCameraEnabled = vi.fn(async (on) => {
      p.isCameraEnabled = on
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
  return { RoomEvent, Room, state, participant }
})

vi.mock('livekit-client', () => ({ Room: lk.Room, RoomEvent: lk.RoomEvent }))

const api = { token: vi.fn() }
vi.mock('@/api', () => ({
  conferences: api,
  getAccessToken: () => 'jwt',
  apiBaseURL: () => '/api',
}))

// ── fake socket ──────────────────────────────────────────────────────────
// The room socket is the other half of a call. A hand-written double rather
// than a library: the assertions here are about the exact frames on the wire,
// which is precisely what a helpful mock would hide.
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

const { useConfTransport, VOLUME_MAX } = await import('@/composables/useConfTransport')
const { useConfRoom } = await import('@/composables/useConfRoom')
const { default: ParticipantsPanel } = await import('@/components/conference/ParticipantsPanel.vue')

beforeEach(() => {
  lk.state.rooms = []
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
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

// Joins a transport with one remote participant already in the room.
async function joined(identity = 'u-1') {
  const t = useConfTransport()
  await t.join('c1')
  const room = lk.state.rooms[0]
  const remote = lk.participant(identity, { name: 'Ира' })
  room.remoteParticipants.set(remote.sid, remote)
  room.fire('connectionState')
  return { t, room, remote }
}

describe('local volume', () => {
  it('turns one participant down without touching anyone else', async () => {
    const { t, remote } = await joined()

    t.setPeerVolume('u-1', 0.4)

    expect(remote.setVolume).toHaveBeenCalledWith(0.4)
    expect(t.peers.value.find((p) => p.id === 'u-1').volume).toBe(0.4)
    // Nothing crossed a wire: this is a gain in one browser, and the person
    // being turned down is deliberately not told.
    expect(sockets).toHaveLength(0)
  })

  it('clamps the level to the usable range', async () => {
    const { t, remote } = await joined()

    t.setPeerVolume('u-1', 99)
    expect(remote.volumes.at(-1)).toBe(VOLUME_MAX)

    t.setPeerVolume('u-1', -3)
    expect(remote.volumes.at(-1)).toBe(0)
  })

  it('reapplies the level after the participant republishes', async () => {
    const { t, room, remote } = await joined()
    t.setPeerVolume('u-1', 0.3)
    remote.volumes.length = 0

    // A track event is exactly what happens when someone's microphone flickers
    // or their connection drops and comes back. The SDK attaches volume to the
    // track, so without reapplying it here the person we turned down would
    // quietly come back to full volume.
    room.fire('connectionState')

    expect(remote.volumes.at(-1)).toBe(0.3)
    expect(t.peers.value.find((p) => p.id === 'u-1').volume).toBe(0.3)
  })

  it('mutes locally and restores the previous level, not the default', async () => {
    const { t, remote } = await joined()
    t.setPeerVolume('u-1', 0.6)

    t.togglePeerMute('u-1')
    expect(remote.volumes.at(-1)).toBe(0)
    expect(t.peers.value.find((p) => p.id === 'u-1').localMuted).toBe(true)

    t.togglePeerMute('u-1')
    // 0.6, not 1: snapping back to the default would undo a deliberate choice
    // every time someone was silenced for a moment.
    expect(remote.volumes.at(-1)).toBe(0.6)
  })

  it('treats moving the slider off zero as unmuting', async () => {
    const { t, remote } = await joined()
    t.togglePeerMute('u-1')

    t.setPeerVolume('u-1', 0.5)

    expect(remote.volumes.at(-1)).toBe(0.5)
    expect(t.peers.value.find((p) => p.id === 'u-1').localMuted).toBe(false)
  })
})

describe('useConfRoom', () => {
  function opened(role = 'host') {
    const room = useConfRoom()
    room.open('c1')
    const ws = sockets.at(-1)
    ws.onopen()
    ws.deliver({ type: 'welcome', conn_id: 'c-1', user_id: 'me', role })
    return { room, ws }
  }

  it('carries the bearer token as a subprotocol, not a query parameter', () => {
    const { ws } = opened()
    expect(ws.url).toContain('/api/conferences/c1/ws')
    expect(ws.protocols).toEqual(['bearer', 'jwt'])
  })

  it('takes the whole roster from each snapshot', () => {
    const { room, ws } = opened()
    ws.deliver({
      type: 'state',
      participants: [{ user_id: 'me', name: 'Я', role: 'host', mic: true }],
      stage: null,
      queue: [],
    })
    expect(room.participants.value).toHaveLength(1)

    // A later snapshot replaces rather than merges — that is what makes a
    // reconnect self-healing instead of leaving ghosts behind.
    ws.deliver({ type: 'state', participants: [], stage: null, queue: [] })
    expect(room.participants.value).toHaveLength(0)
  })

  it('sends moderation commands in the shape the server reads', () => {
    const { room, ws } = opened()
    room.kick('u-1')
    room.forceMute('u-1', true)

    expect(ws.sent).toContainEqual({ type: 'kick', user_id: 'u-1' })
    expect(ws.sent).toContainEqual({ type: 'mute', user_id: 'u-1', muted: true })
  })

  it('restates the device state after a reconnect', () => {
    const { room, ws } = opened()
    room.setMedia(true, false)
    expect(ws.sent).toContainEqual({ type: 'media', mic: true, cam: false })

    // A new connection is a new participant as far as the server is concerned,
    // and it starts everyone muted. A client that stayed silent here would show
    // as muted in the roster while actually talking.
    ws.deliver({ type: 'welcome', conn_id: 'c-2', user_id: 'me', role: 'host' })
    expect(ws.sent.filter((m) => m.type === 'media')).toHaveLength(2)
  })

  it('surfaces the server refusal instead of guessing at permissions', () => {
    const { room, ws } = opened('member')
    ws.deliver({ type: 'denied', action: 'kick', reason: 'not a host' })

    expect(room.isHost.value).toBe(false)
    expect(room.denied.value).toEqual({ action: 'kick', reason: 'not a host' })
  })

  it('reports a force-mute of ourselves from the roster', () => {
    const { room, ws } = opened('member')
    ws.deliver({
      type: 'state',
      participants: [{ user_id: 'me', name: 'Я', role: 'member', mic: false, force_muted: true }],
    })
    expect(room.forceMuted.value).toBe(true)
  })

  it('stops reconnecting once the call is over', () => {
    const { room, ws } = opened()
    ws.deliver({ type: 'ended', reason: 'kicked' })

    expect(room.ended.value).toEqual({ reason: 'kicked' })
    // Retrying here would loop against a 403 for the length of the cooldown.
    expect(room.connected.value).toBe(false)
    expect(sockets).toHaveLength(1)
  })
})

describe('ParticipantsPanel', () => {
  const roster = [
    { user_id: 'me', name: 'Я', role: 'host', mic: true, cam: false },
    { user_id: 'u-1', name: 'Ира', role: 'member', mic: true, cam: false },
  ]
  const peers = [
    { id: 'me', local: true, volume: 1, localMuted: false },
    { id: 'u-1', local: false, volume: 1, localMuted: false },
  ]

  function panel(props = {}) {
    return mount(ParticipantsPanel, {
      props: { people: roster, peers, meId: 'me', canModerate: true, ...props },
    })
  }

  it('offers the host commands for other people only', () => {
    const w = panel()
    expect(w.find('[data-testid="conference-kick-u-1"]').exists()).toBe(true)
    // Kicking yourself is refused by the server anyway; offering the button
    // would just be a way to lose your own call.
    expect(w.find('[data-testid="conference-kick-me"]').exists()).toBe(false)
    w.unmount()
  })

  it('hides the host commands from a plain member', () => {
    const w = panel({ canModerate: false })
    expect(w.find('[data-testid="conference-kick-u-1"]').exists()).toBe(false)
    expect(w.find('[data-testid="conference-force-mute-u-1"]').exists()).toBe(false)
    // The local volume control is not moderation and stays for everyone.
    expect(w.find('[data-testid="conference-local-mute-u-1"]').exists()).toBe(true)
    w.unmount()
  })

  it('says who was silenced by a host rather than by themselves', () => {
    const w = panel({
      people: [roster[0], { ...roster[1], mic: false, force_muted: true }],
    })
    expect(w.find('[data-testid="conference-forced"]').exists()).toBe(true)
    expect(w.text()).toContain('заглушён ведущим')
    w.unmount()
  })

  it('asks the parent to lift a force-mute that is already on', async () => {
    const w = panel({
      people: [roster[0], { ...roster[1], force_muted: true }],
    })
    await w.find('[data-testid="conference-force-mute-u-1"]').trigger('click')
    expect(w.emitted('force-mute')).toEqual([['u-1', false]])
    w.unmount()
  })

  it('puts a raised hand at the top of the list', async () => {
    const w = panel({
      people: [roster[0], { ...roster[1], hand_at: '2026-09-03T01:00:00Z' }],
    })
    await nextTick()
    const rows = w.findAll('[data-testid="conference-participant"]')
    expect(rows[0].text()).toContain('Ира')
    expect(w.find('[data-testid="conference-hand-up"]').exists()).toBe(true)
    w.unmount()
  })

  it('leaves out the volume controls for someone with no audio yet', () => {
    const w = panel({ peers: [peers[0]] })
    expect(w.find('[data-testid="conference-local-mute-u-1"]').exists()).toBe(false)
    w.unmount()
  })
})
