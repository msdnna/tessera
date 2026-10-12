<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { onBeforeRouteLeave, useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { NIcon, NSpin } from 'naive-ui'
import { useMessage } from '@/composables/useMessage'
import {
  AddOutline,
  AlertCircleOutline,
  ArrowBackOutline,
  BookmarkOutline,
  ChatbubbleEllipsesOutline,
  ChevronForwardOutline,
  CloudUploadOutline,
  DocumentsOutline,
  DocumentTextOutline,
  DownloadOutline,
  EllipsisHorizontal,
  FolderOutline,
  GitNetworkOutline,
  GridOutline,
  LinkOutline,
  ListOutline,
  TimeOutline,
  TrashOutline,
  VideocamOutline,
} from '@vicons/ionicons5'
import { conferences as confApi, documents as docsApi } from '@/api'
import EmptyState from '@/components/EmptyState.vue'
import DocEditor from '@/components/documents/DocEditor.vue'
import DocComments from '@/components/documents/DocComments.vue'
import DocHistory from '@/components/documents/DocHistory.vue'
import DocLinks from '@/components/documents/DocLinks.vue'
import DocTemplates from '@/components/documents/DocTemplates.vue'
import DocToc from '@/components/documents/DocToc.vue'
import DocTree from '@/components/documents/DocTree.vue'
import DocTrash from '@/components/documents/DocTrash.vue'
import { useUndoStore } from '@/stores/undo'
import DocPins from '@/components/documents/DocPins.vue'
import ActionMenu from '@/components/kit/ActionMenu.vue'
import { fileToTemplate } from '@/utils/docImport'
import {
  EXPORT_LABELS,
  downloadBlob,
  exportFileName,
  exportRefusalKey,
  importAccept,
  importOfficeFile,
  needsConverter,
  isOfficeFile,
} from '@/utils/docOffice'
import { TEMPLATE_CATEGORIES, builtinCards, builtinContent } from '@/utils/docTemplates'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useAuthStore } from '@/stores/auth'
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
import { useFormat } from '@/composables/useFormat'
import { sGet, sSet } from '@/utils/serverStorage'
import { workspaceName } from '@/utils/defaultNames'
import { toDocJSON } from '@/utils/docSchema'
import { blockNodeById, quoteFromBlock } from '@/utils/docComments'
import { docOutline, headingForBlock } from '@/utils/docToc'

const message = useMessage()
const { t } = useI18n()
const wsStore = useWorkspacesStore()
const authStore = useAuthStore()
const theme = useThemeStore()
const { formatDate, formatDateTime } = useFormat()
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
  message.info(t('documents.view.toast.restoredFromHistory'))
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
  // The class is the e2e anchor: a toast has no element of ours to hang a
  // data-testid on, and its wording moves into the locale files (#2799).
  message.warning(
    t('documents.view.toast.blockLocked', {
      name: held?.name || t('documents.view.toast.someone'),
    }),
    { class: 'msg-block-locked' },
  )
}

function applyPreview(id, data) {
  const row = list.value.find((d) => d.id === id)
  if (!row || !data) return
  if (typeof data.preview === 'string') row.preview = data.preview
  if (data.updated_at) row.updated_at = data.updated_at
}

function fmtDate(v) {
  return v ? formatDate(v, { day: 'numeric', month: 'short' }) : ''
}

// Which document the section is waiting for (#2932). The grid and the editor are
// one route record (#2727), so opening a document is not a remount but a set of
// assignments made after an await — and the reader may click the next tile, press
// Back or leave for the grid while the first one is still loading. Answers then
// come back in network order, and the late one wins by arriving last.
//
// Nothing it does is a harmless stale render: it takes over `selected` and the
// editor body, JOINS THE ABANDONED DOCUMENT'S ROOM (a subscription that holds
// block locks), and canonicalizes the URL — so a reader who clicked B ends up in A
// with the address bar agreeing. `resolveSlug` adds one more: it walks the user's
// workspaces and switches the app's scope to whichever answers.
//
// So every step after an await asks whether it is still the answer being waited
// for. Leaving counts as a new question too: `backToGrid` and the workspace switch
// bump the counter, and an answer that lands afterwards belongs to nobody.
// Same comparison as BoardView / HomeView / TaskModal.
let openSeq = 0

async function loadList() {
  const ws = wsStore.currentId
  if (!ws) return
  try {
    const res = await docsApi.list(ws)
    // The grid is scoped as well: a listing asked for the space the reader left
    // must not fill the tiles of the one they are in now.
    if (ws !== wsStore.currentId) return
    list.value = res.data || []
  } catch (e) {
    if (ws !== wsStore.currentId) return
    message.error(e.message)
  }
}

async function open(doc) {
  if (!doc?.id) return
  const mine = ++openSeq
  await flushSave()
  if (mine !== openSeq) return
  loading.value = true
  try {
    const res = await docsApi.get(doc.id)
    if (mine !== openSeq) return
    selected.value = res.data
    title.value = res.data.title || ''
    content.value = toDocJSON(res.data.content)
    version.value = res.data.updated_at
    resolveConflict(res.data.updated_at)
    openRoom(res.data.id)
    await comments.open(res.data.id, content.value)
    if (mine !== openSeq) return
    // The journal is loaded only while the panel is open: it is a side question
    // about the document, and every opened document paying for it would make the
    // section slower for the readers who never ask it.
    if (historyOpen.value) await versions.open(res.data.id)
    if (linksOpen.value) await links.open(res.data.id)
    if (mine !== openSeq) return
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
    // A document that failed to load reports it — but only while it is still the
    // one being opened. An abandoned 404 must not shout over the document that
    // answered in the meantime, nor take down its spinner.
    if (mine !== openSeq) return
    message.error(e.message)
  } finally {
    if (mine === openSeq) loading.value = false
  }
}

async function backToGrid() {
  // Leaving is a question of its own: a load still in flight is now the answer to
  // nothing, and must not put the reader back inside the document they closed.
  openSeq++
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
  // On a phone every panel is a sheet over the same working area, so opening
  // one has to put the others away — otherwise the history lands on top of the
  // discussion and the document is two overlays deep (#2893).
  if (next && narrow.value) {
    commentsOpen.value = false
    tocOpen.value = false
  }
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
    message.success(
      payload.blockId
        ? t('documents.view.toast.blockLinked')
        : t('documents.view.toast.taskLinked'),
    )
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
    message.success(t('documents.view.toast.sentForApproval'))
  } catch (e) {
    message.error(e.message)
  }
}

async function onDecide({ id, decision, comment }) {
  try {
    await links.decide(id, { decision, comment })
    message.success(
      decision === 'approved'
        ? t('documents.view.toast.signed')
        : t('documents.view.toast.declined'),
    )
  } catch (e) {
    message.error(e.message)
  }
}

async function onCancelApproval(id) {
  try {
    await links.cancel(id)
    message.info(t('documents.view.toast.routeCancelled'))
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
    message.success(t('documents.view.toast.versionSaved'))
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
    message.success(t('documents.view.toast.restored'))
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
  const mine = ++openSeq
  const ids = [wsStore.currentId, ...wsStore.list.map((w) => w.id)].filter(Boolean)
  for (const wsId of new Set(ids)) {
    try {
      const res = await docsApi.bySlug(wsId, slug)
      // The walk is the expensive part of this resolver and the loudest when it
      // is stale: it would move the whole app into another space behind a
      // document nobody is asking for any more.
      if (mine !== openSeq) return true
      if (res.data?.workspace_id && res.data.workspace_id !== wsStore.currentId) {
        await wsStore.selectWorkspace(res.data.workspace_id)
        if (mine !== openSeq) return true
        await loadList()
        if (mine !== openSeq) return true
      }
      selected.value = res.data
      title.value = res.data.title || ''
      content.value = toDocJSON(res.data.content)
      version.value = res.data.updated_at
      openRoom(res.data.id)
      await comments.open(res.data.id, content.value)
      return true
    } catch {
      // 404 in this workspace — try the next one, unless the slug being resolved
      // is no longer the one on screen.
      if (mine !== openSeq) return true
    }
  }
  // Reported as resolved when the question changed under us: the "not found"
  // toast belongs to the slug the reader is on, and this one no longer is.
  return mine !== openSeq
}

async function create() {
  if (!wsStore.currentId) return
  try {
    const res = await docsApi.create(wsStore.currentId, {
      title: t('documents.view.untitled'),
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
    ? t('documents.view.importHint')
    : t('documents.view.importHintLimited', {
        reason: converter.value.reason || t('documents.view.converterOff'),
      }),
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
    throw new Error(converter.value.reason || t('documents.view.error.converterOff'))
  }
  const {
    document: doc,
    imagesDropped,
    imagesDroppedReason,
    sectionsDiffer,
  } = await importOfficeFile(docsApi, wsStore.currentId, file, { parentId: parentId.value })
  // A file whose sections had different page geometries got one sheet, sized to
  // the widest of them (#2821). Said out loud for the same reason the dropped
  // pictures are: the document is a faithful import of its text and a reduction
  // of its layout, and the only way to know that is to be told.
  if (sectionsDiffer) {
    message.warning(t('documents.view.toast.importedSectionsDiffer'), { duration: 8000 })
  }
  // Dropped pictures are said out loud, and with the reason: a bare count told
  // the user something was lost without telling them whether re-inserting the
  // figure as PNG would help or whether it is ours to fix (#2755). The warning
  // stays up longer than the default — it is the only place the reason appears.
  if (imagesDropped) {
    const text = imagesDroppedReason
      ? t('documents.view.toast.importedImagesDroppedWhy', {
          count: imagesDropped,
          reason: imagesDroppedReason,
        })
      : t('documents.view.toast.importedImagesDropped', { count: imagesDropped })
    message.warning(text, { duration: 8000 })
  } else {
    message.success(t('documents.view.toast.imported'))
  }
  return doc
}

// .md and .json are parsed in the browser (D9) and therefore work on an install
// with no converter at all — the same picker covers both paths.
async function importLocal(file) {
  const draft = await fileToTemplate(file)
  const res = await docsApi.create(wsStore.currentId, {
    title: draft.title || t('documents.view.importedTitle'),
    icon: draft.icon || '',
    parent_id: parentId.value,
  })
  await docsApi.updateContent(res.data.id, draft.content, res.data.updated_at)
  message.success(t('documents.view.toast.imported'))
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
    // Awaited: on the desktop this is a Save-as dialog and a write to disk, and
    // a failed write has to reach the catch below rather than an unhandled
    // rejection nobody sees.
    await downloadBlob(res.data, exportFileName(selected.value.title, format))
  } catch (err) {
    message.error(exportError(err))
  } finally {
    exporting.value = ''
  }
}

// An export failure arrives as a Blob, not as JSON, because responseType is
// blob for the whole request; the interceptor reads the sentence back out of
// the body (api/index.js), and the two refusals the sidecar owns are named off
// the status instead — the server phrases those two in Russian of its own.
//
// `err.status`, not `err.response.status`: the interceptor rejects with an
// error of its own making and puts the status there. Reading `.response` gave
// undefined every time, so both branches below were dead and the locale lines
// they point at had never been shown (#2932).
function exportError(err) {
  const key = exportRefusalKey(err?.status)
  if (key) return t(key)
  return err?.message || t('documents.view.error.exportGeneric')
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
// The count on the tree's «Шаблоны» row is known once the gallery has loaded;
// until then the row shows none rather than a request on every visit.
const tplLoaded = ref(false)
// Where a document made from the gallery lands — the folder the grid is in.
const parentTitle = computed(() =>
  trail.value.length ? trail.value[trail.value.length - 1].title : wsLabel.value,
)

// «Пустой документ» — the gallery's first tile, the same create as «Новый».
async function createFromGallery() {
  templatesOpen.value = false
  await create()
}

// The workspace's meetings, for the agenda template's meeting picker (#3046):
// fetched with the gallery, in the background — the grid does not wait for them.
const tplMeetings = ref(null)
async function loadTplMeetings(ws) {
  tplMeetings.value = null
  try {
    const res = await confApi.list(ws)
    if (ws === wsStore.currentId) tplMeetings.value = res.data || []
  } catch {
    tplMeetings.value = []
  }
}

async function openTemplates() {
  templatesOpen.value = true
  tplError.value = ''
  if (!wsStore.currentId) return
  loadTplMeetings(wsStore.currentId)
  tplLoading.value = true
  try {
    const res = await docsApi.templates(wsStore.currentId)
    savedTemplates.value = res.data || []
    tplLoaded.value = true
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
//
// An agenda is created tied to the meeting the gallery offered (#3046) and is
// named after it, so the tree tells two agendas apart.
async function useTemplate(card, { conferenceId = null } = {}) {
  if (!wsStore.currentId || tplBusy.value) return
  tplBusy.value = card.id
  tplError.value = ''
  const meeting = conferenceId ? (tplMeetings.value || []).find((c) => c.id === conferenceId) : null
  try {
    const res = await docsApi.create(wsStore.currentId, {
      title: meeting ? `${card.title}: ${meeting.title}` : card.title,
      icon: card.icon || '',
      parent_id: parentId.value,
      template_id: card.builtin ? undefined : card.id,
      conference_id: meeting ? meeting.id : undefined,
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
async function saveAsTemplate(category = '') {
  if (!selected.value?.id) return
  try {
    await flushSave()
    await docsApi.createTemplate(wsStore.currentId, { document_id: selected.value.id, category })
    message.success(t('documents.view.toast.templateSaved'))
  } catch (e) {
    message.error(e.message)
  }
}

async function uploadTemplate(file, category = '') {
  tplError.value = ''
  try {
    const draft = await fileToTemplate(file)
    await docsApi.createTemplate(wsStore.currentId, { ...draft, category })
    await openTemplates()
  } catch (e) {
    tplError.value = e.message
  }
}

// Re-filing a saved template under another category (#3045).
async function setTemplateCategory(card, category) {
  tplError.value = ''
  try {
    const res = await docsApi.updateTemplate(card.id, { category })
    savedTemplates.value = savedTemplates.value.map((x) =>
      x.id === card.id ? { ...x, category: res.data?.category ?? category } : x,
    )
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
      title: t('documents.view.untitled'),
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

// The document's «⋯» menu (v2, #3106): nested document, the children, save as
// template, export — and «Удалить», which sends the document (with its nested
// ones) to the trash at once and offers the undo in a toast (#3006): the trash
// keeps it 30 days, «Корзина» at the foot of the tree brings it back.
const actionsOpen = ref(false)
const actionsAnchor = ref(null)
const removing = ref(false)
const removeFailed = ref('')
function openActions(e) {
  actionsAnchor.value = e.currentTarget
  removeFailed.value = ''
  actionsOpen.value = true
}
const docActions = computed(() => {
  const items = [{ key: 'nested', label: t('documents.view.action.nested'), icon: AddOutline }]
  if (childCount.value)
    items.push({
      key: 'children',
      label: t('documents.view.action.children', { count: childCount.value }),
      icon: FolderOutline,
    })
  if (!selected.value?.conference_id)
    items.push({
      key: 'meeting',
      label: t('documents.pins.pinMeeting'),
      icon: VideocamOutline,
    })
  // Saving as a template files it under a gallery category (#3045) right away.
  items.push({
    key: 'saveTemplate',
    label: t('documents.view.saveTemplate'),
    icon: BookmarkOutline,
    children: TEMPLATE_CATEGORIES.map((c) => ({
      key: `saveTemplate:${c || 'none'}`,
      label: t(`documents.templates.category.${c || 'none'}`),
    })),
  })
  items.push({ heading: t('documents.view.export') })
  for (const o of exportOptions.value)
    items.push({
      key: `export-${o.key}`,
      label: o.label,
      icon: DownloadOutline,
      ...(exporting.value === o.key ? { busy: true, busyLabel: o.label } : {}),
    })
  items.push({ gap: true })
  const remove = {
    key: 'remove',
    label: t('documents.view.action.remove'),
    icon: TrashOutline,
    danger: true,
  }
  if (removing.value) remove.busy = true
  else if (removeFailed.value)
    remove.failed = { title: t('common.menu.failed'), sub: removeFailed.value }
  items.push(remove)
  return items
})

async function onDocAction(key) {
  if (key === 'nested') createNested()
  else if (key === 'children') drillInto(selected.value)
  else if (key === 'meeting') pinsRef.value?.openMeeting(actionsAnchor.value)
  else if (key.startsWith('saveTemplate:'))
    saveAsTemplate(key === 'saveTemplate:none' ? '' : key.slice('saveTemplate:'.length))
  else if (key.startsWith('export-')) exportAs(key.slice('export-'.length))
  else if (key === 'remove') await remove()
}

const undo = useUndoStore()
async function remove() {
  if (!selected.value?.id) return
  const doc = { id: selected.value.id, title: selected.value.title || t('documents.view.untitled') }
  removing.value = true
  removeFailed.value = ''
  try {
    await docsApi.remove(doc.id, childCount.value > 0)
    undo.push({
      icon: 'trash',
      text: () => t('documents.trash.deleted', { title: doc.title }),
      doneText: () => t('documents.trash.restored', { title: doc.title }),
      undo: async () => {
        await docsApi.untrash(doc.id)
        await loadList()
        trashToken.value++
      },
    })
    trashToken.value++
    actionsOpen.value = false
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
    removeFailed.value = e.message
  } finally {
    removing.value = false
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

/* ---- the document tree (v2, #3106) ---------------------------------------- */

// The tree is a 240px column beside the documents. With a document open it has
// to share the row with the text, the outline rail and the discussion — and the
// text keeping its width is the older promise (#2738): on a narrow desktop the
// tree folds away and its button floats it over the text instead.
const rootEl = ref(null)
const rootWidth = ref(0)
let rootObserver = null
const TREE_BESIDE_DOC = 1100
const TREE_BESIDE_GRID = 720
const treeHidden = ref(sGet('tessera_doc_tree') === '0')
const treeFloating = ref(false)
const treeFits = computed(
  () => rootWidth.value >= (selected.value ? TREE_BESIDE_DOC : TREE_BESIDE_GRID),
)
const showTree = computed(() => !narrow.value && treeFits.value && !treeHidden.value)
function toggleTree() {
  if (treeFits.value) {
    treeHidden.value = !treeHidden.value
    sSet('tessera_doc_tree', treeHidden.value ? '0' : '1')
    treeFloating.value = false
  } else {
    treeFloating.value = !treeFloating.value
  }
}
watch(showTree, (on) => {
  if (on) treeFloating.value = false
})
// The documents' trash (#3006): /documents?trash=1 — from the tree's «Корзина»
// row, or the grid head's button where there is no tree (a phone).
const trashOpen = computed(() => route.query.trash === '1' && !route.params.slug)
const trashToken = ref(0)
async function openTrash() {
  treeFloating.value = false
  if (selected.value) await backToGrid()
  if (trashOpen.value) return closeTrash()
  router.push({ path: '/documents', query: { trash: '1' } })
}
function closeTrash() {
  router.replace('/documents')
}

async function openFromTree(doc) {
  treeFloating.value = false
  if (doc?.id === selected.value?.id) return
  trail.value = []
  await open(doc)
}
async function createFromTree() {
  treeFloating.value = false
  trail.value = []
  await create()
}
const wsLabel = computed(() => workspaceName(wsStore.current))

// The open document's place: its parents, root first — the crumbs in its head.
const ancestors = computed(() => {
  const out = []
  let pid = selected.value?.parent_id
  const seen = new Set()
  while (pid && !seen.has(pid)) {
    seen.add(pid)
    const p = list.value.find((d) => d.id === pid)
    if (!p) break
    out.unshift(p)
    pid = p.parent_id
  }
  return out
})
// The head's stage and meeting chips (#3044, #3046). A pin is not a content
// edit — the server leaves updated_at alone — so only the two ids are taken
// from the answer, never the body or the save token.
const pinsRef = ref(null)
function onPinsUpdate(doc) {
  if (!selected.value || doc?.id !== selected.value.id) return
  selected.value = {
    ...selected.value,
    milestone_id: doc.milestone_id ?? null,
    conference_id: doc.conference_id ?? null,
  }
}
function fmtUpdated(v) {
  return v ? formatDateTime(v) : ''
}
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

// On a phone the outline and the discussion are sheets over the text rather
// than columns beside it, so a document that opened with both on would show
// none of itself. They start put away and are opened one at a time from the top
// bar (#2893); on a wide screen nothing changes — there they fit side by side.
function closeSheets() {
  commentsOpen.value = false
  tocOpen.value = false
}

function toggleComments() {
  commentsOpen.value = !commentsOpen.value
  if (narrow.value && commentsOpen.value) {
    tocOpen.value = false
    // Through showSide, not by clearing the ref: the history and the links
    // panel each hold an open subscription that only their own close() drops.
    if (sidePanel.value) showSide(sidePanel.value)
  }
}

function toggleToc() {
  tocOpen.value = !tocOpen.value
  if (narrow.value && tocOpen.value) {
    commentsOpen.value = false
    if (sidePanel.value) showSide(sidePanel.value)
  }
}

function onNarrowChange(e) {
  narrow.value = e.matches
  if (e.matches) closeSheets()
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
  if (window.ResizeObserver && rootEl.value) {
    rootObserver = new ResizeObserver(([e]) => (rootWidth.value = e.contentRect.width))
    rootObserver.observe(rootEl.value)
  }
  rootWidth.value = rootEl.value?.clientWidth || 0
  window.addEventListener('beforeunload', onBeforeUnload)
  window.addEventListener('resize', scheduleLinks)
  narrowQuery = window.matchMedia?.(`(max-width: ${NARROW_PX}px)`)
  if (narrowQuery) {
    narrow.value = narrowQuery.matches
    if (narrow.value) closeSheets()
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
    if (!ok) message.error(t('documents.view.error.notFound'))
  }
})

onBeforeUnmount(() => {
  window.removeEventListener('beforeunload', onBeforeUnload)
  workEl.value?.removeEventListener('scroll', scheduleLinks, true)
  window.removeEventListener('resize', scheduleLinks)
  narrowQuery?.removeEventListener('change', onNarrowChange)
  workObserver?.disconnect()
  rootObserver?.disconnect()
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
        if (!ok) message.error(t('documents.view.error.notFound'))
      }
      return
    }
    if (selected.value) await backToGrid()
  },
)

watch(
  () => wsStore.currentId,
  () => {
    // A document being opened belongs to the space being left (#2932).
    openSeq++
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
  <div
    ref="rootEl"
    class="docs"
    :class="{ narrow, 'with-tree': showTree, compact: rootWidth < 1180 }"
  >
    <!-- The tree (v2, #3106): beside the documents when it fits, floated over
         them from the head's button when it does not, and gone on a phone —
         there the grid is the way in. -->
    <doc-tree
      v-if="showTree || (treeFloating && !narrow)"
      class="docs-tree"
      :class="{ floating: !showTree }"
      :floating="!showTree"
      :docs="list"
      :selected-id="selected?.id || ''"
      :ws-id="wsStore.currentId || ''"
      :ws-name="wsLabel"
      :templates-count="tplLoaded ? galleryCards.length : 0"
      :trash-active="trashOpen"
      @open="openFromTree"
      @create="createFromTree"
      @templates="openTemplates"
      @close="treeFloating = false"
      @trash="openTrash"
    />

    <div class="docs-main">
      <!-- The trash (#3006): deleted documents with «Вернуть» / «Удалить навсегда». -->
      <doc-trash
        v-if="!selected && trashOpen"
        :ws-id="wsStore.currentId || ''"
        :reload-token="trashToken"
        @restored="loadList"
        @close="closeTrash"
      />
      <!-- Grid: documents as tiles with a preview, no editing surface. -->
      <template v-else-if="!selected">
        <div class="dhead">
          <button
            v-if="!showTree && !narrow"
            type="button"
            class="dh-ic"
            :class="{ on: treeFloating }"
            :title="$t('documents.tree.show')"
            :aria-label="$t('documents.tree.show')"
            data-testid="doc-tree-toggle"
            @click="toggleTree"
          >
            <n-icon :component="GitNetworkOutline" :size="16" />
          </button>
          <nav class="dcrumbs" :aria-label="$t('documents.view.crumbRoot')">
            <button
              type="button"
              class="dcrumb"
              :class="{ cur: !trail.length }"
              @click="crumbTo(-1)"
            >
              {{ $t('documents.view.crumbRoot') }}
            </button>
            <template v-for="(c, i) in trail" :key="c.id">
              <n-icon :component="ChevronForwardOutline" :size="12" class="dcrumb-sep" />
              <button
                type="button"
                class="dcrumb"
                :class="{ cur: i === trail.length - 1 }"
                @click="crumbTo(i)"
              >
                {{ c.title }}
              </button>
            </template>
          </nav>
          <span class="grow" />
          <div class="dh-actions">
            <button
              type="button"
              class="dh-btn"
              data-testid="doc-import"
              :disabled="importing"
              :title="importHint"
              @click="pickImport"
            >
              <n-icon :component="CloudUploadOutline" :size="15" />
              <span class="dh-btn-l">{{ $t('documents.view.import') }}</span>
            </button>
            <input
              ref="importInput"
              type="file"
              class="hidden-file"
              :accept="importAccept()"
              @change="onImportPicked"
            />
            <button
              v-if="narrow"
              type="button"
              class="dh-btn"
              data-testid="doc-trash-open"
              :title="$t('documents.tree.trash')"
              @click="openTrash"
            >
              <n-icon :component="TrashOutline" :size="15" />
            </button>
            <button type="button" class="dh-btn" data-testid="doc-templates" @click="openTemplates">
              <n-icon :component="GridOutline" :size="15" />
              <span class="dh-btn-l">{{ $t('documents.view.fromTemplate') }}</span>
            </button>
            <button type="button" class="dh-btn primary" data-testid="doc-new" @click="create">
              <n-icon :component="AddOutline" :size="16" />
              <span class="dh-btn-l">{{ $t('documents.view.create') }}</span>
            </button>
          </div>
        </div>

        <div v-if="tiles.length" class="grid">
          <button v-for="d in tiles" :key="d.id" type="button" class="tile" @click="open(d)">
            <div class="tile-head">
              <span v-if="d.icon" class="doc-emoji">{{ d.icon }}</span>
              <n-icon v-else :component="DocumentTextOutline" :size="16" class="tile-glyph" />
              <span class="tile-title">{{ d.title || $t('documents.view.untitled') }}</span>
            </div>
            <p class="tile-preview">
              {{ d.preview || $t('documents.view.emptyPreview') }}
            </p>
            <div class="tile-foot">
              <span>{{ fmtDate(d.updated_at) }}</span>
              <span v-if="list.filter((x) => x.parent_id === d.id).length" class="tile-nested">
                <n-icon :component="FolderOutline" :size="12" />
                {{
                  $t('documents.view.nestedCount', {
                    count: list.filter((x) => x.parent_id === d.id).length,
                  })
                }}
              </span>
            </div>
          </button>
        </div>
        <empty-state v-else :icon="DocumentsOutline" :text="$t('documents.view.empty')">
          <button type="button" class="dh-btn primary" @click="create">
            <n-icon :component="AddOutline" :size="16" />
            <span>{{ $t('documents.view.create') }}</span>
          </button>
        </empty-state>
      </template>

      <!-- Editor: the document's head, its title and the working area. -->
      <template v-else>
        <!-- The document's own head row (v2, #3106, mockup DocsPage): where it
             lives on the left, presence, save state and its tools on the right.
             It used to ride in the app header (#2727); in v2 the header names
             the section and the document keeps its controls above itself. -->
        <div class="dhead doc" data-testid="doc-head">
          <button
            v-if="!showTree && !narrow"
            type="button"
            class="dh-ic"
            :class="{ on: treeFloating }"
            :title="$t('documents.tree.show')"
            :aria-label="$t('documents.tree.show')"
            data-testid="doc-tree-toggle"
            @click="toggleTree"
          >
            <n-icon :component="GitNetworkOutline" :size="16" />
          </button>
          <nav class="dcrumbs" :aria-label="$t('documents.view.crumbRoot')">
            <button
              type="button"
              class="dcrumb root"
              :title="$t('documents.view.back')"
              data-testid="doc-back"
              @click="backToGrid"
            >
              <n-icon v-if="narrow" :component="ArrowBackOutline" :size="16" />
              <span v-else>{{ $t('documents.view.crumbRoot') }}</span>
            </button>
            <template v-if="!narrow">
              <template v-for="a in ancestors" :key="a.id">
                <n-icon :component="ChevronForwardOutline" :size="12" class="dcrumb-sep" />
                <button type="button" class="dcrumb" @click="open(a)">
                  {{ a.title || $t('documents.view.untitled') }}
                </button>
              </template>
              <n-icon :component="ChevronForwardOutline" :size="12" class="dcrumb-sep" />
              <span class="dcrumb cur">{{ selected.title || $t('documents.view.untitled') }}</span>
            </template>
          </nav>
          <span class="grow" />
          <span class="status">
            <!-- Who else has this document open (#2729). Each person carries their
                 own colour — the same one that stripes the block they are
                 holding. Editing is a filled disc, the colour stays the identity. -->
            <span v-if="others.length" class="viewers">
              <span
                v-for="v in others"
                :key="v.user_id"
                class="viewer"
                :class="{ editing: v.blocks.length }"
                :style="viewerStyle(v.user_id)"
                :title="
                  v.blocks.length
                    ? $t('documents.view.viewerEditing', { name: v.name })
                    : $t('documents.view.viewerViewing', { name: v.name })
                "
              >
                {{ initials(v.name) }}
              </span>
            </span>
            <span
              class="save-state"
              :class="saving ? 'saving' : dirty ? 'dirty' : 'saved'"
              data-testid="doc-save-state"
              :data-state="saving ? 'saving' : dirty ? 'dirty' : 'saved'"
              :title="
                saving
                  ? $t('documents.view.state.saving')
                  : dirty
                    ? $t('documents.view.state.dirty')
                    : $t('documents.view.state.saved')
              "
            >
              <span class="save-dot" />
              <span class="save-l">{{
                saving
                  ? $t('documents.view.state.saving')
                  : dirty
                    ? $t('documents.view.state.dirty')
                    : $t('documents.view.state.saved')
              }}</span>
            </span>
            <button
              type="button"
              class="dh-ic"
              :class="{ on: commentsOpen }"
              :title="
                commentsOpen
                  ? $t('documents.view.comments.hide')
                  : $t('documents.view.comments.show')
              "
              data-testid="doc-comments-toggle"
              @click="toggleComments"
            >
              <n-icon :component="ChatbubbleEllipsesOutline" :size="16" />
              <span v-if="comments.openCount.value" class="dh-n">{{
                comments.openCount.value
              }}</span>
            </button>
            <button
              type="button"
              class="dh-ic"
              :class="{ on: tocOpen }"
              :title="tocOpen ? $t('documents.view.toc.hide') : $t('documents.view.toc.show')"
              data-testid="doc-toc-toggle"
              @click="toggleToc"
            >
              <n-icon :component="ListOutline" :size="16" />
            </button>
            <button
              type="button"
              class="dh-ic"
              :class="{ on: historyOpen }"
              :title="
                historyOpen ? $t('documents.view.history.hide') : $t('documents.view.history.show')
              "
              data-testid="doc-history-toggle"
              @click="toggleHistory"
            >
              <n-icon :component="TimeOutline" :size="16" />
            </button>
            <button
              type="button"
              class="dh-ic"
              :class="{ on: linksOpen }"
              :title="linksOpen ? $t('documents.view.links.hide') : $t('documents.view.links.show')"
              data-testid="doc-links-toggle"
              @click="toggleLinks"
            >
              <n-icon :component="LinkOutline" :size="16" />
            </button>
            <button
              type="button"
              class="dh-ic"
              :class="{ on: actionsOpen }"
              :title="$t('documents.view.actions')"
              :aria-label="$t('documents.view.actions')"
              data-testid="doc-actions"
              @click="openActions"
            >
              <n-icon :component="EllipsisHorizontal" :size="16" />
            </button>
          </span>
        </div>

        <div v-if="conflict" class="doc-alert warn" role="alert" data-testid="doc-conflict">
          <n-icon :component="AlertCircleOutline" :size="16" />
          <span>{{ $t('documents.view.conflict') }}</span>
          <button type="button" class="doc-alert-act" @click="reload">
            {{ $t('documents.view.reload') }}
          </button>
        </div>
        <div v-else-if="saveError" class="doc-alert bad" role="alert">
          <n-icon :component="AlertCircleOutline" :size="16" />
          <span>{{ saveError }}</span>
        </div>

        <n-spin v-if="loading" size="small" />
        <template v-else>
          <div class="dtitle-wrap">
            <input
              v-model="title"
              class="dtitle"
              :placeholder="$t('documents.view.titlePlaceholder')"
              :aria-label="$t('documents.view.titlePlaceholder')"
              data-testid="doc-title"
              @blur="rename"
              @keydown.enter.prevent="$event.target.blur()"
            />
            <div class="dmeta">
              <span v-if="selected.updated_at" class="dmeta-t">{{
                $t('documents.view.updatedAt', { time: fmtUpdated(selected.updated_at) })
              }}</span>
              <button v-if="childCount" type="button" class="dchip" @click="drillInto(selected)">
                <n-icon :component="FolderOutline" :size="13" />
                {{ $t('documents.view.nestedCount', { count: childCount }) }}
              </button>
              <doc-pins
                ref="pinsRef"
                :doc="selected"
                :ws-id="wsStore.currentId || ''"
                @update="onPinsUpdate"
              />
            </div>
          </div>
          <div ref="workEl" class="work">
            <!-- Which block each remark is about, drawn rather than implied. The
                 layer spans the editor and the panel because the line crosses the
                 gap between them; it never takes a pointer event. -->
            <svg v-if="annotationLines.length" class="annotation-lines" aria-hidden="true">
              <path
                v-for="l in annotationLines"
                :key="l.id"
                :d="l.d"
                :class="{ active: l.active }"
              />
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
              :narrow="narrow"
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
              :narrow="narrow"
              @close="commentsOpen = false"
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
                :can-manage="wsStore.canManage"
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
            <div class="nested-title">{{ $t('documents.view.nestedTitle') }}</div>
            <div class="grid small">
              <button v-for="d in children" :key="d.id" type="button" class="tile" @click="open(d)">
                <div class="tile-head">
                  <span v-if="d.icon" class="doc-emoji">{{ d.icon }}</span>
                  <n-icon v-else :component="DocumentTextOutline" :size="16" class="tile-glyph" />
                  <span class="tile-title">{{ d.title || $t('documents.view.untitled') }}</span>
                </div>
                <p class="tile-preview">{{ d.preview || $t('documents.view.emptyPreview') }}</p>
              </button>
            </div>
          </div>
        </template>
      </template>
    </div>

    <ActionMenu
      v-model:show="actionsOpen"
      :anchor="() => actionsAnchor"
      placement="bottom-end"
      :width="290"
      :header="selected?.title || ''"
      :items="docActions"
      testid="doc-action"
      @select="onDocAction"
    />

    <!-- Outside the grid/editor branches: the gallery is reachable from the
         grid and the tree, but "сохранить как шаблон" refreshes it from the
         editor too. -->
    <doc-templates
      v-model:show="templatesOpen"
      :templates="galleryCards"
      :loading="tplLoading"
      :busy="tplBusy"
      :error="tplError"
      :target="parentTitle"
      :user-id="authStore.user?.id || ''"
      :meetings="tplMeetings"
      @use="useTemplate"
      @blank="createFromGallery"
      @upload="uploadTemplate"
      @remove="removeTemplate"
      @category="setTemplateCategory"
    />
  </div>
</template>

<style scoped>
/* The picker is driven by the Импорт button; a visible file input would sit in
   the header with a browser-styled label nothing else on this screen matches. */
.hidden-file {
  display: none;
}

/* v2 (задача 3106): the tree column and the documents side by side. No lines — the
   tree is a --t-col surface, the head is spacing, panels are their own
   surfaces. */
.docs {
  position: relative;
  display: flex;
  gap: 16px;
  height: 100%;
  min-height: 0;
}
.docs-tree {
  flex: none;
}
/* Floated over the documents when it does not fit beside them. */
.docs-tree.floating {
  position: absolute;
  top: 44px;
  left: 0;
  bottom: 0;
  z-index: 6;
}
.docs-main {
  flex: 1;
  min-width: 0;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.dhead {
  display: flex;
  align-items: center;
  gap: 8px;
  min-height: 36px;
  flex-wrap: wrap;
}
.dcrumbs {
  display: flex;
  align-items: center;
  gap: 4px;
  min-width: 0;
  overflow: hidden;
}
/* An open document's head stays one row: the crumbs give way (the parents
   first, then the title), the tools never wrap under them. */
.dhead.doc {
  flex-wrap: nowrap;
}
.dhead.doc .dcrumbs {
  flex: 0 1 auto;
}
.dhead.doc .status {
  flex: none;
  flex-wrap: nowrap;
}
.dhead.doc .dcrumb {
  flex: 0 1 auto;
  min-width: 36px;
}
.dhead.doc .dcrumb.root {
  flex: none;
}
.dcrumb {
  flex: none;
  max-width: 220px;
  padding: 4px 6px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--t-text3);
  font: inherit;
  font-size: 13px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  cursor: pointer;
}
button.dcrumb:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.dcrumb:focus-visible {
  outline: 2px solid var(--t-primary);
  outline-offset: 1px;
}
.dcrumb.cur {
  flex: 0 1 auto;
  color: var(--t-text1);
  font-weight: 650;
  cursor: default;
}
.dcrumb.root {
  display: inline-flex;
  align-items: center;
}
.dcrumb-sep {
  flex: none;
  color: var(--t-text3);
}
.grow {
  flex: 1;
}
.dh-actions,
.status {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-wrap: wrap;
}
.dh-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 32px;
  padding: 0 12px;
  border: 0;
  border-radius: var(--t-r-ctl);
  background: var(--t-ctl);
  color: var(--t-text1);
  font: inherit;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
}
.dh-btn:hover {
  background: var(--t-hover);
}
.dh-btn:disabled {
  opacity: 0.6;
  cursor: default;
}
.dh-btn.primary {
  background: var(--t-accent-grad);
  color: var(--t-on-primary);
}
.dh-btn.primary:hover {
  filter: brightness(1.06);
}
.dh-btn:focus-visible,
.dh-ic:focus-visible {
  outline: 2px solid var(--t-primary);
  outline-offset: 2px;
}
.dh-ic {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 4px;
  min-width: 30px;
  height: 30px;
  padding: 0 7px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: var(--t-text2);
  font: inherit;
  font-size: 12px;
  cursor: pointer;
}
.dh-ic:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.dh-ic.on {
  background: var(--t-ctl);
  color: var(--t-primary);
}
.dh-n {
  font-weight: 650;
  font-variant-numeric: tabular-nums;
}
.save-state {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  margin-right: 6px;
  font-size: 12px;
  color: var(--t-text3);
  white-space: nowrap;
}
.save-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--t-text3);
}
.save-state.dirty .save-dot,
.save-state.saving .save-dot {
  background: var(--t-primary);
}
/* Presence avatars. Initials rather than photos: the roster is a live hint at
   the edge of the head, and loading an avatar per viewer would be a request
   burst every time someone opens the document. */
.viewers {
  display: flex;
  align-items: center;
  margin-right: 6px;
}
.viewer {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  margin-left: -6px;
  /* The person's own colour, not the app accent: with two people in a document
     one accent-coloured badge cannot say which of them is in the yellow block. */
  border-radius: 50%;
  box-shadow: 0 0 0 2px var(--t-bg);
  background: var(--t-ctl);
  color: var(--doc-user-color);
  font-size: 10px;
  font-weight: 650;
}
.viewer:first-child {
  margin-left: 0;
}
/* Actively editing — filled with the app-wide diagonal gradient of this
   person's hue (--doc-user-grad comes from hueGrad in the script). */
.viewer.editing {
  background: var(--doc-user-grad);
  color: var(--doc-user-on);
}
.doc-alert {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: none;
  padding: 10px 12px;
  border-radius: 12px;
  background: var(--t-col);
  font-size: 13px;
  color: var(--t-text1);
}
.doc-alert.warn :deep(svg) {
  color: var(--t-primary);
}
.doc-alert.bad {
  color: var(--t-error);
}
.doc-alert-act {
  margin-left: auto;
  padding: 4px 10px;
  border: 0;
  border-radius: 7px;
  background: var(--t-ctl);
  color: var(--t-text1);
  font: inherit;
  font-size: 12.5px;
  font-weight: 600;
  cursor: pointer;
}
.dtitle-wrap {
  flex: none;
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 4px 4px 2px;
}
/* The title: 30/700 on the canvas, no field chrome — it is the document's
   heading, edited in place. */
.dtitle {
  width: 100%;
  padding: 0;
  border: 0;
  outline: 0;
  background: transparent;
  color: var(--t-text1);
  font: inherit;
  font-size: 30px;
  font-weight: 700;
  line-height: 1.2;
}
.dtitle::placeholder {
  color: var(--t-placeholder);
}
.dtitle:focus-visible {
  border-radius: 6px;
  box-shadow: 0 0 0 2px var(--t-primary);
}
.dmeta {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  font-size: 13px;
  color: var(--t-text3);
}
.dchip {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  height: 24px;
  padding: 0 9px;
  border: 0;
  border-radius: var(--t-r-pill);
  background: var(--t-pill);
  color: var(--t-text2);
  font: inherit;
  font-size: 12px;
  cursor: pointer;
}
.dchip:hover {
  color: var(--t-text1);
}
.dchip.stub {
  opacity: 0.55;
  cursor: default;
}
.grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
  gap: 12px;
  overflow-y: auto;
  align-content: start;
  padding: 2px;
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
  padding: 14px;
  text-align: left;
  border: 0;
  border-radius: var(--t-r-card);
  background: var(--t-card);
  box-shadow: var(--t-sh-card);
  color: var(--t-text1);
  font: inherit;
  cursor: pointer;
  transition:
    box-shadow 0.12s ease,
    background 0.12s ease;
}
.tile:hover {
  background: var(--t-card-hover);
  box-shadow: var(--t-sh-hover);
}
.tile:focus-visible {
  outline: 2px solid var(--t-primary);
  outline-offset: 2px;
}
.tile-head {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}
.tile-glyph {
  flex: none;
  color: var(--t-text3);
}
.tile-title {
  font-weight: 650;
  font-size: 14px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.tile-preview {
  flex: 1;
  margin: 0;
  font-size: 12.5px;
  line-height: 1.5;
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
  gap: 8px;
  font-size: 11.5px;
  color: var(--t-text3);
}
.tile-nested {
  display: inline-flex;
  align-items: center;
  gap: 4px;
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
/* Idle links stay quiet: a dozen equally bright curves read worse than the
   margin counters they are meant to explain. The selected thread is the one
   that gets the accent. */
.annotation-lines path {
  fill: none;
  stroke: var(--t-text3);
  stroke-opacity: 0.35;
  stroke-width: 1.5;
}
.annotation-lines path.active {
  stroke: var(--t-primary);
  stroke-opacity: 1;
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
   text nothing — opening one cannot reflow a single line. 320px lands almost
   exactly on top of the discussion (300px plus the 12px gap). */
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
  padding: 12px;
  box-sizing: border-box;
  border-radius: var(--t-r-pop);
  background: var(--t-pop);
  /* The panel is opaque and sits over live content: the popover shadow is its
     edge (v2 — no outline). */
  box-shadow: var(--t-sh-pop);
}
.nested {
  flex: none;
}
.nested-title {
  font-size: 11.5px;
  font-weight: 650;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--t-text3);
}
.doc-emoji {
  font-size: 14px;
  line-height: 1;
}
/* Tight head: the save state keeps its dot, the words go to the tooltip. */
.docs.compact .save-l {
  display: none;
}
@media (hover: none) {
  .save-l {
    display: none;
  }
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
  .save-l {
    display: none;
  }
  .dh-btn-l {
    display: none;
  }
  .dh-btn {
    padding: 0 10px;
  }
  .dtitle {
    font-size: 24px;
  }
  .status {
    flex-wrap: nowrap;
  }
}
</style>
