<script setup>
// The floating, minimised conference (#2888).
//
// When a call is live but the user has navigated away from its room, this is the
// call: a small draggable, resizable window pinned over the app showing the
// speaker (or a shared screen), a hover toolbar and a «развернуть» overlay back
// to the full room. It reads the same shared session as ConferenceRoom — one
// transport, one socket — so minimising and restoring is just a change of which
// view is on screen, never a reconnect.
//
// Mounted once at the app root (App.vue), so it survives every route change; it
// paints nothing until there is a session AND the open route is not that call's
// own room.
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { NButton, NIcon, NTooltip } from 'naive-ui'
import {
  MicOutline,
  MicOffOutline,
  VideocamOutline,
  VideocamOffOutline,
  HandRightOutline,
  CallOutline,
  ScanOutline,
} from '@vicons/ionicons5'
import { useConferenceSession } from '@/stores/conference'
import { useAuthStore } from '@/stores/auth'
import ParticipantTile from './ParticipantTile.vue'

const session = useConferenceSession()
const auth = useAuthStore()
const route = useRoute()
const router = useRouter()

const { micOn, camOn, connected, screenPeer, dominant } = session.transport
const room = session.room

// Shown only while a call is live and we are looking at something other than its
// own room. On the room's own route the full ConferenceRoom is what is on screen.
const show = computed(() => session.active && String(route.params.id || '') !== session.activeId)

// The one tile the window shows: a shared screen if there is one, otherwise the
// dominant speaker — the same rule the full stage uses.
const stagePeer = computed(() => screenPeer.value || dominant.value)
const handUp = computed(() => !!room.self.value?.hand_at)

// Whether the tile is currently a dark video/screen or a flat (light in the light
// theme) avatar. The caption's scrim only makes sense over the former; over the
// avatar a dark band reads as a stray smudge, so it is dropped there (#2888 v2).
const tileVideo = computed(() => {
  const p = stagePeer.value
  if (!p) return null
  return screenPeer.value ? p.screenTrack : p.videoTrack
})
const darkTile = computed(() => !!tileVideo.value)

// ── geometry ─────────────────────────────────────────────────────────────
// The window keeps a fixed 16:9 shape, so a resize is one number: the width, with
// the height following. That is what makes «тянуть за любой бордер» change both
// dimensions together, as asked (#2888).
const ASPECT = 16 / 9
const MIN_W = 220 // small enough to tuck away, wide enough for the whole toolbar
const MARGIN = 12 // keep-clear gap from the viewport edges
const POS_KEY = 'tessera_conf_mini'

const vw = () => (typeof window !== 'undefined' ? window.innerWidth : 1280)
const vh = () => (typeof window !== 'undefined' ? window.innerHeight : 800)
// The largest allowed: neither dimension past half the viewport, so the area
// never exceeds a quarter of the screen (#2888 clarification).
const maxW = () => Math.max(MIN_W, Math.min(vw() / 2, (vh() / 2) * ASPECT))

const width = ref(300)
const pos = ref({ x: 0, y: 0 })

function heightFor(w) {
  return w / ASPECT
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
    width.value = 300
    pos.value = { x: vw() - width.value - MARGIN, y: vh() - heightFor(width.value) - MARGIN }
  }
  clamp()
}

// ── drag ───────────────────────────────────────────────────────────────
// Pointer-events with capture, so a fast drag that outruns the window still
// tracks; buttons and resize handles opt out via `.no-drag` / their own handler.
let dragState = null
function onDragDown(e) {
  if (e.target.closest?.('.no-drag')) return
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
function onDragUp() {
  if (!dragState) return
  dragState = null
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
  else dw = (dir.b ? dy : -dy) * ASPECT // pure top/bottom edges pull vertically
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
    <participant-tile
      v-if="stagePeer"
      :key="screenPeer ? `screen-${stagePeer.id}` : stagePeer.sid || stagePeer.id"
      :peer="stagePeer"
      :screen="!!screenPeer"
      class="mini-tile"
    />
    <div v-else class="mini-empty">{{ $t('conferences.media.connecting') }}</div>

    <!-- Centre overlay: back to the full room. -->
    <n-tooltip>
      <template #trigger>
        <button class="mini-expand no-drag" data-testid="conference-mini-expand" @click="expand">
          <n-icon :component="ScanOutline" :size="22" />
        </button>
      </template>
      {{ $t('conferences.mini.expand') }}
    </n-tooltip>

    <!-- The meeting's name, top-left, so a minimised call still says which one.
         Scrim only over a dark video/screen tile — flat over the avatar. -->
    <div class="mini-cap" :class="{ 'on-dark': darkTile }">
      <!-- The recording dot follows the call into the minimised window (#2877).
           Minimising is the state in which a recording is easiest to forget —
           the call is out of sight in a corner — so this is the last place the
           indicator may be dropped. Labelled, not a bare dot, and it takes the
           width it needs: the meeting's name is what gets ellipsised at 220px,
           because a name half-read still identifies the call while a red dot
           with no word next to it identifies nothing. -->
      <span v-if="room.recording.value" class="mini-rec" data-testid="conference-mini-recording">
        <span class="mini-rec-dot" />
        {{ $t('conferences.rec.short') }}
      </span>
      <span class="mini-cap-text">{{ title }}</span>
    </div>

    <!-- Hover toolbar along the bottom: mic, camera, hand, leave. -->
    <div class="mini-bar no-drag">
      <n-tooltip>
        <template #trigger>
          <n-button
            circle
            size="tiny"
            :type="micOn ? 'primary' : 'default'"
            :secondary="!micOn"
            :disabled="!connected || room.forceMuted.value"
            data-testid="conference-mini-mic"
            @click="session.transport.toggleMic()"
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
            size="tiny"
            :type="camOn ? 'primary' : 'default'"
            :secondary="!camOn"
            :disabled="!connected"
            data-testid="conference-mini-cam"
            @click="session.transport.toggleCam()"
          >
            <n-icon :component="camOn ? VideocamOutline : VideocamOffOutline" />
          </n-button>
        </template>
        {{ camOn ? $t('conferences.media.camOff') : $t('conferences.media.camOn') }}
      </n-tooltip>

      <n-tooltip>
        <template #trigger>
          <n-button
            circle
            size="tiny"
            :type="handUp ? 'warning' : 'default'"
            :secondary="!handUp"
            :disabled="!room.connected.value"
            data-testid="conference-mini-hand"
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
            size="tiny"
            type="error"
            data-testid="conference-mini-hangup"
            @click="hangup"
          >
            <n-icon :component="CallOutline" />
          </n-button>
        </template>
        {{ $t('conferences.media.hangup') }}
      </n-tooltip>
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
  border-radius: 12px;
  overflow: hidden;
  background: #000;
  border: 1px solid var(--t-border);
  box-shadow: var(--t-shadow-2, 0 8px 30px rgb(0 0 0 / 35%));
  cursor: grab;
  touch-action: none;
  user-select: none;
}
.mini:active {
  cursor: grabbing;
}
.mini-tile {
  width: 100%;
  height: 100%;
}
/* The tile brings its own rounding and caption; inside the frame it should fill
   edge to edge, and its own name bar is hidden in favour of the frame caption. */
.mini-tile :deep(.tile) {
  border: none;
  border-radius: 0;
  aspect-ratio: auto;
  min-height: 0;
  width: 100%;
  height: 100%;
}
.mini-tile :deep(.bar) {
  display: none;
}
.mini-empty {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 100%;
  height: 100%;
  font-size: 12px;
  color: var(--t-text3);
  background: var(--t-hover);
}
/* Flat over the avatar tile (neutrals stay flat per the design language); the
   scrim is added back only over a dark video/screen, where white text needs it. */
.mini-cap {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 4px 10px;
  font-size: 12px;
  color: var(--t-text2);
  overflow: hidden;
  white-space: nowrap;
  pointer-events: none;
}
/* The title, and only the title, is what gives way when the window is narrow. */
.mini-cap-text {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
}
.mini-cap.on-dark {
  color: #fff;
  background: linear-gradient(to bottom, rgb(0 0 0 / 42%), transparent);
}
/* Recording label (#2877). Its own red, not the caption's colour: over a light
   avatar tile the caption is grey, and a grey "запись" is not a warning. */
.mini-rec {
  flex: none;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: #d03050;
}
.mini-cap.on-dark .mini-rec {
  color: #ff7a90;
}
.mini-rec-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: currentcolor;
  animation: mini-rec-pulse 1.6s ease-in-out infinite;
}
@media (prefers-reduced-motion: reduce) {
  .mini-rec-dot {
    animation: none;
  }
}
@keyframes mini-rec-pulse {
  50% {
    opacity: 0.25;
  }
}
/* Expand-to-full lives dead centre, revealed on hover so it does not sit over
   the speaker the whole time. */
.mini-expand {
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  display: flex;
  align-items: center;
  justify-content: center;
  width: 44px;
  height: 44px;
  border: none;
  border-radius: 50%;
  color: #fff;
  background: rgb(0 0 0 / 42%);
  cursor: pointer;
  opacity: 0;
  transition:
    opacity 0.12s ease,
    background 0.12s ease;
}
.mini:hover .mini-expand {
  opacity: 1;
}
/* Neutral on hover, not accent: the cursor already says it is clickable, and an
   accent fill merged into the accent-gradient avatar behind it (#2888 v2). */
.mini-expand:hover {
  background: rgb(0 0 0 / 62%);
}
/* A soft theme surface instead of a hard dark gradient, so the buttons sit on a
   consistent backdrop that reads over both a light avatar and a dark screen, and
   the inactive (secondary) buttons no longer melt into the tile (#2888 v2). */
.mini-bar {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  padding: 6px;
  background: color-mix(in srgb, var(--t-surface) 80%, transparent);
  backdrop-filter: blur(8px);
  border-top: 1px solid var(--t-border);
  opacity: 0;
  transition: opacity 0.12s ease;
}
.mini:hover .mini-bar {
  opacity: 1;
}
/* Resize handles: a thin band along each edge and a small square at each corner.
   Invisible but grabbable, cursor hints the axis. */
.rz {
  position: absolute;
  z-index: 1;
}
.rz.n,
.rz.s {
  left: 8px;
  right: 8px;
  height: 8px;
  cursor: ns-resize;
}
.rz.e,
.rz.w {
  top: 8px;
  bottom: 8px;
  width: 8px;
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
  width: 14px;
  height: 14px;
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
