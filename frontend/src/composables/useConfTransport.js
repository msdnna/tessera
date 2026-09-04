import { computed, getCurrentInstance, onBeforeUnmount, ref, shallowRef } from 'vue'
import { conferences as confApi } from '@/api'
import { createNoiseFilter, denoiseSupported } from '@/utils/denoise'

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

// Advanced noise suppression (#2889). Remembered per browser rather than per
// call: whether a room is noisy is a property of where someone sits, not of the
// meeting, so asking again every morning would be the wrong question.
export const DENOISE_KEY = 'tessera_conf_denoise'

function storedDenoise() {
  try {
    return localStorage.getItem(DENOISE_KEY) === '1'
  } catch {
    // Private mode or storage disabled — off is the safe default.
    return false
  }
}

// A participant as the UI sees one. Flat and plain on purpose: the tiles diff
// this, and handing them live SDK objects would make every re-render depend on
// SDK internals mutating in place.
export function describe(p, local, volume = VOLUME_DEFAULT, muted = false) {
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
    // A muted camera keeps its publication AND its `.track` object — the SDK's
    // setTrackEnabled mutes the camera in place but *unpublishes* screen share
    // ("screenshare cannot be muted"), so subscription alone still reads as "has
    // video" and the tile paints a black rect over the avatar (#2890). Treat a
    // muted publication like an absent one: nothing to show. TrackMuted/Unmuted
    // are already in the rebuild list above, so this flips back on unmute.
    videoTrack: cam && !cam.isMuted && cam.isSubscribed !== false ? cam.track || null : null,
    // The local mic is never attached — that is a feedback loop, not monitoring.
    audioTrack: local ? null : mic?.track || null,
    // Symmetric with the camera for readability, though screen share is muted by
    // unpublishing (see above), so isMuted here is effectively always false.
    screenTrack: screen && !screen.isMuted && screen.isSubscribed !== false ? screen.track || null : null,
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
  // Our own microphone level, 0…1 (#2883). Measured locally with a WebAudio
  // analyser tapped straight off the mic track — not read from the SFU's
  // per-participant `audioLevel`, which lags by the network round trip and made
  // the meter feel disconnected from one's own voice. Only our own level: the
  // meter is a "is my mic picking me up" self-check, and other people's speech is
  // already shown by the ring around their tile.
  const micLevel = ref(0)
  // Whether our own microphone is running through the spectral gate (#2889).
  // `denoiseAvailable` is separate because a browser without AudioWorklet must
  // hide the control rather than offer one that silently does nothing.
  const denoise = ref(storedDenoise())
  const denoiseAvailable = denoiseSupported()
  // Identity of the loudest current speaker, kept sticky (#2864 round 2): when
  // everyone falls silent we hold the last speaker on the big tile rather than
  // blinking to whoever happens to be first in the list. Updated from the SDK's
  // activeSpeakers, which is already sorted loudest-first and includes us.
  const activeSpeakerId = ref('')

  // Not a ref: the room is an event emitter we hold, never something we render.
  let room = null
  let sdk = null
  // Guards the window between "join started" and "room connected" — a user who
  // clicks away in that window must not end up in a call with no UI attached.
  let leaving = false
  // WebAudio plumbing for the own-mic meter (analyser rebuilt as the mic turns
  // on/off or the device changes) plus the single media loop that drives both the
  // meter and the active-speaker pick.
  let mediaRAF = null
  let lastSpeakerAt = 0
  let audioCtx = null
  let analyser = null
  let analyserBuf = null
  let micSource = null
  // The attached noise filter, and the AudioContext we fall back to when the SDK
  // has not put one on the track yet (#2889).
  let filter = null
  let ownCtx = null

  // The loudest speaker gets the big tile, and it follows the conversation
  // (#2864 round 2): whoever is speaking — including us — steps up, and the last
  // speaker stays there through a silence rather than the stage snapping to an
  // arbitrary peer. Falls back to a remote (then local) only until the first word
  // is spoken, so a fresh call still shows a face instead of nothing.
  const dominant = computed(() => {
    const list = peers.value
    if (!list.length) return null
    const active = list.find((p) => p.id === activeSpeakerId.value)
    if (active) return active
    // No active-speaker signal (none yet, or an SDK/mocks without the sorted
    // list): fall back to whoever is flagged speaking, then to a remote, then to
    // us — so a fresh call still shows a face rather than nothing.
    const speaking = list.find((p) => p.speaking)
    if (speaking) return speaking
    const remote = list.filter((p) => !p.local)
    return remote[0] || list[0]
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
    // Remember the loudest speaker; keep the last one through a silence so the
    // big tile does not blink between people every time the room goes quiet.
    const speakers = room.activeSpeakers || []
    if (speakers.length && speakers[0]?.identity) activeSpeakerId.value = speakers[0].identity
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
    stopMediaLoop()
    detachUnload()
    if (status.value === LIVE) status.value = IDLE
  }

  // The raw MediaStreamTrack of our own microphone, or null. LiveKit's local
  // track exposes it as `mediaStreamTrack`; the analyser taps that directly so
  // the meter reflects the mic itself, not what came back from the SFU.
  function localMicTrack() {
    return localMicPub()?.mediaStreamTrack || null
  }

  /** Our own published microphone track, or null. */
  function localMicPub() {
    return room?.localParticipant?.getTrackPublication?.('microphone')?.track || null
  }

  // livekit-client refuses to attach an audio processor to a track that carries
  // no AudioContext. It normally sets one when publishing, but that happens only
  // once the room has acquired a context of its own, which the browser's autoplay
  // policy can defer. Retrying with our own is cheaper than losing the feature.
  async function attachFilter(track) {
    try {
      await track.setProcessor(filter)
    } catch (e) {
      if (!/audio context/i.test(e?.message || '')) throw e
      const Ctx =
        typeof window !== 'undefined' && (window.AudioContext || window.webkitAudioContext)
      if (!Ctx || !track.setAudioContext) throw e
      ownCtx = ownCtx || new Ctx()
      track.setAudioContext(ownCtx)
      await track.setProcessor(filter)
    }
  }

  /**
   * Put the noise filter on our microphone, or take it off (#2889).
   *
   * It goes on the track, not on the room: LiveKit hands the processed
   * MediaStreamTrack to the sender, so the SFU — and through it everyone else —
   * only ever receives cleaned audio. Our own meter reads
   * `track.mediaStreamTrack`, which *is* the processed track once a processor is
   * set, so the toolbar level shows what the room hears rather than what the
   * microphone picked up. That is what the plan asked for: measure after
   * suppression, otherwise the meter twitches at noise the others cannot hear.
   */
  async function applyDenoise() {
    if (!denoiseAvailable) return
    const track = localMicPub()
    if (!track?.setProcessor) return
    try {
      if (denoise.value) {
        filter = filter || createNoiseFilter()
        await attachFilter(track)
      } else if (filter) {
        await track.stopProcessor()
        filter = null
      }
    } catch {
      // A filter that will not start must not take the call down with it — the
      // microphone still works, it is only noisier. Flipping the flag back keeps
      // the toggle honest about what is actually running.
      denoise.value = false
      filter = null
    }
  }

  /**
   * Turn advanced noise suppression on or off, now and for next time.
   *
   * @param {boolean} on
   */
  async function setDenoise(on) {
    denoise.value = !!on
    await applyDenoise()
    try {
      // Written from the flag *after* applying, so a filter that failed to start
      // is not remembered as enabled.
      localStorage.setItem(DENOISE_KEY, denoise.value ? '1' : '0')
    } catch {
      // No storage — the choice simply lasts for this call.
    }
    // The analyser is bound to whichever track was current when it was built;
    // attaching or removing the processor swaps that track underneath it.
    if (micOn.value) setupAnalyser()
  }
  const toggleDenoise = () => setDenoise(!denoise.value)

  // Build a WebAudio analyser over our own mic track. Rebuilt whenever the source
  // changes (mute/unmute, device switch), because a MediaStreamSource is bound to
  // one track for its life. The media loop below reads it every frame.
  function setupAnalyser() {
    teardownAnalyser()
    const track = localMicTrack()
    const Ctx = typeof window !== 'undefined' && (window.AudioContext || window.webkitAudioContext)
    if (!track || !Ctx) return
    try {
      audioCtx = new Ctx()
      micSource = audioCtx.createMediaStreamSource(new MediaStream([track]))
      analyser = audioCtx.createAnalyser()
      // Small window: we want responsiveness, not spectral detail. 512 samples at
      // 48kHz is ~10ms — well under one animation frame.
      analyser.fftSize = 512
      micSource.connect(analyser)
      analyserBuf = new Uint8Array(analyser.fftSize)
    } catch {
      teardownAnalyser()
    }
  }
  function teardownAnalyser() {
    try {
      micSource?.disconnect()
    } catch {
      // Source already detached with its track; nothing to clean up.
    }
    micSource = null
    analyser = null
    analyserBuf = null
    if (audioCtx) {
      try {
        audioCtx.close()
      } catch {
        // Context already closed.
      }
      audioCtx = null
    }
    micLevel.value = 0
  }

  // One rAF loop, running for the whole call, doing two jobs:
  //   - our own mic level from the analyser, every frame, so the toolbar meter is
  //     smooth (#2883);
  //   - the loudest speaker, throttled, so the big tile follows the conversation
  //     (#2887). This reads participant.audioLevel directly rather than the SDK's
  //     activeSpeakers event, which proved unreliable at picking a winner.
  function mediaTick(ts) {
    if (!room) {
      mediaRAF = null
      return
    }
    if (analyser && analyserBuf) {
      analyser.getByteTimeDomainData(analyserBuf)
      let sum = 0
      for (let i = 0; i < analyserBuf.length; i++) {
        const v = (analyserBuf[i] - 128) / 128
        sum += v * v
      }
      // RMS with a small noise-floor cut, then boosted: conversational speech
      // sits low on the scale, so a raw meter barely twitches at a normal volume.
      const rms = Math.sqrt(sum / analyserBuf.length)
      micLevel.value = Math.min(1, Math.max(0, rms - 0.02) * 6)
    }
    if (ts - lastSpeakerAt >= 120) {
      lastSpeakerAt = ts
      // Threshold keeps the stage from chasing keyboard clicks and room hum.
      let loudest = null
      let max = 0.08
      const consider = (p) => {
        const l = p?.audioLevel || 0
        if (l > max) {
          max = l
          loudest = p.identity
        }
      }
      consider(room.localParticipant)
      for (const p of room.remoteParticipants?.values?.() || []) consider(p)
      // Sticky: only move on a real speaker, so a silence holds the last one.
      if (loudest) activeSpeakerId.value = loudest
    }
    mediaRAF = requestAnimationFrame(mediaTick)
  }
  function startMediaLoop() {
    if (mediaRAF !== null || typeof requestAnimationFrame === 'undefined') return
    lastSpeakerAt = 0
    mediaRAF = requestAnimationFrame(mediaTick)
  }
  function stopMediaLoop() {
    if (mediaRAF !== null && typeof cancelAnimationFrame !== 'undefined')
      cancelAnimationFrame(mediaRAF)
    mediaRAF = null
    teardownAnalyser()
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
        // Browser-native cleanup on the mic (#2883): echo cancellation, noise
        // suppression and auto gain are the baseline every call app turns on, and
        // AGC also lifts a quiet speaker so the level meter has something to show.
        // Heavier suppression sits on top of this as an opt-in track processor
        // (#2889) — these stay on either way, they are cheap and never hurt.
        audioCaptureDefaults: {
          ...(selected.value.audioinput ? { deviceId: selected.value.audioinput } : {}),
          echoCancellation: true,
          noiseSuppression: true,
          autoGainControl: true,
        },
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
      startMediaLoop()
      if (micOn.value) setupAnalyser()
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
    stopMediaLoop()
    const r = room
    room = null
    peers.value = []
    micOn.value = false
    camOn.value = false
    screenOn.value = false
    status.value = IDLE
    // The filter's nodes die with the track, but the fallback context is ours to
    // close — a leaked AudioContext keeps the audio hardware awake after the call.
    filter = null
    if (ownCtx) {
      try {
        await ownCtx.close()
      } catch {
        // Already closed.
      }
      ownCtx = null
    }
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
    // A first unmute publishes the track the filter attaches to, so the filter
    // goes on before the analyser is built over it (#2889).
    if (on) {
      await applyDenoise()
      setupAnalyser()
    } else teardownAnalyser()
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
      // Quality knobs for the shared screen (#2885). The presenter always sees a
      // crisp picture because that is their own raw capture; everyone else sees
      // the *encoded* stream, so the only lever we have on their end is how much
      // the encoder is allowed to spend. Three settings move it the most:
      //   - contentHint 'detail' tells the encoder this is a screen, not a
      //     webcam, so it keeps edges sharp instead of smoothing them;
      //   - a high resolution cap at a modest frame rate — text does not need
      //     30fps, and the saved frames buy per-frame sharpness;
      //   - degradationPreference 'maintain-resolution' so that when the link is
      //     tight the frame rate drops before the resolution does — a laggier but
      //     readable screen beats a smooth blur.
      // It cannot make the remote picture match the local one, but it raises the
      // ceiling well above the SDK's webcam-tuned default.
      await room.localParticipant.setScreenShareEnabled(
        true,
        {
          // Audio too: sharing a tab with a video and having it play silently for
          // everyone else is the classic screen-share disappointment.
          audio: true,
          resolution: { width: 1920, height: 1080, frameRate: 15 },
          contentHint: 'detail',
        },
        {
          screenShareEncoding: { maxBitrate: 3_000_000, maxFramerate: 15 },
          degradationPreference: 'maintain-resolution',
        },
      )
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
      // A new mic track needs a fresh analyser bound to it.
      if (kind === 'audioinput' && micOn.value) setupAnalyser()
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
    micLevel,
    denoise,
    denoiseAvailable,
    join,
    leave,
    toggleMic,
    toggleCam,
    setDenoise,
    toggleDenoise,
    startScreen,
    stopScreen,
    setMic,
    setPeerVolume,
    togglePeerMute,
    selectDevice,
    unblockAudio,
  }
}
