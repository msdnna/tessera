<script setup>
import { ref, computed, watch, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import { SearchOutline } from '@vicons/ionicons5'
import { useHelpStore } from '@/stores/help'
import Kbd from '@/components/Kbd.vue'

const { t } = useI18n()

// Search box over the help index (#2792). Matching runs in the store against
// the in-bundle index — there is no request to debounce, so results update on
// every keystroke.
//
// v2 (#3107): the box lives in the help window's head and the hits drop under
// it; «/» puts the cursor in it from anywhere in the window, ↑↓ walk the hits,
// ↵ opens one, Esc clears the query first and only then lets the window close.
const help = useHelpStore()
const input = ref(null)
const cursor = ref(0)
const open = computed(() => !!help.query.trim())
watch(
  () => help.query,
  () => (cursor.value = 0),
)

function pick(slug) {
  help.query = ''
  help.open(slug)
}

function onInputKey(e) {
  const hits = help.results
  if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
    if (!hits.length) return
    e.preventDefault()
    const step = e.key === 'ArrowDown' ? 1 : -1
    cursor.value = (cursor.value + step + hits.length) % hits.length
  } else if (e.key === 'Enter' && hits[cursor.value]) {
    e.preventDefault()
    pick(hits[cursor.value].slug)
  } else if (e.key === 'Escape' && help.query) {
    e.preventDefault()
    e.stopPropagation()
    help.query = ''
  }
}

// «/» from anywhere in the help window, as long as nothing else takes text.
function onSlash(e) {
  if (e.key !== '/' || e.metaKey || e.ctrlKey || e.altKey) return
  if (!input.value?.closest?.('[data-help-center]')) return
  if (e.target?.closest?.('input, textarea, [contenteditable="true"]')) return
  e.preventDefault()
  input.value?.focus()
}
onMounted(() => window.addEventListener('keydown', onSlash))
onBeforeUnmount(() => window.removeEventListener('keydown', onSlash))
</script>

<template>
  <div class="h-search">
    <label class="h-box">
      <n-icon :component="SearchOutline" :size="15" class="h-box-ic" />
      <input
        ref="input"
        v-model="help.query"
        class="h-input"
        type="search"
        :placeholder="t('help.search.placeholder')"
        data-help-search
        @keydown="onInputKey"
      />
      <Kbd v-if="!help.query" soft>/</Kbd>
    </label>

    <div v-if="open" class="h-results" data-help-results>
      <div v-if="!help.results.length" class="h-empty">{{ t('help.search.empty') }}</div>
      <button
        v-for="(r, i) in help.results"
        :key="r.slug"
        type="button"
        class="h-result"
        :class="{ cur: i === cursor }"
        @mousedown.prevent
        @mouseenter="cursor = i"
        @click="pick(r.slug)"
      >
        <span class="h-result-title">{{ r.title }}</span>
        <span class="h-result-cat">{{ r.category }}</span>
        <span v-if="r.excerpt" class="h-result-excerpt">{{ r.excerpt }}</span>
      </button>
    </div>
  </div>
</template>

<style scoped>
.h-search {
  position: relative;
}
.h-box {
  display: flex;
  align-items: center;
  gap: 8px;
  height: 36px;
  padding: 0 6px 0 11px;
  border-radius: 10px;
  background: var(--t-ctl);
  cursor: text;
}
.h-box:focus-within {
  box-shadow: 0 0 0 2px var(--t-primary);
}
.h-box-ic {
  flex: none;
  color: var(--t-text3);
}
.h-input {
  flex: 1;
  min-width: 0;
  height: 100%;
  border: 0;
  outline: 0;
  background: transparent;
  color: var(--t-text1);
  font: inherit;
  font-size: 13.5px;
}
.h-input::placeholder {
  color: var(--t-placeholder);
}
.h-input::-webkit-search-cancel-button {
  display: none;
}
/* The hits drop under the box, over the window. */
.h-results {
  position: absolute;
  z-index: 3;
  top: calc(100% + 6px);
  left: 0;
  right: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
  max-height: 380px;
  overflow-y: auto;
  padding: 6px;
  border-radius: 14px;
  background: var(--t-pop);
  box-shadow: var(--t-sh-pop);
}
.h-empty {
  font-size: 12.5px;
  color: var(--t-text3);
  padding: 8px 10px;
}
.h-result {
  display: grid;
  grid-template-columns: 1fr auto;
  gap: 2px 8px;
  text-align: left;
  padding: 8px 10px;
  border: none;
  border-radius: 7px;
  background: transparent;
  cursor: pointer;
  font: inherit;
  color: var(--t-text1);
}
.h-result.cur {
  background: var(--t-hover);
}
.h-result-title {
  font-size: 13px;
  font-weight: 600;
}
.h-result-cat {
  font-size: 11px;
  color: var(--t-text3);
  white-space: nowrap;
}
.h-result-excerpt {
  grid-column: 1 / -1;
  font-size: 12px;
  color: var(--t-text2);
  line-height: 1.4;
  /* Excerpts come from the flattened body and can be long — two lines keep the
     result list scannable. */
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
</style>
