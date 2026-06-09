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
    <div class="auth-inner">
      <img class="auth-mark" src="/mark-white.svg" alt="Tessera" />
      <p v-if="subtitle" class="auth-sub">{{ subtitle }}</p>
      <slot />
    </div>
  </div>
</template>

<style scoped>
.auth {
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
.auth-inner {
  width: 100%;
  max-width: 360px;
  display: flex;
  flex-direction: column;
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
