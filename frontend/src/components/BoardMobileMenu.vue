<script setup>
import { ref, computed, h } from 'vue'
import { NButton, NIcon, NDropdown, NModal, NCard } from 'naive-ui'
import {
  EllipsisVerticalOutline,
  PricetagsOutline,
  ArchiveOutline,
  CheckmarkOutline,
} from '@vicons/ionicons5'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { useBoardViewStore } from '@/stores/boardView'
import TagManager from './TagManager.vue'
import TesseraIcon from './TesseraIcon.vue'

const { t } = useI18n()
const store = useBoardViewStore()
const route = useRoute()
const router = useRouter()
const tagsOpen = ref(false)

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
  { key: 'tags', label: t('board.actions.tags'), icon: renderIcon(PricetagsOutline) },
  { key: 'archive', label: t('board.actions.archive'), icon: renderIcon(ArchiveOutline) },
])
// Tick the active layout (right-aligned check).
function renderLabel(option) {
  const active = option.key === 'layout:' + store.layout
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
  else if (key === 'tags') tagsOpen.value = true
  else if (key === 'archive') {
    const q = { ...route.query, archived: '1' }
    delete q.milestone
    router.push({ query: q })
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
      <TagManager :project-id="store.projectId" :tags="store.tagsList" @changed="onChanged" />
    </n-card>
  </n-modal>
</template>
