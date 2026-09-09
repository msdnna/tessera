<script setup>
// One remote audio track, played through a hidden <audio> (#2888).
//
// Split out from the tile on purpose: playback has to keep running while a call
// is minimised, when only a single tile (or none) is on screen. This element is
// mounted by ConferenceAudioSink for the life of the track instead, so which
// tiles are visible no longer decides whether the room can be heard.
import { onBeforeUnmount, ref, watch } from 'vue'

const props = defineProps({
  // A live LiveKit audio track. Never the local mic — playing our own audio back
  // is a feedback loop; the sink filters that out before it gets here.
  track: { type: Object, required: true },
})

const el = ref(null)

watch(
  () => [props.track, el.value],
  ([track], prev) => {
    const old = prev?.[0]
    if (old && old !== track) old.detach(el.value)
    if (track && el.value) track.attach(el.value)
    else if (el.value?.srcObject) el.value.srcObject = null
  },
  { immediate: true, flush: 'post' },
)

onBeforeUnmount(() => {
  if (props.track && el.value) props.track.detach(el.value)
})
</script>

<template>
  <audio ref="el" autoplay />
</template>
