<script setup>
import { ref, watch } from 'vue'
import { NModal, NCard, NButton, NEmpty, NPopconfirm, NIcon, useMessage } from 'naive-ui'
import { TrashOutline, ArrowUndoOutline } from '@vicons/ionicons5'
import { boards as boardsApi, tasks as tasksApi } from '@/api'

const props = defineProps({
  show: { type: Boolean, default: false },
  boardId: { type: String, default: null },
})
const emit = defineEmits(['update:show', 'changed'])

const message = useMessage()
const list = ref([])

async function load() {
  if (!props.boardId) return
  try {
    list.value = (await boardsApi.archive(props.boardId)).data || []
  } catch (e) {
    message.error(e.message)
  }
}
async function restore(t) {
  try {
    await tasksApi.restore(t.id)
    await load()
    emit('changed')
  } catch (e) {
    message.error(e.message)
  }
}
async function remove(t) {
  try {
    await tasksApi.remove(t.id)
    await load()
    emit('changed')
  } catch (e) {
    message.error(e.message)
  }
}

watch(
  () => [props.show, props.boardId],
  ([show]) => show && load(),
)
</script>

<template>
  <n-modal :show="show" @update:show="emit('update:show', $event)">
    <n-card title="Архив" style="width: 480px; max-width: 92vw" role="dialog">
      <div class="list">
        <div v-for="t in list" :key="t.id" class="arow">
          <span class="atitle">{{ t.title }}</span>
          <n-button text size="tiny" title="Восстановить" @click="restore(t)">
            <n-icon :component="ArrowUndoOutline" />
          </n-button>
          <n-popconfirm @positive-click="remove(t)">
            <template #trigger>
              <n-button text size="tiny" type="error" title="Удалить навсегда">
                <n-icon :component="TrashOutline" />
              </n-button>
            </template>
            Удалить навсегда? Действие необратимо.
          </n-popconfirm>
        </div>
        <n-empty v-if="!list.length" description="Архив пуст" />
      </div>
    </n-card>
  </n-modal>
</template>

<style scoped>
.list {
  display: flex;
  flex-direction: column;
  gap: 6px;
  max-height: 60vh;
  overflow-y: auto;
}
.arow {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 10px;
  border: 1px solid var(--t-border);
  border-radius: 8px;
}
.atitle {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
