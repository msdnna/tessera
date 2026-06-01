<script setup>
import { ref, watch, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { NSpin, NCard, NText, NEmpty, NButton, NInput, NModal, useMessage } from 'naive-ui'
import { boards } from '@/api'

const route = useRoute()
const message = useMessage()
const loading = ref(false)
const board = ref(null)
const columns = ref([])
const tasksByCol = ref({})

async function load(id) {
  loading.value = true
  try {
    const [b, c, t] = await Promise.all([boards.get(id), boards.columns(id), boards.tasks(id)])
    board.value = b.data
    columns.value = c.data || []
    const map = {}
    for (const col of columns.value) map[col.id] = []
    for (const task of t.data || []) (map[task.column_id] ||= []).push(task)
    tasksByCol.value = map
  } catch (e) {
    message.error(e.message)
  } finally {
    loading.value = false
  }
}

// ── minimal create (column / task) — placeholder UX until Phase 4 ──
const modal = ref({ show: false, title: '', value: '', submit: null })
function promptCreate(title, submit) {
  modal.value = { show: true, title, value: '', submit }
}
async function confirmCreate() {
  const name = modal.value.value.trim()
  if (!name) return
  try {
    await modal.value.submit(name)
    modal.value.show = false
    await load(route.params.id)
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

onMounted(() => load(route.params.id))
watch(
  () => route.params.id,
  (id) => id && load(id),
)
</script>

<template>
  <n-spin :show="loading">
    <div v-if="board">
      <div class="head">
        <n-text strong style="font-size: 18px">{{ board.name }}</n-text>
        <n-button size="small" @click="newColumn">＋ Колонка</n-button>
      </div>

      <div class="cols">
        <n-card v-for="col in columns" :key="col.id" :title="col.name" size="small" class="col">
          <div v-for="task in tasksByCol[col.id] || []" :key="task.id" class="task">
            {{ task.title }}
          </div>
          <n-empty v-if="!(tasksByCol[col.id] || []).length" description="Пусто" size="small" />
          <template #action>
            <n-button text size="tiny" @click="newTask(col)">＋ задача</n-button>
          </template>
        </n-card>
        <n-empty v-if="!columns.length" description="Нет колонок — создайте первую" />
      </div>

      <n-text depth="3" style="display: block; margin-top: 12px">
        Полноценный канбан с drag &amp; drop и группировкой по тегам — Фаза 4.
      </n-text>
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
}
.cols {
  display: flex;
  gap: 12px;
  margin-top: 12px;
  align-items: flex-start;
  overflow-x: auto;
}
.col {
  min-width: 240px;
  flex: none;
}
.task {
  padding: 8px;
  border: 1px solid rgba(128, 128, 128, 0.25);
  border-radius: 6px;
  margin-bottom: 8px;
  font-size: 14px;
}
</style>
