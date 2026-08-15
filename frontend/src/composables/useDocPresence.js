import { computed, ref, shallowRef } from 'vue'
import { docWsURL } from '@/utils/serverBase'
import { getAccessToken } from '@/api'

const RECONNECT_BASE = 1000 // ms
const RECONNECT_MAX = 15000 // ms

// While the caret stays in a block the client re-sends its lock, so the server's
// TTL only fires for a client that actually died. Refreshing at a third of the
// TTL leaves room for two lost frames before anyone else sees the block go free.
const REFRESH_FRACTION = 3

/**
 * Presence and per-block locks for one open document (#2729).
 *
 * The document socket is a separate connection from the app-wide realtime one:
 * that hub is workspace-scoped and read-only, while this one is per document and
 * carries requests (a lock has to be asked for and can be refused).
 *
 * The server answers with a whole-room snapshot on every change rather than a
 * delta, so nothing here has to reconcile state — a reconnect simply overwrites
 * what we had, which is also why a dropped frame cannot leave a phantom "editing"
 * badge behind.
 *
 * The caller drives it with `open(docId)` / `close()` and asks for a block with
 * `acquire(blockId)`. Acquisition is optimistic: the caller may keep typing while
 * the answer is in flight, and a refusal arrives as `denied`.
 *
 * @returns {object} state refs and `open`, `close`, `acquire`, `release`
 */
export function useDocPresence() {
  const viewers = ref([])
  const locks = ref([])
  const connId = ref('')
  const userId = ref('')
  const connected = ref(false)
  // The block this client is holding (or trying to). Kept separate from `locks`
  // so a refusal can be shown without waiting for the next snapshot.
  const held = ref('')
  const denied = shallowRef(null)
  // Bumped when the server says this document's comment threads changed (#2730).
  const commentsNudge = ref(0)

  let ws = null
  let docId = ''
  let retry = null
  let refresh = null
  let attempts = 0
  let closed = true
  let ttlMs = 30000

  // Locks held by anyone but this connection. A user's *other* tab counts as
  // someone else: it is a different caret in the same document.
  const foreignLocks = computed(() => locks.value.filter((l) => l.conn_id !== connId.value))

  /** Who holds this block, or null when it is free (or held by us). */
  function lockOn(blockId) {
    return foreignLocks.value.find((l) => l.block_id === blockId) || null
  }

  /** Whether this block is off limits right now. */
  function isLocked(blockId) {
    return !!lockOn(blockId)
  }

  function send(msg) {
    if (ws && ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify(msg))
  }

  /**
   * Claims a block for editing, refreshing the claim while it stays held.
   * Asking for the block we already hold is the refresh path and is silent.
   */
  function acquire(blockId) {
    if (!blockId || blockId === held.value) return
    if (held.value) send({ type: 'unlock', block_id: held.value })
    held.value = blockId
    denied.value = null
    send({ type: 'lock', block_id: blockId })
    startRefresh()
  }

  /** Gives the block back, so the next person doesn't wait out the TTL. */
  function release() {
    if (!held.value) return
    send({ type: 'unlock', block_id: held.value })
    held.value = ''
    stopRefresh()
  }

  function startRefresh() {
    stopRefresh()
    refresh = setInterval(
      () => {
        if (held.value) send({ type: 'lock', block_id: held.value })
      },
      Math.max(1000, ttlMs / REFRESH_FRACTION),
    )
  }

  function stopRefresh() {
    if (refresh) clearInterval(refresh)
    refresh = null
  }

  function onMessage(raw) {
    let msg
    try {
      msg = JSON.parse(raw)
    } catch {
      return // malformed frame
    }
    if (msg.type === 'welcome') {
      connId.value = msg.conn_id || ''
      userId.value = msg.user_id || ''
      if (msg.lock_ttl_ms) ttlMs = msg.lock_ttl_ms
      // A reconnect lands here with a *new* connection id, so the block we were
      // holding is now held by nobody. Re-claim it rather than leaving the user
      // typing into a block the server thinks is free.
      if (held.value) {
        send({ type: 'lock', block_id: held.value })
        startRefresh()
      }
      return
    }
    if (msg.type === 'state') {
      viewers.value = msg.viewers || []
      locks.value = msg.locks || []
      return
    }
    if (msg.type === 'comments') {
      // A payload-free nudge: someone's annotation changed. The counter is what
      // the panel watches — it refetches rather than being handed a delta, so a
      // nudge lost to a reconnect costs one stale panel, not a phantom thread.
      commentsNudge.value += 1
      return
    }
    if (msg.type === 'denied') {
      // We lost the race for this block. Drop the claim so the editor stops
      // trying to refresh a lock it does not have.
      if (msg.block_id === held.value) {
        held.value = ''
        stopRefresh()
      }
      denied.value = { blockId: msg.block_id, name: msg.name, userId: msg.user_id }
    }
  }

  function scheduleReconnect() {
    if (closed) return
    // Same full-jitter backoff as the app-wide socket: many tabs must not
    // reconnect in lockstep after a backend restart.
    const cap = Math.min(RECONNECT_MAX, RECONNECT_BASE * 2 ** attempts)
    attempts += 1
    retry = setTimeout(connect, Math.random() * cap)
  }

  function connect() {
    if (closed || !docId) return
    // Read the token per attempt: a refresh-on-401 may have rotated it since.
    const token = getAccessToken()
    if (!token) {
      scheduleReconnect()
      return
    }
    ws = new WebSocket(docWsURL(docId), ['bearer', token])
    ws.onopen = () => {
      attempts = 0
      connected.value = true
    }
    ws.onmessage = (e) => onMessage(e.data)
    ws.onclose = () => {
      connected.value = false
      // Nobody is in the room as far as we know until the next snapshot; showing
      // a stale roster while offline is worse than showing none.
      viewers.value = []
      locks.value = []
      scheduleReconnect()
    }
    ws.onerror = () => ws && ws.close()
  }

  /** Opens the socket for a document, replacing any previous one. */
  function open(id) {
    close()
    if (!id) return
    docId = id
    closed = false
    attempts = 0
    connect()
  }

  /** Closes the socket and forgets the room. */
  function close() {
    closed = true
    docId = ''
    held.value = ''
    denied.value = null
    stopRefresh()
    clearTimeout(retry)
    retry = null
    if (ws) {
      ws.onclose = null
      ws.close()
      ws = null
    }
    connected.value = false
    viewers.value = []
    locks.value = []
  }

  return {
    viewers,
    locks,
    foreignLocks,
    connected,
    connId,
    userId,
    held,
    denied,
    commentsNudge,
    open,
    close,
    acquire,
    release,
    lockOn,
    isLocked,
  }
}
