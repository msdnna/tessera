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
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { NBadge, NButton, NIcon, NInput, NPopover, NSpin, NTooltip } from 'naive-ui'
import {
  Mic,
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
  ExpandOutline,
  ContractOutline,
  RadioButtonOnOutline,
  SquareOutline,
  ChevronForwardOutline,
  ChevronBackOutline,
} from '@vicons/ionicons5'
import { conferences as confApi } from '@/api'
import { mediaSupported, CONNECTING, ERROR, UNAVAILABLE } from '@/composables/useConfTransport'
import { useConferenceSession } from '@/stores/conference'
import ParticipantTile from './ParticipantTile.vue'
import ParticipantsPanel from './ParticipantsPanel.vue'
import ConferenceChat from './ConferenceChat.vue'
import DeviceMenu from './DeviceMenu.vue'
import UserAvatar from '@/components/UserAvatar.vue'

const props = defineProps({
  conferenceId: { type: String, required: true },
  // The room connects only once the user is a participant of record. Membership
  // is decided above this component (the «Подключиться» button), and media
  // follows it — never the other way round, or someone who left the roster
  // would still be heard by everyone in the call.
  active: { type: Boolean, default: false },
  ended: { type: Boolean, default: false },
  // Whether the «Пригласить» control belongs in the rail (#2881). The picker is
  // a popover here now (#2891); the parent still owns the invite call.
  canInvite: { type: Boolean, default: false },
  // Workspace members who can still be invited (#2891): fed from the parent so
  // the popover's list is ready on click. [{ user_id, name, email }]
  invitable: { type: Array, default: () => [] },
  // People invited to the call who are not in the room yet (#2875): the roster
  // now lives only in this rail, so without listing them here an invitation would
  // show nowhere. [{ user_id, user_name, role }]
  invited: { type: Array, default: () => [] },
})
// `hangup` asks the parent to leave the roster too; the parent flipping `active`
// is what actually drops the media session, through the watcher below. One
// direction of control, so the two halves cannot disagree. `invite` carries the
// user id of the person to invite — the popover is here, the API call is the
// parent's (#2891).
const emit = defineEmits(['hangup', 'invite'])

// The session lives in the store now (#2888): one long-lived transport + room
// pair the whole app shares, so leaving this view for another section no longer
// drops the call — the floating ConferenceMiniWindow keeps it going. This
// component is the full-screen view of that same session; the open/join and the
// device/force-mute coordination moved up to the store, which outlives it.
const session = useConferenceSession()
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
  micLevel,
  denoise,
  denoiseAvailable,
  join,
  toggleMic,
  toggleCam,
  toggleDenoise,
  startScreen,
  stopScreen,
  setPeerVolume,
  togglePeerMute,
  selectDevice,
  unblockAudio,
} = session.transport

const room = session.room

const handUp = computed(() => !!room.self.value?.hand_at)
const supported = mediaSupported()
const busy = computed(() => status.value === CONNECTING)

// Own microphone meter on the toolbar button (#2883 round 2): the mic glyph fills
// from the bottom with our own live level. clip-path hides the filled copy from
// the top down, so a louder voice reveals more of it.
const micClip = computed(
  () => `inset(${Math.round((1 - Math.min(1, micLevel.value)) * 100)}% 0 0 0)`,
)

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

// Leaving the section no longer tears the call down — the session lives in the
// store and the mini-window takes over (#2888). Only this view's own screen-share
// heartbeat is local to the component, so that is all there is to clean up here.
onBeforeUnmount(() => {
  clearInterval(heartbeat)
})

// The stage shows a shared screen when there is one, and the speaker otherwise.
// A screen is always the thing people are looking at — that is why it was
// shared — so the dominant speaker steps down into the strip rather than
// competing with it.
const stagePeer = computed(() => screenPeer.value || dominant.value)
const stripPeers = computed(() => (screenPeer.value ? peers.value : others.value))

// ── fullscreen (#2882) ──────────────────────────────────────────────────
// Fullscreen the whole stage column — the shared screen plus the strip — rather
// than a single <video>: a presenter's IDE is the point of going fullscreen, and
// the faces are still worth seeing next to it. Driven off the browser's own
// fullscreenchange so the button label follows Esc as well as our own toggle.
const stageWrapEl = ref(null)
const isFullscreen = ref(false)
const fullscreenSupported = typeof document !== 'undefined' && (document.fullscreenEnabled ?? false)

function onFsChange() {
  isFullscreen.value = typeof document !== 'undefined' && !!document.fullscreenElement
}
if (typeof document !== 'undefined') document.addEventListener('fullscreenchange', onFsChange)
onBeforeUnmount(() => {
  if (typeof document !== 'undefined') document.removeEventListener('fullscreenchange', onFsChange)
})

async function toggleFullscreen() {
  const el = stageWrapEl.value
  if (!el) return
  try {
    if (document.fullscreenElement) await document.exitFullscreen()
    else await el.requestFullscreen?.()
  } catch {
    // Refused by policy or already changing; the button state follows the event.
  }
}

// ── recording (#2877) ───────────────────────────────────────────────────
// The button is a request, never a state: what it shows comes from the room's
// snapshot, the same source everyone else's red dot comes from. That is what
// keeps the host's own screen honest — a button that flipped to "recording" on
// the click would say we are recording during the second the egress worker is
// still failing to start.
const recBusy = ref(false)
// The server's refusal, verbatim and inline rather than as a toast: this
// component has no message provider (it is mounted inside the mini-window's
// session too), and "recording is not configured on this server" is a sentence
// the person pressing the button needs to keep reading, not a flash.
const recError = ref('')
const recording = computed(() => room.recording.value)

async function toggleRecording() {
  if (recBusy.value) return
  recBusy.value = true
  recError.value = ''
  try {
    if (recording.value) await confApi.stopRecording(props.conferenceId)
    else await confApi.startRecording(props.conferenceId)
    // The dot is not set here on purpose — it arrives in the next snapshot.
  } catch (e) {
    recError.value = e.response?.data?.error || e.message
  } finally {
    recBusy.value = false
  }
}

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

// ── rail collapse (#2891) ────────────────────────────────────────────────
// Fold the roster/chat rail away like the app sidebar, so the stage takes the
// whole width when you just want to watch. Remembered in localStorage (non-
// critical UX state, per the project's convention) — refolding it on every join
// would be a chore. On a narrow screen the rail already stacks under the stage,
// where a collapse toggle is meaningless, so it is hidden there and the rail
// always shows (`railVisible`).
const RAIL_KEY = 'tessera_conf_rail'
const railOpen = ref(
  (typeof localStorage !== 'undefined' ? localStorage.getItem(RAIL_KEY) : null) !== '0',
)
const roomNarrow = ref(false)
let railMq = null
function onRailMq(e) {
  roomNarrow.value = e.matches
}
onMounted(() => {
  if (typeof window !== 'undefined' && window.matchMedia) {
    railMq = window.matchMedia('(max-width: 900px)')
    roomNarrow.value = railMq.matches
    railMq.addEventListener?.('change', onRailMq)
  }
})
onBeforeUnmount(() => railMq?.removeEventListener?.('change', onRailMq))
watch(railOpen, (open) => {
  try {
    localStorage.setItem(RAIL_KEY, open ? '1' : '0')
  } catch {
    // private mode / storage disabled — the toggle still works this session
  }
})
// The rail is on screen when it is not collapsed, or when the layout is narrow
// enough that it stacks under the stage regardless of the toggle.
const railVisible = computed(() => railOpen.value || roomNarrow.value)
function toggleRail() {
  railOpen.value = !railOpen.value
}

// ── invite popover (#2891) ───────────────────────────────────────────────
// A click-popover in the rail, like the assignee picker: search over the
// workspace members the parent says are still invitable, one click invites that
// person. No batch select and no «Отправить» — the parent refetches the roster,
// so the invited person drops out of `invitable` (this list) and appears in the
// «приглашённые» block below on the next tick.
const inviteSearch = ref('')
const invitableFiltered = computed(() => {
  const q = inviteSearch.value.trim().toLowerCase()
  if (!q) return props.invitable
  return props.invitable.filter((m) => (m.name || m.email || '').toLowerCase().includes(q))
})
function pickInvite(m) {
  emit('invite', m.user_id)
  // Clear the query so the now-shorter list is visible, not filtered by a name
  // that just left it.
  inviteSearch.value = ''
}
function onInviteShow(show) {
  if (!show) inviteSearch.value = ''
}
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

      <!-- Recording refused by the server (#2877): a server without the egress
           profile running answers here, and the moderator who pressed the button
           is the only one who sees it. -->
      <div v-if="recError" class="notice err" data-testid="conference-recording-error">
        <n-icon :component="WarningOutline" :size="16" />
        <span>{{ recError }}</span>
      </div>

      <div class="body">
        <n-spin :show="busy" class="stage-col">
          <div ref="stageWrapEl" class="stage-wrap">
            <!-- «Идёт запись», for everyone in the call and not only for whoever
                 may stop it (#2877). Overlaid on the stage rather than placed in
                 the notice strip above it so it survives fullscreen — that is
                 where a shared screen is watched, and it is exactly where a
                 recording notice must not disappear. The name answers the
                 question people actually ask, which is by whom. -->
            <div v-if="recording" class="rec-badge" data-testid="conference-recording-dot">
              <span class="rec-dot" />
              <span>{{ $t('conferences.rec.live') }}</span>
              <span v-if="recording.started_by" class="rec-by">
                {{ $t('conferences.rec.by', { name: recording.started_by }) }}
              </span>
            </div>
            <!-- Fullscreen lives on the stage itself (#2882), most useful while a
                 screen is shared. It fullscreens this column, not one <video>.
                 v-if on the tooltip, not the button: an empty trigger slot (when
                 fullscreen is unsupported) makes naive throw. -->
            <n-tooltip v-if="fullscreenSupported && stagePeer">
              <template #trigger>
                <n-button
                  class="fs-btn"
                  circle
                  size="small"
                  secondary
                  data-testid="conference-fullscreen"
                  @click="toggleFullscreen"
                >
                  <n-icon :component="isFullscreen ? ContractOutline : ExpandOutline" />
                </n-button>
              </template>
              {{
                isFullscreen
                  ? $t('conferences.media.exitFullscreen')
                  : $t('conferences.media.fullscreen')
              }}
            </n-tooltip>
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

        <!-- Fold the rail away like the app sidebar (#2891). Hidden on a narrow
             layout, where the rail stacks under the stage and there is nothing to
             fold. Sits in the seam between stage and rail. -->
        <button
          v-if="!roomNarrow"
          type="button"
          class="rail-toggle"
          data-testid="conference-rail-toggle"
          :aria-label="
            railOpen ? $t('conferences.panel.hideRail') : $t('conferences.panel.showRail')
          "
          @click="toggleRail"
        >
          <n-icon :component="railOpen ? ChevronForwardOutline : ChevronBackOutline" :size="16" />
        </button>

        <aside v-show="railVisible" class="rail">
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
                 row so it reads as an action rather than a third tab. The picker
                 is a click-popover (#2891), like the assignee picker. -->
            <n-popover
              v-if="canInvite"
              trigger="click"
              placement="bottom-end"
              class="invite-pop"
              @update:show="onInviteShow"
            >
              <template #trigger>
                <n-button size="tiny" quaternary class="invite" data-testid="conference-invite">
                  <template #icon><n-icon :component="PersonAddOutline" /></template>
                  {{ $t('conferences.invite.button') }}
                </n-button>
              </template>
              <div class="invite-body">
                <n-input
                  v-model:value="inviteSearch"
                  size="small"
                  clearable
                  :placeholder="$t('conferences.invite.search')"
                  data-testid="conference-invite-search"
                />
                <div class="invite-list">
                  <button
                    v-for="m in invitableFiltered"
                    :key="m.user_id"
                    type="button"
                    class="invite-item"
                    data-testid="conference-invite-item"
                    @click="pickInvite(m)"
                  >
                    <user-avatar :user-id="m.user_id" :name="m.name || m.email" class="iiav" />
                    <span class="iiname">{{ m.name || m.email }}</span>
                  </button>
                  <div v-if="!invitableFiltered.length" class="invite-empty">
                    {{
                      invitable.length
                        ? $t('conferences.invite.noMatch')
                        : $t('conferences.invite.allInvited')
                    }}
                  </div>
                </div>
              </div>
            </n-popover>
          </div>

          <div v-show="rail === 'people'" class="people-tab">
            <participants-panel
              :people="room.participants.value"
              :peers="peers"
              :me-id="room.userId.value"
              :can-moderate="room.canModerate.value"
              @volume="setPeerVolume"
              @local-mute="togglePeerMute"
              @kick="room.kick"
              @force-mute="room.forceMute"
            />
            <!-- Invited but not in the room yet (#2875). Kept apart from the live
                 roster above so «в комнате» stays an honest count. -->
            <div v-if="invited.length" class="invited" data-testid="conference-invited">
              <div class="invited-h">{{ $t('conferences.panel.invitedTitle') }}</div>
              <div v-for="p in invited" :key="p.user_id" class="invited-row">
                <user-avatar :user-id="p.user_id" :name="p.user_name" class="iav" />
                <span class="iname">{{ p.user_name }}</span>
                <span class="itag">{{ $t('conferences.presence.invited') }}</span>
              </div>
            </div>
          </div>
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
              <!-- Live level while the mic is on and not force-muted: a ghosted
                   base outline with a solid copy filling from the bottom. -->
              <span
                v-if="micOn && !room.forceMuted.value"
                class="mic-meter"
                data-testid="conference-mic-level"
              >
                <n-icon :component="MicOutline" class="mic-base" />
                <span class="mic-fill" :style="{ clipPath: micClip }">
                  <n-icon :component="Mic" />
                </span>
              </span>
              <n-icon v-else :component="micOn ? MicOutline : MicOffOutline" />
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

        <!-- Recording (#2877) — moderators only, because only they may press it.
             Hidden rather than disabled: unlike the microphone, an ordinary
             participant has nothing to learn from a control they can never use,
             and the red dot already tells them the thing that concerns them. -->
        <n-tooltip v-if="room.canModerate.value">
          <template #trigger>
            <n-button
              circle
              :type="recording ? 'error' : 'default'"
              :loading="recBusy"
              :disabled="!room.connected.value || recBusy"
              data-testid="conference-record"
              @click="toggleRecording()"
            >
              <n-icon :component="recording ? SquareOutline : RadioButtonOnOutline" />
            </n-button>
          </template>
          {{ recording ? $t('conferences.rec.stop') : $t('conferences.rec.start') }}
        </n-tooltip>

        <device-menu
          :devices="devices"
          :selected="selected"
          :disabled="!connected"
          :denoise="denoise"
          :denoise-available="denoiseAvailable"
          @select="selectDevice"
          @toggle-denoise="toggleDenoise"
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
/* Split-pane divider that folds the rail away (#2891), styled like the app
   sidebar's resizer: a slim full-height bar in the seam between stage and rail. */
.rail-toggle {
  flex: none;
  align-self: stretch;
  width: 18px;
  min-height: 44px;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: 1px solid var(--t-border);
  border-radius: 8px;
  background: var(--t-surface);
  color: var(--t-text3);
  cursor: pointer;
  transition:
    background 0.15s,
    color 0.15s;
}
.rail-toggle:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
/* Invite popover (#2891): search + a scrollable list of avatar rows. */
.invite-body {
  width: 240px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.invite-list {
  max-height: 260px;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.invite-item {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  padding: 6px 8px;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: var(--t-text1);
  text-align: left;
  cursor: pointer;
}
.invite-item:hover {
  background: var(--t-hover);
}
.iiav {
  flex: none;
  width: 26px;
  height: 26px;
  border-radius: 50%;
  font-size: 12px;
  color: #fff;
  background: var(--t-accent-grad);
}
.iiname {
  flex: 1;
  min-width: 0;
  font-size: 13px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.invite-empty {
  padding: 8px;
  font-size: 12px;
  color: var(--t-text3);
  text-align: center;
}
.people-tab {
  display: flex;
  flex-direction: column;
}
/* Invited-but-absent list (#2875), set off from the live roster by a divider so
   «в комнате» stays a count of who is actually present. */
.invited {
  margin-top: 6px;
  padding-top: 8px;
  border-top: 1px solid var(--t-border);
}
.invited-h {
  padding: 0 8px 4px;
  font-size: 11px;
  color: var(--t-text3);
}
.invited-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 8px;
}
.iav {
  flex: none;
  width: 28px;
  height: 28px;
  border-radius: 50%;
  font-size: 12px;
  color: #fff;
  background: var(--t-accent-grad);
  opacity: 0.7;
}
.iname {
  flex: 1;
  min-width: 0;
  font-size: 13px;
  color: var(--t-text2);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.itag {
  flex: none;
  font-size: 11px;
  color: var(--t-text3);
}
/* Stacked layout (#2893, round 2). Two things had to change together.
   `align-items: flex-start` is right for the row — it keeps the rail from
   stretching to the stage's height — but in a column it makes both children
   shrink to their content, so the stage ended up 126px wide next to a rail that
   asked for the full width. And that width was `100%`: nothing in this app sets
   a global `box-sizing`, so the rail's own 8px padding and 1px border land
   OUTSIDE the 100%, pushing its right edge to 399px in a 393px viewport — the
   panel «slightly out to the right» from the report. `stretch` gives both the
   pane's width with no percentage to overflow past. */
@media (max-width: 900px) {
  .body {
    flex-direction: column;
    align-items: stretch;
  }
  .rail {
    width: auto;
    max-height: none;
  }
}
.stage-wrap {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
/* Overlaid on the stage tile's top-right corner (#2882). z-index clears the
   video, and the tile's own rounded corners sit under it. */
.fs-btn {
  position: absolute;
  top: 8px;
  right: 8px;
  z-index: 2;
}
/* In fullscreen the column owns the whole screen, so the stage tile is let off
   its viewport cap and the padding keeps it clear of the edges. */
.stage-wrap:fullscreen {
  justify-content: center;
  padding: 16px;
  background: var(--t-bg);
}
.stage-wrap:fullscreen :deep(.tile.stage) {
  max-height: 92vh;
}
/* «Идёт запись» over the stage's top-left corner (#2877), opposite the
   fullscreen button. Its own dark scrim rather than a theme surface: it sits on
   video, which is dark in both themes, and a light pill there would vanish
   against a bright slide. */
.rec-badge {
  position: absolute;
  top: 8px;
  left: 8px;
  z-index: 2;
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 3px 10px;
  border-radius: 999px;
  font-size: 12px;
  color: #fff;
  background: rgba(0, 0, 0, 0.55);
  pointer-events: none;
}
.rec-by {
  opacity: 0.75;
}
.rec-dot {
  flex: none;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #d03050;
  animation: rec-pulse 1.6s ease-in-out infinite;
}
/* Held still under prefers-reduced-motion: a blinking dot is exactly the kind of
   thing that setting exists for, and the badge reads without the animation. */
@media (prefers-reduced-motion: reduce) {
  .rec-dot {
    animation: none;
  }
}
@keyframes rec-pulse {
  50% {
    opacity: 0.25;
  }
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
/* Small fixed-size tiles, centred and wrapping (#2881). NOT 1fr: stretching the
   tracks to fill made each tile grow, and with the tile's 16:9 aspect-ratio a
   wide tile also grew tall, swallowing the gap — the strip read as one solid
   block. A bounded width keeps the tiles compact (like the mockup) and the gap
   visible. The stage tile is separate, so a shared screen still fills its own. */
.strip {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(150px, 176px));
  /* Left-aligned, not centred: with a couple of tiles a centred row drifts off
     the left edge and reads as misaligned against the stage above it. */
  justify-content: start;
  gap: 14px;
}
/* The call controls sit in a bar pinned to the bottom of the window (#2891),
   spanning the content area — everything right of the app sidebar — so they stay
   reachable however far the page is scrolled instead of floating directly under
   the tiles. `left` is fed by the app shell: --app-content-left is the sidebar's
   current width, 0 on mobile. */
.toolbar {
  position: fixed;
  left: var(--app-content-left, 0);
  right: 0;
  bottom: 0;
  z-index: 5;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 14px;
  padding: 10px 14px;
  background: var(--t-surface);
  border-top: 1px solid var(--t-border);
}
/* Toolbar controls ~1.25× the default (#2886): larger than stock so the mic
   meter inside the button reads, but not the oversized 1.5× first tried. The
   icons have no explicit size, so they follow the button's font-size. */
.toolbar :deep(.n-button) {
  width: 42px;
  height: 42px;
  font-size: 20px;
}
/* Own-mic meter (#2883 round 2): a ghosted outline under a solid copy that fills
   from the bottom as clip-path uncovers it. Sized to the button's icon em. */
.mic-meter {
  position: relative;
  display: inline-flex;
  width: 1em;
  height: 1em;
}
.mic-base {
  position: absolute;
  inset: 0;
  opacity: 0.5;
}
.mic-fill {
  position: absolute;
  inset: 0;
  display: inline-flex;
  transition: clip-path 60ms linear;
}
</style>
