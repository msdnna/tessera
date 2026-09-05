<script setup>
// The document editor, alone on the page, for the Android app to embed (#2894 §4).
//
// Why a WebView at all: the editor is TipTap/ProseMirror with our own extensions
// (block ids, locks, slash menu, page geometry, PDF blocks). Compose has no
// rich-text model of that shape, and a partial native editor would silently drop
// the attributes it does not understand the first time someone fixes a typo on
// their phone. So the phone edits the *same* editor — everything around it
// (list, reader, comments, history) stays native.
//
// This view is deliberately thin: no sidebar, no header, no modals, no router
// links. It owns exactly three things — the session the host hands it, the
// document, and the status it reports back.
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { documents as docsApi, setAccessToken, setRefreshHook } from '@/api'
import DocEditor from '@/components/documents/DocEditor.vue'
import { useDocAutosave } from '@/composables/useDocAutosave'
import { useDocComments } from '@/composables/useDocComments'
import { useDocPresence } from '@/composables/useDocPresence'
import { useThemeStore } from '@/stores/theme'
import { useAuthStore } from '@/stores/auth'
import { setI18nLocale } from '@/i18n'
import { toDocJSON } from '@/utils/docSchema'
import {
  annotatePayload,
  embedStatus,
  parseEmbedParams,
  parseImportPayload,
  sendToHost,
} from '@/utils/docEmbed'
import { htmlToDoc } from '@/utils/docImport'
import { normalizeOfficeHtml } from '@/utils/docOfficeHtml'
import { withImportedPage } from '@/utils/docOffice'
import { pdfDocument } from '@/utils/docPdf'
import { userColor } from '@/utils/userColor'
import { onColor } from '@/utils/gradient'

const route = useRoute()
const theme = useThemeStore()
const auth = useAuthStore()

const params = parseEmbedParams(route.query)
const slug = String(route.params.slug || '')

const doc = ref(null)
const content = ref(null)
const version = ref(null)
const loading = ref(true)
const fatal = ref('')

// The host owns the session. When our 15-minute access token dies mid-edit we
// ask for a new one instead of refreshing ourselves: the refresh token is the
// app's, and a second holder rotating it would sign the app out.
let tokenWaiters = []
function askHostForToken() {
  return new Promise((resolve) => {
    tokenWaiters.push(resolve)
    if (!sendToHost('requestToken')) {
      // No bridge (a browser opened the URL by hand): nothing will answer, so
      // fail now rather than leave the save hanging forever.
      tokenWaiters = tokenWaiters.filter((w) => w !== resolve)
      resolve('')
      return
    }
    // A host that goes quiet must not freeze autosave. The save fails, stays
    // queued, and the next attempt asks again.
    setTimeout(() => {
      if (!tokenWaiters.includes(resolve)) return
      tokenWaiters = tokenWaiters.filter((w) => w !== resolve)
      resolve('')
    }, 15000)
  })
}

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
  const res = await docsApi.updateContent(doc.value.id, json, version.value, connId.value)
  version.value = res.data?.updated_at || version.value
  return res.data
})

// Presence and per-block locks come along for free: the phone joins the same
// room as every browser tab, so a block someone is holding is painted and
// refused here too, and the desktop sees the phone's caret (#2729).
const {
  foreignLocks,
  commentsNudge,
  contentNudge,
  connId,
  open: openRoom,
  close: closeRoom,
  acquire: claimBlock,
} = useDocPresence()

// The lock roster with each holder's colour attached — same rule as the web
// view: theming is the page's business, a ProseMirror plugin cannot see it.
const paintedLocks = computed(() =>
  foreignLocks.value.map((l) => {
    const color = userColor(l.user_id)
    return { ...l, color, text_color: onColor(color) }
  }),
)

// Discussion counts are painted in the margin so a remark is visible from the
// phone; the threads themselves are a native sheet (§5), reached by the tap the
// bridge forwards below. The same composable the web panel uses — the counting
// rule (open threads, by block, in document order) belongs in one place.
const comments = useDocComments()
watch(commentsNudge, () => comments.load())

// Someone else saved this document. Reloading under a caret that is mid-word is
// worse than being a few seconds stale, so the host is told and the *user*
// decides — the same shape the web banner has, drawn natively.
watch(contentNudge, () => sendToHost('onRemoteChange'))

const status = computed(() =>
  embedStatus({
    saving: saving.value,
    dirty: dirty.value,
    conflict: conflict.value,
    error: saveError.value,
  }),
)
watch(status, (s) => sendToHost('onStatus', { status: s, error: saveError.value || '' }), {
  immediate: false,
})

function onEditorChange(json) {
  content.value = json
  // Keeps anchor matching in step with the text: a thread whose block was just
  // deleted stops being painted straight away, not after a reload.
  comments.setDoc(json)
  scheduleSave(json)
}

// Refusing a keystroke silently reads as a broken editor. There is no toast on
// this page (no message provider, and a web toast inside a native screen looks
// borrowed) — the host says it in its own snackbar.
function onBlocked(held) {
  sendToHost('onBlocked', held?.name || '')
}

// Both ways into a discussion end up here: the block handle's comment button,
// and a tap on the count the margin paints on a block that already has one. The
// host cannot tell them apart and does not need to — it opens the same sheet on
// the same block, with the existing threads and a composer for a new one.
function onAnnotate(payload) {
  sendToHost('onAnnotate', annotatePayload(content.value, payload?.blockId || ''))
}

function onSelectComments(blockId) {
  sendToHost('onAnnotate', annotatePayload(content.value, blockId || ''))
}

async function uploadImage(file) {
  const fd = new FormData()
  fd.append('file', file)
  const res = await docsApi.uploadAsset(doc.value.id, fd)
  return res.data?.url || ''
}

async function uploadPdf(file) {
  const fd = new FormData()
  fd.append('file', file)
  const res = await docsApi.uploadPdf(doc.value.id, fd)
  return res.data || null
}

/** Re-reads the server's version. The host calls this to end a conflict — after
 *  it, the local text is gone and the server's is what the caret sits in. */
async function reload() {
  if (!doc.value?.id) return
  cancelSave()
  const res = await docsApi.get(doc.value.id)
  doc.value = res.data
  content.value = toDocJSON(res.data.content)
  version.value = res.data.updated_at
  comments.setDoc(content.value)
  resolveConflict(res.data.updated_at)
  sendToHost('onStatus', { status: 'saved', error: '' })
}

async function load() {
  try {
    const res = await docsApi.bySlug(params.workspaceId, slug)
    doc.value = res.data
    content.value = toDocJSON(res.data.content)
    version.value = res.data.updated_at
    openRoom(res.data.id)
    await comments.open(res.data.id, content.value)
    sendToHost('onReady', { id: res.data.id, title: res.data.title || '' })
  } catch (e) {
    fatal.value = e?.message || 'error'
    sendToHost('onError', fatal.value)
  } finally {
    loading.value = false
  }
}

onMounted(async () => {
  // The host drives the look: the app's theme and language, not this page's
  // remembered ones — the editor is a piece of the app's screen and must not
  // arrive white inside a dark one.
  theme.setThemeMode(params.dark ? 'dark' : 'light')
  if (params.locale) await setI18nLocale(params.locale)

  if (!params.ready) {
    fatal.value = 'params'
    loading.value = false
    sendToHost('onError', 'params')
    return
  }
  setAccessToken(params.token)
  auth.setToken(params.token)
  setRefreshHook(askHostForToken)

  // Host → page. Installed before the first request so a token that expires
  // during the initial load is already answerable.
  window.tesseraEmbed = {
    /** Answer to `requestToken`, and the way the host pushes a token it
     *  refreshed on its own account. */
    setToken(token) {
      const value = String(token || '')
      if (value) {
        setAccessToken(value)
        auth.setToken(value)
      }
      const waiters = tokenWaiters
      tokenWaiters = []
      waiters.forEach((resolve) => resolve(value))
    },
    /** Flush pending edits now — the host calls this before it closes the
     *  editor, so the last keystrokes are never the ones that get lost. */
    save: () => flushSave(),
    reload,
    /**
     * Pours a converted office file into the (empty) document this page was
     * opened on — §8. The host has already uploaded the file and created the
     * document; what it cannot do is turn the returned HTML into blocks.
     *
     * It goes through the same normalize→parse pair the web import uses, so a
     * .docx opened on a phone loses exactly as little as one opened in a
     * browser, and is then saved by the ordinary autosave rather than by a
     * second write path.
     */
    applyImport(raw) {
      const payload = parseImportPayload(raw)
      if (!payload || !doc.value?.id) return false
      // A PDF was stored, not converted: there is no HTML to parse, and the
      // body is the single block that points at the file.
      const json = payload.pdf
        ? pdfDocument(payload.pdf)
        : withImportedPage(htmlToDoc(normalizeOfficeHtml(payload.html)), payload.page)
      content.value = json
      comments.setDoc(json)
      scheduleSave(json)
      return true
    },
    setTheme(dark) {
      theme.setThemeMode(dark ? 'dark' : 'light')
    },
  }

  await load()
})

onBeforeUnmount(() => {
  closeRoom()
  comments.close()
  setRefreshHook(null)
  delete window.tesseraEmbed
})
</script>

<template>
  <!-- No config/message provider of our own: App.vue already wraps every route
       in both, and this page is a route like any other. A second provider here
       would give the editor a theme that no longer follows the store. -->
  <div class="doc-embed" data-testid="doc-embed">
    <div v-if="loading" class="embed-note">{{ $t('documents.embed.loading') }}</div>
    <div v-else-if="fatal" class="embed-note" data-testid="doc-embed-error">
      {{ $t('documents.embed.failed') }}
    </div>
    <doc-editor
      v-else
      :model-value="content"
      :upload-image="uploadImage"
      :upload-pdf="uploadPdf"
      :locks="paintedLocks"
      :comments="comments.openCounts.value"
      class="embed-editor"
      @change="onEditorChange"
      @block-focus="claimBlock"
      @blocked="onBlocked"
      @annotate="onAnnotate"
      @select-comments="onSelectComments"
    />
  </div>
</template>

<style scoped>
/* Full viewport and nothing else. No max-width sheet: the phone *is* the sheet,
   and the page geometry the document carries is drawn by the editor itself. */
.doc-embed {
  display: flex;
  flex-direction: column;
  min-height: 100dvh;
  padding: 0 8px env(safe-area-inset-bottom);
  background: var(--t-bg);
  color: var(--t-text1);
}
.embed-editor {
  flex: 1;
  min-height: 0;
}
.embed-note {
  padding: 24px 8px;
  color: var(--t-text3);
  font-size: 13px;
  text-align: center;
}
</style>
