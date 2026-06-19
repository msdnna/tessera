<script setup>
import { ref, computed, h } from 'vue'
import { NButton, NIcon, NDropdown, NModal, NCard } from 'naive-ui'
import {
  EllipsisVerticalOutline,
  GridOutline,
  ListOutline,
  CalendarClearOutline,
  AppsOutline,
  PricetagsOutline,
  ArchiveOutline,
  CheckmarkOutline,
} from '@vicons/ionicons5'
import { useBoardViewStore } from '@/stores/boardView'
import TagManager from './TagManager.vue'
import ArchiveModal from './ArchiveModal.vue'

const store = useBoardViewStore()
const tagsOpen = ref(false)

const renderIcon = (comp) => () => h(NIcon, null, { default: () => h(comp) })
const layouts = [
  { key: 'layout:board', label: 'Доска', icon: renderIcon(GridOutline) },
  { key: 'layout:list', label: 'Список', icon: renderIcon(ListOutline) },
  { key: 'layout:calendar', label: 'Календарь', icon: renderIcon(CalendarClearOutline) },
  { key: 'layout:matrix', label: 'Матрица', icon: renderIcon(AppsOutline) },
]
const options = computed(() => [
  { key: 'view', type: 'group', label: 'Представление', children: layouts },
  { type: 'divider', key: 'd1' },
  { key: 'tags', label: 'Теги', icon: renderIcon(PricetagsOutline) },
  { key: 'archive', label: 'Архив', icon: renderIcon(ArchiveOutline) },
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
      h(NIcon, { size: 15, style: 'color:var(--t-primary)' }, { default: () => h(CheckmarkOutline) }),
    ],
  )
}
function onSelect(key) {
  if (key.startsWith('layout:')) store.layout = key.slice(7)
  else if (key === 'tags') tagsOpen.value = true
  else if (key === 'archive') store.archiveOpen = true
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
    <n-card title="Теги" style="max-width: 360px" role="dialog">
      <TagManager :project-id="store.projectId" :tags="store.tagsList" @changed="onChanged" />
    </n-card>
  </n-modal>

  <ArchiveModal v-model:show="store.archiveOpen" :board-id="store.boardId" @changed="onChanged" />
</template>
