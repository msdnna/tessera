<script setup>
import { ref, reactive, computed, watch, onMounted, nextTick } from 'vue'
import draggable from 'vuedraggable'
import {
  NSpin,
  NButton,
  NInput,
  NText,
  NSelect,
  NPopover,
  NCheckboxGroup,
  NCheckbox,
  NSpace,
  NButtonGroup,
  useMessage,
} from 'naive-ui'
import { boards, tasks as tasksApi, workspaces as wsApi, columns as columnsApi } from '@/api'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useRealtime } from '@/composables/useRealtime'
import { PRIORITY_LABELS } from '@/styles/tokens'
import TaskCard from './TaskCard.vue'
import TaskModal from './TaskModal.vue'
import TagManager from './TagManager.vue'
import ColumnHeader from './ColumnHeader.vue'

const props = defineProps({ boardId: { type: String, required: true } })

const message = useMessage()
const wsStore = useWorkspacesStore()

const loading = ref(false)
const board = ref(null)
const columns = ref([])
const allTasks = ref([])
const lists = ref({})
const tagsMap = reactive({})
const membersMap = reactive({})
const tagsList = computed(() => Object.values(tagsMap))
const membersList = computed(() => Object.values(membersMap))

// view controls
const groupMode = ref('status') // 'status' | 'tag'
const sortBy = ref('position') // 'position' | 'priority' | 'due'
const filters = reactive({ priorities: [], assignees: [], q: '' })
const sortOptions = [
  { label: 'Вручную', value: 'position' },
  { label: 'По приоритету', value: 'priority' },
  { label: 'По сроку', value: 'due' },
]
const priorityFilterOptions = PRIORITY_LABELS.map((label, value) => ({ label, value }))
const memberFilterOptions = computed(() =>
  membersList.value.map((m) => ({ label: m.name, value: m.user_id })),
)
const activeFilterCount = computed(
  () => filters.priorities.length + filters.assignees.length + (filters.q.trim() ? 1 : 0),
)

// modals
const selectedTaskId = ref(null)
const showTaskModal = ref(false)
function openTask(id) {
  selectedTaskId.value = id
  showTaskModal.value = true
}

let dragging = false
let suppressReloadUntil = 0
function suppress() {
  suppressReloadUntil = Date.now() + 1500
}
let reloadTimer = null
function scheduleReload() {
  clearTimeout(reloadTimer)
  reloadTimer = setTimeout(() => load(props.boardId), 200)
}

async function load(id) {
  loading.value = true
  try {
    const [b, c, t] = await Promise.all([boards.get(id), boards.columns(id), boards.tasks(id)])
    board.value = b.data
    columns.value = c.data || []
    allTasks.value = t.data || []
    await loadWorkspaceMeta()
    rebuildLists()
  } catch (e) {
    message.error(e.message)
  } finally {
    loading.value = false
  }
}

async function loadWorkspaceMeta() {
  const wsId = wsStore.currentId
  if (!wsId) return
  const [tg, mem] = await Promise.all([wsApi.tags(wsId), wsApi.members(wsId)])
  for (const k of Object.keys(tagsMap)) delete tagsMap[k]
  for (const t of tg.data || []) tagsMap[t.id] = t
  for (const k of Object.keys(membersMap)) delete membersMap[k]
  for (const m of mem.data || []) membersMap[m.user_id] = m
}

// filter + sort applied before grouping
const filteredTasks = computed(() => {
  let arr = allTasks.value
  if (filters.priorities.length) arr = arr.filter((t) => filters.priorities.includes(t.priority))
  if (filters.assignees.length)
    arr = arr.filter((t) => (t.assignee_ids || []).some((a) => filters.assignees.includes(a)))
  const q = filters.q.trim().toLowerCase()
  if (q) arr = arr.filter((t) => t.title.toLowerCase().includes(q))

  const s = [...arr]
  if (sortBy.value === 'priority') s.sort((a, b) => (b.priority || 0) - (a.priority || 0))
  else if (sortBy.value === 'due')
    s.sort(
      (a, b) =>
        (a.due_date ? Date.parse(a.due_date) : Infinity) -
        (b.due_date ? Date.parse(b.due_date) : Infinity),
    )
  return s
})

const displayColumns = computed(() => {
  if (groupMode.value === 'status') {
    return columns.value.map((c) => ({ key: c.id, name: c.name, color: c.color, status: c }))
  }
  return [
    ...tagsList.value.map((t) => ({ key: t.id, name: t.name, color: t.color, tag: t })),
    { key: '__none__', name: 'Без тегов', color: '', tag: null },
  ]
})

function rebuildLists() {
  const map = {}
  if (groupMode.value === 'status') {
    for (const col of columns.value) map[col.id] = []
    for (const t of filteredTasks.value) (map[t.column_id] ||= []).push(t)
  } else {
    for (const tg of tagsList.value) map[tg.id] = []
    map.__none__ = []
    for (const t of filteredTasks.value) {
      const ids = t.tag_ids || []
      if (!ids.length) map.__none__.push(t)
      else for (const id of ids) if (map[id]) map[id].push(t)
    }
  }
  lists.value = map
}
watch([filteredTasks, groupMode], rebuildLists)

// Mutable mirror of displayColumns for column drag-reorder (status mode only).
const colModel = ref([])
watch(displayColumns, (v) => (colModel.value = [...v]), { immediate: true })

async function onColumnReorder(evt) {
  if (groupMode.value !== 'status') return
  const info = evt.moved || evt.added
  if (!info) return
  const arr = colModel.value
  const before = arr[info.newIndex - 1]
  const after = arr[info.newIndex + 1]
  suppress()
  try {
    await columnsApi.move(info.element.key, {
      before_id: before ? before.key : null,
      after_id: after ? after.key : null,
    })
    scheduleReload()
  } catch (e) {
    message.error(e.message)
    load(props.boardId)
  }
}

// Drag persistence: status mode = reposition; tag mode = add/remove tag.
async function onColChange(evt, dcol) {
  suppress()
  try {
    if (groupMode.value === 'status') {
      const info = evt.added || evt.moved
      if (info) {
        const arr = lists.value[dcol.key]
        const before = arr[info.newIndex - 1]
        const after = arr[info.newIndex + 1]
        await tasksApi.move(info.element.id, {
          column_id: dcol.key,
          before_id: before ? before.id : null,
          after_id: after ? after.id : null,
        })
      }
    } else {
      if (evt.added && dcol.tag) await tasksApi.addTag(evt.added.element.id, dcol.tag.id)
      if (evt.removed && dcol.tag) await tasksApi.removeTag(evt.removed.element.id, dcol.tag.id)
    }
    scheduleReload()
  } catch (e) {
    message.error(e.message)
    load(props.boardId)
  }
}

// ── inline task creation (a reference tracker-style "+ New task" at column bottom) ──
const addingInColumn = ref(null) // dcol.key currently adding into
const newTaskTitle = ref('')
const taskInput = ref(null)
function focusTaskInput() {
  nextTick(() => taskInput.value?.focus?.())
}
function startAddTask(dcol) {
  addingInColumn.value = dcol.key
  newTaskTitle.value = ''
  focusTaskInput()
}
function cancelAddTask() {
  addingInColumn.value = null
  newTaskTitle.value = ''
}
async function submitAddTask(dcol) {
  const title = newTaskTitle.value.trim()
  if (!title) {
    cancelAddTask()
    return
  }
  // Tag mode has no status columns to drop into → use the first board column
  // and pre-tag with the tag column's tag.
  const columnId = groupMode.value === 'status' ? dcol.key : columns.value[0]?.id
  if (!columnId) {
    message.warning('Сначала создайте хотя бы одну колонку-статус')
    return
  }
  suppress()
  newTaskTitle.value = '' // keep the input open for rapid entry
  try {
    const res = await boards.createTask(board.value.id, { column_id: columnId, title })
    if (groupMode.value === 'tag' && dcol.tag) await tasksApi.addTag(res.data.id, dcol.tag.id)
    await load(props.boardId)
    addingInColumn.value = dcol.key // load() reset refs; re-open
    focusTaskInput()
  } catch (e) {
    message.error(e.message)
  }
}

// ── inline column creation ("+ New column" to the right) ──
const addingColumn = ref(false)
const newColumnName = ref('')
const colInput = ref(null)
function startAddColumn() {
  addingColumn.value = true
  newColumnName.value = ''
  nextTick(() => colInput.value?.focus?.())
}
async function submitAddColumn() {
  const name = newColumnName.value.trim()
  if (!name) {
    addingColumn.value = false
    return
  }
  suppress()
  try {
    await boards.createColumn(board.value.id, { name })
    newColumnName.value = ''
    addingColumn.value = false
    await load(props.boardId)
  } catch (e) {
    message.error(e.message)
  }
}

// A task edit or tag-list change touches both tasks and workspace meta, so do
// a full (debounced) board reload.
function onChanged() {
  suppress()
  scheduleReload()
}

useRealtime((ev) => {
  if (ev.scope !== wsStore.currentId) return
  if (dragging || Date.now() < suppressReloadUntil) return
  scheduleReload()
})

onMounted(() => load(props.boardId))
watch(
  () => props.boardId,
  (id) => id && load(id),
)
</script>

<template>
  <n-spin :show="loading">
    <div v-if="board" class="board-wrap">
      <div class="toolbar">
        <n-text strong style="font-size: 18px">{{ board.name }}</n-text>
        <n-space align="center" :size="8">
          <n-button-group size="small">
            <n-button
              :type="groupMode === 'status' ? 'primary' : 'default'"
              @click="groupMode = 'status'"
            >
              Статусы
            </n-button>
            <n-button
              :type="groupMode === 'tag' ? 'primary' : 'default'"
              @click="groupMode = 'tag'"
            >
              Теги
            </n-button>
          </n-button-group>

          <n-select
            v-model:value="sortBy"
            :options="sortOptions"
            size="small"
            style="width: 150px"
          />

          <n-popover trigger="click" placement="bottom-end">
            <template #trigger>
              <n-button size="small" :type="activeFilterCount ? 'primary' : 'default'" ghost>
                Фильтры{{ activeFilterCount ? ` (${activeFilterCount})` : '' }}
              </n-button>
            </template>
            <div class="filters">
              <n-input
                v-model:value="filters.q"
                size="small"
                placeholder="Поиск по названию"
                clearable
              />
              <n-text depth="3" class="flbl">Приоритет</n-text>
              <n-checkbox-group v-model:value="filters.priorities">
                <n-space vertical :size="4">
                  <n-checkbox
                    v-for="o in priorityFilterOptions"
                    :key="o.value"
                    :value="o.value"
                    :label="o.label"
                  />
                </n-space>
              </n-checkbox-group>
              <n-text depth="3" class="flbl">Исполнитель</n-text>
              <n-checkbox-group v-model:value="filters.assignees">
                <n-space vertical :size="4">
                  <n-checkbox
                    v-for="o in memberFilterOptions"
                    :key="o.value"
                    :value="o.value"
                    :label="o.label"
                  />
                </n-space>
              </n-checkbox-group>
            </div>
          </n-popover>

          <n-popover trigger="click" placement="bottom-end">
            <template #trigger>
              <n-button size="small">Теги</n-button>
            </template>
            <TagManager :ws-id="wsStore.currentId" :tags="tagsList" @changed="onChanged" />
          </n-popover>
        </n-space>
      </div>

      <div class="board-scroll">
        <draggable
          :list="colModel"
          group="columns"
          item-key="key"
          handle=".col-grip"
          :disabled="groupMode !== 'status'"
          class="cols"
          :animation="150"
          @change="onColumnReorder"
        >
          <template #item="{ element: dcol }">
            <div class="col" :style="{ '--col-accent': dcol.color || 'var(--t-primary)' }">
              <ColumnHeader
                :dcol="dcol"
                :count="(lists[dcol.key] || []).length"
                :editable="groupMode === 'status'"
                @changed="onChanged"
              />
              <draggable
                :list="lists[dcol.key]"
                group="tasks"
                item-key="id"
                class="drop"
                ghost-class="ghost"
                :animation="150"
                @start="dragging = true"
                @end="dragging = false"
                @change="onColChange($event, dcol)"
              >
                <template #item="{ element }">
                  <div>
                    <TaskCard
                      :task="element"
                      :tags-map="tagsMap"
                      :members-map="membersMap"
                      :tags="tagsList"
                      :members="membersList"
                      :ws-id="wsStore.currentId"
                      @open="openTask"
                      @changed="onChanged"
                    />
                  </div>
                </template>
              </draggable>

              <div v-if="addingInColumn === dcol.key" class="add-task-input">
                <n-input
                  :ref="(el) => (taskInput = el)"
                  v-model:value="newTaskTitle"
                  type="textarea"
                  size="small"
                  :autosize="{ minRows: 1, maxRows: 4 }"
                  placeholder="Название задачи, Enter — создать"
                  @keyup.enter.prevent="submitAddTask(dcol)"
                  @keyup.esc="cancelAddTask"
                  @blur="submitAddTask(dcol)"
                />
              </div>
              <n-button v-else text size="tiny" class="add-btn" @click="startAddTask(dcol)">
                ＋ Создать задачу
              </n-button>
            </div>
          </template>
        </draggable>

        <!-- inline new column (status mode) -->
        <div v-if="groupMode === 'status'" class="add-col">
          <n-input
            v-if="addingColumn"
            ref="colInput"
            v-model:value="newColumnName"
            size="small"
            placeholder="Название колонки"
            @keyup.enter="submitAddColumn"
            @keyup.esc="addingColumn = false"
            @blur="submitAddColumn"
          />
          <n-button v-else dashed block class="add-btn" @click="startAddColumn">
            ＋ Создать колонку
          </n-button>
        </div>
      </div>

      <div v-if="!displayColumns.length" class="empty-board">
        <n-text depth="3">
          {{
            groupMode === 'status'
              ? 'Нет колонок — создайте первую кнопкой «＋ Колонка».'
              : 'Нет тегов — добавьте в «Теги».'
          }}
        </n-text>
      </div>
    </div>

    <TaskModal
      v-model:show="showTaskModal"
      :task-id="selectedTaskId"
      :ws-id="wsStore.currentId"
      :tags="tagsList"
      :members="membersList"
      @changed="onChanged"
    />
  </n-spin>
</template>

<style scoped>
.toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
  flex-wrap: wrap;
}
.filters {
  width: 220px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.flbl {
  font-size: 12px;
  margin-top: 6px;
}
.board-scroll {
  display: flex;
  gap: 12px;
  align-items: flex-start;
  overflow-x: auto;
  padding-bottom: 8px;
}
.cols {
  display: flex;
  gap: 12px;
  align-items: flex-start;
}
.add-col {
  flex: 0 0 220px;
}
.add-task-input {
  margin-top: 4px;
}
.col {
  width: 280px;
  flex: 0 0 280px;
  background: var(--t-surface-alt);
  border-radius: 10px;
  border-top: 3px solid var(--col-accent);
  padding: 10px;
  align-self: flex-start;
  max-height: calc(100vh - 180px);
  display: flex;
  flex-direction: column;
}
.col-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;
  padding: 0 2px;
}
.col-grip {
  cursor: grab;
  color: var(--t-text3);
  font-size: 12px;
  line-height: 1;
}
.col-title {
  flex: 1;
  font-weight: 600;
  color: var(--t-text1);
}
.col-menu {
  font-size: 16px;
  line-height: 1;
}
.col-swatches {
  display: flex;
  gap: 8px;
  margin-top: 12px;
}
.cw {
  width: 24px;
  height: 24px;
  border-radius: 50%;
  border: 2px solid transparent;
  cursor: pointer;
}
.cw.active {
  border-color: var(--t-text1);
}
.cs-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.count {
  font-size: 12px;
  color: var(--t-text3);
  background: var(--t-hover);
  border-radius: 10px;
  padding: 0 7px;
}
.drop {
  flex: 1;
  overflow-y: auto;
  min-height: 8px;
}
.ghost {
  opacity: 0.5;
}
.add-btn {
  margin-top: 6px;
  width: 100%;
  justify-content: center;
  text-transform: uppercase;
  font-size: 11px;
  letter-spacing: 0.4px;
  color: var(--t-text3);
}
.empty-board {
  padding: 24px;
}
</style>
