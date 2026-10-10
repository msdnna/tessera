import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import {
  REACTIONS,
  REACTION_BURST,
  REACTION_MAX,
  REACTION_TOPIC,
  REACTION_TTL_MS,
  addReaction,
  canReact,
  decodeReaction,
  encodeReaction,
  pruneReactions,
} from '@/utils/confReactions'

// Reactions in a call (#3054): over the SFU's data channel, never stored. The
// stand has no SFU, so the wire is a stubbed LiveKit room here.

const lk = vi.hoisted(() => {
  const RoomEvent = {
    ParticipantConnected: 'participantConnected',
    Disconnected: 'disconnected',
    DataReceived: 'dataReceived',
  }
  const state = { rooms: [] }
  class Room {
    constructor() {
      this.handlers = {}
      this.remoteParticipants = new Map()
      this.canPlaybackAudio = true
      this.localParticipant = {
        identity: 'me',
        name: 'Я',
        isMicrophoneEnabled: false,
        getTrackPublication: () => undefined,
        setMicrophoneEnabled: vi.fn(async () => {}),
        setCameraEnabled: vi.fn(async () => {}),
        publishData: vi.fn(async () => {}),
      }
      state.rooms.push(this)
    }
    on(event, fn) {
      ;(this.handlers[event] ||= []).push(fn)
      return this
    }
    emit(event, ...args) {
      for (const fn of this.handlers[event] || []) fn(...args)
    }
    async connect() {}
    async disconnect() {
      this.emit('disconnected')
    }
    static async getLocalDevices() {
      return []
    }
  }
  return { RoomEvent, Room, state }
})
vi.mock('livekit-client', () => ({ Room: lk.Room, RoomEvent: lk.RoomEvent }))
const api = { token: vi.fn() }
vi.mock('@/api', () => ({ conferences: api, getAccessToken: () => null, apiBaseURL: () => '/api' }))

const { useConfTransport } = await import('@/composables/useConfTransport')

describe('the wire form', () => {
  it('round-trips one of the palette, and nothing else', () => {
    for (const e of REACTIONS) expect(decodeReaction(encodeReaction(e))).toBe(e)
    expect(encodeReaction('💩')).toBe(null)
    const enc = new TextEncoder()
    expect(decodeReaction(enc.encode('{"t":"reaction","e":"<b>hi</b>"}'))).toBe(null)
    expect(decodeReaction(enc.encode('{"t":"chat","e":"👍"}'))).toBe(null)
    expect(decodeReaction(enc.encode('not json'))).toBe(null)
    expect(decodeReaction(null)).toBe(null)
  })
})

describe('the feed', () => {
  it('caps the bubbles and prunes the old ones', () => {
    let feed = []
    for (let i = 0; i < REACTION_MAX + 5; i++)
      feed = addReaction(feed, { identity: 'a', emoji: '👍', at: 1000 + i })
    expect(feed).toHaveLength(REACTION_MAX)
    expect(feed.at(-1).at).toBe(1000 + REACTION_MAX + 4)
    expect(pruneReactions(feed, 1000 + REACTION_TTL_MS + REACTION_MAX)).toHaveLength(4)
  })
  it('throttles a burst of our own', () => {
    const now = 10_000
    const sent = Array.from({ length: REACTION_BURST }, (_, i) => now - i * 10)
    expect(canReact(sent, now)).toBe(false)
    expect(canReact(sent.slice(1), now)).toBe(true)
    expect(canReact(sent, now + 5000)).toBe(true)
  })
})

describe('useConfTransport — reactions', () => {
  beforeEach(() => {
    lk.state.rooms = []
    api.token.mockResolvedValue({ data: { url: 'wss://x', token: 'jwt' } })
    Object.defineProperty(navigator, 'mediaDevices', {
      value: { getUserMedia: vi.fn(), enumerateDevices: vi.fn(async () => []) },
      configurable: true,
      writable: true,
    })
  })
  afterEach(() => {
    Object.defineProperty(navigator, 'mediaDevices', {
      value: undefined,
      configurable: true,
      writable: true,
    })
  })

  it('does nothing outside a call', async () => {
    const t = useConfTransport()
    expect(await t.sendReaction('👍')).toBe(false)
    expect(t.reactions.value).toHaveLength(0)
  })

  it('publishes lossy on the reaction topic and shows our own bubble at once', async () => {
    const t = useConfTransport()
    await t.join('c1')
    const room = lk.state.rooms[0]
    expect(await t.sendReaction('🎉')).toBe(true)
    const [payload, opts] = room.localParticipant.publishData.mock.calls[0]
    expect(decodeReaction(payload)).toBe('🎉')
    expect(opts).toEqual({ reliable: false, topic: REACTION_TOPIC })
    expect(t.reactions.value.map((r) => [r.identity, r.emoji])).toEqual([['me', '🎉']])
    // Not from the palette: refused before the wire.
    expect(await t.sendReaction('💩')).toBe(false)
    await t.leave()
    expect(t.reactions.value).toHaveLength(0)
  })

  it("shows a peer's reaction and ignores everything else on the channel", async () => {
    const t = useConfTransport()
    await t.join('c1')
    const room = lk.state.rooms[0]
    const anna = { identity: 'u-anna', name: 'Анна' }
    room.emit('dataReceived', encodeReaction('👏'), anna, 1, REACTION_TOPIC)
    room.emit('dataReceived', encodeReaction('👏'), anna, 1, 'other-topic')
    room.emit(
      'dataReceived',
      new TextEncoder().encode('{"t":"reaction","e":"x"}'),
      anna,
      1,
      REACTION_TOPIC,
    )
    expect(t.reactions.value.map((r) => [r.name, r.emoji])).toEqual([['Анна', '👏']])
    await t.leave()
  })

  it('stops after a burst', async () => {
    const t = useConfTransport()
    await t.join('c1')
    const sent = []
    for (let i = 0; i < REACTION_BURST + 2; i++) sent.push(await t.sendReaction('👍'))
    expect(sent.filter(Boolean)).toHaveLength(REACTION_BURST)
    await t.leave()
  })
})
