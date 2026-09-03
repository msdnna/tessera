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
 * `requestScreen` / `releaseScreen`, `kick` and `forceMute`. Permission is never checked here: a member's kick is
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
  // Whether the server will accept a kick/force-mute from us (#2878). Kept apart
  // from the host role: a workspace admin is a plain member of the call but may
  // still moderate it, so the panel shows its controls on this, not on isHost.
  const canModerate = ref(false)
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
  // Counts snapshots (#2874). The screen-share flow needs to tell "the server
  // has not answered my request yet" from "the server answered and the stage is
  // someone else's" — the two look identical in `stage` alone, and acting on the
  // first would kill a share the moment it started.
  const stateSeq = ref(0)
  // How long the stage survives without a refresh, straight from the server, so
  // the presenter's heartbeat cannot drift out of step with the TTL it feeds.
  const stageTtlMs = ref(30000)
  // The red dot (#2877): {id, started_at, started_by}, or null when nothing is
  // being recorded. It arrives in the snapshot rather than in a reply to whoever
  // pressed the button, which is the whole point — the people who did NOT start
  // it are the ones who have to be told.
  const recording = shallowRef(null)

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
  // The stage belongs to a *connection*, not a person: the same user with the
  // call open in two tabs must not have the second one believe it is presenting
  // and offer a stop-sharing button for a screen it is not publishing.
  const presenting = computed(() => !!stage.value && stage.value.conn_id === connId.value)
  /** Our place in the line, 1-based; 0 when we are not waiting for the stage. */
  const queuePos = computed(() => queue.value.findIndex((q) => q.conn_id === connId.value) + 1)

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

  /**
   * Ask for the screen-share stage (#2874).
   *
   * The answer arrives as a snapshot, never as a return value: the server is the
   * only arbiter, and a client that assumed it had the stage because it asked is
   * exactly how two people end up presenting at once. A host asking while a
   * member presents preempts them — that decision is the server's too.
   */
  function requestScreen() {
    send({ type: 'screen.request' })
  }

  /** Keep our hold on the stage alive; ignored by the server for anyone else. */
  function refreshScreen() {
    send({ type: 'screen.refresh' })
  }

  /** Give the stage up, or drop out of the queue if we were only waiting. */
  function releaseScreen() {
    send({ type: 'screen.release' })
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
      canModerate.value = !!msg.can_moderate
      if (msg.stage_ttl_ms > 0) stageTtlMs.value = msg.stage_ttl_ms
      // A reconnect is a fresh connection to the server, which knows nothing
      // about the devices we already have open.
      if (media.mic || media.cam) send({ type: 'media', ...media })
      return
    }
    if (msg.type === 'state') {
      participants.value = msg.participants || []
      stage.value = msg.stage || null
      queue.value = msg.queue || []
      recording.value = msg.recording || null
      stateSeq.value += 1
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
      // The recording dot is the one thing NOT cleared here, and the asymmetry is
      // deliberate (#2877). This socket carries bookkeeping; the media goes to the
      // SFU on a connection of its own, so losing this one does not stop the
      // recording — it only stops us hearing about it. Clearing the dot would
      // tell the room the recording ended when the only thing that ended was our
      // own connection, and people say things off the record they would not say
      // on it. The next snapshot after a reconnect corrects it either way.
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
    // Drop the moderation grant with the socket: a stale "true" would flash the
    // kick controls for a beat when the next room opens before its welcome lands.
    canModerate.value = false
    participants.value = []
    stage.value = null
    queue.value = []
    // Cleared here, unlike on a dropped socket above: this is leaving the call,
    // not losing touch with it, and a dot carried into the next room would
    // announce a recording that belongs to a meeting we are no longer in.
    recording.value = null
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
    canModerate,
    self,
    forceMuted,
    presenting,
    queuePos,
    stateSeq,
    stageTtlMs,
    recording,
    denied,
    ended,
    chatNudge,
    open,
    close,
    setMedia,
    raiseHand,
    requestScreen,
    refreshScreen,
    releaseScreen,
    kick,
    forceMute: forceMuteUser,
  }
}
