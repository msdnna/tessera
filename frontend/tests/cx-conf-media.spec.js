import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { nextTick } from 'vue'

// Conference media core (#2864, subtask #2871): the transport seam, the tile and
// the device menu. No real SDK and no real hardware — the point of the seam is
// that everything above it is testable without either.

const lk = vi.hoisted(() => {
  const RoomEvent = {
    ParticipantConnected: 'participantConnected',
    ParticipantDisconnected: 'participantDisconnected',
    TrackSubscribed: 'trackSubscribed',
    TrackUnsubscribed: 'trackUnsubscribed',
    TrackMuted: 'trackMuted',
    TrackUnmuted: 'trackUnmuted',
    LocalTrackPublished: 'localTrackPublished',
    LocalTrackUnpublished: 'localTrackUnpublished',
    ActiveSpeakersChanged: 'activeSpeakersChanged',
    ParticipantNameChanged: 'participantNameChanged',
    AudioPlaybackStatusChanged: 'audioPlaybackChanged',
    ConnectionStateChanged: 'connectionStateChanged',
    MediaDevicesChanged: 'mediaDevicesChanged',
    Disconnected: 'disconnected',
  }
  const state = { rooms: [], devices: { audioinput: [], videoinput: [], audiooutput: [] } }

  function participant(identity, over = {}) {
    const pubs = {}
    if (over.video) pubs.camera = { isSubscribed: true, track: over.video }
    if (over.audio) pubs.microphone = { isSubscribed: true, track: over.audio }
    const p = {
      identity,
      sid: `sid-${identity}`,
      name: over.name,
      isSpeaking: !!over.speaking,
      isMicrophoneEnabled: over.mic !== false,
      isCameraEnabled: !!over.video,
      getTrackPublication: (source) => pubs[source],
    }
    p.setMicrophoneEnabled = vi.fn(async (on) => {
      p.isMicrophoneEnabled = on
    })
    p.setCameraEnabled = vi.fn(async (on) => {
      p.isCameraEnabled = on
    })
    return p
  }

  class Room {
    constructor(options) {
      this.options = options
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
    async connect(url, token) {
      this.connectedTo = { url, token }
    }
    async disconnect() {
      this.disconnectCalls = (this.disconnectCalls || 0) + 1
      this.fire('disconnected')
    }
    async switchActiveDevice(kind, deviceId) {
      this.switched = { kind, deviceId }
    }
    async startAudio() {
      this.canPlaybackAudio = true
      this.fire('audioPlaybackChanged')
    }
    static async getLocalDevices(kind) {
      return state.devices[kind] || []
    }
  }
  return { RoomEvent, Room, state, participant }
})

vi.mock('livekit-client', () => ({ Room: lk.Room, RoomEvent: lk.RoomEvent }))

const api = { token: vi.fn() }
vi.mock('@/api', () => ({ conferences: api, getAccessToken: () => null, apiBaseURL: () => '/api' }))

const { useConfTransport, mediaSupported, IDLE, LIVE, ERROR, UNAVAILABLE } =
  await import('@/composables/useConfTransport')
const { default: ParticipantTile } = await import('@/components/conference/ParticipantTile.vue')
const { default: DeviceMenu } = await import('@/components/conference/DeviceMenu.vue')

// jsdom has no navigator.mediaDevices at all — which is also exactly what an
// insecure context looks like, so both states are reachable from here.
function withMedia(on) {
  Object.defineProperty(navigator, 'mediaDevices', {
    value: on ? { getUserMedia: vi.fn() } : undefined,
    configurable: true,
    writable: true,
  })
}

// Drives the composable outside a component; onBeforeUnmount is guarded for it.
function transport() {
  return useConfTransport()
}

beforeEach(() => {
  lk.state.rooms = []
  lk.state.devices = { audioinput: [], videoinput: [], audiooutput: [] }
  api.token.mockReset()
  api.token.mockResolvedValue({
    data: { url: 'wss://tessera.example/livekit', token: 'jwt', expires_in: 900 },
  })
  withMedia(true)
})
afterEach(() => {
  withMedia(false)
  vi.restoreAllMocks()
})

describe('useConfTransport — secure context', () => {
  it('refuses to even ask for a token without navigator.mediaDevices', async () => {
    withMedia(false)
    expect(mediaSupported()).toBe(false)

    const t = transport()
    await t.join('c1')

    // The dev stand (`:8083`, plain http) lands here on every page load: it must
    // be a stated fact, not a failed request the user is invited to retry.
    expect(t.status.value).toBe(UNAVAILABLE)
    expect(api.token).not.toHaveBeenCalled()
    expect(lk.state.rooms).toHaveLength(0)
  })
})

describe('useConfTransport — joining', () => {
  it('connects with the server-issued url and token, mic on and camera off', async () => {
    const t = transport()
    await t.join('c1')

    expect(api.token).toHaveBeenCalledWith('c1')
    const room = lk.state.rooms[0]
    expect(room.connectedTo).toEqual({ url: 'wss://tessera.example/livekit', token: 'jwt' })
    expect(t.status.value).toBe(LIVE)
    // Audio is why people come to a standup; a camera that turns itself on is a
    // surprise nobody asked for.
    expect(room.localParticipant.setMicrophoneEnabled).toHaveBeenCalledWith(true)
    expect(room.localParticipant.setCameraEnabled).not.toHaveBeenCalled()
    expect(t.micOn.value).toBe(true)
    expect(t.camOn.value).toBe(false)
  })

  it('surfaces the sentence our own API refused with', async () => {
    api.token.mockRejectedValue({
      response: { status: 403, data: { error: 'removed from this conference' } },
    })
    const t = transport()
    await t.join('c1')

    expect(t.status.value).toBe(ERROR)
    expect(t.error.value).toBe('removed from this conference')
    expect(lk.state.rooms).toHaveLength(0)
  })

  it('does not join twice while a join is already in flight', async () => {
    const t = transport()
    const first = t.join('c1')
    await t.join('c1')
    await first
    expect(api.token).toHaveBeenCalledTimes(1)
  })
})

describe('useConfTransport — room snapshot', () => {
  async function joined() {
    const t = transport()
    await t.join('c1')
    return { t, room: lk.state.rooms[0] }
  }

  it('rebuilds every peer from the room on any event', async () => {
    const { t, room } = await joined()
    expect(t.peers.value.map((p) => p.id)).toEqual(['me'])

    room.remoteParticipants.set('a', lk.participant('u-a', { name: 'Аня' }))
    room.remoteParticipants.set('b', lk.participant('u-b', { name: 'Боря', speaking: true }))
    room.fire('participantConnected')

    expect(t.peers.value.map((p) => p.name)).toEqual(['Я', 'Аня', 'Боря'])
    // The speaker takes the stage; everyone else, including me, is a small tile.
    expect(t.dominant.value.name).toBe('Боря')
    expect(t.others.value.map((p) => p.name)).toEqual(['Я', 'Аня'])
  })

  it('falls back to the first remote peer when nobody is speaking', async () => {
    const { t, room } = await joined()
    room.remoteParticipants.set('a', lk.participant('u-a', { name: 'Аня' }))
    room.fire('participantConnected')
    // Silence must not blink the stage empty mid-call.
    expect(t.dominant.value.name).toBe('Аня')
  })

  it('never attaches the local microphone', async () => {
    const { t, room } = await joined()
    room.localParticipant.getTrackPublication = (s) =>
      s === 'microphone' ? { isSubscribed: true, track: { id: 'mine' } } : undefined
    room.fire('trackMuted')
    // Attaching it would route the room's speakers back into the room.
    expect(t.peers.value[0].audioTrack).toBeNull()
  })

  it('drops a participant that leaves', async () => {
    const { t, room } = await joined()
    room.remoteParticipants.set('a', lk.participant('u-a', { name: 'Аня' }))
    room.fire('participantConnected')
    room.remoteParticipants.delete('a')
    room.fire('participantDisconnected')
    expect(t.peers.value.map((p) => p.id)).toEqual(['me'])
  })
})

describe('useConfTransport — teardown', () => {
  it('stops the tracks on leave and unhooks the page listeners', async () => {
    const add = vi.spyOn(window, 'addEventListener')
    const remove = vi.spyOn(window, 'removeEventListener')

    const t = transport()
    await t.join('c1')
    const room = lk.state.rooms[0]
    // A closed tab never runs onBeforeUnmount — without these the webcam light
    // stays on after the user navigates away.
    expect(add.mock.calls.map(([e]) => e)).toEqual(
      expect.arrayContaining(['pagehide', 'beforeunload']),
    )

    await t.leave()
    expect(room.disconnectCalls).toBe(1)
    expect(remove.mock.calls.map(([e]) => e)).toEqual(
      expect.arrayContaining(['pagehide', 'beforeunload']),
    )
    expect(t.status.value).toBe(IDLE)
    expect(t.peers.value).toEqual([])
  })

  it('disconnects when the page goes away', async () => {
    const t = transport()
    await t.join('c1')
    const room = lk.state.rooms[0]

    window.dispatchEvent(new Event('pagehide'))
    expect(room.disconnectCalls).toBe(1)
    await t.leave()
  })

  it('returns to idle, not error, when the SFU drops us', async () => {
    const t = transport()
    await t.join('c1')
    lk.state.rooms[0].fire('disconnected')
    // The join button has to come back; an error screen would be a dead end.
    expect(t.status.value).toBe(IDLE)
    expect(t.peers.value).toEqual([])
  })
})

describe('useConfTransport — devices', () => {
  it('lists devices per kind once connected', async () => {
    lk.state.devices = {
      audioinput: [
        { deviceId: 'm1', label: 'Гарнитура' },
        { deviceId: '', label: 'пустой' },
      ],
      videoinput: [{ deviceId: 'c1', label: 'Веб-камера' }],
      audiooutput: [],
    }
    const t = transport()
    await t.join('c1')
    await flushPromises()

    // A device with an empty id cannot be switched to; listing it would offer a
    // choice that silently does nothing.
    expect(t.devices.value.audioinput).toEqual([{ id: 'm1', label: 'Гарнитура' }])
    expect(t.devices.value.audiooutput).toEqual([])
  })

  it('remembers a device picked before joining and captures with it', async () => {
    const t = transport()
    await t.selectDevice('audioinput', 'm1')
    expect(t.selected.value.audioinput).toBe('m1')

    await t.join('c1')
    // The lobby choice has to survive into the call — otherwise the user picks a
    // headset, joins, and is back on the laptop mic.
    expect(lk.state.rooms[0].options.audioCaptureDefaults).toEqual({ deviceId: 'm1' })
  })

  it('switches a live device through the room', async () => {
    const t = transport()
    await t.join('c1')
    await t.selectDevice('videoinput', 'c2')
    expect(lk.state.rooms[0].switched).toEqual({ kind: 'videoinput', deviceId: 'c2' })
  })
})

describe('useConfTransport — audio autoplay', () => {
  it('reports the block and clears it on a gesture', async () => {
    const t = transport()
    await t.join('c1')
    const room = lk.state.rooms[0]

    room.canPlaybackAudio = false
    room.fire('audioPlaybackChanged')
    expect(t.audioBlocked.value).toBe(true)

    await t.unblockAudio()
    expect(t.audioBlocked.value).toBe(false)
  })
})

describe('ParticipantTile', () => {
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
    ...over,
  })
  const track = () => ({ attach: vi.fn(), detach: vi.fn() })

  it('shows the round avatar when there is no camera', () => {
    const w = mount(ParticipantTile, { props: { peer: peer() } })
    expect(w.findComponent({ name: 'UserAvatar' }).exists()).toBe(true)
    expect(w.find('video').isVisible()).toBe(false)
    w.unmount()
  })

  it('attaches and detaches the video track through the SDK', async () => {
    const v = track()
    const w = mount(ParticipantTile, { props: { peer: peer({ videoTrack: v }) } })
    await nextTick()
    // attach(), not srcObject: it is how the SDK learns the tile is being
    // watched, which is what adaptiveStream bills against.
    expect(v.attach).toHaveBeenCalledTimes(1)

    w.unmount()
    expect(v.detach).toHaveBeenCalledTimes(1)
  })

  it('renders no audio element for the local tile', () => {
    const w = mount(ParticipantTile, { props: { peer: peer({ local: true }) } })
    expect(w.find('audio').exists()).toBe(false)
    w.unmount()
  })

  it('marks the muted and speaking states', () => {
    const w = mount(ParticipantTile, { props: { peer: peer({ micOn: false, speaking: true }) } })
    expect(w.classes()).toContain('speaking')
    expect(w.find('.muted-icon').exists()).toBe(true)
    w.unmount()
  })

  it('shows no mute glyph while the microphone is live', () => {
    const w = mount(ParticipantTile, { props: { peer: peer({ micOn: true }) } })
    expect(w.find('.muted-icon').exists()).toBe(false)
    w.unmount()
  })
})

describe('DeviceMenu', () => {
  const mountMenu = (devices, selected = {}) =>
    mount(DeviceMenu, {
      props: {
        devices,
        selected: { audioinput: '', videoinput: '', audiooutput: '', ...selected },
      },
    })
  // Naive registers its components under the bare name; NDropdown is the export
  // alias, not the component's own `name`.
  const dropdown = (w) => w.findComponent({ name: 'Dropdown' })

  it('omits a kind with no devices behind it', () => {
    // Firefox exposes no audiooutput at all; an empty "Динамики" group reads as
    // a bug rather than as a browser limitation.
    const w = mountMenu({
      audioinput: [{ id: 'm1', label: 'Гарнитура' }],
      videoinput: [],
      audiooutput: [],
    })
    const opts = dropdown(w).props('options')
    expect(opts.filter((o) => o.type === 'group')).toHaveLength(1)
    expect(opts.find((o) => o.type === 'group').children[0].key).toBe('audioinput|m1')
    w.unmount()
  })

  it('names an unlabelled device instead of showing a blank row', () => {
    const w = mountMenu({ audioinput: [{ id: 'm1', label: '' }], videoinput: [], audiooutput: [] })
    const opts = dropdown(w).props('options')
    // Labels stay empty until the browser has granted permission once.
    expect(opts.find((o) => o.type === 'group').children[0].label).toBe('Устройство 1')
    w.unmount()
  })

  it('marks the device currently in use', () => {
    const w = mountMenu(
      {
        audioinput: [
          { id: 'm1', label: 'Гарнитура' },
          { id: 'm2', label: 'Ноутбук' },
        ],
        videoinput: [],
        audiooutput: [],
      },
      { audioinput: 'm2' },
    )
    const rows = dropdown(w)
      .props('options')
      .find((o) => o.type === 'group').children
    expect(rows.map((r) => r.label)).toEqual(['Гарнитура', '● Ноутбук'])
    w.unmount()
  })

  it('emits the kind alongside the id', async () => {
    const w = mountMenu(
      { audioinput: [{ id: 'm1', label: 'Гарнитура' }], videoinput: [], audiooutput: [] },
      { audioinput: 'm1' },
    )
    // The device id alone would not say which slot to switch.
    await dropdown(w).vm.$emit('select', 'audioinput|m1')
    expect(w.emitted('select')[0]).toEqual(['audioinput', 'm1'])
    w.unmount()
  })
})
