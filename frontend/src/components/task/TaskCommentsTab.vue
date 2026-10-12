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
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import { useMessage } from '@/composables/useMessage'
import {
  ChatbubbleEllipsesOutline,
  FlagOutline,
  PencilOutline,
  CheckmarkOutline,
  WarningOutline,
  CloudDoneOutline,
  GitMergeOutline,
} from '@vicons/ionicons5'
import { tasks as tasksApi } from '@/api'
import { useAuthStore } from '@/stores/auth'
import { useBoardViewStore } from '@/stores/boardView'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useThemeStore } from '@/stores/theme'
import { resolvedTimezone } from '@/utils/format'
import { fmtWhen, groupThreads, eventText } from '@/utils/taskFeed'
import { sourceMeta } from '@/utils/sources'
import { useFormat } from '@/composables/useFormat'
import { toggleTaskMarker } from '@/utils/markdown'
import { hasCommandLine } from '@/utils/commands'
import { buildMentionItems, mentionIdsIn } from '@/utils/mentions'
import { scrollParent } from '@/utils/dom'
import { sortedColumns } from '@/utils/status'
import { columnName } from '@/utils/defaultNames'
import { columnStatusName } from '@/utils/columnStatus'
import { PRIORITY_COLORS } from '@/styles/tokens'
import { PRIORITY_KEYS, priorityLabel } from '@/utils/priority'
import TesseraIcon from '../TesseraIcon.vue'
import MarkdownEditor from '../MarkdownEditor.vue'
import RichContent from '../RichContent.vue'
import UserAvatar from '../UserAvatar.vue'
import TesseraSpinner from '../TesseraSpinner.vue'
import EmptyState from '../EmptyState.vue'
import ArmButton from '../kit/ArmButton.vue'
import EdReadBubble from '../editor/EdReadBubble.vue'
import { findText, markRange } from '@/utils/fragmentLink'
import { quoteRequest } from '@/utils/quoteBus'
import { CAPS } from '@/utils/keyCaps'

const props = defineProps({
  taskId: { type: String, default: null },
  comments: { type: Array, default: () => [] },
  readonly: { type: Boolean, default: false },
  hydrated: { type: Boolean, default: false },
  // v2 activity (#2958): 'all' interleaves the journal (`events`) with the
  // threads by time; 'comments' is the thread alone.
  mode: { type: String, default: 'comments' },
  events: { type: Array, default: () => [] },
  // The element the composer is teleported to — the modal pins it under the
  // scrolling column; null keeps it here, sticky at the bottom of the pane.
  composerTo: { type: null, default: null },
  composerRows: { type: Number, default: 3 },
  // The task's own people ([{ id, role }]) — first in @ (#3118); whoever wrote
  // in the thread joins them here.
  people: { type: Array, default: () => [] },
  // { number, icon, color } — the fullscreen window's header.
  editorTask: { type: Object, default: null },
})
const emit = defineEmits(['update:comments', 'changed', 'reload-detail'])

const message = useMessage()
const { t, locale } = useI18n()
const { formatters } = useFormat()
const auth = useAuthStore()
const bv = useBoardViewStore()
const store = useWorkspacesStore()
const theme = useThemeStore()

const meId = computed(() => auth.user?.id)
const commentsPane = ref(null) // the .comments container (narrow-layout scroll fallback)
const commentsListEl = ref(null) // the .c-list — the internal scroller in wide layout
const newComment = ref('')
const commentEditor = ref(null)
const editingCommentId = ref(null)
const editingCommentBody = ref('')
// The body as it stood when editing started — the baseline the mention delta is
// measured against (see addedMentions).
const editingCommentOriginal = ref('')
// True while a comment POST is in flight (spinner on the send button, guards
// double-submit); retryInfo holds { attempt, max } while waiting to retry an
// offline send, else null.
const posting = ref(false)
const retryInfo = ref(null)
// The composer's lead avatar (v2 mockup): the reader's own face.
const meAvatar = computed(() =>
  auth.user ? { id: auth.user.id, name: auth.user.name || auth.user.email || '' } : null,
)

// The flat list from the API, assembled into "root + its replies" threads.
const threads = computed(() => groupThreads(props.comments))
// What the pane lists: the threads, or — in the «Всё» view — the threads and the
// journal in one timeline. A comment is in the journal too (kind 'comment'); the
// thread already shows it, so that entry is dropped.
const feed = computed(() => {
  const th = threads.value.map((x) => ({
    kind: 'thread',
    key: x.root.id,
    at: x.root.created_at,
    th: x,
  }))
  if (props.mode !== 'all') return th
  const ev = (props.events || [])
    .filter((e) => e.kind !== 'comment')
    .map((e) => ({ kind: 'event', key: `e${e.id}`, at: e.created_at, e }))
  return [...ev, ...th].sort((a, b) => (a.at < b.at ? -1 : a.at > b.at ? 1 : 0))
})
// A move is drawn as column chips — «from → to», or just «→ to» for entries
// written before the journal kept the source — in the board's column colours
// with the column's status glyph (matched by name: the journal stores names).
const boardCols = computed(() => sortedColumns(bv.columns || []))
function colOf(name) {
  return boardCols.value.find((c) => c.name === name || columnName(c) === name) || null
}
function chipStyle(name) {
  const c = colOf(name)?.color
  return c
    ? { background: `color-mix(in srgb, ${c} 22%, var(--t-pop))`, color: 'var(--t-text1)' }
    : {}
}
function chipIcon(name) {
  const c = colOf(name)
  if (!c) return null
  const cols = boardCols.value
  const doneId = bv.board?.done_column_id || cols[cols.length - 1]?.id
  return columnStatusName({
    isDone: c.id === doneId,
    first: c.id === cols[0]?.id,
    name: c.name,
    nameKey: c.name_key,
  })
}
// A priority change gets the priority's own pill, as on the card.
const prioOf = (e) => {
  const p = Number(e.data?.to)
  return e.kind === 'priority' && PRIORITY_KEYS[p] !== undefined ? p : null
}
function prioStyle(p) {
  const c = PRIORITY_COLORS[p]
  return {
    color: c,
    background: `color-mix(in srgb, ${c} 18%, var(--t-pop))`,
  }
}
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

// The hand-rolled Russian declension this used to do lives in the catalogue now:
// vue-i18n picks the form (the CLDR rule for `ru` is registered in @/i18n), and
// English gets its own two-form message rather than a transliterated third one.
function replyCount(thread) {
  const n = thread.replies.length
  return t('task.comments.replies', n, { n })
}

// Members offered for @-mentions — and, via the same rows, resolved by the hover
// cards. Shared with the description so both render mentions off one shape.
// Full GitLab roster (Object.values(gitlabMembersMap)), not the filtered
// gitlabMembersList: buildMentionItems needs the GitLab rows of OAuth-linked
// members to attach their @gl_username (for both the inserted login and hover-card
// resolution). The raw map is passed too so PAT-linked members get their login.
const mentionItems = computed(() =>
  buildMentionItems(bv.membersList, Object.values(bv.gitlabMembersMap), bv.gitlabMembersMap),
)
// «В задаче» of the @ list: the task's people, then whoever wrote in the thread.
const composerPeople = computed(() => {
  const out = [...props.people]
  const seen = new Set(out.map((p) => p.id))
  for (const c of props.comments || []) {
    if (c.author_id && !seen.has(c.author_id)) {
      seen.add(c.author_id)
      out.push({ id: c.author_id, role: 'commenter' })
    }
  }
  return out
})

// «Цитировать в ответ» (#3118): the bubble over read text (the description, a
// comment) hands its selection over; it lands at the end of the draft as a
// Markdown quote, and the composer takes the focus.
watch(quoteRequest, async (req) => {
  if (!req || props.readonly || (req.taskId && req.taskId !== props.taskId)) return
  const draft = newComment.value.replace(/\s+$/, '')
  newComment.value = draft ? `${draft}\n\n${req.text}` : req.text
  quoteRequest.value = null
  await nextTick()
  commentEditor.value?.focus?.()
})

// ── quick actions ──
// The command registry is workspace-wide (loaded once by the store); the popup
// is only offered where commands actually run — the new-comment composer.
// Editing an existing comment and the description do not execute them.
const commandItems = computed(() => store.commands || [])
const cmdPreview = ref([]) // [{ key, summary, error }] from the dry-run
const cmdCustom = ref([]) // custom keys seen in the draft — kept as plain text
// A chip of the dry run (PackageJ «проверка под полем»): «/assign @илья → Илья
// Петров», or «/estimate три часа — не понимаю …».
function cmdLine(c) {
  const head = [`/${c.key}`, c.arg].filter(Boolean).join(' ')
  return c.error ? `${head} — ${c.error}` : `${head} → ${c.summary}`
}
let cmdTimer = null
// «/due сегодня» is resolved on the server, so the request has to say which
// calendar «сегодня» belongs to — the blank the profile may hold reads as UTC
// there (see resolvedTimezone). Sent on the dry-run too: the hint and the command
// must name the same day.
function commandTz() {
  return resolvedTimezone(theme.timezone)
}
// ── the task can change under this subtree ──
// The modal is never remounted: it swaps `taskId` in place (a subtask row, a
// relation, the «Родитель» link, an activity toast, ?task= in the URL), and this
// tab stays mounted across the swap — resetComposers below exists for exactly
// that. So every request here has to name the task it was started for and stop
// short of the screen if the reader has moved on. Two different mistakes were
// possible, and both were live:
//   • the request ADDRESS read after an await — the offline retry keeps a draft
//     for up to 17s, and resending it took `props.taskId`, i.e. posted the text
//     into whatever task was open by then. Silent: no error, the comment simply
//     lands in someone else's thread.
//   • the ANSWER applied after an await — the left task's thread published
//     through `update:comments` (the modal owns the list) or its command summary
//     reloading the task now on screen.
// `tid` is captured before the first await, used for every call, and re-checked
// before anything reaches the UI (same idiom as SearchBar/TaskModal/BoardView).
const isCurrent = (tid) => tid === props.taskId
// Dry-run the draft against the backend's parser (debounced) instead of
// re-implementing it here: the hint can never disagree with what will happen.
watch(newComment, (body) => {
  clearTimeout(cmdTimer)
  if (!hasCommandLine(body)) {
    cmdPreview.value = []
    cmdCustom.value = []
    return
  }
  const tid = props.taskId
  cmdTimer = setTimeout(async () => {
    try {
      const res = await tasksApi.previewCommands(tid, body, commandTz())
      if (!isCurrent(tid)) return
      cmdPreview.value = res.data?.commands || []
      cmdCustom.value = res.data?.custom || []
    } catch {
      if (!isCurrent(tid)) return
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
// Closing the task modal (and switching to another task in the sidebar panel, which
// keeps this subtree mounted) drops any open reply composer — the modal calls this.
function resetComposers() {
  cancelReply()
  editingCommentId.value = null
}
watch(
  () => props.taskId,
  (id, prev) => {
    resetComposers()
    switchDraft(prev, id)
  },
)

// ── the draft of each task (#3118, PackageJ «Черновик в другой задаче») ──
// The composer keeps one draft per task on this device: leaving a task with a
// half-written comment and coming back finds it where it was, and another
// task's composer doesn't inherit it. It is sent only by ⌘↵ / «Отправить».
const DRAFT_PREFIX = 'tessera_comment_draft:'
const draftAt = ref(null)
// The «saved» line is for a composer at rest, not under the hands typing.
const composerFocused = ref(false)
let draftTimer = 0
function readDraft(id) {
  try {
    return id ? JSON.parse(localStorage.getItem(DRAFT_PREFIX + id) || 'null') : null
  } catch {
    return null
  }
}
function writeDraft(id, body) {
  if (!id) return
  try {
    if (body.trim()) {
      const at = Date.now()
      localStorage.setItem(DRAFT_PREFIX + id, JSON.stringify({ body, at }))
      if (isCurrent(id)) draftAt.value = at
    } else {
      localStorage.removeItem(DRAFT_PREFIX + id)
      if (isCurrent(id)) draftAt.value = null
    }
  } catch {
    /* storage full or off — the draft just lives as long as the tab */
  }
}
function dropDraft(id) {
  clearTimeout(draftTimer)
  writeDraft(id, '')
}
function switchDraft(prev, id) {
  clearTimeout(draftTimer)
  if (prev) writeDraft(prev, newComment.value)
  const d = readDraft(id)
  newComment.value = d?.body || ''
  draftAt.value = d?.at || null
}
switchDraft(null, props.taskId)
watch(newComment, (body) => {
  clearTimeout(draftTimer)
  const id = props.taskId
  draftTimer = setTimeout(() => {
    if (isCurrent(id)) writeDraft(id, body)
  }, 500)
})
onBeforeUnmount(() => {
  clearTimeout(draftTimer)
  if (props.taskId) writeDraft(props.taskId, newComment.value)
})
const draftTime = computed(() =>
  draftAt.value
    ? new Intl.DateTimeFormat(locale.value, { hour: '2-digit', minute: '2-digit' }).format(
        draftAt.value,
      )
    : '',
)
// A link to a fragment of a comment (#3119): the comment into view, the text
// marked for a moment. False when the comment is not in the thread (deleted).
async function revealComment(id, q) {
  await nextTick()
  const el = commentsListEl.value?.querySelector(`[data-comment-id="${id}"]`)
  if (!el) return false
  revealInPane(el)
  markRange(findText(el, q))
  return true
}

defineExpose({ scrollToBottom, autoGrow, resetComposers, revealComment })

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
  const tid = props.taskId
  posting.value = true
  try {
    // attempt 0 is the initial send; 1..N are retries, each preceded by its backoff.
    for (let attempt = 0; attempt <= RETRY_BACKOFFS.length; attempt++) {
      try {
        const res = await tasksApi.addComment(tid, body, mentions, null, commandTz())
        retryInfo.value = null
        // The text went out: its draft is gone. The composer is cleared when it
        // still holds that task's text — once the reader has moved on it holds the
        // other task's own draft (drafts are per task, #3118), which is not ours to
        // throw away.
        dropDraft(tid)
        if (isCurrent(tid)) {
          newComment.value = ''
          commentEditor.value?.clear?.()
        }
        cmdPreview.value = []
        const c = await tasksApi.comments(tid)
        // What happened is reported either way — the toast names the task's own
        // result, and the board still has to hear that that task changed. Only the
        // two screen-level effects are skipped once the reader has moved on: the
        // thread would replace the open task's list, and reload-detail would
        // re-fetch a task no command touched.
        const summary = res.data?.command_summary
        const ranCommands = summary?.applied?.length || summary?.errors?.length
        if (ranCommands) reportCommands(summary)
        emit('changed')
        if (!isCurrent(tid)) return
        emit('update:comments', c.data || [])
        // Quick actions changed the task itself (assignees, column, dates…), and a
        // command-only comment produces no comment row at all — reload the detail so
        // the modal shows the result rather than the pre-command state.
        if (ranCommands) emit('reload-detail')
        scrollToBottom()
        return
      } catch (e) {
        // Only "server unreachable" is worth retrying; an HTTP error (4xx/5xx) is
        // the server rejecting the comment — resending won't change that.
        const backoff = RETRY_BACKOFFS[attempt]
        if (!e.offline || backoff == null) {
          retryInfo.value = null
          message.error(e.offline ? t('task.comments.offlineComment') : e.message)
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
  const tid = props.taskId
  replyingPost.value = true
  try {
    const res = await tasksApi.addComment(tid, body, mentions, rootId, commandTz())
    cancelReply()
    const c = await tasksApi.comments(tid)
    const summary = res.data?.command_summary
    const ranCommands = summary?.applied?.length || summary?.errors?.length
    if (ranCommands) reportCommands(summary)
    emit('changed')
    if (!isCurrent(tid)) return
    emit('update:comments', c.data || [])
    if (ranCommands) emit('reload-detail')
    // Scroll to the reply itself, not to the end of the list.
    const fresh = (c.data || []).find((x) => !known.has(x.id))
    if (fresh) await scrollToComment(fresh.id)
  } catch (e) {
    message.error(e.offline ? t('task.comments.offlineReply') : e.message)
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
  if (applied.length)
    message.success(t('task.comments.commandsApplied', { list: applied.join('; ') }))
  for (const err of summary.errors || []) {
    message.warning(`/${err.key}: ${err.error}`)
  }
}
// Whether this comment's own controls (✎ / ✕ / its checkboxes) are live. Author
// alone is not enough: the modal opened from the archive is read-only, and the
// archived task's thread is there to be read. Without the readonly half, "✕"
// deleted for real out of the archive and "✎" opened an editor whose Save is
// already guarded — so the typed text vanished without a word (#2932).
function ownLive(c) {
  return !props.readonly && !!c.author_id && c.author_id === meId.value
}
// The edit form's editor — one at a time, like the reply's (function ref: a
// string ref inside v-for collects into an array).
const editEditor = ref(null)
async function startEditComment(c) {
  editingCommentId.value = c.id
  editingCommentBody.value = c.body
  editingCommentOriginal.value = c.body
  // Into the field at once: its Esc and ⌘↵ only work from there.
  await nextTick()
  editEditor.value?.focus?.()
}
// Who this edit newly mentions: the people the saved text chips, minus the ones
// the text already chipped before it was opened for editing. Computed here
// rather than server-side because only this side holds the roster a handle is
// rendered against — and only the difference is sent, or fixing a typo would
// notify everyone named in the comment all over again.
function addedMentions(body) {
  const before = new Set(mentionIdsIn(editingCommentOriginal.value, mentionItems.value))
  return mentionIdsIn(body, mentionItems.value).filter((id) => !before.has(id))
}
async function saveComment() {
  if (props.readonly) return
  const body = editingCommentBody.value.trim()
  if (!body) return
  const tid = props.taskId
  try {
    await tasksApi.updateComment(editingCommentId.value, body, addedMentions(body))
    editingCommentId.value = null
    const c = await tasksApi.comments(tid)
    if (!isCurrent(tid)) return
    emit('update:comments', c.data || [])
  } catch (e) {
    message.error(e.message)
  }
}
// Deleting a comment re-reads the thread instead of filtering the row out
// locally, because removing a *root* is not a removal of one row server-side:
// the oldest reply is promoted to root and the rest are re-hung under it
// (promoteThreadSuccessor), precisely so one "✕" never takes someone else's text
// with it. A local filter keeps those replies pointing at the row that is gone,
// and groupThreads then draws each of them as a standalone top-level comment —
// the thread shatters on screen, which is the outcome the promotion exists to
// avoid. Android already re-reads here (TaskDetailViewModel.deleteComment).
async function deleteComment(id) {
  if (props.readonly) return
  const tid = props.taskId
  try {
    await tasksApi.removeComment(id)
  } catch (e) {
    message.error(e.message)
    return
  }
  try {
    const c = await tasksApi.comments(tid)
    if (!isCurrent(tid)) return
    emit('update:comments', c.data || [])
  } catch {
    // The delete went through and only the re-read failed: drop the row locally
    // so the list at least stops showing what is no longer there. Only while that
    // list is still the one on screen — `props.comments` is the OPEN task's thread,
    // and filtering it once the reader has moved on would cut a row out of it.
    if (!isCurrent(tid)) return
    emit(
      'update:comments',
      props.comments.filter((x) => x.id !== id),
    )
  }
}
// Toggle a task checkbox inside a rendered (own) comment → rewrite its markdown
// and persist. Optimistic; reverts on failure.
async function onCommentCheck(c, i) {
  if (props.readonly) return
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
        <div v-for="it in feed" :key="it.key" :class="it.kind === 'event' ? 'c-event' : 'c-thread'">
          <template v-if="it.kind === 'event'">
            <!-- A merge request's line (#2964) is the provider's, not a person's:
                 a neutral glyph bubble and «GitLab ·» instead of an avatar and a name. -->
            <span v-if="it.e.kind === 'code_change'" class="e-src" data-testid="task-event-code">
              <n-icon :component="GitMergeOutline" :size="12" />
            </span>
            <UserAvatar v-else class="e-ava" :user-id="it.e.actor_id" :name="it.e.actor_name" />
            <span class="e-text">
              <template v-if="it.e.kind === 'code_change'">
                <b class="e-src-name">{{ sourceMeta(it.e.data?.source).label }}</b> ·
              </template>
              <b v-else>{{ it.e.actor_name || t('task.history.someone') }}</b>
              <template v-if="it.e.kind === 'moved' && it.e.data?.to">
                {{ t('task.journal.movedVerb') }}
                <template v-if="it.e.data.from">
                  <span class="e-chip" :style="chipStyle(it.e.data.from)">
                    <TesseraIcon
                      v-if="chipIcon(it.e.data.from)"
                      :name="chipIcon(it.e.data.from)"
                      :size="13"
                      :style="{ color: colOf(it.e.data.from)?.color }"
                    />
                    {{ it.e.data.from }}
                  </span>
                </template>
                <span class="e-arrow">→</span>
                <span class="e-chip" :style="chipStyle(it.e.data.to)">
                  <TesseraIcon
                    v-if="chipIcon(it.e.data.to)"
                    :name="chipIcon(it.e.data.to)"
                    :size="13"
                    :style="{ color: colOf(it.e.data.to)?.color }"
                  />
                  {{ it.e.data.to }}
                </span>
              </template>
              <template v-else-if="prioOf(it.e) != null">
                {{ t('task.journal.priorityVerb') }}
                <span class="e-chip e-prio" :style="prioStyle(prioOf(it.e))">
                  <FlagOutline class="e-flag" />
                  {{ priorityLabel(prioOf(it.e)) }}
                </span>
              </template>
              <template v-else-if="it.e.kind === 'renamed' && it.e.data?.to">
                {{ t('task.journal.renamedVerb') }}
                <span class="e-name">{{ it.e.data.to }}</span>
              </template>
              <template v-else>{{ eventText(it.e) }}</template>
            </span>
            <span class="e-when">{{ fmtWhen(it.e.created_at, formatters) }}</span>
          </template>
          <template v-else>
            <div class="comment" :data-comment-id="it.th.root.id">
              <UserAvatar
                class="c-ava"
                :user-id="it.th.root.author_id || ''"
                :src="it.th.root.gl_author_avatar_url"
                :name="it.th.root.author_name || it.th.root.gl_author_name || '?'"
              />
              <div class="c-body">
                <div class="c-head">
                  <span class="c-author">{{
                    it.th.root.author_name ||
                    it.th.root.gl_author_name ||
                    t('task.comments.someone')
                  }}</span>
                  <span v-if="!it.th.root.author_name && it.th.root.gl_author_name" class="c-gl">
                    · GitLab
                  </span>
                  <span class="c-when">{{ fmtWhen(it.th.root.created_at, formatters) }}</span>
                  <span
                    v-if="ownLive(it.th.root) && editingCommentId !== it.th.root.id"
                    class="c-acts"
                  >
                    <button
                      type="button"
                      class="c-ibtn"
                      :title="t('common.action.edit')"
                      :aria-label="t('common.action.edit')"
                      data-testid="comment-edit"
                      @click="startEditComment(it.th.root)"
                    >
                      <n-icon :component="PencilOutline" :size="15" />
                    </button>
                    <ArmButton
                      :title="t('common.action.delete')"
                      :confirm="t('task.arm.delete')"
                      testid="comment-delete"
                      @confirm="deleteComment(it.th.root.id)"
                    />
                  </span>
                </div>
                <MarkdownEditor
                  v-if="editingCommentId === it.th.root.id"
                  :ref="(el) => (editEditor = el)"
                  v-model="editingCommentBody"
                  variant="v2"
                  :mention-items="mentionItems"
                  :task-people="composerPeople"
                  :attach-task-id="taskId"
                  :min-rows="2"
                  :placeholder="t('task.comments.placeholder')"
                  @attachments-changed="emit('changed')"
                  @submit="saveComment"
                  @cancel="editingCommentId = null"
                />
                <!-- A wrapper, not a class on RichContent: it renders a fragment. -->
                <div v-else class="c-bubble">
                  <RichContent
                    class="c-text"
                    :source="it.th.root.body"
                    :members="mentionItems"
                    mention-cards
                    task-refs
                    :interactive="ownLive(it.th.root)"
                    @toggle="onCommentCheck(it.th.root, $event)"
                  />
                </div>
                <div class="c-thread-acts">
                  <button
                    v-if="!readonly"
                    class="c-link"
                    data-testid="comment-reply"
                    @click="startReply(it.th.root.id, it.th.root)"
                  >
                    {{ t('task.comments.reply') }}
                  </button>
                  <button
                    v-if="it.th.replies.length"
                    class="c-link"
                    @click="toggleCollapsed(it.th.root.id)"
                  >
                    {{
                      collapsed.has(it.th.root.id) ? replyCount(it.th) : t('task.comments.collapse')
                    }}
                  </button>
                </div>
              </div>
            </div>

            <!-- Replies: indented under the root and tied to it by the rail. The
               rail is a flat neutral grey — the accent gradient is for
               non-neutral elements only. The grid-rows wrapper animates the
               collapse/expand smoothly (0fr ↔ 1fr). -->
            <Transition name="c-collapse">
              <div v-if="it.th.replies.length && !collapsed.has(it.th.root.id)" class="c-collapse">
                <div class="c-replies">
                  <div
                    v-for="r in it.th.replies"
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
                          r.author_name || r.gl_author_name || t('task.comments.someone')
                        }}</span>
                        <span v-if="!r.author_name && r.gl_author_name" class="c-gl">· GitLab</span>
                        <span class="c-when">{{ fmtWhen(r.created_at, formatters) }}</span>
                        <span v-if="ownLive(r) && editingCommentId !== r.id" class="c-acts">
                          <button
                            type="button"
                            class="c-ibtn"
                            :title="t('common.action.edit')"
                            :aria-label="t('common.action.edit')"
                            data-testid="comment-edit"
                            @click="startEditComment(r)"
                          >
                            <n-icon :component="PencilOutline" :size="15" />
                          </button>
                          <ArmButton
                            :title="t('common.action.delete')"
                            :confirm="t('task.arm.delete')"
                            testid="comment-delete"
                            @confirm="deleteComment(r.id)"
                          />
                        </span>
                      </div>
                      <MarkdownEditor
                        v-if="editingCommentId === r.id"
                        :ref="(el) => (editEditor = el)"
                        v-model="editingCommentBody"
                        variant="v2"
                        :mention-items="mentionItems"
                        :task-people="composerPeople"
                        :attach-task-id="taskId"
                        :min-rows="2"
                        :placeholder="t('task.comments.placeholder')"
                        @attachments-changed="emit('changed')"
                        @submit="saveComment"
                        @cancel="editingCommentId = null"
                      />
                      <div v-else class="c-bubble">
                        <RichContent
                          class="c-text"
                          :source="r.body"
                          :members="mentionItems"
                          mention-cards
                          task-refs
                          :interactive="ownLive(r)"
                          @toggle="onCommentCheck(r, $event)"
                        />
                      </div>
                      <div v-if="!readonly" class="c-thread-acts">
                        <!-- Replying to a reply targets the same root: threads are two
                       levels deep, and the backend collapses it anyway. The reply's
                       own author is the one pre-mentioned, though. -->
                        <button class="c-link" @click="startReply(it.th.root.id, r)">
                          {{ t('task.comments.reply') }}
                        </button>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </Transition>

            <div v-if="replyingTo === it.th.root.id && !readonly" class="c-reply-add">
              <!-- Function ref, not a string one: a string ref inside v-for
                 collects into an array, and getMentions() would be undefined.
                 Only one composer is open at a time, so a single slot is right. -->
              <MarkdownEditor
                :ref="(el) => (replyEditor = el)"
                v-model="replyBody"
                variant="v2"
                :save-label="t('task.comments.reply')"
                :sending="replyingPost"
                :mention-items="mentionItems"
                :task-people="composerPeople"
                :command-items="commandItems"
                :attach-task-id="taskId"
                :min-rows="2"
                :placeholder="t('task.comments.replyPlaceholder')"
                @attachments-changed="emit('changed')"
                @submit="postReply(it.th.root.id)"
                @cancel="cancelReply"
              />
            </div>
          </template>
        </div>
      </TransitionGroup>
      <EmptyState
        v-if="!feed.length"
        class="c-empty"
        size="small"
        :icon="ChatbubbleEllipsesOutline"
        :text="t('task.comments.empty')"
      />
    </div>
    <!-- Selecting a comment's text: «Цитировать в ответ Q · Ссылка на фрагмент». -->
    <EdReadBubble
      :target="() => commentsListEl"
      :task-id="taskId"
      :task="editorTask"
      :can-quote="!readonly"
    />
    <!-- The composer: pinned under the modal's scrolling column when it gives a
         target (v2), else sticky at the bottom of this pane. -->
    <Teleport v-if="!readonly" :to="composerTo" :disabled="!composerTo">
      <div
        class="comment-add"
        :class="{ docked: !!composerTo }"
        data-testid="comment-composer"
        @focusin="composerFocused = true"
        @focusout="composerFocused = $event.currentTarget.contains($event.relatedTarget)"
      >
        <MarkdownEditor
          ref="commentEditor"
          v-model="newComment"
          variant="composer"
          :avatar="meAvatar"
          send
          :sending="posting"
          :mention-items="mentionItems"
          :task-people="composerPeople"
          :command-items="commandItems"
          :command-hints="cmdPreview"
          :full-title="t('documents.editor.fullscreen.comment')"
          :full-task="editorTask"
          :attach-task-id="taskId"
          :min-rows="composerRows"
          :placeholder="t('task.comments.composerPlaceholder')"
          @attachments-changed="emit('changed')"
          @submit="postComment"
        />
        <!-- Offline-retry banner: shown while waiting to resend after a
           network failure; the draft stays put, Cancel aborts. -->
        <Transition name="retry-pop">
          <div v-if="retryInfo" class="retry-bar" data-testid="comment-retry">
            <TesseraSpinner :size="14" />
            <span class="retry-t"
              ><b>{{ t('task.comments.offlineTitle') }}</b> —
              {{
                t('task.comments.retrying', { attempt: retryInfo.attempt, max: retryInfo.max })
              }}</span
            >
            <button class="retry-cancel" @click="cancelRetry">
              {{ t('common.action.cancel') }}
            </button>
          </div>
        </Transition>
        <div
          v-if="draftTime && newComment.trim() && !posting && !composerFocused"
          class="draft-line"
          data-testid="comment-draft"
        >
          <n-icon :component="CloudDoneOutline" :size="13" />
          {{ t('task.comments.draftSaved', { time: draftTime, keys: CAPS.modEnter }) }}
        </div>
        <!-- Dry-run hint: what the built-in commands in the draft will
           do. Custom keys are listed apart — they stay in the text. -->
        <div v-if="cmdPreview.length || cmdCustom.length" class="cmd-preview">
          <span
            v-for="(c, i) in cmdPreview"
            :key="`${c.key}-${i}`"
            class="cmd-chip"
            :class="{ err: c.error }"
            data-testid="cmd-chip"
          >
            <n-icon :component="c.error ? WarningOutline : CheckmarkOutline" :size="13" />
            <span>{{ cmdLine(c) }}</span>
          </span>
          <span v-if="cmdCustom.length" class="cmd-note">
            {{
              t('task.comments.customCommands', { list: cmdCustom.map((k) => `/${k}`).join(', ') })
            }}
          </span>
        </div>
      </div>
    </Teleport>
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
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.c-head {
  display: flex;
  align-items: center;
  gap: 8px;
  min-height: 26px;
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
  font-size: 13px;
  color: var(--t-text3);
}
/* Edit / delete: icon buttons that surface on the row's hover (v2, #3113). */
.c-acts {
  margin-left: auto;
  display: inline-flex;
  align-items: center;
  gap: 2px;
  opacity: 0;
  transition: opacity 0.12s ease;
}
.comment:hover > .c-body > .c-head > .c-acts,
.c-acts:focus-within {
  opacity: 1;
}
@media (hover: none) {
  .c-acts {
    opacity: 1;
  }
}
.c-ibtn {
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  padding: 0;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--t-text3);
  cursor: pointer;
  transition:
    background 0.12s ease,
    color 0.12s ease;
}
.c-ibtn:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.c-ibtn:focus-visible {
  outline: 2px solid var(--t-primary);
  outline-offset: 1px;
}
/* The comment itself (TaskModal mockup L75): a bubble with its corner at the
   author, one level up from the card. */
.c-bubble {
  align-self: flex-start;
  max-width: 100%;
  box-sizing: border-box;
  padding: 10px 14px;
  border-radius: 4px 14px 14px 14px;
  background: color-mix(in srgb, var(--t-sub) 65%, var(--t-pop));
  color: var(--t-text1);
}
.c-text {
  font-size: 14px;
  line-height: 1.55;
}
/* RichContent's root carries no scope of ours, so the paragraph margins of the
   rendered text are trimmed from the bubble — a one-line comment otherwise
   floats in a box twice its height. */
.c-bubble :deep(.md > :first-child) {
  margin-top: 0;
}
.c-bubble :deep(.md > :last-child) {
  margin-bottom: 0;
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
     asModal), and that colour (common `modalColor`, the v2 `pop` level) is not
     --t-surface. That mismatch is exactly the dark-mode seam that got an
     earlier background attempt reverted. --n-color-modal is set as a custom
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
/* Docked under the modal's column (v2): the column scrolls above it, so it is
   neither sticky nor separated by a line — the modal pads it. */
.comment-add.docked {
  position: static;
  margin: 0;
  padding: 0;
  border-top: none;
  background: transparent;
}
.comment-add.docked::before {
  display: none;
}
/* ── «Всё»: journal rows between the threads (v2, #2958) ── */
.c-event {
  display: flex;
  align-items: center;
  gap: 8px;
  min-height: 28px;
  font-size: 13px;
  color: var(--t-text3);
}
.e-src {
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  border-radius: 50%;
  background: var(--t-pill);
  color: var(--t-text2);
}
.e-src-name {
  color: var(--t-text2);
  font-weight: 500;
}
.e-ava {
  flex: none;
  width: 20px;
  height: 20px;
  border-radius: 50%;
  background: var(--t-accent-grad);
  color: var(--t-on-primary);
  font-size: 9px;
  font-weight: 600;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}
.e-text {
  flex: 1;
  min-width: 0;
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 4px 6px;
}
.e-text b {
  color: var(--t-text2);
  font-weight: 500;
}
.e-chip {
  gap: 5px;
  display: inline-flex;
  align-items: center;
  height: 24px;
  padding: 0 10px;
  border-radius: 12px;
  background: var(--t-pill);
  color: var(--t-text2);
  font-size: 12.5px;
  font-weight: 600;
}
.e-flag {
  width: 13px;
  height: 13px;
}
.e-prio {
  font-weight: 600;
}
.e-name {
  color: var(--t-text1);
  font-weight: 500;
}
.e-arrow {
  color: var(--t-text3);
}
.e-when {
  flex: none;
  font-size: 12px;
  color: var(--t-text3);
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
/* «Нет связи — повтор попытки (2/5)…» under the composer (PackageJ «Повтор
   отправки»): a sunken 36px strip with the spinner and «Отмена». */
.retry-bar {
  display: flex;
  align-items: center;
  gap: 9px;
  width: 100%;
  height: 36px;
  padding: 0 6px 0 10px;
  box-sizing: border-box;
  border-radius: 10px;
  background: var(--t-sub);
  font-size: 12.5px;
  color: var(--t-text1);
}
.retry-t {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.retry-t b {
  font-weight: 600;
}
.retry-cancel {
  flex: none;
  height: 26px;
  padding: 0 10px;
  border: none;
  border-radius: 7px;
  background: var(--t-ctl);
  color: var(--t-text1);
  cursor: pointer;
  font: inherit;
  font-size: 12.5px;
}
.retry-cancel:hover {
  background: var(--t-hover);
}
/* The dry run of the draft's commands (PackageJ «проверка под полем»): a green
   chip for what will happen, an amber one for what won't. */
.cmd-preview {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  width: 100%;
}
.cmd-chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  max-width: 100%;
  height: 26px;
  padding: 0 10px 0 8px;
  box-sizing: border-box;
  border-radius: 999px;
  background: linear-gradient(
    to top right,
    color-mix(in srgb, #18a058 12%, var(--t-pop)),
    color-mix(in srgb, #18a058 22%, var(--t-pop))
  );
  color: var(--t-text1);
  font-size: 12.5px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.cmd-chip > .n-icon {
  flex: none;
  color: #18a058;
}
.cmd-chip.err {
  background: linear-gradient(
    to top right,
    color-mix(in srgb, #f0a020 12%, var(--t-pop)),
    color-mix(in srgb, #f0a020 22%, var(--t-pop))
  );
}
.cmd-chip.err > .n-icon {
  color: #f0a020;
}
.draft-line {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--t-text3);
}
.cmd-note {
  font-size: 12px;
  font-style: italic;
  color: var(--t-text3);
}
/* A phone: the composer rides the keyboard (PackageJ «Композер, клавиатура и
   Aa») — a pop bar with a shadow, lifted by the keyboard's height. */
@media (max-width: 768px) {
  .comment-add.docked {
    padding-bottom: var(--kb-h, 0px);
  }
}
</style>
