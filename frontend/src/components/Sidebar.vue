<script setup>
import { ref, computed, watch } from 'vue'
import draggable from 'vuedraggable'
import {
  NSelect,
  NButton,
  NInput,
  NModal,
  NCard,
  NScrollbar,
  NText,
  NIcon,
  NDropdown,
  useMessage,
} from 'naive-ui'
import { DocumentTextOutline, AlarmOutline, AddOutline } from '@vicons/ionicons5'
import { workspaces as wsApi } from '@/api'
import { useWorkspacesStore } from '@/stores/workspaces'
import { moveSidebarGroup, moveSidebarProject } from '@/composables/useSidebarDnd'
import SidebarNode from './SidebarNode.vue'
import ProjectRow from './ProjectRow.vue'
import SidebarFooter from './SidebarFooter.vue'

defineProps({ mobile: { type: Boolean, default: false } })

const store = useWorkspacesStore()
const message = useMessage()

const wsOptions = computed(() => store.list.map((w) => ({ label: w.name, value: w.id })))
const rootGroups = computed(() => store.childGroups(null))
const ungrouped = computed(() => store.projectsInGroup(null))

// Mutable mirrors for vuedraggable at the root level (parent/group = null).
const rootGrpModel = ref([])
const rootProjModel = ref([])
watch(rootGroups, (v) => (rootGrpModel.value = [...v]), { immediate: true })
watch(ungrouped, (v) => (rootProjModel.value = [...v]), { immediate: true })
const onRootGrp = (evt) => moveSidebarGroup(evt, rootGrpModel.value, null, store, message)
const onRootProj = (evt) => moveSidebarProject(evt, rootProjModel.value, null, store, message)

const addOptions = [
  { label: 'Проект', key: 'project' },
  { label: 'Группа', key: 'group' },
]

async function onWorkspaceChange(id) {
  await store.selectWorkspace(id)
}

async function addAtRoot(key) {
  try {
    if (key === 'project') {
      await wsApi.createProject(store.currentId, { name: 'Проект' })
    } else {
      await wsApi.createGroup(store.currentId, { name: 'Группа' })
    }
    await store.refresh()
  } catch (e) {
    message.error(e.message)
  }
}

// new-workspace modal
const wsModal = ref({ show: false, value: '' })
async function createWorkspace() {
  const n = wsModal.value.value.trim()
  if (!n) return
  try {
    const res = await wsApi.create({ name: n })
    wsModal.value.show = false
    wsModal.value.value = ''
    await store.loadWorkspaces()
    await store.selectWorkspace(res.data.id)
  } catch (e) {
    message.error(e.message)
  }
}
</script>

<template>
  <div class="sidebar">
    <div class="brand">
      <span class="brand-mark">mt</span>
      <span class="brand-name">Tessera</span>
    </div>

    <div class="ws-switch">
      <n-select
        :value="store.currentId"
        :options="wsOptions"
        size="small"
        @update:value="onWorkspaceChange"
      />
      <n-button
        quaternary
        circle
        size="small"
        title="Новое пространство"
        @click="wsModal.show = true"
      >
        <n-icon :component="AddOutline" />
      </n-button>
    </div>

    <nav class="nav">
      <router-link to="/notes" class="nav-link">
        <n-icon :component="DocumentTextOutline" :size="18" />
        <span>Заметки</span>
      </router-link>
      <router-link to="/reminders" class="nav-link">
        <n-icon :component="AlarmOutline" :size="18" />
        <span>Напоминания</span>
      </router-link>
    </nav>

    <div class="proj-head">
      <n-text depth="3" strong>Проекты</n-text>
      <n-dropdown trigger="click" :options="addOptions" @select="addAtRoot">
        <n-button text size="small" title="Добавить">
          <n-icon :component="AddOutline" />
        </n-button>
      </n-dropdown>
    </div>

    <n-scrollbar class="tree">
      <draggable
        :list="rootGrpModel"
        group="sidebar-grp"
        item-key="id"
        ghost-class="sb-ghost"
        class="sb-dropzone"
        :animation="150"
        :delay="160"
        :delay-on-touch-only="true"
        :touch-start-threshold="6"
        @change="onRootGrp"
      >
        <template #item="{ element }">
          <SidebarNode :group="element" :depth="0" />
        </template>
      </draggable>
      <draggable
        :list="rootProjModel"
        group="sidebar-proj"
        item-key="id"
        ghost-class="sb-ghost"
        class="sb-dropzone"
        :animation="150"
        :delay="160"
        :delay-on-touch-only="true"
        :touch-start-threshold="6"
        @change="onRootProj"
      >
        <template #item="{ element }">
          <ProjectRow :project="element" :depth="0" />
        </template>
      </draggable>
      <n-text v-if="!rootGroups.length && !ungrouped.length" depth="3" class="empty">
        Пусто — создайте проект или группу через «+».
      </n-text>
    </n-scrollbar>

    <SidebarFooter :mobile="mobile" />

    <n-modal v-model:show="wsModal.show">
      <n-card title="Новое пространство" style="max-width: 360px" role="dialog">
        <n-input
          v-model:value="wsModal.value"
          placeholder="Название"
          @keyup.enter="createWorkspace"
        />
        <template #footer>
          <n-button type="primary" @click="createWorkspace">Создать</n-button>
        </template>
      </n-card>
    </n-modal>
  </div>
</template>

<style scoped>
.sidebar {
  display: flex;
  flex-direction: column;
  height: 100%;
}
.brand {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 14px 14px 4px;
}
.brand-mark {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border-radius: 8px;
  background: var(--t-primary);
  color: var(--t-on-primary);
  font-weight: 700;
  font-size: 13px;
  letter-spacing: -0.5px;
}
.brand-name {
  font-weight: 700;
  font-size: 16px;
  color: var(--t-text1);
}
.ws-switch {
  display: flex;
  gap: 6px;
  padding: 12px;
  align-items: center;
}
.nav {
  display: flex;
  flex-direction: column;
  padding: 0 8px 8px;
}
.nav-link {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 8px;
  border-radius: 6px;
  font-size: 14px;
  color: var(--t-text2);
  text-decoration: none;
}
.nav-link:hover {
  background: var(--t-hover);
}
.nav-link.router-link-active {
  background: color-mix(in srgb, var(--t-primary) 16%, transparent);
  color: var(--t-primary);
  font-weight: 600;
}
.proj-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 4px 12px;
}
.tree {
  flex: 1;
  padding: 0 6px;
}
.empty {
  display: block;
  font-size: 12px;
  padding: 8px;
}
</style>
