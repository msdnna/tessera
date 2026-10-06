<script setup>
// «Аврора» over the start screen (v2, #2960): seven soft spots of the accent
// palette drifting at the top of the page, the whole layer leaning after the
// pointer. The one place a gradient mixes hues — and only as a backdrop.
//
// Cheap on purpose (see the animated-blur note in the project memory): the spots
// are radial gradients that are soft by themselves, so there is no `filter:
// blur` and no blend mode to recomposite every frame — only transforms move.
// Reduced motion keeps the layer still; a hidden tab pauses it.
import { ref, onMounted, onBeforeUnmount } from 'vue'

const SPOTS = [
  { c: '#7c5cff', l: '-4%', t: -230, s: 560, a: 'h-aur1 19s' },
  { c: '#2f80ed', l: '14%', t: -260, s: 480, a: 'h-aur2 23s' },
  { c: '#0eb0a9', l: '30%', t: -240, s: 500, a: 'h-aur3 21s' },
  { c: '#18a058', l: '46%', t: -280, s: 420, a: 'h-aur1 27s' },
  { c: '#f0a020', l: '58%', t: -250, s: 460, a: 'h-aur2 18s' },
  { c: '#e0533d', l: '70%', t: -270, s: 420, a: 'h-aur3 25s' },
  { c: '#eb2f96', l: '80%', t: -230, s: 500, a: 'h-aur1 22s' },
]

const layer = ref(null)
const paused = ref(false)
let raf = 0
let mx = 0
let my = 0
const still =
  typeof window !== 'undefined' && window.matchMedia?.('(prefers-reduced-motion: reduce)')?.matches

function apply() {
  raf = 0
  layer.value?.style.setProperty('--mx', String(mx))
  layer.value?.style.setProperty('--my', String(my))
}
function onMove(e) {
  mx = Math.max(-1, Math.min(1, (e.clientX / window.innerWidth) * 2 - 1))
  my = Math.max(-1, Math.min(1, (e.clientY / window.innerHeight) * 2 - 1))
  if (!raf) raf = requestAnimationFrame(apply)
}
function onLeave() {
  mx = 0
  my = 0
  if (!raf) raf = requestAnimationFrame(apply)
}
function onVisibility() {
  paused.value = document.hidden
}

onMounted(() => {
  if (still) return
  window.addEventListener('pointermove', onMove, { passive: true })
  document.documentElement.addEventListener('pointerleave', onLeave)
  document.addEventListener('visibilitychange', onVisibility)
})
onBeforeUnmount(() => {
  cancelAnimationFrame(raf)
  window.removeEventListener('pointermove', onMove)
  document.documentElement.removeEventListener('pointerleave', onLeave)
  document.removeEventListener('visibilitychange', onVisibility)
})
</script>

<template>
  <div class="h-aurora" :class="{ still, paused }" aria-hidden="true">
    <div ref="layer" class="h-aurora-layer">
      <span
        v-for="(s, i) in SPOTS"
        :key="i"
        class="h-spot"
        :style="{
          left: s.l,
          top: `${s.t}px`,
          width: `${s.s}px`,
          height: `${s.s}px`,
          animation: `${s.a} ease-in-out infinite alternate`,
          background: `radial-gradient(circle, ${s.c} 0, color-mix(in srgb, ${s.c} 40%, transparent) 34%, transparent 66%)`,
        }"
      />
    </div>
  </div>
</template>

<style scoped>
.h-aurora {
  position: absolute;
  left: 0;
  right: 0;
  top: 0;
  height: calc(360px + 52px);
  overflow: hidden;
  pointer-events: none;
  -webkit-mask-image: linear-gradient(to bottom, #000 0, #000 30%, transparent);
  mask-image: linear-gradient(to bottom, #000 0, #000 30%, transparent);
  opacity: var(--h-aurora, 0.5);
}
:global([data-theme='light']) .h-aurora {
  --h-aurora: 0.34;
}
.h-aurora-layer {
  position: absolute;
  inset: 0;
  transform: translate3d(calc(var(--mx, 0) * 46px), calc(var(--my, 0) * 22px), 0);
  transition: transform 0.9s cubic-bezier(0.2, 0.8, 0.2, 1);
}
.h-spot {
  position: absolute;
  border-radius: 50%;
  will-change: transform;
}
.paused .h-spot {
  animation-play-state: paused !important;
}
.still .h-spot {
  animation: none !important;
}
.still .h-aurora-layer {
  transition: none;
}
@keyframes h-aur1 {
  0% {
    transform: translate(0, 0) scale(1);
  }
  50% {
    transform: translate(140px, 40px) scale(1.18);
  }
  100% {
    transform: translate(-80px, 10px) scale(0.92);
  }
}
@keyframes h-aur2 {
  0% {
    transform: translate(0, 0) scale(1.05);
  }
  50% {
    transform: translate(-120px, 50px) scale(0.9);
  }
  100% {
    transform: translate(90px, -10px) scale(1.15);
  }
}
@keyframes h-aur3 {
  0% {
    transform: translate(0, 0) scale(0.95);
  }
  50% {
    transform: translate(60px, -30px) scale(1.2);
  }
  100% {
    transform: translate(-110px, 30px) scale(1);
  }
}
</style>
