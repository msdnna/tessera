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
import { ref, computed, onMounted, onBeforeUnmount, shallowRef } from 'vue'
import { describe } from '@/composables/useConfTransport'
import ParticipantTile from '@/components/conference/ParticipantTile.vue'

// A shallowRef of plain descriptors: describe() hands back flat objects (never
// live SDK handles), and the tiles diff these.
const peers = shallowRef([])
const connected = ref(false)
const fatal = ref('')

let room = null
let sdk = null

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
  sdk = await import('livekit-client')
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
  connected.value = true
  rebuild()
  // Ready — let egress begin capturing. Deferred one frame so the first tiles
  // are painted before the recording starts, or the opening moment is blank.
  requestAnimationFrame(() => {
    console.log('START_RECORDING')
  })
}

onMounted(connect)
onBeforeUnmount(() => {
  if (room) room.disconnect()
})
</script>

<template>
  <div class="rec">
    <div v-if="fatal" class="rec-msg">{{ fatal }}</div>

    <!-- Screen shared → it fills the frame at its own resolution, one camera PiP
         in the bottom-right corner over it. -->
    <template v-else-if="screenPeer">
      <participant-tile
        :key="`screen-${screenPeer.id}`"
        :peer="screenPeer"
        stage
        screen
        class="rec-screen"
      />
      <div v-if="pipPeer" class="rec-pip">
        <participant-tile :key="`pip-${pipPeer.id}`" :peer="pipPeer" />
      </div>
    </template>

    <!-- No screen → the grid of tiles, the way the room looks in the browser. -->
    <div v-else class="rec-grid" :data-count="gridPeers.length">
      <participant-tile v-for="p in gridPeers" :key="p.sid || p.id" :peer="p" class="rec-cell" />
    </div>
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
/* The shared screen: whole frame, letterboxed (contain) so a presenter's slides
   are never cropped. */
.rec-screen {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
}
.rec-screen :deep(video) {
  object-fit: contain !important;
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
