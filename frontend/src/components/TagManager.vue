<script setup>
import { ref, nextTick } from 'vue'
import { NInput, NButton, NText, NIcon, NPopconfirm, useMessage } from 'naive-ui'
import { TrashOutline } from '@vicons/ionicons5'
import { workspaces as wsApi } from '@/api'

const props = defineProps({
  wsId: { type: String, default: null },
  tags: { type: Array, default: () => [] },
})
const emit = defineEmits(['changed'])

const message = useMessage()
const editingId = ref(null)
const nameEdit = ref('')
const nameInput = ref(null)
const newName = ref('')
const swatches = [
  '#7c5cff',
  '#2f80ed',
  '#0eb0a9',
  '#18a058',
  '#f0a020',
  '#e0533d',
  '#eb2f96',
  '#9aa0aa',
]

function startEdit(t) {
  editingId.value = t.id
  nameEdit.value = t.name
  nextTick(() => nameInput.value?.focus?.())
}
// blur/enter: save if changed, else just close (no accidental reset)
async function saveName(t) {
  const n = nameEdit.value.trim()
  editingId.value = null
  if (!n || n === t.name) return
  await patch(t, { name: n })
}
async function setColor(t, c) {
  await patch(t, { color: c })
}
async function patch(t, fields) {
  try {
    await wsApi.updateTag(t.id, { name: t.name, color: t.color || '', ...fields })
    emit('changed')
  } catch (e) {
    message.error(e.message)
  }
}
async function remove(t) {
  try {
    await wsApi.deleteTag(t.id)
    emit('changed')
  } catch (e) {
    message.error(e.message)
  }
}
async function add() {
  const n = newName.value.trim()
  if (!n) return
  try {
    await wsApi.createTag(props.wsId, { name: n, color: swatches[0] })
    newName.value = ''
    emit('changed')
  } catch (e) {
    message.error(e.message)
  }
}
</script>

<template>
  <div class="tagmgr">
    <n-text depth="3" class="head">Теги пространства</n-text>
    <div class="list">
      <div v-for="t in tags" :key="t.id" class="tag-block">
        <div class="tag-row">
          <n-input
            v-if="editingId === t.id"
            :ref="(el) => el && (nameInput = el)"
            v-model:value="nameEdit"
            size="tiny"
            placeholder="Имя тега"
            @keyup.enter="saveName(t)"
            @blur="saveName(t)"
          />
          <span
            v-else
            class="chip"
            title="Двойной клик — переименовать"
            :style="{ background: (t.color || '#888') + '22', color: t.color || '#888' }"
            @dblclick="startEdit(t)"
          >
            {{ t.name }}
          </span>
          <n-popconfirm @positive-click="remove(t)">
            <template #trigger>
              <n-button text size="tiny" type="error">
                <n-icon :component="TrashOutline" />
              </n-button>
            </template>
            Удалить тег? Он снимется со всех задач.
          </n-popconfirm>
        </div>
        <div v-if="editingId === t.id" class="swatches">
          <button
            v-for="s in swatches"
            :key="s"
            class="sw"
            :class="{ active: s === t.color }"
            :style="{ background: s }"
            @mousedown.prevent
            @click="setColor(t, s)"
          />
        </div>
      </div>
      <n-text v-if="!tags.length" depth="3" class="empty">Тегов пока нет.</n-text>
    </div>
    <div class="add">
      <n-input v-model:value="newName" size="tiny" placeholder="Новый тег" @keyup.enter="add" />
      <n-button type="primary" size="tiny" @click="add">Добавить</n-button>
    </div>
  </div>
</template>

<style scoped>
.tagmgr {
  width: 250px;
}
.head {
  display: block;
  font-size: 12px;
  margin-bottom: 8px;
}
.list {
  display: flex;
  flex-direction: column;
  gap: 4px;
  max-height: 280px;
  overflow-y: auto;
  margin-bottom: 10px;
}
.tag-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}
.chip {
  font-size: 12px;
  padding: 2px 10px;
  border-radius: 10px;
  cursor: pointer;
}
.swatches {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin: 8px 0 4px;
  padding-left: 2px;
}
.sw {
  width: 20px;
  height: 20px;
  border-radius: 50%;
  border: 2px solid transparent;
  cursor: pointer;
}
.sw.active {
  border-color: var(--t-text1);
}
.add {
  display: flex;
  gap: 6px;
}
.empty {
  font-size: 12px;
}
</style>
