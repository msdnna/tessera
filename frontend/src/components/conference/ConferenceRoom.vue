<script setup>
// The call itself (#2864, subtask #2871): stage + tiles + bottom toolbar, the
// layout from the mockup in the parent task.
//
// This component owns no media knowledge — it reads `useConfTransport` and
// renders. That split is what lets the SDK move underneath it (see the comment
// at the top of the composable).
//
// It is also where the call's two connections meet (#2872). Media goes to the
// SFU through the transport; presence, hands and moderation go to our own
// server through `useConfRoom`. Neither knows about the other, and this file is
// the only place that has to: it forwards our device state to the room so the
// roster can paint it, and it obeys a force-mute the room reports by actually
// stopping the microphone.
//
// The right-hand rail carries both the roster and the chat (#2873), switched by
// a tab rather than stacked: at 264px they would each get half a panel, and the
// participant rows already have a fixed ideal width. Screen share lands in
// #2874.
import { computed, ref, watch } from 'vue'
import { NBadge, NButton, NIcon, NSpin, NTooltip } from 'naive-ui'
import {
  MicOutline,
  MicOffOutline,
  VideocamOutline,
  VideocamOffOutline,
  CallOutline,
  HandRightOutline,
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
import { useConfRoom } from '@/composables/useConfRoom'
import ParticipantTile from './ParticipantTile.vue'
import ParticipantsPanel from './ParticipantsPanel.vue'
import ConferenceChat from './ConferenceChat.vue'
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
  peers,
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
  setMic,
  setPeerVolume,
  togglePeerMute,
  selectDevice,
  unblockAudio,
} = useConfTransport()

const room = useConfRoom()

watch(
  () => [props.active, props.ended, props.conferenceId],
  ([active, ended]) => {
    if (active && !ended) {
      // The room socket opens even where media cannot: on the dev stand there
      // is no camera to be had, and a roster that still works is more useful
      // than a screen that gives up entirely.
      room.open(props.conferenceId)
      join(props.conferenceId)
    } else {
      room.close()
      leave()
    }
  },
  { immediate: true },
)

// Tell the room what our devices are doing. The SFU knows, but the roster is
// built from the room socket, and a badge that waits for the first audio packet
// shows everyone as muted for the first second of every call.
watch([micOn, camOn], ([mic, cam]) => room.setMedia(mic, cam))

// Obey a force-mute locally instead of only painting it. The server has already
// narrowed our publish permission at the SFU, so the microphone is going quiet
// either way — doing it here as well is what makes the toolbar button agree with
// what the room can hear, rather than showing an active mic that publishes
// nothing.
watch(
  () => room.forceMuted.value,
  (forced) => {
    if (forced && micOn.value) setMic(false)
  },
)

// The call ended, or we were removed. Either way the media session has to go:
// the room socket is already closed, and leaving the SFU connection up would
// keep publishing into a meeting we are no longer part of.
watch(
  () => room.ended.value,
  (over) => {
    if (over) leave()
  },
)

const handUp = computed(() => !!room.self.value?.hand_at)
const supported = mediaSupported()
const busy = computed(() => status.value === CONNECTING)

// Which half of the rail is showing. The roster opens first: knowing who is in
// the call is what you need at second zero, the chat is what you need at minute
// three.
const rail = ref('people')
// Unread count while the chat is hidden. The room's nudge is payload-free, so
// this counts nudges, not messages — close enough for a dot that only says
// "something was said", and it costs no extra request.
const unread = ref(0)
watch(
  () => room.chatNudge.value,
  () => {
    if (rail.value !== 'chat') unread.value += 1
  },
)
watch(rail, (tab) => {
  if (tab === 'chat') unread.value = 0
})
// Leaving the call resets the badge: a "3 unread" carried into the next meeting
// would point at a conversation this rail is no longer showing.
watch(
  () => props.active,
  () => {
    unread.value = 0
    rail.value = 'people'
  },
)
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

      <!-- A refused command, answered by the server rather than guessed at by
           the UI: the buttons are hidden for a member, so seeing this means
           something more interesting than a misclick. -->
      <div v-if="room.denied.value" class="notice err" data-testid="conference-denied">
        <n-icon :component="WarningOutline" :size="16" />
        <span>{{ $t(`conferences.panel.denied.${room.denied.value.action || 'other'}`) }}</span>
      </div>

      <div class="body">
        <n-spin :show="busy" class="stage-col">
          <div class="stage-wrap">
            <participant-tile v-if="dominant" :peer="dominant" stage />
            <div v-else class="notice idle">{{ $t('conferences.media.connecting') }}</div>

            <div v-if="others.length" class="strip" data-testid="conference-strip">
              <participant-tile v-for="p in others" :key="p.sid || p.id" :peer="p" />
            </div>
          </div>
        </n-spin>

        <aside class="rail">
          <div class="rail-tabs">
            <!-- `ngrad` opts these out of the accent-gradient rule in main.css:
                 it paints any primary button that is not ghost/dashed/secondary
                 with a solid gradient, and on a *quaternary* primary that lands
                 accent text on an accent block — the active tab loses its label
                 entirely. -->
            <n-button
              size="tiny"
              :type="rail === 'people' ? 'primary' : 'default'"
              quaternary
              class="ngrad"
              data-testid="conference-rail-people"
              @click="rail = 'people'"
            >
              {{ $t('conferences.panel.title', { count: room.participants.value.length }) }}
            </n-button>
            <n-badge :value="unread" :max="9" :show="rail !== 'chat' && unread > 0" dot>
              <n-button
                size="tiny"
                :type="rail === 'chat' ? 'primary' : 'default'"
                quaternary
                class="ngrad"
                data-testid="conference-rail-chat"
                @click="rail = 'chat'"
              >
                {{ $t('conferences.chat.tab') }}
              </n-button>
            </n-badge>
          </div>

          <participants-panel
            v-show="rail === 'people'"
            :people="room.participants.value"
            :peers="peers"
            :me-id="room.userId.value"
            :can-moderate="room.isHost.value"
            @volume="setPeerVolume"
            @local-mute="togglePeerMute"
            @kick="room.kick"
            @force-mute="room.forceMute"
          />
          <!-- v-show, not v-if: switching back to the chat must not throw away
               the loaded history and refetch it every time. -->
          <conference-chat
            v-show="rail === 'chat'"
            :conference-id="conferenceId"
            :nudge="room.chatNudge.value"
            :readonly="ended"
            :can-moderate="room.isHost.value"
            :visible="rail === 'chat'"
          />
        </aside>
      </div>

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
            <!-- Disabled under a force-mute rather than hidden: the server will
                 refuse the publish anyway, and a button that silently does
                 nothing reads as a broken microphone. -->
            <n-button
              circle
              :type="micOn ? 'primary' : 'default'"
              :disabled="!connected || room.forceMuted.value"
              data-testid="conference-mic"
              @click="toggleMic()"
            >
              <n-icon :component="micOn ? MicOutline : MicOffOutline" />
            </n-button>
          </template>
          {{
            room.forceMuted.value
              ? $t('conferences.panel.youAreForceMuted')
              : micOn
                ? $t('conferences.media.muteMic')
                : $t('conferences.media.unmuteMic')
          }}
        </n-tooltip>

        <n-tooltip>
          <template #trigger>
            <n-button
              circle
              :type="handUp ? 'warning' : 'default'"
              :disabled="!room.connected.value"
              data-testid="conference-hand"
              @click="room.raiseHand(!handUp)"
            >
              <n-icon :component="HandRightOutline" />
            </n-button>
          </template>
          {{ handUp ? $t('conferences.panel.lowerHand') : $t('conferences.panel.raiseHand') }}
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
/* Stage and roster side by side on a desktop, stacked below it. The rail is
   given a fixed basis rather than a fraction: the participant rows have a fixed
   ideal width (avatar + name + slider), and letting them grow with the viewport
   would stretch the slider across half the screen. */
.body {
  display: flex;
  align-items: flex-start;
  gap: 12px;
}
.stage-col {
  flex: 1;
  min-width: 0;
}
/* A column, not a plain block: the chat inside brings its own scroller for the
   log and has to keep its composer pinned under it. overflow-y stays for the
   roster, which is a flat list and does want to scroll as a whole. */
.rail {
  flex: none;
  display: flex;
  flex-direction: column;
  width: 264px;
  max-height: 60vh;
  overflow-y: auto;
  padding: 8px;
  border: 1px solid var(--t-border);
  border-radius: 10px;
}
.rail-tabs {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 0 4px 6px;
}
@media (max-width: 900px) {
  .body {
    flex-direction: column;
  }
  .rail {
    width: 100%;
    max-height: none;
  }
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
