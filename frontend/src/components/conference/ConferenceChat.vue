<script setup>
// The chat rail of a call (#2864, subtask #2873).
//
// Two things about it are not obvious from the markup.
//
// First, the transport. Messages travel over HTTP; the room socket only says
// "the chat changed" (`chatNudge`), and this component answers that by fetching
// the tail. It reads that way because the room's broadcast contract evicts a
// participant whose buffer overflows — chat bodies on that socket would let a
// lively conversation disconnect the people having it. The nudge is idempotent,
// so one lost to a reconnect costs a stale rail until the next message, not a
// message that never appears.
//
// Second, the pictures. An attachment is fetched with our bearer credential and
// rendered from a blob, never through an `<img src>` pointing at the API. An
// <img> can carry neither our header nor a cookie the desktop client has, and
// the alternative — putting call attachments behind nothing but an unguessable
// URL, the way inline media works — is a weaker guarantee than a private
// meeting deserves. Blob URLs are revoked on unmount; leaking them would pin
// every screenshot of every call in memory for the life of the tab.
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NIcon, NInput, NModal, NPopconfirm, NSpin, NTooltip, useMessage } from 'naive-ui'
import {
  AttachOutline,
  CloseOutline,
  DownloadOutline,
  DocumentOutline,
  SendOutline,
  TrashOutline,
} from '@vicons/ionicons5'
import { conferences as confApi } from '@/api'
import { useAuthStore } from '@/stores/auth'
import { useFormat } from '@/composables/useFormat'
import {
  CONF_CHAT_MAX_BODY,
  CONF_CHAT_MAX_FILES,
  CONF_CHAT_MAX_TOTAL_BYTES,
  confChatBodyTooLong,
  confChatCanSend,
  confChatPickError,
  mergeChatTail,
} from '@/utils/confChat'
import { handOffBlob } from '@/utils/blobHandoff'
import { formatFileSize } from '@/utils/docPdf'
import UserAvatar from '@/components/UserAvatar.vue'

const props = defineProps({
  conferenceId: { type: String, required: true },
  // Bumped by the room socket when someone else says something.
  nudge: { type: Number, default: 0 },
  // An ended call keeps its chat readable — it is the closest thing the meeting
  // has to a protocol — but the composer goes away.
  readonly: { type: Boolean, default: false },
  // Hosts and workspace admins may remove anyone's line; everyone may remove
  // their own. Mirrors the server's check rather than replacing it.
  canModerate: { type: Boolean, default: false },
  // Whether the rail is actually showing this half. The parent keeps the chat
  // alive behind `v-show` so switching tabs does not refetch the history — but
  // that means the first load lands on a display:none element, where scrollTop
  // cannot move and "scroll to the newest" silently does nothing. Without this
  // the chat opens at the oldest message every time.
  visible: { type: Boolean, default: true },
})

const { t } = useI18n()
const auth = useAuthStore()
const message = useMessage()
const { formatDateTime } = useFormat()

const PAGE = 50
// The composer's four caps are the server's, restated in `@/utils/confChat` —
// checked here too so a 30 MB video fails instantly instead of after a minute of
// uploading, and so an over-long body is refused before the press rather than by
// a 400 whose message is not even translated.
const MAX_FILES = CONF_CHAT_MAX_FILES

const messages = ref([])
const hasMore = ref(false)
const loading = ref(false)
const sending = ref(false)
const draft = ref('')
const pending = ref([])
const fileInput = ref(null)
const scroller = ref(null)
// Attachment id → object URL. One cache for the whole rail, so scrolling a
// picture out of view and back does not refetch it.
const previews = ref({})
const lightbox = ref(null)

const me = computed(() => auth.user?.id || '')

/** Whether the rail is scrolled to (or near) the bottom. */
function atBottom() {
  const el = scroller.value
  if (!el) return true
  return el.scrollHeight - el.scrollTop - el.clientHeight < 60
}

/**
 * Scroll to the newest message.
 *
 * Only when the reader is already at the bottom: yanking the view down while
 * someone is reading back through the history is how a chat becomes unusable in
 * a busy call.
 */
async function scrollToEnd(force = false) {
  if (!force && !atBottom()) return
  await nextTick()
  const el = scroller.value
  if (el) el.scrollTop = el.scrollHeight
}

/**
 * Whether an answer asked of `id` is still about the call on screen.
 *
 * The rail is reused across calls — `ConferenceRoom` carries no `:key`, only a
 * changing `conferenceId` — so the watcher at the bottom of this file clears the
 * log on a switch. That is half an invalidation: the request already in flight
 * belongs to the call the reader has left, and nothing stopped its answer from
 * landing afterwards. `mergeChatTail` then folds the previous call's page into
 * this one — and since the page it replaces is the newer conversation, the rail
 * ends up showing the old call's messages and nothing else, under the new call's
 * header, until something else refetches. Android has compared the id back after
 * the await since the rail was written (`ConferenceChatViewModel`); the web side
 * had only the clearing half (#2932).
 */
function stale(id) {
  return id !== props.conferenceId
}

/**
 * Loads the newest page and folds it into what is on screen.
 *
 * Folds rather than replaces: a reader who paged back keeps their history when
 * the next nudge arrives — see `mergeChatTail`. `has_more` describes what is
 * older than the fetched page, so it only answers for the rail while the rail
 * holds nothing older itself; once history is loaded, the flag the last
 * "load older" left behind is the true one.
 */
async function loadTail() {
  const id = props.conferenceId
  if (!id) return
  loading.value = true
  try {
    const { data } = await confApi.messages(id, { limit: PAGE })
    if (stale(id)) return
    const stick = atBottom()
    const fresh = data?.messages || []
    const merged = mergeChatTail(messages.value, fresh)
    messages.value = merged
    if (merged.length === fresh.length) hasMore.value = !!data?.has_more
    await scrollToEnd(stick)
  } catch (e) {
    if (stale(id)) return
    // A 403/404 here means the conference went away under us; the screen around
    // this rail already says so, and a second toast would only repeat it.
    // `e.status`, not `e.response.status`: the api client rejects with an error
    // of its own and carries the status there — reading `.response` matched
    // nothing, so the toast this line exists to suppress was shown anyway.
    if (![403, 404].includes(e.status)) message.error(e.message)
  } finally {
    if (!stale(id)) loading.value = false
  }
}

/** Prepends the page before the oldest message on screen. */
async function loadOlder() {
  const oldest = messages.value[0]
  if (!oldest || loading.value) return
  const id = props.conferenceId
  loading.value = true
  try {
    const { data } = await confApi.messages(id, {
      limit: PAGE,
      before_at: oldest.created_at,
      before_id: oldest.id,
    })
    if (stale(id)) return
    const older = data?.messages || []
    // Anchor the view on the message that was at the top, so prepending history
    // does not teleport the reader.
    const el = scroller.value
    const before = el ? el.scrollHeight - el.scrollTop : 0
    messages.value = [...older, ...messages.value]
    hasMore.value = !!data?.has_more
    await nextTick()
    if (el) el.scrollTop = el.scrollHeight - before
  } catch (e) {
    if (stale(id)) return
    message.error(e.message)
  } finally {
    if (!stale(id)) loading.value = false
  }
}

// ── composer ───────────────────────────────────────────────────────────
function pickFiles() {
  fileInput.value?.click()
}

function onFiles(e) {
  const picked = Array.from(e.target.files || [])
  // Reset the input straight away: without it, picking the same file twice in a
  // row fires no change event the second time.
  e.target.value = ''
  for (const f of picked) {
    // The count refusal stops the loop (nothing after it can fit either); the
    // two size ones skip just that file, because a smaller one behind it still
    // can. Warned once per file rather than once per pick — the user picked
    // them one by one and the name is the only way to tell which was dropped.
    const reason = confChatPickError(pending.value, f)
    if (reason === 'tooMany') {
      message.warning(t('conferences.chat.tooManyFiles', { count: MAX_FILES }))
      break
    }
    if (reason === 'tooLarge') {
      message.warning(t('conferences.chat.fileTooBig', { name: f.name }))
      continue
    }
    if (reason === 'tooLargeTotal') {
      message.warning(
        t('conferences.chat.filesTooBig', { limit: formatFileSize(CONF_CHAT_MAX_TOTAL_BYTES) }),
      )
      continue
    }
    pending.value.push(f)
  }
}

function dropPending(i) {
  pending.value.splice(i, 1)
}

// Said before the press rather than after it: the server's answer to an
// over-long message is a 400, and by then the text has already been typed.
const tooLong = computed(() => confChatBodyTooLong(draft.value))

const canSend = computed(() => confChatCanSend(draft.value, pending.value.length, sending.value))

async function send() {
  if (!canSend.value) return
  const id = props.conferenceId
  sending.value = true
  const body = draft.value.trim()
  const files = pending.value
  try {
    const { data } = files.length
      ? await confApi.postMessageWithFiles(id, body, files)
      : await confApi.postMessage(id, body)
    // Sent to the call we have since left: it is stored and the people still in
    // that room will see it, but appending it here would put it in a conversation
    // it was never part of.
    if (stale(id)) return
    draft.value = ''
    pending.value = []
    // Append our own message rather than refetching: the server answers with the
    // stored row, and the round trip we just made is the freshest thing there is.
    if (data?.id && !messages.value.some((m) => m.id === data.id)) messages.value.push(data)
    await scrollToEnd(true)
  } catch (e) {
    message.error(e.message)
  } finally {
    sending.value = false
  }
}

async function remove(msg) {
  try {
    await confApi.removeMessage(msg.id)
    messages.value = messages.value.filter((m) => m.id !== msg.id)
  } catch (e) {
    message.error(e.message)
  }
}

function canRemove(msg) {
  return props.canModerate || (!!msg.user_id && msg.user_id === me.value)
}

// ── attachments ────────────────────────────────────────────────────────
/**
 * Fetches an image attachment with our credential and caches the object URL.
 *
 * Called from the template's :src through a ref lookup rather than awaited
 * inline: the first paint shows a placeholder and the picture replaces it, which
 * is what an `<img>` would do anyway.
 */
async function loadPreview(att) {
  if (!att.is_image || previews.value[att.id]) return
  // Reserve the slot before awaiting, so a second render of the same message
  // (which is one nudge away) does not start a second download.
  previews.value[att.id] = ''
  try {
    const { data } = await confApi.attachment(att.id)
    previews.value[att.id] = URL.createObjectURL(data)
  } catch {
    delete previews.value[att.id]
  }
}

/** Downloads an attachment under its original name. */
async function download(att) {
  try {
    const { data } = await confApi.attachment(att.id)
    // handOffBlob owns the click-then-revoke-late dance — revoking before the
    // browser has started the download cancels it — and the desktop Save-as it
    // takes instead of that dance, which is why this is awaited.
    await handOffBlob(data, att.filename)
  } catch (e) {
    message.error(e.message)
  }
}

function when(ts) {
  return formatDateTime(ts, { hour: '2-digit', minute: '2-digit' })
}

// Every image on screen gets fetched once, as messages arrive.
watch(
  messages,
  (list) => {
    for (const m of list) for (const a of m.attachments || []) loadPreview(a)
  },
  { deep: true, immediate: true },
)

// A different call is a different conversation: drop the log before fetching,
// or the fold above would carry the previous room's history into it.
watch(
  () => props.conferenceId,
  () => {
    messages.value = []
    hasMore.value = false
    // The request in flight no longer speaks for this rail and will not clear the
    // spinner itself (see `stale`); the fetch below turns it back on. Without
    // this, leaving for a call with no id at all — the room unmounts nothing —
    // would leave the rail spinning with nothing on the way.
    loading.value = false
    loadTail()
  },
  { immediate: true },
)
// Someone said something. The nudge carries no payload by design, so the answer
// is always "go and read the tail".
watch(
  () => props.nudge,
  (n) => {
    if (n) loadTail()
  },
)

// Opening the rail is the first moment the log has a height to scroll inside —
// see the `visible` prop. Forced, because a hidden element always measures as
// "at the bottom" and the polite check would decline to move.
watch(
  () => props.visible,
  (shown) => {
    if (shown) scrollToEnd(true)
  },
  { immediate: true },
)

onBeforeUnmount(() => {
  for (const url of Object.values(previews.value)) if (url) URL.revokeObjectURL(url)
})
</script>

<template>
  <div class="chat" data-testid="conference-chat">
    <div ref="scroller" class="log" data-testid="conference-chat-log">
      <n-button
        v-if="hasMore"
        text
        size="tiny"
        class="more"
        :loading="loading"
        data-testid="conference-chat-older"
        @click="loadOlder"
      >
        {{ $t('conferences.chat.loadOlder') }}
      </n-button>

      <div v-if="!messages.length && !loading" class="none">
        {{ $t('conferences.chat.empty') }}
      </div>

      <div v-for="m in messages" :key="m.id" class="msg" data-testid="conference-chat-message">
        <user-avatar :user-id="m.user_id || ''" :name="m.user_name || '?'" class="av" />
        <div class="msg-body">
          <div class="msg-head">
            <span class="who">{{ m.user_name || $t('conferences.chat.unknownAuthor') }}</span>
            <span class="at">{{ when(m.created_at) }}</span>
            <!-- Behind a confirmation like every other destructive action in a
                 call (recording, kick, end) and like the same bin on Android:
                 the server drops the line for everyone in the room and nudges
                 them to refetch, so a misclick on a 12px icon is not undoable. -->
            <n-popconfirm
              v-if="canRemove(m)"
              :positive-button-props="{ type: 'error' }"
              :positive-text="$t('conferences.chat.delete')"
              @positive-click="remove(m)"
            >
              <template #trigger>
                <n-button
                  text
                  size="tiny"
                  class="del"
                  :title="$t('conferences.chat.delete')"
                  data-testid="conference-chat-delete"
                >
                  <n-icon :component="TrashOutline" />
                </n-button>
              </template>
              {{ $t('conferences.chat.confirmDelete') }}
            </n-popconfirm>
          </div>
          <div v-if="m.body" class="text">{{ m.body }}</div>

          <div v-if="m.attachments?.length" class="atts">
            <template v-for="a in m.attachments" :key="a.id">
              <!-- A picture opens over the call rather than in a new tab: the
                   whole point is not having to leave the room to look at it. -->
              <button
                v-if="a.is_image"
                type="button"
                class="thumb"
                data-testid="conference-chat-image"
                @click="lightbox = a"
              >
                <img v-if="previews[a.id]" :src="previews[a.id]" :alt="a.filename" />
                <span v-else class="thumb-wait"><n-spin :size="14" /></span>
              </button>
              <n-tooltip v-else>
                <template #trigger>
                  <button
                    type="button"
                    class="file"
                    data-testid="conference-chat-file"
                    @click="download(a)"
                  >
                    <n-icon :component="DocumentOutline" :size="14" />
                    <span class="fname">{{ a.filename }}</span>
                    <span class="fsize">{{ formatFileSize(a.size) }}</span>
                  </button>
                </template>
                {{ $t('conferences.chat.download') }}
              </n-tooltip>
            </template>
          </div>
        </div>
      </div>
    </div>

    <div v-if="readonly" class="none ro">{{ $t('conferences.chat.closed') }}</div>
    <div v-else class="composer">
      <div v-if="pending.length" class="pending">
        <span v-for="(f, i) in pending" :key="i" class="chip" data-testid="conference-chat-pending">
          <span class="fname">{{ f.name }}</span>
          <n-button text size="tiny" @click="dropPending(i)">
            <n-icon :component="CloseOutline" />
          </n-button>
        </span>
      </div>
      <div v-if="tooLong" class="too-long" data-testid="conference-chat-too-long">
        {{ $t('conferences.chat.tooLong', { count: CONF_CHAT_MAX_BODY }) }}
      </div>
      <div class="row">
        <!-- Enter sends, Shift+Enter breaks the line: this is a chat, not the
             description editor. -->
        <n-input
          v-model:value="draft"
          type="textarea"
          :autosize="{ minRows: 1, maxRows: 4 }"
          :placeholder="$t('conferences.chat.placeholder')"
          data-testid="conference-chat-input"
          @keydown.enter.exact.prevent="send"
        />
        <n-button
          quaternary
          circle
          :title="$t('conferences.chat.attach')"
          data-testid="conference-chat-attach"
          @click="pickFiles"
        >
          <n-icon :component="AttachOutline" />
        </n-button>
        <n-button
          type="primary"
          circle
          :disabled="!canSend"
          :loading="sending"
          data-testid="conference-chat-send"
          @click="send"
        >
          <n-icon :component="SendOutline" />
        </n-button>
        <input ref="fileInput" type="file" multiple class="hidden-input" @change="onFiles" />
      </div>
    </div>

    <!-- The overlay stays inside the call: leaving the room to look at a
         screenshot is exactly what the task asked us not to make people do. -->
    <n-modal
      :show="!!lightbox"
      preset="card"
      style="max-width: 90vw; width: fit-content"
      :bordered="false"
      :title="lightbox?.filename"
      data-testid="conference-chat-lightbox"
      @update:show="(v) => !v && (lightbox = null)"
    >
      <img v-if="lightbox && previews[lightbox.id]" class="full" :src="previews[lightbox.id]" />
      <template #footer>
        <n-button size="small" @click="download(lightbox)">
          <template #icon><n-icon :component="DownloadOutline" /></template>
          {{ $t('conferences.chat.download') }}
        </n-button>
      </template>
    </n-modal>
  </div>
</template>

<style scoped>
.chat {
  display: flex;
  flex-direction: column;
  gap: 8px;
  min-height: 0;
}
/* The log scrolls, the composer does not: a rail whose input walks off the
   bottom as the conversation grows is unusable in a call. */
.log {
  flex: 1;
  min-height: 120px;
  max-height: 46vh;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding-right: 4px;
}
.more {
  align-self: center;
}
.none {
  font-size: 12px;
  color: var(--t-text3);
  text-align: center;
  padding: 12px 0;
}
.none.ro {
  padding: 6px 0 0;
}
.msg {
  display: flex;
  gap: 8px;
  align-items: flex-start;
}
/* UserAvatar takes its size from the caller's class, as everywhere else. */
.av {
  flex: none;
  width: 24px;
  height: 24px;
  border-radius: 50%;
  font-size: 11px;
  color: #fff;
  background: var(--t-accent-grad);
}
.msg-body {
  min-width: 0;
  flex: 1;
}
.msg-head {
  display: flex;
  align-items: baseline;
  gap: 6px;
  font-size: 11px;
  color: var(--t-text3);
}
.who {
  color: var(--t-text2);
  font-weight: 500;
}
/* The delete button only appears on hover, so a rail full of messages is not
   also a rail full of bins. */
.del {
  margin-left: auto;
  opacity: 0;
  transition: opacity 0.12s;
}
.msg:hover .del {
  opacity: 1;
}
.text {
  font-size: 13px;
  color: var(--t-text1);
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
.atts {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 4px;
}
.thumb {
  padding: 0;
  border: 0;
  border-radius: 10px;
  overflow: hidden;
  background: var(--t-hover);
  cursor: pointer;
  line-height: 0;
}
.thumb img {
  display: block;
  max-width: 160px;
  max-height: 120px;
  object-fit: cover;
}
.thumb-wait {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 80px;
  height: 60px;
}
.file {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  max-width: 100%;
  padding: 4px 8px;
  font-size: 12px;
  color: var(--t-text2);
  background: var(--t-ctl);
  border: 0;
  border-radius: 8px;
  cursor: pointer;
}
.fname {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 150px;
}
.fsize {
  color: var(--t-text3);
  flex: none;
}
.composer {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.pending {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.chip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 2px 6px;
  font-size: 11px;
  color: var(--t-text2);
  background: var(--t-hover);
  border-radius: 6px;
}
/* The error red is written out rather than read from a token: there is no
   --t-error, only the --t-error-grad this hue is built from, and a var() naming
   a token that does not exist would just fall through to the inherited colour. */
.too-long {
  font-size: 11px;
  color: #e5484d;
}
.row {
  display: flex;
  align-items: flex-end;
  gap: 6px;
}
.hidden-input {
  display: none;
}
.full {
  display: block;
  max-width: 86vw;
  max-height: 72vh;
  object-fit: contain;
}
</style>
