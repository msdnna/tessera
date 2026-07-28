<script setup>
// Brand lockup: the tessera "t" mark and/or the "tessera" wordmark, painted with
// the accent gradient via CSS masks (theme-reactive). Toggle each part: the
// expanded sidebar / login show the wordmark alone; the collapsed rail shows the
// mark alone. `height` is the box height in px.
defineProps({
  height: { type: Number, default: 22 },
  mark: { type: Boolean, default: true },
  wordmark: { type: Boolean, default: true },
})
</script>

<template>
  <span
    class="brand-logo"
    :class="{ solo: !(mark && wordmark) }"
    :style="{ height: height + 'px' }"
    role="img"
    aria-label="tessera"
  >
    <span v-if="mark" class="bl-mark" />
    <span v-if="wordmark" class="bl-word" />
  </span>
</template>

<style scoped>
.brand-logo {
  display: inline-flex;
  align-items: center;
  gap: 0.42em;
}
.bl-mark,
.bl-word {
  display: block;
  background: var(--t-accent-grad);
  flex: none;
}
/* The "t" glyph fills the full box height (viewBox 69.224×99.008). */
.bl-mark {
  height: 100%;
  aspect-ratio: 69.224 / 99.008;
  -webkit-mask: url(/mark-white.svg) center / contain no-repeat;
  mask: url(/mark-white.svg) center / contain no-repeat;
}
/* Alongside the mark the wordmark sits at ~64% so its cap aligns with the glyph
   body; on its own it fills the box height (viewBox 3371×755). */
.bl-word {
  height: 64%;
  aspect-ratio: 3371 / 755;
  -webkit-mask: url(/wordmark-white.svg) center / contain no-repeat;
  mask: url(/wordmark-white.svg) center / contain no-repeat;
}
.brand-logo.solo .bl-word {
  height: 100%;
}
</style>
