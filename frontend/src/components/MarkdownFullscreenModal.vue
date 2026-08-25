<script setup>
// Distraction-free editing for a long description or comment: the same
// MarkdownEditor, given the whole window, on top of the task modal.
//
// The text itself stays in the caller's `v-model` — the modal edits through it
// rather than keeping a copy. A copy would have to be merged back on close, and
// the description saves on blur/persist, so any divergence would read as lost
// text. Loaded lazily by MarkdownEditor (they reference each other).
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { NModal, NIcon } from 'naive-ui'
import { CloseOutline } from '@vicons/ionicons5'
import MarkdownEditor from './MarkdownEditor.vue'

// `null` rather than a translated default: a prop default is evaluated once per
// instance, so a literal built from t() would keep the language the modal was
// opened in. The computed below reads the catalogue on every render instead.
const props = defineProps({
  show: { type: Boolean, default: false },
  modelValue: { type: String, default: '' },
  title: { type: String, default: null },
  placeholder: { type: String, default: null },
  mentionItems: { type: Array, default: () => [] },
  commandItems: { type: Array, default: () => [] },
  attachTaskId: { type: String, default: null },
})
const { t } = useI18n()
const headTitle = computed(() => props.title ?? t('documents.editor.fullscreen.title'))
const fieldPlaceholder = computed(() => props.placeholder ?? t('documents.editor.placeholder'))
const emit = defineEmits([
  'update:show',
  'update:modelValue',
  'attachments-changed',
  'persist',
  'after-leave',
])

function close() {
  emit('update:show', false)
  // The caller persists on blur; leaving the modal is that moment for it.
  emit('persist')
}
</script>

<template>
  <n-modal
    :show="show"
    class="mdfs-modal"
    :internal-appear="true"
    @update:show="$event ? emit('update:show', true) : close()"
    @after-leave="emit('after-leave')"
  >
    <div class="mdfs">
      <div class="mdfs-head">
        <span class="mdfs-title">{{ headTitle }}</span>
        <button
          type="button"
          class="mdfs-close"
          :title="$t('documents.editor.fullscreen.close')"
          @click="close"
        >
          <n-icon :component="CloseOutline" :size="18" />
        </button>
      </div>
      <div class="mdfs-body">
        <MarkdownEditor
          :model-value="modelValue"
          :placeholder="fieldPlaceholder"
          :mention-items="mentionItems"
          :command-items="commandItems"
          :attach-task-id="attachTaskId"
          split
          :expandable="false"
          @update:model-value="emit('update:modelValue', $event)"
          @attachments-changed="emit('attachments-changed')"
          @persist="emit('persist')"
        />
      </div>
    </div>
  </n-modal>
</template>

<style scoped>
.mdfs {
  display: flex;
  flex-direction: column;
  width: min(1100px, 94vw);
  height: min(880px, 90vh);
  background: var(--t-surface);
  border: 1px solid var(--t-border);
  border-radius: 12px;
  overflow: hidden;
}
.mdfs-head {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 14px;
  border-bottom: 1px solid var(--t-border);
}
.mdfs-title {
  flex: 1;
  font-size: 13px;
  font-weight: 600;
  color: var(--t-text2);
}
.mdfs-close {
  display: inline-flex;
  align-items: center;
  border: none;
  background: transparent;
  color: var(--t-text2);
  border-radius: 6px;
  padding: 4px;
  cursor: pointer;
  transition:
    background 0.12s ease,
    color 0.12s ease;
}
.mdfs-close:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
/* The editor owns the scrolling here (each split pane scrolls on its own), so the
   body just hands it the leftover height — an `overflow: auto` here would give the
   modal a second scrollbar and let the bottom toolbar drift out of view. */
.mdfs-body {
  flex: 1;
  min-height: 0;
  display: flex;
  padding: 12px 16px 14px;
}
.mdfs-body > * {
  flex: 1;
  min-width: 0;
}
</style>
