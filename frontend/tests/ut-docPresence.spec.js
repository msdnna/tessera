import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { useDocPresence } from '@/composables/useDocPresence'

vi.mock('@/api', () => ({ getAccessToken: () => 'tok' }))

// A WebSocket stand-in that records what the client sent and lets the test push
// server frames back. The real socket is never opened in unit tests.
class FakeSocket {
  static last = null
  // The real WebSocket carries these as class constants and the composable
  // checks readyState against them before sending.
  static CONNECTING = 0
  static OPEN = 1
  static CLOSING = 2
  static CLOSED = 3
  constructor(url, protocols) {
    this.url = url
    this.protocols = protocols
    this.readyState = 1 // OPEN
    this.sent = []
    FakeSocket.last = this
  }
  send(raw) {
    this.sent.push(JSON.parse(raw))
  }
  close() {
    this.readyState = 3
    this.onclose?.()
  }
  // Test helpers.
  serverSends(msg) {
    this.onmessage?.({ data: JSON.stringify(msg) })
  }
  opened() {
    this.onopen?.()
  }
}

const welcome = { type: 'welcome', conn_id: 'conn-me', user_id: 'user-me', lock_ttl_ms: 30000 }

describe('useDocPresence', () => {
  beforeEach(() => {
    vi.stubGlobal('WebSocket', FakeSocket)
    FakeSocket.last = null
  })
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.useRealTimers()
  })

  it('connects to the document socket with the bearer subprotocol', () => {
    const p = useDocPresence()
    p.open('doc-1')
    expect(FakeSocket.last.url).toContain('/api/documents/doc-1/ws')
    // The browser WebSocket API can't set a header, so the token rides as a
    // subprotocol — same trick as the app-wide socket.
    expect(FakeSocket.last.protocols).toEqual(['bearer', 'tok'])
    p.close()
  })

  it('mirrors the room snapshot the server sends', () => {
    const p = useDocPresence()
    p.open('doc-1')
    FakeSocket.last.opened()
    FakeSocket.last.serverSends(welcome)
    FakeSocket.last.serverSends({
      type: 'state',
      viewers: [{ user_id: 'user-me', name: 'Я', conns: 1, blocks: [] }],
      locks: [{ block_id: 'b1', user_id: 'user-2', conn_id: 'conn-2', name: 'Боб' }],
    })
    expect(p.viewers.value).toHaveLength(1)
    expect(p.lockOn('b1').name).toBe('Боб')
    expect(p.isLocked('b2')).toBe(false)
    p.close()
  })

  // Our own lock must not paint our own block as unavailable — otherwise the
  // moment you start typing, the editor locks you out of what you are typing.
  it('does not treat this connection’s own lock as foreign', () => {
    const p = useDocPresence()
    p.open('doc-1')
    FakeSocket.last.opened()
    FakeSocket.last.serverSends(welcome)
    FakeSocket.last.serverSends({
      type: 'state',
      viewers: [],
      locks: [{ block_id: 'b1', user_id: 'user-me', conn_id: 'conn-me', name: 'Я' }],
    })
    expect(p.foreignLocks.value).toEqual([])
    expect(p.isLocked('b1')).toBe(false)
    p.close()
  })

  // The same user's second tab is a different caret, so it has to be treated
  // like anyone else — the connection id is what identifies a holder.
  it('treats the user’s other tab as someone else', () => {
    const p = useDocPresence()
    p.open('doc-1')
    FakeSocket.last.opened()
    FakeSocket.last.serverSends(welcome)
    FakeSocket.last.serverSends({
      type: 'state',
      viewers: [],
      locks: [{ block_id: 'b1', user_id: 'user-me', conn_id: 'conn-other-tab', name: 'Я' }],
    })
    expect(p.isLocked('b1')).toBe(true)
    p.close()
  })

  it('claims a block, releases the previous one and gives it back on release', () => {
    const p = useDocPresence()
    p.open('doc-1')
    const sock = FakeSocket.last
    sock.opened()
    sock.serverSends(welcome)

    p.acquire('b1')
    p.acquire('b2')
    p.release()

    expect(sock.sent).toEqual([
      { type: 'lock', block_id: 'b1' },
      // Moving the caret frees the block behind it instead of leaving a trail.
      { type: 'unlock', block_id: 'b1' },
      { type: 'lock', block_id: 'b2' },
      { type: 'unlock', block_id: 'b2' },
    ])
    p.close()
  })

  it('re-asking for the block it already holds sends nothing', () => {
    const p = useDocPresence()
    p.open('doc-1')
    const sock = FakeSocket.last
    sock.opened()
    sock.serverSends(welcome)
    p.acquire('b1')
    p.acquire('b1')
    expect(sock.sent.filter((m) => m.type === 'lock')).toHaveLength(1)
    p.close()
  })

  it('refreshes the claim while the block stays held', async () => {
    vi.useFakeTimers()
    const p = useDocPresence()
    p.open('doc-1')
    const sock = FakeSocket.last
    sock.opened()
    sock.serverSends(welcome)
    p.acquire('b1')
    // The server drops a lock after its TTL; the heartbeat is what keeps a block
    // held while someone is still typing in it.
    await vi.advanceTimersByTimeAsync(11000)
    expect(
      sock.sent.filter((m) => m.type === 'lock' && m.block_id === 'b1').length,
    ).toBeGreaterThan(1)
    p.close()
  })

  it('drops the claim when the server refuses it', () => {
    const p = useDocPresence()
    p.open('doc-1')
    const sock = FakeSocket.last
    sock.opened()
    sock.serverSends(welcome)
    p.acquire('b1')
    sock.serverSends({ type: 'denied', block_id: 'b1', user_id: 'user-2', name: 'Боб' })
    expect(p.held.value).toBe('')
    expect(p.denied.value).toEqual({ blockId: 'b1', name: 'Боб', userId: 'user-2' })
    p.close()
  })

  // A reconnect gets a *new* connection id, so the block we were holding is now
  // held by nobody — without the re-claim the user goes on typing into a block
  // the server thinks is free.
  it('re-claims the held block after a reconnect', () => {
    const p = useDocPresence()
    p.open('doc-1')
    const first = FakeSocket.last
    first.opened()
    first.serverSends(welcome)
    p.acquire('b1')

    first.serverSends({ ...welcome, conn_id: 'conn-me-2' })
    expect(first.sent.filter((m) => m.type === 'lock' && m.block_id === 'b1')).toHaveLength(2)
    p.close()
  })

  it('clears the roster when the socket drops', () => {
    const p = useDocPresence()
    p.open('doc-1')
    const sock = FakeSocket.last
    sock.opened()
    sock.serverSends(welcome)
    sock.serverSends({
      type: 'state',
      viewers: [{ user_id: 'user-2', name: 'Боб', conns: 1, blocks: ['b1'] }],
      locks: [{ block_id: 'b1', user_id: 'user-2', conn_id: 'conn-2', name: 'Боб' }],
    })
    p.close()
    // Showing a roster we can no longer verify is worse than showing none: the
    // badge would claim Боб is editing long after he closed the tab.
    expect(p.viewers.value).toEqual([])
    expect(p.foreignLocks.value).toEqual([])
  })

  it('counts comment nudges instead of trying to carry the change', () => {
    // The frame has no payload on purpose (#2730): the panel refetches, so a
    // nudge lost to a reconnect costs one stale panel and never a phantom thread.
    const p = useDocPresence()
    p.open('doc-1')
    const sock = FakeSocket.last
    sock.opened()
    sock.serverSends(welcome)
    expect(p.commentsNudge.value).toBe(0)

    sock.serverSends({ type: 'comments' })
    sock.serverSends({ type: 'comments' })
    expect(p.commentsNudge.value).toBe(2)
    // A nudge is not presence: it must not clear the roster or the locks.
    expect(p.connId.value).toBe('conn-me')
    p.close()
  })

  it('reports a colleague’s save so the view can pull their text', () => {
    // The rework of #2729: presence alone made collaboration visible but not
    // workable. This frame is what tells the reader their copy has moved on —
    // it carries a timestamp, not the body, because the room evicts a
    // participant whose buffer fills and documents do not fit in that budget.
    const p = useDocPresence()
    p.open('doc-1')
    const sock = FakeSocket.last
    sock.opened()
    sock.serverSends(welcome)
    expect(p.remoteSave.value).toBeNull()

    sock.serverSends({
      type: 'content.saved',
      updated_at: '2026-08-16T10:00:00Z',
      by_conn: 'conn-mate',
      by_user: 'user-mate',
    })
    expect(p.remoteSave.value).toEqual({
      updatedAt: '2026-08-16T10:00:00Z',
      byUser: 'user-mate',
    })
    // Still a nudge, not a state frame: the roster must survive it.
    expect(p.connId.value).toBe('conn-me')
    p.close()
  })

  it('ignores a save announcement that came from this connection', () => {
    // The server already skips the sender; this covers the gap it cannot —
    // a save that left before the welcome frame arrived comes back with our own
    // id on it, and reacting would mean refetching our own typing in a loop.
    const p = useDocPresence()
    p.open('doc-1')
    const sock = FakeSocket.last
    sock.opened()
    sock.serverSends(welcome)

    sock.serverSends({ type: 'content.saved', by_conn: 'conn-me', by_user: 'user-me' })
    expect(p.remoteSave.value).toBeNull()
    p.close()
  })

  it('does not confuse a rollback with a colleague typing', () => {
    // Two different frames on purpose: `content` means the body was replaced
    // from history and is answered with a hard reload and an announcement,
    // which would be absurd to show on every keystroke of a colleague.
    const p = useDocPresence()
    p.open('doc-1')
    const sock = FakeSocket.last
    sock.opened()
    sock.serverSends(welcome)

    sock.serverSends({ type: 'content' })
    expect(p.contentNudge.value).toBe(1)
    expect(p.remoteSave.value).toBeNull()

    sock.serverSends({ type: 'content.saved', by_conn: 'conn-mate', by_user: 'user-mate' })
    expect(p.contentNudge.value).toBe(1)
    expect(p.remoteSave.value).not.toBeNull()
    p.close()
  })

  it('ignores malformed frames', () => {
    const p = useDocPresence()
    p.open('doc-1')
    const sock = FakeSocket.last
    sock.opened()
    expect(() => sock.onmessage({ data: 'not json' })).not.toThrow()
    p.close()
  })
})
