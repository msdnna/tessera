<script setup>
import { ref, computed, h } from 'vue'
import { NButton, NIcon, NDropdown, NModal, NCard, NInput, NText } from 'naive-ui'
import {
  EllipsisVerticalOutline,
  PricetagsOutline,
  ArchiveOutline,
  CheckmarkOutline,
  SettingsOutline,
  FolderOpenOutline,
  SaveOutline,
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
  // Saved views: the folder and the disk that load/save them sit in the very same
  // sliding tools as the gear above, and they have no second trigger anywhere —
  // without this group a view saved on a desktop was unreachable from a phone.
  // Only this layout's views are listed, because that is the set the board itself
  // offers (views are per-visualization).
  {
    key: 'views',
    type: 'group',
    label: t('board.views.group'),
    children: [
      ...store.savedViews.map((v) => ({
        key: `view:${v.id}`,
        label: v.name,
        icon: renderIcon(FolderOpenOutline),
      })),
      { key: 'view-save', label: t('board.views.saveAs'), icon: renderIcon(SaveOutline) },
    ],
  },
  { type: 'divider', key: 'd2' },
  { key: 'tags', label: t('board.actions.tags'), icon: renderIcon(PricetagsOutline) },
  // The label stays «Архив» in both states — the tick below is what says the
  // scope is on, exactly as it does for the active layout right above it.
  { key: 'archive', label: t('board.actions.archive'), icon: renderIcon(ArchiveOutline) },
])
// The loaded view's menu key, so it can be ticked like the active layout is.
const activeViewKey = computed(() => {
  // Guard the empty name: "no view is loaded" is spelled '' and must not tick a
  // view that happens to carry an empty name.
  if (!store.currentViewName) return ''
  const v = store.savedViews.find((x) => x.name === store.currentViewName)
  return v ? `view:${v.id}` : ''
})
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
// Saving asks for a name first (the board owns the config, this only carries the
// name — see the store's requestSaveView). Prefilled with the loaded view, which is
// what the desktop popover's «Перезаписать» chips are for.
const saveOpen = ref(false)
const saveName = ref('')
const canSave = computed(() => !!(saveName.value.trim() || store.currentViewName))
function openSave() {
  saveName.value = store.currentViewName || ''
  saveOpen.value = true
}
function confirmSave() {
  if (!canSave.value) return
  store.requestSaveView(saveName.value.trim())
  saveOpen.value = false
}
function onSelect(key) {
  if (key.startsWith('layout:')) store.layout = key.slice(7)
  else if (key.startsWith('view:')) store.requestApplyView(key.slice(5))
  else if (key === 'view-save') openSave()
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

  <n-modal v-model:show="saveOpen">
    <n-card :title="t('board.views.save')" style="max-width: 360px" role="dialog">
      <n-input
        v-model:value="saveName"
        size="small"
        :placeholder="t('board.views.namePlaceholder')"
        @keyup.enter="confirmSave"
      />
      <n-text v-if="!store.savedViews.length" depth="3" class="views-empty">
        {{ t('board.views.empty') }}
      </n-text>
      <template #footer>
        <n-button type="primary" size="small" block :disabled="!canSave" @click="confirmSave">
          {{ t('common.action.save') }}
        </n-button>
      </template>
    </n-card>
  </n-modal>

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

<style scoped>
.views-empty {
  display: block;
  margin-top: 8px;
  font-size: 12px;
}
</style>
