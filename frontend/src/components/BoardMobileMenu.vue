<script setup>
// The phone's board menu (the «⋮» in its header, #2976): the views, «Вид
// доски», the saved views, the project's tags and stages, the archive. On a
// phone ActionMenu is a sheet from the bottom with 48px rows.
import { computed, ref } from 'vue'
import { NIcon } from 'naive-ui'
import {
  EllipsisVerticalOutline,
  PricetagsOutline,
  ArchiveOutline,
  OptionsOutline,
  CloseCircleOutline,
  TrailSignOutline,
  SettingsOutline,
} from '@vicons/ionicons5'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import ActionMenu from './kit/ActionMenu.vue'
import { useBoardViewStore } from '@/stores/boardView'
import { archiveQuery, withoutArchiveQuery, isArchiveQuery } from '@/utils/boardScope'
import { useProjectWindowStore } from '@/stores/projectWindow'
import { LAYOUTS, LAYOUT_ICONS } from '@/utils/boardViews'

const { t } = useI18n()
const store = useBoardViewStore()
const route = useRoute()
const router = useRouter()
const projectWindow = useProjectWindowStore()
// This menu is the phone's whole board toolbar (the desktop header's
// BoardActions are not there), so «Архив» has to carry its own state here: it
// is ticked while the scope is on and selecting it again leaves — same toggle
// as the desktop button, same query maths (utils/boardScope).
const isArchive = computed(() => isArchiveQuery(route.query))

// Menu options are plain objects rebuilt by the computed, so their labels follow
// a language change like any rendered text would. `glyph` is a Tessera
// icon-pack name (the views), `icon` an ionicon.
const options = computed(() => [
  {
    key: 'view',
    type: 'group',
    label: t('board.actions.view'),
    children: LAYOUTS.map((v) => ({
      key: `layout:${v}`,
      label: t(`board.layout.${v}`),
      glyph: LAYOUT_ICONS[v],
    })),
  },
  { type: 'divider', key: 'd1' },
  // «Вид доски» (#2986): the panel — a bottom sheet here — with the card
  // settings, grouping/filters, the board's name and the saved views (saving too).
  { key: 'customize', label: t('board.tools.customize'), icon: OptionsOutline },
  // The saved views (#2987), one list for every view kind: applying one switches
  // the view it was saved in.
  ...(store.savedViews.length
    ? [
        {
          key: 'views',
          type: 'group',
          label: t('board.views.chip'),
          children: [
            ...store.savedViews.map((v) => ({
              key: `view:${v.id}`,
              label: v.name,
              glyph: LAYOUT_ICONS[v.layout] || LAYOUT_ICONS.board,
            })),
            ...(store.currentViewId
              ? [{ key: 'view-none', label: t('board.views.none'), icon: CloseCircleOutline }]
              : []),
          ],
        },
      ]
    : []),
  { type: 'divider', key: 'd2' },
  // The project's tags and stages open the «Проект» window (#2971) — full
  // screen on a phone.
  { key: 'tags', label: t('board.actions.tags'), icon: PricetagsOutline },
  { key: 'stages', label: t('board.actions.milestones'), icon: TrailSignOutline },
  { key: 'project', label: t('project.menu.settings'), icon: SettingsOutline },
  // The label stays «Архив» in both states — the mark is what says the scope
  // is on, exactly as it does for the active layout right above it.
  { key: 'archive', label: t('board.actions.archive'), icon: ArchiveOutline },
])
// The loaded view's menu key, so it can be marked like the active layout is.
const activeViewKey = computed(() => (store.currentViewId ? `view:${store.currentViewId}` : ''))
// What is currently on: the active layout, the archive scope, the loaded view.
function isActive(option) {
  return option.key === 'archive'
    ? isArchive.value
    : option.key === 'layout:' + store.layout ||
        (!!activeViewKey.value && option.key === activeViewKey.value)
}
// The same list in ActionMenu's shape: groups → headings, dividers → gaps.
const items = computed(() => {
  const row = (o) => ({
    key: o.key,
    label: o.label,
    glyph: o.glyph,
    icon: o.icon,
    active: isActive(o),
  })
  const out = []
  for (const o of options.value) {
    if (o.type === 'divider') out.push({ gap: true, key: o.key })
    else if (o.type === 'group') out.push({ heading: o.label, key: o.key }, ...o.children.map(row))
    else out.push(row(o))
  }
  return out
})
function onSelect(key) {
  if (key.startsWith('layout:')) store.layout = key.slice(7)
  else if (key.startsWith('view:')) store.requestApplyView(key.slice(5))
  else if (key === 'view-none') store.requestClearView()
  else if (key === 'customize') store.openCustomize()
  else if (key === 'tags') projectWindow.open(store.projectId, 'tags')
  else if (key === 'stages') projectWindow.open(store.projectId, 'stages')
  else if (key === 'project') projectWindow.open(store.projectId, 'general')
  else if (key === 'archive') {
    if (isArchive.value) router.replace({ query: withoutArchiveQuery(route.query) })
    else router.push({ query: archiveQuery(route.query) })
  }
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
