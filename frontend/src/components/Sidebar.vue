<script setup>
import { ref, computed, watch, h } from 'vue'
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
  NTooltip,
  useMessage,
} from 'naive-ui'
import {
  HomeOutline,
  DocumentTextOutline,
  AlarmOutline,
  RibbonOutline,
  AddOutline,
  FolderOutline,
  EllipsisHorizontalOutline,
  TrashOutline,
} from '@vicons/ionicons5'

const menuIcon = (icon) => () => h(NIcon, null, { default: () => h(icon) })
const dangerIcon = (icon) => () => h(NIcon, { color: '#e0533d' }, { default: () => h(icon) })
import { useRoute, useRouter } from 'vue-router'
import { workspaces as wsApi } from '@/api'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useAuthStore } from '@/stores/auth'
import ConfirmByName from './ConfirmByName.vue'
import ProjectCreateModal from './ProjectCreateModal.vue'
import {
  moveSidebarGroup,
  moveSidebarProject,
  sidebarDragging,
  onDragStart,
  onDragEnd,
} from '@/composables/useSidebarDnd'
import SidebarNode from './SidebarNode.vue'
import ProjectRow from './ProjectRow.vue'
import EmptyState from './EmptyState.vue'
import SidebarFooter from './SidebarFooter.vue'
import WorkspaceTools from './WorkspaceTools.vue'
import SidebarRailNode from './SidebarRailNode.vue'
import BrandLogo from './BrandLogo.vue'

defineProps({
  mobile: { type: Boolean, default: false },
  collapsed: { type: Boolean, default: false },
  // Expanded but too narrow for the brand tools / add-workspace button.
  narrow: { type: Boolean, default: false },
})

const store = useWorkspacesStore()
const auth = useAuthStore()
const message = useMessage()
const route = useRoute()
const router = useRouter()

const wsOptions = computed(() => store.list.map((w) => ({ label: w.name, value: w.id })))
const rootGroups = computed(() => store.childGroups(null))
const ungrouped = computed(() => store.projectsInGroup(null))

// On the collapsed rail, highlight the root node (top-level group, or ungrouped
// project) that contains the currently-open board's project — so the active
// branch stays visible the way the expanded tree shows it. Resolved from the
// route's projectSlug, walking group ancestors up to the root.
const activeRailId = computed(() => {
  const slug = route.params.projectSlug
  if (!slug) return null
  const proj = store.projects.find((p) => p.slug === slug)
  if (!proj) return null
  if (!proj.group_id) return proj.id // ungrouped → its own rail node
  let g = store.groups.find((x) => x.id === proj.group_id)
  while (g?.parent_id) {
    const parent = store.groups.find((x) => x.id === g.parent_id)
    if (!parent) break
    g = parent
  }
  return g?.id ?? null
})

// Mutable mirrors for vuedraggable at the root level (parent/group = null).
const rootGrpModel = ref([])
const rootProjModel = ref([])
watch(rootGroups, (v) => (rootGrpModel.value = [...v]), { immediate: true })
watch(ungrouped, (v) => (rootProjModel.value = [...v]), { immediate: true })
const onRootGrp = (evt) => moveSidebarGroup(evt, rootGrpModel.value, null, store, message)
const onRootProj = (evt) => moveSidebarProject(evt, rootProjModel.value, null, store, message)

const addOptions = [
  { label: 'Проект', key: 'project', icon: menuIcon(DocumentTextOutline) },
  { label: 'Группа', key: 'group', icon: menuIcon(FolderOutline) },
]

async function onWorkspaceChange(id) {
  await store.selectWorkspace(id)
}

// Projects are named in a modal (the name decides the URL address, which is
// assigned once); groups keep the create-then-rename flow.
const projModalShow = ref(false)

async function addAtRoot(key) {
  if (key === 'project') {
    projModalShow.value = true
    return
  }
  try {
    await wsApi.createGroup(store.currentId, { name: 'Группа' })
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

// Workspace settings menu — deletion is owner-only (server enforces it too).
const isOwner = computed(() => !!store.current && store.current.owner_id === auth.user?.id)
const wsMenuOptions = computed(() =>
  isOwner.value
    ? [
        {
          label: 'Удалить пространство',
          key: 'delete',
          icon: dangerIcon(TrashOutline),
          props: { style: 'color:#e0533d' },
        },
      ]
    : [],
)
const wsDeleteShow = ref(false)
function onWsMenuSelect(key) {
  if (key === 'delete') {
    if (store.list.length <= 1) {
      message.warning('Нельзя удалить единственное пространство')
      return
    }
    wsDeleteShow.value = true
  }
}
async function deleteWorkspace() {
  try {
    await store.removeWorkspace(store.currentId)
    message.success('Пространство удалено')
    router.push('/') // the tree fully changed — land on Home
  } catch (e) {
    message.error(e.message)
  }
}
</script>

<template>
  <div class="sidebar" data-testid="sidebar" :class="{ collapsed, 'sb-dragging': sidebarDragging }">
    <div class="brand">
      <!-- Expanded: wordmark sized so its text stays no taller than the tool icons
           to its right. Collapsed rail: the mark, a touch larger. -->
      <BrandLogo
        class="brand-logo"
        :height="collapsed ? 26 : 19"
        :mark="collapsed"
        :wordmark="!collapsed"
      />
      <!-- Tools live here (right of the logo) when expanded; when the rail is
           collapsed (desktop) they move to the header instead. -->
      <WorkspaceTools
        v-if="mobile || (!collapsed && !narrow)"
        class="brand-tools"
        placement="bottom-start"
      />
    </div>

    <div v-if="collapsed" class="rail-sep" />

    <div v-if="!collapsed" class="ws-switch">
      <n-select
        :value="store.currentId"
        :options="wsOptions"
        size="small"
        @update:value="onWorkspaceChange"
      />
      <n-button
        v-if="!narrow"
        quaternary
        circle
        size="small"
        title="Новое пространство"
        @click="wsModal.show = true"
      >
        <n-icon :component="AddOutline" />
      </n-button>
      <n-dropdown
        v-if="!narrow && wsMenuOptions.length"
        trigger="click"
        placement="bottom-end"
        :options="wsMenuOptions"
        @select="onWsMenuSelect"
      >
        <n-button quaternary circle size="small" title="Настройки пространства">
          <n-icon :component="EllipsisHorizontalOutline" />
        </n-button>
      </n-dropdown>
    </div>

    <nav class="nav">
      <n-tooltip :disabled="!collapsed" placement="right">
        <template #trigger>
          <router-link to="/" class="nav-link" active-class="nav-link-home-noop">
            <n-icon :component="HomeOutline" :size="18" />
            <span v-if="!collapsed">Главная</span>
          </router-link>
        </template>
        Главная
      </n-tooltip>
      <n-tooltip :disabled="!collapsed" placement="right">
        <template #trigger>
          <router-link to="/notes" class="nav-link">
            <n-icon :component="DocumentTextOutline" :size="18" />
            <span v-if="!collapsed">Заметки</span>
          </router-link>
        </template>
        Заметки
      </n-tooltip>
      <n-tooltip :disabled="!collapsed" placement="right">
        <template #trigger>
          <router-link to="/reminders" class="nav-link">
            <n-icon :component="AlarmOutline" :size="18" />
            <span v-if="!collapsed">Напоминания</span>
          </router-link>
        </template>
        Напоминания
      </n-tooltip>
      <n-tooltip :disabled="!collapsed" placement="right">
        <template #trigger>
          <router-link to="/milestones" class="nav-link">
            <n-icon :component="RibbonOutline" :size="18" />
            <span v-if="!collapsed">Этапы</span>
          </router-link>
        </template>
        Этапы
      </n-tooltip>
    </nav>

    <div v-if="collapsed" class="rail-sep" />

    <div v-if="!collapsed" class="proj-head">
      <n-text depth="3" strong>Проекты</n-text>
      <n-dropdown trigger="click" :options="addOptions" @select="addAtRoot">
        <n-button text size="small" title="Добавить">
          <n-icon :component="AddOutline" />
        </n-button>
      </n-dropdown>
    </div>

    <n-scrollbar v-if="!collapsed" class="tree">
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
        @start="onDragStart"
        @end="onDragEnd"
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
        @start="onDragStart"
        @end="onDragEnd"
        @change="onRootProj"
      >
        <template #item="{ element }">
          <ProjectRow :project="element" :depth="0" />
        </template>
      </draggable>
      <EmptyState
        v-if="!rootGroups.length && !ungrouped.length"
        :icon="FolderOutline"
        text="Проектов пока нет"
        size="small"
      />
    </n-scrollbar>
    <n-scrollbar v-else class="rail-scroll">
      <div class="rail">
        <SidebarRailNode
          v-for="g in rootGroups"
          :key="g.id"
          :node="g"
          kind="group"
          :active="g.id === activeRailId"
        />
        <SidebarRailNode
          v-for="p in ungrouped"
          :key="p.id"
          :node="p"
          kind="project"
          :active="p.id === activeRailId"
        />
      </div>
    </n-scrollbar>

    <SidebarFooter :mobile="mobile" :collapsed="collapsed" />

    <ProjectCreateModal v-model:show="projModalShow" @created="store.refresh()" />

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

    <ConfirmByName
      v-model:show="wsDeleteShow"
      :name="store.current?.name || ''"
      title="Удалить пространство"
      message="Пространство будет удалено со всеми проектами, досками и задачами. Действие необратимо."
      @confirm="deleteWorkspace"
    />
  </div>
</template>

<style scoped>
.sidebar {
  display: flex;
  flex-direction: column;
  height: 100%;
  /* Clip rather than scroll when the sidebar is dragged narrow. */
  overflow-x: hidden;
}
.brand {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 14px 12px 4px;
  min-width: 0;
  box-sizing: border-box;
}
/* Pin the expanded header height to the tool row's height. Otherwise the row
   collapses to the (shorter) logo height whenever the tools hide at the `narrow`
   breakpoint or the notification badge changes — and align-items:center then
   nudges the wordmark vertically (visible jitter while dragging the sidebar).
   flex-shrink:0 is essential: the sidebar is a flex column that can overflow, and
   without it the flex layout squeezes .brand down to its (tool-less) min-content,
   defeating the fixed height. */
.sidebar:not(.collapsed) .brand {
  height: 46px;
  flex-shrink: 0;
}
.ws-switch > :first-child {
  flex: 1;
  min-width: 0;
}
.brand-tools {
  margin-left: auto;
}
.sidebar.collapsed .brand {
  justify-content: center;
  padding: 14px 0 8px;
}
.sidebar.collapsed .nav {
  padding: 0 8px 8px;
  align-items: center;
}
.sidebar.collapsed .nav-link {
  justify-content: center;
  width: 40px;
  padding: 8px 0;
}
/* Thin separators between functional groups (logo / nav / projects) in the
   collapsed rail. */
.rail-sep {
  height: 1px;
  background: var(--t-border);
  margin: 6px 12px;
}
.rail-scroll {
  flex: 1;
}
.rail {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  padding: 2px 0;
}
/* Brand lockup (mark + wordmark) in the header; collapses to the mark on the rail. */
.brand-logo {
  flex: none;
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
.nav-link.router-link-active,
.nav-link.router-link-exact-active {
  background: color-mix(in srgb, var(--t-primary) 16%, transparent);
  color: var(--t-primary);
  font-weight: 600;
}
.proj-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 4px 12px;
  margin-top: 8px;
}
.nav {
  margin-top: 2px;
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
