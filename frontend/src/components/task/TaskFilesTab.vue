<script setup>
// «Файлы» tab of the task modal: attachment list, upload and download.
// The list itself stays owned by the modal (the tab label carries its count and
// the modal loads it alongside comments/relations/journal), so mutations here are
// published back through `update:attachments`.
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon, useMessage } from 'naive-ui'
import { AttachOutline, DownloadOutline } from '@vicons/ionicons5'
import { tasks as tasksApi } from '@/api'
import { saveAttachment } from '@/utils/download'
import { ATTACHMENT_MAX_BYTES, attachmentPickError, attachmentSizeText } from '@/utils/upload'
import { formatFileSize } from '@/utils/docPdf'
import EmptyState from '../EmptyState.vue'
import TesseraSpinner from '../TesseraSpinner.vue'
import ArmButton from '../kit/ArmButton.vue'

const props = defineProps({
  taskId: { type: String, default: null },
  attachments: { type: Array, default: () => [] },
  // The modal opened from the archive is read-only. Downloading stays — that is
  // most of why an archived task is opened at all — but uploading a file onto it
  // and deleting one off it do not (#2932).
  readonly: { type: Boolean, default: false },
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
  if (props.readonly) return
  fileInput.value?.click?.()
}
async function onFileChosen(ev) {
  if (props.readonly) return
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
  // Which task the upload belongs to, taken before the first await. The modal is
  // one per board and is not remounted when the task under it changes (a click on
  // a subtask, a relation or «Родителю» swaps `taskId` with this subtree kept
  // alive), and an upload is the longest thing in it — 25 MB over the wire is a
  // window with room to spare. Re-reading props afterwards asked the *new* task
  // for its files: the uploaded one showed up nowhere and got uploaded again.
  const tid = props.taskId
  uploading.value = true
  try {
    const fd = new FormData()
    fd.append('file', file)
    await tasksApi.uploadAttachment(tid, fd)
    const a = await tasksApi.attachments(tid)
    // The list belongs to the modal, so publishing it into another task's tab is
    // not a harmless stale render: the rows would be #A's files under #B's label,
    // and the ✕ on one of them deletes by id — the other task's attachment, with
    // nothing on screen to say so (#2932).
    if (tid !== props.taskId) return
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
  if (props.readonly) return
  const tid = props.taskId
  try {
    await tasksApi.removeAttachment(id)
    // Same guard, other direction: the deletion itself stands (it is addressed by
    // row id), but the list it edits is whichever one the modal is showing now.
    // Without this the reply rewrote the open task's list from the left one's.
    if (tid !== props.taskId) return
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
      <button
        type="button"
        class="f-ibtn"
        :title="t('task.files.download')"
        :aria-label="t('task.files.download')"
        @click="downloadAttachment(a)"
      >
        <n-icon :component="DownloadOutline" :size="15" />
      </button>
      <!-- Deleting a file can't be undone: the two-step button of package A. -->
      <ArmButton
        v-if="!readonly"
        :title="t('task.files.deleteConfirm', { name: a.filename })"
        :confirm="t('task.arm.delete')"
        testid="file-delete"
        @confirm="deleteAttachment(a.id)"
      />
    </div>
    <EmptyState
      v-if="!attachments.length"
      size="small"
      :icon="AttachOutline"
      :text="t('task.files.empty')"
    />
    <input v-if="!readonly" ref="fileInput" type="file" hidden @change="onFileChosen" />
    <button
      v-if="!readonly"
      type="button"
      class="kit-btn small f-attach"
      :disabled="uploading"
      data-testid="file-attach"
      @click="pickFile"
    >
      <TesseraSpinner v-if="uploading" :size="13" />
      <n-icon v-else :component="AttachOutline" :size="14" />
      {{ t('task.files.attach') }}
    </button>
  </div>
</template>

<style scoped>
@import './tab-shared.css';

.files {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.filerow {
  display: flex;
  align-items: center;
  gap: 8px;
  min-height: 34px;
  padding: 0 4px 0 8px;
  border-radius: 9px;
  font-size: 13px;
}
.filerow:hover {
  background: var(--t-hover);
}
.f-ibtn {
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  padding: 0;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--t-text3);
  cursor: pointer;
}
.f-ibtn:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.f-attach {
  align-self: flex-start;
  margin-top: 6px;
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
