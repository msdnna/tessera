<script setup>
// One participant in the call (#2864, subtask #2871).
//
// Video is the exception here, not the rule: we are told the team rarely turns a
// camera on, so the tile is designed around the avatar and merely makes room for
// a stream when one shows up.
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { NIcon, NTooltip } from 'naive-ui'
import {
  MicOffOutline,
  MicOutline,
  DesktopOutline,
  CellularOutline,
  HandRightOutline,
} from '@vicons/ionicons5'
import UserAvatar from '@/components/UserAvatar.vue'
import { personHue } from '@/utils/personHue'

const props = defineProps({
  // The flat descriptor from useConfTransport — never a live SDK object.
  peer: { type: Object, required: true },
  // The dominant-speaker tile: same markup, more room.
  stage: { type: Boolean, default: false },
  // Show this peer's shared screen instead of their camera (#2874). The same
  // person is legitimately on screen twice while presenting — their screen on
  // the stage, their face in the strip — which is why this is a prop rather
  // than something derived from the peer.
  screen: { type: Boolean, default: false },
  // A raised hand, from the room socket (the SFU knows nothing of it) — drawn as
  // an amber badge in the tile's corner (v2, #3107).
  hand: { type: Boolean, default: false },
  // The grid cell of the call's grid layout: it fills its cell instead of
  // keeping 16:9.
  cell: { type: Boolean, default: false },
})

// Which of the peer's two publications this tile is showing. A screen tile with
// nothing shared yet stays blank rather than falling back to the camera: the
// stage is held by a presenter whose first frame is simply still in flight.
const video = computed(() => (props.screen ? props.peer.screenTrack : props.peer.videoTrack))

// Only surface a link that is actually struggling (#2884), and never on a screen
// tile — the presenter's face tile already carries their signal.
const weakConn = computed(
  () => !props.screen && (props.peer.quality === 'poor' || props.peer.quality === 'lost'),
)

const videoEl = ref(null)

// v2 (#3107): a tile without video is drawn in the person's own tone.
const toneStyle = computed(() => ({ '--tone': personHue(props.peer.id) }))

// A phone is held upright, and it publishes what it sees: 9:16. `cover` fits
// that to the width of a 16:9 tile and throws away everything above and below —
// on the desktop the caller's face left the frame entirely (#2896). So a portrait
// stream is fitted by height instead, for the same reason a shared screen is:
// letterboxing shows less, cropping shows the wrong thing.
//
// Measured from the stream rather than assumed from the device, because nothing
// here knows what published the track — and a phone turned sideways mid-call
// becomes an ordinary landscape tile with no reload.
const portrait = ref(false)
function measure() {
  const el = videoEl.value
  // Both zero until the first frame; `resize` fires again when they are known,
  // and once more on every rotation.
  portrait.value = !!el && el.videoWidth > 0 && el.videoHeight > el.videoWidth
}

// attach/detach rather than assigning srcObject: the SDK keeps its own list of
// attached elements and uses it to decide whether a track is still being
// watched — that is what makes adaptiveStream stop paying for a hidden tile.
function bind(track, el) {
  if (!el) return
  if (track) track.attach(el)
  else if (el.srcObject) el.srcObject = null
}

watch(
  () => [video.value, videoEl.value],
  ([track], prev) => {
    const old = prev?.[0]
    // Detaching only our own element, not everywhere: the same camera track is
    // legitimately on screen twice while this person holds the stage.
    if (old && old !== track) old.detach(videoEl.value)
    bind(track, videoEl.value)
    // A tile whose stream went away keeps its last shape otherwise, and the
    // avatar that replaces it would sit in a frame sized for somebody else's
    // phone until the next publication.
    if (!track) portrait.value = false
    else measure()
  },
  { immediate: true, flush: 'post' },
)

onBeforeUnmount(() => {
  // A tile that goes away without detaching leaves the SDK believing someone is
  // still watching, and the stream keeps being paid for.
  if (video.value && videoEl.value) video.value.detach(videoEl.value)
})
</script>

<template>
  <div
    class="tile"
    :class="{ stage, cell, speaking: !screen && peer.speaking, video: !!video, screen, portrait }"
    :style="toneStyle"
    :data-testid="screen ? 'conference-screen-tile' : 'conference-tile'"
  >
    <!-- muted on the local tile is not cosmetic: an unmuted self-view is a
         feedback loop through the room's speakers. Audio is not played here at
         all: it lives in the always-mounted ConferenceAudioSink (#2888), so a
         minimised call — which renders only one tile — is not left silent. -->
    <video
      v-show="video"
      ref="videoEl"
      class="v"
      autoplay
      playsinline
      muted
      @loadedmetadata="measure"
      @resize="measure"
    />

    <!-- Weak-signal badge, top-left, amber for poor and red for lost (#2884). -->
    <n-tooltip v-if="weakConn">
      <template #trigger>
        <div class="conn" :class="peer.quality" data-testid="conference-quality">
          <n-icon :component="CellularOutline" :size="13" />
        </div>
      </template>
      {{ $t(`conferences.quality.${peer.quality}`) }}
    </n-tooltip>

    <div v-if="!video" class="face">
      <!-- Nothing to show yet. On a screen tile that is the presenter's first
           frame still in flight, not a person without a camera, so it says so
           instead of putting their avatar where their screen belongs. -->
      <n-icon v-if="screen" :component="DesktopOutline" :size="32" class="waiting" />
      <user-avatar v-else :user-id="peer.id" :name="peer.name" class="av" />
    </div>

    <span v-if="hand && !screen" class="hand" data-testid="conference-tile-hand">
      <n-icon :component="HandRightOutline" :size="15" />
    </span>

    <!-- The name pill, bottom-left: the mic state in front of the name. -->
    <div class="bar">
      <span class="pill">
        <n-icon v-if="screen" :component="DesktopOutline" :size="13" class="src-icon" />
        <n-icon v-else-if="!peer.micOn" :component="MicOffOutline" :size="13" class="muted-icon" />
        <n-icon v-else :component="MicOutline" :size="13" class="mic-icon" />
        <span class="name">{{ peer.name }}</span>
      </span>
    </div>
  </div>
</template>

<style scoped>
/* v2 (#3107 — PackageE «Звонок»): a tile is a surface in the person's tone
   (--tone, see personHue), radius 18; the speaker gets a 3px accent ring and a
   glow; the name is a dark pill bottom-left. */
.tile {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  aspect-ratio: 16 / 9;
  min-height: 96px;
  border-radius: 18px;
  overflow: hidden;
  background: linear-gradient(
    to top right,
    color-mix(in srgb, var(--tone, var(--t-primary)) 14%, var(--t-col)),
    color-mix(in srgb, var(--tone, var(--t-primary)) 24%, var(--t-col))
  );
  /* The ring is drawn inside (box-shadow), so it never resizes the tile. */
  transition: box-shadow 0.15s ease;
}
/* The stage keeps 16:9 but is capped by the viewport: at a desktop width the
   ratio alone would push the toolbar below the fold. */
.tile.stage {
  min-height: 220px;
  max-height: 44vh;
}
/* A grid cell fills its cell. */
.tile.cell {
  aspect-ratio: auto;
  min-height: 0;
  max-height: none;
  width: 100%;
  height: 100%;
}
/* A live stream is dark by nature; the tone behind it would show in the
   letterbox until the first frame lands. */
.tile.video {
  background: #000;
}
.tile.speaking {
  box-shadow:
    inset 0 0 0 3px var(--t-primary),
    0 0 22px color-mix(in srgb, var(--t-primary) 45%, transparent);
}
.v {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
/* A shared screen is the one thing here that must not be cropped. */
.tile.screen .v {
  object-fit: contain;
}
/* Same trade for a phone held upright (#2896). */
.tile.portrait .v {
  object-fit: contain;
}
.tile.screen.stage {
  max-height: 62vh;
}
.waiting {
  color: var(--t-text3);
}
/* Weak-signal badge (#2884). */
.conn {
  position: absolute;
  top: 10px;
  left: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 3px;
  border-radius: 7px;
  background: rgb(0 0 0 / 45%);
  z-index: 1;
}
.conn.poor {
  color: #f0a020;
}
.conn.lost {
  color: #e88080;
}
/* A raised hand: the amber of attention, top-right. */
.hand {
  position: absolute;
  top: 10px;
  right: 10px;
  z-index: 1;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  border-radius: 9px;
  color: #fff;
  background: linear-gradient(
    to top right,
    color-mix(in srgb, #f0a020 86%, #000),
    #f0a020 50%,
    color-mix(in srgb, #f0a020 86%, #fff)
  );
}
.src-icon,
.mic-icon,
.muted-icon {
  flex: none;
}
.muted-icon {
  color: #ff9a8a;
}
/* The avatar is nudged up so it clears the name pill on a short tile. */
.face {
  display: flex;
  align-items: center;
  justify-content: center;
  margin-bottom: 22px;
}
.stage .face,
.cell .face {
  margin-bottom: 0;
}
.av {
  width: 56px;
  height: 56px;
  border-radius: 50%;
  font-size: 20px;
  font-weight: 700;
  color: #fff;
  background: var(--t-accent-grad);
}
.stage .av,
.cell .av {
  width: 72px;
  height: 72px;
  font-size: 26px;
}
.bar {
  position: absolute;
  left: 10px;
  right: 10px;
  bottom: 10px;
  display: flex;
  align-items: center;
  pointer-events: none;
}
/* The name pill sits on a dark scrim of its own, so it reads over a tone and
   over video alike. */
.pill {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  max-width: 100%;
  height: 24px;
  padding: 0 9px 0 7px;
  box-sizing: border-box;
  border-radius: 999px;
  background: rgb(0 0 0 / 50%);
  color: #fff;
  font-size: 12px;
  font-weight: 600;
}
.name {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
