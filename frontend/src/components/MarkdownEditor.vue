<script setup>
import {
  ref,
  computed,
  nextTick,
  watch,
  onMounted,
  onBeforeUnmount,
  defineAsyncComponent,
} from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon, useMessage } from 'naive-ui'
import {
  LinkOutline,
  ImageOutline,
  GitNetworkOutline,
  EyeOutline,
  CreateOutline,
  SendOutline,
  ListOutline,
  CheckboxOutline,
  ChevronDownOutline,
  AttachOutline,
  ExpandOutline,
  EllipsisHorizontalOutline,
  AtOutline,
  CloudUploadOutline,
  DocumentTextOutline,
} from '@vicons/ionicons5'
import { uploads as uploadsApi, tasks as tasksApi } from '@/api'
import { toggleTaskMarker } from '@/utils/markdown'
import {
  WRAP_PAIRS,
  INDENT,
  wrapSelection,
  indentLines,
  outdentLines,
  orderedListPrefix,
  autoClose,
  deletePair,
  handleEnter,
} from '@/utils/mdEdit'
import { detectSlashQuery, matchCommands, commandInsertText } from '@/utils/commands'
import { mentionIdsIn } from '@/utils/mentions'
import { isTauri } from '@/utils/serverBase'
import { scrollParent } from '@/utils/dom'
import { ATTACHMENT_MAX_BYTES, attachmentPickError, mediaPickMessage } from '@/utils/upload'
import { formatFileSize } from '@/utils/docPdf'
import { CAPS } from '@/utils/keyCaps'
import RichContent from './RichContent.vue'
import Kbd from './Kbd.vue'
import ActionMenu from './kit/ActionMenu.vue'
import AnchorLayer from './kit/AnchorLayer.vue'
import TesseraSpinner from './TesseraSpinner.vue'
import UserAvatar from './UserAvatar.vue'

const props = defineProps({
  modelValue: { type: String, default: '' },
  // null, not a translated literal: a prop default is evaluated once per
  // instance and would freeze the hint in the language of the first render.
  placeholder: { type: String, default: null },
  // Members for @-mentions: [{ id, label, display?, avatarUserId?, avatarSrc?, gitlab? }].
  // `label` is the inserted text (Tessera name or GitLab @username), `display` the row
  // label (falls back to label). Empty → mentions off.
  mentionItems: { type: Array, default: () => [] },
  // Quick actions for the `/`-autocomplete: [{ key, description, arg, builtin }]
  // (see utils/commands.js `commandItems`). Empty → the `/`-popup stays off.
  commandItems: { type: Array, default: () => [] },
  minRows: { type: Number, default: 3 },
  // 'write' | 'preview' — sets the initial tab (re-applied when it changes,
  // e.g. when a different task loads). User tab clicks override locally.
  initialMode: { type: String, default: 'write' },
  // 'default' — bare editor with a slim top bar (documents, the stage goals).
  // 'boxed' — framed composer with a persistent bottom toolbar and an in-place
  // preview toggle (notes, reminders).
  // 'v2' — the v2 edit block (#3113, StagePage mockup): one bar with the
  // Написать / Просмотр segment and the tools, the field with a primary ring, and
  // a footer «# задача · @ человек · Отмена Esc · Сохранить ⌘↵». Esc asks before
  // throwing typed text away. The task description, a comment edit, a reply.
  // 'composer' — the v2 comment composer (#3113, TaskModal mockup): a 46px row
  // with the reader's avatar, the field, «Aa» (the same bar as v2, above the row),
  // «Прикрепить» and «Отправить ⌘↵».
  variant: { type: String, default: 'default' },
  // Boxed only: show a send button in the toolbar that emits `submit`.
  send: { type: Boolean, default: false },
  // Boxed + send only: the parent is posting — the send button shows a spinner and
  // both the button and Ctrl+Enter are inert until it clears (no double-submit).
  sending: { type: Boolean, default: false },
  // Default variant only: render the built-in top toolbar. Set false to host the
  // actions elsewhere (e.g. the description's header) via the exposed methods +
  // `update:mode` — see TaskModal's .desc-head.
  toolbar: { type: Boolean, default: true },
  // Task whose «Файлы» tab receives non-image drops, and whose files the paperclip
  // lists for linking. Null → both are off and drops behave as they always did.
  attachTaskId: { type: String, default: null },
  // Offer the fullscreen editor. False inside the fullscreen modal itself, which
  // hosts this very component (the button there would just reopen it).
  expandable: { type: Boolean, default: true },
  // Two panes side by side — text left, live preview right — instead of the
  // write/preview toggle, and the editor fills the height it is given rather than
  // growing to its content. For the fullscreen modal: at that size the toggle
  // wastes the second half of the window and a short text leaves it empty.
  split: { type: Boolean, default: false },
  // v2: the footer's primary button (null → «Сохранить»); `footer` off hides the
  // footer for a host that draws its own.
  saveLabel: { type: String, default: null },
  footer: { type: Boolean, default: true },
  // v2: issue templates offered from the bar ([{ name }]); picking one emits
  // `template` with its name. `templateLabel` titles the button.
  templates: { type: Array, default: () => [] },
  templateLabel: { type: String, default: '' },
  // composer: whose avatar leads the row ({ id, name, src }).
  avatar: { type: Object, default: null },
  // composer: the send button's caption (null → «Отправить»).
  sendLabel: { type: String, default: null },
})
const emit = defineEmits([
  'update:modelValue',
  'submit',
  'blur',
  'persist',
  'update:mode',
  'attachments-changed',
  // Esc in the field — the host may use it to dismiss the composer (e.g. the reply
  // form). In the v2 variant it means «leave and drop what was typed since the
  // editor opened», asked first when anything was (see requestCancel).
  'cancel',
  'template',
])

const message = useMessage()
const { t } = useI18n()
const fieldPlaceholder = computed(() => props.placeholder ?? t('documents.editor.placeholder'))
const boxed = computed(() => props.variant === 'boxed')
const v2 = computed(() => props.variant === 'v2')
const composer = computed(() => props.variant === 'composer')
// Glyph captions of the bar (letters, not words — the same in every locale).
const AA = 'Aa'
const mode = ref(props.initialMode) // 'write' | 'preview'
// Split shows both panes at once, so the textarea is always on screen and the
// mode toggle has nothing left to switch.
const writing = computed(() => props.split || mode.value === 'write')
watch(mode, (m) => emit('update:mode', m), { immediate: true })
function toggleMode() {
  mode.value = mode.value === 'write' ? 'preview' : 'write'
}
function onSend() {
  if (props.sending) return
  emit('submit')
}
watch(
  () => props.initialMode,
  (m) => {
    mode.value = m
  },
)
const ta = ref(null)
const rootEl = ref(null)
const previewSide = ref(null)

// ── narrow (boxed) composer: fold the formatting/insert buttons into a popover ──
// A comment composer in the sidebar-panel layout is only ~470–500px wide, where the
// full toolbar overruns the box and shoves the send button past its right edge
// (#2743). Below a threshold we collapse every format/insert control into one menu
// button, so only the menu toggle + preview/expand/send stay on the bar — those can
// never overflow. Measured off the editor's own width (ResizeObserver), so it reacts
// to the panel, not the viewport.
const compact = ref(false)
const fmtOpen = ref(false)
// The v2 bar and the composer fold their tools the same way, by the width they
// get (the sidebar layout of the task modal is ~490px, a phone ~360px).
const width = ref(0)
// Narrow enough that the send button drops its caption and the v2 footer its
// key hints.
const narrowSend = computed(() => !!width.value && width.value < 420)
let ro = null
onMounted(() => {
  if (props.variant === 'default' || !rootEl.value || typeof ResizeObserver === 'undefined') return
  ro = new ResizeObserver((entries) => {
    const w = entries[0]?.contentRect?.width
    if (w) {
      // A new width re-wraps the text: the field's height is stale.
      if (Math.round(w) !== Math.round(width.value)) nextTick(autoGrow)
      width.value = w
      compact.value = w < 560
      if (!compact.value) fmtOpen.value = false
    }
  })
  ro.observe(rootEl.value)
})
onBeforeUnmount(() => ro?.disconnect())

// Split editor: keep the live preview lined up with the text as either side scrolls.
// Proportional (the two panes are different heights), with a one-frame guard so the
// programmatic scroll of the follower doesn't bounce back as a second sync.
let syncingScroll = false
function syncScroll(src) {
  if (!props.split || syncingScroll) return
  const lead = src === 'text' ? ta.value : previewSide.value
  const follow = src === 'text' ? previewSide.value : ta.value
  if (!lead || !follow) return
  const room = lead.scrollHeight - lead.clientHeight
  const ratio = room > 0 ? lead.scrollTop / room : 0
  syncingScroll = true
  follow.scrollTop = ratio * (follow.scrollHeight - follow.clientHeight)
  requestAnimationFrame(() => {
    syncingScroll = false
  })
}

function setValue(v) {
  emit('update:modelValue', v)
}

// Auto-grow: the textarea height follows its content so long text isn't trapped
// behind an inner scrollbar (the modal scrolls instead).
function autoGrow() {
  const el = ta.value
  if (!el || props.split) return // split panes are sized by the layout, not the text
  // Skip while the editor has no width — e.g. inside a collapsed panel (grid track
  // at 0fr). scrollHeight measured at ~0 width explodes (text wraps per glyph),
  // which would leave a giant textarea once the panel is shown again. The height is
  // recomputed via the exposed autoGrow() when the panel reappears.
  if (el.clientWidth === 0) return
  // An empty composer is one line high: Chrome counts a wrapped placeholder in
  // scrollHeight, which would open a blank box under the hint.
  if (composer.value && !el.value) {
    el.style.height = ''
    return
  }
  const sp = scrollParent(el)
  const top = sp ? sp.scrollTop : null
  el.style.height = 'auto'
  el.style.height = `${el.scrollHeight}px`
  if (sp != null && top != null) sp.scrollTop = top
}
watch(
  () => props.modelValue,
  () => nextTick(autoGrow),
)
watch(mode, (m) => {
  if (m === 'write') nextTick(autoGrow)
})
onMounted(() => nextTick(autoGrow))

// Preview checkbox toggle: rewrite the matching task marker in the markdown and
// ask the parent to persist (description save / comment update).
function onToggleCheck(i) {
  setValue(toggleTaskMarker(props.modelValue, i))
  emit('persist')
}

// ── markdown formatting (applied to the textarea selection) ──
// replaceRange swaps [s,e) for `text` and leaves the selection at [selS,selE).
// It goes through execCommand('insertText') where the browser supports it so the
// edit joins the textarea's native undo stack: writing the whole value back
// through v-model (the fallback path) makes Ctrl+Z skip straight past the edit
// instead of undoing it, which is especially wrong for wrap-on-typing — the user
// expects the character they just typed to come back off. The fallback still runs
// under WebKitGTK/Tauri and in jsdom, where execCommand is missing.
function replaceRange(s, e, text, selS, selE) {
  const el = ta.value
  if (!el) return
  el.focus()
  el.setSelectionRange(s, e)
  let native = false
  try {
    native =
      typeof document.execCommand === 'function' && document.execCommand('insertText', false, text)
  } catch {
    /* some webviews throw instead of returning false — take the fallback */
  }
  // The native path fires `input`, so onInput already pushed the new value out.
  if (!native) setValue(el.value.slice(0, s) + text + el.value.slice(e))
  nextTick(() => {
    el.focus()
    el.setSelectionRange(selS, selE)
  })
}
// applyEdit takes a whole-value result from utils/mdEdit and narrows it to the
// shortest changed span, so the undo step covers the edit and not the document.
function applyEdit(r) {
  const el = ta.value
  if (!el || !r) return
  const old = el.value
  let s = 0
  while (s < old.length && s < r.value.length && old[s] === r.value[s]) s++
  let oldEnd = old.length
  let newEnd = r.value.length
  while (oldEnd > s && newEnd > s && old[oldEnd - 1] === r.value[newEnd - 1]) {
    oldEnd--
    newEnd--
  }
  replaceRange(s, oldEnd, r.value.slice(s, newEnd), r.start, r.end)
}
function applyAround(before, after = before) {
  const el = ta.value
  if (!el) return
  const { selectionStart: s, selectionEnd: e } = el
  replaceRange(s, e, before + el.value.slice(s, e) + after, s + before.length, e + before.length)
  nextTick(refreshBubble)
}
// `prefix` is a literal or a (line, index) => string — the latter numbers the
// lines of an ordered list.
function applyLinePrefix(prefix) {
  const el = ta.value
  if (!el) return
  const { selectionStart: s, selectionEnd: e } = el
  const val = el.value
  const lineStart = val.lastIndexOf('\n', s - 1) + 1
  let lineEnd = val.indexOf('\n', e)
  if (lineEnd === -1) lineEnd = val.length
  const prefixed = val
    .slice(lineStart, lineEnd)
    .split('\n')
    .map((l, i) => (typeof prefix === 'function' ? prefix(l, i) : prefix) + l)
    .join('\n')
  replaceRange(lineStart, lineEnd, prefixed, lineStart, lineStart + prefixed.length)
  nextTick(refreshBubble)
}
// Link: no prompt — insert a markdown link skeleton starting at https:// and
// drop the caret right after it so the user just keeps typing the address.
function insertLink() {
  const el = ta.value
  if (!el) return
  const { selectionStart: s, selectionEnd: e } = el
  const text = el.value.slice(s, e) || t('documents.editor.snippet.linkText')
  const href = 'https://'
  const caret = s + 1 + text.length + 2 + href.length // after "https://"
  replaceRange(s, e, `[${text}](${href})`, caret, caret)
  nextTick(hideBubble)
}
// Blank lines around the body are load-bearing: without them `marked` keeps the
// contents of <details> as raw HTML and the markdown inside never renders.
function insertSpoiler() {
  const el = ta.value
  const sel = el ? el.value.slice(el.selectionStart, el.selectionEnd) : ''
  const summary = t('documents.editor.snippet.spoilerSummary')
  const body = sel || t('documents.editor.snippet.spoilerBody')
  insertAtCaret(`\n<details><summary>${summary}</summary>\n\n${body}\n\n</details>\n`)
}

// ── insert at caret (images, snippets) ──
function insertAtCaret(text) {
  const el = ta.value
  if (!el) {
    setValue(props.modelValue + text)
    return
  }
  const { selectionStart: s, selectionEnd: e } = el
  const caret = s + text.length
  replaceRange(s, e, text, caret, caret)
}

// ── image upload (button / paste / drop) ──
const imgInput = ref(null)
const uploading = ref(false)
async function uploadImage(file) {
  if (!file || !file.type.startsWith('image/')) return
  // `image/*` is wider than the media route's allowlist and says nothing about
  // size, so both refusals used to arrive from the server — as its hardcoded
  // Russian sentence, and only after a 12 MB paste had finished uploading.
  const bad = mediaPickMessage(file)
  if (bad) {
    message.error(bad)
    return
  }
  uploading.value = true
  try {
    const fd = new FormData()
    fd.append('file', file)
    const res = await uploadsApi.upload(fd)
    if (res.data?.url) insertAtCaret(`\n![${file.name || 'image'}](${res.data.url})\n`)
  } catch (err) {
    message.error(err.message || t('documents.editor.error.image'))
  } finally {
    uploading.value = false
  }
}
function pickImage() {
  imgInput.value?.click?.()
}
function onImgFile(e) {
  const f = e.target.files && e.target.files[0]
  e.target.value = ''
  if (f) uploadImage(f)
}
// Encode raw RGBA pixels (from the Tauri clipboard) into a PNG File.
async function rgbaToPngFile(rgba, width, height) {
  if (!width || !height) return null
  const canvas = document.createElement('canvas')
  canvas.width = width
  canvas.height = height
  canvas
    .getContext('2d')
    .putImageData(new ImageData(new Uint8ClampedArray(rgba), width, height), 0, 0)
  const blob = await new Promise((r) => canvas.toBlob(r, 'image/png'))
  return blob ? new File([blob], `clipboard-${Date.now()}.png`, { type: 'image/png' }) : null
}
async function onPaste(e) {
  const items = e.clipboardData && e.clipboardData.items
  for (const it of items || []) {
    if (it.type && it.type.startsWith('image/')) {
      const f = it.getAsFile()
      if (f) {
        e.preventDefault()
        uploadImage(f)
        return
      }
    }
  }
  // Desktop fallback: WebKitGTK (Linux) often doesn't expose a pasted image via
  // clipboardData — read it from the Tauri clipboard and encode to PNG.
  if (isTauri()) {
    try {
      const { readImage } = await import('@tauri-apps/plugin-clipboard-manager')
      const img = await readImage()
      const size = await img.size()
      const file = await rgbaToPngFile(await img.rgba(), size.width, size.height)
      if (file) uploadImage(file)
    } catch {
      /* no image in clipboard — ignore */
    }
  }
}
// ── task attachments (non-image drops + the paperclip picker) ──
// Images stay on the public media route so they render inline; anything else goes
// to the task's attachments and is linked, because only the attachment route can
// serve an arbitrary file back (see utils/download.js for why it's not a plain link).
const dragging = ref(false)
const attaching = ref(false)
const attachList = ref([])
const attachOpen = ref(false)
const canAttach = computed(() => !!props.attachTaskId)

async function uploadAttachment(file) {
  if (attachmentPickError(file)) {
    message.error(
      t('documents.editor.error.fileTooLarge', { limit: formatFileSize(ATTACHMENT_MAX_BYTES) }),
    )
    return
  }
  attaching.value = true
  try {
    const fd = new FormData()
    fd.append('file', file)
    const res = await tasksApi.uploadAttachment(props.attachTaskId, fd)
    const att = res.data
    if (att?.id)
      insertAtCaret(`[${att.filename || file.name}](/api/attachments/${att.id}/download)`)
    attachList.value = []
    emit('attachments-changed')
  } catch (err) {
    message.error(err.message || t('documents.editor.error.file'))
  } finally {
    attaching.value = false
  }
}
async function openAttachPicker() {
  if (!canAttach.value) return
  attachOpen.value = !attachOpen.value
  if (!attachOpen.value) return
  try {
    const res = await tasksApi.attachments(props.attachTaskId)
    attachList.value = res.data || []
  } catch (err) {
    message.error(err.message || t('documents.editor.error.attachments'))
    attachOpen.value = false
  }
}
function linkAttachment(att) {
  attachOpen.value = false
  insertAtCaret(`[${att.filename}](/api/attachments/${att.id}/download)`)
}

function onDragOver() {
  dragging.value = true
}
function onDragLeave() {
  dragging.value = false
}
function onDrop(e) {
  dragging.value = false
  const files = e.dataTransfer && e.dataTransfer.files
  if (!files || !files.length) return
  const list = [...files]
  const img = list.find((f) => f.type.startsWith('image/'))
  if (img) {
    e.preventDefault()
    uploadImage(img)
    return
  }
  if (canAttach.value) {
    e.preventDefault()
    uploadAttachment(list[0])
  }
}
function insertMermaid() {
  const from = t('documents.editor.snippet.mermaidFrom')
  const to = t('documents.editor.snippet.mermaidTo')
  insertAtCaret(`\n\`\`\`mermaid\nflowchart TD\n  A[${from}] --> B[${to}]\n\`\`\`\n`)
}

// Every button table below is a computed, not a plain array: as constants their
// titles would be resolved once during setup and stay in the language of the
// first render (pitfall 1 of #2799). `key` is the stable identity for v-for —
// the title used to serve as the key and now changes with the language.
//
// Inline marks only — these are what the selection bubble offers. The bubble
// floats over the text the user is reading, so it has to stay short.
const inlineTools = computed(() => [
  {
    key: 'bold',
    t: 'B',
    cls: 'b',
    title: t('documents.editor.tool.bold'),
    fn: () => applyAround('**'),
  },
  {
    key: 'italic',
    t: 'I',
    cls: 'i',
    title: t('documents.editor.tool.italic'),
    fn: () => applyAround('*'),
  },
  {
    key: 'strike',
    t: 'S',
    cls: 's',
    title: t('documents.editor.tool.strike'),
    fn: () => applyAround('~~'),
  },
  { key: 'code', t: '</>', title: t('documents.editor.tool.code'), fn: () => applyAround('`') },
  { key: 'link', icon: LinkOutline, title: t('documents.editor.tool.link'), fn: insertLink },
])
// Block-level actions rewrite whole lines; they live in the persistent toolbar.
const blockTools = computed(() => [
  {
    key: 'heading',
    t: 'H',
    title: t('documents.editor.tool.heading'),
    fn: () => applyLinePrefix('## '),
  },
  {
    key: 'bulletList',
    t: '•',
    title: t('documents.editor.tool.bulletList'),
    fn: () => applyLinePrefix('- '),
  },
  {
    key: 'orderedList',
    icon: ListOutline,
    title: t('documents.editor.tool.orderedList'),
    fn: () => applyLinePrefix(orderedListPrefix),
  },
  {
    key: 'checkbox',
    icon: CheckboxOutline,
    title: t('documents.editor.tool.checkbox'),
    fn: () => applyLinePrefix('- [ ] '),
  },
  {
    key: 'quote',
    t: '❝',
    title: t('documents.editor.tool.quote'),
    fn: () => applyLinePrefix('> '),
  },
  {
    key: 'spoiler',
    icon: ChevronDownOutline,
    title: t('documents.editor.tool.spoiler'),
    fn: insertSpoiler,
  },
])
const tools = computed(() => [...inlineTools.value, ...blockTools.value])
// The boxed composer's full button set (formatting + insert), rendered inline when
// there's room and inside the compact popover when there isn't. Computed so the
// upload/attach busy flags and the attach availability stay reactive.
const barTools = computed(() => [
  ...tools.value,
  {
    key: 'image',
    icon: ImageOutline,
    size: 16,
    title: t('documents.editor.tool.image'),
    fn: pickImage,
    busy: uploading.value,
  },
  {
    key: 'mermaid',
    icon: GitNetworkOutline,
    size: 16,
    title: t('documents.editor.tool.mermaid'),
    fn: insertMermaid,
  },
  ...(canAttach.value
    ? [
        {
          key: 'attach',
          icon: AttachOutline,
          size: 16,
          title: t('documents.editor.tool.attach'),
          fn: openAttachPicker,
          busy: attaching.value,
        },
      ]
    : []),
])

// ── the v2 bar (variant v2, and the composer's «Aa» strip) ──
// The mockup's row first (H B I · list · checkbox · link · @), then what the v1
// description header carried (image, diagram, file, fullscreen); the rest of the
// marks live in «⋯». As the editor narrows, the insert group and then the whole
// row fold into «⋯» too, so the bar never wraps or scrolls.
const V2_MAIN = ['heading', 'bold', 'italic', 'bulletList', 'checkbox', 'link', 'mention']
const V2_INSERT = ['image', 'mermaid', 'attach', 'expand']
const V2_MORE = ['orderedList', 'strike', 'code', 'quote', 'spoiler']
const v2Defs = computed(() => {
  const all = {}
  for (const b of barTools.value) all[b.key] = b
  // The mockup draws the list as a list icon; the numbered one gets the glyph.
  all.bulletList = { ...all.bulletList, icon: ListOutline, t: undefined }
  all.orderedList = { ...all.orderedList, icon: undefined, t: '1.' }
  // The paperclip opens a menu here (a new file, an image, a file the task has).
  if (all.attach) all.attach = { ...all.attach, fn: (e) => openAttachMenu(e?.currentTarget) }
  if (props.mentionItems.length)
    all.mention = {
      key: 'mention',
      icon: AtOutline,
      title: t('documents.editor.tool.mention'),
      fn: insertMention,
    }
  if (props.expandable)
    all.expand = {
      key: 'expand',
      icon: ExpandOutline,
      size: 15,
      title: t('documents.editor.tool.expand'),
      fn: openFullscreen,
    }
  return all
})
const pick = (keys) => keys.map((k) => v2Defs.value[k]).filter(Boolean)
const barMain = computed(() => (width.value && width.value < 380 ? [] : pick(V2_MAIN)))
const barInsert = computed(() => (width.value && width.value < 600 ? [] : pick(V2_INSERT)))
const barMore = computed(() => {
  const shown = new Set([...barMain.value, ...barInsert.value].map((b) => b.key))
  return pick([...V2_MAIN, ...V2_INSERT, ...V2_MORE]).filter((b) => !shown.has(b.key))
})
const moreItems = computed(() =>
  barMore.value.map((b) => ({ key: b.key, label: b.title, icon: b.icon })),
)
const moreOpen = ref(false)
const moreBtn = ref(null)
function onMoreSelect(key) {
  moreOpen.value = false
  // The attach row re-anchors its own menu on the «⋯» it came from.
  const b = v2Defs.value[key]
  if (key === 'attach') openAttachMenu(moreBtn.value)
  else b?.fn()
}
// Insert «@» at the caret (after a space, so it starts a handle) and open the
// people list as if it had been typed.
function insertMention() {
  const el = ta.value
  if (!el) return
  const before = el.value.slice(0, el.selectionStart)
  insertAtCaret(!before || /\s$/.test(before) ? '@' : ' @')
  nextTick(detectSuggest)
}

// ── issue templates (v2 bar) ──
const tplOpen = ref(false)
const tplBtn = ref(null)
const tplItems = computed(() =>
  props.templates.map((x) => ({ key: x.name, label: x.name, icon: DocumentTextOutline })),
)
function onTplSelect(name) {
  tplOpen.value = false
  emit('template', name)
}

// ── «Прикрепить» (composer, v2): an image, a new file, or one the task has ──
const fileInput = ref(null)
const attachMenu = ref(false)
const attachAnchor = ref(null)
const attachBtnEl = ref(null)
async function openAttachMenu(el) {
  attachAnchor.value = el || attachBtnEl.value || moreBtn.value
  attachMenu.value = true
  if (!canAttach.value) return
  try {
    const res = await tasksApi.attachments(props.attachTaskId)
    attachList.value = res.data || []
  } catch {
    attachList.value = []
  }
}
const attachItems = computed(() => {
  const out = [{ key: 'image', label: t('documents.editor.attach.image'), icon: ImageOutline }]
  if (!canAttach.value) return out
  out.push({ key: 'file', label: t('documents.editor.attach.file'), icon: CloudUploadOutline })
  if (attachList.value.length) {
    out.push({
      heading: t('documents.editor.attach.taskFiles'),
      aside: String(attachList.value.length),
    })
    for (const a of attachList.value)
      out.push({ key: `att:${a.id}`, label: a.filename, icon: AttachOutline })
  }
  return out
})
function onAttachSelect(key) {
  attachMenu.value = false
  if (key === 'image') pickImage()
  else if (key === 'file') fileInput.value?.click?.()
  else {
    const a = attachList.value.find((x) => `att:${x.id}` === key)
    if (a) linkAttachment(a)
  }
}
function onFileChosen(e) {
  const f = e.target.files && e.target.files[0]
  e.target.value = ''
  if (f) uploadAttachment(f)
}

// ── composer: the «Aa» strip ──
const fmtOn = ref(false)
const showBar = computed(() => v2.value || (composer.value && fmtOn.value))
function toggleFmt() {
  fmtOn.value = !fmtOn.value
  if (!fmtOn.value) mode.value = 'write'
  nextTick(() => ta.value?.focus())
}

// ── v2: leaving the editor ──
// What the text was when the editor opened. Esc / «Отмена» with nothing typed
// since just leaves; with something typed it first asks, inline in the footer —
// the host reverts to its own copy on `cancel`.
const baseline = ref(props.modelValue || '')
const confirming = ref(false)
function requestCancel() {
  if ((props.modelValue || '') !== baseline.value) {
    confirming.value = true
    nextTick(() => ta.value?.focus())
  } else emit('cancel')
}
function discard() {
  confirming.value = false
  emit('cancel')
}
function keepEditing() {
  confirming.value = false
  nextTick(() => ta.value?.focus())
}
function resetBaseline() {
  baseline.value = props.modelValue || ''
  confirming.value = false
}
function setMode(m) {
  mode.value = m
  if (m === 'write') nextTick(() => ta.value?.focus())
}

// ── selection bubble toolbar ──
const bubble = ref(null) // { top, left } in viewport coords, or null

// Mirror-div technique: measure the pixel position of a caret index in the
// textarea, so the bubble can float right above the selection.
function caretCoords(el, index) {
  const div = document.createElement('div')
  const cs = getComputedStyle(el)
  const copy = [
    'boxSizing',
    'width',
    'paddingTop',
    'paddingRight',
    'paddingBottom',
    'paddingLeft',
    'borderTopWidth',
    'borderRightWidth',
    'borderBottomWidth',
    'borderLeftWidth',
    'fontStyle',
    'fontVariant',
    'fontWeight',
    'fontSize',
    'fontFamily',
    'lineHeight',
    'letterSpacing',
    'textTransform',
    'wordSpacing',
  ]
  copy.forEach((p) => (div.style[p] = cs[p]))
  div.style.position = 'absolute'
  div.style.visibility = 'hidden'
  div.style.whiteSpace = 'pre-wrap'
  div.style.wordWrap = 'break-word'
  div.style.overflow = 'hidden'
  div.style.height = 'auto'
  div.textContent = el.value.slice(0, index)
  const span = document.createElement('span')
  span.textContent = el.value.slice(index) || '.'
  div.appendChild(span)
  document.body.appendChild(div)
  const top = span.offsetTop
  const left = span.offsetLeft
  document.body.removeChild(div)
  return { top, left, line: parseFloat(cs.lineHeight) || 18 }
}
function refreshBubble() {
  const el = ta.value
  if (!el || mode.value !== 'write') return hideBubble()
  if (el.selectionStart === el.selectionEnd) return hideBubble()
  const c = caretCoords(el, el.selectionStart)
  const rect = el.getBoundingClientRect()
  const x = rect.left + c.left - el.scrollLeft
  const y = rect.top + c.top - el.scrollTop
  const left = Math.min(Math.max(x, 8), window.innerWidth - 8)
  bubble.value = { left: `${left}px`, top: `${y - 10}px` }
}
function hideBubble() {
  bubble.value = null
}
function onTaScroll() {
  hideBubble()
  if (mq.value) updateSugPos()
  if (props.split) syncScroll('text')
}
function onBlur() {
  // Defer so a bubble-button mousedown can run before we tear it down.
  setTimeout(() => {
    hideBubble()
    emit('blur')
  }, 120)
}
onBeforeUnmount(hideBubble)

// Close the suggestion popup when clicking anywhere outside it (in addition to
// Esc / Enter / Backspace). Item clicks stay inside `.md2-mentions` and are
// handled by their own mousedown; a click in the textarea or elsewhere dismisses.
function onDocMousedown(e) {
  if (mq.value && !e.target.closest?.('.md2-mentions')) mq.value = null
}
onMounted(() => document.addEventListener('mousedown', onDocMousedown))
onBeforeUnmount(() => document.removeEventListener('mousedown', onDocMousedown))

// ── autocomplete: @-mentions and /-commands ──
// One popup, two sources. `mq` holds the open query: { kind, start, query },
// where `start` is the index of the trigger character in the textarea value.
const mq = ref(null)
const mqIndex = ref(0)
// Viewport position of the popup, anchored just BELOW the trigger line so it does
// not cover the text the user is typing (like GitLab). Measured with the same
// mirror-div technique as the selection bubble; the popup is `position: fixed`
// (viewport coords) so it escapes the editor's clipping — inside the boxed
// composer an `absolute` popup got cropped by the rounded border box.
const sugPos = ref(null)
const sugEl = ref(null) // the rendered <ul>, for post-render clamp/flip measurement
let sugAnchorY = 0 // viewport Y of the trigger line top — used when flipping above
const isCmd = computed(() => mq.value?.kind === 'command')
const mentionMatches = computed(() => {
  if (!mq.value || isCmd.value) return []
  const q = mq.value.query.toLowerCase()
  return props.mentionItems
    .filter((m) => m.label.toLowerCase().includes(q) || (m.display || '').toLowerCase().includes(q))
    .slice(0, 8)
})
const commandMatches = computed(() =>
  isCmd.value ? matchCommands(props.commandItems, mq.value.query) : [],
)
const sugMatches = computed(() => (isCmd.value ? commandMatches.value : mentionMatches.value))

function detectSuggest() {
  const el = ta.value
  if (!el) return (mq.value = null)
  // Read the live DOM value: the modelValue prop lags one input behind.
  const upto = el.value.slice(0, el.selectionStart)
  const m = props.mentionItems.length ? upto.match(/(^|\s)@([^\s@]*)$/) : null
  if (m) {
    mq.value = { kind: 'mention', start: el.selectionStart - m[2].length - 1, query: m[2] }
    mqIndex.value = 0
    updateSugPos()
    return
  }
  // Commands trigger only at the start of a line — see detectSlashQuery.
  const slash = props.commandItems.length ? detectSlashQuery(upto) : null
  if (slash) {
    mq.value = { kind: 'command', ...slash }
    mqIndex.value = 0
    updateSugPos()
    return
  }
  mq.value = null
}
// Does the open popup still describe the text the caret actually sits in?
//
// `detectSuggest` runs from `onInput` alone, but the caret moves without any
// input at all — ArrowLeft/Right, Home/End, PageUp/Down. Nothing re-ran the
// detector then, so `mq.start` went on pointing at a trigger the caret had left,
// and `pickSuggest` spliced the chosen mention over that stale range: with the
// caret sent to the line start, `replaceRange(start, selectionStart, …)` gets an
// end BEFORE its start and pastes `value.slice(0, start) + text + value.slice(end)`
// — the stretch between them is emitted twice, i.e. "привет @an" became
// "привет @anna привет @an". On the native `execCommand` path the damage is
// quieter but no smaller: the mention lands wherever the caret went and the
// half-typed handle stays behind.
//
// Re-deriving from the live DOM rather than tracking every way a caret can move
// (clicks, drag-selection, browser shortcuts, a webview's own gestures) is what
// makes this hold for the paths we don't enumerate below.
function suggestLive() {
  const el = ta.value
  if (!el || !mq.value) return false
  const end = el.selectionStart
  if (end !== el.selectionEnd || end < mq.value.start) return false
  const trigger = mq.value.kind === 'command' ? '/' : '@'
  return el.value.slice(mq.value.start, end) === trigger + mq.value.query
}
// Keys that only move the caret: the popup is never their target, and leaving it
// on screen would keep offering matches for text the caret is no longer in.
// `suggestLive` is the backstop for everything this list cannot name.
const CARET_KEYS = new Set(['ArrowLeft', 'ArrowRight', 'Home', 'End', 'PageUp', 'PageDown'])
// Place the popup below the trigger char's line, aligned under the trigger.
// Coords are viewport-relative (the popup is `position: fixed`). After it renders
// we measure its real size and clamp it into the viewport / flip it above the
// line when there isn't room below — see clampSugPos.
function updateSugPos() {
  const el = ta.value
  if (!el || !mq.value) return (sugPos.value = null)
  const c = caretCoords(el, mq.value.start)
  const rect = el.getBoundingClientRect()
  const x = rect.left + c.left - el.scrollLeft
  const y = rect.top + c.top - el.scrollTop
  sugAnchorY = y
  sugPos.value = { top: `${y + c.line + 6}px`, left: `${Math.max(8, x)}px` }
  nextTick(clampSugPos)
}
function clampSugPos() {
  const box = sugEl.value
  if (!box || !sugPos.value) return
  const r = box.getBoundingClientRect()
  const vw = document.documentElement.clientWidth
  const vh = document.documentElement.clientHeight
  let left = parseFloat(sugPos.value.left)
  let top = parseFloat(sugPos.value.top)
  if (left + r.width > vw - 8) left = Math.max(8, vw - 8 - r.width)
  // Not enough room below and more room above the line → flip the popup up.
  if (top + r.height > vh - 8 && sugAnchorY - 6 - r.height > 8) top = sugAnchorY - 6 - r.height
  sugPos.value = { top: `${top}px`, left: `${left}px` }
}
// Accepting a row swaps the half-typed query for the full mention/command, and
// goes through replaceRange like every other insertion here: writing the whole
// value back instead would drop the textarea's native undo stack, so one picked
// mention would cost the user the undo history of a whole comment.
function pickSuggest(item) {
  if (!item || !mq.value) return
  // The row may have been clicked long after the caret wandered off the query —
  // a mouse click on the popup moves nothing. Dropping the pick beats writing the
  // mention into a range that no longer spells it.
  if (!suggestLive()) return (mq.value = null)
  const el = ta.value
  const cmd = isCmd.value
  const insert = cmd ? commandInsertText(item) : `@${item.label} `
  const start = mq.value.start
  const caret = start + insert.length
  mq.value = null
  replaceRange(start, el.selectionStart, insert, caret, caret)
}

// Tab indents rather than moving focus — but that turns the field into a keyboard
// trap, and the description editor sits inside a modal whose buttons then become
// unreachable. Esc releases the capture (the next Tab leaves), typing takes it back.
const tabTraps = ref(true)

function onInput(e) {
  setValue(e.target.value)
  tabTraps.value = true
  autoGrow()
  detectSuggest()
  hideBubble()
}
function onSelect() {
  if (!mq.value) refreshBubble()
}
function onKeydown(e) {
  // v2 «Отменить правки?» is up: ↵ is «Да», Esc is «Нет»; any other key goes on
  // typing and takes the question away.
  if (confirming.value && !e.isComposing) {
    if (e.key === 'Enter' && !e.ctrlKey && !e.metaKey && !e.shiftKey) {
      e.preventDefault()
      e.stopPropagation()
      discard()
      return
    }
    if (e.key === 'Escape') {
      e.preventDefault()
      e.stopPropagation()
      keepEditing()
      return
    }
    confirming.value = false
  }
  // A keystroke is the first thing that happens after a caret move, so this is
  // where a popup left behind by one is noticed and dropped — before any branch
  // below can act on it.
  if (mq.value && !suggestLive()) mq.value = null
  if (mq.value && sugMatches.value.length) {
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      mqIndex.value = (mqIndex.value + 1) % sugMatches.value.length
      return
    }
    if (e.key === 'ArrowUp') {
      e.preventDefault()
      mqIndex.value = (mqIndex.value - 1 + sugMatches.value.length) % sugMatches.value.length
      return
    }
    // Only the BARE Enter/Tab belongs to the popup. The modified ones are
    // separate gestures of this editor and were being eaten by it: Ctrl/⌘+Enter
    // is «send» (it reached `emit('submit')` only with the popup shut, so a
    // comment ending in «@an» refused to send and typed a mention instead),
    // Shift+Enter is the plain line break, Shift+Tab the outdent.
    if ((e.key === 'Enter' || e.key === 'Tab') && !e.ctrlKey && !e.metaKey && !e.shiftKey) {
      e.preventDefault()
      pickSuggest(sugMatches.value[mqIndex.value])
      return
    }
    if (e.key === 'Escape') {
      e.preventDefault()
      // The popup is a layer of its own: this Esc is spent on it and must not
      // reach the modal underneath (naive-ui closes n-modal from a document
      // keydown), or dismissing an @-suggestion would throw away the draft
      // together with the whole task modal.
      e.stopPropagation()
      mq.value = null
      return
    }
  }
  // Caret-only keys: close the popup as the caret leaves, rather than waiting for
  // the next keystroke to find it stale.
  if (mq.value && CARET_KEYS.has(e.key)) mq.value = null
  if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
    e.preventDefault()
    if (props.sending) return
    emit('submit')
    return
  }
  const el = ta.value
  if (!el || e.ctrlKey || e.metaKey || e.altKey) return
  const { selectionStart: s, selectionEnd: end } = el
  // Typing a bracket/quote over a selection wraps it instead of replacing it.
  // IME composition is skipped — swallowing those keys breaks composed input.
  if (!e.isComposing && e.key.length === 1 && WRAP_PAIRS[e.key] !== undefined && s !== end) {
    e.preventDefault()
    applyEdit(wrapSelection(el.value, s, end, e.key))
    return
  }
  // No selection: auto-close brackets / quotes / backticks and step over a closer
  // the caret already sits before.
  if (!e.isComposing && e.key.length === 1 && s === end) {
    const act = autoClose(el.value, s, e.key)
    if (act) {
      e.preventDefault()
      if (act.type === 'over') {
        el.setSelectionRange(act.caret, act.caret)
      } else {
        applyEdit(act)
      }
      return
    }
  }
  // Backspace between an empty auto-inserted pair deletes both halves.
  if (e.key === 'Backspace' && !e.isComposing && s === end) {
    const r = deletePair(el.value, s)
    if (r) {
      e.preventDefault()
      applyEdit(r)
      return
    }
  }
  // Enter continues a list (or closes a code fence); Shift+Enter is a plain break.
  if (e.key === 'Enter' && !e.isComposing && !e.shiftKey && s === end) {
    const r = handleEnter(el.value, s)
    if (r) {
      e.preventDefault()
      applyEdit(r)
      return
    }
  }
  if (e.key === 'Tab' && tabTraps.value) {
    e.preventDefault()
    if (e.shiftKey) applyEdit(outdentLines(el.value, s, end))
    else if (s === end) insertAtCaret(INDENT)
    else applyEdit(indentLines(el.value, s, end))
    return
  }
  // Esc asks the host to dismiss the composer (the reply form listens; the
  // description doesn't). It's also the tab-trap release: the first Esc gives Tab
  // back to the browser and is kept from the surrounding modal, the second one
  // propagates and lets the modal close as usual.
  // v2: Esc leaves the editor — after asking, when there is typing to lose — and
  // never reaches the dialog underneath.
  if (e.key === 'Escape' && v2.value) {
    e.preventDefault()
    e.stopPropagation()
    requestCancel()
    return
  }
  if (e.key === 'Escape') {
    emit('cancel')
    if (tabTraps.value) {
      tabTraps.value = false
      e.stopPropagation()
    }
  }
}

// Lazy + async so the two components can reference each other (the modal hosts a
// MarkdownEditor of its own) without a circular import at module-eval time.
const MarkdownFullscreenModal = defineAsyncComponent(() => import('./MarkdownFullscreenModal.vue'))
// Two flags so the open/close animation actually plays: `fsOpen` mounts the modal
// (lazily — one async load, not one per editor on the page), `fsShow` drives its
// visibility. Tying the v-if to `show` would rip the modal out of the DOM the
// instant it closes, skipping the leave transition; instead we keep it mounted
// until n-modal's after-leave fires. `internal-appear` gives it the enter
// transition when it mounts already-shown.
const fsOpen = ref(false)
const fsShow = ref(false)
function openFullscreen() {
  fsOpen.value = true
  fsShow.value = true
}
function onFsShow(v) {
  fsShow.value = v
}
function onFsAfterLeave() {
  fsOpen.value = false
}

// Who this text notifies: read off the mentions it renders, not off the popup.
// The popup is one way to write a handle — typing it, pasting it or having the
// reply composer seed it are the others, and all four draw the same chip.
function getMentions() {
  return mentionIdsIn(props.modelValue, props.mentionItems)
}
function clear() {
  setValue('')
  hideBubble()
}
function focus() {
  const el = ta.value
  if (!el) return
  el.focus()
  // Place the caret at the end so a programmatically prefilled draft (e.g. a
  // reply seeded with "@author, ") is typed after, not before, the seed text.
  const end = el.value.length
  el.setSelectionRange?.(end, end)
}
defineExpose({
  getMentions,
  clear,
  focus,
  pickImage,
  insertMermaid,
  toggleMode,
  autoGrow,
  openFullscreen,
  requestCancel,
  resetBaseline,
})
</script>

<template>
  <div
    ref="rootEl"
    class="md2"
    :class="{
      'md2-boxed': boxed,
      'md2-splitmode': split,
      'md2-v2': v2,
      'md2-composer': composer,
      'md2-fmt': composer && fmtOn,
    }"
  >
    <!-- Default variant: a slim top toolbar — insert actions plus a single
         preview/edit toggle (replaces the old Написать / Просмотр tabs). The boxed
         composer moves every control into the bottom toolbar instead. -->
    <div v-if="variant === 'default' && toolbar" class="md2-tabs">
      <span class="md2-spacer" />
      <template v-if="writing">
        <button
          type="button"
          class="md2-act"
          :class="{ busy: uploading }"
          :title="$t('documents.editor.tool.image')"
          @click="pickImage"
        >
          <n-icon :component="ImageOutline" :size="16" />
        </button>
        <button
          type="button"
          class="md2-act"
          :title="$t('documents.editor.tool.mermaid')"
          @click="insertMermaid"
        >
          <n-icon :component="GitNetworkOutline" :size="16" />
        </button>
      </template>
      <button
        v-if="!split"
        type="button"
        class="md2-act"
        :title="
          mode === 'write' ? $t('documents.editor.tool.preview') : $t('documents.editor.tool.edit')
        "
        @click="toggleMode"
      >
        <n-icon :component="mode === 'write' ? EyeOutline : CreateOutline" :size="16" />
      </button>
    </div>

    <!-- v2 bar: the Написать / Просмотр segment and the tools in one strip on the
         input surface (StagePage mockup). mousedown.prevent keeps the caret and
         the selection in the field the tools act on. -->
    <div v-if="showBar" class="md2-bar" data-testid="md2-bar">
      <div class="md2-seg" role="tablist">
        <button
          type="button"
          role="tab"
          class="md2-seg-b"
          :class="{ on: mode === 'write' }"
          :aria-selected="mode === 'write'"
          data-testid="md2-write"
          @mousedown.prevent
          @click="setMode('write')"
        >
          {{ $t('documents.editor.write') }}
        </button>
        <button
          type="button"
          role="tab"
          class="md2-seg-b"
          :class="{ on: mode === 'preview' }"
          :aria-selected="mode === 'preview'"
          data-testid="md2-preview"
          @mousedown.prevent
          @click="setMode('preview')"
        >
          {{ $t('documents.editor.previewTab') }}
        </button>
      </div>
      <span class="md2-spacer" />
      <span v-if="mode === 'write'" class="md2-tools">
        <button
          v-for="b in barMain"
          :key="b.key"
          type="button"
          class="md2-ti"
          :class="[b.cls, { busy: b.busy }]"
          :title="b.title"
          :data-testid="`md2-tool-${b.key}`"
          @mousedown.prevent="b.fn($event)"
        >
          <n-icon v-if="b.icon" :component="b.icon" :size="15" />
          <template v-else>{{ b.t }}</template>
        </button>
        <span v-if="barMain.length && barInsert.length" class="md2-tsep" />
        <button
          v-for="b in barInsert"
          :key="b.key"
          type="button"
          class="md2-ti"
          :class="{ busy: b.busy }"
          :title="b.title"
          :data-testid="`md2-tool-${b.key}`"
          @mousedown.prevent="b.fn($event)"
        >
          <n-icon :component="b.icon" :size="15" />
        </button>
        <button
          v-if="templates.length"
          ref="tplBtn"
          type="button"
          class="md2-ti"
          :class="{ on: tplOpen }"
          :title="templateLabel"
          data-testid="md2-templates"
          @mousedown.prevent
          @click="tplOpen = !tplOpen"
        >
          <n-icon :component="DocumentTextOutline" :size="15" />
        </button>
        <button
          v-if="barMore.length"
          ref="moreBtn"
          type="button"
          class="md2-ti"
          :class="{ on: moreOpen }"
          :title="$t('documents.editor.tool.more')"
          data-testid="md2-more"
          @mousedown.prevent
          @click="moreOpen = !moreOpen"
        >
          <n-icon :component="EllipsisHorizontalOutline" :size="16" />
        </button>
      </span>
    </div>

    <!-- Enumerated rather than image/*: the server rejects SVG (it would run as
         active content on our origin), so the picker shouldn't offer it. -->
    <input
      ref="imgInput"
      type="file"
      accept="image/png,image/jpeg,image/gif,image/webp,image/bmp"
      hidden
      @change="onImgFile"
    />
    <input v-if="canAttach" ref="fileInput" type="file" hidden @change="onFileChosen" />

    <!-- The composer's row (avatar · field · Aa · Прикрепить · Отправить);
         everywhere else this wrapper generates no box. -->
    <div class="md2-main">
      <UserAvatar
        v-if="composer && avatar"
        class="md2-cava"
        :user-id="avatar.id || ''"
        :src="avatar.src"
        :name="avatar.name || '?'"
      />
      <div class="md2-body">
        <!-- Cross-fade (NOT mode="out-in"): out-in defers mounting the incoming pane
           until the outgoing one finishes leaving, and a parent re-render in between
           (the header's write/preview buttons key off `descMode`) wedged the
           transition — it settled on an empty slot, so switching a filled description
           into edit mode showed the toolbar but no textarea (#2742). A plain
           cross-fade mounts both at once; the leaving pane is taken out of flow in CSS
           so the layout doesn't jump. -->
        <Transition name="md2-fade" @after-enter="autoGrow">
          <div v-if="writing" key="write" class="md2-write" :class="{ dragging }">
            <textarea
              ref="ta"
              :value="modelValue"
              :placeholder="fieldPlaceholder"
              :rows="minRows"
              spellcheck="false"
              @input="onInput"
              @keydown="onKeydown"
              @select="onSelect"
              @mouseup="onSelect"
              @scroll="onTaScroll"
              @blur="onBlur"
              @paste="onPaste"
              @dragover.prevent="onDragOver"
              @dragleave="onDragLeave"
              @drop="onDrop"
            />

            <Transition name="bubble">
              <div v-if="bubble" class="md2-bubble" :style="bubble">
                <button
                  v-for="b in inlineTools"
                  :key="b.key"
                  type="button"
                  :class="b.cls"
                  :title="b.title"
                  @mousedown.prevent="b.fn"
                >
                  <n-icon v-if="b.icon" :component="b.icon" :size="15" />
                  <template v-else>{{ b.t }}</template>
                </button>
              </div>
            </Transition>

            <Transition name="md2-sug">
              <ul
                v-if="mq && !isCmd && mentionMatches.length"
                ref="sugEl"
                class="md2-mentions"
                :style="sugPos"
              >
                <template v-for="(m, i) in mentionMatches" :key="m.gitlab ? `gl:${m.label}` : m.id">
                  <li
                    v-if="m.gitlab && (i === 0 || !mentionMatches[i - 1].gitlab)"
                    class="md2-mention-sep"
                  >
                    GitLab
                  </li>
                  <li
                    class="md2-mention-item"
                    :class="{ active: i === mqIndex }"
                    @mousedown.prevent="pickSuggest(m)"
                  >
                    <UserAvatar
                      class="md2-mention-av"
                      :user-id="m.avatarUserId"
                      :src="m.avatarSrc"
                      :name="m.display || m.label"
                    />
                    <span class="md2-mention-name">{{ m.display || m.label }}</span>
                    <!-- The login is what actually gets inserted; without it the
                       row would promise a name and type something else. -->
                    <span v-if="m.hint" class="md2-mention-hint">{{ m.hint }}</span>
                  </li>
                </template>
              </ul>
            </Transition>

            <!-- Quick actions: `/ключ` in mono plus the description, GitLab-style.
               Custom (dictionary) entries carry a neutral badge — they are hints
               for a human reader, the backend never executes them. -->
            <Transition name="md2-sug">
              <ul
                v-if="mq && isCmd && commandMatches.length"
                ref="sugEl"
                class="md2-mentions md2-cmds"
                :style="sugPos"
              >
                <li
                  v-for="(cmd, i) in commandMatches"
                  :key="cmd.key"
                  class="md2-mention-item md2-cmd-item"
                  :class="{ active: i === mqIndex }"
                  @mousedown.prevent="pickSuggest(cmd)"
                >
                  <code class="md2-cmd-key">/{{ cmd.key }}</code>
                  <span class="md2-cmd-desc">{{ cmd.description }}</span>
                  <span v-if="!cmd.builtin" class="md2-cmd-tag">{{
                    $t('documents.editor.customCommand')
                  }}</span>
                </li>
              </ul>
            </Transition>
          </div>

          <RichContent
            v-else
            key="preview"
            class="md2-preview"
            :source="modelValue"
            :members="mentionItems"
            interactive
            task-refs
            :empty="$t('documents.editor.previewEmpty')"
            @toggle="onToggleCheck"
          />
        </Transition>

        <!-- Split only: the preview sits beside the text instead of replacing it,
           and follows every keystroke (same v-model, no second copy). RichContent is
           a multi-root component, so it never inherits this file's scoped attribute —
           the grid/overflow rules have to land on a native wrapper, or the preview
           spills its content over the toolbar with no scrollbar (#2744). -->
        <div
          v-if="split"
          ref="previewSide"
          class="md2-preview-side"
          @scroll="syncScroll('preview')"
        >
          <RichContent
            class="md2-preview"
            :source="modelValue"
            :members="mentionItems"
            interactive
            task-refs
            :empty="$t('documents.editor.previewEmpty')"
            @toggle="onToggleCheck"
          />
        </div>

        <!-- Default variant: a formatting bar under the text. The selection bubble
           only carries inline marks, and the header row (when it is rendered at
           all — TaskModal hosts it in .desc-head) only carries insert/preview, so
           lists, checkboxes and spoilers need a home of their own. -->
        <div v-if="variant === 'default' && writing" class="md2-toolbar md2-format">
          <!-- Without the top bar (the fullscreen window) its two inserts come
               down here. -->
          <template v-if="!toolbar">
            <button
              type="button"
              class="md2-tbtn"
              :class="{ busy: uploading }"
              :title="$t('documents.editor.tool.image')"
              @mousedown.prevent="pickImage"
            >
              <n-icon :component="ImageOutline" :size="15" />
            </button>
            <button
              type="button"
              class="md2-tbtn"
              :title="$t('documents.editor.tool.mermaid')"
              @mousedown.prevent="insertMermaid"
            >
              <n-icon :component="GitNetworkOutline" :size="15" />
            </button>
            <span class="md2-tsep" />
          </template>
          <button
            v-for="b in tools"
            :key="b.key"
            type="button"
            class="md2-tbtn"
            :class="b.cls"
            :title="b.title"
            @mousedown.prevent="b.fn"
          >
            <n-icon v-if="b.icon" :component="b.icon" :size="15" />
            <template v-else>{{ b.t }}</template>
          </button>
          <span v-if="canAttach || expandable" class="md2-tsep" />
          <button
            v-if="canAttach"
            type="button"
            class="md2-tbtn"
            :class="{ busy: attaching }"
            :title="$t('documents.editor.tool.attach')"
            @mousedown.prevent="openAttachPicker"
          >
            <n-icon :component="AttachOutline" :size="16" />
          </button>
          <button
            v-if="expandable"
            type="button"
            class="md2-tbtn"
            :title="$t('documents.editor.tool.expand')"
            @mousedown.prevent="openFullscreen"
          >
            <n-icon :component="ExpandOutline" :size="15" />
          </button>
        </div>

        <!-- Files already on the task: picking one links it into the text. Rendered
           in flow (not a floating popup) so it can't be clipped by the modal. -->
        <ul v-if="attachOpen" class="md2-attach">
          <li v-if="!attachList.length" class="md2-attach-empty">
            {{ $t('documents.editor.attachEmpty') }}
          </li>
          <li
            v-for="a in attachList"
            :key="a.id"
            class="md2-attach-item"
            @mousedown.prevent="linkAttachment(a)"
          >
            <n-icon :component="AttachOutline" :size="14" />
            <span class="md2-attach-name">{{ a.filename }}</span>
          </li>
        </ul>

        <!-- Compact composer: the format/insert buttons live in this popover instead of
           on the bar, so a narrow composer can't overflow (see the toolbar below). In
           flow (not floating) so it can't be clipped by the box. -->
        <div v-if="boxed && compact && fmtOpen && mode === 'write'" class="md2-fmtbar">
          <button
            v-for="b in barTools"
            :key="b.key"
            type="button"
            class="md2-tbtn"
            :class="[b.cls, { busy: b.busy }]"
            :title="b.title"
            @mousedown.prevent="b.fn"
          >
            <n-icon v-if="b.icon" :component="b.icon" :size="b.size || 15" />
            <template v-else>{{ b.t }}</template>
          </button>
        </div>

        <!-- Boxed variant: persistent toolbar under the text (formatting is still also
         available via the selection bubble). mousedown.prevent keeps the caret /
         selection in the textarea so the format buttons act on it. When the composer
         is narrow, the whole format/insert group folds into a single menu button so
         the bar (and the send button on its right) never overflows the box (#2743). -->
        <div v-if="boxed" class="md2-toolbar">
          <template v-if="mode === 'write'">
            <button
              v-if="compact"
              type="button"
              class="md2-tbtn"
              :class="{ active: fmtOpen }"
              :title="$t('documents.editor.tool.format')"
              @mousedown.prevent="fmtOpen = !fmtOpen"
            >
              <n-icon :component="EllipsisHorizontalOutline" :size="18" />
            </button>
            <template v-else>
              <button
                v-for="b in barTools"
                :key="b.key"
                type="button"
                class="md2-tbtn"
                :class="[b.cls, { busy: b.busy }]"
                :title="b.title"
                @mousedown.prevent="b.fn"
              >
                <n-icon v-if="b.icon" :component="b.icon" :size="b.size || 15" />
                <template v-else>{{ b.t }}</template>
              </button>
            </template>
          </template>
          <span class="md2-spacer" />
          <button
            v-if="expandable"
            type="button"
            class="md2-tbtn"
            :title="$t('documents.editor.tool.expand')"
            @mousedown.prevent="openFullscreen"
          >
            <n-icon :component="ExpandOutline" :size="15" />
          </button>
          <button
            type="button"
            class="md2-tbtn"
            :title="
              mode === 'write'
                ? $t('documents.editor.tool.preview')
                : $t('documents.editor.tool.edit')
            "
            @mousedown.prevent="toggleMode"
          >
            <n-icon :component="mode === 'write' ? EyeOutline : CreateOutline" :size="16" />
          </button>
          <button
            v-if="send"
            type="button"
            class="md2-send"
            :class="{ busy: sending }"
            :disabled="sending"
            :title="
              sending ? $t('documents.editor.tool.sending') : $t('documents.editor.tool.send')
            "
            @mousedown.prevent="onSend"
          >
            <TesseraSpinner v-if="sending" :size="16" variant="white" />
            <n-icon v-else :component="SendOutline" :size="16" />
          </button>
        </div>
      </div>
      <template v-if="composer">
        <button
          type="button"
          class="md2-cbtn md2-aa"
          :class="{ on: fmtOn }"
          :aria-pressed="fmtOn"
          :title="$t('documents.editor.tool.formatToggle')"
          data-testid="composer-format"
          @mousedown.prevent
          @click="toggleFmt"
        >
          {{ AA }}
        </button>
        <button
          ref="attachBtnEl"
          type="button"
          class="md2-cbtn"
          :class="{ busy: uploading || attaching, on: attachMenu }"
          :title="$t('documents.editor.tool.attachMenu')"
          data-testid="composer-attach"
          @mousedown.prevent
          @click="attachMenu ? (attachMenu = false) : openAttachMenu(attachBtnEl)"
        >
          <n-icon :component="AttachOutline" :size="17" />
        </button>
        <button
          v-if="send"
          type="button"
          class="md2-csend"
          :class="{ busy: sending, icon: narrowSend }"
          :disabled="sending"
          :title="sending ? $t('documents.editor.tool.sending') : $t('documents.editor.tool.send')"
          data-testid="composer-send"
          @mousedown.prevent
          @click="onSend"
        >
          <TesseraSpinner v-if="sending" :size="14" variant="white" />
          <n-icon v-else-if="narrowSend" :component="SendOutline" :size="15" />
          <template v-else>{{ sendLabel ?? $t('documents.editor.send') }}</template>
          <span v-if="!narrowSend" class="md2-ckbd">{{ CAPS.modEnter }}</span>
        </button>
      </template>
    </div>

    <!-- v2 footer: what # and @ do, then leave or save. «Отменить правки?» takes
         the row over when Esc / «Отмена» would throw typing away. -->
    <div v-if="v2 && footer" class="md2-foot" :class="{ confirming }">
      <template v-if="confirming">
        <span class="md2-confirm-q">{{ $t('documents.editor.discardAsk') }}</span>
        <span class="md2-spacer" />
        <button
          type="button"
          class="md2-fbtn"
          data-testid="md2-keep"
          @mousedown.prevent
          @click="keepEditing"
        >
          {{ $t('common.answer.no') }} <Kbd>{{ CAPS.escape }}</Kbd>
        </button>
        <button
          type="button"
          class="md2-fbtn danger"
          data-testid="md2-discard"
          @mousedown.prevent
          @click="discard"
        >
          {{ $t('common.answer.yes') }} <Kbd>{{ CAPS.enter }}</Kbd>
        </button>
      </template>
      <template v-else>
        <span v-if="!narrowSend" class="md2-hints">
          <Kbd>{{ '#' }}</Kbd>
          <span>{{ $t('documents.editor.hintTask') }}</span>
          <template v-if="mentionItems.length">
            <Kbd>{{ '@' }}</Kbd>
            <span>{{ $t('documents.editor.hintPerson') }}</span>
          </template>
        </span>
        <span class="md2-spacer" />
        <button
          type="button"
          class="md2-fbtn"
          data-testid="md2-cancel"
          @mousedown.prevent
          @click="requestCancel"
        >
          {{ $t('common.action.cancel') }} <Kbd>{{ CAPS.escape }}</Kbd>
        </button>
        <button
          type="button"
          class="md2-fbtn primary"
          :class="{ busy: sending }"
          :disabled="sending"
          data-testid="md2-save"
          @mousedown.prevent
          @click="onSend"
        >
          <TesseraSpinner v-if="sending" :size="12" variant="white" />
          {{ saveLabel ?? $t('common.action.save') }} <Kbd>{{ CAPS.modEnter }}</Kbd>
        </button>
      </template>
    </div>

    <!-- The bar's menus: drawn into the dialog the editor sits in (AnchorLayer),
         so they keep the keyboard inside the task modal's focus trap. -->
    <template v-if="v2 || composer">
      <AnchorLayer
        :show="moreOpen"
        :anchor="() => moreBtn"
        :placement="composer ? 'top-end' : 'bottom-end'"
        focus=".am"
        testid="md2-more-menu"
        @update:show="moreOpen = $event"
      >
        <ActionMenu
          inline
          :show="moreOpen"
          :width="240"
          :items="moreItems"
          testid="md2-more-items"
          @update:show="moreOpen = $event"
          @select="onMoreSelect"
        />
      </AnchorLayer>
      <AnchorLayer
        :show="tplOpen"
        :anchor="() => tplBtn"
        placement="bottom-end"
        focus=".am"
        testid="md2-tpl-menu"
        @update:show="tplOpen = $event"
      >
        <ActionMenu
          inline
          :show="tplOpen"
          :width="260"
          :header="templateLabel"
          :items="tplItems"
          testid="md2-tpl-items"
          @update:show="tplOpen = $event"
          @select="onTplSelect"
        />
      </AnchorLayer>
      <AnchorLayer
        :show="attachMenu"
        :anchor="() => attachAnchor"
        :placement="composer ? 'top-end' : 'bottom-end'"
        focus=".am"
        testid="md2-attach-menu"
        @update:show="attachMenu = $event"
      >
        <ActionMenu
          inline
          :show="attachMenu"
          :width="280"
          :items="attachItems"
          testid="md2-attach-items"
          @update:show="attachMenu = $event"
          @select="onAttachSelect"
        />
      </AnchorLayer>
    </template>

    <!-- Fullscreen editing shares this component's v-model, so there is no second
         copy of the text to merge back on close. -->
    <MarkdownFullscreenModal
      v-if="expandable && fsOpen"
      :show="fsShow"
      :model-value="modelValue"
      :placeholder="placeholder"
      :mention-items="mentionItems"
      :command-items="commandItems"
      :attach-task-id="attachTaskId"
      @update:show="onFsShow"
      @after-leave="onFsAfterLeave"
      @update:model-value="setValue"
      @attachments-changed="emit('attachments-changed')"
      @persist="emit('persist')"
    />
  </div>
</template>

<style scoped>
/* Seamless — the editor shares the modal's background, no box of its own. */
.md2 {
  width: 100%;
}
/* Slim top toolbar (insert actions + preview/edit toggle). */
.md2-tabs {
  display: flex;
  align-items: center;
  gap: 2px;
  margin-bottom: 6px;
}
.md2-spacer {
  flex: 1;
}
.md2-act {
  border: none;
  background: transparent;
  color: var(--t-text2);
  cursor: pointer;
  padding: 4px 6px;
  border-radius: 6px;
  display: inline-flex;
  align-items: center;
  transition:
    background 0.12s ease,
    color 0.12s ease;
}
.md2-act:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.md2-act.busy {
  opacity: 0.5;
  pointer-events: none;
}
/* Cross-fade between write / preview panes. The leaving pane is pulled out of flow
   (absolute) so the incoming one owns the layout — no jump, and no `mode="out-in"`
   wedge (see the template note for #2742). `.md2-body` is the positioning context. */
.md2-body {
  position: relative;
}
.md2-fade-enter-active,
.md2-fade-leave-active {
  transition: opacity 0.15s ease;
}
.md2-fade-leave-active {
  position: absolute;
  inset-inline: 0;
  top: 0;
  pointer-events: none;
}
.md2-fade-enter-from,
.md2-fade-leave-to {
  opacity: 0;
}
@media (prefers-reduced-motion: reduce) {
  .md2-fade-enter-active,
  .md2-fade-leave-active {
    transition: none;
  }
}
.md2-write {
  position: relative;
}
.md2-write textarea {
  display: block;
  width: 100%;
  box-sizing: border-box;
  border: none;
  outline: none;
  /* Auto-grows to its content (height set in JS); no inner scrollbar. */
  resize: none;
  overflow: hidden;
  padding: 2px 0;
  background: transparent;
  color: var(--t-text1);
  /* Monospace so indentation (nested lists) lines up while editing. */
  font-family: ui-monospace, SFMono-Regular, 'JetBrains Mono', Menlo, Consolas, monospace;
  font-size: 13px;
  line-height: 1.55;
  min-height: calc(v-bind(minRows) * 1.55em);
}
.md2-write textarea::placeholder {
  color: var(--t-placeholder);
}

/* Floating selection toolbar */
.md2-bubble {
  position: fixed;
  z-index: 4100;
  transform: translate(-4px, -100%);
  display: flex;
  gap: 2px;
  padding: 4px;
  background: var(--t-pop);
  border-radius: 10px;
  box-shadow: var(--t-sh-pop);
}
.md2-bubble button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 28px;
  height: 28px;
  padding: 0 7px;
  border: none;
  border-radius: 7px;
  background: transparent;
  color: var(--t-text1);
  font-size: 13px;
  line-height: 1;
  cursor: pointer;
}
.md2-bubble button:hover {
  background: var(--t-hover);
}
.bubble-enter-active,
.bubble-leave-active {
  transition:
    opacity 0.12s ease,
    margin-top 0.12s ease;
}
.bubble-enter-from,
.bubble-leave-to {
  opacity: 0;
  margin-top: 4px;
}
.md2-bubble button.b {
  font-weight: 700;
}
.md2-bubble button.i {
  font-style: italic;
}
.md2-bubble button.s {
  text-decoration: line-through;
}

.md2-mentions {
  /* Fixed (viewport) positioning so the popup escapes the editor's clipping —
     inside the boxed composer an absolute popup got cropped by the rounded box.
     `top`/`left` come from the inline :style (computed under the caret line). */
  position: fixed;
  left: 0;
  top: 0;
  z-index: 4100;
  margin: 0;
  padding: 6px;
  list-style: none;
  min-width: 220px;
  max-width: min(360px, 90vw);
  max-height: 264px;
  overflow-y: auto;
  box-sizing: border-box;
  background: var(--t-pop);
  border-radius: var(--t-r-card);
  box-shadow: var(--t-sh-pop);
  --t-surface: var(--t-pop);
}
.md2-mention-item {
  display: flex;
  align-items: center;
  gap: 10px;
  min-height: 32px;
  padding: 0 8px 0 8px;
  border-radius: 7px;
  font-size: 13.5px;
  color: var(--t-text1);
  cursor: pointer;
}
.md2-mention-item:hover,
.md2-mention-item.active {
  /* Neutral grey highlight (matches the on-card assignee picker), not the accent. */
  background: var(--t-hover);
}
.md2-mention-av {
  flex: none;
  width: 22px;
  height: 22px;
  border-radius: 50%;
  background: var(--t-accent-grad);
  color: var(--t-on-primary);
  font-size: 9px;
  font-weight: 600;
}
.md2-mention-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
/* The login that will be inserted — muted and never squeezed out, since it is
   the part the name alone doesn't tell you. */
.md2-mention-hint {
  flex: none;
  margin-left: auto;
  padding-left: 8px;
  color: var(--t-text3);
  font-size: 12px;
}
/* Suggestion popup enter/leave: soft fade + a short rise. Position is driven by
   `top`/`left` (inline :style), so the animation is free to use `transform`
   without fighting the fixed positioning / flip-up logic. */
.md2-sug-enter-active {
  transition:
    opacity 0.12s ease-out,
    transform 0.12s ease-out;
}
.md2-sug-leave-active {
  transition:
    opacity 0.09s ease-in,
    transform 0.09s ease-in;
}
.md2-sug-enter-from,
.md2-sug-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}
@media (prefers-reduced-motion: reduce) {
  .md2-sug-enter-active,
  .md2-sug-leave-active {
    transition: none;
  }
  .md2-sug-enter-from,
  .md2-sug-leave-to {
    transform: none;
  }
}
.md2-cmds {
  min-width: 280px;
}
.md2-cmd-item {
  gap: 10px;
}
.md2-cmd-key {
  flex: none;
  font-family: ui-monospace, SFMono-Regular, 'JetBrains Mono', Menlo, Consolas, monospace;
  font-size: 12px;
  color: var(--t-text1);
}
.md2-cmd-desc {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 12px;
  color: var(--t-text3);
}
.md2-cmd-tag {
  /* Neutral, flat — custom commands are documentation, not an accent action. */
  margin-left: auto;
  flex: none;
  padding: 1px 7px;
  border-radius: 999px;
  background: var(--t-pill);
  font-size: 10px;
  color: var(--t-text3);
}
.md2-mention-sep {
  padding: 8px 10px 4px;
  font-size: 11.5px;
  font-weight: 600;
  letter-spacing: 0.05em;
  text-transform: uppercase;
  color: var(--t-text3);
  cursor: default;
}
.md2-preview {
  padding: 2px 0;
  min-height: calc(v-bind(minRows) * 1.55em);
}

/* ── split mode (fullscreen editor) ──
   Text left, live preview right, formatting bar across the bottom. The whole
   component takes the height its host gives it; each pane scrolls on its own, so
   a short text no longer leaves a dead band under the toolbar and a long one
   doesn't push the toolbar off the modal. */
.md2-splitmode {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
}
.md2-splitmode .md2-body {
  flex: 1;
  min-height: 0;
  display: grid;
  grid-template-columns: 1fr 1fr;
  grid-template-rows: minmax(0, 1fr) auto auto;
  column-gap: 16px;
}
.md2-splitmode .md2-write {
  grid-column: 1;
  grid-row: 1;
  display: flex;
  min-height: 0;
}
/* Fills the pane instead of auto-growing (autoGrow() is a no-op here), so the
   textarea is what scrolls. */
.md2-splitmode .md2-write textarea {
  flex: 1;
  height: 100%;
  min-height: 0;
  overflow: auto;
}
.md2-splitmode .md2-preview-side {
  grid-column: 2;
  grid-row: 1;
  min-height: 0;
  overflow: auto;
  padding-left: 16px;
  border-left: 1px solid var(--t-border);
}
.md2-splitmode .md2-toolbar {
  grid-column: 1 / -1;
  grid-row: 2;
}
.md2-splitmode .md2-attach {
  grid-column: 1 / -1;
  grid-row: 3;
}
/* Narrow screens: one column — the preview goes under the text rather than
   squeezing both into unreadable strips. */
@media (max-width: 720px) {
  .md2-splitmode .md2-body {
    grid-template-columns: 1fr;
    grid-template-rows: minmax(0, 1fr) minmax(0, 1fr) auto auto;
  }
  .md2-splitmode .md2-preview-side {
    grid-column: 1;
    grid-row: 2;
    padding-left: 0;
    padding-top: 10px;
    border-left: none;
    border-top: 1px solid var(--t-border);
  }
  .md2-splitmode .md2-toolbar {
    grid-row: 3;
  }
  .md2-splitmode .md2-attach {
    grid-row: 4;
  }
}

/* ── boxed composer (comments) ──
   Mirrors the Naive n-input skin (background / hover border / text colours) so
   the composer reads as one more field of the modal instead of a floating box. */
.md2-boxed .md2-body {
  border: 1px solid var(--t-border);
  border-radius: 10px;
  background: var(--t-input-bg);
  padding: 8px 10px;
  transition: border-color 0.15s ease;
}
.md2-boxed .md2-body:hover:not(:focus-within) {
  border-color: color-mix(in srgb, var(--t-primary) 45%, var(--t-border));
}
.md2-boxed .md2-body:focus-within {
  border-color: var(--t-primary);
}
.md2-boxed .md2-write textarea {
  color: var(--t-text2);
}
.md2-toolbar {
  display: flex;
  align-items: center;
  gap: 2px;
  margin-top: 6px;
  padding-top: 6px;
  border-top: 1px solid var(--t-border);
}
/* Seamless variant: the editor has no box, so the bar wraps rather than forcing
   a horizontal scroll in the narrow description column. */
.md2-format {
  flex-wrap: wrap;
}
/* Compact composer: the folded-away format/insert buttons, shown above the bar when
   its menu button is toggled. Wraps so it fits any composer width. */
.md2-fmtbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 2px;
  margin-top: 6px;
  padding: 4px;
  border: 1px solid var(--t-border);
  border-radius: 8px;
  background: var(--t-input-bg);
}
.md2-tbtn.active {
  background: var(--t-hover);
  color: var(--t-text1);
}
/* Drop target feedback: without it a dragged file gives no hint that the editor
   will take it (the textarea has no box of its own to change). */
.md2-write.dragging::after {
  content: '';
  position: absolute;
  inset: -4px;
  border: 1px dashed var(--t-primary);
  border-radius: 8px;
  background: color-mix(in srgb, var(--t-primary) 6%, transparent);
  pointer-events: none;
}
/* Task files, listed for linking into the text. */
.md2-attach {
  margin: 6px 0 0;
  padding: 4px;
  list-style: none;
  max-height: 200px;
  overflow-y: auto;
  background: var(--t-input-bg);
  border: 1px solid var(--t-border);
  border-radius: 8px;
}
.md2-attach-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 5px 8px;
  border-radius: 6px;
  font-size: 13px;
  color: var(--t-text1);
  cursor: pointer;
}
.md2-attach-item:hover {
  background: var(--t-hover);
}
.md2-attach-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.md2-attach-empty {
  padding: 6px 8px;
  font-size: 12px;
  color: var(--t-text3);
}
.md2-tbtn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 26px;
  height: 26px;
  padding: 0 5px;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: var(--t-text2);
  font-size: 13px;
  line-height: 1;
  cursor: pointer;
  transition:
    background 0.12s ease,
    color 0.12s ease;
}
.md2-tbtn:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.md2-tbtn.busy {
  opacity: 0.5;
  pointer-events: none;
}
.md2-tbtn.b {
  font-weight: 700;
}
.md2-tbtn.i {
  font-style: italic;
}
.md2-tbtn.s {
  text-decoration: line-through;
}
.md2-tsep {
  width: 1px;
  align-self: stretch;
  margin: 2px 4px;
  background: var(--t-border);
}
/* The composer's row wrapper — no box anywhere else. */
.md2-main {
  display: contents;
}

/* ── v2 bar (variant v2; the composer's «Aa» strip) — StagePage mockup ── */
.md2-bar {
  display: flex;
  align-items: center;
  gap: 2px;
  min-width: 0;
  padding: 3px;
  margin-bottom: 8px;
  box-sizing: border-box;
  border-radius: 9px;
  background: var(--t-input-bg);
}
.md2-seg {
  flex: none;
  display: flex;
  gap: 2px;
}
.md2-seg-b {
  height: 26px;
  padding: 0 10px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--t-text2);
  font: inherit;
  font-size: 12.5px;
  white-space: nowrap;
  cursor: pointer;
}
.md2-seg-b.on {
  background: var(--t-card);
  box-shadow: var(--t-sh-card);
  color: var(--t-text1);
  font-weight: 600;
}
.md2-seg-b:not(.on):hover {
  color: var(--t-text1);
}
.md2-tools {
  display: flex;
  align-items: center;
  gap: 2px;
  min-width: 0;
  color: var(--t-text3);
}
.md2-ti {
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 26px;
  padding: 0;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--t-text3);
  font: inherit;
  font-size: 13px;
  font-weight: 800;
  line-height: 1;
  cursor: pointer;
  transition:
    background 0.12s ease,
    color 0.12s ease;
}
.md2-ti:hover,
.md2-ti.on {
  background: var(--t-hover);
  color: var(--t-text1);
}
.md2-ti.i {
  font-style: italic;
  font-weight: 600;
}
.md2-ti.s {
  text-decoration: line-through;
}
.md2-ti.busy {
  opacity: 0.5;
  pointer-events: none;
}
.md2-bar .md2-tsep {
  align-self: center;
  height: 16px;
  margin: 0 3px;
  background: color-mix(in srgb, var(--t-text3) 30%, transparent);
}

/* ── v2 field: the input surface with the primary ring, monospace 13/22 ── */
.md2-v2 .md2-write textarea {
  padding: 12px 14px;
  border-radius: 10px;
  background: var(--t-input-bg);
  box-shadow: 0 0 0 2px color-mix(in srgb, var(--t-primary) 55%, transparent);
  color: var(--t-text2);
  line-height: 22px;
  min-height: calc(v-bind(minRows) * 22px + 24px);
  transition: box-shadow 0.12s ease;
}
.md2-v2 .md2-write textarea:focus {
  box-shadow: 0 0 0 2px var(--t-primary);
}
.md2-v2 .md2-preview {
  padding: 12px 14px;
  border-radius: 10px;
  background: color-mix(in srgb, var(--t-input-bg) 55%, transparent);
  min-height: calc(v-bind(minRows) * 22px + 24px);
  box-sizing: border-box;
}
.md2-v2 .md2-write.dragging::after {
  inset: 0;
  border-radius: 10px;
}

/* ── v2 footer ── */
.md2-foot {
  display: flex;
  align-items: center;
  gap: 8px;
  min-height: 30px;
  margin-top: 10px;
  font-size: 12px;
  color: var(--t-text3);
}
.md2-hints {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  white-space: nowrap;
}
.md2-hints > span + .kbd {
  margin-left: 4px;
}
.md2-confirm-q {
  font-size: 13px;
  font-weight: 600;
  color: var(--t-text1);
}
.md2-fbtn {
  flex: none;
  display: inline-flex;
  align-items: center;
  gap: 8px;
  height: 30px;
  padding: 0 5px 0 10px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: var(--t-text2);
  font: inherit;
  font-size: 13px;
  white-space: nowrap;
  cursor: pointer;
}
.md2-fbtn:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.md2-fbtn.primary,
.md2-fbtn.danger {
  padding-left: 12px;
  color: #fff;
  font-weight: 600;
}
.md2-fbtn.primary {
  background: var(--t-accent-grad);
}
.md2-fbtn.danger {
  background: linear-gradient(
    to top right,
    color-mix(in srgb, #e0533d 86%, #000),
    #e0533d 50%,
    color-mix(in srgb, #e0533d 86%, #fff)
  );
}
.md2-fbtn.primary:hover {
  box-shadow: 0 4px 14px color-mix(in srgb, var(--t-primary) 40%, transparent);
}
.md2-fbtn.danger:hover {
  box-shadow: 0 4px 14px color-mix(in srgb, #e0533d 40%, transparent);
}
.md2-fbtn.primary .kbd,
.md2-fbtn.danger .kbd {
  background: rgba(255, 255, 255, 0.22);
  color: #fff;
  box-shadow: none;
}
.md2-fbtn.busy {
  opacity: 0.8;
  cursor: progress;
}

/* ── composer (TaskModal mockup L80–87): one 46px row on the input surface ── */
.md2-composer .md2-main {
  display: flex;
  align-items: flex-end;
  gap: 10px;
  min-height: 46px;
  padding: 0 8px 0 12px;
  box-sizing: border-box;
  border-radius: 13px;
  background: var(--t-input-bg);
  transition: box-shadow 0.12s ease;
}
.md2-composer .md2-main:focus-within {
  box-shadow: 0 0 0 2px color-mix(in srgb, var(--t-primary) 55%, transparent);
}
.md2-composer .md2-body {
  flex: 1;
  min-width: 0;
  align-self: stretch;
  display: flex;
  flex-direction: column;
  justify-content: center;
  padding: 12px 0;
}
.md2-composer .md2-write textarea {
  padding: 0;
  font-family: inherit;
  font-size: 14px;
  line-height: 22px;
  min-height: 22px;
  /* ~8 lines, then the field scrolls rather than eating the column. */
  max-height: 176px;
  overflow-y: auto;
  color: var(--t-text1);
}
.md2-composer .md2-preview {
  min-height: 22px;
  max-height: 240px;
  overflow-y: auto;
  font-size: 14px;
}
.md2-cava {
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  margin-bottom: 11px;
  border-radius: 50%;
  background: var(--t-accent-grad);
  color: var(--t-on-primary);
  font-size: 9.5px;
  font-weight: 600;
}
.md2-cbtn {
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 32px;
  height: 32px;
  margin-bottom: 7px;
  padding: 0 6px;
  box-sizing: border-box;
  border: 0;
  border-radius: 9px;
  background: transparent;
  color: var(--t-text2);
  font: inherit;
  font-size: 13px;
  font-weight: 650;
  cursor: pointer;
  transition:
    background 0.12s ease,
    color 0.12s ease;
}
.md2-cbtn:hover,
.md2-cbtn.on {
  background: var(--t-hover);
  color: var(--t-text1);
}
.md2-cbtn.on {
  color: var(--t-primary);
}
.md2-cbtn.busy {
  opacity: 0.5;
  pointer-events: none;
}
.md2-csend {
  flex: none;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 32px;
  margin-bottom: 7px;
  padding: 0 6px 0 10px;
  box-sizing: border-box;
  border: 0;
  border-radius: 9px;
  background: var(--t-accent-grad);
  color: #fff;
  font: inherit;
  font-size: 13px;
  font-weight: 600;
  white-space: nowrap;
  cursor: pointer;
  transition: box-shadow 0.12s ease;
}
.md2-csend.icon {
  width: 32px;
  padding: 0;
  justify-content: center;
}
.md2-csend:hover {
  box-shadow: 0 4px 14px color-mix(in srgb, var(--t-primary) 40%, transparent);
}
.md2-csend.busy {
  opacity: 0.8;
  cursor: progress;
}
.md2-ckbd {
  display: inline-flex;
  align-items: center;
  height: 20px;
  padding: 0 5px;
  border-radius: 5px;
  background: rgba(255, 255, 255, 0.2);
  font-size: 11px;
  font-weight: 600;
}
.md2-send {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 32px;
  height: 28px;
  margin-left: 4px;
  padding: 0 8px;
  border: none;
  border-radius: 8px;
  background: var(--t-accent-grad);
  color: var(--t-on-primary);
  cursor: pointer;
  transition: filter 0.12s ease;
}
.md2-send:hover {
  filter: brightness(1.06);
}
.md2-send.busy {
  cursor: default;
  filter: none;
}
</style>
