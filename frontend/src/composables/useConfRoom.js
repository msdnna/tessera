import { computed, ref, shallowRef } from 'vue'
import { confWsURL } from '@/utils/serverBase'
import { getAccessToken } from '@/api'

// The conference room socket (#2864, subtask #2872) — everything in a call that
// is not media.
//
// Two connections, on purpose. Audio and video go straight to the SFU through
// useConfTransport; this one carries the room's own bookkeeping: who is here,
// who has a hand up, who holds the screen-share stage, and the moderation
// commands. They are separate because the SFU has no opinion about any of it —
// LiveKit only verifies a token signature, so "may this person kick that one"
// can only be answered by our server.
//
// The server sends a whole-room snapshot on every change rather than a delta
// (the same contract as the document socket), which is what makes a reconnect
// self-healing: the new state overwrites the old one and a frame lost to a
// dropped connection cannot leave a participant on screen who left ten minutes
// ago.

const RECONNECT_BASE = 1000 // ms
const RECONNECT_MAX = 15000 // ms

/**
 * Live state and commands for one conference room.
 *
 * The caller drives it with `open(conferenceId)` / `close()`, reads
 * `participants` / `stage` / `queue`, and acts through `setMedia`, `raiseHand`,
 * `kick` and `forceMute`. Permission is never checked here: a member's kick is
 * sent, refused by the server and answered with `denied`. Hiding the button is
 * a courtesy to honest users, not a security boundary.
 *
 * @returns {object} state refs plus the command functions
 */
export function useConfRoom() {
  const participants = ref([])
  const stage = shallowRef(null)
  const queue = ref([])
  const connected = ref(false)
  const connId = ref('')
  const userId = ref('')
  const role = ref('member')
  // The last refusal from the server: {action, reason}. Shown once and cleared
  // by the caller — a stale "not a host" under a button nobody pressed is worse
  // than no message at all.
  const denied = shallowRef(null)
  // Set when the call is over or we were removed, with the server's reason
  // ('ended', 'deleted', 'kicked'). The room screen reads it to say which.
  const ended = shallowRef(null)
  // Bumped when the server says the chat changed (#2873). Payload-free by
  // design: messages are fetched over HTTP, so a nudge lost to a reconnect
  // costs a stale panel rather than a phantom message.
  const chatNudge = ref(0)

  let ws = null
  let confId = ''
  let retry = null
  let attempts = 0
  let closed = true
  // Last media state we reported, so a reconnect can restate it: the server
  // starts every connection with mic and camera off, and a client that stayed
  // silent would show as muted while actually talking.
  let media = { mic: false, cam: false }

  const isHost = computed(() => role.value === 'host')
  /** Me, as the room sees me — the source of truth for my own force-mute. */
  const self = computed(() => participants.value.find((p) => p.user_id === userId.value) || null)
  /** Whether a host has silenced me. The room refuses my mic while it is true. */
  const forceMuted = computed(() => !!self.value?.force_muted)

  function send(msg) {
    if (ws && ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify(msg))
  }

  /**
   * Report this connection's microphone and camera state.
   *
   * The room is not the source of truth for the tracks — the SFU is — but the
   * roster has to paint a muted badge before the first audio packet arrives,
   * and a force-muted participant reporting a live microphone is refused rather
   * than believed.
   */
  function setMedia(mic, cam) {
    media = { mic: !!mic, cam: !!cam }
    send({ type: 'media', ...media })
  }

  /** Raise or lower a hand; the server orders the list by when it went up. */
  function raiseHand(up) {
    send({ type: 'hand', up: !!up })
  }

  /** Remove someone from the call (hosts only — the server decides). */
  function kick(targetUserId) {
    denied.value = null
    send({ type: 'kick', user_id: targetUserId })
  }

  /** Silence someone for everyone, or lift it (hosts only). */
  function forceMuteUser(targetUserId, muted) {
    denied.value = null
    send({ type: 'mute', user_id: targetUserId, muted: !!muted })
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
      role.value = msg.role || 'member'
      // A reconnect is a fresh connection to the server, which knows nothing
      // about the devices we already have open.
      if (media.mic || media.cam) send({ type: 'media', ...media })
      return
    }
    if (msg.type === 'state') {
      participants.value = msg.participants || []
      stage.value = msg.stage || null
      queue.value = msg.queue || []
      return
    }
    if (msg.type === 'denied') {
      denied.value = { action: msg.action || '', reason: msg.reason || '' }
      return
    }
    if (msg.type === 'chat') {
      chatNudge.value += 1
      return
    }
    if (msg.type === 'ended') {
      // Not a transport failure, so it must not be retried: reconnecting into a
      // call we were thrown out of would loop against a 403, and reconnecting
      // into one that ended would show a room of ghosts.
      ended.value = { reason: msg.reason || 'ended' }
      close()
    }
  }

  function scheduleReconnect() {
    if (closed) return
    // Full-jitter backoff, as everywhere else: a backend restart must not have
    // every open call reconnect in lockstep.
    const cap = Math.min(RECONNECT_MAX, RECONNECT_BASE * 2 ** attempts)
    attempts += 1
    retry = setTimeout(connect, Math.random() * cap)
  }

  function connect() {
    if (closed || !confId) return
    // Read the token per attempt: a refresh-on-401 may have rotated it since.
    const token = getAccessToken()
    if (!token) {
      scheduleReconnect()
      return
    }
    ws = new WebSocket(confWsURL(confId), ['bearer', token])
    ws.onopen = () => {
      attempts = 0
      connected.value = true
    }
    ws.onmessage = (e) => onMessage(e.data)
    ws.onclose = () => {
      connected.value = false
      // Showing a stale roster while offline is worse than showing none: the
      // panel would offer a mute button for someone who may have left.
      participants.value = []
      stage.value = null
      queue.value = []
      scheduleReconnect()
    }
    ws.onerror = () => ws && ws.close()
  }

  /** Opens the socket for a conference, replacing any previous one. */
  function open(id) {
    close()
    if (!id) return
    confId = id
    closed = false
    attempts = 0
    ended.value = null
    connect()
  }

  /** Closes the socket and forgets the room. */
  function close() {
    closed = true
    confId = ''
    media = { mic: false, cam: false }
    denied.value = null
    clearTimeout(retry)
    retry = null
    if (ws) {
      ws.onclose = null
      ws.close()
      ws = null
    }
    connected.value = false
    participants.value = []
    stage.value = null
    queue.value = []
  }

  return {
    participants,
    stage,
    queue,
    connected,
    connId,
    userId,
    role,
    isHost,
    self,
    forceMuted,
    denied,
    ended,
    chatNudge,
    open,
    close,
    setMedia,
    raiseHand,
    kick,
    forceMute: forceMuteUser,
  }
}
