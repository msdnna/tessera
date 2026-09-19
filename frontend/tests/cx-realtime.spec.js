import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { defineComponent } from 'vue'
import { mount } from '@vue/test-utils'

vi.mock('@/utils/serverBase', () => ({ wsURL: () => 'ws://tessera.test/api/ws' }))

// The access token lives in the api module since #2684 (it used to be read from
// localStorage here). Mocked so this spec doesn't pull in axios.
const { getAccessToken, setToken } = vi.hoisted(() => {
  let token = ''
  return { getAccessToken: () => token, setToken: (v) => (token = v) }
})
vi.mock('@/api', () => ({ getAccessToken }))

import { useRealtime } from '@/composables/useRealtime'

// Records every `new WebSocket(url, protocols)` so the tests can assert what
// credential the socket was opened with.
const opened = []

class FakeWebSocket {
  static CLOSED = 3
  static CLOSING = 2
  constructor(url, protocols) {
    this.url = url
    this.protocols = protocols
    this.readyState = 1
    // close() is not instantaneous in a browser: the socket goes to CLOSING and
    // only reaches CLOSED (and fires onclose) once the handshake finishes. The
    // gap between the two is where a reconnect can race, so model it.
    this.close = vi.fn(() => {
      if (this.readyState !== FakeWebSocket.CLOSED) this.readyState = FakeWebSocket.CLOSING
    })
    opened.push(this)
  }
  // Drive the composable's handler as the browser would on a dropped socket.
  drop() {
    this.readyState = FakeWebSocket.CLOSED
    this.onclose?.()
  }
  // Finish a close handshake that close() started.
  finishClose() {
    this.drop()
  }
}

// Mount a throwaway component so the composable's onMounted/onUnmounted run.
function mountRealtime(onEvent = () => {}, onResync = () => {}) {
  return mount(
    defineComponent({
      setup() {
        useRealtime(onEvent, onResync)
        return () => null
      },
    }),
  )
}

describe('useRealtime', () => {
  let origWS
  beforeEach(() => {
    opened.length = 0
    origWS = globalThis.WebSocket
    globalThis.WebSocket = FakeWebSocket
    localStorage.clear()
    setToken('')
    vi.useFakeTimers()
  })
  afterEach(() => {
    globalThis.WebSocket = origWS
    vi.useRealTimers()
  })

  // The socket is authenticated: the browser WebSocket API can't set headers,
  // so the token rides as the second subprotocol. A query param would leak it
  // into the access log.
  it('opens the socket with the bearer subprotocol carrying the token', () => {
    setToken('jwt-abc')
    const w = mountRealtime()
    expect(opened).toHaveLength(1)
    expect(opened[0].protocols).toEqual(['bearer', 'jwt-abc'])
    expect(opened[0].url).not.toContain('jwt-abc') // never in the URL
    w.unmount()
  })

  it('does not open an unauthenticated socket when there is no token', () => {
    const w = mountRealtime()
    expect(opened).toHaveLength(0)
    w.unmount()
  })

  // A missing token means "mid-refresh", not "give up": the retry must keep
  // running or the board goes permanently silent after a token rotation.
  it('retries after a missing token and picks up the rotated one', () => {
    const w = mountRealtime()
    expect(opened).toHaveLength(0)
    setToken('jwt-fresh')
    vi.advanceTimersByTime(60000)
    expect(opened.length).toBeGreaterThan(0)
    expect(opened.at(-1).protocols).toEqual(['bearer', 'jwt-fresh'])
    w.unmount()
  })

  // The server cuts the socket on any membership change (Hub.DropUser), and a
  // refresh-on-401 rotates the access token — so the reconnect must use the
  // token as it is *now*, not the one captured at mount.
  it('re-reads the token on reconnect rather than reusing the first one', () => {
    setToken('jwt-old')
    const w = mountRealtime()
    expect(opened[0].protocols).toEqual(['bearer', 'jwt-old'])

    setToken('jwt-new')
    opened[0].drop()
    vi.advanceTimersByTime(60000)

    expect(opened).toHaveLength(2)
    expect(opened[1].protocols).toEqual(['bearer', 'jwt-new'])
    w.unmount()
  })

  // A `resync` marker means the server dropped an event for us; the caller must
  // reload its view (onResync), and the marker itself must not reach onEvent.
  it('routes a resync marker to onResync, not onEvent', () => {
    setToken('jwt')
    const onEvent = vi.fn()
    const onResync = vi.fn()
    const w = mountRealtime(onEvent, onResync)

    opened[0].onopen() // first open — initial load, no resync
    opened[0].onmessage({ data: JSON.stringify({ type: 'resync' }) })

    expect(onResync).toHaveBeenCalledTimes(1)
    expect(onEvent).not.toHaveBeenCalled()
    w.unmount()
  })

  // A reconnect means we were offline and missed events; reload rather than
  // resume mid-stream. The very first open is the initial load and must not.
  it('calls onResync on reconnect but not on the first open', () => {
    setToken('jwt')
    const onResync = vi.fn()
    const w = mountRealtime(() => {}, onResync)

    opened[0].onopen()
    expect(onResync).not.toHaveBeenCalled()

    opened[0].drop()
    vi.advanceTimersByTime(60000)
    expect(opened).toHaveLength(2)
    opened[1].onopen()

    expect(onResync).toHaveBeenCalledTimes(1)
    w.unmount()
  })

  // Waking from sleep is exactly the case reconnectNow was written for, and it
  // arrives with the old socket half-dead: onerror has already called close(),
  // so the socket sits in CLOSING while `online` fires. Replacing it must also
  // let go of it — otherwise its onclose still fires on the way out, schedules a
  // reconnect on top of the live replacement, and leaves that replacement open
  // with nothing referencing it: every event arrives twice and unmount can no
  // longer close it.
  it('lets go of the closing socket it replaces on wake-up', () => {
    setToken('jwt')
    const onEvent = vi.fn()
    const w = mountRealtime(onEvent)
    const first = opened[0]
    first.onopen()

    first.onerror() // the link died; the composable closes the socket
    expect(first.readyState).toBe(FakeWebSocket.CLOSING)

    window.dispatchEvent(new Event('online'))
    expect(opened).toHaveLength(2)
    const live = opened[1]
    live.onopen()

    // The old socket's close handshake completes only now.
    first.finishClose()
    vi.advanceTimersByTime(60000)

    expect(opened).toHaveLength(2) // no third socket behind the live one
    live.onmessage({ data: JSON.stringify({ scope: 's', type: 'task.updated' }) })
    expect(onEvent).toHaveBeenCalledTimes(1) // delivered once, not twice

    w.unmount()
    expect(live.close).toHaveBeenCalled() // unmount still owns the live socket
  })

  // Same trap, one step further: the discarded socket's own handlers must not
  // reach back through the composable's `ws` binding and close the replacement.
  it('does not let a discarded socket error the live one shut', () => {
    setToken('jwt')
    const w = mountRealtime()
    const first = opened[0]
    first.onopen()

    first.onerror()
    window.dispatchEvent(new Event('online'))
    const live = opened[1]
    live.onopen()

    first.onerror?.() // a late error frame on the socket we walked away from
    expect(live.readyState).toBe(1) // the live socket is untouched

    w.unmount()
  })
})
