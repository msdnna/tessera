<script setup>
import { ref, computed, watch, h } from 'vue'
import { useI18n } from 'vue-i18n'
import draggable from 'vuedraggable'
import { NButton, NScrollbar, NText, NIcon, NDropdown, NTooltip, useMessage } from 'naive-ui'
import {
  CheckmarkDoneOutline,
  DocumentTextOutline,
  FileTrayOutline,
  DocumentsOutline,
  VideocamOutline,
  RibbonOutline,
  AddOutline,
  FolderOutline,
  ChevronDownOutline,
  ChevronForwardOutline,
} from '@vicons/ionicons5'

const menuIcon = (icon) => () => h(NIcon, null, { default: () => h(icon) })
import { useRoute } from 'vue-router'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useConflictsStore } from '@/stores/conflicts'
import { useInboxStore } from '@/stores/inbox'
import { useWorkspaceMenu } from '@/composables/useWorkspaceMenu'
import { workspaceName } from '@/utils/defaultNames'
import ProjectCreatePopover from './sidebar/ProjectCreatePopover.vue'
import NewGroupRow from './sidebar/NewGroupRow.vue'
import { useSidebarDraft } from '@/composables/useSidebarDraft'
import { handleRowKeys, handleTreeNav } from '@/composables/useRowKeys'
import { useShortcuts } from '@/composables/useShortcuts'
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
import SidebarRailNode from './SidebarRailNode.vue'
import { useSidebarSize } from '@/composables/useSidebarSize'

defineProps({
  mobile: { type: Boolean, default: false },
  collapsed: { type: Boolean, default: false },
})

const { t } = useI18n()
const store = useWorkspacesStore()
const conflicts = useConflictsStore()
const inbox = useInboxStore()
const message = useMessage()
const route = useRoute()

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
  // parentGroupOf, not parent_id: it follows the cycle-broken edges the tree is
  // drawn from, so this walk ends on the same root the nodes show instead of
  // going round a ring forever (#2932).
  let parent = g ? store.parentGroupOf(g.id) : null
  while (parent) {
    g = parent
    parent = store.parentGroupOf(g.id)
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

// v2 interaction model (#2969): a project is created in a popover form at the
// «+», a group as an empty row typed in place at the end of the tree. (The
// form reports its project to the Get Started guide itself; so does the row.)
const projFormOpen = ref(false)
const addBtn = ref(null)
const draft = useSidebarDraft()
function addAtRoot(key) {
  if (key === 'project') projFormOpen.value = true
  else draft.startGroup(null)
}

// ── keyboard: ↑/↓ over the rows, G S to get in, ⇧N on «Проекты» ──
const treeBox = ref(null)
function onTreeKey(e) {
  handleTreeNav(e, treeBox.value?.closest('.sidebar') || treeBox.value)
}
function onHeadKey(e) {
  handleRowKeys(e, {
    'row.new': () => draft.startGroup(null),
    'row.menu': () => addBtn.value?.$el?.click(),
  })
}
function focusTree() {
  const root = treeBox.value
  if (!root) return
  const target =
    root.querySelector('.board-row.active[data-tree-row]') || root.querySelector('[data-tree-row]')
  target?.focus()
  target?.scrollIntoView?.({ block: 'nearest' })
}
useShortcuts({ 'nav.goSidebar': focusTree })

// The workspace menu under its name (v2): members, integrations, appearance,
// create, delete — the doors of the former tool row live here now.
const wsHead = ref(null)
const wsMenu = useWorkspaceMenu(null, () => wsHead.value)
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
        :options="wsMenu.options.value"
        @select="wsMenu.select"
      >
        <button ref="wsHead" type="button" class="ws-head" data-testid="ws-head">
          <span class="ws-head-name">{{ workspaceName(store.current) }}</span>
          <span
            v-if="conflicts.count"
            class="ws-head-dot"
            :title="t('shell.tools.integrationsConflicts', { count: conflicts.count })"
          />
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
    <div v-if="collapsed" class="rail-sep" />

    <nav class="nav">
      <!-- «Входящие» (#2999): one row in place of «Заметки» and «Напоминания».
           It opens the panel rather than a page; the count is the reminders
           that went off. -->
      <n-tooltip :disabled="!collapsed" placement="right">
        <template #trigger>
          <button
            type="button"
            class="nav-link nav-btn"
            :class="{ 'router-link-active': inbox.panelOpen }"
            data-nav="inbox"
            data-testid="nav-inbox"
            :aria-pressed="inbox.panelOpen"
            @click="inbox.toggle()"
          >
            <n-icon :component="FileTrayOutline" :size="16" />
            <span v-if="!collapsed" class="nav-label">{{ t('shell.nav.inbox') }}</span>
            <span v-if="inbox.fired && !collapsed" class="nav-count">{{ inbox.fired }}</span>
            <span v-else-if="inbox.fired" class="nav-dot" />
          </button>
        </template>
        {{ t('shell.nav.inbox') }}
      </n-tooltip>
      <n-tooltip :disabled="!collapsed" placement="right">
        <template #trigger>
          <router-link to="/" class="nav-link" active-class="nav-link-home-noop" data-nav="home">
            <n-icon :component="CheckmarkDoneOutline" :size="16" />
            <span v-if="!collapsed">{{ t('shell.nav.home') }}</span>
          </router-link>
        </template>
        {{ t('shell.nav.home') }}
      </n-tooltip>
      <n-tooltip :disabled="!collapsed" placement="right">
        <template #trigger>
          <router-link to="/documents" class="nav-link" data-nav="documents">
            <n-icon :component="DocumentsOutline" :size="16" />
            <span v-if="!collapsed">{{ t('shell.nav.documents') }}</span>
          </router-link>
        </template>
        {{ t('shell.nav.documents') }}
      </n-tooltip>
      <n-tooltip :disabled="!collapsed" placement="right">
        <template #trigger>
          <router-link to="/conferences" class="nav-link" data-nav="conferences">
            <n-icon :component="VideocamOutline" :size="16" />
            <span v-if="!collapsed">{{ t('shell.nav.conferences') }}</span>
          </router-link>
        </template>
        {{ t('shell.nav.conferences') }}
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

    <div
      v-if="!collapsed"
      class="proj-head"
      data-tree-row
      tabindex="-1"
      @keydown="onHeadKey"
      @keydown.down.prevent="onTreeKey"
      @keydown.up.prevent="onTreeKey"
    >
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
        <n-button
          ref="addBtn"
          text
          size="small"
          :title="t('shell.tree.add')"
          data-tour="proj-add"
          data-testid="tree-add"
        >
          <n-icon :component="AddOutline" />
        </n-button>
      </n-dropdown>
    </div>

    <!-- content-style, not CSS: Naive styles the content box itself, so a
         padding rule from here doesn't win — and without one the rows ran flush
         to the sidebar's right edge. The same 8px gutter as the links above. -->
    <n-scrollbar v-if="!collapsed" class="tree" content-style="padding: 0 8px">
      <div ref="treeBox" class="tree-box" @keydown="onTreeKey">
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
        <NewGroupRow v-if="draft.isGroupDraft(null)" />
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
          v-if="!rootGroups.length && !ungrouped.length && !draft.draft.value"
          :icon="FolderOutline"
          :text="t('shell.tree.empty')"
          size="small"
        />
      </div>
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

    <ProjectCreatePopover v-model:show="projFormOpen" :anchor="() => addBtn?.$el || null" />
  </div>
</template>

<style scoped>
.proj-head {
  outline: 0;
  border-radius: var(--t-r-ctl);
}
.proj-head:focus-visible {
  box-shadow: inset 0 0 0 2px var(--t-primary);
}
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
/* A fixed header height. flex-shrink:0 is essential: the sidebar is a flex
   column that can overflow, and without it the flex layout squeezes .brand down
   to its min-content, defeating the fixed height. */
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
/* Unresolved GitLab conflicts — the count is in the menu's Integrations row. */
.ws-head-dot {
  flex: none;
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: #e0922f;
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
.nav-btn {
  position: relative;
  width: 100%;
  border: 0;
  background: transparent;
  font: inherit;
  font-size: 13.5px;
  text-align: left;
  cursor: pointer;
}
.nav-label {
  flex: 1;
}
.nav-count {
  min-width: 20px;
  height: 18px;
  padding: 0 6px;
  box-sizing: border-box;
  border-radius: 999px;
  background: var(--t-accent-grad);
  color: #fff;
  font-size: 11px;
  font-weight: 700;
  line-height: 18px;
  text-align: center;
}
.nav-dot {
  position: absolute;
  top: 7px;
  right: 9px;
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--t-accent-grad);
}
.nav-link :deep(.n-icon) {
  color: var(--t-text3);
}
.nav-link:hover {
  background: var(--t-hover-side);
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
}

.empty {
  display: block;
  font-size: 12px;
  padding: 8px;
}
</style>
