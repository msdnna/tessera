<script setup>
import { ref, reactive, watch, onMounted } from 'vue'
import draggable from 'vuedraggable'
import { NSpin, NButton, NInput, NModal, NCard, NText, useMessage } from 'naive-ui'
import { boards, tasks as tasksApi, workspaces as wsApi } from '@/api'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useRealtime } from '@/composables/useRealtime'
import TaskCard from './TaskCard.vue'

const props = defineProps({ boardId: { type: String, required: true } })
const emit = defineEmits(['open'])

const message = useMessage()
const wsStore = useWorkspacesStore()

const loading = ref(false)
const board = ref(null)
const columns = ref([])
const lists = ref({}) // columnId -> task[]
const tagsMap = reactive({})
const membersMap = reactive({})

let dragging = false
let suppressReloadUntil = 0
function suppress() {
  suppressReloadUntil = Date.now() + 1500
}

async function load(id) {
  loading.value = true
  try {
    const [b, c, t] = await Promise.all([boards.get(id), boards.columns(id), boards.tasks(id)])
    board.value = b.data
    columns.value = c.data || []
    const map = {}
    for (const col of columns.value) map[col.id] = []
    for (const task of t.data || []) (map[task.column_id] ||= []).push(task)
    lists.value = map
    await loadWorkspaceMeta()
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

// Drag end: the moved task's destination list is already mutated by v-model.
// Compute its neighbours and persist via the move API (server recomputes pos).
async function onChange(evt, col) {
  const info = evt.added || evt.moved
  if (!info) return // `removed` is handled by the destination column's `added`
  const arr = lists.value[col.id]
  const idx = info.newIndex
  const before = arr[idx - 1]
  const after = arr[idx + 1]
  suppress()
  try {
    await tasksApi.move(info.element.id, {
      column_id: col.id,
      before_id: before ? before.id : null,
      after_id: after ? after.id : null,
    })
  } catch (e) {
    message.error(e.message)
    await load(props.boardId)
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
function newTask(col) {
  promptCreate('Новая задача', (title) =>
    boards.createTask(board.value.id, { column_id: col.id, title }),
  )
}

// ── realtime ──
let reloadTimer = null
useRealtime((ev) => {
  if (ev.scope !== wsStore.currentId) return
  if (dragging || Date.now() < suppressReloadUntil) return
  clearTimeout(reloadTimer)
  reloadTimer = setTimeout(() => load(props.boardId), 400)
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
      <div class="head">
        <n-text strong style="font-size: 18px">{{ board.name }}</n-text>
        <n-button size="small" @click="newColumn">＋ Колонка</n-button>
      </div>

      <div class="cols">
        <div
          v-for="col in columns"
          :key="col.id"
          class="col"
          :style="{ '--col-accent': col.color || 'var(--t-primary)' }"
        >
          <div class="col-head">
            <span class="col-title">{{ col.name }}</span>
            <span class="count">{{ (lists[col.id] || []).length }}</span>
          </div>
          <draggable
            :list="lists[col.id]"
            group="tasks"
            item-key="id"
            class="drop"
            ghost-class="ghost"
            :animation="150"
            @start="dragging = true"
            @end="dragging = false"
            @change="onChange($event, col)"
          >
            <template #item="{ element }">
              <div>
                <TaskCard
                  :task="element"
                  :tags-map="tagsMap"
                  :members-map="membersMap"
                  @click="emit('open', element.id)"
                />
              </div>
            </template>
          </draggable>
          <n-button text size="tiny" class="add-task" @click="newTask(col)">＋ задача</n-button>
        </div>

        <div v-if="!columns.length" class="empty-board">
          <n-text depth="3">Нет колонок — создайте первую кнопкой «＋ Колонка».</n-text>
        </div>
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
  </n-spin>
</template>

<style scoped>
.head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
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
  max-height: calc(100vh - 150px);
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
.col-title {
  font-weight: 600;
  color: var(--t-text1);
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
