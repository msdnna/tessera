<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { onBeforeRouteLeave, useRoute, useRouter } from 'vue-router'
import {
  NAlert,
  NButton,
  NDropdown,
  NIcon,
  NInput,
  NModal,
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
  EllipsisHorizontalOutline,
  GridOutline,
  LinkOutline,
  ListOutline,
  TimeOutline,
} from '@vicons/ionicons5'
import { documents as docsApi } from '@/api'
import EmptyState from '@/components/EmptyState.vue'
import DocEditor from '@/components/documents/DocEditor.vue'
import DocComments from '@/components/documents/DocComments.vue'
import DocHistory from '@/components/documents/DocHistory.vue'
import DocLinks from '@/components/documents/DocLinks.vue'
import DocTemplates from '@/components/documents/DocTemplates.vue'
import DocToc from '@/components/documents/DocToc.vue'
import { fileToTemplate } from '@/utils/docImport'
import {
  EXPORT_LABELS,
  downloadBlob,
  exportFileName,
  importAccept,
  importOfficeFile,
  needsConverter,
  isOfficeFile,
} from '@/utils/docOffice'
import { builtinCards, builtinContent } from '@/utils/docTemplates'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useThemeStore } from '@/stores/theme'
import { initials } from '@/utils/initials'
import { mergeRemoteBlocks } from '@/utils/docMerge'
import { linkGeometry } from '@/utils/docAnnotationLines'
import { userColor } from '@/utils/userColor'
import { hueGrad, onColor, readableHue } from '@/utils/gradient'
import { useDocAutosave } from '@/composables/useDocAutosave'
import { useDocComments } from '@/composables/useDocComments'
import { useDocLinks } from '@/composables/useDocLinks'
import { useDocPresence } from '@/composables/useDocPresence'
import { useDocVersions } from '@/composables/useDocVersions'
import { toDocJSON } from '@/utils/docSchema'
import { blockNodeById, quoteFromBlock } from '@/utils/docComments'
import { docOutline, headingForBlock } from '@/utils/docToc'

const message = useMessage()
const wsStore = useWorkspacesStore()
const theme = useThemeStore()
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
  // connId goes with the save so the server can skip announcing it back to us.
  // It is our *connection*, not our user: the same person's second tab is a
  // second caret and does need to hear about this.
  const res = await docsApi.updateContent(selected.value.id, json, version.value, connId.value)
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
  remoteSave,
  connId,
  held: heldBlock,
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

// The journal and the links panel are not columns of their own: they are the two
// faces of one sidebar that slides in over the discussion (#2738). Sharing a
// single slot is what makes them harmless — as separate fixed-width columns they
// took 600px of a 984px working area and left the text 14px wide.
//
// Neither is ever open on arrival. That is the point of the change: a reader who
// wants the text gets the text, and a sidebar is something you ask for.
const sidePanel = ref('')

// Version journal, snapshots and rollback (#2731).
const versions = useDocVersions()
const historyOpen = computed(() => sidePanel.value === 'history')

// Task links and approval protocols (#2732). Loaded lazily like the journal: a
// document is read far more often than it is linked or signed, and every reader
// paying two requests for a panel they never open would make the section slower
// for the common case.
const links = useDocLinks()
const linksOpen = computed(() => sidePanel.value === 'links')
watch(linksNudge, () => links.load())

// Outline and internal links (#2733). Unlike the panels above it needs no
// request — the outline is derived from the tree already in memory — but the
// derivation is still guarded by the toggle: it walks the document, and doing
// that on every keystroke for a reader who never opened the panel would be a
// cost with nothing on the other side of it.
// On by default since #2738. It was hidden on arrival while it was a 260px
// column and had to earn its place in the row; as a 22px rail of ticks (#2728)
// it costs nothing and is worth more shown than asked for.
const tocOpen = ref(true)
const editorRef = ref(null)
// The block the caret is in, mirrored so the outline can say which section is
// being read. Recorded here rather than asked of the editor, because the editor
// already reports it for the block lock.
const focusedBlockId = ref('')
const outline = computed(() => (tocOpen.value ? docOutline(content.value) : []))
const activeHeadingId = computed(() =>
  tocOpen.value ? headingForBlock(content.value, focusedBlockId.value) : '',
)

function onBlockFocus(blockId) {
  focusedBlockId.value = blockId || ''
  claimBlock(blockId)
}

// The panel has the heading's id; only the editor can scroll to it.
function goToHeading(blockId) {
  editorRef.value?.goToBlock?.(blockId)
}

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

// A colleague saved. Their text has to arrive here, or the room is a set of
// badges over documents that quietly diverge — and the reader's own next save
// then loses to the updated_at guard, which is what "перезагрузите страницу"
// was really saying.
//
// The fetch is coalesced: a colleague typing produces an announcement every
// autosave debounce, and answering each with its own round trip would put one
// GET per second per reader on the server for no extra information.
let pullTimer = null
watch(remoteSave, () => {
  if (!selected.value?.id) return
  clearTimeout(pullTimer)
  pullTimer = setTimeout(pullRemote, 250)
})

async function pullRemote() {
  const id = selected.value?.id
  if (!id) return
  try {
    const res = await docsApi.get(id)
    applyRemoteContent(res.data)
  } catch {
    // A failed pull is not worth interrupting anyone over: the next save
    // announcement retries it, and the version guard still protects the text.
  }
}

// Merges the server's document into the editor and re-bases our saves on it.
//
// Blocks we are holding keep their local content — that is where the caret and
// the not-yet-saved keystrokes are. Everything else, including which blocks
// exist and in what order, comes from the server.
//
// Returns false when the merge refused (a block with no id, from an old import):
// the caller decides between "leave it, the next save will notice" and a hard
// reload, because guessing at unaddressable blocks is how text goes missing.
function applyRemoteContent(doc) {
  if (!doc || doc.id !== selected.value?.id) return false
  const remote = toDocJSON(doc.content)
  const merged = mergeRemoteBlocks(content.value, remote, {
    keepIds: [heldBlock.value, focusedBlockId.value],
  })
  if (!merged) return false
  // Order matters: apply first, then re-base. applyRemote runs through the
  // editor's onUpdate, which writes content.value — doing it the other way round
  // would leave version pointing at a document we had not taken yet.
  editorRef.value?.applyRemote?.(merged)
  content.value = merged
  comments.setDoc(merged)
  if (doc.updated_at) {
    version.value = doc.updated_at
    resolveConflict(doc.updated_at)
  }
  applyPreview(doc.id, doc)
  return true
}

// A 409 still happens — a save can leave here between the announcement and the
// server's write — but it stops being a dead end. Instead of asking the user to
// reload (and lose what they typed), take the server's version, keep the block
// we are holding, and send our text again on top of it.
//
// One attempt at a time: if the retry conflicts too, the banner stays and the
// manual reload button is the answer. Retrying in a loop against a colleague who
// is typing steadily is how a "self-healing" client burns a CPU on both ends.
let recoveredAt = 0
watch(conflict, async (bad) => {
  if (!bad || !selected.value?.id) return
  if (Date.now() - recoveredAt < 3000) return
  recoveredAt = Date.now()
  try {
    const res = await docsApi.get(selected.value.id)
    if (applyRemoteContent(res.data)) scheduleSave(content.value)
  } catch {
    // Leave the banner: the user still has the explicit reload.
  }
})

// Other people in this document. The server already collapses one person's tabs
// into a single viewer, and our own id comes from the socket's welcome frame —
// no auth store needed to know who "me" is here.
const others = computed(() => viewers.value.filter((v) => v.user_id !== myUserId.value))

// Everyone gets a colour derived from their id, so the same person is the same
// colour on every screen in the room — the badge, the avatar and the stripe down
// their block all agree. readableHue keeps it legible in the active theme; the
// base palette alone is too dark against a dark background.
function colorFor(userId) {
  return readableHue(userColor(userId), theme.isDark)
}

// What an avatar needs: the person's colour, the app's diagonal gradient of that
// same hue, and a legible ink for the moment it becomes a fill.
//
// The gradient comes from hueGrad rather than being written out in CSS for two
// reasons: it is the shared definition of the accent-gradient design language,
// and the theming guard in cx-doc-editor.spec.js rightly refuses literal colours
// in this file's stylesheet. onColor matters too — --t-on-primary is tuned for
// the purple accent and goes unreadable on the bright end of the palette.
function viewerStyle(userId) {
  const c = colorFor(userId)
  return { '--doc-user-color': c, '--doc-user-grad': hueGrad(c), '--doc-user-on': onColor(c) }
}

// The lock roster the editor paints, with each holder's colour attached. The
// editor is handed the colour rather than the rule for computing it: theming is
// this view's business, and a ProseMirror plugin cannot see the theme anyway.
const paintedLocks = computed(() =>
  foreignLocks.value.map((l) => {
    const color = colorFor(l.user_id)
    return { ...l, color, text_color: onColor(color) }
  }),
)

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
    if (param && route.params.slug !== param) {
      // Opening a document from the list is a step forward and gets a history
      // entry, so Back returns to the list — with replace() there was no entry
      // to go back to, and Back left the section entirely. Canonicalizing a URL
      // that already names a document (a deep link by id, or the slug a document
      // earns on its first save) stays a replace: it is the same page under its
      // proper name, not a second one.
      if (route.params.slug) router.replace(`/documents/${param}`)
      else router.push(`/documents/${param}`)
    }
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
  clearTimeout(pullTimer)
  closeRoom()
  comments.close()
  versions.close()
  links.close()
  // The sidebar belongs to the open document, not to the section: coming back to
  // the grid and into another document should not land on someone else's journal.
  sidePanel.value = ''
  selected.value = null
  content.value = null
  if (route.params.slug) router.replace('/documents')
}

// The one way in and out of the sidebar. Both panels load lazily: opening one is
// the moment its data is wanted, and a document read without it never fetches.
// Clicking the button of the panel already showing closes the sidebar, so the
// toolbar keeps reading as two toggles even though they share a slot.
//
// The outgoing panel is released even when another takes its place — the sidebar
// shows one at a time, so a journal left loaded behind a links panel nobody can
// see is a subscription to updates for a hidden view.
async function showSide(name) {
  const next = sidePanel.value === name ? '' : name
  const prev = sidePanel.value
  if (next === prev) return
  sidePanel.value = next
  if (prev === 'history') versions.close()
  else if (prev === 'links') links.close()
  if (!selected.value?.id) return
  if (next === 'history') await versions.open(selected.value.id)
  else if (next === 'links') await links.open(selected.value.id)
}

function toggleHistory() {
  return showSide('history')
}

function toggleLinks() {
  return showSide('links')
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

// PDF is listed in both branches on purpose: it is stored rather than converted
// (#2733), so it is importable with no sidecar deployed. Leaving it out of the
// unavailable-hint would hide a feature that works.
const importHint = computed(() =>
  converter.value.available
    ? 'Импортировать документ (docx, odt, rtf, pdf, md, json)'
    : `Импортировать документ (pdf, md, json)\u2009— ${converter.value.reason || 'конвертация офисных форматов недоступна'}`,
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
  if (needsConverter(file.name) && !converter.value.available) {
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

// Delete / new nested doc used to sit as bare buttons under the text, where they
// were in the way of reading (#2727). They live in the header's «Действия» menu
// now — with the delete confirmation moved to a dialog, because a menu item has
// no anchor a popconfirm could hang off.
const docActions = computed(() => {
  const items = [{ key: 'nested', label: 'Вложенный документ' }]
  if (childCount.value)
    items.push({ key: 'children', label: `Показать вложенные (${childCount.value})` })
  items.push({ key: 'div', type: 'divider' })
  items.push({ key: 'remove', label: 'Удалить документ' })
  return items
})

function onDocAction(key) {
  if (key === 'nested') createNested()
  else if (key === 'children') drillInto(selected.value)
  else if (key === 'remove') removeAsk.value = true
}

const removeAsk = ref(false)

async function remove() {
  removeAsk.value = false
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

// A PDF dropped into an open page, as opposed to imported as a document of its
// own. Needs no sidecar either — the file is stored, not converted (#2733).
async function uploadPdf(file) {
  const fd = new FormData()
  fd.append('file', file)
  const res = await docsApi.uploadPdf(selected.value.id, fd)
  return res.data || null
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

/* ---- annotation link layer (#2730 rework) -------------------------------- */

// The curves tying a discussed block to its card in the panel.
const workEl = ref(null)
const commentsRef = ref(null)
const annotationLines = ref([])
// Below this the panel goes *under* the editor (see the media query at the
// bottom of this file), and a line drawn across that stack would connect two
// things that are no longer side by side.
const NARROW_PX = 900
const narrow = ref(false)
// The header slots (Topbar.vue) exist only when this view runs inside AppLayout;
// a unit test mounting it bare has none, and a narrow header has no room anyway.
// Either way the controls stay in .head instead of vanishing into a missing
// teleport target.
const topbarSlots = ref(false)
const inTopbar = computed(() => topbarSlots.value && !narrow.value)
let linkFrame = 0
let workObserver = null
let narrowQuery = null

function measureLinks() {
  linkFrame = 0
  // An open sidebar sits on top of the discussion (#2738), so the cards these
  // lines point at are behind it. Drawn anyway they would run out of the text
  // and stop dead at the sidebar's edge, pointing at nothing.
  if (!commentsOpen.value || sidePanel.value || narrow.value || !workEl.value) {
    annotationLines.value = []
    return
  }
  const cards = commentsRef.value?.cardAnchors?.() || []
  const ids = [...new Set(cards.map((c) => c.blockId).filter(Boolean))]
  const blocks = ids.length ? editorRef.value?.blockAnchors?.(ids) || [] : []
  // Both sides measure in viewport coordinates; the layer is positioned against
  // the working area, so everything is rebased onto it here — the two children
  // have no business knowing where their common parent sits.
  const box = workEl.value.getBoundingClientRect()
  const rebase = (p) => ({ ...p, x: p.x - box.left, y: p.y - box.top })
  annotationLines.value = linkGeometry({
    blocks: blocks.map(rebase),
    cards: cards.map(rebase),
    activeBlockId: comments.activeBlockId.value,
  })
}

// Coalesced to a frame: a scroll fires far more often than the screen redraws,
// and every trigger below can land in the same tick as the others.
function scheduleLinks() {
  if (linkFrame) return
  linkFrame = requestAnimationFrame(measureLinks)
}

function onNarrowChange(e) {
  narrow.value = e.matches
}

// Anything that moves either end invalidates every line, so they are all one
// trigger. The editor transaction matters as much as the scroll: an edit *above*
// pushes every block below it, and without this the lines start lying at the
// first keystroke.
watch(
  [
    () => comments.groups.value,
    () => comments.activeBlockId.value,
    () => content.value,
    commentsOpen,
    tocOpen,
    sidePanel,
    narrow,
  ],
  scheduleLinks,
  { deep: true },
)

// The working area is behind the loading branch, so it appears (and disappears
// with the open document) long after mount — the listeners follow the element
// rather than the component's lifetime.
watch(workEl, (el, prev) => {
  if (prev) {
    prev.removeEventListener('scroll', scheduleLinks, true)
    workObserver?.unobserve(prev)
  }
  if (!el) {
    annotationLines.value = []
    return
  }
  // Capture, because scroll does not bubble: one listener on the working area
  // then catches both scroll boxes inside it (the text and the panel), which
  // scroll independently of each other.
  el.addEventListener('scroll', scheduleLinks, true)
  workObserver?.observe(el)
  scheduleLinks()
})

onMounted(async () => {
  topbarSlots.value = !!document.getElementById('tb-slot-left')
  window.addEventListener('beforeunload', onBeforeUnload)
  window.addEventListener('resize', scheduleLinks)
  narrowQuery = window.matchMedia?.(`(max-width: ${NARROW_PX}px)`)
  if (narrowQuery) {
    narrow.value = narrowQuery.matches
    narrowQuery.addEventListener('change', onNarrowChange)
  }
  if (window.ResizeObserver) {
    workObserver = new ResizeObserver(scheduleLinks)
    if (workEl.value) workObserver.observe(workEl.value)
  }
  loadConverter()
  await loadList()
  if (route.params.slug) {
    const ok = await resolveSlug(route.params.slug)
    if (!ok) message.error('Документ не найден')
  }
})

onBeforeUnmount(() => {
  window.removeEventListener('beforeunload', onBeforeUnload)
  workEl.value?.removeEventListener('scroll', scheduleLinks, true)
  window.removeEventListener('resize', scheduleLinks)
  narrowQuery?.removeEventListener('change', onNarrowChange)
  workObserver?.disconnect()
  if (linkFrame) cancelAnimationFrame(linkFrame)
  clearTimeout(pullTimer)
  flushSave()
  closeRoom()
})

onBeforeRouteLeave(async () => {
  await flushSave()
})

// The document and the list are one route record (documents/:slug?), so moving
// between them re-renders this view instead of remounting it. Without this watch
// the URL changed and the open document stayed on screen — clicking «Документы»
// in the sidebar with a document open looked like a dead link (#2727). Covers the
// browser Back button for the same reason.
watch(
  () => route.params.slug,
  async (slug) => {
    if (slug) {
      const current = selected.value?.slug || selected.value?.id
      if (slug !== current) {
        const ok = await resolveSlug(slug)
        if (!ok) message.error('Документ не найден')
      }
      return
    }
    if (selected.value) await backToGrid()
  },
)

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
      <!-- «К списку» and the document's own controls ride in the app header:
           there is free space beside the search, and the working area is left to
           the document itself (#2727). On a narrow screen the header has no room
           for them, and in a bare unit-test mount the slots do not exist — both
           cases fall back to rendering here, in .head. -->
      <div class="head">
        <teleport to="#tb-slot-left" :disabled="!inTopbar">
          <n-button quaternary size="small" @click="backToGrid">
            <template #icon><n-icon :component="ArrowBackOutline" /></template>
            К списку
          </n-button>
        </teleport>
        <teleport to="#tb-slot-right" :disabled="!inTopbar">
          <span class="status">
            <!-- Who else has this document open (#2729). Each person carries their
               own colour — the same one that stripes the block they are holding,
               which is what makes "жёлтый блок занят" readable as a sentence
               about a person rather than as generic "someone is editing". The
               editing state is a ring, not a different colour: the colour is the
               identity and must not double as a status. -->
            <span v-if="others.length" class="viewers">
              <span
                v-for="v in others"
                :key="v.user_id"
                class="viewer"
                :class="{ editing: v.blocks.length }"
                :style="viewerStyle(v.user_id)"
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
              :title="tocOpen ? 'Скрыть оглавление' : 'Оглавление'"
              data-testid="doc-toc-toggle"
              @click="tocOpen = !tocOpen"
            >
              <template #icon><n-icon :component="ListOutline" /></template>
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
            <n-dropdown trigger="click" :options="docActions" @select="onDocAction">
              <n-button quaternary size="tiny" title="Действия" data-testid="doc-actions">
                <template #icon><n-icon :component="EllipsisHorizontalOutline" /></template>
              </n-button>
            </n-dropdown>
            <n-text v-if="saving" depth="3">Сохранение…</n-text>
            <n-text v-else-if="dirty" depth="3">Есть несохранённые правки</n-text>
            <n-text v-else depth="3">Все изменения сохранены</n-text>
          </span>
        </teleport>
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
        <n-input v-model:value="title" placeholder="Заголовок" class="title plain" @blur="rename" />
        <div ref="workEl" class="work">
          <!-- Which block each remark is about, drawn rather than implied. The
               layer spans the editor and the panel because the line crosses the
               gap between them; it never takes a pointer event. -->
          <svg v-if="annotationLines.length" class="annotation-lines" aria-hidden="true">
            <path v-for="l in annotationLines" :key="l.id" :d="l.d" :class="{ active: l.active }" />
          </svg>
          <doc-editor
            ref="editorRef"
            :model-value="content"
            :upload-image="uploadImage"
            :upload-pdf="uploadPdf"
            :locks="paintedLocks"
            :comments="comments.openCounts.value"
            class="editor"
            @change="onEditorChange"
            @block-focus="onBlockFocus"
            @blocked="onBlocked"
            @blur="onEditorBlur"
            @annotate="onAnnotate"
            @select-comments="comments.activeBlockId.value = $event"
          />
          <doc-toc
            v-if="tocOpen"
            :rows="outline"
            :active-id="activeHeadingId"
            @go="goToHeading"
            @close="tocOpen = false"
          />
          <doc-comments
            v-if="commentsOpen"
            ref="commentsRef"
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
          <!-- The sidebar overlays the discussion instead of joining the row:
               that is what keeps the text column from shrinking, and it is why
               the two panels below are mutually exclusive rather than merely
               narrow (#2738). It never covers the whole working area — the
               document stays readable behind it. -->
          <aside v-if="sidePanel" class="side" data-testid="doc-side">
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
          </aside>
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
        <!-- Delete / nested-document moved into the header's «Действия» menu
             (#2727): under the text they were permanent furniture beside a rare
             action. -->
        <n-modal
          v-model:show="removeAsk"
          preset="dialog"
          type="error"
          title="Удалить документ?"
          positive-text="Удалить"
          negative-text="Отмена"
          :positive-button-props="{ type: 'error' }"
          :content="
            childCount
              ? `Документ будет удалён вместе с вложенными (${childCount}).`
              : 'Документ будет удалён.'
          "
          @positive-click="remove"
        />
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
/* Both halves teleport into the app header on a wide screen; what stays behind
   is an empty flex row that would still spend the .docs gap. */
.head:empty {
  display: none;
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
  /* The person's own colour, not the app accent: with two people in a document
     one accent-coloured badge cannot say which of them is in the yellow block.
     Falls back to neutral when no colour was supplied. */
  border: 1px solid var(--doc-user-color, var(--t-surface));
  border-radius: 50%;
  background: var(--t-surface-alt);
  color: var(--doc-user-color, var(--t-text2));
  font-size: 10px;
  font-weight: 600;
}
.viewer:first-child {
  margin-left: 0;
}
/* Actively editing — filled with the app-wide diagonal gradient, but of this
   person's hue rather than the accent (--doc-user-grad comes from hueGrad in the
   script; the accent is the fallback when nobody supplied a colour). */
.viewer.editing {
  background: var(--doc-user-grad, var(--t-accent-grad));
  color: var(--doc-user-on, var(--t-on-primary));
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
  /* The reference frame for the annotation-link layer below. */
  position: relative;
}
/* Sits over both halves, so without pointer-events: none it would swallow every
   click meant for the text or for a thread. */
.annotation-lines {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  pointer-events: none;
  z-index: 1;
  overflow: visible;
}
/* Idle links stay at border weight: a dozen equally bright curves read worse
   than the margin counters they are meant to explain. The selected thread is
   the one that gets the accent. */
.annotation-lines path {
  fill: none;
  stroke: var(--t-border);
  stroke-width: 1.5;
}
.annotation-lines path.active {
  stroke: var(--t-primary);
  stroke-width: 2;
}
.editor {
  flex: 1;
  min-width: 0;
  min-height: 0;
}
/* The journal and the links panel (task 2738 — no hash: the theme guard in
   cx-doc-editor.spec.js reads one as a literal colour). Taken out of the flow
   entirely: in the row they were two more fixed 300px columns, and three panels
   beside a flex:1 text column left the text 14px wide. Overlaid, they cost the
   text nothing — opening one cannot reflow a single line.

   320px is chosen against the discussion, not against the window: that panel is
   300px plus the 12px gap, so the sidebar lands almost exactly on top of it and
   the document keeps the width it already had. With the discussion hidden the
   same 320px eats into the text instead, which is the tradeoff the user picked —
   the document stays on screen either way. */
.side {
  position: absolute;
  top: 0;
  right: 0;
  bottom: 0;
  /* Over the annotation layer (z-index 1), which spans the whole working area. */
  z-index: 2;
  display: flex;
  flex-direction: column;
  width: 320px;
  max-width: 100%;
  min-height: 0;
  padding: 10px;
  border: 1px solid var(--t-border);
  border-radius: 8px;
  background: var(--t-surface);
  /* The panel is opaque and sits over live content, so it needs an edge the flat
     border alone does not give it — without the shadow the discussion behind it
     reads as part of the same column. The blur is drawn in the border colour
     rather than a black wash: a literal colour cannot follow the theme, and a
     dark halo that looks right on white turns into a smudge on the dark one. */
  box-shadow: -8px 0 20px var(--t-border);
}
/* On a narrow screen the panel goes under the editor instead of squeezing the
   text into a column nobody can read. */
@media (max-width: 900px) {
  .work {
    flex-direction: column;
  }
  /* Nothing to overlay in a single column: the sidebar becomes a sheet over the
     whole working area, which is the only place left where it is readable. */
  .side {
    width: 100%;
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
