<script setup>
import { onBeforeUnmount, reactive, ref, shallowRef, watch } from 'vue'
import { EditorContent, Editor } from '@tiptap/vue-3'
import { NButton, NIcon, NInput, useMessage } from 'naive-ui'
import { AddOutline, CheckmarkOutline, CloseOutline, ReorderTwoOutline } from '@vicons/ionicons5'
import { docExtensions, toDocJSON } from '@/utils/docSchema'
import { BLOCK_ID_META, ensureBlockIds } from '@/utils/docExtensions/blockId'
import {
  blockAtClientY,
  endBlockDrag,
  startBlockDrag,
  topLevelBlocks,
} from '@/utils/docExtensions/dragHandle'
import { slashState } from '@/utils/docExtensions/slashMenu'
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
const surface = ref(null)
const fileInput = ref(null)
const linkOpen = ref(false)
const linkValue = ref('')
const uploading = ref(false)

// The gutter that carries the drag handle. `pos` is the document position of
// the block it currently addresses — everything the handle does needs it.
const handle = reactive({ visible: false, top: 0, pos: 0, index: 0 })
const slash = reactive({ active: false, items: [], index: 0, left: 0, top: 0 })

// Tracks the JSON we last emitted, so the watcher below can tell "the parent
// loaded a different document" from "the parent echoed back our own edit".
let lastEmitted = null

function build() {
  editor.value = new Editor({
    content: ensureBlockIds(toDocJSON(props.modelValue)),
    editable: props.editable,
    extensions: docExtensions({
      placeholder: props.placeholder,
      uploadImage: (file) => props.uploadImage?.(file),
      onUploadError: (err) => message.error(err?.message || 'Не удалось загрузить изображение'),
      // The "Изображение" entry of the slash menu opens the picker below; the
      // menu itself has no business knowing about file inputs.
      onSlashExternal: () => pickImage(),
    }),
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
    onTransaction: ({ editor: e }) => syncSlash(e),
    onBlur: () => emit('blur'),
  })
}

build()

// The slash menu lives in the plugin; this mirrors it into Vue state and puts
// the popup under the "/" the user typed.
function syncSlash(e) {
  const s = slashState(e.state)
  slash.active = s.active
  slash.items = s.items
  slash.index = s.index
  if (!s.active || !surface.value) return
  try {
    const coords = e.view.coordsAtPos(s.from)
    const box = surface.value.getBoundingClientRect()
    slash.left = coords.left - box.left
    slash.top = coords.bottom - box.top + 4
  } catch {
    // coordsAtPos throws while the view is being torn down; the popup is about
    // to disappear anyway.
    slash.active = false
  }
}

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
  (v) => {
    editor.value?.setEditable(v)
    if (!v) handle.visible = false
  },
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

/* ---- drag handle -------------------------------------------------------- */

function onSurfaceMove(e) {
  const view = editor.value?.view
  if (!view || !props.editable) return
  const found = blockAtClientY(topLevelBlocks(view), e.clientY)
  if (!found || !surface.value) {
    handle.visible = false
    return
  }
  handle.visible = true
  handle.top = found.rect.top - surface.value.getBoundingClientRect().top
  handle.pos = found.pos
  handle.index = found.index
}

function onHandleDragStart(e) {
  const view = editor.value?.view
  if (!view || !startBlockDrag(view, handle.pos, e)) e.preventDefault()
}

function onHandleDragEnd() {
  endBlockDrag(editor.value?.view)
}

// Clicking the grip selects the block, which is also what makes the keyboard
// move (Alt+Shift+Arrow) address the block the pointer is on.
function selectBlock() {
  const view = editor.value?.view
  if (!view) return
  editor.value.chain().focus().setNodeSelection(handle.pos).run()
}

// The "+" inserts an empty paragraph after the hovered block and types the
// slash into it, so the same menu serves both entry points.
function insertBelow() {
  const view = editor.value?.view
  if (!view) return
  const node = view.state.doc.nodeAt(handle.pos)
  if (!node) return
  const end = handle.pos + node.nodeSize
  editor.value
    .chain()
    .focus()
    .insertContentAt(end, { type: 'paragraph' })
    .setTextSelection(end + 1)
    .insertContent('/')
    .run()
}

// The content box scrolls under a gutter that does not, so both anchors go
// stale the moment it moves.
function onScroll() {
  handle.visible = false
  if (editor.value) syncSlash(editor.value)
}

function runSlash(item) {
  editor.value?.commands.slashRun(item)
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
    <div
      ref="surface"
      class="doc-surface"
      @mousemove="onSurfaceMove"
      @mouseleave="handle.visible = false"
    >
      <div v-show="handle.visible" class="doc-gutter" :style="{ top: `${handle.top}px` }">
        <button
          type="button"
          class="gutter-btn"
          title="Вставить блок ниже"
          @mousedown.prevent="insertBelow"
        >
          <n-icon :component="AddOutline" :size="14" />
        </button>
        <button
          type="button"
          class="gutter-btn grip"
          draggable="true"
          title="Перетащить блок"
          @dragstart="onHandleDragStart"
          @dragend="onHandleDragEnd"
          @click="selectBlock"
        >
          <n-icon :component="ReorderTwoOutline" :size="16" />
        </button>
      </div>
      <!-- .md carries the app-wide markdown typography (code, pre, img, links from
           main.css). Reusing it is the point: a document whose code blocks and
           links look different from a task description is exactly the mismatch
           this task set out to avoid. -->
      <editor-content class="doc-content md" :editor="editor" @scroll="onScroll" />
      <div
        v-if="slash.active"
        class="slash-menu"
        :style="{ left: `${slash.left}px`, top: `${slash.top}px` }"
      >
        <button
          v-for="(item, i) in slash.items"
          :key="item.key"
          type="button"
          class="slash-item"
          :class="{ on: i === slash.index }"
          @mousedown.prevent="runSlash(item)"
        >
          <n-icon :component="item.icon" :size="15" />
          <span class="slash-label">{{ item.label }}</span>
          <span class="slash-hint">{{ item.hint }}</span>
        </button>
      </div>
    </div>
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
/* The gutter and the slash popup are absolutely positioned against this box,
   so both anchors are computed from its rect. */
.doc-surface {
  position: relative;
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
}
.doc-content {
  flex: 1;
  overflow-y: auto;
  /* Left padding is the gutter's lane: without it the handle overlaps the first
     characters of every line. */
  padding: 12px 2px 12px 34px;
  min-height: 0;
}
.hidden-file {
  display: none;
}

/* ---- drag handle ---- */
.doc-gutter {
  position: absolute;
  left: 0;
  display: flex;
  align-items: center;
  gap: 1px;
  /* Nudged up so the pair reads as centred on the first line of the block. */
  margin-top: 1px;
}
.gutter-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 20px;
  padding: 0;
  border: none;
  border-radius: 4px;
  background: transparent;
  color: var(--t-text3);
  cursor: pointer;
  transition:
    background 0.12s ease,
    color 0.12s ease;
}
.gutter-btn:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.gutter-btn.grip {
  cursor: grab;
}
.gutter-btn.grip:active {
  cursor: grabbing;
}

/* ---- slash menu ---- */
.slash-menu {
  position: absolute;
  z-index: 10;
  width: 260px;
  max-height: 260px;
  overflow-y: auto;
  padding: 4px;
  border: 1px solid var(--t-border);
  border-radius: 8px;
  background: var(--t-surface);
}
.slash-item {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  padding: 5px 8px;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: var(--t-text1);
  font-size: 13px;
  text-align: left;
  cursor: pointer;
}
.slash-item:hover,
.slash-item.on {
  background: var(--t-hover);
}
.slash-label {
  flex: 1;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.slash-hint {
  color: var(--t-text3);
  font-size: 11px;
  white-space: nowrap;
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
/* The block picked up by the handle. ProseMirror marks a NodeSelection with
   this class, and without a visible cue a drag looks like it grabbed nothing. */
.doc-content :deep(.ProseMirror .ProseMirror-selectednode) {
  outline: 2px solid var(--t-primary);
  outline-offset: 2px;
  border-radius: 4px;
}
/* Placeholder shown while a dropped image uploads (imageDrop.js). */
.doc-content :deep(.doc-upload-placeholder) {
  display: inline-block;
  padding: 2px 8px;
  border: 1px dashed var(--t-border);
  border-radius: 6px;
  color: var(--t-text3);
  font-size: 12px;
}
</style>
