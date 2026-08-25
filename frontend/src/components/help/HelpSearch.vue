<script setup>
import { useI18n } from 'vue-i18n'
import { NInput, NIcon } from 'naive-ui'
import { SearchOutline } from '@vicons/ionicons5'
import { useHelpStore } from '@/stores/help'

const { t } = useI18n()

// Search box over the help index (#2792). Matching runs in the store against
// the in-bundle index — there is no request to debounce, so results update on
// every keystroke.
const help = useHelpStore()

function pick(slug) {
  help.query = ''
  help.open(slug)
}
</script>

<template>
  <div class="h-search">
    <n-input
      v-model:value="help.query"
      :placeholder="t('help.search.placeholder')"
      clearable
      size="small"
      data-help-search
    >
      <template #prefix>
        <n-icon :component="SearchOutline" />
      </template>
    </n-input>

    <div v-if="help.query.trim()" class="h-results">
      <div v-if="!help.results.length" class="h-empty">{{ t('help.search.empty') }}</div>
      <button
        v-for="r in help.results"
        :key="r.slug"
        type="button"
        class="h-result"
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
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.h-results {
  display: flex;
  flex-direction: column;
  gap: 2px;
  max-height: 320px;
  overflow-y: auto;
}
.h-empty {
  font-size: 12px;
  color: var(--t-text3);
  padding: 6px 8px;
}
.h-result {
  display: grid;
  grid-template-columns: 1fr auto;
  gap: 2px 8px;
  text-align: left;
  padding: 6px 8px;
  border: none;
  border-radius: 6px;
  background: transparent;
  cursor: pointer;
  font: inherit;
  color: var(--t-text1);
}
.h-result:hover {
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
  line-height: 1.35;
  /* Excerpts come from the flattened body and can be long — two lines keep the
     result list scannable. */
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
</style>
