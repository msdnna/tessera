import { onMounted, onUnmounted } from 'vue'
import { wsURL } from '@/utils/serverBase'

// useRealtime opens the /api/ws WebSocket and invokes `onEvent({scope,type,data})`
// for every server broadcast. Auto-reconnects with a fixed backoff. The caller
// filters events by scope (workspace id) itself.
export function useRealtime(onEvent) {
  let ws = null
  let retry = null
  let closed = false

  function connect() {
    // Web: ws(s)://<location.host>/api/ws. Desktop (Tauri): derived from the
    // configured server origin. See utils/serverBase.js.
    ws = new WebSocket(wsURL())
    ws.onmessage = (e) => {
      try {
        onEvent(JSON.parse(e.data))
      } catch {
        // ignore malformed frames
      }
    }
    ws.onclose = () => {
      if (!closed) retry = setTimeout(connect, 3000)
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
