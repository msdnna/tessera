import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

// What a workspace role change does to a call already open (#2932).
//
// The seat — my call role and whether the server will take a kick from me — is
// resolved once, server-side, at the upgrade, and arrives in the `welcome`
// frame. Nothing else on the wire carries it: `can_moderate` is not in the
// roster snapshot, so a promotion or a demotion mid-call reached this client
// through nothing at all, and the panel kept painting the controls the welcome
// frame had granted until the person left the meeting and came back.
//
// `seat` is the server correcting that in place. The security half is the
// server's — a demoted admin's kick is refused whether or not this file exists —
// so what is being pinned here is the other half: that the controls follow the
// right instead of outliving it.

const api = {
  token: vi.fn(() => Promise.reject(new Error('no media in jsdom'))),
}
vi.mock('@/api', () => ({
  conferences: api,
  getAccessToken: () => 'jwt',
  apiBaseURL: () => '/api',
}))

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

const { useConfRoom } = await import('@/composables/useConfRoom')

beforeEach(() => {
  sockets.length = 0
  vi.stubGlobal('WebSocket', FakeSocket)
})
afterEach(() => {
  vi.unstubAllGlobals()
})

// A room already joined as a workspace admin: a plain member of the call, with
// moderation granted on top — exactly the seat conferenceSeat hands out.
function joinedAsAdmin() {
  const room = useConfRoom()
  room.open('c1')
  const ws = sockets.at(-1)
  ws.onopen()
  ws.deliver({
    type: 'welcome',
    conn_id: 'c-1',
    user_id: 'me',
    role: 'member',
    can_moderate: true,
  })
  return { room, ws }
}

describe('a workspace role change during a call', () => {
  it('takes the moderation controls when the seat says so', () => {
    const { room, ws } = joinedAsAdmin()
    expect(room.canModerate.value).toBe(true)

    ws.deliver({ type: 'seat', role: 'member', can_moderate: false })

    expect(room.canModerate.value).toBe(false)
  })

  it('hands them over on a promotion, without a rejoin', () => {
    const room = useConfRoom()
    room.open('c1')
    const ws = sockets.at(-1)
    ws.onopen()
    ws.deliver({
      type: 'welcome',
      conn_id: 'c-1',
      user_id: 'me',
      role: 'member',
      can_moderate: false,
    })
    expect(room.canModerate.value).toBe(false)

    ws.deliver({ type: 'seat', role: 'member', can_moderate: true })

    expect(room.canModerate.value).toBe(true)
  })

  it('leaves the rest of the room alone', () => {
    const { room, ws } = joinedAsAdmin()
    ws.deliver({
      type: 'state',
      participants: [
        { user_id: 'me', name: 'Я', role: 'member', conns: 1 },
        { user_id: 'u-2', name: 'Ира', role: 'host', conns: 1 },
      ],
      stage: { user_id: 'u-2', conn_id: 'c-2', name: 'Ира', since: '2026-09-20T00:00:00Z' },
      queue: [],
    })
    const seq = room.stateSeq.value

    ws.deliver({ type: 'seat', role: 'member', can_moderate: false })

    // The seat is about this connection and nothing else: a frame that quietly
    // emptied the roster or dropped the presenter would blank the call screen
    // on a membership edit somebody else made.
    expect(room.participants.value).toHaveLength(2)
    expect(room.stage.value?.user_id).toBe('u-2')
    expect(room.stateSeq.value).toBe(seq)
    expect(room.connId.value).toBe('c-1')
    expect(room.userId.value).toBe('me')
  })

  it('carries the call role too, when the seat changes it', () => {
    const { room, ws } = joinedAsAdmin()
    expect(room.isHost.value).toBe(false)

    ws.deliver({ type: 'seat', role: 'host', can_moderate: true })

    expect(room.isHost.value).toBe(true)
  })

  it('does not survive leaving the call', () => {
    const { room, ws } = joinedAsAdmin()
    ws.deliver({ type: 'seat', role: 'member', can_moderate: true })
    expect(room.canModerate.value).toBe(true)

    room.close()

    // The next room gets its rights from its own welcome frame. A `true` carried
    // across would flash the kick controls over somebody else's meeting.
    expect(room.canModerate.value).toBe(false)
  })

  it('is re-read from the welcome frame after a reconnect', () => {
    const { room, ws } = joinedAsAdmin()
    ws.deliver({ type: 'seat', role: 'member', can_moderate: false })
    expect(room.canModerate.value).toBe(false)

    // A reconnect resolves the seat afresh, server-side, off the membership as
    // it stands now — so the welcome frame is the authority again, and a seat
    // correction from the previous connection must not outlive it.
    ws.deliver({
      type: 'welcome',
      conn_id: 'c-2',
      user_id: 'me',
      role: 'member',
      can_moderate: true,
    })

    expect(room.canModerate.value).toBe(true)
  })
})
