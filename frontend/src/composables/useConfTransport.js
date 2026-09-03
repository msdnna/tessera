import { computed, getCurrentInstance, onBeforeUnmount, ref, shallowRef } from 'vue'
import { conferences as confApi } from '@/api'

// The seam between the conference UI and the SFU (#2864, subtask #2871).
//
// Everything the room screen knows about media goes through here, and nothing
// above this file imports `livekit-client`. That is not ceremony: the SDK is the
// one dependency in this feature we do not control, and its participant/track
// API has already been renamed once across a major version. Keeping it behind a
// plain `{ status, peers, micOn, … }` surface means a version bump touches one
// file instead of five components.
//
// The SDK is loaded with a dynamic import on the first join, not statically:
// it is ~400 KB that nobody who never opens a call should pay for, and it also
// keeps it out of the vendor chunk the whole app boots from.

// Statuses the UI branches on.
//
// `unavailable` is deliberately not `error`. It means the browser will not hand
// out a camera or a microphone at all — no secure context — and no retry will
// change that. On the dev stand (`:8083`, plain http, by IP) this is the normal
// state, so the room must explain it rather than show a broken call.
export const IDLE = 'idle'
export const CONNECTING = 'connecting'
export const LIVE = 'live'
export const ERROR = 'error'
export const UNAVAILABLE = 'unavailable'

// getUserMedia and enumerateDevices only exist in a secure context. Checking for
// the object rather than calling it keeps the failure quiet and instant — asking
// first and catching would surface a console error on every dev page load.
export function mediaSupported() {
  return typeof navigator !== 'undefined' && !!navigator.mediaDevices?.getUserMedia
}

// Volume bounds for the per-participant slider (#2872). Above 1 is real gain,
// not just a louder UI number — the quiet colleague on a laptop microphone is
// the whole reason the control exists — but it is capped, because past roughly
// twice the source level the clipping is worse than the quietness.
export const VOLUME_MAX = 2
export const VOLUME_DEFAULT = 1

// A participant as the UI sees one. Flat and plain on purpose: the tiles diff
// this, and handing them live SDK objects would make every re-render depend on
// SDK internals mutating in place.
function describe(p, local, volume = VOLUME_DEFAULT, muted = false) {
  // v2 exposes publications by source; the track is absent until subscribed,
  // which is exactly when the tile should still be showing the avatar.
  const cam = p.getTrackPublication?.('camera')
  const mic = p.getTrackPublication?.('microphone')
  // Screen share is a second publication on the same participant (#2874), not a
  // second participant: the presenter keeps their tile and their camera while
  // the stage shows what they are sharing.
  const screen = p.getTrackPublication?.('screen_share')
  const screenAudio = p.getTrackPublication?.('screen_share_audio')
  return {
    id: p.identity,
    sid: p.sid,
    // How loudly *we* hear them, and whether we silenced them for ourselves.
    // Both are local-only; a host's force-mute is a different thing and arrives
    // through the room socket.
    volume,
    localMuted: muted,
    // LiveKit carries the display name we minted server-side alongside the
    // token; falling back to the identity would put a raw UUID on a tile.
    name: p.name || p.identity,
    local,
    speaking: !!p.isSpeaking,
    micOn: !!p.isMicrophoneEnabled,
    camOn: !!p.isCameraEnabled,
    // Connection quality as the SFU rates it (#2884): 'excellent'|'good'|'poor'|
    // 'lost'|'unknown'. The tile only surfaces it when it goes bad, the way
    // Telemost does — a green bar on every tile is noise.
    quality: p.connectionQuality || 'unknown',
    // shallow-held elsewhere: these are SDK objects with their own lifecycle,
    // and making them deeply reactive would have Vue walk a MediaStreamTrack.
    videoTrack: cam?.isSubscribed === false ? null : cam?.track || null,
    // The local mic is never attached — that is a feedback loop, not monitoring.
    audioTrack: local ? null : mic?.track || null,
    screenTrack: screen?.isSubscribed === false ? null : screen?.track || null,
    // Same reason as the microphone: playing our own shared tab back into the
    // room is a feedback loop, and a loud one — the tab is usually a video.
    screenAudioTrack: local ? null : screenAudio?.track || null,
  }
}

/**
 * Media core for one conference room (#2871).
 *
 * The caller drives it with `join(conferenceId)` / `leave()` and reads `peers`.
 * State is rebuilt as a whole snapshot on every SDK event instead of patched
 * per-event — the same shape `internal/confroom` uses on the wire, and for the
 * same reason: a missed event can then never leave a phantom tile behind.
 *
 * @returns {object} state refs plus `join`, `leave`, `toggleMic`, `toggleCam`,
 *   `startScreen`, `stopScreen`, `selectDevice`, `unblockAudio`
 */
export function useConfTransport() {
  const status = ref(IDLE)
  const error = ref('')
  const peers = shallowRef([])
  const micOn = ref(false)
  const camOn = ref(false)
  // Whether *we* are publishing a screen (#2874). Read from the SDK on every
  // snapshot rather than remembered from the click: the browser's own "stop
  // sharing" bar ends the capture without telling us, and a flag we set
  // ourselves would leave the toolbar claiming a share that ended minutes ago.
  const screenOn = ref(false)
  // Browsers refuse to play audio until the page has been interacted with. The
  // SDK reports that instead of silently dropping sound, and the room turns it
  // into a "включить звук" button — otherwise a whole call is mute with no clue.
  const audioBlocked = ref(false)
  const devices = ref({ audioinput: [], videoinput: [], audiooutput: [] })
  const selected = ref({ audioinput: '', videoinput: '', audiooutput: '' })
  // Per-participant playback, entirely local (#2872): turning someone down here
  // changes nothing for anyone else in the call, which is the difference between
  // this and the host's force-mute. Keyed by identity rather than sid so it
  // survives that person reconnecting — the annoying background noise is still
  // the same person after their wifi blipped.
  const volumes = ref({})
  // Kept apart from a volume of zero so unmuting restores the level that was
  // set before, instead of snapping everyone back to the default.
  const localMuted = ref({})
  // Live microphone level per participant, identity → 0…1 (#2883). Polled on a
  // rAF loop rather than folded into the peer snapshot: it changes many times a
  // second and rebuilding every tile that often would be wasteful, so the meter
  // reads this small map instead. The local participant is in it too, which is
  // what makes the toolbar and the own tile a working "is my mic picking up".
  const audioLevels = ref({})

  // Not a ref: the room is an event emitter we hold, never something we render.
  let room = null
  let sdk = null
  // Guards the window between "join started" and "room connected" — a user who
  // clicks away in that window must not end up in a call with no UI attached.
  let leaving = false
  // The audio-level poll handle and its last tick, so the loop runs at ~15fps
  // rather than every animation frame.
  let levelRAF = null
  let lastLevelAt = 0

  // The loudest speaker gets the big tile. Ties and silence fall back to the
  // first remote peer so the stage never blinks empty mid-call; with nobody
  // else in the room it is the local tile, which is also the join preview.
  const dominant = computed(() => {
    const list = peers.value
    if (!list.length) return null
    const remote = list.filter((p) => !p.local)
    return remote.find((p) => p.speaking) || remote[0] || list[0]
  })
  const others = computed(() => peers.value.filter((p) => p !== dominant.value))
  const connected = computed(() => status.value === LIVE)
  // Whoever is actually publishing a screen right now. The room socket also has
  // an opinion — it holds the stage and the queue — but this one is the media
  // fact: the stage says who *may* present, this says whose pixels have arrived.
  // The tile is only useful once they have, so the layout follows this one.
  const screenPeer = computed(() => peers.value.find((p) => p.screenTrack) || null)

  // The level this participant should be played at, folding the local mute in.
  function levelFor(identity) {
    if (localMuted.value[identity]) return 0
    const v = volumes.value[identity]
    return typeof v === 'number' ? v : VOLUME_DEFAULT
  }

  function sync() {
    if (!room) return
    const local = room.localParticipant
    const list = local ? [describe(local, true)] : []
    for (const p of room.remoteParticipants?.values?.() || []) {
      const level = levelFor(p.identity)
      // Reapplied on every snapshot, not only when the slider moves: the SDK
      // attaches volume to the audio track, and a participant who reconnects or
      // republishes arrives at full volume again. Without this, turning someone
      // down would quietly undo itself the first time their microphone flickers.
      applyVolume(p, level)
      list.push(describe(p, false, levelFor(p.identity), !!localMuted.value[p.identity]))
    }
    peers.value = list
    micOn.value = !!local?.isMicrophoneEnabled
    camOn.value = !!local?.isCameraEnabled
    screenOn.value = !!local?.isScreenShareEnabled
    audioBlocked.value = room.canPlaybackAudio === false
  }

  // Every room event collapses into one snapshot rebuild, so the list of events
  // is a coverage question, not a correctness one: a missed event costs a stale
  // tile until the next one, never a wrong one.
  function listen(RoomEvent) {
    const events = [
      RoomEvent.ParticipantConnected,
      RoomEvent.ParticipantDisconnected,
      // Publish/unpublish of a *remote* track, distinct from subscribe events:
      // when a presenter stops sharing, TrackUnpublished is the authoritative
      // signal that the screen publication is gone. Without it a snapshot could
      // keep a dead screenTrack, leaving the stage black for everyone else until
      // a reload (#2880).
      RoomEvent.TrackPublished,
      RoomEvent.TrackUnpublished,
      RoomEvent.TrackSubscribed,
      RoomEvent.TrackUnsubscribed,
      RoomEvent.TrackMuted,
      RoomEvent.TrackUnmuted,
      RoomEvent.LocalTrackPublished,
      RoomEvent.LocalTrackUnpublished,
      RoomEvent.ActiveSpeakersChanged,
      RoomEvent.ParticipantNameChanged,
      RoomEvent.AudioPlaybackStatusChanged,
      RoomEvent.ConnectionStateChanged,
      // Repaints the per-tile weak-signal indicator when the SFU re-rates a
      // participant's link (#2884).
      RoomEvent.ConnectionQualityChanged,
    ].filter(Boolean)
    for (const e of events) room.on(e, sync)
    if (RoomEvent.Disconnected) room.on(RoomEvent.Disconnected, onDisconnected)
    if (RoomEvent.MediaDevicesChanged) room.on(RoomEvent.MediaDevicesChanged, refreshDevices)
  }

  // A disconnect we did not ask for — the SFU restarted, the call ended, or the
  // token expired mid-reconnect. Falling back to idle (not error) keeps the room
  // screen usable: the join button comes back instead of a dead-end message.
  function onDisconnected() {
    room = null
    peers.value = []
    micOn.value = false
    camOn.value = false
    screenOn.value = false
    stopLevels()
    detachUnload()
    if (status.value === LIVE) status.value = IDLE
  }

  // Sample every participant's microphone level into `audioLevels`. Throttled to
  // ~15fps: the meter is a coarse "are you being heard", not a waveform, and a
  // full map rebuild on every frame is wasted work for a dozen tiles.
  function pollLevels(ts) {
    if (!room) {
      levelRAF = null
      return
    }
    if (ts - lastLevelAt >= 66) {
      lastLevelAt = ts
      const next = {}
      const local = room.localParticipant
      if (local) next[local.identity] = local.audioLevel || 0
      for (const p of room.remoteParticipants?.values?.() || []) {
        next[p.identity] = p.audioLevel || 0
      }
      audioLevels.value = next
    }
    levelRAF = requestAnimationFrame(pollLevels)
  }
  function startLevels() {
    if (levelRAF !== null || typeof requestAnimationFrame === 'undefined') return
    lastLevelAt = 0
    levelRAF = requestAnimationFrame(pollLevels)
  }
  function stopLevels() {
    if (levelRAF !== null && typeof cancelAnimationFrame !== 'undefined') {
      cancelAnimationFrame(levelRAF)
    }
    levelRAF = null
    audioLevels.value = {}
  }

  async function refreshDevices() {
    if (!sdk || !mediaSupported()) return
    try {
      const kinds = ['audioinput', 'videoinput', 'audiooutput']
      const next = { audioinput: [], videoinput: [], audiooutput: [] }
      for (const kind of kinds) {
        const found = await sdk.Room.getLocalDevices(kind, false)
        next[kind] = (found || [])
          .filter((d) => d.deviceId)
          .map((d) => ({ id: d.deviceId, label: d.label }))
      }
      devices.value = next
    } catch {
      // Enumeration failing is not worth breaking a working call over: the menu
      // just shows nothing and the browser default keeps being used.
    }
  }

  // The camera indicator stays lit until the tracks are actually stopped, and a
  // closed tab never runs onBeforeUnmount — so the teardown is wired to both.
  // Without the pagehide half, leaving the page mid-call leaves the user
  // watching their own webcam light stay on.
  const onUnload = () => {
    try {
      room?.disconnect()
    } catch {
      // The page is going away; a failed disconnect has nowhere to be reported.
    }
  }
  function attachUnload() {
    if (typeof window === 'undefined') return
    window.addEventListener('pagehide', onUnload)
    window.addEventListener('beforeunload', onUnload)
  }
  function detachUnload() {
    if (typeof window === 'undefined') return
    window.removeEventListener('pagehide', onUnload)
    window.removeEventListener('beforeunload', onUnload)
  }

  /**
   * Enter the conference's media room.
   *
   * Order matters: the token is asked for first, because the server is the one
   * that decides whether this user may be here at all (and a kicked user is
   * refused there, not here). Only then do we touch the camera — asking for
   * hardware before knowing the answer would pop a permission prompt at someone
   * who is about to be told "no".
   *
   * @param {string} conferenceId
   * @param {{mic?: boolean, cam?: boolean}} want initial device state
   */
  async function join(conferenceId, want = {}) {
    if (status.value === CONNECTING || status.value === LIVE) return
    if (!mediaSupported()) {
      status.value = UNAVAILABLE
      return
    }
    leaving = false
    status.value = CONNECTING
    error.value = ''
    try {
      const { data } = await confApi.token(conferenceId)
      sdk = await import('livekit-client')
      if (leaving) {
        status.value = IDLE
        return
      }
      room = new sdk.Room({
        // adaptiveStream drops the resolution of a tile nobody is looking at,
        // and dynacast stops publishing layers nobody subscribes to. Both matter
        // more here than usual: this runs over a corporate wireguard link.
        adaptiveStream: true,
        dynacast: true,
        audioCaptureDefaults: selected.value.audioinput
          ? { deviceId: selected.value.audioinput }
          : undefined,
        videoCaptureDefaults: selected.value.videoinput
          ? { deviceId: selected.value.videoinput }
          : undefined,
      })
      listen(sdk.RoomEvent)
      await room.connect(data.url, data.token)
      if (leaving) {
        await room.disconnect()
        room = null
        status.value = IDLE
        return
      }
      attachUnload()
      // Mic on, camera off is the honest default for a daily standup: audio is
      // why people are here, and a camera that turns itself on is a surprise.
      await setMic(want.mic !== false)
      if (want.cam) await setCam(true)
      status.value = LIVE
      sync()
      refreshDevices()
      startLevels()
    } catch (e) {
      // A refusal from our own API is the interesting case and already carries a
      // sentence a human wrote (503 not configured, 409 ended, 403 kicked).
      error.value = e.response?.data?.error || e.message || String(e)
      status.value = ERROR
      try {
        await room?.disconnect()
      } catch {
        // Already down; the status above is what the user acts on.
      }
      room = null
    }
  }

  async function leave() {
    leaving = true
    detachUnload()
    stopLevels()
    const r = room
    room = null
    peers.value = []
    micOn.value = false
    camOn.value = false
    screenOn.value = false
    status.value = IDLE
    try {
      await r?.disconnect()
    } catch {
      // Nothing to do — we have already dropped our side of the room.
    }
  }

  async function setMic(on) {
    if (!room) return
    await room.localParticipant.setMicrophoneEnabled(on)
    sync()
  }
  async function setCam(on) {
    if (!room) return
    await room.localParticipant.setCameraEnabled(on)
    sync()
  }
  const toggleMic = () => setMic(!micOn.value)
  const toggleCam = () => setCam(!camOn.value)

  /**
   * Start publishing a screen (#2874). Reports whether the capture actually
   * began.
   *
   * Must be called straight from a click: `getDisplayMedia` is only granted
   * inside a user gesture, and an await before it (asking our server for the
   * stage first, say) spends that gesture and has the browser refuse the picker
   * outright. So the room is asked in parallel, and a share that turns out not
   * to hold the stage is stopped again — see the watcher in ConferenceRoom.
   *
   * A cancelled picker is not an error: the user changed their mind, and the
   * room screen has nothing to report.
   */
  async function startScreen() {
    if (!room) return false
    try {
      // Audio too: sharing a tab with a video and having it play silently for
      // everyone else is the classic screen-share disappointment.
      await room.localParticipant.setScreenShareEnabled(true, { audio: true })
    } catch (e) {
      // NotAllowedError is the cancelled picker; anything else is worth saying
      // out loud, because the button will otherwise look simply broken.
      if (e?.name !== 'NotAllowedError') error.value = e?.message || String(e)
    }
    sync()
    return screenOn.value
  }

  /** Stop publishing our screen. Safe to call when we are not sharing. */
  async function stopScreen() {
    if (!room) return
    try {
      await room.localParticipant.setScreenShareEnabled(false)
    } catch {
      // The capture is already gone (the browser's own stop button, or the tab
      // being closed); the snapshot below is what the UI reads either way.
    }
    sync()
  }

  // setVolume is a v2 method on RemoteParticipant; guarded because the local
  // participant does not have it and a stubbed room in a test need not.
  function applyVolume(p, level) {
    try {
      p.setVolume?.(level)
    } catch {
      // A participant whose audio track has already gone: the next snapshot
      // will not include them, and there is nothing to report.
    }
  }

  function remoteByIdentity(identity) {
    for (const p of room?.remoteParticipants?.values?.() || []) {
      if (p.identity === identity) return p
    }
    return null
  }

  /**
   * Set how loudly we hear one participant (#2872).
   *
   * Purely local: nobody else's call changes, and the person being turned down
   * is not told. That is deliberate — this is the volume knob, not moderation,
   * and a "someone muted you" signal would make it one.
   *
   * @param {string} identity the participant's user id
   * @param {number} level 0…VOLUME_MAX, where 1 is the source level
   */
  function setPeerVolume(identity, level) {
    const v = Math.min(VOLUME_MAX, Math.max(0, Number(level) || 0))
    volumes.value = { ...volumes.value, [identity]: v }
    // Moving the slider off zero is the same intent as unmuting; leaving the
    // mute set would make the control look broken.
    if (v > 0 && localMuted.value[identity]) {
      localMuted.value = { ...localMuted.value, [identity]: false }
    }
    const p = remoteByIdentity(identity)
    if (p) applyVolume(p, levelFor(identity))
    sync()
  }

  /** Silence one participant for ourselves only, keeping their level for later. */
  function togglePeerMute(identity) {
    localMuted.value = { ...localMuted.value, [identity]: !localMuted.value[identity] }
    const p = remoteByIdentity(identity)
    if (p) applyVolume(p, levelFor(identity))
    sync()
  }

  /**
   * Pick an input or output device.
   *
   * Remembered even when called before joining, so the choice made in the lobby
   * survives into the call — `join` feeds it to the capture defaults.
   *
   * @param {'audioinput'|'videoinput'|'audiooutput'} kind
   * @param {string} deviceId
   */
  async function selectDevice(kind, deviceId) {
    selected.value = { ...selected.value, [kind]: deviceId }
    if (!room) return
    try {
      await room.switchActiveDevice(kind, deviceId)
    } catch (e) {
      error.value = e.message || String(e)
    }
  }

  // Answers the browser's autoplay policy: it only accepts this from inside a
  // real user gesture, which is why it is a button and not something we retry.
  async function unblockAudio() {
    try {
      await room?.startAudio()
      audioBlocked.value = room?.canPlaybackAudio === false
    } catch {
      // Still blocked — the button stays, the user can try again.
    }
  }

  // Guarded because the transport is also driven straight from tests, where
  // there is no component to unmount — the SDK stub would otherwise be torn
  // down by a lifecycle hook Vue refuses to register.
  if (getCurrentInstance()) onBeforeUnmount(leave)

  return {
    status,
    error,
    peers,
    dominant,
    others,
    connected,
    screenPeer,
    micOn,
    camOn,
    screenOn,
    audioBlocked,
    devices,
    selected,
    volumes,
    localMuted,
    audioLevels,
    join,
    leave,
    toggleMic,
    toggleCam,
    startScreen,
    stopScreen,
    setMic,
    setPeerVolume,
    togglePeerMute,
    selectDevice,
    unblockAudio,
  }
}
