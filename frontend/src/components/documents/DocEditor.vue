<script setup>
import { onBeforeUnmount, ref, shallowRef, watch } from 'vue'
import { EditorContent, Editor } from '@tiptap/vue-3'
import { NButton, NIcon, NInput, useMessage } from 'naive-ui'
import { CheckmarkOutline, CloseOutline } from '@vicons/ionicons5'
import { docExtensions, toDocJSON } from '@/utils/docSchema'
import { BLOCK_ID_META, ensureBlockIds } from '@/utils/docExtensions/blockId'
import DocToolbar from './DocToolbar.vue'

const props = defineProps({
  modelValue: { type: Object, default: null },
  editable: { type: Boolean, default: true },
  placeholder: { type: String, default: 'Начните писать…' },
  // Uploads a File and resolves to the URL to embed. Injected rather than
  // imported so the editor stays free of API knowledge (and testable offline).
  uploadImage: { type: Function, default: null },
})
const emit = defineEmits(['update:modelValue', 'change', 'blur'])

const message = useMessage()
const editor = shallowRef(null)
const fileInput = ref(null)
const linkOpen = ref(false)
const linkValue = ref('')
const uploading = ref(false)

// Tracks the JSON we last emitted, so the watcher below can tell "the parent
// loaded a different document" from "the parent echoed back our own edit".
let lastEmitted = null

function build() {
  editor.value = new Editor({
    content: ensureBlockIds(toDocJSON(props.modelValue)),
    editable: props.editable,
    extensions: docExtensions({ placeholder: props.placeholder }),
    onUpdate: ({ editor: e, transaction }) => {
      // The BlockId extension stamps missing ids on load. That is bookkeeping,
      // not a user edit: emitting it would mark a freshly opened document dirty
      // and trigger an autosave nobody asked for.
      if (transaction?.getMeta(BLOCK_ID_META)) return
      const json = e.getJSON()
      lastEmitted = json
      emit('update:modelValue', json)
      emit('change', json)
    },
    onBlur: () => emit('blur'),
  })
}

build()

watch(
  () => props.modelValue,
  (next) => {
    if (!editor.value || next === lastEmitted) return
    const current = editor.value.getJSON()
    if (JSON.stringify(current) === JSON.stringify(next)) return
    // emitUpdate: false — loading a document is not an edit and must not start
    // an autosave cycle (which would then race the load it came from).
    editor.value.commands.setContent(ensureBlockIds(toDocJSON(next)), { emitUpdate: false })
  },
)

watch(
  () => props.editable,
  (v) => editor.value?.setEditable(v),
)

onBeforeUnmount(() => {
  editor.value?.destroy()
  editor.value = null
})

function pickImage() {
  fileInput.value?.click?.()
}

async function onFile(e) {
  const file = e.target.files && e.target.files[0]
  e.target.value = ''
  if (!file || !props.uploadImage) return
  uploading.value = true
  try {
    const url = await props.uploadImage(file)
    if (url) editor.value?.chain().focus().setImage({ src: url, alt: file.name }).run()
  } catch (err) {
    message.error(err.message || 'Не удалось загрузить изображение')
  } finally {
    uploading.value = false
  }
}

function openLink() {
  linkValue.value = editor.value?.getAttributes('link')?.href || ''
  linkOpen.value = true
}

function applyLink() {
  const href = linkValue.value.trim()
  const chain = editor.value?.chain().focus().extendMarkRange('link')
  if (!chain) return
  if (href) chain.setLink({ href }).run()
  else chain.unsetLink().run()
  linkOpen.value = false
}

defineExpose({ editor })
</script>

<template>
  <div class="doc-editor" :class="{ readonly: !editable }">
    <doc-toolbar
      v-if="editable"
      :editor="editor"
      :disabled="uploading"
      @pick-image="pickImage"
      @set-link="openLink"
    />
    <div v-if="linkOpen" class="link-row">
      <n-input
        v-model:value="linkValue"
        size="small"
        placeholder="https://…"
        @keyup.enter="applyLink"
      />
      <n-button size="small" quaternary title="Применить" @click="applyLink">
        <template #icon><n-icon :component="CheckmarkOutline" /></template>
      </n-button>
      <n-button size="small" quaternary title="Отмена" @click="linkOpen = false">
        <template #icon><n-icon :component="CloseOutline" /></template>
      </n-button>
    </div>
    <!-- .md carries the app-wide markdown typography (code, pre, img, links from
         main.css). Reusing it is the point: a document whose code blocks and
         links look different from a task description is exactly the mismatch
         this task set out to avoid. -->
    <editor-content class="doc-content md" :editor="editor" />
    <input ref="fileInput" type="file" accept="image/*" class="hidden-file" @change="onFile" />
  </div>
</template>

<style scoped>
.doc-editor {
  display: flex;
  flex-direction: column;
  min-height: 0;
  flex: 1;
}
.link-row {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 6px 0;
}
.doc-content {
  flex: 1;
  overflow-y: auto;
  padding: 12px 2px;
  min-height: 0;
}
.hidden-file {
  display: none;
}

/* ProseMirror renders plain DOM, so naive-ui's themeOverrides never reach it —
   the CSS custom properties are the only channel the theme has here. Every
   colour below therefore comes from a --t-* token with no literal fallback. */
.doc-content :deep(.ProseMirror) {
  outline: none;
  min-height: 240px;
  color: var(--t-text1);
  line-height: 1.6;
}
.doc-content :deep(.ProseMirror p.is-editor-empty:first-child::before) {
  content: attr(data-placeholder);
  float: left;
  height: 0;
  pointer-events: none;
  color: var(--t-placeholder);
}
.doc-content :deep(.ProseMirror h1),
.doc-content :deep(.ProseMirror h2),
.doc-content :deep(.ProseMirror h3) {
  color: var(--t-text1);
  margin: 14px 0 6px;
  line-height: 1.3;
}
.doc-content :deep(.ProseMirror blockquote) {
  border-left: 2px solid var(--t-border);
  padding-left: 10px;
  color: var(--t-text2);
  margin: 8px 0;
}
.doc-content :deep(.ProseMirror ul[data-type='taskList']) {
  list-style: none;
  padding-left: 4px;
}
.doc-content :deep(.ProseMirror ul[data-type='taskList'] li) {
  display: flex;
  align-items: flex-start;
  gap: 6px;
}
.doc-content :deep(.ProseMirror table) {
  border-collapse: collapse;
  width: 100%;
  margin: 10px 0;
  table-layout: fixed;
}
.doc-content :deep(.ProseMirror th),
.doc-content :deep(.ProseMirror td) {
  border: 1px solid var(--t-border);
  padding: 6px 8px;
  vertical-align: top;
}
.doc-content :deep(.ProseMirror th) {
  background: var(--t-surface-alt);
  font-weight: 600;
  text-align: left;
}
.doc-content :deep(.ProseMirror hr) {
  border: none;
  border-top: 1px solid var(--t-border);
  margin: 14px 0;
}
.doc-content :deep(.ProseMirror .selectedCell::after) {
  content: '';
  position: absolute;
  inset: 0;
  background: var(--t-hover);
  pointer-events: none;
}
</style>
