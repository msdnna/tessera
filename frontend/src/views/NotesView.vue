<script setup>
import { ref, watch, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { NButton, NInput, NText, NEmpty, NPopconfirm, useMessage } from 'naive-ui'
import { notes as notesApi } from '@/api'
import { useWorkspacesStore } from '@/stores/workspaces'

const message = useMessage()
const wsStore = useWorkspacesStore()
const route = useRoute()

const list = ref([])
const selected = ref(null)
const title = ref('')
const body = ref('')

async function loadList() {
  if (!wsStore.currentId) return
  try {
    const res = await notesApi.list(wsStore.currentId)
    list.value = res.data || []
    const wanted = route.query.note
    if (wanted) {
      const found = list.value.find((n) => n.id === String(wanted))
      if (found) select(found)
    }
  } catch (e) {
    message.error(e.message)
  }
}

function select(note) {
  selected.value = note
  title.value = note.title
  body.value = note.body || ''
}

function newNote() {
  selected.value = { id: null }
  title.value = ''
  body.value = ''
}

async function save() {
  const t = title.value.trim()
  if (!t) return
  try {
    if (selected.value?.id) {
      await notesApi.update(selected.value.id, { title: t, body: body.value })
    } else {
      const res = await notesApi.create(wsStore.currentId, { title: t, body: body.value })
      selected.value = res.data
    }
    await loadList()
    message.success('Сохранено')
  } catch (e) {
    message.error(e.message)
  }
}

async function remove() {
  if (!selected.value?.id) return
  try {
    await notesApi.remove(selected.value.id)
    selected.value = null
    await loadList()
  } catch (e) {
    message.error(e.message)
  }
}

onMounted(loadList)
watch(
  () => wsStore.currentId,
  () => {
    selected.value = null
    loadList()
  },
)
watch(
  () => route.query.note,
  () => loadList(),
)
</script>

<template>
  <div class="notes">
    <div class="list">
      <n-button type="primary" block size="small" @click="newNote">＋ Новая заметка</n-button>
      <div
        v-for="n in list"
        :key="n.id"
        class="note-item"
        :class="{ active: selected?.id === n.id }"
        @click="select(n)"
      >
        <div class="ni-title">{{ n.title }}</div>
        <div class="ni-snippet">{{ (n.body || '').slice(0, 60) }}</div>
      </div>
      <n-empty v-if="!list.length" description="Заметок пока нет" size="small" />
    </div>

    <div class="editor">
      <template v-if="selected">
        <n-input v-model:value="title" placeholder="Заголовок" class="title" />
        <n-input
          v-model:value="body"
          type="textarea"
          placeholder="Текст заметки…"
          :autosize="{ minRows: 12 }"
        />
        <div class="actions">
          <n-popconfirm v-if="selected.id" @positive-click="remove">
            <template #trigger>
              <n-button quaternary type="error">Удалить</n-button>
            </template>
            Удалить заметку?
          </n-popconfirm>
          <n-button type="primary" @click="save">Сохранить</n-button>
        </div>
      </template>
      <div v-else class="placeholder">
        <n-text depth="3">Выберите заметку или создайте новую</n-text>
      </div>
    </div>
  </div>
</template>

<style scoped>
.notes {
  display: flex;
  gap: 16px;
  height: 100%;
}
.list {
  width: 260px;
  flex: none;
  display: flex;
  flex-direction: column;
  gap: 8px;
  overflow-y: auto;
}
.note-item {
  padding: 8px 10px;
  border: 1px solid var(--t-border);
  border-radius: 8px;
  cursor: pointer;
  background: var(--t-surface);
}
.note-item.active {
  border-color: var(--t-primary);
}
.ni-title {
  font-weight: 600;
  color: var(--t-text1);
}
.ni-snippet {
  font-size: 12px;
  color: var(--t-text3);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.editor {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 10px;
  min-width: 0;
}
.title :deep(input) {
  font-size: 18px;
  font-weight: 600;
}
.actions {
  display: flex;
  justify-content: space-between;
}
.placeholder {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100%;
}
</style>
