<script setup>
import { ref } from 'vue'
import { NModal, NCard, NInput, NButton, NSpace, NText, NPopconfirm, useMessage } from 'naive-ui'
import { workspaces as wsApi } from '@/api'

const props = defineProps({
  show: { type: Boolean, default: false },
  wsId: { type: String, default: null },
  tags: { type: Array, default: () => [] },
})
const emit = defineEmits(['update:show', 'changed'])

const message = useMessage()
const name = ref('')
const color = ref('#7c5cff')

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

async function add() {
  const n = name.value.trim()
  if (!n || !props.wsId) return
  try {
    await wsApi.createTag(props.wsId, { name: n, color: color.value })
    name.value = ''
    emit('changed')
  } catch (e) {
    message.error(e.message)
  }
}
async function remove(id) {
  try {
    await wsApi.deleteTag(id)
    emit('changed')
  } catch (e) {
    message.error(e.message)
  }
}
</script>

<template>
  <n-modal :show="show" @update:show="emit('update:show', $event)">
    <n-card title="Теги пространства" style="width: 440px; max-width: 92vw" role="dialog">
      <div class="list">
        <div v-for="t in tags" :key="t.id" class="tag-row">
          <span
            class="chip"
            :style="{ background: (t.color || '#888') + '22', color: t.color || '#888' }"
          >
            {{ t.name }}
          </span>
          <n-popconfirm @positive-click="remove(t.id)">
            <template #trigger>
              <n-button text size="tiny" type="error">✕</n-button>
            </template>
            Удалить тег? Он снимется со всех задач.
          </n-popconfirm>
        </div>
        <n-text v-if="!tags.length" depth="3">Тегов пока нет.</n-text>
      </div>

      <div class="add">
        <div class="swatches">
          <button
            v-for="s in swatches"
            :key="s"
            class="sw"
            :class="{ active: s === color }"
            :style="{ background: s }"
            @click="color = s"
          />
        </div>
        <n-space>
          <n-input
            v-model:value="name"
            size="small"
            placeholder="Название тега"
            @keyup.enter="add"
          />
          <n-button type="primary" size="small" @click="add">Добавить</n-button>
        </n-space>
      </div>
    </n-card>
  </n-modal>
</template>

<style scoped>
.list {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-bottom: 14px;
  max-height: 240px;
  overflow-y: auto;
}
.tag-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.chip {
  font-size: 12px;
  padding: 2px 10px;
  border-radius: 10px;
}
.swatches {
  display: flex;
  gap: 6px;
  margin-bottom: 8px;
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
