<script setup>
// The phone's board menu (the «⋮» in its header, #2976): the views, «Вид
// доски», the saved views, then the board's actions — the project's tags,
// stages and settings, GitLab, the archive (useBoardActions, the same rows as
// the desktop «⋯», #3112). On a phone ActionMenu is a sheet from the bottom
// with 48px rows.
import { computed, ref } from 'vue'
import { NIcon } from 'naive-ui'
import { EllipsisVerticalOutline, OptionsOutline, CloseCircleOutline } from '@vicons/ionicons5'
import { useI18n } from 'vue-i18n'
import ActionMenu from './kit/ActionMenu.vue'
import { useBoardViewStore } from '@/stores/boardView'
import { useBoardActions } from '@/composables/useBoardActions'
import { LAYOUTS, LAYOUT_ICONS } from '@/utils/boardViews'

const { t } = useI18n()
const store = useBoardViewStore()
const actions = useBoardActions()

// Rows are plain objects rebuilt by the computed, so their labels follow a
// language change like any rendered text would. `glyph` is a Tessera icon-pack
// name (the views), `icon` an ionicon. The active layout and the loaded view
// are marked as current.
const items = computed(() => [
  { heading: t('board.actions.view'), key: 'h-view' },
  ...LAYOUTS.map((v) => ({
    key: `layout:${v}`,
    label: t(`board.layout.${v}`),
    glyph: LAYOUT_ICONS[v],
    active: store.layout === v,
  })),
  { gap: true, key: 'd1' },
  // «Вид доски» (#2986): the panel — a bottom sheet here — with the card
  // settings, grouping/filters, the board's name and the saved views (saving too).
  { key: 'customize', label: t('board.tools.customize'), icon: OptionsOutline },
  // The saved views (#2987), one list for every view kind: applying one switches
  // the view it was saved in.
  ...(store.savedViews.length
    ? [
        { heading: t('board.views.chip'), key: 'h-views' },
        ...store.savedViews.map((v) => ({
          key: `view:${v.id}`,
          label: v.name,
          glyph: LAYOUT_ICONS[v.layout] || LAYOUT_ICONS.board,
          active: store.currentViewId === v.id,
        })),
        ...(store.currentViewId
          ? [{ key: 'view-none', label: t('board.views.none'), icon: CloseCircleOutline }]
          : []),
      ]
    : []),
  { gap: true, key: 'd2' },
  ...actions.items.value,
])
function onSelect(key) {
  if (actions.select(key)) return
  if (key.startsWith('layout:')) store.layout = key.slice(7)
  else if (key.startsWith('view:')) store.requestApplyView(key.slice(5))
  else if (key === 'view-none') store.requestClearView()
  else if (key === 'customize') store.openCustomize()
}

const open = ref(false)
const btn = ref(null)
</script>

<template>
  <!-- A 44px target in the phone's header (#2976). -->
  <button
    ref="btn"
    type="button"
    class="bmm-btn"
    :aria-label="t('board.actions.menu')"
    :aria-expanded="open"
    data-testid="board-mobile-menu"
    @click="open = !open"
  >
    <n-icon :component="EllipsisVerticalOutline" :size="20" />
  </button>
  <ActionMenu
    v-model:show="open"
    :anchor="() => btn"
    placement="bottom-end"
    :items="items"
    :width="260"
    testid="board-mobile-menu-list"
    @select="onSelect"
  />
</template>

<style scoped>
.bmm-btn {
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 44px;
  height: 44px;
  padding: 0;
  border: 0;
  border-radius: 12px;
  background: transparent;
  color: var(--t-text2);
  cursor: pointer;
  -webkit-tap-highlight-color: transparent;
}
.bmm-btn:active {
  background: var(--t-hover);
}
</style>
