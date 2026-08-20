<script setup>
import { computed, onBeforeUnmount, reactive, ref, shallowRef, watch } from 'vue'
import { EditorContent, Editor } from '@tiptap/vue-3'
import { NButton, NIcon, NInput, NSelect, useMessage } from 'naive-ui'
import {
  AddOutline,
  ChatbubbleEllipsesOutline,
  CheckmarkOutline,
  CloseOutline,
  ReorderTwoOutline,
} from '@vicons/ionicons5'
import { docExtensions, editableDoc } from '@/utils/docSchema'
import { BLOCK_ID_META, ensureBlockIds } from '@/utils/docExtensions/blockId'
import {
  blockAtClientY,
  centerOffset,
  endBlockDrag,
  firstLineBox,
  startBlockDrag,
  topLevelBlocks,
} from '@/utils/docExtensions/dragHandle'
import { groupSlashItems } from '@/utils/docSlash'
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
// `snap` suppresses the travel animation for one frame — see onSurfaceMove.
const handle = reactive({ visible: false, snap: true, top: 0, left: 0, pos: 0, index: 0 })
// The gutter element itself — its height is measured rather than assumed, so
// the centring below keeps working when the button size changes.
const gutter = ref(null)
const slash = reactive({ active: false, items: [], index: 0, left: 0, top: 0 })
// Display-only bucketing of the filtered items; `slash.items` stays the list the
// arrow keys and the highlighted index address.
const slashGroups = computed(() => groupSlashItems(slash.items))

// Tracks the JSON we last emitted, so the watcher below can tell "the parent
// loaded a different document" from "the parent echoed back our own edit".
let lastEmitted = null

// Marks a transaction as someone else's edit arriving over the document socket
// rather than this user's typing (#2729 rework).
const REMOTE_META = 'docEditor$remote'

function build() {
  editor.value = new Editor({
    content: ensureBlockIds(editableDoc(props.modelValue)),
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
    editor.value.commands.setContent(ensureBlockIds(editableDoc(next)), { emitUpdate: false })
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
  // Moving from one block to the next slides the handle across (задача 2728);
  // the first appearance must not, or it would fly in from wherever the last
  // hover left it — including from off-screen after a scroll. The class and the
  // new `top` are applied in the same DOM update, so the jump is not animated,
  // and the rAF hands the transition back before the pointer can reach another
  // block.
  if (!handle.visible) {
    handle.snap = true
    requestAnimationFrame(() => {
      handle.snap = false
    })
  }
  handle.visible = true
  // Centred on the block's FIRST LINE, not on its box. A heading's box opens a
  // margin above its text and sets its own line-height, so the box top is a
  // different place from the line — which is why the handle used to look
  // centred on a paragraph and adrift on a heading.
  const line = firstLineBox(view, found.pos, found.rect)
  handle.top =
    centerOffset(line.top, line.bottom, gutter.value?.offsetHeight || 0) - surfaceRect.top
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

/* ---- anchors for the annotation links (#2730) ---------------------------- */

/**
 * Where each of the given blocks attaches on the text side.
 *
 * The x is the right edge of the block's own box — which is the text column, not
 * the end of the text, since a block fills the column however short its content
 * is. So the line leaves exactly where the block's underline stops, and a
 * one-word paragraph does not start its line in the middle of empty space.
 * (Anchoring to the sheet edge instead leaves a stub in the gutter: the sheet is
 * wider than the column, so the visible part of the curve would be the last few
 * pixels before the panel.) Clamped to the sheet in case a wide table overruns.
 *
 * The y is the block's BOTTOM EDGE — that is where the dashed underline of a
 * discussed block runs, so the curve leaves as a continuation of that line
 * instead of starting off it. Anchoring to the first line's centre (as this did
 * before) put the start half a line above the underline on a one-line block and
 * a whole paragraph above it on a longer one, which read as two unrelated marks
 * on the same block rather than one mark carried out to the panel.
 *
 * Blocks scrolled out of the content box are reported `visible: false` rather
 * than dropped: the caller distinguishes "gone from the document" (a detached
 * thread) from "off screen" (no line this frame).
 *
 * @param {Array<string>} ids block ids to measure
 * @returns {Array<{id: string, x: number, y: number, visible: boolean}>}
 */
function blockAnchors(ids) {
  const view = editor.value?.view
  if (!view || !ids || !ids.length) return []
  const want = new Set(ids)
  // The scroll box is the element EditorContent renders around .ProseMirror;
  // going through the view keeps this working without a second template ref.
  const scroller = view.dom.parentElement
  if (!scroller) return []
  const clip = scroller.getBoundingClientRect()
  const sheet = view.dom.getBoundingClientRect()
  const out = []
  for (const b of blockRanges(view.state.doc)) {
    if (!b.id || !want.has(b.id)) continue
    const dom = view.nodeDOM(b.from)
    const rect = dom?.getBoundingClientRect?.()
    if (!rect) continue
    const y = rect.bottom - underlineOffset(dom)
    const x = Math.min(rect.right, sheet.right)
    out.push({ id: b.id, x, y, visible: y >= clip.top && y <= clip.bottom })
  }
  return out
}

// Half the dashed border, so the curve starts on the underline's centre line
// rather than on the border box's outer edge — at 1 px the two differ by half a
// pixel, but the rule is read off the CSS instead of hardcoded here, so a
// thicker underline keeps the join exact.
function underlineOffset(dom) {
  if (!dom || dom.nodeType !== 1 || typeof getComputedStyle !== 'function') return 0
  const w = Number.parseFloat(getComputedStyle(dom).borderBottomWidth)
  return Number.isFinite(w) ? w / 2 : 0
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

defineExpose({ editor, goToBlock, applyRemote, blockAnchors })
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
      <!-- Hidden by visibility, not by v-show/display: the centring below
           measures the row's height, and a display:none row measures zero — the
           handle would land half its own height too low on the first hover and
           only correct itself on the second. -->
      <div
        ref="gutter"
        class="doc-gutter"
        :class="{ off: !handle.visible, snap: handle.snap }"
        :style="{ top: `${handle.top}px`, left: `${handle.left}px` }"
      >
        <button
          type="button"
          class="gutter-btn"
          title="Вставить блок ниже"
          @mousedown.prevent="insertBelow"
        >
          <n-icon :component="AddOutline" :size="16" />
        </button>
        <button
          type="button"
          class="gutter-btn"
          title="Обсудить блок"
          @mousedown.prevent="annotateBlock"
        >
          <n-icon :component="ChatbubbleEllipsesOutline" :size="16" />
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
        <!-- Grouped for reading only: the buttons stay a flat run, and the
             highlighted index still addresses slash.items, so the arrow keys and
             the mouse cannot disagree about which entry is current. The headings
             are plain text, not buttons — Enter must never "insert" a group. -->
        <div v-for="g in slashGroups" :key="g.group" class="slash-group">
          <div class="slash-group-title">{{ g.group }}</div>
          <button
            v-for="item in g.items"
            :key="item.key"
            type="button"
            class="slash-item"
            :class="{ on: slash.items.indexOf(item) === slash.index }"
            @mousedown.prevent="runSlash(item)"
          >
            <n-icon :component="item.icon" :size="16" />
            <span class="slash-text">
              <span class="slash-label">{{ item.label }}</span>
              <span class="slash-hint">{{ item.hint }}</span>
            </span>
          </button>
        </div>
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
/* Same 8px as the sheet below: the work area is the sheet's frame, and a square
   frame around a rounded sheet read as an unfinished corner (задача 2727).
   No task number written as "#NNNN" in this stylesheet — the theming guard in
   cx-doc-editor.spec.js reads four hex digits after a hash as a literal colour. */
.doc-content {
  flex: 1;
  overflow-y: auto;
  padding: 16px 12px 28px;
  background: var(--t-surface-alt);
  border-radius: 8px;
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
/* No margin nudge here any more: the top is computed from the line box (see
   onSurfaceMove), so the row is centred by arithmetic rather than by eye. */
.doc-gutter {
  position: absolute;
  z-index: 5;
  display: flex;
  align-items: center;
  gap: 4px;
  /* Only `top` travels: `left` follows the sheet, and animating it would drag
     the handle sideways across the text on every window resize. */
  transition: top 0.12s ease-out;
}
/* Set for the frame the handle appears on, so it does not slide in from the
   last block it addressed (see onSurfaceMove). */
.doc-gutter.snap {
  transition: none;
}
.doc-gutter.off {
  visibility: hidden;
  pointer-events: none;
}
.gutter-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
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
  width: 300px;
  max-height: 320px;
  overflow-y: auto;
  padding: 4px;
  border: 1px solid var(--t-border);
  border-radius: 8px;
  background: var(--t-surface);
}
/* A rule between groups rather than around them: the first group sits right
   under the menu's own border and a second line there would read as a gap. */
.slash-group + .slash-group {
  margin-top: 4px;
  padding-top: 4px;
  border-top: 1px solid var(--t-border);
}
.slash-group-title {
  padding: 2px 8px 4px;
  color: var(--t-text3);
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.06em;
  text-transform: uppercase;
}
.slash-item {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  min-height: 32px;
  padding: 4px 8px;
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
/* Label above hint instead of beside it. Side by side they competed for one
   line and the hint truncated the label ("Список с то…"); stacked, both fit and
   the ellipsis rules below never fire in practice. */
.slash-text {
  display: flex;
  flex-direction: column;
  min-width: 0;
  line-height: 1.25;
}
.slash-label {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.slash-hint {
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
   the width of the three gutter buttons — 3×24 plus two 4px gaps plus the inset.
   The asymmetry is deliberate and maintained by hand: a lane narrower than the
   row would put the buttons on top of the first characters.
   The width is visual only — this is still a block document, and page size,
   margins and orientation remain export-time settings (задача 2733), so there
   are no page breaks here. */
.doc-content :deep(.ProseMirror) {
  outline: none;
  box-sizing: border-box;
  max-width: 820px;
  margin: 0 auto;
  padding: 40px 40px 56px 88px;
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
/* The gap-cursor stays legal around table/horizontalRule/pdfEmbed blocks; its
   border colour is hard-coded black in @tiptap/core, invisible on a dark
   surface — pin it to the theme text token (task 2761). */
.doc-content :deep(.ProseMirror-gapcursor::after) {
  border-top-color: var(--t-text1);
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
/* The checkbox is centred on the FIRST LINE, not on the item. `align-items:
   center` on the item would look right on the one-line task in the screenshot
   and put the box in the middle of the paragraph on a task that wraps to three
   lines. Giving the label the line's own height centres it against the line and
   leaves the rest of the item alone. */
.doc-content :deep(.ProseMirror ul[data-type='taskList'] li > label) {
  display: flex;
  align-items: center;
  flex: 0 0 auto;
  height: 1.6em; /* = the .ProseMirror line-height, so it tracks it */
  margin: 0;
  user-select: none;
}
.doc-content :deep(.ProseMirror ul[data-type='taskList'] li > div) {
  flex: 1 1 auto;
  min-width: 0;
}
/* The paragraph inside a task carries the same margin as a standalone one,
   which would push the text down past the box it is labelled by. */
.doc-content :deep(.ProseMirror ul[data-type='taskList'] li p) {
  margin: 0;
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
/* Cell selection is a tint painted BEHIND the cell text, not a layer on top of
   it: the previous absolute ::after covered the very text you were selecting
   (задача 2739 — number written without the hash, the theming guard reads a
   #NNNN in this block as a literal colour). A plain background also cannot
   escape the cell, so the whole work area can no longer be flooded. Declared
   after the th rule so it wins over the header fill. */
.doc-content :deep(.ProseMirror .selectedCell) {
  background: color-mix(in srgb, var(--t-primary) 18%, var(--t-surface));
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

/* The line marking where a dragged block will land (задача 2728). The colour
   lives here rather than in the extension's `color` option because that option
   takes a literal string, and a literal cannot follow the theme — the option is
   set to `false` in docSchema.js precisely so this rule wins.

   Scoped to .doc-surface and not to .doc-content, which is the wrapper every
   other editor rule here uses: prosemirror-dropcursor appends its element to
   `view.dom.offsetParent`, and .doc-content has no `position`, so the nearest
   positioned ancestor — and the actual parent of this element — is the surface
   one level up. Under .doc-content the rule would simply never match. */
.doc-surface :deep(.doc-dropcursor) {
  background: var(--t-primary);
  border-radius: 1px;
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
