<script setup>
import { useHelpStore } from '@/stores/help'

// Category → article tree for the help centre (#2792). The order comes from the
// index, which the build script already sorted, so nothing is re-sorted here.
// v2 (#3107): category names are the tree's bold rows, articles are indented
// 30px rows, the open one on the active level.
const help = useHelpStore()
</script>

<template>
  <nav class="h-nav">
    <div v-for="cat in help.categories" :key="cat.key" class="h-cat">
      <div class="h-cat-name">{{ cat.name }}</div>
      <!-- Buttons, not links: the help centre is a window over the reader's
           board (#2792), so switching articles must not touch the URL. -->
      <button
        v-for="a in cat.articles"
        :key="a.slug"
        type="button"
        class="h-link"
        :class="{ 'h-link-active': a.slug === help.current }"
        :aria-current="a.slug === help.current ? 'page' : undefined"
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
  gap: 6px;
}
.h-cat {
  display: flex;
  flex-direction: column;
  gap: 1px;
}
.h-cat-name {
  padding: 6px 10px;
  font-size: 13px;
  font-weight: 700;
  color: var(--t-text1);
}
.h-link {
  display: block;
  width: 100%;
  min-height: 30px;
  border: none;
  background: transparent;
  font: inherit;
  text-align: left;
  cursor: pointer;
  padding: 6px 10px 6px 26px;
  border-radius: 7px;
  font-size: 13px;
  color: var(--t-text2);
  line-height: 1.35;
}
.h-link:hover {
  background: var(--t-hover-side);
  color: var(--t-text1);
}
.h-link:focus-visible {
  outline: 2px solid var(--t-primary);
  outline-offset: -2px;
}
/* Active state is driven by the store's `current` — there is no route behind
   these items to derive it from. */
.h-link-active,
.h-link-active:hover {
  background: var(--t-active);
  color: var(--t-text1);
  font-weight: 600;
}
</style>
