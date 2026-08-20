<script setup>
import { ref, computed, watch, nextTick, h } from 'vue'
import draggable from 'vuedraggable'
import {
  NIcon,
  NButton,
  NInput,
  NDropdown,
  NPopover,
  NPopconfirm,
  NModal,
  NCard,
  useMessage,
} from 'naive-ui'
import {
  FolderOutline,
  ChevronForwardOutline,
  EllipsisHorizontalOutline,
  AddOutline,
  CreateOutline,
  TrashOutline,
  DocumentTextOutline,
} from '@vicons/ionicons5'

const menuIcon = (icon) => () => h(NIcon, null, { default: () => h(icon) })
const dangerIcon = (icon) => () => h(NIcon, { color: '#e0533d' }, { default: () => h(icon) })
import { workspaces as wsApi, groups as groupsApi } from '@/api'
import { hueGrad } from '@/utils/gradient'
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
import { useTourStore } from '@/stores/tour'
import ProjectRow from './ProjectRow.vue'
import ProjectCreateModal from './ProjectCreateModal.vue'
import ProjectIcon from './ProjectIcon.vue'
import IconColorPicker from './IconColorPicker.vue'

const props = defineProps({
  group: { type: Object, required: true },
  depth: { type: Number, default: 0 },
})

const store = useWorkspacesStore()
const message = useMessage()
const tree = useTreeExpand()
const tour = useTourStore()

// Persisted expand state; groups default open.
const expanded = computed({
  get: () => tree.isExpanded(props.group.id, true),
  set: (v) => tree.setExpanded(props.group.id, v),
})
const renaming = ref(false)
const nameEdit = ref('')
const renameInput = ref(null)
const settingsShow = ref(false)

// right-click context menu
const ctxShow = ref(false)
const ctxX = ref(0)
const ctxY = ref(0)
const ctxOptions = [
  { label: 'Новый проект', key: 'add-project', icon: menuIcon(DocumentTextOutline) },
  { label: 'Новая группа', key: 'add-group', icon: menuIcon(FolderOutline) },
  { type: 'divider', key: 'd1' },
  { label: 'Переименовать', key: 'rename', icon: menuIcon(CreateOutline) },
  {
    label: 'Удалить группу',
    key: 'delete',
    icon: dangerIcon(TrashOutline),
    props: { style: 'color:#e0533d' },
  },
]
function onCtx(e) {
  if (pressMoved()) return
  ctxShow.value = false
  ctxX.value = e.clientX
  ctxY.value = e.clientY
  nextTick(() => (ctxShow.value = true))
}
const lp = useLongPress(onCtx)
function onCtxSelect(key) {
  ctxShow.value = false
  if (key === 'add-project') onAdd('project')
  else if (key === 'add-group') onAdd('group')
  else if (key === 'rename') startRename()
  else if (key === 'delete') confirmDelete.value = true
}

// Context-menu delete needs its own confirmation (the settings-popover path uses
// an inline n-popconfirm; the dropdown menu can't host one, so it opens a modal).
const confirmDelete = ref(false)

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
  } catch (e) {
    message.error(e.message)
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
  { label: 'Проект', key: 'project', icon: menuIcon(DocumentTextOutline) },
  { label: 'Группа', key: 'group', icon: menuIcon(FolderOutline) },
]

// Projects are named in a modal (see ProjectCreateModal); groups keep the
// create-then-rename flow.
const projModalShow = ref(false)

async function onAdd(key) {
  if (key === 'project') {
    projModalShow.value = true
    return
  }
  try {
    // The new group is reported to the guide (as the project modal reports its
    // project) so the steps that follow point at *this* group and not at the
    // first one in the tree (#2778 rework).
    const res = await wsApi.createGroup(store.currentId, {
      name: 'Группа',
      parent_id: props.group.id,
    })
    tour.noteCreated({ groupId: res.data?.id })
    await store.refresh()
    expanded.value = true
  } catch (e) {
    message.error(e.message)
  }
}

async function onProjectCreated() {
  await store.refresh()
  expanded.value = true
}

function startRename() {
  settingsShow.value = false
  nameEdit.value = props.group.name
  renaming.value = true
  nextTick(() => renameInput.value?.focus())
}
async function remove() {
  confirmDelete.value = false
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
  await applyGroup({ name: n })
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
      class="row group-row"
      data-tour="group-row"
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
          <IconColorPicker
            :icon="group.icon"
            :color="group.color"
            :mode="group.icon_mode || 'badge'"
            :initials="initials"
            fallback-folder
            transparent-default
            @update:icon="applyGroup({ icon: $event })"
            @update:color="applyGroup({ color: $event })"
            @update:mode="applyGroup({ icon_mode: $event })"
          />
          <n-button type="primary" ghost size="small" block @click="startRename">
            <template #icon><n-icon :component="CreateOutline" /></template>
            Переименовать
          </n-button>
          <n-popconfirm
            :positive-button-props="{ type: 'error' }"
            positive-text="Удалить"
            @positive-click="remove"
          >
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
        @start="onDragStart"
        @end="onDragEnd"
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
        @start="onDragStart"
        @end="onDragEnd"
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

    <ProjectCreateModal
      v-model:show="projModalShow"
      :group-id="group.id"
      @created="onProjectCreated"
    />

    <n-modal v-model:show="confirmDelete">
      <n-card
        :title="`Удалить группу «${group.name}»?`"
        style="max-width: 400px"
        role="dialog"
        :bordered="false"
      >
        <p class="confirm-msg">
          Подгруппы удалятся, проекты станут без группы. Действие необратимо.
        </p>
        <template #footer>
          <div class="confirm-actions">
            <n-button size="small" @click="confirmDelete = false">Отмена</n-button>
            <n-button type="error" size="small" @click="remove()">
              <template #icon><n-icon :component="TrashOutline" /></template>
              Удалить
            </n-button>
          </div>
        </template>
      </n-card>
    </n-modal>
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
.confirm-msg {
  margin: 0;
  color: var(--t-text2);
  font-size: 13px;
  line-height: 1.5;
}
.confirm-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
</style>
