import { onMounted, onUnmounted } from 'vue'
import { wsURL } from '@/utils/serverBase'

// useRealtime opens the /api/ws WebSocket and invokes `onEvent({scope,type,data})`
// for every server broadcast. Auto-reconnects with exponential backoff (capped,
// jittered) so a backend restart or a flapping link doesn't hammer the server
// with a fixed-interval retry storm from every open tab. The caller filters
// events by scope (workspace id) itself.
const RECONNECT_BASE = 1000 // ms
const RECONNECT_MAX = 30000 // ms

export function useRealtime(onEvent) {
  let ws = null
  let retry = null
  let attempts = 0
  let closed = false

  function scheduleReconnect() {
    if (closed) return
    // Exponential backoff with full jitter: base*2^n capped, then randomised in
    // [0, cap] so many tabs don't reconnect in lockstep after an outage.
    const cap = Math.min(RECONNECT_MAX, RECONNECT_BASE * 2 ** attempts)
    attempts += 1
    retry = setTimeout(connect, Math.random() * cap)
  }

  function connect() {
    // Web: ws(s)://<location.host>/api/ws. Desktop (Tauri): derived from the
    // configured server origin. See utils/serverBase.js.
    ws = new WebSocket(wsURL())
    ws.onopen = () => {
      attempts = 0 // healthy connection → reset the backoff
    }
    ws.onmessage = (e) => {
      try {
        onEvent(JSON.parse(e.data))
      } catch {
        // ignore malformed frames
      }
    }
    ws.onclose = () => {
      if (!closed) scheduleReconnect()
    }
    ws.onerror = () => ws && ws.close()
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
    if (ws) {
      ws.onclose = null
      ws.close()
    }
  })
}
