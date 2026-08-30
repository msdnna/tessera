<script setup>
// Description block extracted from TaskModal (#2745). Rendered in the left column
// for the modal/fullscreen layouts, and as the first tab ("Описание") in the
// sidebar layout so a long description no longer buries the comments/subtasks tabs.
// The parent owns persistence (saveDesc), attachment reloads and GitLab template
// fetching — this component only renders the editor/preview and relays events.
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon, NSelect } from 'naive-ui'
import {
  ImageOutline,
  GitNetworkOutline,
  ExpandOutline,
  EyeOutline,
  CreateOutline,
} from '@vicons/ionicons5'
import RichContent from '../RichContent.vue'
import MarkdownEditor from '../MarkdownEditor.vue'

const props = defineProps({
  modelValue: { type: String, default: '' },
  readonly: { type: Boolean, default: false },
  members: { type: Array, default: () => [] },
  taskId: { type: String, default: null },
  initialMode: { type: String, default: 'write' },
  showTemplate: { type: Boolean, default: false },
  templateValue: { type: [String, null], default: null },
  templateOptions: { type: Array, default: () => [] },
})

const emit = defineEmits([
  'update:modelValue',
  'update:templateValue',
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

const template = computed({
  get: () => props.templateValue,
  set: (v) => emit('update:templateValue', v),
})

// The editor's toolbar lives in the section header (.desc-head); we drive it
// through the editor ref and mirror its write/preview mode to swap the toggle icon.
const descEditor = ref(null)
const descMode = ref(props.initialMode === 'preview' ? 'preview' : 'write')

// CSS `content` can't call t(), so the empty-preview hint is handed to the
// stylesheet as a variable. JSON.stringify supplies the quotes CSS needs and
// escapes anything awkward inside the translation.
const emptyDescVar = computed(() => JSON.stringify(t('task.desc.placeholder')))
</script>

<template>
  <div class="section" :style="{ '--t-desc-empty': emptyDescVar }">
    <div class="desc-head">
      <span class="slabel">{{ t('task.tab.description') }}</span>
      <div class="desc-head-r">
        <n-select
          v-if="showTemplate"
          v-model:value="template"
          :options="templateOptions"
          size="small"
          clearable
          :placeholder="t('task.desc.template')"
          class="tpl-select"
          @update:value="emit('apply-template', $event)"
        />
        <div v-if="!readonly" class="desc-acts">
          <template v-if="descMode === 'write'">
            <button
              class="desc-act"
              :title="t('task.desc.insertImage')"
              @click="descEditor?.pickImage()"
            >
              <n-icon :component="ImageOutline" :size="16" />
            </button>
            <button
              class="desc-act"
              :title="t('task.desc.insertMermaid')"
              @click="descEditor?.insertMermaid()"
            >
              <n-icon :component="GitNetworkOutline" :size="16" />
            </button>
          </template>
          <button
            class="desc-act"
            data-testid="desc-fullscreen"
            :title="t('task.desc.fullscreen')"
            @click="descEditor?.openFullscreen()"
          >
            <n-icon :component="ExpandOutline" :size="16" />
          </button>
          <button
            class="desc-act"
            :title="descMode === 'write' ? t('task.desc.preview') : t('common.action.edit')"
            @click="descEditor?.toggleMode()"
          >
            <n-icon :component="descMode === 'write' ? EyeOutline : CreateOutline" :size="16" />
          </button>
        </div>
      </div>
    </div>
    <RichContent
      v-if="readonly"
      :source="description || t('task.desc.emptyMarkdown')"
      :members="members"
      mention-cards
      task-refs
    />
    <MarkdownEditor
      v-else
      ref="descEditor"
      :key="taskId"
      v-model="description"
      :toolbar="false"
      :placeholder="t('task.desc.placeholder')"
      :min-rows="3"
      :initial-mode="initialMode"
      :attach-task-id="taskId"
      @update:mode="descMode = $event"
      @attachments-changed="emit('attachments-changed')"
      @blur="emit('blur')"
      @persist="emit('persist')"
    />
  </div>
</template>

<style scoped>
.section {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.slabel {
  font-size: 12px;
  color: var(--t-text3);
}
.desc-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}
.desc-head-r {
  display: flex;
  align-items: center;
  gap: 4px;
}
.desc-head .tpl-select {
  width: 200px;
}
.desc-acts {
  display: flex;
  align-items: center;
  gap: 2px;
}
.desc-act {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: none;
  background: transparent;
  color: var(--t-text2);
  cursor: pointer;
  padding: 4px 6px;
  border-radius: 6px;
  transition:
    background 0.12s ease,
    color 0.12s ease;
}
.desc-act:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}

/* rendered markdown (readonly description preview) */
.md {
  font-size: 14px;
  line-height: 1.55;
  color: var(--t-text1);
  word-break: break-word;
}
/* Belt-and-braces hint for a description that renders to nothing. The live
   empty path is the `task.desc.emptyMarkdown` fallback above, so this rarely
   fires — but CSS `content` cannot reach the catalogue, hence the variable the
   template fills in (a bare Russian literal here would survive any switch). */
.md:empty::before {
  content: var(--t-desc-empty, '');
  color: var(--t-text3);
}
.section > .md {
  padding: 8px 10px;
  border-radius: 8px;
  cursor: text;
  min-height: 40px;
}
.section > .md:hover {
  background: var(--t-surface-alt);
}
.md :deep(p) {
  margin: 0 0 8px;
}
.md :deep(p:last-child) {
  margin-bottom: 0;
}
.md :deep(ul),
.md :deep(ol) {
  margin: 0 0 8px;
  padding-left: 20px;
}
.md :deep(code) {
  background: var(--t-surface-alt);
  padding: 1px 5px;
  border-radius: 4px;
  font-size: 0.9em;
}
.md :deep(pre) {
  background: var(--t-surface-alt);
  padding: 10px;
  border-radius: 8px;
  overflow-x: auto;
}
.md :deep(a) {
  color: var(--t-primary);
}
.md :deep(blockquote) {
  margin: 0 0 8px;
  padding-left: 10px;
  border-left: 3px solid var(--t-border);
  color: var(--t-text2);
}
</style>
