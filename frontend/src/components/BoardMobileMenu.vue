<script setup>
import { ref, computed, h } from 'vue'
import { NButton, NIcon, NDropdown, NModal, NCard } from 'naive-ui'
import {
  EllipsisVerticalOutline,
  PricetagsOutline,
  ArchiveOutline,
  CheckmarkOutline,
  SettingsOutline,
} from '@vicons/ionicons5'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { useBoardViewStore } from '@/stores/boardView'
import { archiveQuery, withoutArchiveQuery, isArchiveQuery } from '@/utils/boardScope'
import TagManager from './TagManager.vue'
import TesseraIcon from './TesseraIcon.vue'

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
const LAYOUTS = ['board', 'list', 'calendar', 'timeline', 'gantt', 'matrix']
const LAYOUT_ICONS = {
  board: 'layout-kanban',
  list: 'layout-list',
  calendar: 'layout-calendar',
  timeline: 'layout-timeline',
  gantt: 'layout-gantt',
  matrix: 'layout-matrix',
}
// The customize drawer doesn't apply to the time-axis layouts — same gate the
// composer's gear button carries (`v-if="!timelineLike"` in KanbanBoard).
const timelineLike = computed(() => store.layout === 'timeline' || store.layout === 'gantt')
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
  // «Настроить доску» is here because the gear that opens it lives in the composer's
  // right-hand tools, and those slide out (max-width: 0 + pointer-events: none) once
  // the chips plus the reserved search slice fill 75% of the bar — on a phone that is
  // two or three facets. Nothing else opens the drawer, so without this entry the card
  // size, the pill visibility, the board's name/icon and auto-collapse of empty columns
  // are simply unreachable from a phone.
  ...(timelineLike.value
    ? []
    : [
        {
          key: 'customize',
          label: t('board.tools.customize'),
          icon: renderIcon(SettingsOutline),
        },
      ]),
  { key: 'tags', label: t('board.actions.tags'), icon: renderIcon(PricetagsOutline) },
  // The label stays «Архив» in both states — the tick below is what says the
  // scope is on, exactly as it does for the active layout right above it.
  { key: 'archive', label: t('board.actions.archive'), icon: renderIcon(ArchiveOutline) },
])
// Tick whatever is currently on: the active layout, and the archive scope.
function renderLabel(option) {
  const active =
    option.key === 'archive' ? isArchive.value : option.key === 'layout:' + store.layout
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
