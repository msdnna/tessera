<script setup>
import { ref, computed, nextTick, onBeforeUnmount } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { NIcon, NButton, NInput, NPopover, NPopconfirm, NText, NDropdown, useMessage } from 'naive-ui'
import {
  GridOutline,
  EllipsisHorizontalOutline,
  ChevronForwardOutline,
  AddOutline,
  CreateOutline,
  TrashOutline,
} from '@vicons/ionicons5'
import { projects as projApi, boards as boardsApi } from '@/api'
import { useWorkspacesStore } from '@/stores/workspaces'
import { PROJECT_ICONS, iconComponent } from '@/utils/projectIcons'
import { pressMoved } from '@/utils/dnd'
import { useLongPress } from '@/composables/useLongPress'

const props = defineProps({
  project: { type: Object, required: true },
  depth: { type: Number, default: 0 },
})

const store = useWorkspacesStore()
const router = useRouter()
const route = useRoute()
const message = useMessage()

const expanded = ref(false)
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

const swatches = ['', '#7c5cff', '#2f80ed', '#0eb0a9', '#18a058', '#f0a020', '#e0533d', '#eb2f96']
const boards = computed(() => store.boardsByProject[props.project.id] || [])
const iconComp = computed(() => iconComponent(props.project.icon))
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
async function remove() {
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
  { label: 'Новая доска', key: 'add-board' },
  { type: 'divider', key: 'd1' },
  { label: 'Переименовать', key: 'rename' },
  { label: 'Удалить проект', key: 'delete' },
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
  { label: 'Открыть', key: 'open' },
  { label: 'Переименовать', key: 'rename' },
  { label: 'Удалить доску', key: 'delete' },
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
  if (key === 'open') router.push(`/board/${b.id}`)
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
        :style="{ background: project.color || 'var(--t-primary)' }"
        @click="toggle"
      >
        <n-icon v-if="iconComp" :component="iconComp" :size="13" />
        <template v-else>{{ initials }}</template>
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
          <div class="icons">
            <button
              class="ic"
              :class="{ active: !project.icon }"
              title="Инициалы"
              @click="updateField({ icon: '' })"
            >
              {{ initials }}
            </button>
            <button
              v-for="i in PROJECT_ICONS"
              :key="i.key"
              class="ic"
              :class="{ active: project.icon === i.key }"
              @click="updateField({ icon: i.key })"
            >
              <n-icon :component="i.component" :size="16" />
            </button>
          </div>
          <div class="swatches">
            <button
              v-for="s in swatches"
              :key="s || 'none'"
              class="sw"
              :class="{ active: s === (project.color || '') }"
              :style="{ background: s || 'var(--t-border)' }"
              @click="updateField({ color: s })"
            />
          </div>
          <div class="action-row">
            <n-button size="small" @click="startRename">
              <template #icon><n-icon :component="CreateOutline" /></template>
              Переименовать
            </n-button>
            <n-popconfirm @positive-click="remove">
              <template #trigger>
                <n-button type="error" ghost size="small">
                  <template #icon><n-icon :component="TrashOutline" /></template>
                  Удалить
                </n-button>
              </template>
              Удалить проект со всеми досками?
            </n-popconfirm>
          </div>
        </div>
      </n-popover>
    </div>

    <div v-if="expanded" class="boards">
      <div
        v-for="b in boards"
        :key="b.id"
        class="row board-row"
        :class="{ active: route.params.id === b.id }"
        @click="editingBoardId !== b.id && router.push(`/board/${b.id}`)"
        @contextmenu.prevent.stop="onBoardCtx($event, b)"
        @touchstart.passive="bStart($event, b)"
        @touchend="bCancel"
        @touchcancel="bCancel"
      >
        <n-icon :component="GridOutline" :size="14" />
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
            <n-popconfirm @positive-click="removeBoard(b)">
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
  color: var(--t-text3);
  transition: transform 0.15s;
  font-size: 12px;
}
.chev.open {
  transform: rotate(90deg);
}
.picon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  border-radius: 5px;
  color: #fff;
  font-size: 10px;
  line-height: 1;
  font-weight: 700;
  white-space: nowrap;
  overflow: hidden;
  flex: none;
}
.name {
  flex: 1;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  color: var(--t-text1);
}
.boards {
  padding-left: 18px;
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
.icons {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 6px;
}
.ic {
  width: 28px;
  height: 28px;
  border-radius: 6px;
  border: 1px solid var(--t-border);
  background: var(--t-surface);
  color: var(--t-text2);
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: 10px;
  line-height: 1;
  font-weight: 700;
  white-space: nowrap;
  overflow: hidden;
}
.ic.active {
  border-color: var(--t-primary);
  color: var(--t-primary);
}
.swatches {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 6px;
}
.sw {
  width: 22px;
  height: 22px;
  border-radius: 50%;
  border: 2px solid transparent;
  cursor: pointer;
}
.sw.active {
  border-color: var(--t-text1);
}
</style>
