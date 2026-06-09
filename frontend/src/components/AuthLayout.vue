<script setup>
// Branded shell for the auth screens (login / register): a full-bleed brand
// gradient with the white monogram + wordmark, and a themed surface card that
// holds the form (passed via the default slot). Mirrors the Android AuthScreen.
defineProps({
  title: { type: String, default: '' },
})
</script>

<template>
  <div class="auth">
    <div class="auth-brand">
      <img class="auth-mark" src="/mark-white.svg" alt="" />
      <span class="auth-word">Tessera</span>
    </div>
    <div class="auth-card">
      <h1 v-if="title" class="auth-title">{{ title }}</h1>
      <slot />
    </div>
  </div>
</template>

<style scoped>
.auth {
  min-height: 100vh;
  min-height: 100dvh;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 28px;
  padding: 24px;
  box-sizing: border-box;
  /* Fixed brand gradient (same-hue diagonal, brand purple) — stays on-brand
     regardless of the user's chosen UI accent. */
  background: linear-gradient(to top right, #6d5fe0 0%, #7c6cff 50%, #9183ff 100%);
}
.auth-brand {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 10px;
}
.auth-mark {
  width: 84px;
  height: 70px;
  filter: drop-shadow(0 6px 18px rgba(24, 11, 70, 0.35));
}
.auth-word {
  font-size: 24px;
  font-weight: 700;
  letter-spacing: 0.4px;
  color: #fff;
}
.auth-card {
  width: 100%;
  max-width: 380px;
  background: var(--t-surface);
  border: 1px solid var(--t-border);
  border-radius: 14px;
  padding: 24px;
  box-shadow: 0 18px 50px rgba(24, 11, 70, 0.28);
}
.auth-title {
  margin: 0 0 18px;
  font-size: 18px;
  font-weight: 600;
  color: var(--t-text1);
  text-align: center;
}
</style>
