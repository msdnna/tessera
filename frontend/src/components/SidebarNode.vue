<script setup>
import { ref, computed, watch, nextTick } from 'vue'
import draggable from 'vuedraggable'
import { NIcon, NButton, NInput, NDropdown, NPopover, NPopconfirm, useMessage } from 'naive-ui'
import {
  FolderOutline,
  ChevronForwardOutline,
  EllipsisHorizontalOutline,
  AddOutline,
  CreateOutline,
  TrashOutline,
} from '@vicons/ionicons5'
import { workspaces as wsApi, groups as groupsApi } from '@/api'
import { useWorkspacesStore } from '@/stores/workspaces'
import { moveSidebarGroup, moveSidebarProject } from '@/composables/useSidebarDnd'
import ProjectRow from './ProjectRow.vue'

const props = defineProps({
  group: { type: Object, required: true },
  depth: { type: Number, default: 0 },
})

const store = useWorkspacesStore()
const message = useMessage()

const expanded = ref(true)
const renaming = ref(false)
const nameEdit = ref('')
const renameInput = ref(null)
const settingsShow = ref(false)

// right-click context menu
const ctxShow = ref(false)
const ctxX = ref(0)
const ctxY = ref(0)
const ctxOptions = [
  { label: 'Новый проект', key: 'add-project' },
  { label: 'Новая группа', key: 'add-group' },
  { type: 'divider', key: 'd1' },
  { label: 'Переименовать', key: 'rename' },
  { label: 'Удалить группу', key: 'delete' },
]
function onCtx(e) {
  ctxShow.value = false
  ctxX.value = e.clientX
  ctxY.value = e.clientY
  nextTick(() => (ctxShow.value = true))
}
function onCtxSelect(key) {
  ctxShow.value = false
  if (key === 'add-project') onAdd('project')
  else if (key === 'add-group') onAdd('group')
  else if (key === 'rename') startRename()
  else if (key === 'delete') remove()
}

const subgroups = computed(() => store.childGroups(props.group.id))
const childProjects = computed(() => store.projectsInGroup(props.group.id))

const grpModel = ref([])
const projModel = ref([])
watch(subgroups, (v) => (grpModel.value = [...v]), { immediate: true })
watch(childProjects, (v) => (projModel.value = [...v]), { immediate: true })
const onGrpChange = (evt) => moveSidebarGroup(evt, grpModel.value, props.group.id, store, message)
const onProjChange = (evt) =>
  moveSidebarProject(evt, projModel.value, props.group.id, store, message)

const addOptions = [
  { label: 'Проект', key: 'project' },
  { label: 'Группа', key: 'group' },
]

async function onAdd(key) {
  const wsId = store.currentId
  try {
    if (key === 'project') {
      await wsApi.createProject(wsId, { name: 'Проект', group_id: props.group.id })
    } else {
      await wsApi.createGroup(wsId, { name: 'Группа', parent_id: props.group.id })
    }
    await store.refresh()
    expanded.value = true
  } catch (e) {
    message.error(e.message)
  }
}

function startRename() {
  settingsShow.value = false
  nameEdit.value = props.group.name
  renaming.value = true
  nextTick(() => renameInput.value?.focus())
}
async function remove() {
  try {
    await groupsApi.remove(props.group.id)
    await store.refresh()
  } catch (e) {
    message.error(e.message)
  }
}

// click-outside saves if changed, else cancels
async function commitRename() {
  renaming.value = false
  const n = nameEdit.value.trim()
  if (!n || n === props.group.name) return
  try {
    await groupsApi.update(props.group.id, { name: n })
    await store.refresh()
  } catch (e) {
    message.error(e.message)
  }
}
</script>

<template>
  <div class="group-node">
    <div class="row group-row" @contextmenu.prevent.stop="onCtx">
      <n-icon
        class="chev"
        :class="{ open: expanded }"
        :component="ChevronForwardOutline"
        @click="expanded = !expanded"
      />
      <n-icon :component="FolderOutline" :size="16" class="folder" @click="expanded = !expanded" />
      <n-input
        v-if="renaming"
        ref="renameInput"
        v-model:value="nameEdit"
        size="tiny"
        @keyup.enter="commitRename"
        @blur="commitRename"
      />
      <span v-else class="name" @click="expanded = !expanded" @dblclick="startRename">
        {{ group.name }}
      </span>
      <n-dropdown trigger="click" :options="addOptions" @select="onAdd">
        <n-button class="hover-btn" text size="tiny" title="Добавить" @click.stop>
          <n-icon :component="AddOutline" />
        </n-button>
      </n-dropdown>
      <n-popover v-model:show="settingsShow" trigger="click" placement="right-start">
        <template #trigger>
          <n-button class="hover-btn" text size="tiny" @click.stop>
            <n-icon :component="EllipsisHorizontalOutline" />
          </n-button>
        </template>
        <div class="gsettings">
          <n-button size="small" block @click="startRename">
            <template #icon><n-icon :component="CreateOutline" /></template>
            Переименовать
          </n-button>
          <n-popconfirm @positive-click="remove">
            <template #trigger>
              <n-button type="error" ghost size="small" block>
                <template #icon><n-icon :component="TrashOutline" /></template>
                Удалить группу
              </n-button>
            </template>
            Подгруппы удалятся, проекты станут без группы. Удалить?
          </n-popconfirm>
        </div>
      </n-popover>
    </div>

    <div v-show="expanded" class="children">
      <draggable
        :list="grpModel"
        group="sidebar-grp"
        item-key="id"
        ghost-class="sb-ghost"
        class="sb-dropzone"
        :animation="150"
        :delay="160"
        :delay-on-touch-only="true"
        :touch-start-threshold="6"
        @change="onGrpChange"
      >
        <template #item="{ element }">
          <SidebarNode :group="element" :depth="depth + 1" />
        </template>
      </draggable>
      <draggable
        :list="projModel"
        group="sidebar-proj"
        item-key="id"
        ghost-class="sb-ghost"
        class="sb-dropzone"
        :animation="150"
        :delay="160"
        :delay-on-touch-only="true"
        :touch-start-threshold="6"
        @change="onProjChange"
      >
        <template #item="{ element }">
          <ProjectRow :project="element" :depth="depth + 1" />
        </template>
      </draggable>
    </div>

    <n-dropdown
      trigger="manual"
      placement="bottom-start"
      :show="ctxShow"
      :x="ctxX"
      :y="ctxY"
      :options="ctxOptions"
      @select="onCtxSelect"
      @clickoutside="ctxShow = false"
    />
  </div>
</template>

<style scoped>
.row {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 5px 8px;
  border-radius: 6px;
  cursor: pointer;
  font-size: 14px;
}
.row:hover {
  background: var(--t-hover);
}
.hover-btn {
  opacity: 0;
}
.group-row:hover .hover-btn {
  opacity: 1;
}
.chev {
  color: var(--t-text3);
  font-size: 12px;
  transition: transform 0.15s;
}
.chev.open {
  transform: rotate(90deg);
}
.folder {
  color: var(--t-text2);
  flex: none;
}
.name {
  flex: 1;
  font-weight: 600;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  color: var(--t-text1);
}
/* Nesting is shown by indenting the whole subtree, so the drag placeholder
   (.sb-ghost) inside a group is visually indented too — making it clear the
   item will land inside the group. */
.children {
  margin-left: 15px;
  padding-left: 6px;
  border-left: 1px solid var(--t-border);
}
.gsettings {
  width: 200px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
</style>
