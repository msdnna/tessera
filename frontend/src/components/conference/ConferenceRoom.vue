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
// participant rows already have a fixed ideal width.
//
// Screen share (#2874) is the third thing that meets here, and the only one
// where the two connections disagree by design: our server owns the *stage* (who
// may present, who waits, and a host preempting a member), the SFU owns the
// *pixels*. This file is what keeps them honest — a capture that turns out not
// to hold the stage is stopped rather than published alongside someone else's.
import { computed, onBeforeUnmount, ref, watch } from 'vue'
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
  DesktopOutline,
  StopCircleOutline,
  PersonAddOutline,
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
  // Whether the «Пригласить» control belongs in the rail (#2881). The invite
  // dialog itself lives in the parent view, so pressing it only emits upward.
  canInvite: { type: Boolean, default: false },
})
// `hangup` asks the parent to leave the roster too; the parent flipping `active`
// is what actually drops the media session, through the watcher below. One
// direction of control, so the two halves cannot disagree. `invite` opens the
// parent's invite dialog — the roster moved in here, so the button did too.
const emit = defineEmits(['hangup', 'invite'])

const {
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

// ── screen share (#2874) ────────────────────────────────────────────────
// We asked for the stage and have not given up on it — which is not the same as
// holding it (the server decides that) and not the same as capturing (the
// browser does). Keeping the three apart is what lets the queue work: waiting
// for a turn, holding a turn we have not started capturing for, and actually
// sharing are three different buttons.
const wantScreen = ref(false)
// The snapshot number our request was sent at, so a stage that is not ours can
// be told from a stage the server has not answered about yet. Without it the
// very first watcher tick would stop the capture we just started.
let askedAt = 0
let heartbeat = null

/** Someone else is presenting: their pixels, or their claim on the stage. */
const otherPresenter = computed(() => {
  if (room.presenting.value) return null
  if (screenPeer.value && !screenPeer.value.local) return screenPeer.value.name
  return room.stage.value?.name || null
})
// The stage is ours, the capture is not running: the queue reached us while we
// were waiting. The picker cannot be reopened without a fresh click — a browser
// only grants getDisplayMedia inside a gesture — so this is a prompt, not
// something the code can do on the user's behalf.
const myTurn = computed(() => wantScreen.value && room.presenting.value && !screenOn.value)

/**
 * Toggle our screen share.
 *
 * Runs directly off the click, and the order inside matters. The room is told
 * first because it is the arbiter; the capture is started in the same tick
 * because `getDisplayMedia` is only granted inside a user gesture and any await
 * before it spends that gesture. When the stage is visibly held by someone else
 * we skip the picker entirely — making the user choose a window only to have it
 * stopped a moment later is worse than telling them they are in the queue.
 */
async function toggleScreen() {
  if (screenOn.value) {
    wantScreen.value = false
    await stopScreen()
    room.releaseScreen()
    return
  }
  if (wantScreen.value && !room.presenting.value) {
    // Cancelling a wait: "never mind" drops us out of the queue.
    wantScreen.value = false
    room.releaseScreen()
    return
  }
  wantScreen.value = true
  askedAt = room.stateSeq.value
  if (!room.presenting.value) room.requestScreen()
  if (room.stage.value && !room.presenting.value) return // queued; wait our turn
  if (!(await startScreen())) {
    // The picker was dismissed. Holding the stage after that would park the
    // queue behind a share that never starts.
    wantScreen.value = false
    room.releaseScreen()
  }
}

// The stage went to someone else while we were capturing — a host preempting a
// member, or our hold expiring after a network stall. Stopping here rather than
// waiting for the user is the whole point of having one arbiter: two screens
// published at once is exactly what the queue exists to prevent.
watch(
  () => [room.stateSeq.value, room.presenting.value, screenOn.value],
  () => {
    if (screenOn.value && !room.presenting.value && room.stateSeq.value > askedAt) {
      wantScreen.value = false
      stopScreen()
    }
  },
)

// Hold the stage while we are actually sharing. The server expires a hold that
// stops being refreshed, which is what frees the queue when a presenter's laptop
// lid closes without the socket noticing.
watch(
  () => [screenOn.value, room.presenting.value, room.stageTtlMs.value],
  ([sharing, mine, ttl]) => {
    clearInterval(heartbeat)
    heartbeat = null
    if (!sharing || !mine) return
    // A third of the TTL: two beats may be lost to a stalled connection before
    // the stage is taken away from someone who is still presenting.
    heartbeat = setInterval(() => room.refreshScreen(), Math.max(1000, ttl / 3))
  },
  { immediate: true },
)

// Leaving the call takes the stage with it: the socket is closing, and a hold
// nobody refreshes would keep the queue waiting out the TTL for nothing.
watch(
  () => [props.active, props.ended],
  ([active, ended]) => {
    if (active && !ended) return
    wantScreen.value = false
    clearInterval(heartbeat)
    heartbeat = null
  },
)

onBeforeUnmount(() => clearInterval(heartbeat))

// The stage shows a shared screen when there is one, and the speaker otherwise.
// A screen is always the thing people are looking at — that is why it was
// shared — so the dominant speaker steps down into the strip rather than
// competing with it.
const stagePeer = computed(() => screenPeer.value || dominant.value)
const stripPeers = computed(() => (screenPeer.value ? peers.value : others.value))

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
            <participant-tile
              v-if="stagePeer"
              :key="screenPeer ? `screen-${stagePeer.id}` : stagePeer.sid || stagePeer.id"
              :peer="stagePeer"
              stage
              :screen="!!screenPeer"
            />
            <div v-else class="notice idle">{{ $t('conferences.media.connecting') }}</div>

            <!-- Who holds the stage and who is behind them. The queue is the
                 server's, not a local guess: a member who asked while a host was
                 presenting has to see that they are waiting, not that their
                 click did nothing. -->
            <div
              v-if="room.stage.value || room.queuePos.value"
              class="stage-note"
              data-testid="conference-stage-note"
            >
              <n-icon :component="DesktopOutline" :size="14" />
              <!-- Holding the stage is not the same as filling it. Between the
                   queue reaching us and the picker being answered the stage is
                   already ours while nothing is captured, and saying "you are
                   presenting" there contradicts the "start sharing" prompt
                   printed directly underneath. -->
              <span v-if="room.presenting.value && screenOn">
                {{ $t('conferences.screen.youPresent') }}
              </span>
              <span v-else-if="room.presenting.value">
                {{ $t('conferences.screen.stageYours') }}
              </span>
              <span v-else-if="otherPresenter">
                {{ $t('conferences.screen.presenting', { name: otherPresenter }) }}
              </span>
              <span
                v-if="room.queuePos.value"
                class="qpos"
                data-testid="conference-screen-queuepos"
              >
                {{ $t('conferences.screen.queued', { n: room.queuePos.value }) }}
              </span>
              <span v-else-if="room.queue.value.length" class="qpos">
                {{ $t('conferences.screen.waiting', { count: room.queue.value.length }) }}
              </span>
            </div>

            <!-- Our turn came up while we waited. The browser will only reopen
                 the picker inside a click, so this has to be a button. -->
            <div v-if="myTurn" class="notice turn" data-testid="conference-screen-turn">
              <span>{{ $t('conferences.screen.yourTurn') }}</span>
              <n-button size="tiny" type="primary" @click="startScreen()">
                {{ $t('conferences.screen.start') }}
              </n-button>
            </div>

            <div v-if="stripPeers.length" class="strip" data-testid="conference-strip">
              <participant-tile v-for="p in stripPeers" :key="p.sid || p.id" :peer="p" />
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
            <!-- Invite lives here now (#2881), pushed to the far end of the tab
                 row so it reads as an action rather than a third tab. -->
            <n-button
              v-if="canInvite"
              size="tiny"
              quaternary
              class="invite"
              data-testid="conference-invite"
              @click="emit('invite')"
            >
              <template #icon><n-icon :component="PersonAddOutline" /></template>
              {{ $t('conferences.invite.button') }}
            </n-button>
          </div>

          <participants-panel
            v-show="rail === 'people'"
            :people="room.participants.value"
            :peers="peers"
            :me-id="room.userId.value"
            :can-moderate="room.canModerate.value"
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
            :can-moderate="room.canModerate.value"
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

        <n-tooltip>
          <template #trigger>
            <!-- Enabled even while someone else presents: pressing it is how you
                 get into the queue, and a disabled button would read as "screen
                 share is broken" rather than "wait your turn". -->
            <n-button
              circle
              :type="screenOn ? 'primary' : wantScreen ? 'warning' : 'default'"
              :disabled="!connected"
              data-testid="conference-screen"
              @click="toggleScreen()"
            >
              <n-icon :component="screenOn ? StopCircleOutline : DesktopOutline" />
            </n-button>
          </template>
          {{
            screenOn
              ? $t('conferences.screen.stop')
              : wantScreen
                ? $t('conferences.screen.cancel')
                : $t('conferences.screen.share')
          }}
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
/* Wider now that it is the only side panel (#2881): the participants column that
   used to sit beside it is gone, so the roster and chat get its room. */
.rail {
  flex: none;
  display: flex;
  flex-direction: column;
  width: 320px;
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
/* The invite action sits at the far end of the tab row, away from the two tabs. */
.rail-tabs .invite {
  margin-left: auto;
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
/* The stage caption: who is presenting and who is behind them. Wraps rather
   than truncates — the Russian strings are long, and "вы в очереди: 2" is the
   half that must not be the half that disappears. */
.stage-note {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--t-text2);
}
.qpos {
  color: var(--t-text3);
}
.notice.turn {
  justify-content: center;
}
/* Auto-fill, not a fixed count: the same strip has to hold two people and ten,
   and a wrapping grid degrades to a single column on the mobile layout for
   free. */
.strip {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
  gap: 12px;
}
.toolbar {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  padding-top: 4px;
}
</style>
