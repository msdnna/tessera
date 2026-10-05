<script setup>
// The cheat sheet (v2, #2956): every visible catalog entry by group, opened
// with `?` from anywhere or the «Клавиши» button in the header. Rendered from
// the same catalog the dispatcher reads, so it can't list a key that doesn't work.
import { ref, computed, watch, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { NModal, NInput, NIcon } from 'naive-ui'
import { SearchOutline } from '@vicons/ionicons5'
import ShortcutKeys from './ShortcutKeys.vue'
import { CATALOG, GROUPS } from '@/shortcuts/catalog'
import { keysText } from '@/shortcuts/labels'
import { useQuickStore } from '@/stores/quick'
import { useShortcuts } from '@/composables/useShortcuts'
import { useOverlayBack } from '@/composables/useOverlayBack'

const { t } = useI18n()
const quick = useQuickStore()
const q = ref('')
const searchEl = ref(null)

const show = computed({
  get: () => quick.sheetOpen,
  set: (v) => (quick.sheetOpen = v),
})
useOverlayBack(show, () => (show.value = false))
useShortcuts({ 'nav.shortcuts': () => (quick.sheetOpen = !quick.sheetOpen) })

watch(show, async (open) => {
  if (!open) return
  q.value = ''
  await nextTick()
  searchEl.value?.focus()
})

// Search by what the key does or by the key itself («G B», «⇧T»).
const groups = computed(() => {
  const term = q.value.trim().toLowerCase()
  return GROUPS.map((g) => ({
    id: g,
    title: t(`shell.keys.groups.${g}`),
    rows: CATALOG.filter((e) => e.group === g)
      .map((e) => ({ id: e.id, label: t(`shell.keys.${e.id}`) }))
      .filter(
        (r) =>
          !term ||
          r.label.toLowerCase().includes(term) ||
          keysText(r.id).toLowerCase().includes(term.replace(/\s+/g, ' ')),
      ),
  })).filter((g) => g.rows.length)
})
</script>

<template>
  <n-modal v-model:show="show" :auto-focus="false" class="ks-modal">
    <div class="ks" role="dialog" aria-modal="true" data-testid="shortcuts-sheet">
      <div class="ks-head">
        <span class="ks-title">{{ t('shell.keys.title') }}</span>
        <span class="ks-sp" />
        <n-input
          ref="searchEl"
          v-model:value="q"
          class="ks-search"
          size="small"
          clearable
          :placeholder="t('shell.keys.search')"
        >
          <template #prefix><n-icon :component="SearchOutline" /></template>
        </n-input>
        <ShortcutKeys id="palette.close" />
      </div>
      <div class="ks-grid">
        <div v-for="g in groups" :key="g.id" class="ks-group">
          <div class="ks-group-t">{{ g.title }}</div>
          <div v-for="r in g.rows" :key="r.id" class="ks-row" :data-shortcut="r.id">
            <span class="ks-label">{{ r.label }}</span>
            <ShortcutKeys :id="r.id" />
          </div>
        </div>
        <div v-if="!groups.length" class="ks-empty">{{ t('shell.keys.nothing') }}</div>
      </div>
      <div class="ks-foot">
        <span>{{ t('shell.keys.footBefore') }}</span>
        <ShortcutKeys id="nav.shortcuts" />
        <span>{{ t('shell.keys.footAfter') }}</span>
      </div>
    </div>
  </n-modal>
</template>

<style scoped>
.ks {
  width: 920px;
  max-width: calc(100vw - 32px);
  max-height: calc(100vh - 64px);
  display: flex;
  flex-direction: column;
  border-radius: 18px;
  background: var(--t-pop);
  box-shadow: var(--t-sh-drag);
  overflow: hidden;
  color: var(--t-text1);
}
.ks-head {
  flex: none;
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 20px 22px 14px;
}
.ks-title {
  font-size: 18px;
  font-weight: 650;
}
.ks-sp {
  flex: 1;
}
.ks-search {
  width: 280px;
  --n-height: 34px !important;
}
.ks-grid {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 8px 28px;
  padding: 6px 22px 22px;
  align-items: start;
}
.ks-group {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding-top: 10px;
}
.ks-group-t {
  padding: 0 0 6px;
  font-size: 11.5px;
  font-weight: 600;
  letter-spacing: 0.05em;
  text-transform: uppercase;
  color: var(--t-text3);
}
.ks-row {
  display: flex;
  align-items: center;
  gap: 10px;
  min-height: 32px;
  padding: 0 6px;
  margin: 0 -6px;
  border-radius: 7px;
}
.ks-row:hover {
  background: var(--t-hover);
}
.ks-label {
  flex: 1;
  min-width: 0;
  font-size: 13px;
  color: var(--t-text2);
}
.ks-empty {
  grid-column: 1 / -1;
  padding: 24px 0;
  text-align: center;
  font-size: 13px;
  color: var(--t-text3);
}
.ks-foot {
  flex: none;
  display: flex;
  align-items: center;
  gap: 8px;
  height: 44px;
  padding: 0 22px;
  background: color-mix(in srgb, var(--t-pop) 55%, var(--t-bg));
  font-size: 12.5px;
  color: var(--t-text3);
}
@media (max-width: 900px) {
  .ks-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
