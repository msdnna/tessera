<script setup>
import { onBeforeUnmount, reactive, ref, shallowRef, watch } from 'vue'
import { EditorContent, Editor } from '@tiptap/vue-3'
import { NButton, NIcon, NInput, NSelect, useMessage } from 'naive-ui'
import {
  AddOutline,
  ChatbubbleEllipsesOutline,
  CheckmarkOutline,
  CloseOutline,
  ReorderTwoOutline,
} from '@vicons/ionicons5'
import { docExtensions, toDocJSON } from '@/utils/docSchema'
import { BLOCK_ID_META, ensureBlockIds } from '@/utils/docExtensions/blockId'
import {
  blockAtClientY,
  endBlockDrag,
  startBlockDrag,
  topLevelBlocks,
} from '@/utils/docExtensions/dragHandle'
import { slashState } from '@/utils/docExtensions/slashMenu'
import { TextSelection } from '@tiptap/pm/state'
import { applyBlockLocks, blockIdAtSelection, blockRanges } from '@/utils/docExtensions/blockLocks'
import { blockOrderChanged } from '@/utils/docMerge'
import { applyBlockComments } from '@/utils/docExtensions/blockComments'
import { quoteFromBlock } from '@/utils/docComments'
import { scrollToBlockId } from '@/utils/docExtensions/internalLink'
import { docOutline, headingLabel, internalHref, internalTargetId } from '@/utils/docToc'
import { MAX_PDF_BYTES } from '@/utils/docPdf'
import DocToolbar from './DocToolbar.vue'

const props = defineProps({
  modelValue: { type: Object, default: null },
  editable: { type: Boolean, default: true },
  placeholder: { type: String, default: 'Начните писать…' },
  // Uploads a File and resolves to the URL to embed. Injected rather than
  // imported so the editor stays free of API knowledge (and testable offline).
  uploadImage: { type: Function, default: null },
  // Uploads a PDF and resolves to {src, name, size}. Absent on a read-only
  // surface, in which case the slash entry simply does nothing.
  uploadPdf: { type: Function, default: null },
  // Blocks other people are editing right now: [{block_id, name, user_id}].
  // The editor paints them and refuses input inside them (#2729); it does not
  // know where the list comes from.
  locks: { type: Array, default: () => [] },
  // Open discussions per block: [{block_id, count}] (#2730). Painted in the
  // margin; the threads themselves live in the panel next door.
  comments: { type: Array, default: () => [] },
})
const emit = defineEmits([
  'update:modelValue',
  'change',
  'blur',
  'block-focus',
  'blocked',
  'annotate',
  'select-comments',
])

const message = useMessage()
const editor = shallowRef(null)
const surface = ref(null)
const fileInput = ref(null)
const pdfInput = ref(null)
const linkOpen = ref(false)
const linkValue = ref('')
// Headings offered as internal link targets, filled when the dialog opens.
const headings = ref([])
const uploading = ref(false)

// The gutter that carries the drag handle. `pos` is the document position of
// the block it currently addresses — everything the handle does needs it.
const handle = reactive({ visible: false, top: 0, left: 0, pos: 0, index: 0 })
const slash = reactive({ active: false, items: [], index: 0, left: 0, top: 0 })

// Tracks the JSON we last emitted, so the watcher below can tell "the parent
// loaded a different document" from "the parent echoed back our own edit".
let lastEmitted = null

// Marks a transaction as someone else's edit arriving over the document socket
// rather than this user's typing (#2729 rework).
const REMOTE_META = 'docEditor$remote'

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
      // The menu hands back the item it chose; which picker opens is this
      // component's business, since the file inputs live here.
      onSlashExternal: (item) => (item?.key === 'pdf' ? pickPdf() : pickImage()),
      onBlocked: (held) => emit('blocked', held),
      onSelectComments: (blockId) => emit('select-comments', blockId),
    }),
    onUpdate: ({ editor: e, transaction }) => {
      // The BlockId extension stamps missing ids on load. That is bookkeeping,
      // not a user edit: emitting it would mark a freshly opened document dirty
      // and trigger an autosave nobody asked for.
      if (transaction?.getMeta(BLOCK_ID_META)) return
      const json = e.getJSON()
      lastEmitted = json
      emit('update:modelValue', json)
      // A remote edit updates the parent's copy but is not a change *by* this
      // client: `change` is what schedules the autosave, and saving what we were
      // just told would bounce the edit back to its author (who would then apply
      // it and save it back to us).
      if (transaction?.getMeta(REMOTE_META)) return
      emit('change', json)
    },
    onTransaction: ({ editor: e }) => {
      syncSlash(e)
      reportBlock(e)
    },
    onBlur: () => {
      // The parent gives the block back on blur, so forget which one we claimed —
      // otherwise coming back to the same block would look like "no change" and
      // never re-claim it.
      lastBlockId = ''
      emit('blur')
    },
  })
}

build()

// The block the caret is in, reported on change so the parent can claim it.
// Emitting only on change keeps a lock refresh from being sent on every
// keystroke — the claim itself is a heartbeat, not a per-edit message.
let lastBlockId = ''
function reportBlock(e) {
  // Only a *focused* editor claims anything. Without this the initial
  // setContent transaction claims the first block of every document the moment
  // it is opened, so a reader with the tab parked on a page would hold its first
  // paragraph against everyone else for as long as the tab lived — the client
  // keeps refreshing the lock, so the TTL never collects it either.
  if (!e.isFocused) {
    lastBlockId = ''
    return
  }
  const id = blockIdAtSelection(e.state)
  if (id === lastBlockId) return
  lastBlockId = id
  emit('block-focus', id)
}

// Locks arrive from outside (the document socket) and are pushed into the
// plugin, which owns the decorations and the input guard.
watch(
  () => props.locks,
  (next) => applyBlockLocks(editor.value?.view, next),
  { deep: true },
)

// Same channel for the comment markers: the plugin owns the decorations, the
// panel owns the threads, and neither knows about the other.
watch(
  () => props.comments,
  (next) => applyBlockComments(editor.value?.view, next),
  { deep: true, immediate: true },
)

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

function pickPdf() {
  pdfInput.value?.click?.()
}

async function onPdfFile(e) {
  const file = e.target.files && e.target.files[0]
  e.target.value = ''
  if (!file || !props.uploadPdf) return
  if (file.size > MAX_PDF_BYTES) {
    message.error('Файл больше 20 МБ')
    return
  }
  uploading.value = true
  try {
    const pdf = await props.uploadPdf(file)
    if (pdf?.src) editor.value?.chain().focus().insertPdf(pdf).run()
  } catch (err) {
    message.error(err.message || 'Не удалось загрузить PDF')
  } finally {
    uploading.value = false
  }
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
  // The heading list is taken once, when the dialog opens: recomputing it per
  // keystroke would walk the whole tree while the user types a URL that has
  // nothing to do with headings.
  headings.value = docOutline(editor.value?.getJSON() || null).map((row) => ({
    label: `${'· '.repeat(row.depth)}${headingLabel(row)}`,
    value: internalHref(row.id),
  }))
  linkOpen.value = true
}

// Picking a heading fills the same field the URL is typed into rather than
// applying straight away, so the two ways of making a link end in one place and
// the chosen target is visible before it is committed.
function pickHeading(href) {
  linkValue.value = href || ''
}

function applyLink() {
  const href = linkValue.value.trim()
  const chain = editor.value?.chain().focus().extendMarkRange('link')
  if (!chain) return
  if (href) chain.setLink({ href }).run()
  else chain.unsetLink().run()
  linkOpen.value = false
}

// Jumps to a block — used by the outline panel, which has the id but no view.
function goToBlock(blockId) {
  return scrollToBlockId(editor.value?.view, blockId)
}

/**
 * Applies a colleague's edit to the open document (#2729 rework).
 *
 * `setContent` is not an option here even though the tree is complete: it tears
 * the editor down and rebuilds it, taking the caret, the selection and any input
 * newer than the last debounce with it. Someone typing while a colleague saves
 * would watch their cursor jump to the top of the page every second or so.
 *
 * So the change goes in as an ordinary transaction, and one of two shapes:
 *
 *  • same blocks in the same order — replace only the blocks whose content
 *    actually differs, back to front so the earlier positions stay valid. Every
 *    untouched block, including the one being typed in, is never addressed;
 *  • blocks added, removed or reordered — swap the whole content in one step and
 *    remap the selection through it. Diffing moves properly would buy a smoother
 *    caret in a case that is already rare (someone else restructuring the
 *    document while you type in it) at the price of a diff nobody can review.
 *
 * @param {object} json the merged document to apply
 * @returns {boolean} whether anything was applied
 */
function applyRemote(json) {
  const view = editor.value?.view
  if (!view || !json?.content) return false
  const { state } = view
  const target = state.schema.nodeFromJSON(json)
  if (target.eq(state.doc)) return false

  const { from } = state.selection
  const tr = state.tr
  if (blockOrderChanged(editor.value.getJSON(), json)) {
    tr.replaceWith(0, state.doc.content.size, target.content)
  } else {
    const ranges = blockRanges(state.doc)
    for (let i = ranges.length - 1; i >= 0; i -= 1) {
      const next = target.child(i)
      if (state.doc.child(i).eq(next)) continue
      tr.replaceWith(ranges[i].from, ranges[i].to, next)
    }
  }
  if (!tr.docChanged) return false
  tr.setSelection(
    TextSelection.near(tr.doc.resolve(Math.min(tr.mapping.map(from), tr.doc.content.size))),
  )
  // Out of the undo stack: Ctrl+Z must not reach for a colleague's text. And
  // flagged remote, so onUpdate syncs the parent without scheduling a save —
  // without that, applying an edit would trigger a save that announces it back,
  // and two clients would resave the document to each other forever.
  tr.setMeta('addToHistory', false).setMeta(REMOTE_META, true)
  view.dispatch(tr)
  return true
}

/* ---- drag handle -------------------------------------------------------- */

// Distance from the sheet's left edge to the handle. Keeps the three buttons
// inside the sheet's left padding instead of floating in the work area beside
// it, where a narrow window would put them off-screen.
const GUTTER_INSET = 4

function onSurfaceMove(e) {
  const view = editor.value?.view
  if (!view || !props.editable) return
  const found = blockAtClientY(topLevelBlocks(view), e.clientY)
  if (!found || !surface.value) {
    handle.visible = false
    return
  }
  const surfaceRect = surface.value.getBoundingClientRect()
  handle.visible = true
  handle.top = found.rect.top - surfaceRect.top
  // Anchored to the sheet, not to the surface: the sheet is centred in the work
  // area, so a fixed `left: 0` would leave the handle stranded against the far
  // edge of the window. It rides in the sheet's left padding, which is sized to
  // hold it (.doc-content :deep(.ProseMirror) below).
  handle.left = view.dom.getBoundingClientRect().left - surfaceRect.left + GUTTER_INSET
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

// Starts a discussion about the block under the handle (#2730).
//
// The quote is taken here, from the text as it stands right now, because that is
// the only moment it is certainly the text the remark is about — the block goes
// on being edited afterwards, and the annotation itself writes nothing into the
// document.
function annotateBlock() {
  const view = editor.value?.view
  if (!view) return
  const node = view.state.doc.nodeAt(handle.pos)
  if (!node) return
  const blockId = node.attrs?.id || ''
  if (!blockId) return
  emit('annotate', { blockId, quote: quoteFromBlock(node.toJSON()) })
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

defineExpose({ editor, goToBlock, applyRemote })
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
        placeholder="https://… или раздел документа"
        data-testid="doc-link-href"
        @keyup.enter="applyLink"
      />
      <n-select
        v-if="headings.length"
        class="link-heading"
        size="small"
        clearable
        placeholder="Раздел…"
        :options="headings"
        :value="internalTargetId(linkValue) ? linkValue : null"
        data-testid="doc-link-heading"
        @update:value="pickHeading"
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
      <div
        v-show="handle.visible"
        class="doc-gutter"
        :style="{ top: `${handle.top}px`, left: `${handle.left}px` }"
      >
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
          class="gutter-btn"
          title="Обсудить блок"
          @mousedown.prevent="annotateBlock"
        >
          <n-icon :component="ChatbubbleEllipsesOutline" :size="13" />
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
    <input
      ref="pdfInput"
      type="file"
      accept="application/pdf,.pdf"
      class="hidden-file"
      @change="onPdfFile"
    />
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
/* The heading picker is a shortcut next to the URL field, not an equal half of
   the row: most links are still typed. */
.link-heading {
  flex: 0 0 180px;
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
/* The work area the sheet sits on. Its own background is what makes the sheet
   read as a sheet — there is no shadow to lean on, because the theming guard in
   cx-doc-editor.spec.js forbids literal colours and the palette has no shadow
   token. The contrast comes from the surface/surface-alt pair instead, which is
   defined in both themes and follows the accent the user picks. */
.doc-content {
  flex: 1;
  overflow-y: auto;
  padding: 16px 12px 28px;
  background: var(--t-surface-alt);
  min-height: 0;
}
.hidden-file {
  display: none;
}

/* ---- drag handle ---- */
/* `left` is set inline from the sheet's rect — see onSurfaceMove.
   The z-index is load-bearing now that the handle rides inside the sheet's
   padding rather than beside it: ProseMirror gives its own element
   `position: relative`, and being a later sibling it would otherwise paint over
   the handle and swallow every click on it. */
.doc-gutter {
  position: absolute;
  z-index: 5;
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
/* The label is what the user is reading; the hint gives way first, so
   "Маркированный список" stays whole instead of losing its ending to a
   nowrap hint that never shrinks. */
.slash-label {
  flex: 1 0 auto;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.slash-hint {
  flex: 0 1 auto;
  min-width: 0;
  overflow: hidden;
  color: var(--t-text3);
  font-size: 11px;
  white-space: nowrap;
  text-overflow: ellipsis;
}

/* ProseMirror renders plain DOM, so naive-ui's themeOverrides never reach it —
   the CSS custom properties are the only channel the theme has here. Every
   colour below therefore comes from a --t-* token with no literal fallback. */
/* The editing surface itself is the sheet: bounded width, centred, its own
   background against the work area. The left padding doubles as the drag
   handle's lane (see GUTTER_INSET above), so it is wider than the right one by
   the width of the three gutter buttons.
   The width is visual only — this is still a block document, and page size,
   margins and orientation remain export-time settings (задача 2733), so there
   are no page breaks here. */
.doc-content :deep(.ProseMirror) {
  outline: none;
  box-sizing: border-box;
  max-width: 820px;
  margin: 0 auto;
  padding: 40px 40px 56px 56px;
  border: 1px solid var(--t-border);
  border-radius: 8px;
  background: var(--t-surface);
  min-height: 480px;
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
/* A block someone else is editing (задача 2729 — written without the hash on
   purpose: the theming guard in cx-doc-editor.spec.js reads a #NNNN in this
   block as a literal colour). The cue is a left border plus a
   floating name — not a background fill, which would fight the accent gradient
   and make the text harder to read while you are still allowed to read it.
   Colours come from --t-* only: ProseMirror renders plain DOM, so naive-ui's
   themeOverrides never reach it, and a literal here would survive the light
   theme and break in the dark one. */
.doc-content :deep(.ProseMirror .doc-block-locked) {
  position: relative;
  /* --doc-lock-color is the holder's own colour, set on the decoration by the
     view (задача 2729 rework). With two colleagues in a document, one accent
     colour for both would say "occupied" without saying by whom, which is the
     whole point of the badge. Falls back to the accent when unset. */
  border-left: 2px solid var(--doc-lock-color, var(--t-primary));
  margin-left: -10px;
  padding-left: 8px;
  border-radius: 2px;
}
.doc-content :deep(.ProseMirror .doc-block-locked::after) {
  content: attr(data-locked-by);
  position: absolute;
  top: -8px;
  right: 0;
  padding: 0 6px;
  border-radius: 8px;
  background: var(--doc-lock-color, var(--t-primary));
  color: var(--doc-lock-text, var(--t-on-primary));
  font-size: 10px;
  line-height: 16px;
  white-space: nowrap;
  pointer-events: none;
}

/* A block with an open discussion (задача 2730 — the hash is left off on
   purpose: the theming guard in cx-doc-editor.spec.js reads a #NNNN in this
   block as a literal colour). A dotted underline plus a count in the margin,
   not a fill: the annotation must stay legible as text, and a highlighted
   background would collide with the lock cue on a block that has both.
   Colours are --t-* only — ProseMirror renders plain DOM, so naive-ui's
   themeOverrides never reach here. */
.doc-content :deep(.ProseMirror .doc-block-commented) {
  position: relative;
  border-bottom: 1px dashed var(--t-primary);
}
.doc-content :deep(.ProseMirror .doc-block-commented::before) {
  content: attr(data-comment-count);
  position: absolute;
  left: -18px;
  top: 0;
  width: 14px;
  height: 14px;
  border-radius: 50%;
  background: var(--t-accent-grad);
  color: var(--t-on-primary);
  font-size: 9px;
  line-height: 14px;
  text-align: center;
  pointer-events: none;
}

/* The block an internal link or an outline entry just jumped to (задача 2733 —
   the hash is left off on purpose: the theming guard in cx-doc-editor.spec.js
   reads a #NNNN in this block as a literal colour). A smooth scroll ends
   somewhere in the middle of the page with nothing saying which block was the
   point of it, so the arrival is marked for a moment and then fades. The cue is
   a soft fill rather than an outline: an outline would be mistaken for the
   node-selection ring the drag handle draws. Colours are --t-* only —
   ProseMirror renders plain DOM, out of reach of naive-ui's themeOverrides. */
.doc-content :deep(.ProseMirror .doc-block-target) {
  border-radius: 4px;
  box-shadow: 0 0 0 6px var(--t-hover);
  background: var(--t-hover);
  transition:
    background 0.4s ease,
    box-shadow 0.4s ease;
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
