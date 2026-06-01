<script setup>
import { ref, reactive, computed, watch, onMounted } from 'vue'
import draggable from 'vuedraggable'
import {
  NSpin,
  NButton,
  NInput,
  NModal,
  NCard,
  NText,
  NSelect,
  NPopover,
  NCheckboxGroup,
  NCheckbox,
  NSpace,
  NButtonGroup,
  NPopconfirm,
  NIcon,
  useMessage,
} from 'naive-ui'
import { ReorderThreeOutline, EllipsisHorizontalOutline } from '@vicons/ionicons5'
import { boards, tasks as tasksApi, workspaces as wsApi, columns as columnsApi } from '@/api'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useRealtime } from '@/composables/useRealtime'
import { PRIORITY_LABELS } from '@/styles/tokens'
import TaskCard from './TaskCard.vue'
import TaskModal from './TaskModal.vue'
import TagManager from './TagManager.vue'

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
const showTagManager = ref(false)
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

// ── column settings (rename / color / delete) ──
const colSwatches = [
  '',
  '#7c5cff',
  '#2f80ed',
  '#0eb0a9',
  '#18a058',
  '#f0a020',
  '#e0533d',
  '#eb2f96',
]
const colSettings = ref({ show: false, id: null, name: '', color: '' })
function openColSettings(dcol) {
  colSettings.value = { show: true, id: dcol.key, name: dcol.name, color: dcol.color || '' }
}
async function saveColSettings() {
  suppress()
  try {
    await columnsApi.update(colSettings.value.id, {
      name: colSettings.value.name.trim(),
      color: colSettings.value.color,
    })
    colSettings.value.show = false
    await load(props.boardId)
  } catch (e) {
    message.error(e.message)
  }
}
async function deleteColumn() {
  suppress()
  try {
    await columnsApi.remove(colSettings.value.id)
    colSettings.value.show = false
    await load(props.boardId)
  } catch (e) {
    message.error(e.message)
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

// ── create column / task ──
const modal = ref({ show: false, title: '', value: '', submit: null })
function promptCreate(title, submit) {
  modal.value = { show: true, title, value: '', submit }
}
async function confirmCreate() {
  const name = modal.value.value.trim()
  if (!name) return
  suppress()
  try {
    await modal.value.submit(name)
    modal.value.show = false
    await load(props.boardId)
  } catch (e) {
    message.error(e.message)
  }
}
function newColumn() {
  promptCreate('Новая колонка', (name) => boards.createColumn(board.value.id, { name }))
}
function newTask(dcol) {
  // In tag mode there are no status columns to drop into; fall back to the
  // first board column and pre-tag the task with the column's tag.
  const columnId = groupMode.value === 'status' ? dcol.key : columns.value[0]?.id
  if (!columnId) {
    message.warning('Сначала создайте хотя бы одну колонку-статус')
    return
  }
  promptCreate('Новая задача', async (title) => {
    const res = await boards.createTask(board.value.id, { column_id: columnId, title })
    if (groupMode.value === 'tag' && dcol.tag) await tasksApi.addTag(res.data.id, dcol.tag.id)
  })
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

          <n-button size="small" @click="showTagManager = true">Теги</n-button>
          <n-button v-if="groupMode === 'status'" size="small" @click="newColumn"
            >＋ Колонка</n-button
          >
        </n-space>
      </div>

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
            <div class="col-head">
              <n-icon
                v-if="groupMode === 'status'"
                :component="ReorderThreeOutline"
                class="col-grip"
                title="Перетащить"
              />
              <span class="col-title">{{ dcol.name }}</span>
              <span class="count">{{ (lists[dcol.key] || []).length }}</span>
              <n-button
                v-if="groupMode === 'status'"
                text
                size="tiny"
                class="col-menu"
                @click="openColSettings(dcol)"
              >
                <n-icon :component="EllipsisHorizontalOutline" />
              </n-button>
            </div>
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
                    @click="openTask(element.id)"
                  />
                </div>
              </template>
            </draggable>
            <n-button text size="tiny" class="add-task" @click="newTask(dcol)">＋ задача</n-button>
          </div>
        </template>
      </draggable>

      <div v-if="!displayColumns.length" class="empty-board">
        <n-text depth="3">
          {{
            groupMode === 'status'
              ? 'Нет колонок — создайте первую.'
              : 'Нет тегов — добавьте в «Теги».'
          }}
        </n-text>
      </div>
    </div>

    <n-modal v-model:show="modal.show">
      <n-card :title="modal.title" style="max-width: 360px" role="dialog">
        <n-input v-model:value="modal.value" placeholder="Название" @keyup.enter="confirmCreate" />
        <template #footer>
          <n-button type="primary" @click="confirmCreate">Создать</n-button>
        </template>
      </n-card>
    </n-modal>

    <n-modal v-model:show="colSettings.show">
      <n-card title="Настройки колонки" style="width: 360px; max-width: 92vw" role="dialog">
        <n-input
          v-model:value="colSettings.name"
          placeholder="Название колонки"
          @keyup.enter="saveColSettings"
        />
        <div class="col-swatches">
          <button
            v-for="s in colSwatches"
            :key="s || 'none'"
            class="cw"
            :class="{ active: s === colSettings.color }"
            :style="{ background: s || 'var(--t-border)' }"
            :title="s || 'По умолчанию'"
            @click="colSettings.color = s"
          />
        </div>
        <template #footer>
          <div class="cs-footer">
            <n-popconfirm @positive-click="deleteColumn">
              <template #trigger>
                <n-button quaternary type="error">Удалить</n-button>
              </template>
              Удалить колонку со всеми её задачами?
            </n-popconfirm>
            <n-button type="primary" @click="saveColSettings">Сохранить</n-button>
          </div>
        </template>
      </n-card>
    </n-modal>

    <TaskModal
      v-model:show="showTaskModal"
      :task-id="selectedTaskId"
      :tags="tagsList"
      :members="membersList"
      @changed="onChanged"
    />

    <TagManager
      v-model:show="showTagManager"
      :ws-id="wsStore.currentId"
      :tags="tagsList"
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
.cols {
  display: flex;
  gap: 12px;
  align-items: flex-start;
  overflow-x: auto;
  padding-bottom: 8px;
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
.add-task {
  margin-top: 6px;
  align-self: flex-start;
}
.empty-board {
  padding: 24px;
}
</style>
