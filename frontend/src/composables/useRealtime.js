import { onMounted, onUnmounted } from 'vue'
import { wsURL } from '@/utils/serverBase'
import { getAccessToken } from '@/api'

// useRealtime opens the /api/ws WebSocket and invokes `onEvent({scope,type,data})`
// for every server broadcast. Auto-reconnects with exponential backoff (capped,
// jittered) so a backend restart or a flapping link doesn't hammer the server
// with a fixed-interval retry storm from every open tab.
//
// The socket is authenticated: the server only sends events for workspaces the
// user belongs to, and refuses the handshake without a valid token. The browser
// WebSocket API can't set an Authorization header, so the token rides as a
// subprotocol (`['bearer', token]`) — a header, unlike a query param, which
// would end up in the server's access log. The caller still filters by scope to
// pick out the workspace it is currently viewing.
const RECONNECT_BASE = 1000 // ms
const RECONNECT_MAX = 30000 // ms

// onResync (optional) is called when the caller should reload its current view
// from scratch: either the server sent a `resync` marker (it had to drop an event
// because our buffer overflowed), or we just reconnected after an outage and
// missed whatever happened while the socket was down. Live deltas alone can't
// close either gap.
//
// `hello` (optional) returns a client→server frame to send on every open — the
// board's presence (#3116): a reconnect re-announces it, since the server took
// the old socket off the board when it closed. The returned `send` sends a
// frame now if the socket is open (the next open sends `hello` anyway).
export function useRealtime(onEvent, onResync, { hello = null } = {}) {
  let ws = null
  let retry = null
  let attempts = 0
  let closed = false
  let everConnected = false

  function scheduleReconnect() {
    if (closed) return
    // Exponential backoff with full jitter: base*2^n capped, then randomised in
    // [0, cap] so many tabs don't reconnect in lockstep after an outage.
    const cap = Math.min(RECONNECT_MAX, RECONNECT_BASE * 2 ** attempts)
    attempts += 1
    retry = setTimeout(connect, Math.random() * cap)
  }

  // discard lets go of a socket we are replacing (or unmounting): detach every
  // handler, then close it. Detaching is the point — a socket closed but still
  // wired keeps calling back long after we stopped listening to it. Its onclose
  // would schedule a reconnect on top of the live replacement, leaving that
  // replacement open with nothing referencing it (events arrive twice; unmount
  // can no longer close it), and its onerror would close whatever `ws` points at
  // by then, which is someone else's socket.
  function discard(sock) {
    if (!sock) return
    sock.onopen = null
    sock.onmessage = null
    sock.onclose = null
    sock.onerror = null
    sock.close()
  }

  function connect() {
    // Read the token on every (re)connect rather than once: a refresh-on-401
    // may have rotated it since the last attempt, and the reconnect is what
    // picks the new one up.
    const token = getAccessToken()
    if (!token) {
      // Mid-refresh (or logged out): retry on the same backoff instead of
      // going permanently silent.
      scheduleReconnect()
      return
    }
    // Whatever we held is not ours any more. Every path that reaches a new
    // socket goes through here, so this is the one place that has to let go of
    // the old one — including reconnectNow's, where the old socket is still
    // CLOSING and its onclose has not fired yet.
    discard(ws)
    // Web: ws(s)://<location.host>/api/ws. Desktop (Tauri): derived from the
    // configured server origin. See utils/serverBase.js.
    // Handlers close over `sock`, not `ws`: by the time a late frame arrives,
    // `ws` may already be the replacement.
    const sock = new WebSocket(wsURL(), ['bearer', token])
    ws = sock
    sock.onopen = () => {
      attempts = 0 // healthy connection → reset the backoff
      // A reconnect (not the first open) means we were offline for a while and
      // missed events; reload rather than resume mid-stream. The first open is
      // the initial load, which the caller already did.
      if (everConnected) onResync?.()
      everConnected = true
      send(hello?.())
    }
    sock.onmessage = (e) => {
      try {
        const msg = JSON.parse(e.data)
        // The server dropped at least one event for us; reload the view instead
        // of applying deltas around a hole.
        if (msg?.type === 'resync') {
          onResync?.()
          return
        }
        onEvent(msg)
      } catch {
        // ignore malformed frames
      }
    }
    sock.onclose = () => {
      if (!closed) scheduleReconnect()
    }
    sock.onerror = () => sock.close()
  }

  function send(frame) {
    if (!frame || !ws || ws.readyState !== WebSocket.OPEN) return
    try {
      ws.send(JSON.stringify(frame))
    } catch {
      // a closing socket — the reconnect's hello carries it
    }
  }

  // The OS can silently kill the socket on sleep/hibernate (notably when the
  // desktop app is minimised to the tray); the backoff eventually recovers, but
  // reconnect immediately when the machine comes back online or the window is
  // shown again so live events resume without a lag.
  function reconnectNow() {
    if (closed) return
    const dead = !ws || ws.readyState === WebSocket.CLOSED || ws.readyState === WebSocket.CLOSING
    if (dead) {
      clearTimeout(retry)
      attempts = 0 // user-triggered recovery → reconnect immediately, no backoff
      connect()
    }
  }
  function onVisible() {
    if (document.visibilityState === 'visible') reconnectNow()
  }

  onMounted(() => {
    connect()
    window.addEventListener('online', reconnectNow)
    document.addEventListener('visibilitychange', onVisible)
  })
  onUnmounted(() => {
    closed = true
    clearTimeout(retry)
    window.removeEventListener('online', reconnectNow)
    document.removeEventListener('visibilitychange', onVisible)
    discard(ws)
    ws = null
  })
  return { send }
}
