<script setup>
import { ref, computed, watch, h } from 'vue'
import { useI18n } from 'vue-i18n'
import draggable from 'vuedraggable'
import {
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
  DocumentsOutline,
  VideocamOutline,
  AlarmOutline,
  RibbonOutline,
  AddOutline,
  FolderOutline,
  TrashOutline,
  ChevronDownOutline,
  ChevronForwardOutline,
  ColorPaletteOutline,
} from '@vicons/ionicons5'

const menuIcon = (icon) => () => h(NIcon, null, { default: () => h(icon) })
const dangerIcon = (icon) => () => h(NIcon, { color: '#e0533d' }, { default: () => h(icon) })
import { useRoute, useRouter } from 'vue-router'
import { workspaces as wsApi } from '@/api'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useAuthStore } from '@/stores/auth'
import { useTourStore } from '@/stores/tour'
import { workspaceName } from '@/utils/defaultNames'
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
import WorkspaceAppearanceModal from './rail/WorkspaceAppearanceModal.vue'
import { useSidebarSize } from '@/composables/useSidebarSize'

defineProps({
  mobile: { type: Boolean, default: false },
  collapsed: { type: Boolean, default: false },
})

const { t } = useI18n()
const store = useWorkspacesStore()
const auth = useAuthStore()
const tour = useTourStore()
const message = useMessage()
const route = useRoute()
const router = useRouter()

const { toggle: toggleSidebar } = useSidebarSize()
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

// Computed, not a module constant: the labels have to follow a language switch
// (a frozen table would keep the language of the first render).
const addOptions = computed(() => [
  { label: t('shell.tree.addProject'), key: 'project', icon: menuIcon(DocumentTextOutline) },
  { label: t('shell.tree.addGroup'), key: 'group', icon: menuIcon(FolderOutline) },
])

// Projects are named in a modal (the name decides the URL address, which is
// assigned once); groups keep the create-then-rename flow.
const projModalShow = ref(false)

// The project modal reports its new project so the Get Started guide can point
// its next step («+» to add a board) at that row rather than the first (#2753).
function onProjectCreated(project) {
  tour.noteCreated({ projectId: project?.id })
  store.refresh()
}

async function addAtRoot(key) {
  if (key === 'project') {
    projModalShow.value = true
    return
  }
  try {
    // Same as onProjectCreated above: the guide's «Группа создана» and «перетащите
    // проект в группу» steps have to point at the group this click just made, not
    // at whichever group happens to be first in the tree (#2778 rework).
    // The seed name is localised: a group is renamed right after it appears (the
    // guide's next step is exactly that), so it reads in the language the person
    // is working in rather than in whichever one the server was seeded with.
    const res = await wsApi.createGroup(store.currentId, { name: t('shell.group.defaultName') })
    tour.noteCreated({ groupId: res.data?.id })
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
    await store.createWorkspace(n)
    wsModal.value.show = false
    wsModal.value.value = ''
  } catch (e) {
    message.error(e.message)
  }
}

const isOwner = computed(() => !!store.current && store.current.owner_id === auth.user?.id)
// The workspace menu under its name: its tile (owners/admins), a new one, and
// — for the owner — deleting it.
const wsMenuOptions = computed(() => [
  ...(store.canManage
    ? [
        {
          label: t('shell.rail.appearance'),
          key: 'appearance',
          icon: menuIcon(ColorPaletteOutline),
        },
      ]
    : []),
  { label: t('shell.workspace.create'), key: 'create', icon: menuIcon(AddOutline) },
  ...(isOwner.value
    ? [
        {
          label: t('shell.workspace.delete'),
          key: 'delete',
          icon: dangerIcon(TrashOutline),
          props: { style: 'color:#e0533d' },
        },
      ]
    : []),
])
const appearanceShow = ref(false)
const wsDeleteShow = ref(false)
function onWsMenuSelect(key) {
  if (key === 'appearance') appearanceShow.value = true
  else if (key === 'create') wsModal.value.show = true
  else if (key === 'delete') {
    if (store.list.length <= 1) {
      message.warning(t('shell.workspace.deleteLast'))
      return
    }
    wsDeleteShow.value = true
  }
}
async function deleteWorkspace() {
  try {
    await store.removeWorkspace(store.currentId)
    message.success(t('shell.workspace.deleted'))
    router.push('/') // the tree fully changed — land on Home
  } catch (e) {
    message.error(e.message)
  }
}
</script>

<template>
  <div class="sidebar" data-testid="sidebar" :class="{ collapsed, 'sb-dragging': sidebarDragging }">
    <div class="brand">
      <!-- The logo and the workspace tiles live on the rail now (v2, #2955);
           the header names the current workspace and holds its menu. -->
      <n-dropdown
        v-if="!collapsed"
        trigger="click"
        placement="bottom-start"
        :options="wsMenuOptions"
        @select="onWsMenuSelect"
      >
        <button type="button" class="ws-head" data-testid="ws-head">
          <span class="ws-head-name">{{ workspaceName(store.current) }}</span>
          <n-icon :component="ChevronDownOutline" :size="13" class="ws-head-chev" />
        </button>
      </n-dropdown>
      <button
        v-else
        type="button"
        class="ws-expand"
        :title="t('shell.layout.expand')"
        @click="toggleSidebar"
      >
        <n-icon :component="ChevronForwardOutline" :size="16" />
      </button>
    </div>
    <!-- Tools get a row of their own under the name, so a long workspace name
         isn't squeezed; when the rail is collapsed (desktop) they move to the
         header instead. -->
    <div v-if="mobile || !collapsed" class="tools-row">
      <WorkspaceTools placement="bottom-start" />
    </div>

    <div v-if="collapsed" class="rail-sep" />

    <nav class="nav">
      <n-tooltip :disabled="!collapsed" placement="right">
        <template #trigger>
          <router-link to="/" class="nav-link" active-class="nav-link-home-noop" data-nav="home">
            <n-icon :component="HomeOutline" :size="16" />
            <span v-if="!collapsed">{{ t('shell.nav.home') }}</span>
          </router-link>
        </template>
        {{ t('shell.nav.home') }}
      </n-tooltip>
      <n-tooltip :disabled="!collapsed" placement="right">
        <template #trigger>
          <router-link to="/notes" class="nav-link" data-nav="notes">
            <n-icon :component="DocumentTextOutline" :size="16" />
            <span v-if="!collapsed">{{ t('shell.nav.notes') }}</span>
          </router-link>
        </template>
        {{ t('shell.nav.notes') }}
      </n-tooltip>
      <n-tooltip :disabled="!collapsed" placement="right">
        <template #trigger>
          <router-link to="/documents" class="nav-link" data-nav="documents">
            <n-icon :component="DocumentsOutline" :size="16" />
            <span v-if="!collapsed" class="nav-label">
              {{ t('shell.nav.documents')
              }}<sup class="nav-badge">{{ t('shell.nav.newBadge') }}</sup>
            </span>
          </router-link>
        </template>
        {{ t('shell.nav.documents') }}
      </n-tooltip>
      <n-tooltip :disabled="!collapsed" placement="right">
        <template #trigger>
          <router-link to="/conferences" class="nav-link" data-nav="conferences">
            <n-icon :component="VideocamOutline" :size="16" />
            <span v-if="!collapsed" class="nav-label">
              {{ t('shell.nav.conferences')
              }}<sup class="nav-badge">{{ t('shell.nav.newBadge') }}</sup>
            </span>
          </router-link>
        </template>
        {{ t('shell.nav.conferences') }}
      </n-tooltip>
      <n-tooltip :disabled="!collapsed" placement="right">
        <template #trigger>
          <router-link to="/reminders" class="nav-link" data-nav="reminders">
            <n-icon :component="AlarmOutline" :size="16" />
            <span v-if="!collapsed">{{ t('shell.nav.reminders') }}</span>
          </router-link>
        </template>
        {{ t('shell.nav.reminders') }}
      </n-tooltip>
      <n-tooltip :disabled="!collapsed" placement="right">
        <template #trigger>
          <router-link to="/milestones" class="nav-link" data-nav="milestones">
            <n-icon :component="RibbonOutline" :size="16" />
            <span v-if="!collapsed">{{ t('shell.nav.milestones') }}</span>
          </router-link>
        </template>
        {{ t('shell.nav.milestones') }}
      </n-tooltip>
      <!-- No «Помощь» item here (#2792): the help centre is a modal opened from
           the «Помощь» menu at the bottom of the sidebar, not a destination that
           replaces the board the reader is working on. -->
    </nav>

    <div v-if="collapsed" class="rail-sep" />

    <div v-if="!collapsed" class="proj-head">
      <n-text depth="3" strong>{{ t('shell.tree.projects') }}</n-text>
      <!-- node-props tags each option for the Get Started guide (#2753): naive
           merges them into the option node, so the tour can anchor on a menu
           item the same way it anchors on any other element. -->
      <n-dropdown
        trigger="click"
        :options="addOptions"
        :node-props="(o) => ({ 'data-tour': `menu-${o.key}` })"
        @select="addAtRoot"
      >
        <n-button text size="small" :title="t('shell.tree.add')" data-tour="proj-add">
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
        :text="t('shell.tree.empty')"
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

    <ProjectCreateModal v-model:show="projModalShow" @created="onProjectCreated" />

    <n-modal v-model:show="wsModal.show">
      <n-card :title="t('shell.workspace.create')" style="max-width: 360px" role="dialog">
        <n-input
          v-model:value="wsModal.value"
          :placeholder="t('shell.workspace.namePlaceholder')"
          @keyup.enter="createWorkspace"
        />
        <template #footer>
          <n-button type="primary" @click="createWorkspace">{{
            t('common.action.create')
          }}</n-button>
        </template>
      </n-card>
    </n-modal>

    <WorkspaceAppearanceModal v-model:show="appearanceShow" :workspace="store.current" />

    <ConfirmByName
      v-model:show="wsDeleteShow"
      :name="workspaceName(store.current)"
      :title="t('shell.workspace.delete')"
      :message="t('shell.workspace.deleteMessage')"
      @confirm="deleteWorkspace"
    />
  </div>
</template>

<style scoped>
.sidebar {
  display: flex;
  flex-direction: column;
  height: 100%;
  /* Clip rather than scroll while the sidebar animates between widths. */
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
   collapses to the (shorter) logo height whenever the notification badge
   changes — and align-items:center then nudges the wordmark vertically.
   flex-shrink:0 is essential: the sidebar is a flex column that can overflow, and
   without it the flex layout squeezes .brand down to its (tool-less) min-content,
   defeating the fixed height. */
.sidebar:not(.collapsed) .brand {
  height: 46px;
  flex-shrink: 0;
}
/* Current workspace, v2 header (#2955): its name and the menu chevron. */
.ws-head {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  height: 32px;
  padding: 0 8px;
  margin-left: -4px;
  border: none;
  border-radius: var(--t-r-ctl);
  background: transparent;
  color: var(--t-text1);
  font: inherit;
  font-size: 14.5px;
  font-weight: 650;
  cursor: pointer;
}
.ws-head:hover {
  background: var(--t-hover-side);
}
.ws-head-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.ws-head-chev {
  flex: none;
  color: var(--t-text3);
}
.ws-expand {
  width: 40px;
  height: 36px;
  display: flex;
  align-items: center;
  justify-content: center;
  border: none;
  border-radius: var(--t-r-ctl);
  background: transparent;
  color: var(--t-text3);
  cursor: pointer;
}
.ws-expand:hover {
  background: var(--t-hover-side);
  color: var(--t-text1);
}
.tools-row {
  display: flex;
  padding: 0 8px 6px;
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
/* Gap between functional groups (logo / nav / projects) in the collapsed rail —
   v2 separates them with space, not a line (#2953). */
.rail-sep {
  height: 12px;
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
.nav {
  display: flex;
  flex-direction: column;
  padding: 0 8px 8px;
}
.nav-link {
  display: flex;
  align-items: center;
  gap: 10px;
  height: 32px;
  padding: 0 8px;
  border-radius: var(--t-r-ctl);
  font-size: 13.5px;
  color: var(--t-text2);
  text-decoration: none;
}
.nav-link :deep(.n-icon) {
  color: var(--t-text3);
}
.nav-link:hover {
  background: var(--t-hover-side);
}
.nav-label {
  display: inline-flex;
  align-items: flex-start;
}
.nav-badge {
  margin-left: 3px;
  padding: 1px 5px;
  border-radius: 999px;
  font-size: 9px;
  line-height: 1.3;
  font-weight: 700;
  letter-spacing: 0.03em;
  text-transform: lowercase;
  color: var(--t-primary);
  background: color-mix(in srgb, var(--t-primary) 16%, transparent);
}
/* Active item: the v2 active level (accent mixed over the sidebar) and the
   icon painted with the accent gradient. */
.nav-link.router-link-active,
.nav-link.router-link-exact-active {
  background: var(--t-active);
  color: var(--t-text1);
  font-weight: 600;
}
.nav-link.router-link-active :deep([fill='currentColor']),
.nav-link.router-link-exact-active :deep([fill='currentColor']) {
  fill: url(#t-accent-grad-svg);
}
.nav-link.router-link-active :deep([stroke='currentColor']),
.nav-link.router-link-exact-active :deep([stroke='currentColor']) {
  stroke: url(#t-accent-grad-svg);
}
.proj-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 28px;
  padding: 0 16px;
  margin-top: 14px;
}
.proj-head :deep(.n-text) {
  font-size: 11.5px;
  font-weight: 600;
  letter-spacing: 0.05em;
  text-transform: uppercase;
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
