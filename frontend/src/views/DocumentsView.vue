<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { onBeforeRouteLeave, useRoute, useRouter } from 'vue-router'
import {
  NAlert,
  NButton,
  NDropdown,
  NIcon,
  NInput,
  NPopconfirm,
  NSpin,
  NText,
  useMessage,
} from 'naive-ui'
import {
  AddOutline,
  ArrowBackOutline,
  BookmarkOutline,
  ChatbubbleEllipsesOutline,
  CloudUploadOutline,
  DocumentsOutline,
  DownloadOutline,
  GridOutline,
  LinkOutline,
  TimeOutline,
} from '@vicons/ionicons5'
import { documents as docsApi } from '@/api'
import EmptyState from '@/components/EmptyState.vue'
import DocEditor from '@/components/documents/DocEditor.vue'
import DocComments from '@/components/documents/DocComments.vue'
import DocHistory from '@/components/documents/DocHistory.vue'
import DocLinks from '@/components/documents/DocLinks.vue'
import DocTemplates from '@/components/documents/DocTemplates.vue'
import { fileToTemplate } from '@/utils/docImport'
import {
  EXPORT_LABELS,
  downloadBlob,
  exportFileName,
  importAccept,
  importOfficeFile,
  isOfficeFile,
} from '@/utils/docOffice'
import { builtinCards, builtinContent } from '@/utils/docTemplates'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useDocAutosave } from '@/composables/useDocAutosave'
import { useDocComments } from '@/composables/useDocComments'
import { useDocLinks } from '@/composables/useDocLinks'
import { useDocPresence } from '@/composables/useDocPresence'
import { useDocVersions } from '@/composables/useDocVersions'
import { toDocJSON } from '@/utils/docSchema'
import { blockNodeById, quoteFromBlock } from '@/utils/docComments'

const message = useMessage()
const wsStore = useWorkspacesStore()
const route = useRoute()
const router = useRouter()

const list = ref([])
const selected = ref(null)
const content = ref(null)
const title = ref('')
const loading = ref(false)
// Breadcrumb trail of containers the user drilled into. The grid replaced the
// tree (review of #2726), so nesting needs a way to be walked; a tile always
// opens the document itself, and the trail is how you get back out.
const trail = ref([])

const parentId = computed(() =>
  trail.value.length ? trail.value[trail.value.length - 1].id : null,
)
const tiles = computed(() => list.value.filter((d) => (d.parent_id || null) === parentId.value))
const childCount = computed(() =>
  selected.value ? list.value.filter((d) => d.parent_id === selected.value.id).length : 0,
)
const children = computed(() =>
  selected.value ? list.value.filter((d) => d.parent_id === selected.value.id) : [],
)

// The version the editor is based on. Sent with every save so a document edited
// somewhere else answers 409 instead of being overwritten.
const version = ref(null)

const {
  saving,
  dirty,
  conflict,
  error: saveError,
  schedule: scheduleSave,
  flush: flushSave,
  cancel: cancelSave,
  resolveConflict,
} = useDocAutosave(async (json) => {
  const res = await docsApi.updateContent(selected.value.id, json, version.value)
  version.value = res.data?.updated_at || version.value
  applyPreview(selected.value.id, res.data)
  // Every save writes the journal (#2731), so an open history panel goes stale
  // the moment its owner keeps typing — and stale here is worse than merely old:
  // the comparison is made against the newest entry, so the panel would answer
  // "версии совпадают" about a document that has since moved on. Only while the
  // panel is open; a document read without it still costs no journal request.
  if (historyOpen.value) await versions.load()
  return res.data
})

// Presence and per-block locks for the open document (#2729). A separate socket
// from the app-wide realtime one: that hub is workspace-scoped and read-only.
const {
  viewers,
  foreignLocks,
  userId: myUserId,
  commentsNudge,
  contentNudge,
  linksNudge,
  open: openRoom,
  close: closeRoom,
  acquire: claimBlock,
  release: releaseBlock,
} = useDocPresence()

// Block-anchored annotations (#2730). They live beside the document rather than
// inside it: nothing here writes to the content, so a remark can be made on a
// block someone else is editing, and an imported document is annotatable as soon
// as it has block ids.
const comments = useDocComments()
const commentsOpen = ref(true)

// A colleague's comment arrives as a payload-free nudge on the document socket;
// the panel refetches instead of being handed a delta, so a nudge lost to a
// reconnect costs one stale panel and nothing more.
watch(commentsNudge, () => comments.load())

// Version journal, snapshots and rollback (#2731).
const versions = useDocVersions()
const historyOpen = ref(false)

// Task links and approval protocols (#2732). Loaded lazily like the journal: a
// document is read far more often than it is linked or signed, and every reader
// paying two requests for a panel they never open would make the section slower
// for the common case.
const links = useDocLinks()
const linksOpen = ref(false)
watch(linksNudge, () => links.load())

// A rollback replaces the body under everyone reading it. Reloading is not
// optional here: the editor is holding both the old tree and the old updated_at,
// so the next keystroke would either resurrect the reverted text or collide with
// the restore. An unsaved local edit is flushed first — it becomes the version
// the rollback pushed aside, which is exactly where it belongs.
// The nudge also comes back to whoever pressed "восстановить"; that client has
// already reloaded, so it skips the round trip and the announcement rather than
// telling the user about their own click.
let restoredAt = 0
watch(contentNudge, async () => {
  if (!selected.value?.id || Date.now() - restoredAt < 3000) return
  await flushSave()
  await reload()
  message.info('Документ восстановлен из истории')
})

// Other people in this document. The server already collapses one person's tabs
// into a single viewer, and our own id comes from the socket's welcome frame —
// no auth store needed to know who "me" is here.
const others = computed(() => viewers.value.filter((v) => v.user_id !== myUserId.value))

function initials(name) {
  const parts = (name || '').trim().split(/\s+/).filter(Boolean)
  if (!parts.length) return '?'
  return (parts[0][0] + (parts[1]?.[0] || '')).toUpperCase()
}

// Refusing a keystroke silently reads as a broken editor, so say who has the
// block. Throttled by naive-ui itself (identical messages stack), and the
// message is the only feedback there is — the caret simply stops responding.
let blockedAt = 0
function onBlocked(held) {
  const now = Date.now()
  if (now - blockedAt < 2000) return
  blockedAt = now
  message.warning(`Блок редактирует ${held?.name || 'другой участник'}`)
}

function applyPreview(id, data) {
  const row = list.value.find((d) => d.id === id)
  if (!row || !data) return
  if (typeof data.preview === 'string') row.preview = data.preview
  if (data.updated_at) row.updated_at = data.updated_at
}

function fmtDate(v) {
  return v ? new Date(v).toLocaleDateString('ru-RU', { day: 'numeric', month: 'short' }) : ''
}

async function loadList() {
  if (!wsStore.currentId) return
  try {
    const res = await docsApi.list(wsStore.currentId)
    list.value = res.data || []
  } catch (e) {
    message.error(e.message)
  }
}

async function open(doc) {
  if (!doc?.id) return
  await flushSave()
  loading.value = true
  try {
    const res = await docsApi.get(doc.id)
    selected.value = res.data
    title.value = res.data.title || ''
    content.value = toDocJSON(res.data.content)
    version.value = res.data.updated_at
    resolveConflict(res.data.updated_at)
    openRoom(res.data.id)
    await comments.open(res.data.id, content.value)
    // The journal is loaded only while the panel is open: it is a side question
    // about the document, and every opened document paying for it would make the
    // section slower for the readers who never ask it.
    if (historyOpen.value) await versions.open(res.data.id)
    if (linksOpen.value) await links.open(res.data.id)
    const param = res.data.slug || res.data.id
    if (param && route.params.slug !== param) router.replace(`/documents/${param}`)
  } catch (e) {
    message.error(e.message)
  } finally {
    loading.value = false
  }
}

async function backToGrid() {
  await flushSave()
  // Leaving the document frees whatever block we held right away, instead of
  // making everyone else wait out the lock TTL.
  closeRoom()
  comments.close()
  versions.close()
  links.close()
  selected.value = null
  content.value = null
  if (route.params.slug) router.replace('/documents')
}

// The history panel loads lazily: opening it is the moment the journal is
// wanted, and a document read without it never fetches one.
async function toggleHistory() {
  historyOpen.value = !historyOpen.value
  if (historyOpen.value && selected.value?.id) await versions.open(selected.value.id)
  else if (!historyOpen.value) versions.close()
}

// Same lazy rule for links and protocols.
async function toggleLinks() {
  linksOpen.value = !linksOpen.value
  if (linksOpen.value && selected.value?.id) await links.open(selected.value.id)
  else if (!linksOpen.value) links.close()
}

// The block a new link would be pinned to. It follows the annotation panel's
// selection rather than carrying a second one: "выбранный блок" has to mean the
// same thing in both panels, or the link lands on a paragraph the user is not
// looking at. No selection links the document as a whole, which is the common case.
const linkAnchorId = computed(() => comments.activeBlockId.value || '')
const linkAnchorQuote = computed(() => {
  const node = blockNodeById(content.value, linkAnchorId.value)
  return node ? quoteFromBlock(node) : ''
})

async function onLink(payload) {
  try {
    await links.link(payload)
    message.success(payload.blockId ? 'Блок связан с задачей' : 'Задача связана')
  } catch (e) {
    message.error(e.message)
  }
}

async function onUnlink(linkId) {
  try {
    await links.unlink(linkId)
  } catch (e) {
    message.error(e.message)
  }
}

async function onRaiseApproval(payload) {
  try {
    await links.raise(payload)
    // Raising a route pins a manual snapshot, so an open journal is now a
    // version behind — same reasoning as the autosave path above.
    if (historyOpen.value) await versions.load()
    message.success('Документ отправлен на согласование')
  } catch (e) {
    message.error(e.message)
  }
}

async function onDecide({ id, decision, comment }) {
  try {
    await links.decide(id, { decision, comment })
    message.success(decision === 'approved' ? 'Подписано' : 'Отклонено')
  } catch (e) {
    message.error(e.message)
  }
}

async function onCancelApproval(id) {
  try {
    await links.cancel(id)
    message.info('Маршрут отозван')
  } catch (e) {
    message.error(e.message)
  }
}

// Opening the task leaves the documents section entirely, so unsaved text is
// flushed first — the board route unmounts this view and the debounce with it.
async function onOpenTask(link) {
  if (!link?.task_board_id) return
  await flushSave()
  router.push(`/board/${link.task_board_id}?task=${link.task_id}`)
}

async function onSnapshot(label) {
  try {
    await versions.snapshot(label)
    message.success('Версия сохранена')
  } catch (e) {
    message.error(e.message)
  }
}

async function onRestore(versionId) {
  if (!selected.value?.id) return
  try {
    // Anything still in the debounce is written first, so the rollback pushes a
    // complete state into the journal rather than one that lost the last
    // sentence the user typed.
    await flushSave()
    restoredAt = Date.now()
    const doc = await versions.restore(versionId)
    if (doc) {
      selected.value = doc
      content.value = toDocJSON(doc.content)
      version.value = doc.updated_at
      resolveConflict(doc.updated_at)
      comments.setDoc(content.value)
      applyPreview(doc.id, doc)
    }
    message.success('Документ восстановлен')
  } catch (e) {
    message.error(e.message)
  }
}

// Drill into a container from the editor: the trail lets the grid show its
// children, and the document itself stays reachable through the crumb.
async function drillInto(doc) {
  await backToGrid()
  trail.value = [...trail.value, { id: doc.id, title: doc.title }]
}

function crumbTo(index) {
  trail.value = index < 0 ? [] : trail.value.slice(0, index + 1)
}

async function reload() {
  if (!selected.value?.id) return
  const doc = { id: selected.value.id }
  resolveConflict(null)
  await open(doc)
  await loadList()
  if (historyOpen.value) await versions.load()
  if (linksOpen.value) await links.load()
}

// Resolves the :slug deep link. Slugs are unique per workspace, not globally,
// so the current workspace is tried first and the user's others after — and the
// resolver's workspace_id then switches the app's scope. Without that switch the
// document opens while the rest of the UI stays pointed elsewhere (#2721).
async function resolveSlug(slug) {
  const ids = [wsStore.currentId, ...wsStore.list.map((w) => w.id)].filter(Boolean)
  for (const wsId of new Set(ids)) {
    try {
      const res = await docsApi.bySlug(wsId, slug)
      if (res.data?.workspace_id && res.data.workspace_id !== wsStore.currentId) {
        await wsStore.selectWorkspace(res.data.workspace_id)
        await loadList()
      }
      selected.value = res.data
      title.value = res.data.title || ''
      content.value = toDocJSON(res.data.content)
      version.value = res.data.updated_at
      openRoom(res.data.id)
      await comments.open(res.data.id, content.value)
      return true
    } catch {
      // 404 in this workspace — try the next one.
    }
  }
  return false
}

async function create() {
  if (!wsStore.currentId) return
  try {
    const res = await docsApi.create(wsStore.currentId, {
      title: 'Без названия',
      parent_id: parentId.value,
    })
    await loadList()
    await open(res.data)
  } catch (e) {
    message.error(e.message)
  }
}

// Office import/export (#2733). The sidecar is optional, so the section asks
// once whether it is deployed and hides the office half of the picker when it
// is not — offering an action that fails after the file dialog is worse than
// not offering it.
const converter = ref({ available: false, export_formats: ['html'] })
const importInput = ref(null)
const importing = ref(false)
const exporting = ref('')

async function loadConverter() {
  try {
    const res = await docsApi.converterStatus()
    converter.value = res.data || { available: false }
  } catch {
    // A failure here must not break the section: it only decides whether one
    // button is shown.
    converter.value = { available: false }
  }
}

// html is always offered — it is rendered by the backend itself and needs no
// sidecar, so the cheapest export never depends on the heaviest dependency.
const exportOptions = computed(() => {
  const formats = converter.value.export_formats?.length ? converter.value.export_formats : ['html']
  return formats.map((f) => ({ key: f, label: EXPORT_LABELS[f] || f.toUpperCase() }))
})

const importHint = computed(() =>
  converter.value.available
    ? 'Импортировать документ (docx, odt, rtf, md, json)'
    : `Импортировать документ (md, json)\u2009— ${converter.value.reason || 'конвертация офисных форматов недоступна'}`,
)

function pickImport() {
  importInput.value?.click()
}

async function onImportPicked(e) {
  const file = e.target.files?.[0]
  // Cleared before anything can fail, so picking the same file twice in a row
  // still fires a change event.
  e.target.value = ''
  if (!file || !wsStore.currentId) return
  importing.value = true
  try {
    const doc = isOfficeFile(file.name) ? await importOffice(file) : await importLocal(file)
    await loadList()
    await open(doc)
  } catch (err) {
    message.error(err.message)
  } finally {
    importing.value = false
  }
}

async function importOffice(file) {
  if (!converter.value.available) {
    throw new Error(converter.value.reason || 'Конвертация офисных форматов недоступна')
  }
  const { document: doc, imagesDropped } = await importOfficeFile(
    docsApi,
    wsStore.currentId,
    file,
    { parentId: parentId.value },
  )
  // Dropped pictures are said out loud. A document that quietly lost half its
  // figures looks like a successful import until somebody scrolls.
  if (imagesDropped) {
    message.warning(`Документ импортирован, изображений пропущено: ${imagesDropped}`)
  } else {
    message.success('Документ импортирован')
  }
  return doc
}

// .md and .json are parsed in the browser (D9) and therefore work on an install
// with no converter at all — the same picker covers both paths.
async function importLocal(file) {
  const draft = await fileToTemplate(file)
  const res = await docsApi.create(wsStore.currentId, {
    title: draft.title || 'Импортированный документ',
    icon: draft.icon || '',
    parent_id: parentId.value,
  })
  await docsApi.updateContent(res.data.id, draft.content, res.data.updated_at)
  message.success('Документ импортирован')
  return res.data
}

async function exportAs(format) {
  if (!selected.value?.id) return
  exporting.value = format
  try {
    // The open editor may hold edits the debounce has not sent yet, and the
    // export is rendered from what the server has — without this flush a user
    // would export the document as it was a second ago.
    await flushSave()
    const res = await docsApi.exportFile(selected.value.id, format)
    downloadBlob(res.data, exportFileName(selected.value.title, format))
  } catch (err) {
    message.error(exportError(err))
  } finally {
    exporting.value = ''
  }
}

// An export failure arrives as a Blob, not as JSON, because responseType is
// blob for the whole request — so the interceptor's message is not usable and
// the reason has to be read back out of the body.
function exportError(err) {
  const status = err?.response?.status
  if (status === 503) return 'Сервис конвертации документов недоступен'
  if (status === 422) return 'Не удалось преобразовать документ'
  return err?.message || 'Не удалось выгрузить документ'
}

// Template gallery (#2734). Saved templates come from the workspace; the
// built-in starters are frontend constants and are appended, so an empty
// gallery still has something to start from.
const templatesOpen = ref(false)
const savedTemplates = ref([])
const tplLoading = ref(false)
const tplBusy = ref('')
const tplError = ref('')

const galleryCards = computed(() => [...savedTemplates.value, ...builtinCards()])

async function openTemplates() {
  templatesOpen.value = true
  tplError.value = ''
  if (!wsStore.currentId) return
  tplLoading.value = true
  try {
    const res = await docsApi.templates(wsStore.currentId)
    savedTemplates.value = res.data || []
  } catch (e) {
    tplError.value = e.message
  } finally {
    tplLoading.value = false
  }
}

// Creating from a template goes through the normal create endpoint with
// template_id, so slug, position and authorship stay on the one path that owns
// them. A built-in has no row to point at, so its body is written straight
// after the create — the document is empty for exactly that one call, which is
// also why the version journal records no "empty" baseline for it.
async function useTemplate(card) {
  if (!wsStore.currentId || tplBusy.value) return
  tplBusy.value = card.id
  tplError.value = ''
  try {
    const res = await docsApi.create(wsStore.currentId, {
      title: card.title,
      icon: card.icon || '',
      parent_id: parentId.value,
      template_id: card.builtin ? undefined : card.id,
    })
    let doc = res.data
    if (card.builtin) {
      const content = builtinContent(card.key)
      if (content) {
        const saved = await docsApi.updateContent(doc.id, content, doc.updated_at)
        doc = saved.data
      }
    }
    templatesOpen.value = false
    await loadList()
    await open(doc)
  } catch (e) {
    tplError.value = e.message
  } finally {
    tplBusy.value = ''
  }
}

// Saving the open document as a template sends only its id: copying the body
// through the browser would let a stale editor state become the template, and
// the server already has the saved document.
async function saveAsTemplate() {
  if (!selected.value?.id) return
  try {
    await flushSave()
    await docsApi.createTemplate(wsStore.currentId, { document_id: selected.value.id })
    message.success('Шаблон сохранён')
  } catch (e) {
    message.error(e.message)
  }
}

async function uploadTemplate(file) {
  tplError.value = ''
  try {
    const draft = await fileToTemplate(file)
    await docsApi.createTemplate(wsStore.currentId, draft)
    await openTemplates()
  } catch (e) {
    tplError.value = e.message
  }
}

async function removeTemplate(card) {
  try {
    await docsApi.removeTemplate(card.id)
    savedTemplates.value = savedTemplates.value.filter((t) => t.id !== card.id)
  } catch (e) {
    tplError.value = e.message
  }
}

async function createNested() {
  if (!selected.value?.id) return
  const parent = selected.value
  try {
    const res = await docsApi.create(wsStore.currentId, {
      title: 'Без названия',
      parent_id: parent.id,
    })
    await loadList()
    await open(res.data)
    trail.value = [...trail.value, { id: parent.id, title: parent.title }]
  } catch (e) {
    message.error(e.message)
  }
}

async function rename() {
  const t = title.value.trim()
  if (!t || !selected.value?.id || t === selected.value.title) return
  try {
    const res = await docsApi.update(selected.value.id, { title: t })
    // Title changes bump updated_at server-side, so the editor's version has to
    // follow — otherwise the next autosave hits a 409 against our own rename.
    selected.value = res.data
    version.value = res.data.updated_at
    resolveConflict(res.data.updated_at)
    await loadList()
  } catch (e) {
    message.error(e.message)
  }
}

async function remove() {
  if (!selected.value?.id) return
  try {
    await docsApi.remove(selected.value.id, childCount.value > 0)
    cancelSave()
    closeRoom()
    comments.close()
    versions.close()
    links.close()
    selected.value = null
    content.value = null
    router.replace('/documents')
    await loadList()
  } catch (e) {
    message.error(e.message)
  }
}

async function uploadImage(file) {
  const fd = new FormData()
  fd.append('file', file)
  const res = await docsApi.uploadAsset(selected.value.id, fd)
  return res.data?.url || ''
}

function onEditorChange(json) {
  content.value = json
  // The panel matches anchors against the live tree, so a thread whose block was
  // just deleted moves to "Блок удалён" straight away instead of after a reload.
  comments.setDoc(json)
  scheduleSave(json)
}

// The block the next comment will be filed against, together with the quote the
// editor captured for it. Kept here rather than in the composable: it is the
// state of a half-written remark, and it dies with the panel, not with the
// document.
const pendingBlockId = ref('')
const pendingQuote = ref('')

// The gutter's "обсудить блок" button. It only arms the draft box — a remark
// needs its text before it can be filed, and filing an empty one to be edited
// later is how a review fills up with placeholder threads.
function onAnnotate({ blockId, quote }) {
  commentsOpen.value = true
  pendingBlockId.value = blockId
  pendingQuote.value = quote
  comments.activeBlockId.value = blockId
}

function clearAnchor() {
  pendingBlockId.value = ''
  pendingQuote.value = ''
}

async function onCommentAdd(body) {
  try {
    await comments.add({
      blockId: pendingBlockId.value,
      body,
      quote: pendingQuote.value,
    })
    clearAnchor()
  } catch (e) {
    message.error(e.message)
  }
}

async function onCommentReply({ id, body }) {
  try {
    await comments.reply(id, body)
  } catch (e) {
    message.error(e.message)
  }
}

async function onCommentEdit({ id, body }) {
  try {
    await comments.edit(id, body)
  } catch (e) {
    message.error(e.message)
  }
}

async function onCommentResolve({ id, resolved }) {
  try {
    await comments.resolve(id, resolved)
  } catch (e) {
    message.error(e.message)
  }
}

async function onCommentRemove(id) {
  try {
    await comments.remove(id)
  } catch (e) {
    message.error(e.message)
  }
}

// Leaving the editor gives the block back: someone whose caret is parked in a
// paragraph in an unfocused tab is not editing it.
function onEditorBlur() {
  releaseBlock()
  flushSave()
}

// The tab closing mid-debounce is the one case a promise cannot save: the
// handler warns instead, and flush() still gets its chance in the common case
// where the browser keeps the page alive long enough.
function onBeforeUnload(e) {
  if (!dirty.value) return
  flushSave()
  e.preventDefault()
  e.returnValue = ''
}

onMounted(async () => {
  window.addEventListener('beforeunload', onBeforeUnload)
  loadConverter()
  await loadList()
  if (route.params.slug) {
    const ok = await resolveSlug(route.params.slug)
    if (!ok) message.error('Документ не найден')
  }
})

onBeforeUnmount(() => {
  window.removeEventListener('beforeunload', onBeforeUnload)
  flushSave()
  closeRoom()
})

onBeforeRouteLeave(async () => {
  await flushSave()
})

watch(
  () => wsStore.currentId,
  () => {
    cancelSave()
    closeRoom()
    versions.close()
    links.close()
    selected.value = null
    content.value = null
    trail.value = []
    loadList()
  },
)
</script>

<template>
  <div class="docs">
    <!-- Grid: documents as tiles with a preview, no editing surface. -->
    <template v-if="!selected">
      <div class="head">
        <div class="crumbs">
          <n-button text size="small" @click="crumbTo(-1)">Документы</n-button>
          <template v-for="(c, i) in trail" :key="c.id">
            <span class="sep">/</span>
            <n-button text size="small" @click="crumbTo(i)">{{ c.title }}</n-button>
          </template>
        </div>
        <n-button
          size="small"
          data-testid="doc-import"
          :loading="importing"
          :title="importHint"
          @click="pickImport"
        >
          <template #icon><n-icon :component="CloudUploadOutline" /></template>
          Импорт
        </n-button>
        <input
          ref="importInput"
          type="file"
          class="hidden-file"
          :accept="importAccept()"
          @change="onImportPicked"
        />
        <n-button size="small" data-testid="doc-templates" @click="openTemplates">
          <template #icon><n-icon :component="GridOutline" /></template>
          Из шаблона
        </n-button>
        <n-button type="primary" size="small" @click="create">
          <template #icon><n-icon :component="AddOutline" /></template>
          Новый документ
        </n-button>
      </div>

      <div v-if="tiles.length" class="grid">
        <button v-for="d in tiles" :key="d.id" type="button" class="tile" @click="open(d)">
          <div class="tile-head">
            <span v-if="d.icon" class="doc-emoji">{{ d.icon }}</span>
            <n-icon v-else :component="DocumentsOutline" :size="16" />
            <span class="tile-title">{{ d.title || 'Без названия' }}</span>
          </div>
          <p class="tile-preview">
            {{ d.preview || 'Пустой документ' }}
          </p>
          <div class="tile-foot">
            <span>{{ fmtDate(d.updated_at) }}</span>
            <span v-if="list.filter((x) => x.parent_id === d.id).length">
              вложенных: {{ list.filter((x) => x.parent_id === d.id).length }}
            </span>
          </div>
        </button>
      </div>
      <empty-state v-else :icon="DocumentsOutline" text="Документов пока нет" size="small" />
    </template>

    <!-- Editor: title + working area, as asked in the review of #2726. -->
    <template v-else>
      <div class="head">
        <n-button quaternary size="small" @click="backToGrid">
          <template #icon><n-icon :component="ArrowBackOutline" /></template>
          К списку
        </n-button>
        <span class="status">
          <!-- Who else has this document open (#2729). Someone editing a block
               carries the accent ring; a reader is flat. -->
          <span v-if="others.length" class="viewers">
            <span
              v-for="v in others"
              :key="v.user_id"
              class="viewer"
              :class="{ editing: v.blocks.length }"
              :title="v.blocks.length ? `${v.name} — редактирует` : `${v.name} — смотрит`"
            >
              {{ initials(v.name) }}
            </span>
          </span>
          <n-button
            quaternary
            size="tiny"
            :title="commentsOpen ? 'Скрыть обсуждение' : 'Показать обсуждение'"
            @click="commentsOpen = !commentsOpen"
          >
            <template #icon><n-icon :component="ChatbubbleEllipsesOutline" /></template>
            {{ comments.openCount.value || '' }}
          </n-button>
          <n-button
            quaternary
            size="tiny"
            :title="historyOpen ? 'Скрыть историю' : 'История версий'"
            @click="toggleHistory"
          >
            <template #icon><n-icon :component="TimeOutline" /></template>
          </n-button>
          <n-button
            quaternary
            size="tiny"
            :title="linksOpen ? 'Скрыть связи' : 'Связи и согласование'"
            data-testid="doc-links-toggle"
            @click="toggleLinks"
          >
            <template #icon><n-icon :component="LinkOutline" /></template>
          </n-button>
          <n-button
            quaternary
            size="tiny"
            title="Сохранить как шаблон"
            data-testid="doc-save-template"
            @click="saveAsTemplate"
          >
            <template #icon><n-icon :component="BookmarkOutline" /></template>
          </n-button>
          <n-dropdown trigger="click" :options="exportOptions" @select="exportAs">
            <n-button
              quaternary
              size="tiny"
              title="Выгрузить документ"
              data-testid="doc-export"
              :loading="!!exporting"
            >
              <template #icon><n-icon :component="DownloadOutline" /></template>
            </n-button>
          </n-dropdown>
          <n-text v-if="saving" depth="3">Сохранение…</n-text>
          <n-text v-else-if="dirty" depth="3">Есть несохранённые правки</n-text>
          <n-text v-else depth="3">Все изменения сохранены</n-text>
        </span>
      </div>

      <n-alert v-if="conflict" type="warning" class="conflict">
        Документ изменён в другом месте — ваши последние правки не сохранены.
        <n-button text size="small" @click="reload">Загрузить актуальную версию</n-button>
      </n-alert>
      <n-alert v-else-if="saveError" type="error" class="conflict">
        {{ saveError }}
      </n-alert>

      <n-spin v-if="loading" size="small" />
      <template v-else>
        <n-input v-model:value="title" placeholder="Заголовок" class="title" @blur="rename" />
        <div class="work">
          <doc-editor
            :model-value="content"
            :upload-image="uploadImage"
            :locks="foreignLocks"
            :comments="comments.openCounts.value"
            class="editor"
            @change="onEditorChange"
            @block-focus="claimBlock"
            @blocked="onBlocked"
            @blur="onEditorBlur"
            @annotate="onAnnotate"
            @select-comments="comments.activeBlockId.value = $event"
          />
          <doc-comments
            v-if="commentsOpen"
            :groups="comments.groups.value"
            :active-block-id="comments.activeBlockId.value"
            :user-id="myUserId"
            :loading="comments.loading.value"
            :pending-quote="pendingQuote"
            :pending-block="!!pendingBlockId"
            @add="onCommentAdd"
            @reply="onCommentReply"
            @edit="onCommentEdit"
            @resolve="onCommentResolve"
            @remove="onCommentRemove"
            @clear-anchor="clearAnchor"
            @select="comments.activeBlockId.value = $event"
          />
          <doc-history
            v-if="historyOpen"
            :versions="versions.versions.value"
            :selected-id="versions.selectedId.value"
            :baseline="versions.baseline.value"
            :rows="versions.rows.value"
            :summary="versions.summary.value"
            :ready="versions.ready.value"
            :loading="versions.loading.value"
            :error="versions.error.value"
            @select="versions.select"
            @snapshot="onSnapshot"
            @restore="onRestore"
            @close="toggleHistory"
          />
          <doc-links
            v-if="linksOpen"
            :links="links.links.value"
            :approvals="links.approvals.value"
            :user-id="myUserId"
            :ws-id="wsStore.currentId"
            :can-raise="links.canRaise.value"
            :loading="links.loading.value"
            :error="links.error.value"
            :anchor-block-id="linkAnchorId"
            :anchor-quote="linkAnchorQuote"
            @link="onLink"
            @unlink="onUnlink"
            @raise="onRaiseApproval"
            @decide="onDecide"
            @cancel="onCancelApproval"
            @open-task="onOpenTask"
            @close="toggleLinks"
          />
        </div>
        <div v-if="children.length" class="nested">
          <n-text depth="3">Вложенные документы</n-text>
          <div class="grid small">
            <button v-for="d in children" :key="d.id" type="button" class="tile" @click="open(d)">
              <div class="tile-head">
                <span v-if="d.icon" class="doc-emoji">{{ d.icon }}</span>
                <n-icon v-else :component="DocumentsOutline" :size="16" />
                <span class="tile-title">{{ d.title || 'Без названия' }}</span>
              </div>
              <p class="tile-preview">{{ d.preview || 'Пустой документ' }}</p>
            </button>
          </div>
        </div>
        <div class="actions">
          <n-popconfirm
            :positive-button-props="{ type: 'error' }"
            positive-text="Удалить"
            @positive-click="remove"
          >
            <template #trigger>
              <n-button type="error" ghost>Удалить</n-button>
            </template>
            <template v-if="childCount">
              Удалить документ вместе с вложенными ({{ childCount }})?
            </template>
            <template v-else>Удалить документ?</template>
          </n-popconfirm>
          <span class="grow" />
          <n-button v-if="childCount" quaternary @click="drillInto(selected)">
            Показать вложенные ({{ childCount }})
          </n-button>
          <n-button @click="createNested">
            <template #icon><n-icon :component="AddOutline" /></template>
            Вложенный документ
          </n-button>
        </div>
      </template>
    </template>

    <!-- Outside the grid/editor branches: the gallery is reachable from the
         grid, but "сохранить как шаблон" refreshes it from the editor too. -->
    <doc-templates
      v-model:show="templatesOpen"
      :templates="galleryCards"
      :loading="tplLoading"
      :busy="tplBusy"
      :error="tplError"
      @use="useTemplate"
      @upload="uploadTemplate"
      @remove="removeTemplate"
    />
  </div>
</template>

<style scoped>
/* The picker is driven by the Импорт button; a visible file input would sit in
   the header with a browser-styled label nothing else on this screen matches. */
.hidden-file {
  display: none;
}

.docs {
  display: flex;
  flex-direction: column;
  gap: 12px;
  height: 100%;
  min-height: 0;
}
.head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}
.crumbs {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  overflow: hidden;
}
.sep {
  color: var(--t-text3);
}
.status {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
}
/* Presence avatars. Initials rather than photos: the roster is a live hint at
   the edge of the header, and loading an avatar per viewer would be a request
   burst every time someone opens the document. */
.viewers {
  display: flex;
  align-items: center;
}
.viewer {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  margin-left: -6px;
  border: 1px solid var(--t-surface);
  border-radius: 50%;
  background: var(--t-surface-alt);
  color: var(--t-text2);
  font-size: 10px;
  font-weight: 600;
}
.viewer:first-child {
  margin-left: 0;
}
/* Actively editing — the accent gradient, as everywhere else a non-neutral
   element appears. */
.viewer.editing {
  background: var(--t-accent-grad);
  color: var(--t-on-primary);
  border-color: transparent;
}
.grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
  gap: 12px;
  overflow-y: auto;
  align-content: start;
}
.grid.small {
  grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
  margin-top: 6px;
}
/* Tiles are neutral surfaces — no accent gradient: the grid is a list, and
   every card carrying the accent would leave nothing for it to emphasise. */
.tile {
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-height: 128px;
  padding: 12px;
  text-align: left;
  border: 1px solid var(--t-border);
  border-radius: 10px;
  background: var(--t-surface);
  color: var(--t-text1);
  cursor: pointer;
  transition:
    border-color 0.12s ease,
    background 0.12s ease;
}
.tile:hover {
  background: var(--t-hover);
  border-color: var(--t-primary);
}
.tile-head {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}
.tile-title {
  font-weight: 600;
  font-size: 14px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.tile-preview {
  flex: 1;
  margin: 0;
  font-size: 12px;
  line-height: 1.45;
  color: var(--t-text3);
  overflow: hidden;
  display: -webkit-box;
  -webkit-line-clamp: 4;
  line-clamp: 4;
  -webkit-box-orient: vertical;
}
.tile-foot {
  display: flex;
  justify-content: space-between;
  font-size: 11px;
  color: var(--t-text3);
}
.title :deep(input) {
  font-size: 18px;
  font-weight: 600;
}
/* Editor and annotation panel share the working area; the panel is fixed-width
   so the text column does not reflow every time a thread is opened. */
.work {
  display: flex;
  gap: 12px;
  flex: 1;
  min-height: 0;
}
.editor {
  flex: 1;
  min-width: 0;
  min-height: 0;
}
/* On a narrow screen the panel goes under the editor instead of squeezing the
   text into a column nobody can read. */
@media (max-width: 900px) {
  .work {
    flex-direction: column;
  }
}
.conflict {
  flex: none;
}
.nested {
  flex: none;
}
.actions {
  display: flex;
  align-items: center;
  gap: 8px;
}
.grow {
  flex: 1;
}
.doc-emoji {
  font-size: 14px;
  line-height: 1;
}
</style>
