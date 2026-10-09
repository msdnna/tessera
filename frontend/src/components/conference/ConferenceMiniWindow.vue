<script setup>
// The floating, minimised conference (#2888).
//
// When a call is live but the user has navigated away from its room, this is the
// call: a small draggable, resizable window pinned over the app. It reads the
// same shared session as ConferenceRoom — one transport, one socket — so
// minimising and restoring is just a change of which view is on screen, never a
// reconnect.
//
// v2 (#3107 — PackageE «Мини-окно»): 340px on the popover level, radius 16, the
// popover shadow. The head is the handle — the call's name and how long you
// have been in it, and «развернуть»; two tiles (who is speaking, and the next
// person); the controls and «Выйти» at the foot. Released near an edge it
// sticks to it. ⌘⇧M folds the call in and out, ⌘D / ⌘E / ⌘⇧H work from here.
//
// Mounted once at the app root (App.vue), so it survives every route change; it
// paints nothing until there is a session AND the open route is not that call's
// own room.
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { NIcon, NTooltip } from 'naive-ui'
import {
  MicOutline,
  MicOffOutline,
  VideocamOutline,
  VideocamOffOutline,
  HandRightOutline,
  CallOutline,
  ExpandOutline,
} from '@vicons/ionicons5'
import { useConferenceSession } from '@/stores/conference'
import { useAuthStore } from '@/stores/auth'
import { useShortcuts } from '@/composables/useShortcuts'
import { leaveCallView } from '@/utils/confNav'
import ParticipantTile from './ParticipantTile.vue'
import ShortcutKeys from '@/components/ShortcutKeys.vue'

const session = useConferenceSession()
const auth = useAuthStore()
const route = useRoute()
const router = useRouter()
const { t } = useI18n()

const { micOn, camOn, connected, screenPeer, dominant, peers } = session.transport
const room = session.room

// Shown only while a call is live and we are looking at something other than its
// own room. On the room's own route the full ConferenceRoom is what is on screen.
const onRoom = computed(() => String(route.params.id || '') === session.activeId)
const show = computed(() => session.active && !onRoom.value)

// The tiles: a shared screen or the dominant speaker — the same rule the full
// stage uses — and, next to it, the next person in the call.
const stagePeer = computed(() => screenPeer.value || dominant.value)
const secondPeer = computed(() => {
  const first = stagePeer.value
  return peers.value.find((p) => p !== first && !(screenPeer.value && p === first)) || null
})
const restCount = computed(() => {
  const shown = (stagePeer.value ? 1 : 0) + (secondPeer.value ? 1 : 0)
  return Math.max(0, room.participants.value.length - shown)
})
const handUp = computed(() => !!room.self.value?.hand_at)
const handUps = computed(() => {
  const out = new Set()
  for (const p of room.participants.value) if (p.hand_at) out.add(p.user_id)
  return out
})

// How long we have been in this call, for the head («· 24:12»).
const since = ref(Date.now())
const now = ref(Date.now())
let clock = null
watch(
  () => session.activeId,
  (id) => {
    since.value = Date.now()
    clearInterval(clock)
    clock = id ? setInterval(() => (now.value = Date.now()), 1000) : null
  },
  { immediate: true },
)
onBeforeUnmount(() => clearInterval(clock))
const elapsed = computed(() => {
  const sec = Math.max(0, Math.floor((now.value - since.value) / 1000))
  const h = Math.floor(sec / 3600)
  const m = Math.floor((sec % 3600) / 60)
  const ss = String(sec % 60).padStart(2, '0')
  return h ? `${h}:${String(m).padStart(2, '0')}:${ss}` : `${m}:${ss}`
})

// ── geometry ─────────────────────────────────────────────────────────────
// The window keeps its shape, so a resize is one number: the width, with the
// height following. That is what makes «тянуть за любой бордер» change both
// dimensions together, as asked (#2888). v2: a head and a foot of fixed height
// around a row of two tiles that scale with the width.
const HEAD = 40
const FOOT = 54
const TILES = 0.3 // tile row height per px of width
// The foot's controls and the leave button still fit at this width.
const MIN_W = 280
const MARGIN = 12 // keep-clear gap from the viewport edges
const SNAP = 56 // released this close to an edge, the window sticks to it
const POS_KEY = 'tessera_conf_mini'

const vw = () => (typeof window !== 'undefined' ? window.innerWidth : 1280)
const vh = () => (typeof window !== 'undefined' ? window.innerHeight : 800)
// The largest allowed: neither dimension past half the viewport, so the area
// never exceeds a quarter of the screen (#2888 clarification).
const maxW = () => Math.max(MIN_W, Math.min(vw() / 2, 520))

const width = ref(340)
const pos = ref({ x: 0, y: 0 })

function heightFor(w) {
  return HEAD + FOOT + Math.round(w * TILES)
}
const frameStyle = computed(() => ({
  left: `${pos.value.x}px`,
  top: `${pos.value.y}px`,
  width: `${width.value}px`,
  height: `${heightFor(width.value)}px`,
}))

// Force a geometry back inside the viewport and the size bounds. Called after a
// restore, a drag/resize and a viewport resize, so the window can never end up
// off-screen or larger than a quarter of the page.
function clamp() {
  width.value = Math.min(maxW(), Math.max(MIN_W, width.value))
  const h = heightFor(width.value)
  const maxX = Math.max(MARGIN, vw() - width.value - MARGIN)
  const maxY = Math.max(MARGIN, vh() - h - MARGIN)
  pos.value = {
    x: Math.min(maxX, Math.max(MARGIN, pos.value.x)),
    y: Math.min(maxY, Math.max(MARGIN, pos.value.y)),
  }
}

function persist() {
  try {
    localStorage.setItem(
      POS_KEY,
      JSON.stringify({ x: pos.value.x, y: pos.value.y, w: width.value }),
    )
  } catch {
    // Storage disabled — the window still works, it just forgets where it was.
  }
}

function restoreGeometry() {
  let saved
  try {
    saved = JSON.parse(localStorage.getItem(POS_KEY) || 'null')
  } catch {
    saved = null
  }
  if (saved && Number.isFinite(saved.w)) {
    width.value = saved.w
    pos.value = { x: saved.x, y: saved.y }
  } else {
    // First run: park it in the bottom-right corner.
    width.value = 340
    pos.value = { x: vw() - width.value - MARGIN, y: vh() - heightFor(width.value) - MARGIN }
  }
  clamp()
}

// ── drag ───────────────────────────────────────────────────────────────
// Pointer-events with capture, so a fast drag that outruns the window still
// tracks; buttons and resize handles opt out via `.no-drag` / their own handler.
let dragState = null
function onDragDown(e) {
  if (e.target.closest?.('.no-drag') || !e.target.closest?.('.mini-head')) return
  dragState = { px: e.clientX, py: e.clientY, x: pos.value.x, y: pos.value.y }
  e.currentTarget.setPointerCapture?.(e.pointerId)
}
function onDragMove(e) {
  if (!dragState) return
  pos.value = {
    x: dragState.x + (e.clientX - dragState.px),
    y: dragState.y + (e.clientY - dragState.py),
  }
  clamp()
}
// Released near an edge, the window sticks to it — the corners are where a
// call should sit out of the way.
function snap() {
  const h = heightFor(width.value)
  const right = vw() - width.value - MARGIN
  const bottom = vh() - h - MARGIN
  let { x, y } = pos.value
  if (x - MARGIN < SNAP) x = MARGIN
  else if (right - x < SNAP) x = right
  if (y - MARGIN < SNAP) y = MARGIN
  else if (bottom - y < SNAP) y = bottom
  pos.value = { x, y }
}
function onDragUp() {
  if (!dragState) return
  dragState = null
  snap()
  persist()
}

// ── resize ─────────────────────────────────────────────────────────────
// One handler for all eight handles. The proportional grow comes from deriving a
// single width delta from whichever axis the handle pulls on, then keeping the
// opposite edge pinned so the window grows out from where it is grabbed.
let rzState = null
function onResizeDown(e, dir) {
  e.stopPropagation()
  rzState = {
    px: e.clientX,
    py: e.clientY,
    w: width.value,
    right: pos.value.x + width.value,
    bottom: pos.value.y + heightFor(width.value),
    x: pos.value.x,
    y: pos.value.y,
    dir,
  }
  e.currentTarget.setPointerCapture?.(e.pointerId)
}
function onResizeMove(e) {
  if (!rzState) return
  const { dir } = rzState
  const dx = e.clientX - rzState.px
  const dy = e.clientY - rzState.py
  let dw
  if (dir.r) dw = dx
  else if (dir.l) dw = -dx
  else dw = (dir.b ? dy : -dy) / TILES // pure top/bottom edges pull vertically
  width.value = Math.min(maxW(), Math.max(MIN_W, rzState.w + dw))
  const h = heightFor(width.value)
  pos.value = {
    x: dir.l ? rzState.right - width.value : rzState.x,
    y: dir.t ? rzState.bottom - h : rzState.y,
  }
  clamp()
}
function onResizeUp() {
  if (!rzState) return
  rzState = null
  persist()
}

const HANDLES = [
  { pos: 'n', dir: { t: true } },
  { pos: 's', dir: { b: true } },
  { pos: 'e', dir: { r: true } },
  { pos: 'w', dir: { l: true } },
  { pos: 'ne', dir: { t: true, r: true } },
  { pos: 'nw', dir: { t: true, l: true } },
  { pos: 'se', dir: { b: true, r: true } },
  { pos: 'sw', dir: { b: true, l: true } },
]

// ── actions ────────────────────────────────────────────────────────────
function expand() {
  router.push(`/conferences/${session.activeId}`)
}
function hangup() {
  session.hangup()
}

// ⌘⇧M folds the call in from its room and back out from anywhere; the call's
// own keys work while it is folded (the room's shadow these while it is open).
useShortcuts(
  {
    'meet.minimize': () => (onRoom.value ? leaveCallView(router, session.activeId) : expand()),
    'meet.mic': () => connected.value && !room.forceMuted.value && session.transport.toggleMic(),
    'meet.cam': () => connected.value && session.transport.toggleCam(),
    'meet.leave': () => hangup(),
  },
  { when: () => session.active },
)

function onViewportResize() {
  clamp()
}

onMounted(() => {
  restoreGeometry()
  if (typeof window !== 'undefined') window.addEventListener('resize', onViewportResize)
})
onBeforeUnmount(() => {
  if (typeof window !== 'undefined') window.removeEventListener('resize', onViewportResize)
})

// Walk back into a call after a reload once we know who we are (#2888). Immediate
// so a reload straight into an authenticated session restores at once; the watch
// also covers signing in after the app has already mounted.
watch(
  () => auth.isAuthenticated,
  (yes) => {
    if (yes) session.restore()
  },
  { immediate: true },
)
</script>

<template>
  <div
    v-if="show"
    class="mini"
    data-testid="conference-mini"
    :style="frameStyle"
    @pointerdown="onDragDown"
    @pointermove="onDragMove"
    @pointerup="onDragUp"
    @pointercancel="onDragUp"
  >
    <!-- The head is the handle: the call, how long we have been in it, and the
         way back to the full room. The recording follows the call here too
         (#2877) — minimised is when it is easiest to forget. -->
    <div class="mini-head">
      <span class="mini-dot" :class="{ rec: !!room.recording.value }" />
      <span v-if="room.recording.value" class="mini-rec" data-testid="conference-mini-recording">
        {{ $t('conferences.rec.short') }}
      </span>
      <span class="mini-title">{{ session.title }}</span>
      <span class="mini-time">· {{ elapsed }}</span>
      <n-tooltip>
        <template #trigger>
          <button
            type="button"
            class="mini-icon no-drag"
            :aria-label="$t('conferences.mini.expand')"
            data-testid="conference-mini-expand"
            @click="expand"
          >
            <n-icon :component="ExpandOutline" :size="14" />
          </button>
        </template>
        <span class="mini-tip">
          {{ $t('conferences.mini.expand') }} <ShortcutKeys id="meet.minimize" />
        </span>
      </n-tooltip>
    </div>

    <div class="mini-tiles" :class="{ one: !secondPeer }">
      <participant-tile
        v-if="stagePeer"
        :key="screenPeer ? `screen-${stagePeer.id}` : stagePeer.sid || stagePeer.id"
        :peer="stagePeer"
        :screen="!!screenPeer"
        :hand="!screenPeer && handUps.has(stagePeer.id)"
        cell
        class="mini-tile"
      />
      <div v-else class="mini-empty">{{ $t('conferences.media.connecting') }}</div>
      <participant-tile
        v-if="secondPeer"
        :key="secondPeer.sid || secondPeer.id"
        :peer="secondPeer"
        :hand="handUps.has(secondPeer.id)"
        cell
        class="mini-tile"
      />
    </div>

    <div class="mini-foot no-drag">
      <n-tooltip>
        <template #trigger>
          <button
            type="button"
            class="mb"
            :class="{ off: !micOn }"
            :disabled="!connected || room.forceMuted.value"
            data-testid="conference-mini-mic"
            @click="session.transport.toggleMic()"
          >
            <n-icon :component="micOn ? MicOutline : MicOffOutline" />
          </button>
        </template>
        {{ micOn ? $t('conferences.media.muteMic') : $t('conferences.media.unmuteMic') }}
      </n-tooltip>

      <n-tooltip>
        <template #trigger>
          <button
            type="button"
            class="mb"
            :class="{ off: !camOn }"
            :disabled="!connected"
            data-testid="conference-mini-cam"
            @click="session.transport.toggleCam()"
          >
            <n-icon :component="camOn ? VideocamOutline : VideocamOffOutline" />
          </button>
        </template>
        {{ camOn ? $t('conferences.media.camOff') : $t('conferences.media.camOn') }}
      </n-tooltip>

      <n-tooltip>
        <template #trigger>
          <button
            type="button"
            class="mb"
            :class="{ hand: handUp }"
            :disabled="!room.connected.value"
            data-testid="conference-mini-hand"
            @click="room.raiseHand(!handUp)"
          >
            <n-icon :component="HandRightOutline" />
          </button>
        </template>
        {{ handUp ? $t('conferences.panel.lowerHand') : $t('conferences.panel.raiseHand') }}
      </n-tooltip>

      <span class="mini-more">
        <template v-if="restCount">{{
          t('conferences.mini.more', restCount, { named: { count: restCount } })
        }}</template>
      </span>

      <button type="button" class="mb leave" data-testid="conference-mini-hangup" @click="hangup">
        <n-icon :component="CallOutline" />
        <span>{{ $t('conferences.actions.leave') }}</span>
      </button>
    </div>

    <!-- Eight resize handles: four edges pull proportionally, four corners too. -->
    <span
      v-for="h in HANDLES"
      :key="h.pos"
      class="rz no-drag"
      :class="h.pos"
      @pointerdown="onResizeDown($event, h.dir)"
      @pointermove="onResizeMove"
      @pointerup="onResizeUp"
      @pointercancel="onResizeUp"
    />
  </div>
</template>

<style scoped>
.mini {
  position: fixed;
  z-index: 2400;
  display: flex;
  flex-direction: column;
  border-radius: 16px;
  overflow: hidden;
  background: var(--t-pop);
  box-shadow: var(--t-sh-pop);
  touch-action: none;
  user-select: none;
}
/* The head is the handle. */
.mini-head {
  flex: none;
  display: flex;
  align-items: center;
  gap: 6px;
  height: 40px;
  padding: 0 6px 0 14px;
  box-sizing: border-box;
  font-size: 13px;
  cursor: grab;
}
.mini:active .mini-head {
  cursor: grabbing;
}
.mini-dot {
  flex: none;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #18a058;
}
.mini-dot.rec {
  background: #e0533d;
  animation: mini-rec-pulse 1.6s ease-in-out infinite;
}
/* Recording label (#2877): its own red word, not a bare dot. */
.mini-rec {
  flex: none;
  font-size: 12px;
  font-weight: 700;
  color: #e0533d;
}
.mini-title {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-weight: 700;
  color: var(--t-text1);
}
.mini-time {
  flex: none;
  font-variant-numeric: tabular-nums;
  color: var(--t-text3);
  margin-right: auto;
}
.mini-icon {
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: var(--t-text2);
  cursor: pointer;
}
.mini-icon:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.mini-tip {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}
@media (prefers-reduced-motion: reduce) {
  .mini-dot.rec {
    animation: none;
  }
}
@keyframes mini-rec-pulse {
  50% {
    opacity: 0.25;
  }
}
.mini-tiles {
  flex: 1;
  min-height: 0;
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 6px;
  padding: 0 8px;
}
.mini-tiles.one {
  grid-template-columns: 1fr;
}
/* Smaller tile furniture inside the window. */
.mini-tile :deep(.av) {
  width: 40px;
  height: 40px;
  font-size: 15px;
}
.mini-tile {
  border-radius: 12px;
}
.mini-tile :deep(.bar) {
  left: 6px;
  right: 6px;
  bottom: 6px;
}
.mini-tile :deep(.pill) {
  height: 20px;
  font-size: 11px;
}
.mini-tile :deep(.hand) {
  top: 6px;
  right: 6px;
  width: 24px;
  height: 24px;
  border-radius: 7px;
}
.mini-empty {
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 12px;
  font-size: 12px;
  color: var(--t-text3);
  background: var(--t-col);
}
.mini-foot {
  flex: none;
  display: flex;
  align-items: center;
  gap: 6px;
  height: 54px;
  padding: 0 8px;
  box-sizing: border-box;
}
.mb {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  width: 34px;
  height: 34px;
  padding: 0;
  border: 0;
  border-radius: 11px;
  background: var(--t-ctl);
  color: var(--t-text1);
  font: inherit;
  font-size: 16px;
  cursor: pointer;
}
.mb:hover:not(:disabled) {
  background: var(--t-hover);
}
.mb:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}
.mb.off {
  color: var(--t-text3);
}
.mb.hand {
  background: color-mix(in srgb, #f0a020 26%, var(--t-ctl));
  color: #f0a020;
}
.mb:focus-visible,
.mini-icon:focus-visible {
  outline: 2px solid var(--t-primary);
  outline-offset: 1px;
}
.mini-more {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  text-align: center;
  font-size: 12px;
  color: var(--t-text3);
}
.mb.leave {
  width: auto;
  padding: 0 12px;
  color: #fff;
  font-size: 13px;
  font-weight: 650;
  background: linear-gradient(
    to top right,
    color-mix(in srgb, #e0533d 86%, #000),
    #e0533d 50%,
    color-mix(in srgb, #e0533d 86%, #fff)
  );
}
.mb.leave .n-icon {
  font-size: 15px;
}
/* Resize handles: a thin band along each edge and a small square at each corner.
   Invisible but grabbable, cursor hints the axis. */
.rz {
  position: absolute;
  z-index: 1;
}
.rz.n,
.rz.s {
  left: 14px;
  right: 14px;
  height: 6px;
  cursor: ns-resize;
}
.rz.e,
.rz.w {
  top: 14px;
  bottom: 14px;
  width: 6px;
  cursor: ew-resize;
}
.rz.n {
  top: 0;
}
.rz.s {
  bottom: 0;
}
.rz.e {
  right: 0;
}
.rz.w {
  left: 0;
}
.rz.ne,
.rz.nw,
.rz.se,
.rz.sw {
  width: 12px;
  height: 12px;
}
.rz.ne {
  top: 0;
  right: 0;
  cursor: nesw-resize;
}
.rz.nw {
  top: 0;
  left: 0;
  cursor: nwse-resize;
}
.rz.se {
  bottom: 0;
  right: 0;
  cursor: nwse-resize;
}
.rz.sw {
  bottom: 0;
  left: 0;
  cursor: nesw-resize;
}
</style>
