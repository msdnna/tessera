<script setup>
// Server-side recording template (#2877), rendered by egress's headless Chrome —
// NOT a page a human ever opens. Egress navigates to
// `/rec/egress?url=<ws>&token=<t>&layout=<l>` (query-param names verified against
// livekit/egress v1.14.1), we connect to the SFU with that token as a hidden,
// subscribe-only participant, render the call, and egress captures the tab.
//
// This is what makes the mp4 look like the conference does in the browser rather
// than egress's dark built-in grid: a shared screen fills the frame at its own
// resolution, a camera rides in a small PiP in the corner, and a participant with
// no video is their avatar tile — the same ParticipantTile the room uses, fed by
// the same describe() snapshot (so the #2890 muted-camera→avatar rule holds here
// too, for free).
//
// The egress contract, both halves verified in the binary (grep START_RECORDING /
// END_RECORDING /usr/bin/egress ⇒ present): print START_RECORDING once we are
// ready for the capture to begin, END_RECORDING when the room closes so egress
// finalises the file. Nothing here calls our API — egress owns the token, we own
// the pixels.
import { ref, computed, onMounted, onBeforeUnmount, shallowRef, watch } from 'vue'
import { describe } from '@/composables/useConfTransport'
import ParticipantTile from '@/components/conference/ParticipantTile.vue'

// A shallowRef of plain descriptors: describe() hands back flat objects (never
// live SDK handles), and the tiles diff these.
const peers = shallowRef([])
const fatal = ref('')
const screenVideoEl = ref(null)
const audioSink = ref(null)

let room = null

// The presenter's screen, if anyone is sharing — it takes the whole frame.
const screenPeer = computed(() => peers.value.find((p) => p.screenTrack) || null)
// One camera rides along as PiP over a shared screen: the presenter's own face
// when they have a camera, otherwise the loudest speaker's. A single PiP, not a
// strip — the recording is about the screen, and a row of faces would eat it.
const pipPeer = computed(() => {
  const scr = screenPeer.value
  if (!scr) return null
  if (scr.videoTrack) return scr
  return peers.value.find((p) => p.videoTrack) || null
})
// No screen shared: the plain grid of tiles, exactly as the room shows it.
const gridPeers = computed(() => peers.value)

// describe() needs the SDK participant; the recorder never renders itself, so
// only the remote participants are walked.
function rebuild() {
  if (!room) return
  const out = []
  for (const p of room.remoteParticipants.values()) out.push(describe(p, false))
  peers.value = out
  syncAudio()
}

// The shared screen is a bare, full-frame <video>, NOT a ParticipantTile: the
// tile caps a screen at 62vh (.tile.screen.stage) to leave room for the room's
// strip, which in a recording is just empty letterbox bands (#2877 rework).
// attach()/detach() like the tile does, so the SDK counts the element as watched.
watch(
  [() => screenPeer.value?.screenTrack || null, screenVideoEl],
  ([track, el], prev) => {
    const old = prev?.[0]
    if (old && old !== track && el) old.detach(el)
    if (track && el) track.attach(el)
    else if (el && el.srcObject) el.srcObject = null
  },
  { flush: 'post' },
)

// Audio playback for the capture. Egress records the browser TAB's sound, so
// every participant's audio has to actually play on this page — and it will not
// on its own: ParticipantTile stopped rendering audio at #2888, when it moved to
// the room's shared session sink, which this standalone page does not have. So we
// keep our own sink of <audio> elements, one per live audio (mic + shared-tab),
// idempotent by track so a rebuild does not stack duplicates.
const audioEls = new Map()
function syncAudio() {
  const sink = audioSink.value
  if (!sink) return
  const live = new Set()
  for (const p of peers.value) {
    for (const t of [p.audioTrack, p.screenAudioTrack]) {
      if (!t) continue
      live.add(t)
      if (!audioEls.has(t)) {
        const el = t.attach()
        el.autoplay = true
        sink.appendChild(el)
        audioEls.set(t, el)
      }
    }
  }
  for (const [t, el] of [...audioEls]) {
    if (live.has(t)) continue
    try {
      t.detach(el)
    } catch {
      // track already gone; drop the element regardless
    }
    el.remove()
    audioEls.delete(t)
  }
}

// Egress runs Chrome in a locale that differs from the room's language, so it
// offers to translate the page — and that bubble lands IN the recording. Tell
// Chrome not to (#2877 rework). Only this page, not the whole app.
function suppressTranslate() {
  const html = document.documentElement
  html.setAttribute('translate', 'no')
  html.classList.add('notranslate')
  const m = document.createElement('meta')
  m.name = 'google'
  m.content = 'notranslate'
  document.head.appendChild(m)
}

async function connect() {
  const q = new URLSearchParams(window.location.search)
  const url = q.get('url')
  const token = q.get('token')
  if (!url || !token) {
    // Nothing to record against — say so on the page (an operator hitting this
    // URL by hand should see why) and never print START_RECORDING.
    fatal.value = 'recorder: missing url or token'
    return
  }
  const sdk = await import('livekit-client')
  const { Room, RoomEvent } = sdk
  room = new Room({
    // The opposite of the room's own client: adaptiveStream downgrades a tile
    // nobody is "looking at", but the recorder is looking at all of them at full
    // size — a downgraded stream is exactly the crushed quality this task exists
    // to fix. dynacast off for the same reason.
    adaptiveStream: false,
    dynacast: false,
  })
  const repaint = [
    RoomEvent.ParticipantConnected,
    RoomEvent.ParticipantDisconnected,
    RoomEvent.TrackPublished,
    RoomEvent.TrackUnpublished,
    RoomEvent.TrackSubscribed,
    RoomEvent.TrackUnsubscribed,
    RoomEvent.TrackMuted,
    RoomEvent.TrackUnmuted,
    RoomEvent.ActiveSpeakersChanged,
  ]
  for (const ev of repaint) room.on(ev, rebuild)
  // Room closed (our backend stopped the egress, or everyone left): tell egress
  // to finalise the mp4. The egress-sdk's autoEnd does the same thing.
  room.on(RoomEvent.Disconnected, () => {
    console.log('END_RECORDING')
  })
  try {
    await room.connect(url, token)
  } catch (e) {
    fatal.value = 'recorder: ' + (e?.message || 'connect failed')
    return
  }
  rebuild()
  // Ready — let egress begin capturing. Deferred one frame so the first tiles
  // are painted before the recording starts, or the opening moment is blank.
  requestAnimationFrame(() => {
    console.log('START_RECORDING')
  })
}

onMounted(() => {
  suppressTranslate()
  connect()
})
onBeforeUnmount(() => {
  for (const [t, el] of audioEls) {
    try {
      t.detach(el)
    } catch {
      // already detached
    }
    el.remove()
  }
  audioEls.clear()
  if (room) room.disconnect()
})
</script>

<template>
  <div class="rec">
    <template v-if="!fatal">
      <!-- Hidden audio sink: the tab's sound is what egress records. -->
      <div ref="audioSink" class="rec-audio" aria-hidden="true" />

      <!-- Shared screen, full frame. Always in the DOM (v-show) so its ref is
           stable for attach/detach; hidden when nobody is sharing. -->
      <video
        v-show="screenPeer"
        ref="screenVideoEl"
        class="rec-screen-vid"
        autoplay
        muted
        playsinline
      />
      <div v-if="screenPeer && pipPeer" class="rec-pip">
        <participant-tile :key="`pip-${pipPeer.id}`" :peer="pipPeer" />
      </div>

      <!-- No screen → the grid of tiles, the way the room looks in the browser. -->
      <div v-if="!screenPeer" class="rec-grid" :data-count="gridPeers.length">
        <participant-tile v-for="p in gridPeers" :key="p.sid || p.id" :peer="p" class="rec-cell" />
      </div>
    </template>

    <div v-else class="rec-msg">{{ fatal }}</div>
  </div>
</template>

<style scoped>
/* Fills the egress viewport (1920×1080 at the configured preset). Solid dark
   background so the recording reads like the app in its dark theme rather than a
   white browser page. */
.rec {
  position: fixed;
  inset: 0;
  background: #0b0b12;
  overflow: hidden;
}
.rec-msg {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #9aa;
  font-size: 20px;
}
.rec-audio {
  position: absolute;
  width: 0;
  height: 0;
  overflow: hidden;
}
/* The shared screen: the WHOLE frame. contain (not cover) so a presenter's slide
   is letterboxed rather than cropped when its aspect differs from 16:9; when it
   matches — the common case — it fills edge to edge with no bands, which is the
   height cap this rework removed. */
.rec-screen-vid {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  object-fit: contain;
  background: #000;
}
/* Camera picture-in-picture, bottom-right over the screen. 22vw keeps it small
   enough not to cover the slide it sits on. */
.rec-pip {
  position: absolute;
  right: 2.5%;
  bottom: 4%;
  width: 22vw;
  max-width: 420px;
  aspect-ratio: 16 / 9;
  border-radius: 12px;
  overflow: hidden;
  box-shadow: 0 6px 24px rgba(0, 0, 0, 0.5);
}
.rec-pip :deep(.tile) {
  width: 100%;
  height: 100%;
}
/* No-screen grid: centred, auto-fitting tiles that grow to fill the frame. */
.rec-grid {
  position: absolute;
  inset: 0;
  display: grid;
  gap: 16px;
  padding: 24px;
  align-content: center;
  justify-content: center;
  grid-template-columns: repeat(auto-fit, minmax(320px, 1fr));
}
/* One participant reads better big than lost in a wide grid. */
.rec-grid[data-count='1'] {
  grid-template-columns: minmax(0, 70vw);
}
.rec-cell {
  aspect-ratio: 16 / 9;
}
</style>
