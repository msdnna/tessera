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
import { NIcon, NInput, NPopover, NSpin, NTooltip } from 'naive-ui'
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
  PeopleOutline,
  ChatbubbleOutline,
  HappyOutline,
  GridOutline,
  PersonOutline,
  ContractOutline as MinimizeOutline,
  ChevronForwardOutline,
} from '@vicons/ionicons5'
import { conferences as confApi } from '@/api'
import { mediaSupported, CONNECTING, ERROR, UNAVAILABLE } from '@/composables/useConfTransport'
import { useConferenceSession } from '@/stores/conference'
import ParticipantTile from './ParticipantTile.vue'
import ParticipantsPanel from './ParticipantsPanel.vue'
import ConferenceChat from './ConferenceChat.vue'
import DeviceMenu from './DeviceMenu.vue'
import UserAvatar from '@/components/UserAvatar.vue'
import ShortcutKeys from '@/components/ShortcutKeys.vue'
import { useShortcuts } from '@/composables/useShortcuts'
import { stubTask } from '@/utils/featureStubs'
import '@/components/kit/kit.css'

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
// `minimize` asks the parent to fold the call into the mini-window (v2, #3107):
// leaving the room's route is what does it — the session lives in the store.
const emit = defineEmits(['hangup', 'invite', 'minimize'])

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
    recError.value = e.message
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

// ── v2 layout (#3107 — PackageE «Звонок») ───────────────────────────────
// Grid: every participant an equal tile — the mockup's default. Speaker: the
// stage plus the strip, as before. A shared screen always takes the stage.
// Remembered per browser (non-critical UX state → localStorage).
const LAYOUT_KEY = 'tessera_conf_layout'
const layout = ref(
  (typeof localStorage !== 'undefined' ? localStorage.getItem(LAYOUT_KEY) : null) === 'speaker'
    ? 'speaker'
    : 'grid',
)
function setLayout(v) {
  layout.value = v
  try {
    localStorage.setItem(LAYOUT_KEY, v)
  } catch {
    // storage disabled — the choice still holds for this visit
  }
}
const gridMode = computed(() => layout.value === 'grid' && !screenPeer.value)
const gridStyle = computed(() => {
  const n = peers.value.length || 1
  const cols = n <= 1 ? 1 : n <= 4 ? 2 : n <= 9 ? 3 : 4
  return { gridTemplateColumns: `repeat(${cols}, minmax(0, 1fr))` }
})

// Raised hands come from the room socket, keyed by the user id the SFU uses as
// the participant's identity.
const handUps = computed(() => {
  const out = new Set()
  for (const p of room.participants.value) if (p.hand_at) out.add(p.user_id)
  return out
})

// The recording's own clock for the bar («Запись · 24:12»).
const now = ref(Date.now())
let clock = null
watch(
  () => !!room.recording.value,
  (on) => {
    clearInterval(clock)
    clock = on ? setInterval(() => (now.value = Date.now()), 1000) : null
  },
  { immediate: true },
)
onBeforeUnmount(() => clearInterval(clock))
const recClock = computed(() => {
  const at = Date.parse(room.recording.value?.started_at || '')
  if (!at) return ''
  const sec = Math.max(0, Math.floor((now.value - at) / 1000))
  const h = Math.floor(sec / 3600)
  const m = Math.floor((sec % 3600) / 60)
  const ss = String(sec % 60).padStart(2, '0')
  return h ? `${h}:${String(m).padStart(2, '0')}:${ss}` : `${m}:${ss}`
})

// The toolbar's «Участники» and «Чат» open the rail on that tab, and close it
// when it is already showing it.
function railTab(tab) {
  if (railOpen.value && rail.value === tab && !roomNarrow.value) railOpen.value = false
  else {
    railOpen.value = true
    rail.value = tab
  }
}

// Reactions in the call are #3054 — a visible, inactive button until then.
const reactionsTask = stubTask('meet.reactions')

// ⌘D microphone, ⌘E camera, ⌘⇧H leave — while this call is on screen.
useShortcuts(
  {
    'meet.mic': () => connected.value && !room.forceMuted.value && toggleMic(),
    'meet.cam': () => connected.value && toggleCam(),
    'meet.leave': () => emit('hangup'),
  },
  { when: () => props.active && !props.ended },
)

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
        <button type="button" class="kit-btn small" @click="join(conferenceId)">
          {{ $t('conferences.media.retry') }}
        </button>
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

      <!-- The call's bar: who is in it, the layout, folding the rail, the
           mini-window. -->
      <div class="bar">
        <span class="bar-count">
          <n-icon :component="PeopleOutline" :size="14" />
          {{
            $t('conferences.call.count', room.participants.value.length, {
              named: { count: room.participants.value.length },
            })
          }}
        </span>
        <span class="bar-spacer" />
        <div class="seg" role="group" :aria-label="$t('conferences.call.layout')">
          <n-tooltip :delay="400">
            <template #trigger>
              <button
                type="button"
                class="seg-btn"
                :class="{ on: layout === 'grid' }"
                :aria-pressed="layout === 'grid'"
                data-testid="conference-layout-grid"
                @click="setLayout('grid')"
              >
                <n-icon :component="GridOutline" :size="15" />
              </button>
            </template>
            {{ $t('conferences.call.grid') }}
          </n-tooltip>
          <n-tooltip :delay="400">
            <template #trigger>
              <button
                type="button"
                class="seg-btn"
                :class="{ on: layout === 'speaker' }"
                :aria-pressed="layout === 'speaker'"
                data-testid="conference-layout-speaker"
                @click="setLayout('speaker')"
              >
                <n-icon :component="PersonOutline" :size="15" />
              </button>
            </template>
            {{ $t('conferences.call.speaker') }}
          </n-tooltip>
        </div>
        <!-- Fold the rail away like the app sidebar (#2891). Hidden on a narrow
             layout, where the rail stacks under the stage. -->
        <n-tooltip v-if="!roomNarrow" :delay="400">
          <template #trigger>
            <button
              type="button"
              class="icon-btn rail-toggle"
              data-testid="conference-rail-toggle"
              :aria-label="
                railOpen ? $t('conferences.panel.hideRail') : $t('conferences.panel.showRail')
              "
              @click="toggleRail"
            >
              <n-icon
                :component="railOpen ? ChevronForwardOutline : ChatbubbleOutline"
                :size="15"
              />
            </button>
          </template>
          {{ railOpen ? $t('conferences.panel.hideRail') : $t('conferences.panel.showRail') }}
        </n-tooltip>
        <n-tooltip :delay="400">
          <template #trigger>
            <button
              type="button"
              class="icon-btn"
              data-testid="conference-minimize"
              :aria-label="$t('conferences.mini.minimize')"
              @click="emit('minimize')"
            >
              <n-icon :component="MinimizeOutline" :size="15" />
            </button>
          </template>
          <span class="tip">
            {{ $t('conferences.mini.minimize') }} <ShortcutKeys id="meet.minimize" />
          </span>
        </n-tooltip>
      </div>

      <div class="body">
        <n-spin :show="busy" class="stage-col">
          <div ref="stageWrapEl" class="stage-wrap" :class="{ grid: gridMode }">
            <!-- «Идёт запись», for everyone in the call and not only for whoever
                 may stop it (#2877). Overlaid on the stage so it survives
                 fullscreen — where a shared screen is watched. -->
            <div v-if="recording" class="rec-badge" data-testid="conference-recording-dot">
              <span class="rec-dot" />
              <span>{{ $t('conferences.rec.live') }}</span>
              <span v-if="recClock" class="rec-clock">{{ recClock }}</span>
              <span v-if="recording.started_by" class="rec-by">
                {{ $t('conferences.rec.by', { name: recording.started_by }) }}
              </span>
            </div>
            <!-- Fullscreen lives on the stage itself (#2882). v-if on the
                 tooltip, not the button: an empty trigger slot makes naive throw. -->
            <n-tooltip v-if="fullscreenSupported && (stagePeer || gridMode)">
              <template #trigger>
                <button
                  type="button"
                  class="icon-btn fs-btn"
                  data-testid="conference-fullscreen"
                  @click="toggleFullscreen"
                >
                  <n-icon :component="isFullscreen ? ContractOutline : ExpandOutline" :size="15" />
                </button>
              </template>
              {{
                isFullscreen
                  ? $t('conferences.media.exitFullscreen')
                  : $t('conferences.media.fullscreen')
              }}
            </n-tooltip>

            <!-- Grid: everybody an equal tile. -->
            <div v-if="gridMode && peers.length" class="grid-tiles" :style="gridStyle">
              <participant-tile
                v-for="p in peers"
                :key="p.sid || p.id"
                :peer="p"
                :hand="handUps.has(p.id)"
                cell
              />
            </div>

            <template v-else>
              <participant-tile
                v-if="stagePeer"
                :key="screenPeer ? `screen-${stagePeer.id}` : stagePeer.sid || stagePeer.id"
                :peer="stagePeer"
                stage
                :screen="!!screenPeer"
                :hand="!screenPeer && handUps.has(stagePeer.id)"
              />
              <div v-else class="notice idle">{{ $t('conferences.media.connecting') }}</div>
            </template>

            <!-- Who holds the stage and who is behind them. The queue is the
                 server's, not a local guess. -->
            <div
              v-if="room.stage.value || room.queuePos.value"
              class="stage-note"
              data-testid="conference-stage-note"
            >
              <n-icon :component="DesktopOutline" :size="14" />
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
              <button type="button" class="kit-btn small primary" @click="startScreen()">
                {{ $t('conferences.screen.start') }}
              </button>
            </div>

            <div v-if="!gridMode && stripPeers.length" class="strip" data-testid="conference-strip">
              <participant-tile
                v-for="p in stripPeers"
                :key="p.sid || p.id"
                :peer="p"
                :hand="handUps.has(p.id)"
              />
            </div>
          </div>
        </n-spin>

        <aside v-show="railVisible" class="rail">
          <div class="rail-tabs">
            <div class="seg wide" role="tablist">
              <button
                type="button"
                role="tab"
                class="seg-btn"
                :class="{ on: rail === 'people' }"
                :aria-selected="rail === 'people'"
                data-testid="conference-rail-people"
                @click="rail = 'people'"
              >
                {{ $t('conferences.panel.title', { count: room.participants.value.length }) }}
              </button>
              <button
                type="button"
                role="tab"
                class="seg-btn"
                :class="{ on: rail === 'chat' }"
                :aria-selected="rail === 'chat'"
                data-testid="conference-rail-chat"
                @click="rail = 'chat'"
              >
                {{ $t('conferences.chat.tab') }}
                <span v-if="rail !== 'chat' && unread > 0" class="seg-n">{{
                  unread > 9 ? '9+' : unread
                }}</span>
              </button>
            </div>
            <!-- Invite lives here (#2881), a click-popover (#2891), like the
                 assignee picker. -->
            <n-popover
              v-if="canInvite"
              trigger="click"
              placement="bottom-end"
              class="invite-pop"
              @update:show="onInviteShow"
            >
              <template #trigger>
                <button
                  type="button"
                  class="icon-btn invite"
                  :aria-label="$t('conferences.invite.button')"
                  :title="$t('conferences.invite.button')"
                  data-testid="conference-invite"
                >
                  <n-icon :component="PersonAddOutline" :size="15" />
                </button>
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
        <button type="button" class="kit-btn small primary" @click="unblockAudio()">
          {{ $t('conferences.media.enableAudio') }}
        </button>
      </div>

      <!-- The controls: 46px buttons, radius 15, on a floating dock. -->
      <div class="toolbar">
        <n-tooltip>
          <template #trigger>
            <!-- Disabled under a force-mute rather than hidden: the server will
                 refuse the publish anyway, and a button that silently does
                 nothing reads as a broken microphone. -->
            <button
              type="button"
              class="cb"
              :class="{ off: !micOn }"
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
            </button>
          </template>
          <span class="tip">
            {{
              room.forceMuted.value
                ? $t('conferences.panel.youAreForceMuted')
                : micOn
                  ? $t('conferences.media.muteMic')
                  : $t('conferences.media.unmuteMic')
            }}
            <ShortcutKeys id="meet.mic" />
          </span>
        </n-tooltip>

        <n-tooltip>
          <template #trigger>
            <button
              type="button"
              class="cb"
              :class="{ off: !camOn }"
              :disabled="!connected"
              data-testid="conference-cam"
              @click="toggleCam()"
            >
              <n-icon :component="camOn ? VideocamOutline : VideocamOffOutline" />
            </button>
          </template>
          <span class="tip">
            {{ camOn ? $t('conferences.media.camOff') : $t('conferences.media.camOn') }}
            <ShortcutKeys id="meet.cam" />
          </span>
        </n-tooltip>

        <n-tooltip>
          <template #trigger>
            <!-- Enabled even while someone else presents: pressing it is how you
                 get into the queue. -->
            <button
              type="button"
              class="cb"
              :class="{ on: screenOn, wait: wantScreen && !screenOn }"
              :disabled="!connected"
              data-testid="conference-screen"
              @click="toggleScreen()"
            >
              <n-icon :component="screenOn ? StopCircleOutline : DesktopOutline" />
            </button>
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
             A soft red while it runs. -->
        <n-tooltip v-if="room.canModerate.value">
          <template #trigger>
            <button
              type="button"
              class="cb rec"
              :class="{ live: !!recording }"
              :disabled="!room.connected.value || recBusy"
              data-testid="conference-record"
              @click="toggleRecording()"
            >
              <n-icon :component="recording ? SquareOutline : RadioButtonOnOutline" />
            </button>
          </template>
          {{ recording ? $t('conferences.rec.stop') : $t('conferences.rec.start') }}
        </n-tooltip>

        <!-- Reactions are #3054: visible, inactive. -->
        <n-tooltip>
          <template #trigger>
            <span class="cb-stub">
              <button type="button" class="cb" disabled data-testid="conference-reactions">
                <n-icon :component="HappyOutline" />
              </button>
            </span>
          </template>
          {{ $t('common.stub.soon', { n: reactionsTask }) }}
        </n-tooltip>

        <n-tooltip>
          <template #trigger>
            <button
              type="button"
              class="cb"
              :class="{ hand: handUp }"
              :disabled="!room.connected.value"
              data-testid="conference-hand"
              @click="room.raiseHand(!handUp)"
            >
              <n-icon :component="HandRightOutline" />
            </button>
          </template>
          {{ handUp ? $t('conferences.panel.lowerHand') : $t('conferences.panel.raiseHand') }}
        </n-tooltip>

        <span class="cb-gap" />

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
            <button
              type="button"
              class="cb"
              :class="{ on: railVisible && rail === 'people' && !roomNarrow }"
              data-testid="conference-people"
              @click="railTab('people')"
            >
              <n-icon :component="PeopleOutline" />
            </button>
          </template>
          {{ $t('conferences.detail.participants') }}
        </n-tooltip>
        <n-tooltip>
          <template #trigger>
            <button
              type="button"
              class="cb"
              :class="{ on: railVisible && rail === 'chat' && !roomNarrow }"
              data-testid="conference-chat-toggle"
              @click="railTab('chat')"
            >
              <n-icon :component="ChatbubbleOutline" />
              <span v-if="rail !== 'chat' && unread > 0" class="cb-dot" />
            </button>
          </template>
          {{ $t('conferences.chat.tab') }}
        </n-tooltip>

        <span class="cb-gap" />

        <button
          type="button"
          class="cb leave"
          data-testid="conference-leave"
          @click="emit('hangup')"
        >
          <n-icon :component="CallOutline" />
          <span class="leave-l">{{ $t('conferences.actions.leave') }}</span>
          <ShortcutKeys id="meet.leave" joined />
        </button>
      </div>
    </template>
  </div>
</template>

<style scoped>
/* v2 (#3107 — PackageE «Звонок»): the call fills the page — a bar, the tiles
   and the rail side by side, the controls on a dock at the bottom. No lines:
   the rail is a --t-col surface, the bar and the dock are spacing and levels. */
.room {
  display: flex;
  flex-direction: column;
  gap: 10px;
  height: 100%;
  min-height: 0;
}
.notice {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 9px 12px;
  border-radius: 12px;
  font-size: 12.5px;
  color: var(--t-text2);
  background: var(--t-col);
}
.notice.idle {
  justify-content: center;
  color: var(--t-text3);
  min-height: 120px;
}
.notice.err {
  color: color-mix(in srgb, #e0533d 82%, var(--t-text1));
  background: color-mix(in srgb, #e0533d 12%, transparent);
}
.notice.turn {
  justify-content: center;
}

/* ── the bar ── */
.bar {
  flex: none;
  display: flex;
  align-items: center;
  gap: 8px;
  min-height: 32px;
}
.bar-count {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12.5px;
  color: var(--t-text2);
}
.bar-spacer {
  flex: 1;
}
/* A segmented control on the sunken level. */
.seg {
  display: inline-flex;
  gap: 2px;
  padding: 3px;
  border-radius: 10px;
  background: var(--t-sub);
}
.seg.wide {
  flex: 1;
  display: grid;
  grid-auto-flow: column;
  grid-auto-columns: 1fr;
}
.seg-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  min-width: 30px;
  height: 26px;
  padding: 0 8px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--t-text2);
  font: inherit;
  font-size: 12.5px;
  white-space: nowrap;
  cursor: pointer;
}
.seg-btn:hover {
  color: var(--t-text1);
}
.seg-btn.on {
  background: var(--t-pop);
  color: var(--t-text1);
  font-weight: 600;
  box-shadow: var(--t-sh-card);
}
.seg-n {
  min-width: 16px;
  height: 16px;
  padding: 0 4px;
  box-sizing: border-box;
  border-radius: 999px;
  background: var(--t-accent-grad);
  color: #fff;
  font-size: 10px;
  font-weight: 700;
  line-height: 16px;
}
.icon-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  border: 0;
  border-radius: 9px;
  background: transparent;
  color: var(--t-text2);
  cursor: pointer;
}
.icon-btn:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.icon-btn:focus-visible,
.seg-btn:focus-visible,
.cb:focus-visible {
  outline: 2px solid var(--t-primary);
  outline-offset: 2px;
}
.tip {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}

/* ── stage and rail ── */
.body {
  flex: 1;
  min-height: 0;
  display: flex;
  align-items: stretch;
  gap: 12px;
}
.stage-col {
  flex: 1;
  min-width: 0;
  min-height: 0;
}
.stage-col :deep(.n-spin-content) {
  height: 100%;
}
.stage-wrap {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: 10px;
  height: 100%;
  min-height: 0;
}
.grid-tiles {
  flex: 1;
  min-height: 0;
  display: grid;
  grid-auto-rows: minmax(0, 1fr);
  gap: 10px;
}
.rail {
  flex: none;
  display: flex;
  flex-direction: column;
  width: 316px;
  min-height: 0;
  overflow-y: auto;
  padding: 10px;
  box-sizing: border-box;
  border-radius: 18px;
  background: var(--t-col);
}
.rail-tabs {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 8px;
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
  border-radius: 7px;
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
/* The chat fills the rail: its log scrolls, its composer stays at the foot. */
.rail :deep(.chat) {
  flex: 1;
  min-height: 0;
}
.rail :deep(.chat .log) {
  max-height: none;
}
/* Invited-but-absent list (#2875), set apart from the live roster by space. */
.invited {
  margin-top: 12px;
}
.invited-h {
  padding: 0 8px 4px;
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.06em;
  text-transform: uppercase;
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
/* Stacked layout (#2893, round 2): `stretch` gives both the pane's width with
   no percentage to overflow past. */
@media (max-width: 900px) {
  .room {
    height: auto;
    padding-bottom: 84px;
  }
  .body {
    flex-direction: column;
    align-items: stretch;
  }
  .rail {
    width: auto;
    max-height: none;
  }
  .rail :deep(.chat .log) {
    max-height: 46vh;
  }
  .grid-tiles {
    grid-auto-rows: minmax(140px, auto);
  }
}
/* Overlaid on the stage's top-right corner (#2882). */
.fs-btn {
  position: absolute;
  top: 10px;
  right: 10px;
  z-index: 2;
  background: rgb(0 0 0 / 40%);
  color: #fff;
}
.fs-btn:hover {
  background: rgb(0 0 0 / 60%);
  color: #fff;
}
.stage-wrap.grid .fs-btn,
.stage-wrap.grid .rec-badge {
  top: 12px;
}
/* In fullscreen the column owns the whole screen. */
.stage-wrap:fullscreen {
  justify-content: center;
  padding: 16px;
  background: var(--t-bg);
}
.stage-wrap:fullscreen :deep(.tile.stage) {
  max-height: 92vh;
}
/* «Идёт запись» over the stage's top-left corner (#2877): a soft red pill on a
   dark scrim — it sits on video, which is dark in both themes. */
.rec-badge {
  position: absolute;
  top: 10px;
  left: 10px;
  z-index: 2;
  display: flex;
  align-items: center;
  gap: 6px;
  height: 26px;
  padding: 0 10px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 600;
  color: #fff;
  background: color-mix(in srgb, #e0533d 55%, rgb(0 0 0 / 60%));
  pointer-events: none;
}
.rec-clock {
  font-variant-numeric: tabular-nums;
}
.rec-by {
  font-weight: 400;
  opacity: 0.8;
}
.rec-dot {
  flex: none;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #fff;
  animation: rec-pulse 1.6s ease-in-out infinite;
}
@media (prefers-reduced-motion: reduce) {
  .rec-dot {
    animation: none;
  }
}
@keyframes rec-pulse {
  50% {
    opacity: 0.3;
  }
}
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
/* Speaker layout on a desktop: the stage takes the height the strip leaves. */
@media (min-width: 901px) {
  .stage-wrap:not(.grid) > :deep(.tile.stage) {
    flex: 1;
    min-height: 0;
    max-height: none;
    aspect-ratio: auto;
  }
}
/* Small fixed-size tiles under the stage, left-aligned (#2881). */
.strip {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(150px, 176px));
  justify-content: start;
  gap: 10px;
}

/* ── the dock ── */
.toolbar {
  flex: none;
  align-self: center;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px;
  border-radius: 20px;
  background: var(--t-pop);
  box-shadow: var(--t-sh-pop);
}
@media (max-width: 900px) {
  /* On a phone the dock stays in reach at the bottom of the screen. */
  .toolbar {
    position: fixed;
    left: 50%;
    bottom: 12px;
    z-index: 5;
    transform: translateX(-50%);
    max-width: calc(100vw - 16px);
    overflow-x: auto;
    gap: 6px;
  }
}
.cb {
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  width: 46px;
  height: 46px;
  padding: 0;
  border: 0;
  border-radius: 15px;
  background: var(--t-ctl);
  color: var(--t-text1);
  font: inherit;
  font-size: 20px;
  cursor: pointer;
  transition:
    background 0.12s ease,
    color 0.12s ease;
}
.cb:hover:not(:disabled) {
  background: var(--t-hover);
}
.cb:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}
/* Off: the device is off — the glyph dimmed, the surface stays neutral. */
.cb.off {
  color: var(--t-text3);
}
/* On: an active toggle (sharing, the open rail tab) on the soft accent. */
.cb.on {
  background: color-mix(in srgb, var(--t-primary) 22%, var(--t-ctl));
  color: var(--t-primary);
}
.cb.wait {
  background: color-mix(in srgb, #f0a020 22%, var(--t-ctl));
  color: #f0a020;
}
.cb.hand {
  background: color-mix(in srgb, #f0a020 26%, var(--t-ctl));
  color: #f0a020;
}
/* Recording: a soft red; a live recording is a stronger one. */
.cb.rec {
  color: #e0533d;
}
.cb.rec.live {
  background: color-mix(in srgb, #e0533d 26%, var(--t-ctl));
}
.cb-stub {
  display: inline-flex;
}
.cb-gap {
  width: 6px;
}
.cb-dot {
  position: absolute;
  top: 9px;
  right: 9px;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--t-primary);
}
/* «Выйти»: the red action with its key. */
.cb.leave {
  width: auto;
  padding: 0 10px 0 14px;
  font-size: 18px;
  color: #fff;
  background: linear-gradient(
    to top right,
    color-mix(in srgb, #e0533d 86%, #000),
    #e0533d 50%,
    color-mix(in srgb, #e0533d 86%, #fff)
  );
}
.cb.leave:hover {
  background: linear-gradient(
    to top right,
    color-mix(in srgb, #e0533d 80%, #000),
    #e0533d 50%,
    color-mix(in srgb, #e0533d 92%, #fff)
  );
  box-shadow: 0 4px 14px color-mix(in srgb, #e0533d 40%, transparent);
}
.leave-l {
  font-size: 14px;
  font-weight: 650;
}
.cb.leave :deep(.kbd) {
  background: rgba(255, 255, 255, 0.22);
  color: #fff;
  box-shadow: none;
}
/* Own-mic meter (#2883 round 2): a ghosted outline under a solid copy that fills
   from the bottom as clip-path uncovers it. */
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
@media (max-width: 900px) {
  .cb {
    width: 42px;
    height: 42px;
    border-radius: 13px;
    flex: none;
  }
  .leave-l,
  .cb.leave :deep(.sk) {
    display: none;
  }
}
</style>
