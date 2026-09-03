<script setup>
// The call's audio, decoupled from the visuals (#2888).
//
// Mounted once at the app root for the whole life of a session, so every remote
// participant is heard no matter which view is on screen — the full room, the
// floating mini-window, or neither. Before this, audio was attached inside each
// tile, so minimising the call (one tile) or navigating away (no tiles) silenced
// everyone and expanding again audibly re-attached the streams.
import { computed } from 'vue'
import { useConferenceSession } from '@/stores/conference'
import ConferenceAudioTrack from './ConferenceAudioTrack.vue'

const session = useConferenceSession()
const { peers } = session.transport

// One entry per remote audio source: a participant's microphone and, when they
// are presenting, their shared tab's audio. Local tracks are skipped — hearing
// our own mic back is a feedback loop.
const sources = computed(() => {
  const out = []
  for (const p of peers.value) {
    if (p.local) continue
    if (p.audioTrack) out.push({ key: `${p.id}:mic`, track: p.audioTrack })
    if (p.screenAudioTrack) out.push({ key: `${p.id}:screen`, track: p.screenAudioTrack })
  }
  return out
})
</script>

<template>
  <div class="conf-audio-sink" aria-hidden="true">
    <conference-audio-track v-for="s in sources" :key="s.key" :track="s.track" />
  </div>
</template>

<style scoped>
/* Purely functional — it plays sound, it shows nothing. */
.conf-audio-sink {
  position: absolute;
  width: 0;
  height: 0;
  overflow: hidden;
}
</style>
