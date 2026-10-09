<script setup>
// The task's description (v2 — #3113, and the editor of #3118, PackageJ
// «Описание»). Read as prose; a click on the text (not on a link, a checkbox, a
// mention, a picture or a diagram), «Изменить I» on hover, or I (⇧E — its key of
// #3113 — still works) opens the v2 editor in place: the Написать / Просмотр
// segment with the tools, the field, and «Отмена Esc · Сохранить ⌘↵». Esc with
// changes asks «Отменить правки?» (↵ throws them away, Esc goes on). An empty
// description is a single «Добавить описание…» line, not an open editor.
//
// Selecting read text raises the bubble «Цитировать в ответ Q · Ссылка на
// фрагмент» (EdReadBubble).
//
// The parent owns persistence (`persist` — the description saves on blur as it
// always did, so nothing typed is lost to a click elsewhere), attachment reloads
// and the GitLab templates; this component owns the read/edit state and the copy
// it reverts to when the edit is cancelled. A write-back conflict on the
// description that surfaces while it is being edited, or right after a save
// (someone changed it in GitLab meanwhile), opens the conflicts window (package
// F) on this task.
import { ref, computed, nextTick, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import { CreateOutline } from '@vicons/ionicons5'
import RichContent from '../RichContent.vue'
import MarkdownEditor from '../MarkdownEditor.vue'
import ShortcutKeys from '../ShortcutKeys.vue'
import EdReadBubble from '../editor/EdReadBubble.vue'
import { toggleTaskMarker } from '@/utils/markdown'
import { isEditClick } from '@/utils/descEdit'
import { useConflictsStore } from '@/stores/conflicts'

const props = defineProps({
  modelValue: { type: String, default: '' },
  readonly: { type: Boolean, default: false },
  // @-mention rows (buildMentionItems) — offered in the editor and resolved by
  // the hover cards of the read text. No /-commands: those run in comments only
  // (the server never reads a description for them).
  members: { type: Array, default: () => [] },
  // The task's own people — first in @ ([{ id, role }]).
  people: { type: Array, default: () => [] },
  // { number, icon, color } — the fullscreen window's header.
  editorTask: { type: Object, default: null },
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
const conflicts = useConflictsStore()

const description = computed({
  get: () => props.modelValue,
  set: (v) => emit('update:modelValue', v),
})

const editing = ref(false)
const editor = ref(null)
const proseEl = ref(null)
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
// A save that GitLab may answer with a conflict: watched for a minute after it.
const CONFLICT_WATCH_MS = 60000
let watchUntil = 0
function finish() {
  editing.value = false
  watchUntil = Date.now() + CONFLICT_WATCH_MS
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
    watchUntil = 0
  },
)

// A parallel edit from GitLab: the write-back of this description met a newer
// one there. The conflicts window takes it from here (package F, «Конфликты»).
const descConflict = computed(
  () =>
    (conflicts.list || []).find(
      (c) => c.task_id === props.taskId && (c.fields || []).includes('description'),
    ) || null,
)
watch(
  () => descConflict.value?.id,
  (id, prev) => {
    if (!id || id === prev) return
    if (editing.value || Date.now() < watchUntil) conflicts.openResolver(props.taskId)
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
        :task-people="people"
        :attach-task-id="taskId"
        :templates="templates"
        :template-label="t('task.desc.template')"
        :discard-text="t('task.desc.discard')"
        :full-task="editorTask"
        @template="emit('apply-template', $event)"
        @attachments-changed="emit('attachments-changed')"
        @blur="emit('blur')"
        @persist="emit('persist')"
        @submit="finish"
        @cancel="cancel"
      />
    </template>
    <template v-else-if="modelValue && modelValue.trim()">
      <!-- A wrapper, not a class on RichContent: it renders a fragment. The text
           is the door to the editor: it tints on hover, and «Изменить I» shows
           in its corner (PackageJ «Описание · чтение, ховер»). -->
      <div
        ref="proseEl"
        class="desc-prose"
        :class="{ editable: !readonly }"
        :title="readonly ? undefined : t('task.desc.editHint')"
        data-testid="desc-prose"
        @click="onProseClick"
      >
        <button
          v-if="!readonly"
          type="button"
          class="desc-edit"
          data-testid="desc-edit"
          @click.stop="startEdit"
        >
          <n-icon :component="CreateOutline" :size="13" class="desc-edit-ic" />
          {{ t('task.desc.edit') }}
          <ShortcutKeys id="task.editDesc" />
        </button>
        <RichContent
          :source="modelValue"
          :members="members"
          mention-cards
          task-refs
          :interactive="!readonly"
          @toggle="onToggle"
        />
      </div>
      <EdReadBubble :target="() => proseEl" :task-id="taskId" :can-quote="!readonly" />
    </template>
    <button
      v-else-if="!readonly"
      type="button"
      class="desc-empty"
      data-testid="desc-add"
      @click="startEdit"
    >
      <span class="desc-empty-t">{{ t('task.desc.add') }}</span>
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
  /* The hover tint reaches past the text without moving it. */
  margin: -6px -10px;
  padding: 6px 10px;
  cursor: text;
  transition: background 0.12s ease;
}
.desc-prose.editable:hover {
  background: color-mix(in srgb, var(--t-hover) 60%, transparent);
}
.desc-prose {
  position: relative;
}
/* «Изменить I» in the corner of the hovered text (PackageJ «Описание · чтение,
   ховер»). */
.desc-edit {
  position: absolute;
  right: 8px;
  top: 6px;
  z-index: 1;
  display: flex;
  align-items: center;
  gap: 7px;
  height: 28px;
  padding: 0 5px 0 9px;
  border: 0;
  border-radius: 8px;
  background: var(--t-ctl);
  color: var(--t-text1);
  font: inherit;
  font-size: 12.5px;
  cursor: pointer;
  opacity: 0;
  transition: opacity 0.12s ease;
}
.desc-prose:hover .desc-edit,
.desc-edit:focus-visible {
  opacity: 1;
}
.desc-edit-ic {
  color: var(--t-text2);
}
@media (hover: none) {
  .desc-edit {
    display: none;
  }
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
.desc-prose :deep(.md p) {
  margin: 0 0 8px;
}
.desc-prose :deep(.md ul),
.desc-prose :deep(.md ol) {
  margin: 4px 0 8px;
  padding-left: 22px;
}
.desc-prose :deep(.md li) {
  margin: 4px 0;
}
.desc-prose :deep(.md li > p) {
  margin: 0;
}
.desc-prose :deep(.md li::marker) {
  color: var(--t-text3);
}
/* A ticked list written loose («- [ ] …» after a blank line) wraps its box in a
   paragraph — RichContent's own rule only sees a bare one. */
.desc-prose :deep(.md li:has(> p > input[type='checkbox'])) {
  list-style: none;
}
.desc-prose :deep(.md code) {
  padding: 1px 5px;
  border-radius: 5px;
  background: var(--t-pill);
  color: var(--t-text1);
  font-size: 12.5px;
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
.desc-empty-t {
  flex: 1;
  min-width: 0;
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
  .desc-key {
    display: none;
  }
}
</style>
