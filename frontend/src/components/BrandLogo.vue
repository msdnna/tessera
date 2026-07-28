<script setup>
// Brand lockup: the tessera "t" mark + the "tessera" wordmark, both painted with
// the accent gradient via CSS masks (theme-reactive on light/dark). `wordmark`
// off → just the mark (e.g. a collapsed sidebar rail). `height` is the lockup
// height in px; the wordmark is optically smaller than the tall "t" glyph.
defineProps({
  height: { type: Number, default: 22 },
  wordmark: { type: Boolean, default: true },
})
</script>

<template>
  <span class="brand-logo" :style="{ height: height + 'px' }" role="img" aria-label="tessera">
    <span class="bl-mark" />
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
/* The "t" glyph fills the full lockup height (viewBox 69.224×99.008). */
.bl-mark {
  height: 100%;
  aspect-ratio: 69.224 / 99.008;
  -webkit-mask: url(/mark-white.svg) center / contain no-repeat;
  mask: url(/mark-white.svg) center / contain no-repeat;
}
/* The wordmark sits at ~64% of the lockup height so its cap aligns with the
   glyph's body (viewBox 3371×755). */
.bl-word {
  height: 64%;
  aspect-ratio: 3371 / 755;
  -webkit-mask: url(/wordmark-white.svg) center / contain no-repeat;
  mask: url(/wordmark-white.svg) center / contain no-repeat;
}
</style>
