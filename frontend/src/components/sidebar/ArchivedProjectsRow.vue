<script setup>
// «Архив проектов» (#3005): the foot of the project tree, drawn only while the
// workspace has archived projects. The tree leaves them out, so this row is the
// way back to one: its menu lists them, each with «Открыть» (the first board, on
// view only), «Настройки проекта» (the «Проект» window) and — for owners and
// admins — «Восстановить», which puts the project back into the tree.
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { NIcon } from 'naive-ui'
import {
  ArchiveOutline,
  ArrowUndoOutline,
  EyeOutline,
  FolderOpenOutline,
  SettingsOutline,
} from '@vicons/ionicons5'
import ActionMenu from '../kit/ActionMenu.vue'
import { projects as projApi } from '@/api'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useProjectWindowStore } from '@/stores/projectWindow'
import { useProjectArchive } from '@/composables/useProjectArchive'
import { useToast } from '@/composables/useToast'
import { humanizeError } from '@/utils/errors'

const { t } = useI18n()
const router = useRouter()
const store = useWorkspacesStore()
const projectWindow = useProjectWindowStore()
const { restoreProject } = useProjectArchive()
const message = useToast()

const open = ref(false)
const rowEl = ref(null)
const list = computed(() => store.archivedProjects || [])

const items = computed(() =>
  list.value.map((p) => ({
    key: 'p:' + p.id,
    label: p.name,
    icon: FolderOpenOutline,
    subWidth: 240,
    testid: 'archived-project',
    children: [
      { key: 'open:' + p.id, label: t('project.archive.open'), icon: EyeOutline },
      { key: 'settings:' + p.id, label: t('project.menu.settings'), icon: SettingsOutline },
      {
        key: 'restore:' + p.id,
        label: t('project.archive.restore'),
        icon: ArrowUndoOutline,
        disabled: !store.canManage,
        disabledTitle: t('project.menu.managerOnly'),
      },
    ],
  })),
)

async function openProject(p) {
  try {
    const { data } = await projApi.boards(p.id)
    const board = (data || [])[0]
    if (board) router.push(`/project/${p.slug}/board/${board.slug}`)
    else projectWindow.open(p.id, 'general')
  } catch (e) {
    message?.error(humanizeError(e))
  }
}

function onSelect(key) {
  const [op, id] = key.split(':')
  const p = list.value.find((x) => x.id === id)
  if (!p) return
  open.value = false
  if (op === 'open') openProject(p)
  else if (op === 'settings') projectWindow.open(p.id, 'general')
  else if (op === 'restore') restoreProject(p)
}
</script>

<template>
  <div v-if="list.length" class="arch">
    <button
      ref="rowEl"
      type="button"
      class="arch-row"
      :class="{ on: open }"
      data-testid="archived-projects"
      @click="open = !open"
    >
      <n-icon :component="ArchiveOutline" :size="15" class="arch-glyph" />
      <span class="arch-name">{{ t('project.archive.list') }}</span>
      <span class="arch-n">{{ list.length }}</span>
    </button>
    <ActionMenu
      v-model:show="open"
      :anchor="() => rowEl"
      :header="t('project.archive.list')"
      :header-aside="String(list.length)"
      :items="items"
      :width="260"
      testid="archived-projects-menu"
      @select="onSelect"
    />
  </div>
</template>

<style scoped>
.arch {
  margin-top: 6px;
  padding-top: 6px;
  border-top: 1px solid var(--t-hover);
}
.arch-row {
  display: flex;
  align-items: center;
  gap: 7px;
  width: 100%;
  height: 30px;
  padding: 0 8px;
  box-sizing: border-box;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: var(--t-text2);
  font: inherit;
  font-size: 13px;
  text-align: left;
  cursor: pointer;
}
.arch-row:hover,
.arch-row.on {
  background: var(--t-hover);
  color: var(--t-text1);
}
.arch-row:focus-visible {
  outline: none;
  box-shadow: inset 0 0 0 2px var(--t-primary);
}
.arch-glyph {
  color: var(--t-text3);
}
.arch-name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.arch-n {
  font-size: 11.5px;
  color: var(--t-text3);
}
</style>
