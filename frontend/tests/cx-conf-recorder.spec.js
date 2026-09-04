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

  it('fills the frame with the shared screen and the presenter camera as a PiP', async () => {
    const w = await mountRecorder()
    const room = lk.rooms[0]
    room.remoteParticipants.set(
      'a',
      lk.participant('u-a', { name: 'Аня', screen: { id: 'scr' }, video: { id: 'cam' } }),
    )
    room.fire('participantConnected')
    await nextTick()

    // Screen tile carries screen=true; the PiP is a plain (camera) tile.
    const screen = w.find('.rec-screen')
    expect(screen.exists()).toBe(true)
    expect(screen.attributes('data-screen')).toBe('true')
    expect(w.find('.rec-pip').exists()).toBe(true)
    // No grid while a screen is shared.
    expect(w.find('.rec-grid').exists()).toBe(false)
    w.unmount()
  })

  it('shows the grid of tiles when nobody is sharing a screen', async () => {
    const w = await mountRecorder()
    const room = lk.rooms[0]
    room.remoteParticipants.set('a', lk.participant('u-a', { name: 'Аня' }))
    room.remoteParticipants.set('b', lk.participant('u-b', { name: 'Боря' }))
    room.fire('participantConnected')
    await nextTick()

    expect(w.find('.rec-screen').exists()).toBe(false)
    expect(w.find('.rec-grid').exists()).toBe(true)
    expect(w.findAll('.rec-cell')).toHaveLength(2)
    w.unmount()
  })

  it('tells egress END_RECORDING when the room closes', async () => {
    const w = await mountRecorder()
    lk.rooms[0].fire('disconnected')
    expect(console.log).toHaveBeenCalledWith('END_RECORDING')
    w.unmount()
  })
})
