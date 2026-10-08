<script setup>
// «Документы» tab of the task modal (#2732): the other end of the link the
// documents panel creates. Read-mostly by design — a link is made where the
// context is, next to the clause it is about, and this side is where you find
// out which documents mention this task and open them.
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import { CloseOutline, DocumentTextOutline } from '@vicons/ionicons5'
import { documents as docsApi } from '@/api'
import EmptyState from '../EmptyState.vue'
import ArmButton from '../kit/ArmButton.vue'

const props = defineProps({
  links: { type: Array, default: () => [] },
  // Which task the shown links belong to. Carried only to tell a reply apart
  // from the task it was asked for — see unlink below.
  taskId: { type: String, default: null },
  // The modal opened from the archive is read-only. Opening the document stays
  // — this tab is read-mostly anyway — but unlinking it does not (#2932).
  readonly: { type: Boolean, default: false },
})
const emit = defineEmits(['update:links', 'open-document'])
const { t } = useI18n()

// Unlinking goes through the link row's own endpoint — one row, one id,
// whichever end you are looking at it from.
async function unlink(id) {
  if (props.readonly) return
  // The modal is one per board and keeps this subtree alive when the task under
  // it changes, so by the time the reply lands `props.links` may already be the
  // other task's list — and publishing the filtered copy back would overwrite it
  // with a list nobody asked for (#2932). The unlink itself stands either way.
  const tid = props.taskId
  await docsApi.unlinkTask(id)
  if (tid !== props.taskId) return
  emit(
    'update:links',
    props.links.filter((l) => l.id !== id),
  )
}
</script>

<template>
  <div class="docs">
    <div v-for="l in links" :key="l.id" class="docrow">
      <button
        type="button"
        class="doc-link"
        data-testid="task-doc-link"
        @click="emit('open-document', l)"
      >
        <span v-if="l.document_icon" class="doc-emoji">{{ l.document_icon }}</span>
        <n-icon v-else :component="DocumentTextOutline" :size="14" />
        <span class="doc-title">{{ l.document_title || t('task.docs.untitled') }}</span>
        <!-- An anchored link says which clause it is about; the quote is what
             still answers that once the clause has been rewritten. -->
        <span v-if="l.block_id" class="doc-anchor" :title="l.quote">
          {{ l.quote || t('task.docs.fragment') }}
        </span>
      </button>
      <ArmButton
        v-if="!readonly"
        :icon="CloseOutline"
        :title="t('task.docs.unlink')"
        :confirm="t('task.arm.unlink')"
        testid="doc-unlink"
        @confirm="unlink(l.id)"
      />
    </div>
    <EmptyState
      v-if="!links.length"
      size="small"
      :icon="DocumentTextOutline"
      :text="t('task.docs.empty')"
    />
  </div>
</template>

<style scoped>
@import './tab-shared.css';

.docs {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.docrow {
  display: flex;
  align-items: center;
  gap: 8px;
}
.doc-link {
  display: flex;
  align-items: center;
  gap: 6px;
  flex: 1;
  min-width: 0;
  border: none;
  background: none;
  cursor: pointer;
  text-align: left;
  padding: 4px 6px;
  border-radius: 6px;
  color: var(--t-text1);
  font-size: 13px;
}
.doc-link:hover {
  background: var(--t-hover);
}
.doc-emoji {
  flex: none;
  font-size: 14px;
  line-height: 1;
}
.doc-title {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.doc-anchor {
  flex: none;
  max-width: 40%;
  color: var(--t-text3);
  font-size: 11px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
