<script setup>
// Branded auth screen (login / register / recover): a themed light/dark
// background with a header (brand monogram + screen title) above a centred card
// holding the slotted form. Behind the card sits a soft, blurred brand-purple
// glow that drifts toward the pointer as it moves anywhere on the page — only
// the transform animates (the blurred bitmap is rasterised once), so it's cheap.
// A theme toggle sits in the top-right corner.
// Input / button / link styling for the slotted form lives in main.css under
// `.auth` (scoped styles can't reach slotted nodes).
import { ref, onMounted, onBeforeUnmount } from 'vue'
import { NIcon } from 'naive-ui'
import { SunnyOutline, MoonOutline } from '@vicons/ionicons5'
import { useThemeStore } from '@/stores/theme'

defineProps({
  title: { type: String, default: '' },
})

const theme = useThemeStore()

const card = ref(null)
const glowX = ref(0) // current (eased) glow offset from card centre, px
const glowY = ref(0)
let targetX = 0
let targetY = 0
let raf = 0

const REST_X = 72 // resting offset (before any pointer move) — top-right peek
const REST_Y = -58
const MAX = 120 // furthest the glow centre may lean from the card centre
const REACH = 3 // pointer distance ÷ REACH drives the lean (smaller = stronger)

function onMove(e) {
  const el = card.value
  if (!el) return
  // Lean the glow toward the pointer from wherever it is on the page, clamped so
  // it never strays further than MAX from the card centre — the glow appears to
  // track the cursor from behind the form.
  const r = el.getBoundingClientRect()
  const cx = r.left + r.width / 2
  const cy = r.top + r.height / 2
  const ox = e.clientX - cx
  const oy = e.clientY - cy
  const len = Math.hypot(ox, oy) || 1
  const mag = Math.min(len / REACH, MAX)
  targetX = (ox / len) * mag
  targetY = (oy / len) * mag
}

function tick() {
  glowX.value += (targetX - glowX.value) * 0.1
  glowY.value += (targetY - glowY.value) * 0.1
  raf = requestAnimationFrame(tick)
}

onMounted(() => {
  targetX = REST_X
  targetY = REST_Y
  glowX.value = REST_X
  glowY.value = REST_Y
  // Respect reduced-motion: leave the glow parked at its resting spot.
  if (window.matchMedia?.('(prefers-reduced-motion: reduce)').matches) return
  window.addEventListener('pointermove', onMove, { passive: true })
  raf = requestAnimationFrame(tick)
})
onBeforeUnmount(() => {
  window.removeEventListener('pointermove', onMove)
  cancelAnimationFrame(raf)
})
</script>

<template>
  <div class="auth">
    <button
      class="auth-theme-toggle"
      type="button"
      :title="theme.isDark ? 'Светлая тема' : 'Тёмная тема'"
      :aria-label="theme.isDark ? 'Светлая тема' : 'Тёмная тема'"
      @click="theme.toggle()"
    >
      <n-icon :component="theme.isDark ? SunnyOutline : MoonOutline" :size="20" />
    </button>

    <div class="auth-stage">
      <div class="auth-header">
        <span class="auth-logo" aria-hidden="true" />
        <span v-if="title" class="auth-title">{{ title }}</span>
      </div>

      <div class="auth-card-wrap">
        <!-- Glow lives on the light theme only — on dark its soft gradient showed
             unavoidable colour banding, so it's hidden there (see styles). -->
        <div
          class="auth-glow"
          aria-hidden="true"
          :style="{ transform: `translate3d(calc(-50% + ${glowX}px), calc(-50% + ${glowY}px), 0)` }"
        />
        <div ref="card" class="auth-card">
          <slot />
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.auth {
  position: relative;
  overflow: hidden;
  min-height: 100vh;
  min-height: 100dvh;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  box-sizing: border-box;
  background: var(--t-bg);
}

/* Theme toggle, top-right corner. */
.auth-theme-toggle {
  position: absolute;
  top: 18px;
  right: 18px;
  z-index: 2;
  width: 40px;
  height: 40px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  border: 1px solid var(--t-border);
  background: var(--t-surface);
  color: var(--t-text2);
  cursor: pointer;
  transition:
    background 0.15s ease,
    color 0.15s ease,
    border-color 0.15s ease;
}
.auth-theme-toggle:hover {
  color: var(--t-primary);
  border-color: var(--t-primary);
}

.auth-stage {
  position: relative;
  width: 100%;
  max-width: 400px;
  animation: auth-rise 0.5s cubic-bezier(0.22, 1, 0.36, 1) both;
}
@media (prefers-reduced-motion: reduce) {
  .auth-stage {
    animation: none;
  }
}
@keyframes auth-rise {
  from {
    opacity: 0;
    transform: translateY(12px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

/* Header above the card: monogram (no badge) on the left, screen title beside
   it — mirrors the reference. The monogram is the white mark used as a CSS mask,
   tinted brand-purple in light mode and white in dark mode. */
.auth-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin: 0 4px 18px;
}
.auth-logo {
  width: 40px;
  height: 34px;
  flex: none;
  background-color: #6d5fe0;
  -webkit-mask: url(/mark-white.svg) center / contain no-repeat;
  mask: url(/mark-white.svg) center / contain no-repeat;
  /* The monogram sits in the upper part of its viewBox, so box-centring leaves it
     reading higher than the title — nudge it down onto the title's axis. */
  transform: translateY(3px);
}
[data-theme='dark'] .auth-logo {
  background-color: #ffffff;
}
.auth-title {
  font-size: 22px;
  font-weight: 700;
  line-height: 1;
  letter-spacing: -0.01em;
  color: var(--t-text1);
}
/* On light theme the title shares the logo's brand purple. */
[data-theme='light'] .auth-title {
  color: #6d5fe0;
}

.auth-card-wrap {
  position: relative;
}

/* Soft brand-purple glow behind the card. Sized larger than the card so its
   blurred edge bleeds out all around; offsetting it concentrates the bleed on
   one side. Only `transform` changes per frame (compositor-only) — the blur is
   rasterised once, so this is cheap. */
.auth-glow {
  position: absolute;
  top: 50%;
  left: 50%;
  width: 142%;
  aspect-ratio: 1;
  border-radius: 50%;
  z-index: 0;
  pointer-events: none;
  filter: blur(62px);
  background: radial-gradient(
    circle at 50% 50%,
    rgba(124, 108, 255, 0.38) 0%,
    rgba(109, 95, 224, 0.2) 38%,
    rgba(145, 131, 255, 0) 70%
  );
  will-change: transform;
}
/* On dark the soft purple glow banded visibly (8-bit steps the eye catches on
   dark backgrounds); rather than fight it with dithering, drop it entirely —
   the card carries the brand on its own there. */
[data-theme='dark'] .auth-glow {
  display: none;
}

.auth-card {
  position: relative;
  z-index: 1;
  display: flex;
  flex-direction: column;
  background: var(--t-surface);
  border: 1px solid var(--t-border);
  border-radius: 18px;
  padding: 30px;
  box-shadow: 0 18px 50px -14px rgba(24, 11, 70, 0.3);
}
/* The default --t-border is barely visible on the white card over a light
   background — give the light-theme card a slightly firmer edge, tinted toward
   the brand purple so the rim quietly echoes the glow behind it. */
[data-theme='light'] .auth-card {
  border-color: color-mix(in srgb, #6d5fe0 28%, #d5d5d5);
}
[data-theme='dark'] .auth-card {
  box-shadow: 0 18px 54px -14px rgba(0, 0, 0, 0.6);
}
</style>
