<script setup>
// One participant in the call (#2864, subtask #2871).
//
// Video is the exception here, not the rule: we are told the team rarely turns a
// camera on, so the tile is designed around the avatar and merely makes room for
// a stream when one shows up.
import { onBeforeUnmount, ref, watch } from 'vue'
import { NIcon } from 'naive-ui'
import { MicOffOutline } from '@vicons/ionicons5'
import UserAvatar from '@/components/UserAvatar.vue'

const props = defineProps({
  // The flat descriptor from useConfTransport — never a live SDK object.
  peer: { type: Object, required: true },
  // The dominant-speaker tile: same markup, more room.
  stage: { type: Boolean, default: false },
})

const videoEl = ref(null)
const audioEl = ref(null)

// attach/detach rather than assigning srcObject: the SDK keeps its own list of
// attached elements and uses it to decide whether a track is still being
// watched — that is what makes adaptiveStream stop paying for a hidden tile.
function bind(track, el) {
  if (!el) return
  if (track) track.attach(el)
  else if (el.srcObject) el.srcObject = null
}

watch(
  () => [props.peer.videoTrack, videoEl.value],
  ([track], prev) => {
    const old = prev?.[0]
    // Detaching only our own element, not everywhere: the same camera track is
    // legitimately on screen twice while this person holds the stage.
    if (old && old !== track) old.detach(videoEl.value)
    bind(track, videoEl.value)
  },
  { immediate: true, flush: 'post' },
)

watch(
  () => [props.peer.audioTrack, audioEl.value],
  ([track], prev) => {
    const old = prev?.[0]
    if (old && old !== track) old.detach(audioEl.value)
    bind(track, audioEl.value)
  },
  { immediate: true, flush: 'post' },
)

onBeforeUnmount(() => {
  // A tile that goes away without detaching leaves the SDK believing someone is
  // still watching, and the stream keeps being paid for.
  if (props.peer.videoTrack && videoEl.value) props.peer.videoTrack.detach(videoEl.value)
  if (props.peer.audioTrack && audioEl.value) props.peer.audioTrack.detach(audioEl.value)
})
</script>

<template>
  <div
    class="tile"
    :class="{ stage, speaking: peer.speaking, video: !!peer.videoTrack }"
    data-testid="conference-tile"
  >
    <!-- muted on the local tile is not cosmetic: an unmuted self-view is a
         feedback loop through the room's speakers. -->
    <video v-show="peer.videoTrack" ref="videoEl" class="v" autoplay playsinline muted />
    <audio v-if="!peer.local" ref="audioEl" autoplay />

    <div v-if="!peer.videoTrack" class="face">
      <user-avatar :user-id="peer.id" :name="peer.name" class="av" />
    </div>

    <div class="bar">
      <span class="name">{{ peer.name }}</span>
      <n-icon v-if="!peer.micOn" :component="MicOffOutline" :size="14" class="muted-icon" />
    </div>
  </div>
</template>

<style scoped>
.tile {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  aspect-ratio: 16 / 9;
  min-height: 96px;
  border-radius: 10px;
  overflow: hidden;
  background: var(--t-hover);
  /* Transparent border reserved up front so the speaking ring does not resize
     the tile when it appears (the border-box trick used across the app). */
  border: 2px solid transparent;
}
/* The stage keeps 16:9 but is capped by the viewport: at a desktop width the
   ratio alone would push the toolbar below the fold, and a call whose controls
   need scrolling to reach is worse than a smaller speaker. */
.tile.stage {
  min-height: 220px;
  max-height: 44vh;
}
/* A live stream is dark by nature; the flat neutral behind it would show as a
   grey frame in the letterbox until the first frame lands. */
.tile.video {
  background: #000;
}
.tile.speaking {
  border-color: #18a058;
}
.v {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.face {
  display: flex;
  align-items: center;
  justify-content: center;
}
/* Same circular avatar as everywhere else in the app, just larger. The gradient
   comes from the accent-gradient helper on the shared component. */
.av {
  width: 56px;
  height: 56px;
  border-radius: 50%;
  font-size: 20px;
  color: #fff;
  background: var(--t-accent-grad);
}
.stage .av {
  width: 96px;
  height: 96px;
  font-size: 32px;
}
.bar {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 0;
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 4px 8px;
  font-size: 12px;
  color: var(--t-text2);
}
/* Only a video tile gets the scrim. Over an arbitrary camera frame there is no
   safe text colour, so white on a dark fade is the only thing that always
   reads; over the flat neutral of an avatar tile the same scrim is a dark smear
   across a light UI, and the design language keeps neutrals flat. */
.tile.video .bar {
  color: #fff;
  background: linear-gradient(to top, rgb(0 0 0 / 55%), transparent);
}
.name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.muted-icon {
  flex: none;
  opacity: 0.85;
}
</style>
