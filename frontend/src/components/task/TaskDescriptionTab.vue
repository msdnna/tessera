<script setup>
// The task's description (v2, #3113 — TaskModal / StagePage mockups). Read as
// prose; a click on the text (not on a link, a checkbox, a mention, a picture or
// a diagram) or ⇧E opens the v2 editor in place: the Написать / Просмотр bar with
// the tools, the ringed field and the «Отмена Esc · Сохранить ⌘↵» footer. An
// empty description is a single «Добавить описание…» line, not an open editor.
//
// The parent owns persistence (`persist` — the description saves on blur as it
// always did, so nothing typed is lost to a click elsewhere), attachment reloads
// and the GitLab templates; this component owns the read/edit state and the copy
// it reverts to when the edit is cancelled.
import { ref, computed, nextTick, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import { CreateOutline } from '@vicons/ionicons5'
import RichContent from '../RichContent.vue'
import MarkdownEditor from '../MarkdownEditor.vue'
import ShortcutKeys from '../ShortcutKeys.vue'
import { toggleTaskMarker } from '@/utils/markdown'
import { isEditClick } from '@/utils/descEdit'

const props = defineProps({
  modelValue: { type: String, default: '' },
  readonly: { type: Boolean, default: false },
  // @-mention rows (buildMentionItems) — offered in the editor and resolved by
  // the hover cards of the read text. No /-commands: those run in comments only
  // (the server never reads a description for them).
  members: { type: Array, default: () => [] },
  taskId: { type: String, default: null },
  // GitLab issue templates ([{ name }]) offered from the editor's bar.
  templates: { type: Array, default: () => [] },
})

const emit = defineEmits([
  'update:modelValue',
  'apply-template',
  'attachments-changed',
  'blur',
  'persist',
])

const { t } = useI18n()

const description = computed({
  get: () => props.modelValue,
  set: (v) => emit('update:modelValue', v),
})

const editing = ref(false)
const editor = ref(null)
// The text as it was when editing began — what «Отмена» goes back to. The
// editor asks before discarding; by then a blur may already have saved the
// typing, so the revert is persisted too.
const original = ref('')

async function startEdit() {
  if (props.readonly || editing.value) return
  original.value = props.modelValue || ''
  editing.value = true
  await nextTick()
  editor.value?.focus()
}
function finish() {
  editing.value = false
  emit('persist')
}
function cancel() {
  const changed = (props.modelValue || '') !== original.value
  editing.value = false
  if (changed) {
    description.value = original.value
    emit('persist')
  }
}
// A different task in the same (never remounted) modal starts in reading mode.
watch(
  () => props.taskId,
  () => {
    editing.value = false
  },
)

function onProseClick(e) {
  if (!isEditClick(e)) return
  startEdit()
}
// The checkboxes of a read description stay live: a tick rewrites the marker and
// saves, without opening the editor.
function onToggle(i) {
  description.value = toggleTaskMarker(props.modelValue, i)
  emit('persist')
}

defineExpose({ startEdit, editing })
</script>

<template>
  <div class="desc" :class="{ editing }">
    <template v-if="editing">
      <MarkdownEditor
        ref="editor"
        v-model="description"
        variant="v2"
        :placeholder="t('task.desc.placeholder')"
        :min-rows="4"
        :mention-items="members"
        :attach-task-id="taskId"
        :templates="templates"
        :template-label="t('task.desc.template')"
        @template="emit('apply-template', $event)"
        @attachments-changed="emit('attachments-changed')"
        @blur="emit('blur')"
        @persist="emit('persist')"
        @submit="finish"
        @cancel="cancel"
      />
    </template>
    <template v-else-if="modelValue && modelValue.trim()">
      <!-- A wrapper, not a class on RichContent: it renders a fragment. -->
      <div
        class="desc-prose"
        :class="{ editable: !readonly }"
        data-testid="desc-prose"
        @click="onProseClick"
      >
        <RichContent
          :source="modelValue"
          :members="members"
          mention-cards
          task-refs
          :interactive="!readonly"
          @toggle="onToggle"
        />
      </div>
      <button
        v-if="!readonly"
        type="button"
        class="desc-edit"
        data-testid="desc-edit"
        :title="t('task.desc.edit')"
        @click="startEdit"
      >
        <n-icon :component="CreateOutline" :size="14" />
        {{ t('task.desc.edit') }}
        <ShortcutKeys id="task.editDesc" />
      </button>
    </template>
    <button
      v-else-if="!readonly"
      type="button"
      class="desc-empty"
      data-testid="desc-add"
      @click="startEdit"
    >
      <span>{{ t('task.desc.add') }}</span>
      <ShortcutKeys id="task.editDesc" class="desc-key" />
    </button>
    <div v-else class="desc-none">{{ t('task.desc.none') }}</div>
  </div>
</template>

<style scoped>
.desc {
  position: relative;
}
/* Reading: the mockup's prose (TaskModal L38) — 14.5/1.6 in the second text
   colour, a 640px measure. */
.desc-prose {
  max-width: 640px;
  font-size: 14.5px;
  line-height: 1.6;
  color: var(--t-text2);
  border-radius: 10px;
}
.desc-prose.editable {
  cursor: text;
}
.desc-prose :deep(.md) {
  font-size: inherit;
  line-height: inherit;
  color: inherit;
}
.desc-prose :deep(.md > :first-child) {
  margin-top: 0;
}
.desc-prose :deep(.md > :last-child) {
  margin-bottom: 0;
}
.desc-prose :deep(.md h1),
.desc-prose :deep(.md h2),
.desc-prose :deep(.md h3),
.desc-prose :deep(.md h4) {
  margin: 12px 0 6px;
  font-size: 14.5px;
  font-weight: 650;
  color: var(--t-text1);
}
.desc-prose :deep(.md h1) {
  font-size: 16px;
}
.desc-prose :deep(.md strong) {
  color: var(--t-text1);
}
.desc-prose :deep(.md code) {
  padding: 1px 5px;
  border-radius: 5px;
  background: var(--t-pill);
  color: var(--t-text1);
  font-size: 12.5px;
}
/* «Изменить ⇧E»: a quiet door at the top right, shown on hover / focus. */
.desc-edit {
  position: absolute;
  top: -4px;
  right: 0;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 28px;
  padding: 0 5px 0 9px;
  border: 0;
  border-radius: 8px;
  background: var(--t-ctl);
  color: var(--t-text2);
  font: inherit;
  font-size: 12.5px;
  font-weight: 500;
  cursor: pointer;
  opacity: 0;
  transition: opacity 0.12s ease;
}
.desc:hover .desc-edit,
.desc-edit:focus-visible {
  opacity: 1;
}
.desc-edit:hover {
  color: var(--t-text1);
}
/* Empty: one line that reads as a field to fill, not an open editor. */
.desc-empty {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  max-width: 640px;
  height: 38px;
  padding: 0 8px 0 12px;
  box-sizing: border-box;
  border: 0;
  border-radius: 10px;
  background: transparent;
  color: var(--t-placeholder);
  font: inherit;
  font-size: 14.5px;
  text-align: left;
  cursor: text;
  transition: background 0.12s ease;
}
.desc-empty > span {
  flex: 1;
}
.desc-empty:hover,
.desc-empty:focus-visible {
  background: var(--t-input-bg);
  outline: 0;
}
.desc-key {
  opacity: 0;
  transition: opacity 0.12s ease;
}
.desc-empty:hover .desc-key,
.desc-empty:focus-visible .desc-key {
  opacity: 1;
}
.desc-none {
  font-size: 14px;
  color: var(--t-text3);
}
@media (hover: none) {
  .desc-edit,
  .desc-key {
    display: none;
  }
}
</style>
