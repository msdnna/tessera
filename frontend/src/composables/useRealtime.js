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

  onMounted(connect)
  onUnmounted(() => {
    closed = true
    clearTimeout(retry)
    if (ws) {
      ws.onclose = null
      ws.close()
    }
  })
}
