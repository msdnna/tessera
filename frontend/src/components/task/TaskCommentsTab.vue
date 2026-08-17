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
import { fmtWhen } from '@/utils/taskFeed'
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
        <div v-for="c in comments" :key="c.id" class="comment">
          <UserAvatar
            class="c-ava"
            :user-id="c.author_id || ''"
            :src="c.gl_author_avatar_url"
            :name="c.author_name || c.gl_author_name || '?'"
          />
          <div class="c-body">
            <div class="c-head">
              <span class="c-author">{{ c.author_name || c.gl_author_name || 'Кто-то' }}</span>
              <span v-if="!c.author_name && c.gl_author_name" class="c-gl">· GitLab</span>
              <span class="c-when">{{ fmtWhen(c.created_at) }}</span>
              <span v-if="c.author_id === meId" class="c-acts">
                <button class="c-act" title="Изменить" @click="startEditComment(c)">✎</button>
                <n-popconfirm
                  :positive-button-props="{ type: 'error' }"
                  positive-text="Удалить"
                  @positive-click="deleteComment(c.id)"
                >
                  <template #trigger>
                    <button class="c-act" title="Удалить">✕</button>
                  </template>
                  Удалить комментарий?
                </n-popconfirm>
              </span>
            </div>
            <template v-if="editingCommentId === c.id">
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
              :source="c.body"
              :members="mentionItems"
              task-refs
              :interactive="c.author_id === meId"
              @toggle="onCommentCheck(c, $event)"
            />
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
