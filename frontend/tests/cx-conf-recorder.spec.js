import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { nextTick } from 'vue'

// The server-side recording template (#2877): the page egress's headless Chrome
// opens to compose the mp4. No real SDK — a fake Room hands it participants, and
// the assertions are the three things the recording has to get right: a shared
// screen fills the frame with the camera as a PiP, a screenless call is the grid
// of tiles, and egress is told START_RECORDING only once we actually connected.

const lk = vi.hoisted(() => {
  const RoomEvent = {
    ParticipantConnected: 'participantConnected',
    ParticipantDisconnected: 'participantDisconnected',
    TrackPublished: 'trackPublished',
    TrackUnpublished: 'trackUnpublished',
    TrackSubscribed: 'trackSubscribed',
    TrackUnsubscribed: 'trackUnsubscribed',
    TrackMuted: 'trackMuted',
    TrackUnmuted: 'trackUnmuted',
    ActiveSpeakersChanged: 'activeSpeakersChanged',
    Disconnected: 'disconnected',
  }
  const rooms = []
  function participant(identity, over = {}) {
    const pubs = {}
    if (over.video) pubs.camera = { isSubscribed: true, isMuted: false, track: over.video }
    if (over.screen) pubs.screen_share = { isSubscribed: true, isMuted: false, track: over.screen }
    if (over.audio) pubs.microphone = { isSubscribed: true, isMuted: false, track: over.audio }
    return {
      identity,
      sid: `sid-${identity}`,
      name: over.name,
      isSpeaking: false,
      isMicrophoneEnabled: true,
      isCameraEnabled: !!over.video,
      connectionQuality: 'excellent',
      getTrackPublication: (s) => pubs[s],
    }
  }
  class Room {
    constructor() {
      this.handlers = {}
      this.remoteParticipants = new Map()
      rooms.push(this)
    }
    on(ev, fn) {
      ;(this.handlers[ev] ||= []).push(fn)
      return this
    }
    fire(ev) {
      for (const fn of this.handlers[ev] || []) fn()
    }
    async connect(url, token) {
      this.connectedTo = { url, token }
    }
    async disconnect() {
      this.disconnected = true
    }
  }
  return { RoomEvent, Room, rooms, participant }
})
vi.mock('livekit-client', () => ({ Room: lk.Room, RoomEvent: lk.RoomEvent }))

const { default: RecorderView } = await import('@/views/RecorderView.vue')

// A fake SDK track: attach(el) binds video to the passed element; attach() with
// no arg (audio sink) returns a fresh element the way livekit-client does.
const mediaTrack = (id) => ({
  id,
  attach: vi.fn((el) => el || document.createElement('audio')),
  detach: vi.fn(),
})

function setSearch(qs) {
  Object.defineProperty(window, 'location', {
    value: { search: qs },
    configurable: true,
    writable: true,
  })
}

beforeEach(() => {
  lk.rooms.length = 0
  // requestAnimationFrame runs synchronously so START_RECORDING is observable.
  vi.spyOn(window, 'requestAnimationFrame').mockImplementation((cb) => {
    cb()
    return 1
  })
  vi.spyOn(console, 'log').mockImplementation(() => {})
  setSearch('?url=wss://sfu/x&token=jwt&layout=speaker')
})
afterEach(() => {
  vi.restoreAllMocks()
})

async function mountRecorder() {
  const w = mount(RecorderView, {
    global: {
      stubs: {
        ParticipantTile: {
          props: { peer: Object, stage: Boolean, screen: Boolean },
          template: '<div class="tile-stub" :data-id="peer.id" :data-screen="screen" />',
        },
      },
    },
  })
  await flushPromises() // dynamic import of livekit-client + connect()
  await flushPromises()
  return w
}

describe('RecorderView — egress recording template', () => {
  it('connects with the url and token egress put in the query, then signals START_RECORDING', async () => {
    const w = await mountRecorder()
    const room = lk.rooms[0]
    expect(room.connectedTo).toEqual({ url: 'wss://sfu/x', token: 'jwt' })
    // The capture must not begin before we are on the call, or the opening frames
    // are blank.
    expect(console.log).toHaveBeenCalledWith('START_RECORDING')
    w.unmount()
  })

  it('refuses to start — and never signals — without a token', async () => {
    setSearch('?url=wss://sfu/x')
    const w = await mountRecorder()
    expect(lk.rooms).toHaveLength(0)
    expect(console.log).not.toHaveBeenCalledWith('START_RECORDING')
    expect(w.text()).toContain('missing url or token')
    w.unmount()
  })

  it('attaches the shared screen to the full-frame video, with a camera PiP', async () => {
    const w = await mountRecorder()
    const room = lk.rooms[0]
    const scr = mediaTrack('scr')
    room.remoteParticipants.set(
      'a',
      lk.participant('u-a', { name: 'Аня', screen: scr, video: mediaTrack('cam') }),
    )
    room.fire('participantConnected')
    await nextTick()

    // The screen goes to the bare full-frame <video> (no ParticipantTile cap),
    // via attach() — the height limit this rework removed lived on the tile.
    expect(scr.attach).toHaveBeenCalled()
    expect(w.find('.rec-screen-vid').exists()).toBe(true)
    // A camera PiP over it; no grid while a screen is shared.
    expect(w.find('.rec-pip').exists()).toBe(true)
    expect(w.find('.rec-grid').exists()).toBe(false)
    w.unmount()
  })

  it('shows the grid of tiles when nobody is sharing a screen', async () => {
    const w = await mountRecorder()
    const room = lk.rooms[0]
    room.remoteParticipants.set('a', lk.participant('u-a', { name: 'Аня', video: mediaTrack('a') }))
    room.remoteParticipants.set('b', lk.participant('u-b', { name: 'Боря', video: mediaTrack('b') }))
    room.fire('participantConnected')
    await nextTick()

    expect(w.find('.rec-pip').exists()).toBe(false)
    expect(w.find('.rec-grid').exists()).toBe(true)
    expect(w.findAll('.rec-cell')).toHaveLength(2)
    w.unmount()
  })

  it('plays every participant audio, or egress records silence (#2888 removed the tile sink)', async () => {
    const w = await mountRecorder()
    const room = lk.rooms[0]
    const mic = mediaTrack('mic')
    room.remoteParticipants.set('a', lk.participant('u-a', { name: 'Аня', audio: mic }))
    room.fire('participantConnected')
    await nextTick()

    // The custom template records the tab's sound, so a remote mic must actually
    // play here — ParticipantTile stopped rendering audio at #2888.
    expect(mic.attach).toHaveBeenCalled()
    expect(w.find('.rec-audio').element.children.length).toBe(1)
    w.unmount()
  })


  it('tells egress END_RECORDING when the room closes', async () => {
    const w = await mountRecorder()
    lk.rooms[0].fire('disconnected')
    expect(console.log).toHaveBeenCalledWith('END_RECORDING')
    w.unmount()
  })
})
