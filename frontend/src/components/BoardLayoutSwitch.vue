<script setup>
import { computed, h } from 'vue'
import { useI18n } from 'vue-i18n'
import { NDropdown } from 'naive-ui'
import { ChevronDownOutline } from '@vicons/ionicons5'
import { NIcon } from 'naive-ui'
import { useBoardViewStore } from '@/stores/boardView'
import TesseraIcon from './TesseraIcon.vue'

const store = useBoardViewStore()
const { t } = useI18n()
// Tessera icon-pack names per visualization. Labels are resolved per render so
// the switcher follows a language change.
const LAYOUTS = [
  { value: 'board', icon: 'layout-kanban' },
  { value: 'list', icon: 'layout-list' },
  { value: 'calendar', icon: 'layout-calendar' },
  { value: 'timeline', icon: 'layout-timeline' },
  { value: 'gantt', icon: 'layout-gantt' },
  { value: 'matrix', icon: 'layout-matrix' },
]
// v2 segmented control (#2954): three segments on the bar, the rest behind a
// chevron. Канбан and Список always show; the third slot is Таймлайн, or the
// active view when it is one of the overflow ones, so the current view is
// always a visible, highlighted segment.
const PRIMARY = ['board', 'list', 'timeline']
const label = (v) => t(`board.layout.${v}`)
const shown = computed(() => {
  const keys = PRIMARY.includes(store.layout) ? PRIMARY : ['board', 'list', store.layout]
  return keys.map((k) => LAYOUTS.find((o) => o.value === k))
})
const overflow = computed(() =>
  LAYOUTS.filter((o) => !shown.value.some((s) => s.value === o.value)).map((o) => ({
    key: o.value,
    label: label(o.value),
    icon: () => h(TesseraIcon, { name: o.icon }),
  })),
)
</script>

<template>
  <div class="layout-switch" data-tour="board-layout">
    <button
      v-for="o in shown"
      :key="o.value"
      type="button"
      class="seg"
      :class="{ active: store.layout === o.value }"
      @click="store.layout = o.value"
    >
      <TesseraIcon
        :name="o.icon"
        class="seg-ic"
        :class="{ 'grad-icon': store.layout === o.value }"
      />
      {{ label(o.value) }}
    </button>
    <n-dropdown
      trigger="click"
      placement="bottom-start"
      :options="overflow"
      @select="(k) => (store.layout = k)"
    >
      <button type="button" class="seg seg-more" :title="t('board.layout.more')">
        <n-icon :component="ChevronDownOutline" :size="13" />
      </button>
    </n-dropdown>
  </div>
</template>

<style scoped>
.layout-switch {
  display: flex;
  align-items: center;
  gap: 2px;
  flex: none;
  padding: 3px;
  border-radius: var(--t-r-ctl);
  background: var(--t-ctl);
}
.seg {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 28px;
  padding: 0 10px;
  border: none;
  border-radius: calc(var(--t-r-ctl) - 2px);
  background: transparent;
  color: var(--t-text2);
  font: inherit;
  font-size: 13px;
  white-space: nowrap;
  cursor: pointer;
}
.seg:hover {
  background: var(--t-hover);
}
.seg-ic {
  font-size: 15px;
  color: var(--t-text3);
}
/* The active view lifts to the card level with the card shadow; its glyph takes
   the accent gradient. */
.seg.active {
  background: var(--t-card);
  box-shadow: var(--t-sh-card);
  color: var(--t-text1);
  font-weight: 600;
}
.seg.active .seg-ic {
  color: var(--t-primary);
}
.seg-more {
  width: 28px;
  padding: 0;
  justify-content: center;
  color: var(--t-text3);
}
</style>
