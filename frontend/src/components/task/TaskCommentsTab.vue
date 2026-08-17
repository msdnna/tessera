<script setup>
// «Комментарии» tab of the task modal: the thread, the composer with @-mentions and
// /-commands, the command dry-run hint and the offline-retry banner.
//
// The thread itself is owned by the modal (its count sits on the tab label and it is
// fetched alongside relations/files/journal), so edits are published back through
// `update:comments`. Two things stay the modal's job and are asked for by event:
// re-fetching the task after a quick action changed it (`reload-detail`) and telling
// the board something happened (`changed`).
//
// `hydrated` gates the enter fade to genuinely new comments — the modal flips it once
// the initial population has rendered, so a re-opened task doesn't fade in its whole
// history.
import { ref, computed, watch, nextTick, onBeforeUnmount } from 'vue'
import { NButton, NSpace, NPopconfirm, useMessage } from 'naive-ui'
import { ChatbubbleEllipsesOutline } from '@vicons/ionicons5'
import { tasks as tasksApi } from '@/api'
import { useAuthStore } from '@/stores/auth'
import { useBoardViewStore } from '@/stores/boardView'
import { useWorkspacesStore } from '@/stores/workspaces'
import { fmtWhen, groupThreads } from '@/utils/taskFeed'
import { toggleTaskMarker } from '@/utils/markdown'
import { hasCommandLine } from '@/utils/commands'
import { scrollParent } from '@/utils/dom'
import MarkdownEditor from '../MarkdownEditor.vue'
import RichContent from '../RichContent.vue'
import UserAvatar from '../UserAvatar.vue'
import TesseraSpinner from '../TesseraSpinner.vue'
import EmptyState from '../EmptyState.vue'

const props = defineProps({
  taskId: { type: String, default: null },
  comments: { type: Array, default: () => [] },
  readonly: { type: Boolean, default: false },
  hydrated: { type: Boolean, default: false },
})
const emit = defineEmits(['update:comments', 'changed', 'reload-detail'])

const message = useMessage()
const auth = useAuthStore()
const bv = useBoardViewStore()
const store = useWorkspacesStore()

const meId = computed(() => auth.user?.id)
const commentsPane = ref(null) // the .comments container (narrow-layout scroll fallback)
const commentsListEl = ref(null) // the .c-list — the internal scroller in wide layout
const newComment = ref('')
const commentEditor = ref(null)
const editingCommentId = ref(null)
const editingCommentBody = ref('')
// True while a comment POST is in flight (spinner on the send button, guards
// double-submit); retryInfo holds { attempt, max } while waiting to retry an
// offline send, else null.
const posting = ref(false)
const retryInfo = ref(null)

// The flat list from the API, assembled into "root + its replies" threads.
const threads = computed(() => groupThreads(props.comments))
// id of the thread root whose reply composer is open, and its draft.
const replyingTo = ref(null)
const replyBody = ref('')
const replyEditor = ref(null)
const replyingPost = ref(false)
// Thread roots whose replies are collapsed. Local-only UI state: threads start
// expanded, and a collapsed one says how many replies it hides.
const collapsed = ref(new Set())

function toggleCollapsed(rootId) {
  const next = new Set(collapsed.value)
  if (next.has(rootId)) next.delete(rootId)
  else next.add(rootId)
  collapsed.value = next // reassign — a mutated Set is not reactive
}

// Scroll to a freshly-arrived comment whoever wrote it. Own sends already scroll
// (postComment pins to the bottom, postReply to the reply), but a comment that
// appears by any other route — a realtime/refetch update from another user —
// would otherwise land off-screen. Gated by `hydrated` so the initial population
// (empty → full on open) doesn't trigger it; the modal handles the open scroll.
watch(
  () => props.comments,
  (next, prev) => {
    if (!props.hydrated) return
    const before = new Set((prev || []).map((c) => c.id))
    const added = (next || []).filter((c) => !before.has(c.id))
    if (!added.length) return
    // The newest by arrival is the last in thread order isn't guaranteed (a reply
    // sorts under its root), so pick the max by created_at.
    const target = added.reduce((a, b) => (a.created_at >= b.created_at ? a : b))
    scrollToComment(target.id)
  },
)

// The @-handle that renderRich/GitLab will resolve for a comment's author:
// a Tessera member mentions by display name, a GitLab-only author by the
// @username we can recover from the members list (its display name won't
// resolve), else the raw name as a courtesy.
function mentionHandle(c) {
  if (!c) return ''
  if (c.author_name) return c.author_name
  const glName = c.gl_author_name
  if (!glName) return ''
  const g = bv.gitlabMembersList.find((m) => (m.gl_name || m.gl_username) === glName)
  return g?.gl_username || glName
}

// Open the reply composer for a thread. `target` is the comment being answered
// (root or a reply) — its author is pre-mentioned so the reply reads as a reply.
async function startReply(rootId, target) {
  replyingTo.value = rootId
  const handle = mentionHandle(target)
  replyBody.value = handle ? `@${handle}, ` : ''
  // Answering a collapsed thread with the answers hidden would be writing blind.
  if (collapsed.value.has(rootId)) toggleCollapsed(rootId)
  await nextTick()
  replyEditor.value?.focus?.()
  // Reveal the just-opened composer: it lands mid-list (under its thread) and the
  // sticky footer floats over the bottom of the pane, so plain focus/centre can leave
  // it behind that footer. Two frames of grace first — the boxed editor (toolbar +
  // auto-grown textarea) is still short at nextTick, and measuring then makes
  // revealInPane think the form already fits and skip the scroll.
  requestAnimationFrame(() =>
    requestAnimationFrame(() => revealInPane(commentsListEl.value?.querySelector('.c-reply-add'))),
  )
}

// Scroll an element into view within the comments scroller, kept clear of the
// composer that is `position: sticky; bottom: 0` and floats over the bottom of the
// pane. Done by explicit maths on the resolved scroller rather than scrollIntoView:
// the sticky footer isn't part of scrollIntoView's model (it would park the element
// underneath), and a freshly-mounted reply form races its smooth animation.
function revealInPane(el) {
  if (!el) return
  const sp = scrollParent(el)
  if (!sp) return
  const composer = commentsPane.value?.querySelector('.comment-add')
  const compH = composer ? composer.getBoundingClientRect().height : 0
  const spRect = sp.getBoundingClientRect()
  const r = el.getBoundingClientRect()
  const top = spRect.top + 8 // small breathing room at the pane top
  const bottom = spRect.bottom - compH - 12 // reserve the sticky-composer band
  let delta = 0
  if (r.height > bottom - top || r.top < top)
    delta = r.top - top // taller than the band, or above the fold → show its top
  else if (r.bottom > bottom) delta = r.bottom - bottom // below the fold → lift above the composer
  if (delta) sp.scrollBy({ top: delta, behavior: 'smooth' })
}

function cancelReply() {
  replyingTo.value = null
  replyBody.value = ''
}

function replyCount(t) {
  const n = t.replies.length
  const forms =
    n % 10 === 1 && n % 100 !== 11
      ? 'ответ'
      : n % 10 >= 2 && n % 10 <= 4 && (n % 100 < 10 || n % 100 >= 20)
        ? 'ответа'
        : 'ответов'
  return `${n} ${forms}`
}

// Members offered for @-mentions. Tessera members insert their display name;
// GitLab-only users (no Tessera account) insert their `@username` so GitLab resolves
// the mention on writeback. `label` is the inserted text, `display` the row. The
// store's gitlabMembersList already drops GitLab users mapped to a Tessera member,
// so nobody shows up twice.
const mentionItems = computed(() => [
  ...bv.membersList.map((m) => ({
    id: m.user_id,
    label: m.name,
    display: m.name,
    avatarUserId: m.user_id,
  })),
  ...bv.gitlabMembersList.map((g) => ({
    id: null,
    label: g.gl_username,
    display: g.gl_name || g.gl_username,
    avatarSrc: g.gl_avatar_url,
    gitlab: true,
  })),
])

// ── quick actions ──
// The command registry is workspace-wide (loaded once by the store); the popup
// is only offered where commands actually run — the new-comment composer.
// Editing an existing comment and the description do not execute them.
const commandItems = computed(() => store.commands || [])
const cmdPreview = ref([]) // [{ key, summary, error }] from the dry-run
const cmdCustom = ref([]) // custom keys seen in the draft — kept as plain text
let cmdTimer = null
// Dry-run the draft against the backend's parser (debounced) instead of
// re-implementing it here: the hint can never disagree with what will happen.
watch(newComment, (body) => {
  clearTimeout(cmdTimer)
  if (!hasCommandLine(body)) {
    cmdPreview.value = []
    cmdCustom.value = []
    return
  }
  cmdTimer = setTimeout(async () => {
    try {
      const res = await tasksApi.previewCommands(props.taskId, body)
      cmdPreview.value = res.data?.commands || []
      cmdCustom.value = res.data?.custom || []
    } catch {
      cmdPreview.value = []
      cmdCustom.value = []
    }
  }, 400)
})
onBeforeUnmount(() => {
  clearTimeout(cmdTimer)
  cancelRetry()
})

// Scroll the comments to the bottom (the newest message) so a freshly-sent comment
// isn't hidden behind the composer and a re-opened task lands on the latest activity.
// Pins to the bottom across several frames until the height stops changing — late
// reflow (web-font swap, async markdown/avatars) otherwise strands a long thread
// mid-way. Instant, not smooth: a smooth animation races that same reflow and stops
// short. The scroller is .c-list in the wide layout, the modal body in the stacked
// one (resolved via scrollParent; null → nothing scrolls, no-op).
async function scrollToBottom() {
  await nextTick()
  const list = commentsListEl.value
  const sp = list && list.scrollHeight > list.clientHeight ? list : scrollParent(commentsPane.value)
  if (!sp) return
  let last = -1
  let stable = 0
  let frames = 0
  const pin = () => {
    sp.scrollTop = sp.scrollHeight
    if (sp.scrollHeight === last) stable += 1
    else stable = 0
    last = sp.scrollHeight
    // Stop once the height has held for a few frames, or after a hard cap (~0.5s).
    if (stable < 3 && ++frames < 30) requestAnimationFrame(pin)
  }
  pin()
}
// The composer's textarea can't measure its height while the right panel is
// collapsed (0-width) — the modal asks for a remeasure once it is visible again.
function autoGrow() {
  commentEditor.value?.autoGrow?.()
}
defineExpose({ scrollToBottom, autoGrow })

// ── offline retry for a comment POST ──
// A network error (server unreachable — err.offline) is retried a few times with
// a growing backoff; an HTTP error is not (the server answered — resending won't
// help). The draft is preserved throughout, and a banner under the composer shows
// progress with a Cancel that aborts the wait.
const RETRY_BACKOFFS = [2000, 5000, 10000]
let retryTimer = null
let retryResolve = null
// Wait `ms`, resolving true on timeout or false if cancelRetry() fires first.
function waitOrCancel(ms) {
  return new Promise((resolve) => {
    retryResolve = resolve
    retryTimer = setTimeout(() => {
      retryTimer = null
      retryResolve = null
      resolve(true)
    }, ms)
  })
}
function cancelRetry() {
  if (retryTimer) {
    clearTimeout(retryTimer)
    retryTimer = null
  }
  if (retryResolve) {
    const r = retryResolve
    retryResolve = null
    r(false)
  }
  retryInfo.value = null
}

async function postComment() {
  const body = newComment.value.trim()
  if (!body || posting.value) return
  const mentions = commentEditor.value?.getMentions?.() || []
  posting.value = true
  try {
    // attempt 0 is the initial send; 1..N are retries, each preceded by its backoff.
    for (let attempt = 0; attempt <= RETRY_BACKOFFS.length; attempt++) {
      try {
        const res = await tasksApi.addComment(props.taskId, body, mentions)
        retryInfo.value = null
        newComment.value = ''
        commentEditor.value?.clear?.()
        cmdPreview.value = []
        const c = await tasksApi.comments(props.taskId)
        emit('update:comments', c.data || [])
        // Quick actions changed the task itself (assignees, column, dates…), and a
        // command-only comment produces no comment row at all — reload the detail so
        // the modal shows the result rather than the pre-command state.
        const summary = res.data?.command_summary
        if (summary?.applied?.length || summary?.errors?.length) {
          emit('reload-detail')
          reportCommands(summary)
        }
        emit('changed')
        scrollToBottom()
        return
      } catch (e) {
        // Only "server unreachable" is worth retrying; an HTTP error (4xx/5xx) is
        // the server rejecting the comment — resending won't change that.
        const backoff = RETRY_BACKOFFS[attempt]
        if (!e.offline || backoff == null) {
          retryInfo.value = null
          message.error(e.offline ? 'Сервер недоступен — комментарий не отправлен' : e.message)
          return
        }
        retryInfo.value = { attempt: attempt + 1, max: RETRY_BACKOFFS.length }
        if (!(await waitOrCancel(backoff))) return // cancelled by the user
      }
    }
  } finally {
    posting.value = false
    retryInfo.value = null
  }
}

// Post a reply into an existing thread. Deliberately not folded into
// postComment: the offline-retry loop and the command dry-run belong to the main
// composer, and a reply lands mid-list, so scrolling to the bottom (what
// postComment does) would take the screen past what was just written.
async function postReply(rootId) {
  const body = replyBody.value.trim()
  if (!body || replyingPost.value) return
  const mentions = replyEditor.value?.getMentions?.() || []
  const known = new Set((props.comments || []).map((c) => c.id))
  replyingPost.value = true
  try {
    const res = await tasksApi.addComment(props.taskId, body, mentions, rootId)
    cancelReply()
    const c = await tasksApi.comments(props.taskId)
    emit('update:comments', c.data || [])
    const summary = res.data?.command_summary
    if (summary?.applied?.length || summary?.errors?.length) {
      emit('reload-detail')
      reportCommands(summary)
    }
    emit('changed')
    // Scroll to the reply itself, not to the end of the list.
    const fresh = (c.data || []).find((x) => !known.has(x.id))
    if (fresh) await scrollToComment(fresh.id)
  } catch (e) {
    message.error(e.offline ? 'Сервер недоступен — ответ не отправлен' : e.message)
  } finally {
    replyingPost.value = false
  }
}

async function scrollToComment(id) {
  await nextTick()
  revealInPane(commentsListEl.value?.querySelector(`[data-comment-id="${id}"]`))
}

// Report what the backend actually did — intent and result can differ (a
// recurring task bounces straight out of the done column), so we echo its text.
function reportCommands(summary) {
  const applied = (summary.applied || []).map((o) => o.summary || `/${o.key}`)
  if (applied.length) message.success(`Применено: ${applied.join('; ')}`)
  for (const err of summary.errors || []) {
    message.warning(`/${err.key}: ${err.error}`)
  }
}
function startEditComment(c) {
  editingCommentId.value = c.id
  editingCommentBody.value = c.body
}
async function saveComment() {
  if (props.readonly) return
  const body = editingCommentBody.value.trim()
  if (!body) return
  try {
    await tasksApi.updateComment(editingCommentId.value, body)
    editingCommentId.value = null
    const c = await tasksApi.comments(props.taskId)
    emit('update:comments', c.data || [])
  } catch (e) {
    message.error(e.message)
  }
}
async function deleteComment(id) {
  try {
    await tasksApi.removeComment(id)
    emit(
      'update:comments',
      props.comments.filter((x) => x.id !== id),
    )
  } catch (e) {
    message.error(e.message)
  }
}
// Toggle a task checkbox inside a rendered (own) comment → rewrite its markdown
// and persist. Optimistic; reverts on failure.
async function onCommentCheck(c, i) {
  const prev = c.body
  const next = toggleTaskMarker(prev, i)
  if (next === prev) return
  c.body = next
  try {
    await tasksApi.updateComment(c.id, next)
  } catch (e) {
    c.body = prev
    message.error(e.message)
  }
}
</script>

<template>
  <div ref="commentsPane" class="comments">
    <div ref="commentsListEl" class="c-list">
      <!-- display:contents wrapper (.c-items) so the comment rows stay
           direct flex children of .c-list; only newly-posted comments
           fade in (no `appear`, so the initial list doesn't animate). -->
      <TransitionGroup :name="hydrated ? 'c-fade' : ''" tag="div" class="c-items">
        <div v-for="t in threads" :key="t.root.id" class="c-thread">
          <div class="comment" :data-comment-id="t.root.id">
            <UserAvatar
              class="c-ava"
              :user-id="t.root.author_id || ''"
              :src="t.root.gl_author_avatar_url"
              :name="t.root.author_name || t.root.gl_author_name || '?'"
            />
            <div class="c-body">
              <div class="c-head">
                <span class="c-author">{{
                  t.root.author_name || t.root.gl_author_name || 'Кто-то'
                }}</span>
                <span v-if="!t.root.author_name && t.root.gl_author_name" class="c-gl">
                  · GitLab
                </span>
                <span class="c-when">{{ fmtWhen(t.root.created_at) }}</span>
                <span v-if="t.root.author_id === meId" class="c-acts">
                  <button class="c-act" title="Изменить" @click="startEditComment(t.root)">
                    ✎
                  </button>
                  <n-popconfirm
                    :positive-button-props="{ type: 'error' }"
                    positive-text="Удалить"
                    @positive-click="deleteComment(t.root.id)"
                  >
                    <template #trigger>
                      <button class="c-act" title="Удалить">✕</button>
                    </template>
                    Удалить комментарий?
                  </n-popconfirm>
                </span>
              </div>
              <template v-if="editingCommentId === t.root.id">
                <MarkdownEditor
                  v-model="editingCommentBody"
                  variant="boxed"
                  :mention-items="mentionItems"
                  :attach-task-id="taskId"
                  :min-rows="2"
                  placeholder="Комментарий…"
                  @attachments-changed="emit('changed')"
                  @submit="saveComment"
                />
                <n-space :size="6" style="margin-top: 6px">
                  <n-button size="tiny" type="primary" @click="saveComment">Сохранить</n-button>
                  <n-button size="tiny" @click="editingCommentId = null">Отмена</n-button>
                </n-space>
              </template>
              <RichContent
                v-else
                class="c-text"
                :source="t.root.body"
                :members="mentionItems"
                task-refs
                :interactive="t.root.author_id === meId"
                @toggle="onCommentCheck(t.root, $event)"
              />
              <div class="c-thread-acts">
                <button v-if="!readonly" class="c-link" @click="startReply(t.root.id, t.root)">
                  Ответить
                </button>
                <button v-if="t.replies.length" class="c-link" @click="toggleCollapsed(t.root.id)">
                  {{ collapsed.has(t.root.id) ? replyCount(t) : 'Свернуть ответы' }}
                </button>
              </div>
            </div>
          </div>

          <!-- Replies: indented under the root and tied to it by the rail. The
               rail is a flat neutral grey — the accent gradient is for
               non-neutral elements only. The grid-rows wrapper animates the
               collapse/expand smoothly (0fr ↔ 1fr). -->
          <Transition name="c-collapse">
            <div v-if="t.replies.length && !collapsed.has(t.root.id)" class="c-collapse">
              <div class="c-replies">
                <div
                  v-for="r in t.replies"
                  :key="r.id"
                  class="comment c-reply"
                  :data-comment-id="r.id"
                >
                  <UserAvatar
                    class="c-ava"
                    :user-id="r.author_id || ''"
                    :src="r.gl_author_avatar_url"
                    :name="r.author_name || r.gl_author_name || '?'"
                  />
                  <div class="c-body">
                    <div class="c-head">
                      <span class="c-author">{{
                        r.author_name || r.gl_author_name || 'Кто-то'
                      }}</span>
                      <span v-if="!r.author_name && r.gl_author_name" class="c-gl">· GitLab</span>
                      <span class="c-when">{{ fmtWhen(r.created_at) }}</span>
                      <span v-if="r.author_id === meId" class="c-acts">
                        <button class="c-act" title="Изменить" @click="startEditComment(r)">
                          ✎
                        </button>
                        <n-popconfirm
                          :positive-button-props="{ type: 'error' }"
                          positive-text="Удалить"
                          @positive-click="deleteComment(r.id)"
                        >
                          <template #trigger>
                            <button class="c-act" title="Удалить">✕</button>
                          </template>
                          Удалить комментарий?
                        </n-popconfirm>
                      </span>
                    </div>
                    <template v-if="editingCommentId === r.id">
                      <MarkdownEditor
                        v-model="editingCommentBody"
                        variant="boxed"
                        :mention-items="mentionItems"
                        :attach-task-id="taskId"
                        :min-rows="2"
                        placeholder="Комментарий…"
                        @attachments-changed="emit('changed')"
                        @submit="saveComment"
                      />
                      <n-space :size="6" style="margin-top: 6px">
                        <n-button size="tiny" type="primary" @click="saveComment"
                          >Сохранить</n-button
                        >
                        <n-button size="tiny" @click="editingCommentId = null">Отмена</n-button>
                      </n-space>
                    </template>
                    <RichContent
                      v-else
                      class="c-text"
                      :source="r.body"
                      :members="mentionItems"
                      task-refs
                      :interactive="r.author_id === meId"
                      @toggle="onCommentCheck(r, $event)"
                    />
                    <div v-if="!readonly" class="c-thread-acts">
                      <!-- Replying to a reply targets the same root: threads are two
                       levels deep, and the backend collapses it anyway. The reply's
                       own author is the one pre-mentioned, though. -->
                      <button class="c-link" @click="startReply(t.root.id, r)">Ответить</button>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </Transition>

          <div v-if="replyingTo === t.root.id && !readonly" class="c-reply-add">
            <!-- Function ref, not a string one: a string ref inside v-for
                 collects into an array, and getMentions() would be undefined.
                 Only one composer is open at a time, so a single slot is right. -->
            <MarkdownEditor
              :ref="(el) => (replyEditor = el)"
              v-model="replyBody"
              variant="boxed"
              send
              :sending="replyingPost"
              :mention-items="mentionItems"
              :command-items="commandItems"
              :attach-task-id="taskId"
              :min-rows="2"
              placeholder="Ответить… (Ctrl+Enter — отправить)"
              @attachments-changed="emit('changed')"
              @submit="postReply(t.root.id)"
            />
            <n-space :size="6" style="margin-top: 6px">
              <n-button
                size="tiny"
                type="primary"
                :loading="replyingPost"
                @click="postReply(t.root.id)"
              >
                Ответить
              </n-button>
              <n-button size="tiny" @click="cancelReply">Отмена</n-button>
            </n-space>
          </div>
        </div>
      </TransitionGroup>
      <EmptyState
        v-if="!comments.length"
        class="c-empty"
        size="small"
        :icon="ChatbubbleEllipsesOutline"
        text="Комментариев пока нет"
      />
    </div>
    <div v-if="!readonly" class="comment-add">
      <MarkdownEditor
        ref="commentEditor"
        v-model="newComment"
        variant="boxed"
        send
        :sending="posting"
        :mention-items="mentionItems"
        :command-items="commandItems"
        :attach-task-id="taskId"
        :min-rows="3"
        placeholder="Написать комментарий… (@ — упоминание, / — команда, Ctrl+Enter — отправить)"
        @attachments-changed="emit('changed')"
        @submit="postComment"
      />
      <!-- Offline-retry banner: shown while waiting to resend after a
           network failure; the draft stays put, Cancel aborts. -->
      <Transition name="retry-pop">
        <div v-if="retryInfo" class="retry-bar">
          <TesseraSpinner :size="14" />
          <span>Нет связи — повтор попытки ({{ retryInfo.attempt }}/{{ retryInfo.max }})…</span>
          <button class="retry-cancel" @click="cancelRetry">Отмена</button>
        </div>
      </Transition>
      <!-- Dry-run hint: what the built-in commands in the draft will
           do. Custom keys are listed apart — they stay in the text. -->
      <div v-if="cmdPreview.length || cmdCustom.length" class="cmd-preview">
        <span v-for="(c, i) in cmdPreview" :key="`${c.key}-${i}`" class="cmd-chip">
          <code>/{{ c.key }}</code>
          <span :class="{ err: c.error }">{{ c.error || c.summary }}</span>
        </span>
        <span v-if="cmdCustom.length" class="cmd-note">
          {{ cmdCustom.map((k) => `/${k}`).join(', ') }} — останется текстом
        </span>
      </div>
    </div>
  </div>
</template>

<style scoped>
@import './tab-shared.css';

.comments {
  display: flex;
  flex-direction: column;
  gap: 12px;
  min-height: 100%;
}
/* Comments list grows so the empty state can sit centred and the composer
   (below) sinks to the bottom of the pane. */
.c-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
  flex: 1 1 auto;
  /* When a comment is appended while scrolled to the bottom, the browser's scroll
     anchoring tries to keep an upper element fixed and nudges scrollTop — which read
     as a "bounce" (the top comment briefly slid under the pinned tabs). We pin to the
     bottom ourselves, so let the anchor go. */
  overflow-anchor: none;
}
.c-empty {
  margin: auto 0;
}
/* Wrapper generates no box — comment rows stay direct flex children of .c-list. */
.c-items {
  display: contents;
}
/* Newly-posted comments ease in — opacity only. A translate would shift layout mid-
   scroll and fight the bottom-pin (the "bounce" where the top row clipped under the
   tabs), so the motion is a pure fade. */
.c-fade-enter-active {
  transition: opacity 0.25s ease;
}
.c-fade-enter-from {
  opacity: 0;
}
@media (prefers-reduced-motion: reduce) {
  .c-fade-enter-active {
    transition: none;
  }
}
.comment {
  display: flex;
  gap: 10px;
}
/* A thread is one flex child of .c-list (which owns the vertical gap between
   threads); the root and its replies are stacked inside it more tightly, so a
   branch reads as one block rather than as loose rows. */
.c-thread {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
/* Replies sit indented behind a rail. The rail is flat neutral grey on purpose:
   the accent gradient belongs to non-neutral elements only. */
.c-replies {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-left: 13px; /* half the 28px avatar → the rail lines up with its centre */
  padding-left: 15px;
  border-left: 2px solid var(--t-border);
}
.c-reply .c-ava {
  width: 22px;
  height: 22px;
  font-size: 10px;
}
.c-reply-add {
  margin-left: 13px;
  padding-left: 15px;
  border-left: 2px solid var(--t-border);
}
.c-thread-acts {
  display: flex;
  gap: 12px;
  margin-top: 2px;
}
.c-link {
  background: none;
  border: 0;
  padding: 0;
  cursor: pointer;
  font-size: 12px;
  color: var(--t-text3);
  transition: color 0.15s ease;
}
.c-link:hover {
  color: var(--t-primary);
}
.c-ava {
  flex: none;
  width: 28px;
  height: 28px;
  border-radius: 50%;
  background: var(--t-accent-grad);
  color: var(--t-on-primary);
  font-size: 11px;
  font-weight: 600;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}
.c-body {
  flex: 1;
  min-width: 0;
}
.c-head {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 2px;
}
.c-author {
  font-weight: 600;
  font-size: 13px;
  color: var(--t-text1);
}
.c-gl {
  font-size: 11px;
  color: var(--t-text3);
}
.c-when {
  font-size: 11px;
  color: var(--t-text3);
}
.c-acts {
  margin-left: auto;
  display: inline-flex;
  gap: 4px;
}
.c-text {
  font-size: 13px;
}
/* Rendered markdown wraps text in <p>, whose default 1em top/bottom margin opens
   a wide gap under the header and above the actions — a one-line comment looked
   double-spaced. Collapse the outermost margins so a comment reads tight; spacing
   between multiple paragraphs/blocks inside is kept. */
.c-text :deep(> :first-child) {
  margin-top: 0;
}
.c-text :deep(> :last-child) {
  margin-bottom: 0;
}
/* Smooth collapse/expand of a thread's replies. The grid-rows 0fr↔1fr trick
   animates auto content height; overflow is clipped only mid-transition so an
   open mention/command popup in a reply editor isn't cut off at rest. */
.c-collapse {
  display: grid;
  grid-template-rows: 1fr;
}
.c-collapse-enter-active,
.c-collapse-leave-active {
  transition:
    grid-template-rows 0.25s ease,
    opacity 0.25s ease;
}
.c-collapse-enter-active > .c-replies,
.c-collapse-leave-active > .c-replies {
  overflow: hidden;
}
.c-collapse > .c-replies {
  min-height: 0;
}
.c-collapse-enter-from,
.c-collapse-leave-to {
  grid-template-rows: 0fr;
  opacity: 0;
}
@media (prefers-reduced-motion: reduce) {
  .c-collapse-enter-active,
  .c-collapse-leave-active {
    transition: none;
  }
}
/* Composer pinned to the bottom of the comments pane (sticks while scrolling). */
.comment-add {
  display: flex;
  flex-direction: column;
  gap: 8px;
  align-items: flex-start;
  position: sticky;
  bottom: 0;
  /* The footer MUST paint its own background: a sticky element does NOT inherit
     the card's surface, it is simply transparent, and the list scrolls right
     under it. While the boxed editor was the only opaque child this stayed
     nearly invisible; any extra transparent row — the command dry-run hint,
     the offline-retry bar — puts comment text straight through the footer.
     Paint it with --n-color-modal, NOT --t-surface: a card *inside a modal*
     is painted by Naive with `background: var(--n-color-modal)` (card cssr,
     asModal), and our theme overrides only Card.color / Modal.color — the
     common `modalColor` var stays Naive's default (dark: rgb(44,44,50) vs our
     --t-surface #1e1e24). That mismatch is exactly the dark-mode seam that got
     an earlier background attempt reverted. --n-color-modal is set as a custom
     property on the .n-card root, so it inherits down here and matches the card
     by construction, in both themes. */
  background: var(--n-color-modal, var(--t-surface));
  padding-top: 10px;
  margin-top: 4px;
  border-top: 1px solid var(--t-border);
}
/* The spacing above the border — .comments' 12px flex gap plus the 4px nudge
   below — sits OUTSIDE the background box, so the list stayed visible through
   that band while scrolling (the same bleed-under-the-top-border TaskModal
   documents). Repaint it, without moving the border: offset by the 1px border
   so the pseudo covers the gap and not the line itself. */
.comment-add::before {
  content: '';
  position: absolute;
  left: 0;
  right: 0;
  bottom: calc(100% + 1px);
  height: 16px;
  background: inherit;
}
.comment-add > :first-child {
  width: 100%;
}
/* Offline-retry banner under the composer — neutral, informational. Same pop as the
   header's share-icon morph; renamed because the transition classes have to resolve
   inside this component's scope now. */
.retry-pop-enter-active,
.retry-pop-leave-active {
  transition:
    opacity 0.14s ease,
    transform 0.14s ease;
}
.retry-pop-enter-from,
.retry-pop-leave-to {
  opacity: 0;
  transform: scale(0.7);
}
@media (prefers-reduced-motion: reduce) {
  .retry-pop-enter-active,
  .retry-pop-leave-active {
    transition: none;
  }
  .retry-pop-enter-from,
  .retry-pop-leave-to {
    transform: none;
  }
}
.retry-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  font-size: 12px;
  color: var(--t-text3);
}
.retry-cancel {
  margin-left: auto;
  border: none;
  background: none;
  color: var(--t-primary);
  cursor: pointer;
  font-size: 12px;
  padding: 2px 4px;
}
.retry-cancel:hover {
  text-decoration: underline;
}
/* Command dry-run hint under the composer — flat and neutral: it states a fact,
   it is not an action. */
.cmd-preview {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 10px;
  width: 100%;
  font-size: 12px;
  color: var(--t-text3);
}
.cmd-chip {
  display: inline-flex;
  align-items: center;
  gap: 5px;
}
.cmd-chip code {
  font-family: ui-monospace, SFMono-Regular, 'JetBrains Mono', Menlo, Consolas, monospace;
  color: var(--t-text2);
}
.cmd-chip .err {
  color: var(--t-error, #e5484d);
}
.cmd-note {
  font-style: italic;
}
</style>
