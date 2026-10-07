<script setup>
// A group in the sidebar tree (v2 interaction model, #2969): the row is a
// keyboard stop (↑/↓ from the tree, ←/→ fold, F2 rename, ⇧N new project, «.»
// menu), renames in place, and its «+» opens the new-project popover at itself.
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import draggable from 'vuedraggable'
import { NIcon, NButton, useMessage } from 'naive-ui'
import {
  FolderOutline,
  ChevronForwardOutline,
  EllipsisHorizontalOutline,
  AddOutline,
  CreateOutline,
  TrashOutline,
  DocumentTextOutline,
  ColorPaletteOutline,
} from '@vicons/ionicons5'
import { groups as groupsApi } from '@/api'
import { hueGrad } from '@/utils/gradient'
import { humanizeError } from '@/utils/errors'
import { useWorkspacesStore } from '@/stores/workspaces'
import {
  moveSidebarGroup,
  moveSidebarProject,
  onDragStart,
  onDragEnd,
} from '@/composables/useSidebarDnd'
import { pressMoved } from '@/utils/dnd'
import { useLongPress } from '@/composables/useLongPress'
import { useTreeExpand } from '@/composables/useTreeExpand'
import { useSidebarDraft } from '@/composables/useSidebarDraft'
import { handleRowKeys } from '@/composables/useRowKeys'
import ProjectRow from './ProjectRow.vue'
import ProjectIcon from './ProjectIcon.vue'
import InlineEdit from './kit/InlineEdit.vue'
import ActionMenu from './kit/ActionMenu.vue'
import ProjectCreatePopover from './sidebar/ProjectCreatePopover.vue'
import AppearancePopover from './sidebar/AppearancePopover.vue'
import NewGroupRow from './sidebar/NewGroupRow.vue'

const props = defineProps({
  group: { type: Object, required: true },
  depth: { type: Number, default: 0 },
})

const { t } = useI18n()
const store = useWorkspacesStore()
const message = useMessage()
const tree = useTreeExpand()
const draft = useSidebarDraft()

// Persisted expand state; groups default open.
const expanded = computed({
  get: () => tree.isExpanded(props.group.id, true),
  set: (v) => tree.setExpanded(props.group.id, v),
})

const rowEl = ref(null)
const plusBtn = ref(null)
const moreBtn = ref(null)

// ── the row menu: «⋯», right click, long press, «.» / ⇧F10 ──
const menuOpen = ref(false)
const menuAt = ref(null)
const menuAnchor = () => menuAt.value || rowEl.value
function openMenu(at = null) {
  menuAt.value = at
  menuOpen.value = true
}
function onCtx(e) {
  if (pressMoved()) return
  openMenu({ x: e.clientX, y: e.clientY })
}
const lp = useLongPress(onCtx)

// Delete state is read by the menu rows (busy / failed), so it lives up here.
const deleting = ref(false)
const deleteFailed = ref('')

const subgroups = computed(() => store.childGroups(props.group.id))
const childProjects = computed(() => store.projectsInGroup(props.group.id))
const menuItems = computed(() => [
  { key: 'add-project', label: t('shell.group.newProject'), icon: DocumentTextOutline, keys: 'row.new' },
  { key: 'add-group', label: t('shell.group.newGroup'), icon: FolderOutline },
  { gap: true },
  { key: 'rename', label: t('shell.group.rename'), icon: CreateOutline, keys: 'row.rename' },
  { key: 'icon', label: t('shell.group.appearance'), icon: ColorPaletteOutline, keys: 'row.icon' },
  { gap: true },
  {
    key: 'delete',
    label: t('shell.group.delete'),
    icon: TrashOutline,
    danger: true,
    keys: 'row.delete',
    confirm: {
      title: t('shell.group.deleteArmed', { name: props.group.name }),
      sub: t('shell.group.deleteArmedSub'),
    },
    busy: deleting.value,
    failed: deleteFailed.value
      ? { title: t('shell.group.deleteFailed'), sub: deleteFailed.value }
      : null,
  },
])
function onMenu(key) {
  if (key === 'add-project') openProjectForm()
  else if (key === 'add-group') startSubgroup()
  else if (key === 'rename') startRename()
  else if (key === 'icon') openAppearance(true)
  else if (key === 'delete') remove()
}

const initials = computed(() => (props.group.name || '?').trim().slice(0, 2).toUpperCase())

// Update preserves all fields (handler replaces name/icon/color together).
async function applyGroup(patch) {
  try {
    await groupsApi.update(props.group.id, {
      name: props.group.name,
      icon: props.group.icon || '',
      color: props.group.color || '',
      icon_mode: props.group.icon_mode || 'badge',
      ...patch,
    })
    await store.refresh()
    return true
  } catch (e) {
    message.error(humanizeError(e))
    return false
  }
}

// Icon colouring: "badge" tints the box, "icon" leaves it transparent and tints
// the glyph instead. Groups with no colour stay neutral in badge mode.
const iconMode = computed(() => props.group.icon_mode === 'icon')
const colored = computed(() => props.group.color && props.group.color !== 'transparent')
const boxStyle = computed(() =>
  !iconMode.value && colored.value
    ? { background: hueGrad(props.group.color) }
    : { background: 'transparent' },
)
const bare = computed(() => iconMode.value || !colored.value)
const glyphColor = computed(() =>
  iconMode.value ? (colored.value ? props.group.color : 'var(--t-primary)') : '',
)

const grpModel = ref([])
const projModel = ref([])
watch(subgroups, (v) => (grpModel.value = [...v]), { immediate: true })
watch(childProjects, (v) => (projModel.value = [...v]), { immediate: true })
const onGrpChange = (evt) => moveSidebarGroup(evt, grpModel.value, props.group.id, store, message)
const onProjChange = (evt) =>
  moveSidebarProject(evt, projModel.value, props.group.id, store, message)

// ── new project: popover form at the «+» (or at the row for ⇧N / the menu) ──
const projectFormOpen = ref(false)
const projectFormAt = ref(null)
function openProjectForm(at = null) {
  projectFormAt.value = at
  projectFormOpen.value = true
}
const projectFormAnchor = () =>
  projectFormAt.value === 'plus' ? plusBtn.value?.$el || rowEl.value : rowEl.value
function onProjectCreated() {
  expanded.value = true
}

// ── new subgroup: an empty row at the end of this group's children ──
function startSubgroup() {
  expanded.value = true
  draft.startGroup(props.group.id)
}

// ── appearance ──
const appearanceOpen = ref(false)
const appearanceFromMenu = ref(false)
function openAppearance(fromMenu = false) {
  appearanceFromMenu.value = fromMenu
  appearanceOpen.value = true
}
function appearanceBack() {
  appearanceOpen.value = false
  openMenu(menuAt.value)
}

// ── rename in place ──
const renaming = ref(false)
const nameEdit = ref('')
const renameBusy = ref(false)
function startRename() {
  nameEdit.value = props.group.name
  renaming.value = true
}
async function commitRename(value, { via }) {
  const n = value.trim()
  if (!n || n === props.group.name) {
    // An empty name is not saved: ↵ does nothing, a click away gives up.
    if (!n && via === 'enter') return
    renaming.value = false
    return
  }
  renameBusy.value = true
  const ok = await applyGroup({ name: n })
  renameBusy.value = false
  if (ok) {
    renaming.value = false
    rowEl.value?.focus()
  }
}
function cancelRename() {
  renaming.value = false
  rowEl.value?.focus()
}

// ── delete: irreversible (subgroups go, projects lose the group) → two steps ──
async function remove() {
  deleting.value = true
  deleteFailed.value = ''
  try {
    await groupsApi.remove(props.group.id)
    menuOpen.value = false
    await store.refresh()
  } catch (e) {
    deleteFailed.value = humanizeError(e)
  } finally {
    deleting.value = false
  }
}

// ── keys of the focused row ──
function onRowKey(e) {
  if (renaming.value) return
  if (e.key === 'ArrowRight' && !e.metaKey && !e.ctrlKey && !e.altKey) {
    e.preventDefault()
    if (!expanded.value) expanded.value = true
    return
  }
  if (e.key === 'ArrowLeft' && !e.metaKey && !e.ctrlKey && !e.altKey) {
    e.preventDefault()
    if (expanded.value) expanded.value = false
    else rowEl.value?.parentElement?.parentElement?.closest('.group-node')?.querySelector('[data-tree-row]')?.focus()
    return
  }
  handleRowKeys(e, {
    'row.open': () => (expanded.value = !expanded.value),
    'row.rename': startRename,
    'row.renameR': startRename,
    'row.new': () => openProjectForm(),
    'row.menu': () => openMenu(),
    'row.menuF10': () => openMenu(),
    'row.icon': () => openAppearance(false),
    'row.delete': () => openMenu(),
  })
}
</script>

<template>
  <!-- data-tour-group is this group's identity for the Get Started guide, the
       twin of data-tour-project on a project block (#2778): the steps about the
       group the user just created scope their anchor to it, and it doubles as
       the "address" a dragged project reports (its closest() group node), which
       is how the guide tells an actual move from a mid-drag frame. On the node,
       so a project in a subgroup reports the subgroup, not its parent. -->
  <div class="group-node" :data-tour-group="group.id">
    <div
      ref="rowEl"
      class="row group-row"
      :class="{ ctx: menuOpen, editing: renaming }"
      data-tour="group-row"
      data-tree-row
      tabindex="-1"
      :aria-expanded="expanded"
      @keydown="onRowKey"
      @contextmenu.prevent.stop="onCtx"
      @touchstart.passive="lp.start"
      @touchend="lp.cancel"
      @touchcancel="lp.cancel"
    >
      <n-icon
        class="chev"
        :class="{ open: expanded }"
        :component="ChevronForwardOutline"
        @click="expanded = !expanded"
      />
      <span
        class="gicon"
        :class="{ 'gicon-bare': bare }"
        :style="boxStyle"
        @click="expanded = !expanded"
      >
        <ProjectIcon
          v-if="group.icon"
          :icon="group.icon"
          :initials="initials"
          :size="15"
          :color="glyphColor"
        />
        <n-icon v-else :component="FolderOutline" :size="16" :color="glyphColor || undefined" />
      </span>
      <InlineEdit
        v-if="renaming"
        v-model="nameEdit"
        :busy="renameBusy"
        :hint-left="-40"
        @commit="commitRename"
        @cancel="cancelRename"
      />
      <span v-else class="name" @click="expanded = !expanded" @dblclick="startRename">
        {{ group.name }}
      </span>
      <template v-if="!renaming">
        <n-button
          ref="plusBtn"
          class="hover-btn"
          text
          size="tiny"
          :class="{ open: projectFormOpen && projectFormAt === 'plus' }"
          :title="t('shell.group.newProject')"
          data-testid="group-add-project"
          @click.stop="openProjectForm('plus')"
        >
          <n-icon :component="AddOutline" />
        </n-button>
        <n-button
          ref="moreBtn"
          class="hover-btn"
          text
          size="tiny"
          :class="{ open: menuOpen && !menuAt }"
          data-testid="group-menu"
          @click.stop="openMenu()"
        >
          <n-icon :component="EllipsisHorizontalOutline" />
        </n-button>
      </template>
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
        @start="onDragStart"
        @end="onDragEnd"
        @change="onGrpChange"
      >
        <template #item="{ element }">
          <SidebarNode :group="element" :depth="depth + 1" />
        </template>
      </draggable>
      <NewGroupRow v-if="draft.isGroupDraft(group.id)" :parent-id="group.id" />
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
        @start="onDragStart"
        @end="onDragEnd"
        @change="onProjChange"
      >
        <template #item="{ element }">
          <ProjectRow :project="element" :depth="depth + 1" />
        </template>
      </draggable>
    </div>

    <ActionMenu
      v-model:show="menuOpen"
      :anchor="menuAnchor"
      :placement="menuAt ? 'point' : 'right-start'"
      :header="group.name"
      :items="menuItems"
      testid="group-actions"
      @select="onMenu"
    />
    <ProjectCreatePopover
      v-model:show="projectFormOpen"
      :anchor="projectFormAnchor"
      :group-id="group.id"
      :group-name="group.name"
      @created="onProjectCreated"
    />
    <AppearancePopover
      v-model:show="appearanceOpen"
      :anchor="() => rowEl"
      :subtitle="group.name"
      :icon="group.icon"
      :color="group.color"
      :mode="group.icon_mode || 'badge'"
      :initials="initials"
      fallback-folder
      transparent-default
      :escape-back="appearanceFromMenu"
      @escape="appearanceBack"
      @update:icon="applyGroup({ icon: $event })"
      @update:color="applyGroup({ color: $event })"
      @update:mode="applyGroup({ icon_mode: $event })"
    />
  </div>
</template>

<style scoped>
.row {
  display: flex;
  align-items: center;
  gap: 6px;
  min-height: 32px;
  box-sizing: border-box;
  padding: 0 8px;
  border-radius: var(--t-r-ctl);
  cursor: pointer;
  font-size: 13.5px;
}
.row:hover {
  background: var(--t-hover-side);
}
.hover-btn {
  opacity: 0;
}
.group-row:hover .hover-btn {
  opacity: 1;
}
.chev {
  width: 14px;
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: var(--t-text3);
  font-size: 12px;
  transition: transform 0.15s;
}
.chev.open {
  transform: rotate(90deg);
}
.gicon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  border-radius: 5px;
  color: #fff;
  flex: none;
}
.gicon-bare {
  color: var(--t-text2);
}
/* v2 (#2953): a group reads as a section label over its projects. */
.group-row {
  min-height: 28px;
}
.name {
  flex: 1;
  font-size: 12.5px;
  font-weight: 600;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  color: var(--t-text3);
}
/* Nesting is shown by indenting the whole subtree, so the drag placeholder
   (.sb-ghost) inside a group is visually indented too — making it clear the
   item will land inside the group. v2 (#2953): indent only, no guide line. */
.children {
  margin-left: 14px;
}
.row {
  outline: 0;
}
.row:focus-visible {
  background: var(--t-hover-side);
  box-shadow: inset 0 0 0 2px var(--t-primary);
}
.row.ctx {
  background: var(--t-hover);
  box-shadow: var(--t-sh-card);
}
.row.ctx .hover-btn,
.row:focus-visible .hover-btn {
  opacity: 1;
}
.hover-btn.open {
  opacity: 1;
  background: var(--t-active);
  color: var(--t-primary);
  border-radius: 7px;
}
.row.editing {
  padding-right: 4px;
}
</style>
