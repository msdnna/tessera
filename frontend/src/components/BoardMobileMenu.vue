<script setup>
import { ref, computed, h } from 'vue'
import { NButton, NIcon, NDropdown, NModal, NCard } from 'naive-ui'
import {
  EllipsisVerticalOutline,
  PricetagsOutline,
  ArchiveOutline,
  CheckmarkOutline,
} from '@vicons/ionicons5'
import { useRoute, useRouter } from 'vue-router'
import { useBoardViewStore } from '@/stores/boardView'
import TagManager from './TagManager.vue'
import TesseraIcon from './TesseraIcon.vue'

const store = useBoardViewStore()
const route = useRoute()
const router = useRouter()
const tagsOpen = ref(false)

const renderIcon = (comp) => () => h(NIcon, null, { default: () => h(comp) })
// Tessera icon-pack glyph for the view (layout) menu entries.
const renderVIcon = (name) => () => h(TesseraIcon, { name, size: 18 })
const layouts = [
  { key: 'layout:board', label: 'Доска', icon: renderVIcon('layout-kanban') },
  { key: 'layout:list', label: 'Список', icon: renderVIcon('layout-list') },
  { key: 'layout:calendar', label: 'Календарь', icon: renderVIcon('layout-calendar') },
  { key: 'layout:timeline', label: 'Таймлайн', icon: renderVIcon('layout-timeline') },
  { key: 'layout:gantt', label: 'Гант', icon: renderVIcon('layout-gantt') },
  { key: 'layout:matrix', label: 'Матрица', icon: renderVIcon('layout-matrix') },
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
    <n-card title="Теги" style="max-width: 360px" role="dialog">
      <TagManager :project-id="store.projectId" :tags="store.tagsList" @changed="onChanged" />
    </n-card>
  </n-modal>
</template>
