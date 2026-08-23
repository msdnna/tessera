<script setup>
import { useHelpStore } from '@/stores/help'

// Category → article tree for the help centre (#2792). The order comes from the
// index, which the build script already sorted, so nothing is re-sorted here.
const help = useHelpStore()
</script>

<template>
  <nav class="h-nav">
    <div v-for="cat in help.categories" :key="cat.name" class="h-cat">
      <div class="h-cat-name">{{ cat.name }}</div>
      <!-- Buttons, not links: the help centre is a modal over the reader's board
           (#2792), so switching articles must not touch the URL. -->
      <button
        v-for="a in cat.articles"
        :key="a.slug"
        type="button"
        class="h-link"
        :class="{ 'h-link-active': a.slug === help.current }"
        @click="help.open(a.slug)"
      >
        {{ a.title }}
      </button>
    </div>
  </nav>
</template>

<style scoped>
.h-nav {
  display: flex;
  flex-direction: column;
  gap: 18px;
}
.h-cat-name {
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--t-text3);
  padding: 0 8px 6px;
}
.h-link {
  display: block;
  width: 100%;
  border: none;
  background: transparent;
  font: inherit;
  text-align: left;
  cursor: pointer;
  padding: 6px 8px;
  border-radius: 6px;
  font-size: 13px;
  color: var(--t-text2);
  text-decoration: none;
  line-height: 1.35;
}
.h-link:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
/* Active state is driven by the store's `current` — there is no route behind
   these items to derive it from. */
.h-link-active {
  background: var(--t-hover);
  color: var(--t-primary);
  font-weight: 600;
}
</style>
