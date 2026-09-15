<script setup>
// «Файлы» tab of the task modal: attachment list, upload and download.
// The list itself stays owned by the modal (the tab label carries its count and
// the modal loads it alongside comments/relations/journal), so mutations here are
// published back through `update:attachments`.
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon, NButton, NPopconfirm, useMessage } from 'naive-ui'
import { AttachOutline, DownloadOutline, TrashOutline } from '@vicons/ionicons5'
import { tasks as tasksApi } from '@/api'
import { saveAttachment } from '@/utils/download'
import { ATTACHMENT_MAX_BYTES, attachmentPickError, attachmentSizeText } from '@/utils/upload'
import { formatFileSize } from '@/utils/docPdf'
import EmptyState from '../EmptyState.vue'

const props = defineProps({
  taskId: { type: String, default: null },
  attachments: { type: Array, default: () => [] },
})
const emit = defineEmits(['update:attachments', 'changed'])

const message = useMessage()
const { t } = useI18n()
const fileInput = ref(null)
const uploading = ref(false)

// Both halves of a size are locale business — the unit *and* the decimal
// separator — so the whole label comes from utils/upload, where it can be
// pinned with a test (#2932).
function pickFile() {
  fileInput.value?.click?.()
}
async function onFileChosen(ev) {
  const file = ev.target.files?.[0]
  if (!file) return
  // Say what the server would say, before spending the upload on it. Its own
  // refusal is a hardcoded Russian sentence ("файл больше 25 МБ") that errors.js
  // passes through untouched — and it only arrives once the whole file has gone
  // over the wire.
  if (attachmentPickError(file)) {
    message.error(t('task.files.tooLarge', { limit: formatFileSize(ATTACHMENT_MAX_BYTES) }))
    ev.target.value = ''
    return
  }
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
    if ((await saveAttachment(att.id, att.filename)) === 'saved')
      message.success(t('task.files.saved'))
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
      <span class="f-size">{{ attachmentSizeText(a.size) }}</span>
      <button class="c-act" :title="t('task.files.download')" @click="downloadAttachment(a)">
        <n-icon :component="DownloadOutline" />
      </button>
      <n-popconfirm
        :positive-button-props="{ type: 'error' }"
        :positive-text="t('common.action.delete')"
        @positive-click="deleteAttachment(a.id)"
      >
        <template #trigger>
          <button class="c-act" :title="t('common.action.delete')">
            <n-icon :component="TrashOutline" />
          </button>
        </template>
        {{ t('task.files.deleteConfirm', { name: a.filename }) }}
      </n-popconfirm>
    </div>
    <EmptyState
      v-if="!attachments.length"
      size="small"
      :icon="AttachOutline"
      :text="t('task.files.empty')"
    />
    <input ref="fileInput" type="file" hidden @change="onFileChosen" />
    <n-button size="small" :loading="uploading" @click="pickFile">
      <template #icon><n-icon :component="AttachOutline" /></template>
      {{ t('task.files.attach') }}
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
