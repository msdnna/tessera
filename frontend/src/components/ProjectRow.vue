<script setup>
import { ref, computed, nextTick, watch, onBeforeUnmount, h } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import {
  NIcon,
  NButton,
  NInput,
  NPopover,
  NPopconfirm,
  NText,
  NDropdown,
  useMessage,
} from 'naive-ui'
import {
  GridOutline,
  EllipsisHorizontalOutline,
  ChevronForwardOutline,
  AddOutline,
  CreateOutline,
  TrashOutline,
  OpenOutline,
} from '@vicons/ionicons5'

const menuIcon = (icon) => () => h(NIcon, null, { default: () => h(icon) })
// Red icon for destructive menu entries (the label is reddened via the option's
// props.style; NIcon needs the colour set explicitly — it doesn't inherit it).
const dangerIcon = (icon) => () => h(NIcon, { color: '#e0533d' }, { default: () => h(icon) })
import { projects as projApi, boards as boardsApi } from '@/api'
import { hueGrad } from '@/utils/gradient'
import { useWorkspacesStore } from '@/stores/workspaces'
import ProjectIcon from './ProjectIcon.vue'
import IconColorPicker from './IconColorPicker.vue'
import ConfirmByName from './ConfirmByName.vue'
import { pressMoved } from '@/utils/dnd'
import { useLongPress } from '@/composables/useLongPress'
import { useTreeExpand } from '@/composables/useTreeExpand'

const props = defineProps({
  project: { type: Object, required: true },
  depth: { type: Number, default: 0 },
})

const store = useWorkspacesStore()
const router = useRouter()
const route = useRoute()
const message = useMessage()
const tree = useTreeExpand()

// Persisted expand state; projects default closed.
const expanded = computed({
  get: () => tree.isExpanded(props.project.id, false),
  set: (v) => tree.setExpanded(props.project.id, v),
})
const addingBoard = ref(false)
const newBoardName = ref('')
const renaming = ref(false)
const nameEdit = ref('')
const renameInput = ref(null)
const boardInput = ref(null)
const settingsShow = ref(false)

// board inline edit
const editingBoardId = ref(null)
const boardNameEdit = ref('')
const boardEditInput = ref(null)
function startBoardRename(b) {
  editingBoardId.value = b.id
  boardNameEdit.value = b.name
  nextTick(() => boardEditInput.value?.focus?.())
}
async function commitBoardRename(b) {
  editingBoardId.value = null
  const n = boardNameEdit.value.trim()
  if (!n || n === b.name) return
  try {
    await boardsApi.update(b.id, { name: n })
    await store.loadBoards(props.project.id)
  } catch (e) {
    message.error(e.message)
  }
}
async function removeBoard(b) {
  try {
    await boardsApi.remove(b.id)
    await store.loadBoards(props.project.id)
  } catch (e) {
    message.error(e.message)
  }
}

const boards = computed(() => store.boardsByProject[props.project.id] || [])
const initials = computed(() => (props.project.name || '?').trim().slice(0, 2).toUpperCase())

async function toggle() {
  expanded.value = !expanded.value
  if (expanded.value && !store.boardsByProject[props.project.id]) {
    await store.loadBoards(props.project.id)
  }
}

// inline rename — click-outside saves if changed, else cancels
function startRename() {
  settingsShow.value = false
  nameEdit.value = props.project.name
  renaming.value = true
  nextTick(() => renameInput.value?.focus())
}
async function commitRename() {
  renaming.value = false
  const n = nameEdit.value.trim()
  if (!n || n === props.project.name) return
  await updateField({ name: n })
}

// settings apply immediately (mirrors the column header pattern)
async function updateField(patch) {
  try {
    await projApi.update(props.project.id, {
      name: props.project.name,
      color: props.project.color || '',
      icon: props.project.icon || '',
      group_id: props.project.group_id || null,
      ...patch,
    })
    await store.refresh()
  } catch (e) {
    message.error(e.message)
  }
}
const confirmDelete = ref(false)
function remove() {
  confirmDelete.value = true
}
async function doRemove() {
  try {
    await projApi.remove(props.project.id)
    await store.refresh()
  } catch (e) {
    message.error(e.message)
  }
}

// right-click menus (project row + board rows)
const pcShow = ref(false)
const pcX = ref(0)
const pcY = ref(0)
const pcOptions = [
  { label: 'Новая доска', key: 'add-board', icon: menuIcon(AddOutline) },
  { type: 'divider', key: 'd1' },
  { label: 'Переименовать', key: 'rename', icon: menuIcon(CreateOutline) },
  { label: 'Удалить проект', key: 'delete', icon: dangerIcon(TrashOutline), props: { style: 'color:#e0533d' } },
]
function onProjectCtx(e) {
  if (pressMoved()) return
  pcShow.value = false
  pcX.value = e.clientX
  pcY.value = e.clientY
  nextTick(() => (pcShow.value = true))
}
function onProjectCtxSelect(key) {
  pcShow.value = false
  if (key === 'add-board') startAddBoard()
  else if (key === 'rename') startRename()
  else if (key === 'delete') remove()
}

const bcShow = ref(false)
const bcX = ref(0)
const bcY = ref(0)
const bcTarget = ref(null)
const bcOptions = [
  { label: 'Открыть', key: 'open', icon: menuIcon(OpenOutline) },
  { label: 'Переименовать', key: 'rename', icon: menuIcon(CreateOutline) },
  { label: 'Удалить доску', key: 'delete', icon: dangerIcon(TrashOutline), props: { style: 'color:#e0533d' } },
]
function onBoardCtx(e, b) {
  bcTarget.value = b
  bcShow.value = false
  bcX.value = e.clientX
  bcY.value = e.clientY
  nextTick(() => (bcShow.value = true))
}
function onBoardCtxSelect(key) {
  const b = bcTarget.value
  bcShow.value = false
  if (!b) return
  if (key === 'open') router.push(`/project/${props.project.slug}/board/${b.slug}`)
  else if (key === 'rename') startBoardRename(b)
  else if (key === 'delete') removeBoard(b)
}

// Touch long-press → context menus (the native contextmenu is unreliable on the
// draggable rows inside the mobile drawer).
const lpProj = useLongPress(onProjectCtx)
let bTimer = null
function bStart(e, b) {
  const t = e.touches && e.touches[0]
  if (!t) return
  const x = t.clientX
  const y = t.clientY
  clearTimeout(bTimer)
  bTimer = setTimeout(() => {
    if (!pressMoved()) onBoardCtx({ clientX: x, clientY: y }, b)
  }, 450)
}
function bCancel() {
  clearTimeout(bTimer)
}
onBeforeUnmount(bCancel)

// Load boards whenever the project is (or becomes) expanded — covers both the
// initial mount and expand state restored from persistence AFTER mount (which
// doesn't go through toggle(), so an onMounted-only check missed it and left
// "нет досок" until a manual collapse+expand).
watch(
  expanded,
  (v) => {
    if (v && !store.boardsByProject[props.project.id]) store.loadBoards(props.project.id)
  },
  { immediate: true },
)

// inline board creation via the "+" button
function startAddBoard() {
  expanded.value = true
  addingBoard.value = true
  newBoardName.value = ''
  nextTick(() => boardInput.value?.focus())
}
async function addBoard() {
  const n = newBoardName.value.trim()
  // Clear + close before await so the @blur on input removal doesn't duplicate.
  newBoardName.value = ''
  addingBoard.value = false
  if (!n) return
  try {
    await projApi.createBoard(props.project.id, { name: n })
    await store.loadBoards(props.project.id)
    expanded.value = true
  } catch (e) {
    message.error(e.message)
  }
}
</script>

<template>
  <div class="project-block">
    <div
      class="row project-row"
      @contextmenu.prevent.stop="onProjectCtx"
      @touchstart.passive="lpProj.start"
      @touchend="lpProj.cancel"
      @touchcancel="lpProj.cancel"
    >
      <n-icon
        class="chev"
        :class="{ open: expanded }"
        :component="ChevronForwardOutline"
        @click="toggle"
      />
      <span
        class="picon"
        :class="{ 'picon-bare': project.color === 'transparent' }"
        :style="{
          background:
            project.color === 'transparent'
              ? 'transparent'
              : hueGrad(project.color || 'var(--t-primary)'),
        }"
        @click="toggle"
      >
        <ProjectIcon :icon="project.icon" :initials="initials" :size="13" />
      </span>
      <n-input
        v-if="renaming"
        ref="renameInput"
        v-model:value="nameEdit"
        size="tiny"
        @keyup.enter="commitRename"
        @blur="commitRename"
      />
      <span v-else class="name" @click="toggle" @dblclick="startRename">{{ project.name }}</span>

      <n-button
        class="hover-btn"
        text
        size="tiny"
        title="Добавить доску"
        @click.stop="startAddBoard"
      >
        <n-icon :component="AddOutline" />
      </n-button>
      <n-popover v-model:show="settingsShow" trigger="click" placement="right-start">
        <template #trigger>
          <n-button class="hover-btn" text size="tiny" @click.stop>
            <n-icon :component="EllipsisHorizontalOutline" />
          </n-button>
        </template>
        <div class="settings">
          <IconColorPicker
            :icon="project.icon"
            :color="project.color"
            :initials="initials"
            allow-upload
            @update:icon="updateField({ icon: $event })"
            @update:color="updateField({ color: $event })"
          />
          <div class="action-row">
            <n-button type="primary" ghost size="small" @click="startRename">
              <template #icon><n-icon :component="CreateOutline" /></template>
              Переименовать
            </n-button>
            <n-button type="error" ghost size="small" @click="remove">
              <template #icon><n-icon :component="TrashOutline" /></template>
              Удалить
            </n-button>
          </div>
        </div>
      </n-popover>
    </div>

    <div v-if="expanded" class="boards">
      <div
        v-for="b in boards"
        :key="b.id"
        class="row board-row"
        :class="{ active: route.params.projectSlug === project.slug && route.params.boardSlug === b.slug }"
        @click="editingBoardId !== b.id && router.push(`/project/${project.slug}/board/${b.slug}`)"
        @contextmenu.prevent.stop="onBoardCtx($event, b)"
        @touchstart.passive="bStart($event, b)"
        @touchend="bCancel"
        @touchcancel="bCancel"
      >
        <span class="chev-spacer" />
        <span class="bicon"><n-icon :component="GridOutline" :size="15" /></span>
        <n-input
          v-if="editingBoardId === b.id"
          :ref="(el) => el && (boardEditInput = el)"
          v-model:value="boardNameEdit"
          size="tiny"
          @click.stop
          @keyup.enter="commitBoardRename(b)"
          @blur="commitBoardRename(b)"
        />
        <span v-else class="name" @dblclick.stop="startBoardRename(b)">{{ b.name }}</span>
        <n-popover trigger="click" placement="right-start">
          <template #trigger>
            <n-button class="hover-btn" text size="tiny" @click.stop>
              <n-icon :component="EllipsisHorizontalOutline" />
            </n-button>
          </template>
          <div class="action-col" @click.stop>
            <n-button size="small" block @click="startBoardRename(b)">
              <template #icon><n-icon :component="CreateOutline" /></template>
              Переименовать
            </n-button>
            <n-popconfirm
              :positive-button-props="{ type: 'error' }"
              positive-text="Удалить"
              @positive-click="removeBoard(b)"
            >
              <template #trigger>
                <n-button type="error" ghost size="small" block>
                  <template #icon><n-icon :component="TrashOutline" /></template>
                  Удалить доску
                </n-button>
              </template>
              Удалить доску «{{ b.name }}» со всеми задачами?
            </n-popconfirm>
          </div>
        </n-popover>
      </div>
      <div v-if="addingBoard" class="row">
        <n-input
          ref="boardInput"
          v-model:value="newBoardName"
          size="tiny"
          placeholder="Название доски"
          @keyup.enter="addBoard"
          @blur="addBoard"
        />
      </div>
      <n-text v-if="!boards.length && !addingBoard" depth="3" class="empty">нет досок</n-text>
    </div>

    <n-dropdown
      trigger="manual"
      placement="bottom-start"
      :show="pcShow"
      :x="pcX"
      :y="pcY"
      :options="pcOptions"
      @select="onProjectCtxSelect"
      @clickoutside="pcShow = false"
    />
    <n-dropdown
      trigger="manual"
      placement="bottom-start"
      :show="bcShow"
      :x="bcX"
      :y="bcY"
      :options="bcOptions"
      @select="onBoardCtxSelect"
      @clickoutside="bcShow = false"
    />

    <ConfirmByName
      v-model:show="confirmDelete"
      :name="project.name"
      title="Удалить проект"
      message="Проект будет удалён со всеми досками и задачами. Действие необратимо."
      @confirm="doRemove"
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
.project-row:hover .hover-btn,
.board-row:hover .hover-btn {
  opacity: 1;
}
.chev {
  width: 14px;
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: var(--t-text3);
  transition: transform 0.15s;
  font-size: 12px;
}
.chev.open {
  transform: rotate(90deg);
}
/* Leaf rows (boards) have no chevron — keep the column so their icon/text line
   up with rows that do. */
.chev-spacer {
  width: 14px;
  flex: none;
}
.picon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  border-radius: 5px;
  color: #fff;
  font-size: 10px;
  line-height: 1;
  font-weight: 700;
  white-space: nowrap;
  overflow: hidden;
  flex: none;
}
/* Board (leaf) icon box — same footprint as project/group icons. */
.bicon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  color: var(--t-text3);
  flex: none;
}
/* No coloured square — let a custom icon sit on the panel; keep glyph/initials
   readable. */
.picon-bare {
  color: var(--t-text1);
}
.name {
  flex: 1;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  color: var(--t-text1);
}
/* Match the group→child gutter (SidebarNode .children) so the whole tree shares
   one indentation style. */
.boards {
  margin-left: 15px;
  padding-left: 6px;
  border-left: 1px solid var(--t-border);
}
.board-row {
  font-size: 13px;
  color: var(--t-text2);
}
.board-row.active {
  background: color-mix(in srgb, var(--t-primary) 16%, transparent);
  color: var(--t-primary);
  font-weight: 600;
}
.empty {
  font-size: 12px;
  padding: 2px 8px;
}
.settings {
  width: 264px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.action-row {
  display: flex;
  gap: 8px;
}
.action-row :deep(.n-button) {
  flex: 1;
}
.action-col {
  display: flex;
  flex-direction: column;
  gap: 8px;
  width: 190px;
}
</style>
