<script setup>
// «Файлы» tab of the task modal: attachment list, upload and download.
// The list itself stays owned by the modal (the tab label carries its count and
// the modal loads it alongside comments/relations/journal), so mutations here are
// published back through `update:attachments`.
import { ref } from 'vue'
import { NIcon, NButton, NPopconfirm, useMessage } from 'naive-ui'
import { AttachOutline, DownloadOutline, TrashOutline } from '@vicons/ionicons5'
import { tasks as tasksApi } from '@/api'
import { isTauri } from '@/utils/serverBase'
import EmptyState from '../EmptyState.vue'

const props = defineProps({
  taskId: { type: String, default: null },
  attachments: { type: Array, default: () => [] },
})
const emit = defineEmits(['update:attachments', 'changed'])

const message = useMessage()
const fileInput = ref(null)
const uploading = ref(false)

function fmtSize(bytes) {
  if (bytes < 1024) return `${bytes} Б`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(0)} КБ`
  return `${(bytes / 1024 / 1024).toFixed(1)} МБ`
}
function pickFile() {
  fileInput.value?.click?.()
}
async function onFileChosen(ev) {
  const file = ev.target.files?.[0]
  if (!file) return
  uploading.value = true
  try {
    const fd = new FormData()
    fd.append('file', file)
    await tasksApi.uploadAttachment(props.taskId, fd)
    const a = await tasksApi.attachments(props.taskId)
    emit('update:attachments', a.data || [])
    emit('changed')
  } catch (e) {
    message.error(e.message)
  } finally {
    uploading.value = false
    ev.target.value = ''
  }
}
async function downloadAttachment(att) {
  try {
    const res = await tasksApi.downloadAttachment(att.id)
    // Desktop: a real "Save as…" dialog + write to disk (the webview can't drive
    // an <a download> file save). Web keeps the anchor-download path.
    if (isTauri()) {
      const { save } = await import('@tauri-apps/plugin-dialog')
      const { writeFile } = await import('@tauri-apps/plugin-fs')
      const path = await save({ defaultPath: att.filename })
      if (!path) return
      await writeFile(path, new Uint8Array(await res.data.arrayBuffer()))
      message.success('Файл сохранён')
      return
    }
    const url = URL.createObjectURL(res.data)
    const a = document.createElement('a')
    a.href = url
    a.download = att.filename
    document.body.appendChild(a)
    a.click()
    a.remove()
    URL.revokeObjectURL(url)
  } catch (e) {
    message.error(e.message)
  }
}
async function deleteAttachment(id) {
  try {
    await tasksApi.removeAttachment(id)
    emit(
      'update:attachments',
      props.attachments.filter((x) => x.id !== id),
    )
  } catch (e) {
    message.error(e.message)
  }
}
</script>

<template>
  <div class="files">
    <div v-for="a in attachments" :key="a.id" class="filerow">
      <n-icon :component="AttachOutline" class="f-ico" />
      <button class="f-name" @click="downloadAttachment(a)">{{ a.filename }}</button>
      <span class="f-size">{{ fmtSize(a.size) }}</span>
      <button class="c-act" title="Скачать" @click="downloadAttachment(a)">
        <n-icon :component="DownloadOutline" />
      </button>
      <n-popconfirm
        :positive-button-props="{ type: 'error' }"
        positive-text="Удалить"
        @positive-click="deleteAttachment(a.id)"
      >
        <template #trigger>
          <button class="c-act" title="Удалить">
            <n-icon :component="TrashOutline" />
          </button>
        </template>
        Удалить файл «{{ a.filename }}»?
      </n-popconfirm>
    </div>
    <EmptyState v-if="!attachments.length" size="small" :icon="AttachOutline" text="Файлов пока нет" />
    <input ref="fileInput" type="file" hidden @change="onFileChosen" />
    <n-button size="small" :loading="uploading" @click="pickFile">
      <template #icon><n-icon :component="AttachOutline" /></template>
      Прикрепить файл
    </n-button>
  </div>
</template>

<style scoped>
@import './tab-shared.css';

.files {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.filerow {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
}
.f-ico {
  color: var(--t-text3);
  flex: none;
}
.f-name {
  flex: 1;
  min-width: 0;
  border: none;
  background: none;
  cursor: pointer;
  text-align: left;
  color: var(--t-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.f-size {
  font-size: 11px;
  color: var(--t-text3);
  flex: none;
}
</style>
