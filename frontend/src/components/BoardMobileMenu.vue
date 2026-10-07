<script setup>
import { ref, computed, h } from 'vue'
import { NButton, NIcon, NDropdown, NModal, NCard } from 'naive-ui'
import {
  EllipsisVerticalOutline,
  PricetagsOutline,
  ArchiveOutline,
  CheckmarkOutline,
  OptionsOutline,
  CloseCircleOutline,
} from '@vicons/ionicons5'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { useBoardViewStore } from '@/stores/boardView'
import { archiveQuery, withoutArchiveQuery, isArchiveQuery } from '@/utils/boardScope'
import TagManager from './TagManager.vue'
import TesseraIcon from './TesseraIcon.vue'
import { LAYOUTS, LAYOUT_ICONS } from '@/utils/boardViews'

const { t } = useI18n()
const store = useBoardViewStore()
const route = useRoute()
const router = useRouter()
const tagsOpen = ref(false)
// This menu is the phone's whole board toolbar (Topbar renders it *instead of*
// BoardActions), so «Архив» has to carry its own state here: it is ticked while
// the scope is on and selecting it again leaves — same toggle as the desktop
// button, same query maths (utils/boardScope).
const isArchive = computed(() => isArchiveQuery(route.query))

const renderIcon = (comp) => () => h(NIcon, null, { default: () => h(comp) })
// Tessera icon-pack glyph for the view (layout) menu entries.
const renderVIcon = (name) => () => h(TesseraIcon, { name, size: 18 })
// Menu options are plain objects rebuilt by the computed, so their labels follow
// a language change like any rendered text would.
const options = computed(() => [
  {
    key: 'view',
    type: 'group',
    label: t('board.actions.view'),
    children: LAYOUTS.map((v) => ({
      key: `layout:${v}`,
      label: t(`board.layout.${v}`),
      icon: renderVIcon(LAYOUT_ICONS[v]),
    })),
  },
  { type: 'divider', key: 'd1' },
  // «Вид доски» (#2986): the panel — a bottom sheet here — with the card
  // settings, grouping/filters, the board's name and the saved views (saving too).
  { key: 'customize', label: t('board.tools.customize'), icon: renderIcon(OptionsOutline) },
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
              icon: renderVIcon(LAYOUT_ICONS[v.layout] || LAYOUT_ICONS.board),
            })),
            ...(store.currentViewId
              ? [
                  {
                    key: 'view-none',
                    label: t('board.views.none'),
                    icon: renderIcon(CloseCircleOutline),
                  },
                ]
              : []),
          ],
        },
      ]
    : []),
  { type: 'divider', key: 'd2' },
  { key: 'tags', label: t('board.actions.tags'), icon: renderIcon(PricetagsOutline) },
  // The label stays «Архив» in both states — the tick below is what says the
  // scope is on, exactly as it does for the active layout right above it.
  { key: 'archive', label: t('board.actions.archive'), icon: renderIcon(ArchiveOutline) },
])
// The loaded view's menu key, so it can be ticked like the active layout is.
const activeViewKey = computed(() => (store.currentViewId ? `view:${store.currentViewId}` : ''))
// Tick whatever is currently on: the active layout, the archive scope, the loaded view.
function renderLabel(option) {
  const active =
    option.key === 'archive'
      ? isArchive.value
      : option.key === 'layout:' + store.layout ||
        (!!activeViewKey.value && option.key === activeViewKey.value)
  if (!active) return option.label
  return h(
    'div',
    { style: 'display:flex;align-items:center;justify-content:space-between;gap:24px' },
    [
      h('span', option.label),
      h(
        NIcon,
        { size: 15, style: 'color:var(--t-primary)' },
        { default: () => h(CheckmarkOutline) },
      ),
    ],
  )
}
function onSelect(key) {
  if (key.startsWith('layout:')) store.layout = key.slice(7)
  else if (key.startsWith('view:')) store.requestApplyView(key.slice(5))
  else if (key === 'view-none') store.requestClearView()
  else if (key === 'customize') store.openCustomize()
  else if (key === 'tags') tagsOpen.value = true
  else if (key === 'archive') {
    if (isArchive.value) router.replace({ query: withoutArchiveQuery(route.query) })
    else router.push({ query: archiveQuery(route.query) })
  }
}
function onChanged() {
  store.bumpReload()
}
</script>

<template>
  <n-dropdown
    trigger="click"
    placement="bottom-end"
    :options="options"
    :render-label="renderLabel"
    @select="onSelect"
  >
    <n-button quaternary circle>
      <n-icon :component="EllipsisVerticalOutline" />
    </n-button>
  </n-dropdown>

  <n-modal v-model:show="tagsOpen">
    <n-card :title="t('board.actions.tags')" style="max-width: 360px" role="dialog">
      <!-- prefix-names is not decoration: TagManager merges the prefix-label editor
           over it, and the endpoint is a full replace — without it a save wipes the
           project's labels (same props as the desktop BoardActions popover). -->
      <TagManager
        :project-id="store.projectId"
        :tags="store.tagsList"
        :prefix-names="store.prefixNames"
        @changed="onChanged"
      />
    </n-card>
  </n-modal>
</template>
