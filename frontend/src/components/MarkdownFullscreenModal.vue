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
import { CloseOutline, ExpandOutline } from '@vicons/ionicons5'
import MarkdownEditor from './MarkdownEditor.vue'
import Kbd from './Kbd.vue'
import { CAPS } from '@/utils/keyCaps'

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
    <!-- v2 window (#3113): the large-window head — 56px, title, close, Esc. -->
    <div class="mdfs" data-testid="md-fullscreen">
      <header class="mdfs-head">
        <span class="mdfs-badge"><n-icon :component="ExpandOutline" :size="13" /></span>
        <span class="mdfs-title">{{ headTitle }}</span>
        <span class="mdfs-grow" />
        <button
          type="button"
          class="mdfs-close"
          :title="$t('documents.editor.fullscreen.close')"
          :aria-label="$t('documents.editor.fullscreen.close')"
          @click="close"
        >
          <n-icon :component="CloseOutline" :size="18" />
        </button>
        <Kbd>{{ CAPS.escape }}</Kbd>
      </header>
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
  border-radius: var(--t-r-col);
  background: var(--t-pop);
  box-shadow: var(--t-sh-drag);
  color: var(--t-text1);
  overflow: hidden;
  --t-surface: var(--t-pop);
}
.mdfs-head {
  flex: none;
  display: flex;
  align-items: center;
  gap: 10px;
  height: 56px;
  padding: 0 16px 0 20px;
  box-sizing: border-box;
}
.mdfs-badge {
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border-radius: 7px;
  background: var(--t-accent-grad);
  color: #fff;
}
.mdfs-title {
  min-width: 0;
  font-size: 15px;
  font-weight: 650;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.mdfs-grow {
  flex: 1;
}
.mdfs-close {
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  border: 0;
  border-radius: var(--t-r-ctl);
  background: transparent;
  color: var(--t-text2);
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
  padding: 0 20px 18px;
}
/* The split editor inside: no v1 lines — the panes sit on the input surface,
   the toolbar is a v2 strip. */
.mdfs .mdfs-body :deep(.md2-write textarea),
.mdfs .mdfs-body :deep(.md2-preview-side) {
  padding: 12px 14px;
  border-radius: 10px;
  box-sizing: border-box;
}
.mdfs .mdfs-body :deep(.md2-write textarea) {
  background: var(--t-input-bg);
}
.mdfs .mdfs-body :deep(.md2-preview-side) {
  border-left: 0;
  background: color-mix(in srgb, var(--t-input-bg) 45%, transparent);
}
.mdfs .mdfs-body :deep(.md2-toolbar) {
  margin-top: 10px;
  padding: 3px;
  border-top: 0;
  border-radius: 9px;
  background: var(--t-input-bg);
}
.mdfs .mdfs-body :deep(.md2-tsep) {
  align-self: center;
  height: 16px;
  margin: 0 3px;
}
@media (max-width: 720px) {
  .mdfs .mdfs-body :deep(.md2-preview-side) {
    border-top: 0;
    margin-top: 10px;
  }
}
.mdfs-body > * {
  flex: 1;
  min-width: 0;
}
</style>
