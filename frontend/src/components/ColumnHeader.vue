<script setup>
import { ref } from 'vue'
import { NIcon, NButton, NInput, NPopover, NPopconfirm, useMessage } from 'naive-ui'
import {
  ReorderThreeOutline,
  EllipsisHorizontalOutline,
  TrashOutline,
  CheckmarkDoneOutline,
} from '@vicons/ionicons5'
import { columns as columnsApi } from '@/api'

const props = defineProps({
  dcol: { type: Object, required: true },
  count: { type: Number, default: 0 },
  editable: { type: Boolean, default: false }, // status columns only
  isDone: { type: Boolean, default: false }, // task-completing column
})
const emit = defineEmits(['changed', 'set-done'])

function toggleDone() {
  emit('set-done', props.isDone ? null : props.dcol.key)
  settingsOpen.value = false
}

const message = useMessage()
const renaming = ref(false)
const nameEdit = ref('')
const settingsOpen = ref(false)
const swatches = ['', '#7c5cff', '#2f80ed', '#0eb0a9', '#18a058', '#f0a020', '#e0533d', '#eb2f96']

function startRename() {
  if (!props.editable) return
  nameEdit.value = props.dcol.name
  renaming.value = true
}
async function commitRename() {
  renaming.value = false
  const n = nameEdit.value.trim()
  if (!n || n === props.dcol.name) return
  try {
    await columnsApi.update(props.dcol.key, { name: n, color: props.dcol.color || '' })
    emit('changed')
  } catch (e) {
    message.error(e.message)
  }
}
async function setColor(c) {
  try {
    await columnsApi.update(props.dcol.key, { name: props.dcol.name, color: c })
    emit('changed')
  } catch (e) {
    message.error(e.message)
  }
}
async function removeCol() {
  try {
    await columnsApi.remove(props.dcol.key)
    settingsOpen.value = false
    emit('changed')
  } catch (e) {
    message.error(e.message)
  }
}
</script>

<template>
  <div class="col-head">
    <n-icon v-if="editable" :component="ReorderThreeOutline" class="col-grip" title="Перетащить" />
    <n-input
      v-if="renaming"
      v-model:value="nameEdit"
      size="tiny"
      autofocus
      @keyup.enter="commitRename"
      @blur="commitRename"
    />
    <span v-else class="col-title" @dblclick="startRename">{{ dcol.name }}</span>
    <n-icon
      v-if="isDone"
      :component="CheckmarkDoneOutline"
      class="done-mark"
      title="Завершающая колонка — задачи здесь отмечаются выполненными"
    />
    <span class="count">{{ count }}</span>
    <n-popover v-if="editable" v-model:show="settingsOpen" trigger="click" placement="bottom-end">
      <template #trigger>
        <n-button text size="tiny" class="col-menu">
          <n-icon :component="EllipsisHorizontalOutline" />
        </n-button>
      </template>
      <div class="settings">
        <div class="swatches">
          <button
            v-for="s in swatches"
            :key="s || 'none'"
            class="sw"
            :class="{ active: s === (dcol.color || '') }"
            :style="{ background: s || 'var(--t-border)' }"
            :title="s || 'По умолчанию'"
            @click="setColor(s)"
          />
        </div>
        <n-button
          :type="isDone ? 'success' : 'default'"
          :ghost="isDone"
          size="small"
          block
          @click="toggleDone"
        >
          <template #icon><n-icon :component="CheckmarkDoneOutline" /></template>
          {{ isDone ? 'Снять завершение' : 'Сделать завершающей' }}
        </n-button>
        <n-popconfirm @positive-click="removeCol">
          <template #trigger>
            <n-button type="error" ghost size="small" block>
              <template #icon><n-icon :component="TrashOutline" /></template>
              Удалить колонку
            </n-button>
          </template>
          Удалить колонку со всеми задачами?
        </n-popconfirm>
      </div>
    </n-popover>
  </div>
</template>

<style scoped>
.col-head {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 8px;
  padding: 0 2px;
}
.col-grip {
  cursor: grab;
  color: var(--t-text3);
  font-size: 12px;
}
.col-title {
  flex: 1;
  font-weight: 600;
  color: var(--t-text1);
  cursor: text;
}
.count {
  font-size: 12px;
  color: var(--t-text3);
  background: var(--t-hover);
  border-radius: 10px;
  padding: 0 7px;
}
.done-mark {
  color: var(--t-success, #18a058);
  font-size: 15px;
}
.col-menu {
  font-size: 16px;
}
.settings {
  width: 220px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.settings :deep(.n-button__content) {
  white-space: nowrap;
}
.swatches {
  display: flex;
  flex-wrap: wrap;
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
