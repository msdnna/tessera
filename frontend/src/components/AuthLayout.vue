<script setup>
// Branded auth screen (login / register): a full-bleed brand gradient with the
// white "mt" monogram and a subtitle, and the form sitting directly on the
// gradient (no card) — mirrors the Android AuthScreen. The form itself is passed
// via the default slot; input / button / link styling for the slotted content
// lives in main.css under `.auth` (scoped styles can't reach slotted nodes).
defineProps({
  subtitle: { type: String, default: '' },
})
</script>

<template>
  <div class="auth">
    <!-- Slow-drifting aurora blobs over the base brand gradient — the "airy"
         animated background. Purely decorative; sits behind .auth-inner. -->
    <div class="auth-aurora" aria-hidden="true">
      <span class="blob b1" />
      <span class="blob b2" />
      <span class="blob b3" />
    </div>
    <div class="auth-inner">
      <img class="auth-mark" src="/mark-white.svg" alt="Tessera" />
      <p v-if="subtitle" class="auth-sub">{{ subtitle }}</p>
      <slot />
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
  /* Fixed brand gradient (same-hue diagonal, brand purple) — stays on-brand
     regardless of the user's chosen UI accent. */
  background: linear-gradient(to top right, #6d5fe0 0%, #7c6cff 50%, #9183ff 100%);
}

/* ── Airy drifting aurora ────────────────────────────────────────────────────
   Three large, soft, light-lavender blobs that slowly drift and breathe over the
   base gradient. They use plain source-over (NOT mix-blend-mode: screen) and a
   smaller blur: `screen` + a 60px blur forced the GPU to re-blur AND re-blend the
   full-viewport layer against the backdrop every frame, which pegged the CPU/GPU
   (fans spinning up). With normal compositing the blurred bitmap is rasterised
   once and only cheaply re-transformed per frame. The blobs are light tints that
   fade to transparent, so source-over still only lightens the purple. */
.auth-aurora {
  position: absolute;
  inset: 0;
  overflow: hidden;
  pointer-events: none;
  z-index: 0;
}
.auth-aurora .blob {
  position: absolute;
  border-radius: 50%;
  filter: blur(44px);
  opacity: 0.5;
  will-change: transform;
}
.auth-aurora .b1 {
  width: 60vmax;
  height: 60vmax;
  top: -22vmax;
  left: -16vmax;
  background: radial-gradient(circle at 50% 50%, #b3a6ff 0%, rgba(179, 166, 255, 0) 68%);
  animation: auth-drift-1 26s ease-in-out infinite alternate;
}
.auth-aurora .b2 {
  width: 52vmax;
  height: 52vmax;
  bottom: -20vmax;
  right: -14vmax;
  background: radial-gradient(circle at 50% 50%, #9787ff 0%, rgba(151, 135, 255, 0) 66%);
  animation: auth-drift-2 32s ease-in-out infinite alternate;
}
.auth-aurora .b3 {
  width: 44vmax;
  height: 44vmax;
  top: 28%;
  left: 36%;
  background: radial-gradient(circle at 50% 50%, #cfc6ff 0%, rgba(207, 198, 255, 0) 64%);
  animation: auth-drift-3 38s ease-in-out infinite alternate;
}
@keyframes auth-drift-1 {
  from {
    transform: translate3d(0, 0, 0) scale(1);
  }
  to {
    transform: translate3d(10vmax, 6vmax, 0) scale(1.18);
  }
}
@keyframes auth-drift-2 {
  from {
    transform: translate3d(0, 0, 0) scale(1.1);
  }
  to {
    transform: translate3d(-9vmax, -7vmax, 0) scale(0.92);
  }
}
@keyframes auth-drift-3 {
  from {
    transform: translate3d(0, 0, 0) scale(0.95);
  }
  to {
    transform: translate3d(-7vmax, 9vmax, 0) scale(1.22);
  }
}
@media (prefers-reduced-motion: reduce) {
  .auth-aurora .blob {
    animation: none;
  }
}

.auth-inner {
  position: relative;
  z-index: 1;
  width: 100%;
  max-width: 360px;
  display: flex;
  flex-direction: column;
  animation: auth-rise 0.5s cubic-bezier(0.22, 1, 0.36, 1) both;
}
@media (prefers-reduced-motion: reduce) {
  .auth-inner {
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
.auth-mark {
  width: 76px;
  height: 64px;
  align-self: center;
  filter: drop-shadow(0 6px 18px rgba(24, 11, 70, 0.35));
}
.auth-sub {
  margin: 14px 0 26px;
  text-align: center;
  font-size: 15px;
  color: rgba(255, 255, 255, 0.85);
}
</style>
