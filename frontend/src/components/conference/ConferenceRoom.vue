<script setup>
// The call itself (#2864, subtask #2871): stage + tiles + bottom toolbar, the
// layout from the mockup in the parent task.
//
// This component owns no media knowledge — it reads `useConfTransport` and
// renders. That split is what lets the SDK move underneath it (see the comment
// at the top of the composable).
//
// The chat rail, the participants panel and screen share land in #2872–#2874;
// the grid already leaves room for them rather than being rebuilt later.
import { computed, watch } from 'vue'
import { NButton, NIcon, NSpin, NTooltip } from 'naive-ui'
import {
  MicOutline,
  MicOffOutline,
  VideocamOutline,
  VideocamOffOutline,
  CallOutline,
  VolumeHighOutline,
  WarningOutline,
} from '@vicons/ionicons5'
import {
  useConfTransport,
  mediaSupported,
  CONNECTING,
  ERROR,
  UNAVAILABLE,
} from '@/composables/useConfTransport'
import ParticipantTile from './ParticipantTile.vue'
import DeviceMenu from './DeviceMenu.vue'

const props = defineProps({
  conferenceId: { type: String, required: true },
  // The room connects only once the user is a participant of record. Membership
  // is decided above this component (the «Подключиться» button), and media
  // follows it — never the other way round, or someone who left the roster
  // would still be heard by everyone in the call.
  active: { type: Boolean, default: false },
  ended: { type: Boolean, default: false },
})
// `hangup` asks the parent to leave the roster too; the parent flipping `active`
// is what actually drops the media session, through the watcher below. One
// direction of control, so the two halves cannot disagree.
const emit = defineEmits(['hangup'])

const {
  status,
  error,
  dominant,
  others,
  connected,
  micOn,
  camOn,
  audioBlocked,
  devices,
  selected,
  join,
  leave,
  toggleMic,
  toggleCam,
  selectDevice,
  unblockAudio,
} = useConfTransport()

watch(
  () => [props.active, props.ended, props.conferenceId],
  ([active, ended]) => {
    if (active && !ended) join(props.conferenceId)
    else leave()
  },
  { immediate: true },
)

const supported = mediaSupported()
const busy = computed(() => status.value === CONNECTING)
</script>

<template>
  <div class="room" data-testid="conference-room">
    <!-- Not a secure context: the browser hides the devices outright, so this is
         a statement of fact rather than a failure to retry. -->
    <div
      v-if="!supported || status === UNAVAILABLE"
      class="notice"
      data-testid="conference-insecure"
    >
      <n-icon :component="WarningOutline" :size="16" />
      <span>{{ $t('conferences.media.insecure') }}</span>
    </div>

    <div v-else-if="!active" class="notice idle">
      {{ $t('conferences.media.notJoined') }}
    </div>

    <template v-else>
      <div v-if="status === ERROR" class="notice err" data-testid="conference-media-error">
        <n-icon :component="WarningOutline" :size="16" />
        <span>{{ error || $t('conferences.media.failed') }}</span>
        <n-button size="tiny" quaternary @click="join(conferenceId)">
          {{ $t('conferences.media.retry') }}
        </n-button>
      </div>

      <n-spin :show="busy">
        <div class="stage-wrap">
          <participant-tile v-if="dominant" :peer="dominant" stage />
          <div v-else class="notice idle">{{ $t('conferences.media.connecting') }}</div>

          <div v-if="others.length" class="strip" data-testid="conference-strip">
            <participant-tile v-for="p in others" :key="p.sid || p.id" :peer="p" />
          </div>
        </div>
      </n-spin>

      <!-- Autoplay policy: sound stays blocked until a real gesture, and without
           this button the whole call is silent with nothing to say why. -->
      <div v-if="audioBlocked" class="notice" data-testid="conference-audio-blocked">
        <n-icon :component="VolumeHighOutline" :size="16" />
        <span>{{ $t('conferences.media.audioBlocked') }}</span>
        <n-button size="tiny" type="primary" @click="unblockAudio()">
          {{ $t('conferences.media.enableAudio') }}
        </n-button>
      </div>

      <div class="toolbar">
        <n-tooltip>
          <template #trigger>
            <n-button
              circle
              :type="micOn ? 'primary' : 'default'"
              :disabled="!connected"
              data-testid="conference-mic"
              @click="toggleMic()"
            >
              <n-icon :component="micOn ? MicOutline : MicOffOutline" />
            </n-button>
          </template>
          {{ micOn ? $t('conferences.media.muteMic') : $t('conferences.media.unmuteMic') }}
        </n-tooltip>

        <n-tooltip>
          <template #trigger>
            <n-button
              circle
              :type="camOn ? 'primary' : 'default'"
              :disabled="!connected"
              data-testid="conference-cam"
              @click="toggleCam()"
            >
              <n-icon :component="camOn ? VideocamOutline : VideocamOffOutline" />
            </n-button>
          </template>
          {{ camOn ? $t('conferences.media.camOff') : $t('conferences.media.camOn') }}
        </n-tooltip>

        <device-menu
          :devices="devices"
          :selected="selected"
          :disabled="!connected"
          @select="selectDevice"
        />

        <n-tooltip>
          <template #trigger>
            <n-button circle type="error" data-testid="conference-hangup" @click="emit('hangup')">
              <n-icon :component="CallOutline" />
            </n-button>
          </template>
          {{ $t('conferences.media.hangup') }}
        </n-tooltip>
      </div>
    </template>
  </div>
</template>

<style scoped>
.room {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.notice {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 10px;
  border-radius: 8px;
  font-size: 12px;
  color: var(--t-text2);
  background: var(--t-hover);
}
.notice.idle {
  justify-content: center;
  color: var(--t-text3);
  min-height: 120px;
}
.notice.err {
  color: #d03050;
}
.stage-wrap {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
/* Auto-fill, not a fixed count: the same strip has to hold two people and ten,
   and a wrapping grid degrades to a single column on the mobile layout for
   free. */
.strip {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
  gap: 8px;
}
.toolbar {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  padding-top: 4px;
}
</style>
