<script setup>
// «Документы» tab of the task modal (#2732): the other end of the link the
// documents panel creates. Read-mostly by design — a link is made where the
// context is, next to the clause it is about, and this side is where you find
// out which documents mention this task and open them.
import { NIcon, NPopconfirm } from 'naive-ui'
import { CloseOutline, DocumentTextOutline } from '@vicons/ionicons5'
import { documents as docsApi } from '@/api'
import EmptyState from '../EmptyState.vue'

const props = defineProps({
  links: { type: Array, default: () => [] },
})
const emit = defineEmits(['update:links', 'open-document'])

// Unlinking goes through the link row's own endpoint — one row, one id,
// whichever end you are looking at it from.
async function unlink(id) {
  await docsApi.unlinkTask(id)
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
        <span class="doc-title">{{ l.document_title || 'Без названия' }}</span>
        <!-- An anchored link says which clause it is about; the quote is what
             still answers that once the clause has been rewritten. -->
        <span v-if="l.block_id" class="doc-anchor" :title="l.quote">
          {{ l.quote || 'фрагмент' }}
        </span>
      </button>
      <n-popconfirm
        :positive-button-props="{ type: 'error' }"
        positive-text="Убрать"
        @positive-click="unlink(l.id)"
      >
        <template #trigger>
          <button class="c-act" title="Убрать связь">
            <n-icon :component="CloseOutline" />
          </button>
        </template>
        Убрать связь с документом?
      </n-popconfirm>
    </div>
    <EmptyState
      v-if="!links.length"
      size="small"
      :icon="DocumentTextOutline"
      text="Связанных документов нет"
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
