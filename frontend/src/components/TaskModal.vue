<script setup>
import { tourEvent } from '@/utils/tourEvents'
import { ref, watch, computed, nextTick, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { NModal, NInput, NButton, NPopover, NIcon, NSpin, useMessage } from 'naive-ui'
import {
  FlagOutline,
  CalendarClearOutline,
  CheckmarkOutline,
  PlayForwardOutline,
  ChevronDownOutline,
  ArrowUndoOutline,
  LayersOutline,
  LogoGitlab,
  OpenOutline,
  RepeatOutline,
  TimeOutline,
  TrailSignOutline,
  SettingsOutline,
} from '@vicons/ionicons5'
import { tasks as tasksApi, boards as boardsApi, projects as projApi, gitlab as glApi } from '@/api'
import { storeToRefs } from 'pinia'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useProjectWindowStore } from '@/stores/projectWindow'
import { useBoardViewStore } from '@/stores/boardView'
import { PRIORITY_COLORS } from '@/styles/tokens'
import { priorityLabel, priorityOptions as priorityOptionList } from '@/utils/priority'
import { hueGrad, softFill, readableHue, onColor } from '@/utils/gradient'
import { buildTagGroups, matchTagByName } from '@/utils/tagGroups'
import { milestoneRange } from '@/utils/milestones'
import { resolvedTimezone } from '@/utils/format'
import { useFormat } from '@/composables/useFormat'
import { sortedColumns, columnById, nextColumn, doneTarget, moveNeighbors } from '@/utils/status'
import { planComplete } from '@/utils/cardNav'
import { columnStatusName } from '@/utils/columnStatus'
import { useShortcuts } from '@/composables/useShortcuts'
import { useTaskModalMenu } from '@/composables/useTaskModalMenu'
import { columnName } from '@/utils/defaultNames'
import { taskLink } from '@/utils/taskLink'
import { buildMentionItems } from '@/utils/mentions'
import { copyText } from '@/utils/clipboard'
import { normalizeTitle } from '@/utils/title'
import { glPickerMembers } from '@/utils/assignees'
import {
  dismissesSidebar,
  effectiveTaskLayout,
  loadTaskLayout,
  saveTaskLayout,
} from '@/utils/taskLayout'
import { useResponsive } from '@/composables/useResponsive'
import {
  formatEstimate,
  formatEstimateFull,
  estimateRangeShort,
  parseEstimateInput,
  scaleOptions,
  estimatePlaceholder,
  sumEstimates,
} from '@/utils/estimation'
import { useThemeStore } from '@/stores/theme'
import { useTagFit } from '@/composables/useTagFit'
import DueEditor from './DueEditor.vue'
import PopMenu from './PopMenu.vue'
import TagPill from './TagPill.vue'
import UserAvatar from './UserAvatar.vue'
import TesseraSpinner from './TesseraSpinner.vue'
import TesseraIcon from './TesseraIcon.vue'
import ShortcutKeys from './ShortcutKeys.vue'
import TaskModalHeader from './task/TaskModalHeader.vue'
import TaskCommentsTab from './task/TaskCommentsTab.vue'
import TaskDescriptionTab from './task/TaskDescriptionTab.vue'
import TaskSubtasksTab from './task/TaskSubtasksTab.vue'
import TaskInboxSection from './task/TaskInboxSection.vue'
import TaskRelationsTab from './task/TaskRelationsTab.vue'
import TaskFilesTab from './task/TaskFilesTab.vue'
import TaskDocumentsTab from './task/TaskDocumentsTab.vue'

const props = defineProps({
  show: { type: Boolean, default: false },
  taskId: { type: String, default: null },
  // Archive view: the task is shown read-only (no edits/comments); the footer
  // offers Restore instead of Save/Archive/Delete.
  readonly: { type: Boolean, default: false },
  // The kanban's live top-level task list, so the parent picker doesn't re-fetch
  // what is already in memory. Stays a prop: it's the board's *task* state, which
  // the board owns, not shared board context.
  boardTopTasks: { type: Array, default: () => [] },
  // { index, total }: where the task sits in the board's current order — the
  // header's «2 из 18» and J/K (v2, #2958). The board answers `navigate`.
  nav: { type: Object, default: null },
})
const emit = defineEmits(['update:show', 'changed', 'open', 'restore', 'navigate'])

// Board context (workspace/project, tags, members, milestones, the board itself
// with its columns, and the GitLab integration flags) comes from the board store
// instead of thirteen props threaded through the kanban. Bound to the names the
// rest of this component and its template already use. `board`/`boardColumns` are
// used only when the opened task belongs to the open board; a deep-link or
// cross-board open still falls back to fetching.
const bv = useBoardViewStore()
const {
  board,
  columns: boardColumns,
  metaTagPrefixes,
  tagsList: tags,
  membersList: members,
  milestonesList: milestones,
  gitlabMembersList: gitlabMembers,
  gitlabCanCreate,
  gitlabFetchTemplates,
  gitlabIntegrationId,
  gitlabCanGroup,
} = storeToRefs(bv)
const tagPrefixNames = bv.prefixNames
const wsId = computed(() => bv.wsId)
// The description used to hand RichContent the raw member rows, which carry no
// `label` — so highlightMentions matched nothing and multi-word names fell
// through to the generic handle token ("@Ann Lee" highlighted as "@Ann"). Same
// builder as the comments tab, so both sides highlight and resolve alike.
// Full GitLab roster (not the assignee-picker's filtered gitlabMembers): an
// OAuth-linked member is mentioned by their @gl_username, which buildMentionItems
// folds onto their Tessera row so the hover card resolves.
const mentionItems = computed(() =>
  buildMentionItems(members.value, Object.values(bv.gitlabMembersMap), bv.gitlabMembersMap),
)
const projectId = computed(() => bv.projectId)

const store = useWorkspacesStore()
const theme = useThemeStore()
const { t } = useI18n()
const { formatDue, formatRelative, formatDate, formatters } = useFormat()
// Tag colour clamped for legible text on the active theme.
const tagText = (c) => readableHue(c, theme.isDark)
const router = useRouter()
const message = useMessage()
const loading = ref(false)
const task = ref(null)
// Read-only is a property of the TASK, not of the screen it was opened from.
// The `readonly` prop carries the board's archive SCOPE (?archived=1); `archived_at`
// carries the task's own state — and half the doors into this modal walk past the
// scope entirely: a relation (openRelated pushes ?task= with no archived=1), a
// notification (ListNotifications doesn't filter the archive), a document link, a
// `#N` chip, a copied link. The backend has no archive guard at all — archive is a
// client convention — so an archived task reached that way took edits for real: its
// pills were live, its status picker moved it back onto the board, comments posted.
// Hence the OR: the scope alone was only ever half the answer (#2932).
const ro = computed(() => props.readonly || !!task.value?.archived_at)
const boardInfo = ref(null) // { name, projectId } for the breadcrumb
const parentCandidates = ref([]) // top-level tasks on the board (for attach)

// ── rich detail (#8): comments, relations, files, journal
// Open an existing description on the Preview tab; an empty one on Write.
// The editor itself lives in TaskDescriptionTab (in the left column for modal/
// fullscreen, or the first tab in the sidebar layout — #2745).
const descInitialMode = ref('write')

// ── wide-screen only: hide the properties panel to give the content the whole
// width (the «⋯» menu). Persisted in localStorage (non-critical UX state). In the
// stacked layout the properties are part of the flow, so the state is a no-op
// there. ──
const { isMobile: narrow } = useResponsive(1099)
// A phone (#2976 — package G «Задача»): the card is a sheet the full height of
// the screen under the status bar, radius 18; pulled down by its handle, it
// closes.
const { isMobile: phone } = useResponsive()
const pullY = ref(0)
let pull = null
function onPullDown(e) {
  if (e.button > 0) return
  pull = { y: e.clientY, t: performance.now(), id: e.pointerId }
  e.currentTarget.setPointerCapture?.(e.pointerId)
}
function onPullMove(e) {
  if (!pull || e.pointerId !== pull.id) return
  pullY.value = Math.max(0, e.clientY - pull.y)
}
function onPullUp(e) {
  if (!pull) return
  const dy = Math.max(0, e.clientY - pull.y)
  const v = dy / Math.max(1, performance.now() - pull.t)
  const h = cardRef.value?.offsetHeight || 800
  pull = null
  pullY.value = 0
  if (dy > h * 0.25 || v > 0.6) close()
}

// ── where to show the task: centred modal (default), fullscreen, or a right-hand
// side panel that leaves the board underneath usable (#2716). All three are the
// same n-modal with a different card class and three props — swapping the wrapper
// for an n-drawer would tear down the subtree and lose the active tab, the comment
// draft and the unsaved description on every switch. ──
const savedLayout = ref(loadTaskLayout())
const layout = computed(() => effectiveTaskLayout(savedLayout.value, narrow.value))
function setLayout(v) {
  savedLayout.value = v
  saveTaskLayout(v)
}

// `wide` means "there is room for two columns", not "the window is wide": the
// sidebar layout keeps a wide window but gives the card ~560px, where the
// properties panel can't sit beside the content.
const wide = computed(() => !narrow.value && layout.value !== 'sidebar')

// ── Sidebar layout: a click on empty space dismisses the panel (see
// `dismissesSidebar` for which clicks count). mousedown in the capture phase, not
// click: it fires before a menu item can remove itself from the DOM, and a text
// selection that starts inside the card and ends out on the board isn't mistaken
// for an outside click. ──
const cardRef = ref(null)
function onOutsidePointerDown(e) {
  if (dismissesSidebar(e.target, cardRef.value)) emit('update:show', false)
}
let dismissBound = false
function bindDismiss(on) {
  if (on === dismissBound) return
  dismissBound = on
  if (on) document.addEventListener('mousedown', onOutsidePointerDown, true)
  else document.removeEventListener('mousedown', onOutsidePointerDown, true)
}
watch(() => props.show && layout.value === 'sidebar', bindDismiss, { immediate: true })
onBeforeUnmount(() => bindDismiss(false))

// The sidebar panel is `position: fixed` over the right edge, so it covers the board
// underneath — the rightmost columns (e.g. «Готово») end up behind it, unreachable by
// the board's own horizontal scroll (#2743). While the panel is up we publish its
// width as a root CSS var; the board reserves that much scroll slack on its right so
// every column can be scrolled out from under the panel. Cleared when it closes.
function setPanelReserve(on) {
  const root = document.documentElement
  if (on) root.style.setProperty('--task-panel-w', 'min(560px, 100vw)')
  else root.style.removeProperty('--task-panel-w')
}
watch(() => props.show && layout.value === 'sidebar', setPanelReserve, { immediate: true })
onBeforeUnmount(() => setPanelReserve(false))

const PANE_KEY = 'tessera_task_pane'
const rightHidden = ref(localStorage.getItem(PANE_KEY) === 'hidden')
function toggleRightPane() {
  rightHidden.value = !rightHidden.value
  try {
    localStorage.setItem(PANE_KEY, rightHidden.value ? 'hidden' : 'shown')
  } catch {
    /* storage disabled — non-fatal */
  }
  // The content column changes width — the composer re-measures its textarea.
  nextTick(() => commentsTab.value?.autoGrow?.())
}

const comments = ref([])
// Gates the enter fade to genuinely-new comments: false during a task's initial
// population (the whole thread shouldn't fade in on open), flipped true once loaded.
// Owned here because only this component knows when a population is a load and when
// it is a user posting; the tab just renders it.
const commentsHydrated = ref(false)
const commentsTab = ref(null)
const subtasksTab = ref(null)
const inboxSection = ref(null)

// ── «Поделиться»: copy a shareable link to this task ──
// The link (/board/<id>?task=<n>) self-canonicalizes to slugs, so it is correct
// regardless of which board/workspace the modal was opened from. On copy the
// share icon morphs into a checkmark for 1.6s — that IS the confirmation (no toast).
const shareUrl = computed(() => taskLink(task.value))
const shared = ref(false)
let shareTimer = null
async function shareTask() {
  const url = shareUrl.value
  if (!url) return
  if (!(await copyText(url))) {
    message.error(t('task.head.shareFailed'))
    return
  }
  shared.value = true
  clearTimeout(shareTimer)
  shareTimer = setTimeout(() => (shared.value = false), 1600)
}

// Per-task transient UI that this component owns (the share confirmation).
watch(
  () => props.taskId,
  () => {
    shared.value = false
    clearTimeout(shareTimer)
  },
)
onBeforeUnmount(() => {
  clearTimeout(shareTimer)
})

const relations = ref([])

const attachments = ref([])
const events = ref([])
// Documents that link to this task (#2732) — the other end of the link the
// documents panel creates.
const docLinks = ref([])

const title = ref('')
const description = ref('')
// Last persisted description. Only used to mark the field "filled" for the Get
// Started guide (#2759): keying that off `description` would end the step on the
// first keystroke and yank the popover away mid-sentence.
const descSaved = ref('')
const priority = ref(0)
const dueTs = ref(null)
const startTs = ref(null)
const estimate = ref(null) // canonical estimate value | null
const estInput = ref('') // free-text buffer for the estimate popover
const recurrence = ref(null) // full recurrence rule object | null
const columns = ref([]) // board columns: status row, subtask rows, recurrence selects
const doneColumnId = ref(null) // boards.done_column_id — target of the «close» check
const completed = ref(false)
const selectedTags = ref([])
const selectedAssignees = ref([])
const newTagName = ref('')
// «Управлять тегами…» (#2990): the picker closes and the project window opens on
// its tag table, filtered by what was typed.
const projectWindow = useProjectWindowStore()
function manageTags() {
  const pid = projectId.value || boardInfo.value?.projectId
  if (!pid) return
  const filter = newTagName.value.trim()
  fieldOpen.value = ''
  projectWindow.open(pid, 'tags', { filter })
}

// Computed: the ladder is rebuilt per render so a language switch reaches it.
const priorityOptions = computed(() => priorityOptionList())

// ── estimation ──────────────────────────────────────────────
// Effective unit config for this task's project (project override → workspace
// default → built-in). boardInfo.projectId is the reliable source once loaded.
const estCfg = computed(() => store.estimationFor(projectId.value || boardInfo.value?.projectId))
// Compact estimate ("8н") — used to prefill the free-text editor, which parses
// that same syntax.
const estLabel = computed(() => formatEstimate(estimate.value, estCfg.value))
// Full spelled-out estimate ("8 недель"), shown as the row's value.
const estFull = computed(() => formatEstimateFull(estimate.value, estCfg.value))
// Projected window from start_date, free-form ("18 мая → 13 июл."), shown inline.
const estRange = computed(() =>
  estimateRangeShort(task.value?.start_date, estimate.value, estCfg.value, formatters.value),
)
const estOptions = computed(() => scaleOptions(estCfg.value)) // non-empty only for points
const estPlaceholder = computed(() => estimatePlaceholder(estCfg.value))
// Rollup hint: sum of direct subtask estimates, shown when the task has children.
const subtaskEstimate = computed(() => sumEstimates(task.value?.subtasks))
const subtaskEstimateLabel = computed(() =>
  subtaskEstimate.value != null ? formatEstimate(subtaskEstimate.value, estCfg.value) : '',
)
function setEstimate(val) {
  estimate.value = val && val > 0 ? val : null
  applyMeta()
}
// Apply what was typed in the estimate box. An unreadable entry is reported and
// left alone: the field is free text ("1ч 30", "3д 4ч", "полдня"), and the old
// `setEstimate(parse(...))` turned every misread into a silent clear — the number
// a person had planned with vanished on a typo, with nothing on screen to say so.
function applyEstInput() {
  const { value, ok } = parseEstimateInput(estInput.value, estCfg.value)
  if (!ok) {
    message.error(t('task.estimate.unreadable', { value: estInput.value.trim() }))
    return
  }
  setEstimate(value)
}
function clearEstimate() {
  estInput.value = ''
  setEstimate(null)
}
// Prefill the free-text buffer with the current value when the popover opens.
function onEstShow(shown) {
  setField('estimate', shown)
  if (shown) estInput.value = estLabel.value
}

const tagObjs = computed(() =>
  selectedTags.value.map((id) => tags.value.find((t) => t.id === id)).filter(Boolean),
)
// Picker tags grouped by prefix (friendly name); a single prefix-less bucket
// renders flat without a header.
const tagPickerGroups = computed(() =>
  buildTagGroups(tags.value, tagPrefixNames, metaTagPrefixes.value),
)
const tagPickerHeaders = computed(() => tagPickerGroups.value.length > 1)
// Flat picker set — what a typed name is matched against, so the answer covers
// exactly the tags the reader could have clicked instead (GitLab meta-prefixed
// tags are hidden here on purpose: attaching one desyncs from the field it drives).
const tagPickerFlat = computed(() => tagPickerGroups.value.flatMap((g) => g.tags))

// The tags trigger shows as many WHOLE tag chips as fit on one line, then "+N"
// (shared with the stacked card row via useTagFit — which also re-observes the
// trigger on remount, fixing the "reopen shows only 1 chip" bug).
const tagsValEl = ref(null)
const tagsMeasureEl = ref(null)
const { visibleCount: visibleTagCount } = useTagFit(tagsValEl, tagsMeasureEl, tagObjs, { gap: 5 })
const assigneeObjs = computed(() =>
  selectedAssignees.value.map((id) => members.value.find((m) => m.user_id === id)).filter(Boolean),
)
// External GitLab assignees (display-only) from the task detail.
const glAssignees = computed(() => task.value?.gitlab_assignees || [])
// GitLab half of the assignee picker: the project roster plus whoever this task
// already carries that the roster has since dropped (a hand-pinned assignee who
// left the GitLab project outlives it) — without them the modal drew the face and
// offered no row to un-assign it. Same rule as the card's picker; see utils/assignees.
const glPickerList = computed(() => glPickerMembers(gitlabMembers.value, glAssignees.value))
// Author = who created the card (read-only): the GitLab issue author for synced
// tasks, otherwise the Tessera user resolved from created_by.
const author = computed(() => {
  const t = task.value
  if (!t) return null
  if (t.gitlab && t.gitlab.author) {
    return {
      name: t.gitlab.author_name || t.gitlab.author,
      login: t.gitlab.author,
      avatar: t.gitlab.author_avatar_url,
      gl: true,
    }
  }
  if (t.created_by) {
    const m = bv.membersMap[t.created_by]
    if (m) return { name: m.name, id: t.created_by }
  }
  return null
})
const dueLabel = computed(() => {
  const d = dueTs.value ? formatDue(new Date(dueTs.value).toISOString()) : ''
  const s = startTs.value ? formatDue(new Date(startTs.value).toISOString()) : ''
  if (s && d) return `${s} → ${d}`
  if (s) return `${s} →`
  return d
})

// Per-task due-notification selectors (sentinel -1 / 'inherit' = user default),
// passed to the shared DueEditor.
const dueNotify = computed(() => {
  const t = task.value || {}
  return {
    enabled: t.due_notify_enabled == null ? 'inherit' : t.due_notify_enabled ? 'on' : 'off',
    lead: t.due_lead_minutes ?? -1,
    repeat: t.due_repeat_minutes ?? -1,
  }
})

// The task's project — the header's icon and first crumb.
const project = computed(
  () => store.projects.find((p) => p.id === boardInfo.value?.projectId) || null,
)

// The board keeps ONE TaskModal mounted and swaps `taskId` underneath it (a subtask
// row, a relation, the «Родитель» link, an activity toast, ?task= in the URL), so two
// detail loads can be in flight at once and the slower one answers last. Without a
// token saying which task an answer was asked for, the load the reader has LEFT fills
// the form — title/description/priority/dates/completed — of the task they are now
// looking at, and the next Save sends that payload to `props.taskId`: the first task's
// content written silently over the second one's (#2932).
//
// Same idiom as SearchBar, and taken before the first early return for the same
// reason: a call that goes on to do nothing (the modal closed, taskId cleared) still
// has to invalidate whatever is in flight, or its answer repopulates what we cleared.
let loadSeq = 0
async function loadDetail() {
  const mine = ++loadSeq
  if (!props.taskId) return
  loading.value = true
  try {
    const res = await tasksApi.get(props.taskId)
    if (mine !== loadSeq) return
    const t = res.data
    task.value = t
    title.value = t.title
    description.value = t.description || ''
    descSaved.value = description.value
    descInitialMode.value = t.description ? 'preview' : 'write'
    priority.value = t.priority || 0
    dueTs.value = t.due_date ? new Date(t.due_date).getTime() : null
    startTs.value = t.start_date ? new Date(t.start_date).getTime() : null
    estimate.value = t.estimate ?? null
    recurrence.value = t.recurrence || null
    completed.value = !!t.completed_at
    selectedTags.value = (t.tags || []).map((x) => x.id)
    selectedAssignees.value = (t.assignees || []).map((x) => x.id)
    // Board context is already in the kanban's memory for the current board — use
    // it instead of re-fetching the board + its full task/column lists on every
    // open (the tasks list is the same payload the board just loaded). Only a
    // deep-link or a cross-board open (task not on this board) falls back to GETs.
    const ctx = board.value && board.value.id === t.board_id ? board.value : null
    if (ctx) {
      boardInfo.value = { name: ctx.name, projectId: ctx.project_id }
      parentCandidates.value = (props.boardTopTasks || []).filter((x) => x.id !== t.id)
      columns.value = (boardColumns.value || []).map((c) => ({
        id: c.id,
        name: c.name,
        // name_key rides along or the status chip and the move menu would caption
        // seeded columns by their stored Russian name (#2800).
        name_key: c.name_key,
        color: c.color,
        position: c.position,
      }))
      doneColumnId.value = ctx.done_column_id || null
    } else {
      try {
        const b = await boardsApi.get(t.board_id)
        boardInfo.value = { name: b.data.name, projectId: b.data.project_id }
        const cols = await boardsApi.columns(t.board_id)
        // parentCandidates powers the "make subtask of…" picker (top-level tasks).
        const bt = await boardsApi.tasks(t.board_id)
        // Three more round-trips, three more chances for the task to change under us.
        if (mine !== loadSeq) return
        parentCandidates.value = (bt.data || []).filter((x) => x.id !== t.id)
        // color/position feed the status chip and the "shift right" button; the
        // recurrence selects only ever needed id/name.
        columns.value = (cols.data || []).map((c) => ({
          id: c.id,
          name: c.name,
          name_key: c.name_key,
          color: c.color,
          position: c.position,
        }))
        doneColumnId.value = b.data.done_column_id || null
      } catch {
        if (mine !== loadSeq) return
        boardInfo.value = null
        parentCandidates.value = []
        columns.value = []
        doneColumnId.value = null
      }
    }
    loadSiblings(t, mine)
    loadExtras(mine)
    if (!t.gitlab) loadGlTemplates(mine)
  } catch (e) {
    if (mine !== loadSeq) return
    message.error(e.message)
  } finally {
    // Only the live load owns the spinner: a stale one clearing it would uncover a
    // half-populated form while the task on screen is still loading.
    if (mine === loadSeq) loading.value = false
  }
}

// The parent's subtask list (ordered) is only needed when this task is itself a
// subtask and only to pin its position on a move — best-effort, one extra GET.
// The same response also carries the parent's own title, which the «Родитель»
// row shows (#2861) — so that stays one request, not two.
async function loadSiblings(t, mine) {
  siblings.value = []
  parentTask.value = null
  if (!t?.parent_id) return
  try {
    const res = await tasksApi.get(t.parent_id)
    if (mine !== loadSeq) return
    siblings.value = res.data?.subtasks || []
    parentTask.value = res.data || null
  } catch {
    if (mine !== loadSeq) return
    siblings.value = []
    parentTask.value = null
  }
}

// Set when the modal (re)opens; consumed once by loadExtras to auto-scroll the
// comments to the newest — so opening scrolls to the bottom, but an in-place
// reload (a move, a subtask edit) does not yank the view back down.
let scrollOnOpen = false
// Comments / relations / attachments / journal load in parallel — none of them
// should block the modal opening, so failures are swallowed individually.
// The editor can add a file to the task (a non-image drop into the description),
// and the «Файлы» tab plus its badge count are owned here — so it has to be told.
async function reloadAttachments() {
  // Called from the editor (a file dropped into the description), not from a load —
  // so it takes the current token rather than being handed one.
  const mine = loadSeq
  try {
    const a = await tasksApi.attachments(props.taskId)
    if (mine !== loadSeq) return
    attachments.value = a.data || []
    emit('changed')
  } catch {
    /* the file is uploaded either way; the list refreshes on reopen */
  }
}
async function loadExtras(mine) {
  const id = props.taskId
  // Populate the thread without the enter fade (only later, user-posted comments fade).
  commentsHydrated.value = false
  const [c, r, a, e, d] = await Promise.allSettled([
    tasksApi.comments(id),
    tasksApi.relations(id),
    tasksApi.attachments(id),
    tasksApi.events(id),
    tasksApi.documents(id),
  ])
  // `id` was captured before the requests, but capturing it never protected the
  // assignments below — this is the check that does.
  if (mine !== loadSeq) return
  comments.value = c.status === 'fulfilled' ? c.value.data || [] : []
  relations.value = r.status === 'fulfilled' ? r.value.data || [] : []
  attachments.value = a.status === 'fulfilled' ? a.value.data || [] : []
  events.value = e.status === 'fulfilled' ? e.value.data || [] : []
  docLinks.value = d.status === 'fulfilled' ? d.value.data || [] : []
  // On open, land on the newest comment — but only in the wide layout, where the
  // right column scrolls on its own. In the stacked layout the whole modal scrolls,
  // and jumping to the bottom would skip past the title and description.
  if (scrollOnOpen && wide.value) commentsTab.value?.scrollToBottom()
  scrollOnOpen = false
  // Enable the enter fade only after this population has rendered.
  nextTick(() => (commentsHydrated.value = true))
}

watch(
  () => [props.show, props.taskId],
  ([show]) => {
    if (show) {
      scrollOnOpen = true
      loadDetail()
    } else {
      // Invalidate the load in flight: closing is not a new load, so nothing else
      // bumps the token, and the answer would otherwise repopulate the form of
      // whatever task is opened next.
      loadSeq++
      loading.value = false
      // Closing the panel drops any open reply/edit composer (the subtree stays
      // mounted, so it wouldn't clear itself).
      commentsTab.value?.resetComposers?.()
    }
  },
)

function close() {
  emit('update:show', false)
}
function buildPayload() {
  return {
    title: normalizeTitle(title.value),
    description: description.value,
    priority: priority.value,
    due_date: dueTs.value ? new Date(dueTs.value).toISOString() : null,
    start_date: startTs.value ? new Date(startTs.value).toISOString() : null,
    estimate: estimate.value,
    recurrence: recurrence.value,
    completed: completed.value,
  }
}
// Tappable fields persist immediately; Save commits the text fields.
async function applyMeta() {
  if (ro.value) return
  try {
    const res = await tasksApi.update(props.taskId, buildPayload())
    task.value = res.data
    // The server may have rescheduled a recurring task (completing it bounces it
    // back out of done with an advanced due date) — resync from the response.
    completed.value = !!res.data.completed_at
    dueTs.value = res.data.due_date ? new Date(res.data.due_date).getTime() : null
    startTs.value = res.data.start_date ? new Date(res.data.start_date).getTime() : null
    estimate.value = res.data.estimate ?? null
    emit('changed')
    refreshEvents()
  } catch (e) {
    message.error(e.message)
  }
}
// The «Всё» feed shows the journal: a change made here lands in it at once.
async function refreshEvents() {
  const id = props.taskId
  try {
    const r = await tasksApi.events(id)
    if (id === props.taskId) events.value = r.data || []
  } catch {
    /* the feed catches up on the next open */
  }
}
function setPriority(p) {
  priority.value = p
  applyMeta()
  if (p) tourEvent('task.priority')
}
// ── milestone («Этап») ──
const newMilestoneTitle = ref('')
// Locally-created milestones shown immediately, before the board reloads its meta.
const extraMilestones = ref([])
const milestoneOptions = computed(() => {
  const seen = new Set(milestones.value.map((m) => m.id))
  return [...milestones.value, ...extraMilestones.value.filter((m) => !seen.has(m.id))]
})
const taskMilestone = computed(() =>
  task.value?.milestone_id
    ? milestoneOptions.value.find((m) => m.id === task.value.milestone_id) || null
    : null,
)
async function setMilestone(milestoneId) {
  try {
    if (milestoneId) await tasksApi.setMilestone(props.taskId, milestoneId)
    else await tasksApi.clearMilestone(props.taskId)
    if (task.value) task.value.milestone_id = milestoneId || null
    emit('changed')
  } catch (e) {
    message.error(e.message)
  }
}
async function createMilestone() {
  const title = newMilestoneTitle.value.trim()
  if (!title || !projectId.value) return
  try {
    const { data } = await projApi.createMilestone(projectId.value, { title })
    extraMilestones.value.push(data)
    newMilestoneTitle.value = ''
    await setMilestone(data.id)
  } catch (e) {
    message.error(e.message)
  }
}
// DueEditor commits start + due + recurrence together.
function onDueApply(patch) {
  dueTs.value = patch.due_date ? new Date(patch.due_date).getTime() : null
  if (dueTs.value) tourEvent('task.due')
  startTs.value = patch.start_date ? new Date(patch.start_date).getTime() : null
  recurrence.value = patch.recurrence
  applyMeta()
}
async function onDueNotify(patch) {
  const n = dueNotify.value
  const lead = patch.lead ?? n.lead
  const repeat = patch.repeat ?? n.repeat
  const enabled = patch.enabled ?? n.enabled
  try {
    await tasksApi.dueNotify(props.taskId, {
      lead_minutes: lead === -1 ? null : lead,
      repeat_minutes: repeat === -1 ? null : repeat,
      enabled: enabled === 'inherit' ? null : enabled === 'on',
    })
    const res = await tasksApi.get(props.taskId)
    task.value = res.data
    emit('changed')
  } catch (e) {
    void e
  }
}
function setCompleted(v) {
  completed.value = v
  applyMeta()
}

// ── status (board column) ───────────────────────────────────
// The «Выполнено» switch became a status row: [● Колонка ▾] [▸ shift] [✓ close].
// Moving is PATCH /tasks/:id/move — which already works for subtasks — so the
// backend does the completed/reopened bookkeeping and the journal entry for us.
const sortedCols = computed(() => sortedColumns(columns.value))
const currentColumn = computed(() => columnById(columns.value, task.value?.column_id))
const nextCol = computed(() => nextColumn(columns.value, task.value?.column_id))
const doneCol = computed(() => doneTarget(columns.value, doneColumnId.value))
const moving = ref(false)
// Sibling order of this task inside its parent (empty for a top-level task) —
// only used to keep the position stable on a move.
const siblings = ref([])
// The parent task itself, from the same GET as `siblings` — only its title (and
// number, until the title lands) is used, by the «Родитель» row.
const parentTask = ref(null)
// Empty while the parent GET is in flight (or if it failed) — the row then shows
// an ellipsis placeholder rather than a chip with no text.
const parentLabel = computed(() => parentTask.value?.title || '…')

async function moveToColumn(columnId) {
  if (ro.value || !task.value || !columnId || columnId === task.value.column_id) return
  moving.value = true
  try {
    // Neighbours for the PATCH: a subtask pins its row in the parent's list, a
    // top-level card names none and is appended by the server — see moveNeighbors
    // for why this client must not compute the column's end itself.
    await tasksApi.move(task.value.id, {
      column_id: columnId,
      ...moveNeighbors(task.value, siblings.value),
    })
    // A recurring task completed by entering the done column bounces straight
    // back out with an advanced due date — re-read instead of trusting the click.
    await loadDetail()
    emit('changed')
  } catch (e) {
    message.error(e.message)
  } finally {
    moving.value = false
  }
}

// The checkmark closes the task through the board's done column (so the backend
// stamps completed_at and logs it); boards without one fall back to the flag.
async function closeTask() {
  if (ro.value || !task.value) return
  const plan = planComplete({
    completed: completed.value,
    columnId: task.value.column_id,
    doneColumnId: doneCol.value?.id,
  })
  if (plan.op === 'uncomplete') return setCompleted(false)
  if (plan.op === 'move') return moveToColumn(plan.columnId)
  setCompleted(true)
}

// ── Create a GitLab issue from this task ──
// Offered (button under «Родитель») only on the integration board when push_create
// is on and the task isn't already linked. The issue is built from the task's own
// properties + description; an optional repo template can prefill the description
// editor (above) before creating. On success the task becomes a synced issue task.
const glCreating = ref(false)
const glTemplates = ref([])
const glTemplate = ref(null)
const glTemplateOptions = computed(() =>
  glTemplates.value.map((t) => ({ label: t.name, value: t.name })),
)
async function loadGlTemplates(mine) {
  glTemplates.value = []
  glTemplate.value = null
  if (!gitlabFetchTemplates.value || !wsId.value) return
  try {
    const res = await glApi.issueTemplates(wsId.value, gitlabIntegrationId.value)
    if (mine !== loadSeq) return
    glTemplates.value = res.data || []
  } catch {
    if (mine !== loadSeq) return
    glTemplates.value = []
  }
}
// Picking a template fills the description editor (which is the task's description,
// and becomes the issue body on create); the editor's blur persists it.
function applyGlTemplate(name) {
  const tpl = glTemplates.value.find((t) => t.name === name)
  if (tpl) description.value = tpl.content
}
// glAssetSuffix summarises what creation did with the task's attachments (#2713).
// Uploading bytes makes the call noticeably slower than it used to be, so the toast
// says what the wait bought — and names skipped files, since a skipped one is a link
// the issue will not render.
function glAssetSuffix(stats) {
  if (!stats) return ''
  const parts = []
  if (stats.uploaded) parts.push(t('task.gitlab.assetsUploaded', { n: stats.uploaded }))
  if (stats.skipped) parts.push(t('task.gitlab.assetsSkipped', { n: stats.skipped }))
  return parts.length ? ` (${parts.join(', ')})` : ''
}
async function createGlIssue() {
  glCreating.value = true
  try {
    // Persist the current title/description/properties first so the issue mirrors
    // the on-screen state, then create (the backend reads the saved task).
    await applyMeta()
    // The due date crosses over as a day, not an instant — send the calendar it
    // was written in (the profile zone, else the browser's).
    const created = await glApi.createIssue(props.taskId, {}, resolvedTimezone(theme.timezone))
    // Re-fetch so the task picks up its GitLab provenance and renders as synced.
    const res = await tasksApi.get(props.taskId)
    task.value = res.data
    message.success(
      t('task.gitlab.created', {
        iid: res.data.gitlab?.iid,
        suffix: glAssetSuffix(created.data?.attachments),
      }),
    )
    emit('changed')
  } catch (e) {
    message.error(e.message)
  } finally {
    glCreating.value = false
  }
}
// ── Mark this task's issue as a grouped parent (#2592) ──
// Offered only for an already-linked task on an integration that allows pushing
// children. Ungrouping is refused by the backend (409) while GitLab-linked subtasks
// still hang off this issue — that message is what the user sees.
const glGrouping = ref(false)
const glGroupAction = computed(() => {
  if (glGrouping.value) return t('task.gitlab.saving')
  return task.value?.gitlab?.is_group ? t('task.gitlab.ungroup') : t('task.gitlab.group')
})
async function toggleGlGroup() {
  const link = task.value?.gitlab
  if (!link || glGrouping.value) return
  const grouped = link.is_group === true
  glGrouping.value = true
  try {
    const res = grouped ? await glApi.clearGroup(props.taskId) : await glApi.setGroup(props.taskId)
    // The endpoint answers with the refreshed link view, so no re-fetch is needed for
    // the button itself; `changed` still fires because the board card shows the badge.
    task.value = { ...task.value, gitlab: res.data }
    message.success(grouped ? t('task.gitlab.groupCleared') : t('task.gitlab.groupSet'))
    emit('changed')
  } catch (e) {
    message.error(e.message)
  } finally {
    glGrouping.value = false
  }
}
// Archive (soft delete). Subtasks go to the archive with the parent unless the
// user chooses to detach them (keep them on the board).
// The head's «⋯» (#3112): GitLab, the side panel, archive (at once, with an
// undo toast) and the two-step delete.
const moreMenu = useTaskModalMenu({
  task,
  readonly: () => ro.value,
  wide: () => wide.value,
  paneHidden: () => rightHidden.value,
  togglePane: () => toggleRightPane(),
  restore: () => emit('restore', props.taskId),
  changed: () => emit('changed'),
  close: () => close(),
})

// ── transfer to another board (the header's crumbs menu) ──
async function transferTo(boardId) {
  try {
    await tasksApi.transfer(props.taskId, { board_id: boardId })
    emit('changed')
    close()
  } catch (e) {
    message.error(e.message)
  }
}
// Detach this task from its parent → becomes a top-level board card.
async function detachFromParent() {
  try {
    await tasksApi.setParent(props.taskId, null)
    emit('changed')
    close()
  } catch (e) {
    message.error(e.message)
  }
}
// Attach this task as a subtask of another (becomes its child).
async function attachTo(parentId) {
  try {
    await tasksApi.setParent(props.taskId, parentId)
    emit('changed')
    close()
  } catch (e) {
    message.error(e.message)
  }
}

async function toggleTag(id) {
  try {
    if (selectedTags.value.includes(id)) {
      await tasksApi.removeTag(props.taskId, id)
      selectedTags.value = selectedTags.value.filter((x) => x !== id)
    } else {
      await tasksApi.addTag(props.taskId, id)
      selectedTags.value = [...selectedTags.value, id]
    }
    emit('changed')
  } catch (e) {
    message.error(e.message)
  }
}
const creatingTag = ref(false)
async function createTag() {
  const n = newTagName.value.trim()
  // The guard is what makes a second Enter harmless: the field is cleared only once
  // the tag is on the task, so an impatient repeat used to send a second create —
  // and the second one came back «имя занято» about the tag the first had just made.
  if (!n || creatingTag.value) return
  // A name the project already carries is a request for THAT tag, not for a twin:
  // the grid above is not filtered by what's typed, so on a long tag list typing the
  // name is the way to reach it. Creating was a dead end — the server refuses the
  // duplicate (409) and the task stays untagged.
  const existing = matchTagByName(tagPickerFlat.value, n)
  if (existing) {
    newTagName.value = ''
    if (selectedTags.value.includes(existing.id)) message.info(t('task.tags.alreadyOn'))
    else await toggleTag(existing.id)
    return
  }
  const palette = ['#7c5cff', '#2f80ed', '#0eb0a9', '#18a058', '#f0a020', '#e0533d', '#eb2f96']
  creatingTag.value = true
  try {
    const res = await projApi.createTag(projectId.value, {
      name: n,
      color: palette[Math.floor(Math.random() * palette.length)],
    })
    await tasksApi.addTag(props.taskId, res.data.id)
    selectedTags.value = [...selectedTags.value, res.data.id]
    newTagName.value = ''
    emit('changed')
  } catch (e) {
    message.error(e.message)
  } finally {
    creatingTag.value = false
  }
}
async function toggleAssignee(uid) {
  try {
    if (selectedAssignees.value.includes(uid)) {
      await tasksApi.removeAssignee(props.taskId, uid)
      selectedAssignees.value = selectedAssignees.value.filter((x) => x !== uid)
    } else {
      await tasksApi.addAssignee(props.taskId, uid)
      selectedAssignees.value = [...selectedAssignees.value, uid]
    }
    emit('changed')
    refreshEvents()
  } catch (e) {
    message.error(e.message)
  }
}

function isGlAssigned(username) {
  return glAssignees.value.some((g) => g.gl_username === username)
}
// Assign/unassign a GitLab project member (may have no Tessera account). On
// integration boards with push_assignees on, the backend mirrors this to the issue.
async function toggleGlAssignee(m) {
  try {
    if (isGlAssigned(m.gl_username))
      await tasksApi.removeGitlabAssignee(props.taskId, m.gl_username)
    else
      await tasksApi.pinGitlabAssignee(props.taskId, {
        gl_username: m.gl_username,
        gl_name: m.gl_name,
        gl_avatar_url: m.gl_avatar_url,
      })
    const res = await tasksApi.get(props.taskId)
    task.value = res.data
    emit('changed')
  } catch (e) {
    message.error(e.message)
  }
}

// ── rich description ──
async function saveDesc() {
  descSaved.value = description.value
  await applyMeta()
  if (descSaved.value) tourEvent('task.described')
}

function openRelated(rel) {
  if (rel.related_board_id) {
    close()
    router.push(`/board/${rel.related_board_id}?task=${rel.related_task_id}`)
  }
}

// Opening a linked document leaves the board, so the modal closes first. The
// slug is preferred over the id for the same reason the documents section
// prefers it: it is the shareable form of the address.
function openDocument(link) {
  if (!link?.document_id) return
  close()
  router.push(`/documents/${link.document_slug || link.document_id}`)
}

// Adding, completing or moving a subtask changes the parent task itself (its
// subtask list, its rollups), so the tab reports the mutation and the reload
// stays here — it is the modal that owns the task detail.
async function onSubtaskChanged() {
  await loadDetail()
  emit('changed')
}

// ── v2 (#2958): no Save/Cancel footer — everything persists as it changes ──
// The title commits on blur / Enter; the description already persists on blur.
// Closing flushes whatever is still dirty, so nothing typed is lost.
async function saveTitle() {
  if (ro.value || !task.value) return
  const next = normalizeTitle(title.value)
  if (!next) {
    title.value = task.value.title
    return
  }
  if (next !== task.value.title) await applyMeta()
}
function onTitleEnter(e) {
  e.target?.blur?.()
}
// The title grows with its text. Measured by hand: the card animates in and the
// panel layouts change its width, so a size taken once on mount goes stale — it
// is re-fitted on load, on open and whenever the field's width changes.
const titleEl = ref(null)
function fitTitle() {
  const el = titleEl.value
  if (!el) return
  el.style.height = 'auto'
  el.style.height = `${el.scrollHeight}px`
}
let titleRO = null
watch(titleEl, (el) => {
  titleRO?.disconnect()
  titleRO = null
  if (!el || typeof ResizeObserver === 'undefined') return
  let w = 0
  titleRO = new ResizeObserver(([e]) => {
    if (Math.round(e.contentRect.width) === w) return
    w = Math.round(e.contentRect.width)
    fitTitle()
  })
  titleRO.observe(el)
})
watch(title, () => nextTick(fitTitle))
onBeforeUnmount(() => titleRO?.disconnect())
watch(
  () => props.show,
  (show) => {
    if (show || ro.value || !task.value) return
    if (normalizeTitle(title.value) !== task.value.title) saveTitle()
    else if (description.value !== descSaved.value) saveDesc()
  },
)

// ── status glyphs: the board header's four shapes, for any column ──
function statusIcon(col) {
  if (!col) return 'status-todo'
  const cols = sortedCols.value
  const done = doneCol.value ? col.id === doneCol.value.id : col.id === cols[cols.length - 1]?.id
  return columnStatusName({
    isDone: done,
    first: col.id === cols[0]?.id,
    name: col.name,
    nameKey: col.name_key,
  })
}
// The status button carries its column's tone (the mockup's tinted pill).
const statusTone = computed(() => {
  const c = currentColumn.value?.color || 'var(--t-primary)'
  return {
    '--st-c': c,
    background: `linear-gradient(to top right, color-mix(in srgb, ${c} 13%, var(--t-pop)), color-mix(in srgb, ${c} 22%, var(--t-pop)))`,
  }
})
const priorityTone = computed(() => {
  const c = PRIORITY_COLORS[priority.value]
  return priority.value
    ? {
        color: readableHue(c, theme.isDark),
        background: `linear-gradient(to top right, color-mix(in srgb, ${c} 14%, var(--t-sub)), color-mix(in srgb, ${c} 24%, var(--t-sub)))`,
      }
    : {}
})
const modifiedLabel = computed(() =>
  task.value?.updated_at
    ? t('task.head.modified', { when: formatRelative(task.value.updated_at) })
    : '',
)
const createdLabel = computed(() => {
  const at = task.value?.created_at
  if (!at) return ''
  const when = formatDate(at, { day: 'numeric', month: 'short' })
  return author.value
    ? t('task.head.createdBy', { name: author.value.name, when })
    : t('task.head.created', { when })
})
const assigneeNames = computed(() =>
  [
    ...assigneeObjs.value.map((u) => (u.name || '').split(/\s+/)[0]),
    ...glAssignees.value.map((g) => g.gl_name || g.gl_username),
  ]
    .filter(Boolean)
    .join(', '),
)

// Teleport targets inside this card: the properties panel moves to the side
// column when there is room for one, and the composer sits under the content.
// Element refs, not selectors — they resolve inside this card whatever else is
// on the page, and stay disabled (inline) until the element exists.
const sideEl = ref(null)
const composerEl = ref(null)

// ── activity: «Всё» (comments + journal) / «Комментарии» / «Файлы» ──
const activityMode = ref('all')

// ── field popovers, opened by click or by their key (S/P/A/D/L) ──
// One controlled slot: a key opens its field's popover, picking a single value
// (status, priority) closes it, multi-pickers stay open for more.
const fieldOpen = ref('')
function setField(name, open) {
  if (open) fieldOpen.value = name
  else if (fieldOpen.value === name) fieldOpen.value = ''
}
function openField(name) {
  if (ro.value) return
  // The keys reach the panel's fields; a hidden panel comes back first.
  if (name !== 'status' && wide.value && rightHidden.value) toggleRightPane()
  nextTick(() => (fieldOpen.value = name))
}
function pickPriority(p) {
  fieldOpen.value = ''
  setPriority(p)
}
function pickColumn(id) {
  fieldOpen.value = ''
  moveToColumn(id)
}
watch(
  () => props.taskId,
  () => (fieldOpen.value = ''),
)

// ── keys (v2, #2958): J/K walk the board's order, the field keys mirror the
// selected card's. Live only while this modal is the top layer and nothing in it
// has an open menu — a dropdown or another dialog keeps its keys.
function keysLive() {
  if (!props.show || !task.value || typeof document === 'undefined') return false
  if (document.querySelectorAll('.n-modal-container .n-modal').length > 1) return false
  if (document.querySelector('.n-dropdown-menu')) return false
  return !fieldOpen.value
}
const canEdit = () => !ro.value

// Esc (v2, #2958): closes what is open inside the modal first — a field's
// popover, a header menu — and the modal only when nothing is. Naive's own
// close-on-esc is off: it shut the whole modal over an open dropdown.
const headerRef = ref(null)
function onEsc() {
  if (fieldOpen.value) {
    fieldOpen.value = ''
    return
  }
  if (headerRef.value?.closeMenus()) return
  close()
}
useShortcuts(
  { 'task.close': onEsc },
  {
    when: () =>
      props.show &&
      typeof document !== 'undefined' &&
      document.querySelectorAll('.n-modal-container .n-modal').length === 1 &&
      !document.querySelector('.n-dropdown-menu'),
  },
)
useShortcuts(
  {
    'task.next': () => props.nav && emit('navigate', 1),
    'task.prev': () => props.nav && emit('navigate', -1),
    'task.complete': () => canEdit() && closeTask(),
    'task.status': () => openField('status'),
    'task.assignees': () => openField('assignees'),
    'task.priority': (d) => canEdit() && setPriority(Number(d)),
    'task.priorityPick': () => openField('priority'),
    'task.due': () => openField('due'),
    'task.tags': () => openField('tags'),
    'task.subtask': () => canEdit() && subtasksTab.value?.startAdd(),
    // «Напоминания и заметки» (#2998): reminders and notes are the reader's own,
    // so a read-only task still takes them — only an archived one doesn't.
    'task.remind': () => !ro.value && inboxSection.value?.startRemind(),
    'task.note': () => !ro.value && inboxSection.value?.startNote(),
  },
  { when: keysLive },
)
</script>

<template>
  <!-- One wrapper for all three layouts. With show-mask=false Naive puts
       pointer-events:none on the container and :all on the card, and only closes on
       a click that lands in the container — so in the sidebar layout the board
       underneath stays fully live (cards clickable, drag-n-drop working) and a
       stray click on it doesn't slam the panel shut. -->
  <n-modal
    :show="show"
    :show-mask="layout !== 'sidebar'"
    :block-scroll="layout !== 'sidebar'"
    :trap-focus="layout !== 'sidebar'"
    :auto-focus="false"
    :close-on-esc="false"
    @update:show="emit('update:show', $event)"
  >
    <div
      ref="cardRef"
      class="tm-card"
      :class="{
        'tm-fullscreen': layout === 'fullscreen',
        'tm-sidebar': layout === 'sidebar',
        'tm-wide': wide,
        'tm-pane-hidden': wide && rightHidden,
        'tm-phone': phone,
        'tm-pulling': pullY > 0,
      }"
      :style="pullY ? { transform: `translateY(${pullY}px)` } : null"
      role="dialog"
      aria-modal="true"
      data-testid="task-modal"
    >
      <div
        v-if="phone"
        class="tm-grab"
        aria-hidden="true"
        @pointerdown="onPullDown"
        @pointermove="onPullMove"
        @pointerup="onPullUp"
        @pointercancel="onPullUp"
      >
        <span class="tm-handle" />
      </div>
      <TaskModalHeader
        ref="headerRef"
        :task="task"
        :project="project"
        :board-name="boardInfo?.name || ''"
        :nav="nav"
        :layout="layout"
        :narrow="narrow"
        :shared="shared"
        :readonly="ro"
        :more-items="moreMenu.items.value"
        @navigate="emit('navigate', $event)"
        @update:layout="setLayout"
        @share="shareTask"
        @close="close"
        @transfer="transferTo"
        @more="moreMenu.select"
      />

      <n-spin :show="loading" :rotate="false" class="tm-spin">
        <template #icon><TesseraSpinner /></template>
        <div class="tm-body">
          <div class="tm-main">
            <div class="tm-scroll">
              <div v-if="ro" class="tm-ro-banner">
                <span>{{ t('task.footer.readonly') }}</span>
                <n-button size="small" type="primary" @click="emit('restore', taskId)">
                  <template #icon><n-icon :component="ArrowUndoOutline" /></template>
                  {{ t('task.action.restore') }}
                </n-button>
              </div>

              <!-- status · shift right · complete · when it last changed -->
              <div class="tm-actions">
                <n-popover
                  v-if="!ro && sortedCols.length"
                  :show="fieldOpen === 'status'"
                  trigger="click"
                  placement="bottom-start"
                  @update:show="setField('status', $event)"
                >
                  <template #trigger>
                    <button
                      class="tm-status col-chip"
                      :style="statusTone"
                      :disabled="moving"
                      data-testid="task-status"
                    >
                      <TesseraIcon
                        :name="statusIcon(currentColumn)"
                        :size="15"
                        class="tm-status-ico"
                      />
                      <span>{{ columnName(currentColumn) || t('task.value.columnNone') }}</span>
                      <n-icon :component="ChevronDownOutline" :size="12" class="tm-status-chev" />
                      <ShortcutKeys id="task.status" />
                    </button>
                  </template>
                  <PopMenu class="pmenu">
                    <div
                      v-for="c in sortedCols"
                      :key="c.id"
                      class="menu-item col-item"
                      :class="{ cur: c.id === task?.column_id }"
                      @click="pickColumn(c.id)"
                    >
                      <TesseraIcon
                        :name="statusIcon(c)"
                        :size="14"
                        :style="{ color: c.color || 'var(--t-text3)' }"
                      />
                      <span>{{ columnName(c) }}</span>
                    </div>
                  </PopMenu>
                </n-popover>
                <span v-else class="tm-status col-chip static" :style="statusTone">
                  <TesseraIcon :name="statusIcon(currentColumn)" :size="15" class="tm-status-ico" />
                  <span>{{ columnName(currentColumn) || '—' }}</span>
                </span>
                <template v-if="!ro">
                  <button
                    class="st-btn tm-next"
                    :disabled="!nextCol || moving"
                    :title="
                      nextCol
                        ? t('task.status.next', { name: columnName(nextCol) })
                        : t('task.status.last')
                    "
                    @click="moveToColumn(nextCol?.id)"
                  >
                    <n-icon :component="PlayForwardOutline" :size="14" />
                  </button>
                  <button
                    class="st-btn tm-done"
                    :class="{ on: completed }"
                    :disabled="moving"
                    :title="completed ? t('task.status.reopen') : t('task.status.complete')"
                    data-testid="task-complete"
                    @click="closeTask"
                  >
                    <n-icon :component="CheckmarkOutline" :size="15" class="tm-done-ico" />
                    <span>{{
                      completed ? t('task.action.completed') : t('task.action.complete')
                    }}</span>
                    <ShortcutKeys id="task.complete" />
                  </button>
                </template>
                <span class="grow" />
                <span v-if="modifiedLabel" class="tm-modified">{{ modifiedLabel }}</span>
              </div>

              <div class="tm-head-text">
                <textarea
                  ref="titleEl"
                  v-model="title"
                  rows="1"
                  class="tm-title"
                  :placeholder="t('task.head.titlePlaceholder')"
                  :readonly="ro"
                  data-testid="task-title"
                  @input="fitTitle"
                  @blur="saveTitle"
                  @keydown.enter.prevent="onTitleEnter"
                />
                <div class="tm-meta">
                  <span v-if="task?.number">#{{ task.number }}</span>
                  <span v-if="task?.number && createdLabel">·</span>
                  <span>{{ createdLabel }}</span>
                </div>
              </div>

              <!-- The properties: beside the content when there is room, in the
                   flow right here otherwise (CSS moves the one panel). -->
              <Teleport :to="sideEl" :disabled="!wide || !sideEl">
                <aside v-if="!wide || !rightHidden" class="tm-panel">
                  <div class="tp-sec">{{ t('task.panel.props') }}</div>
                  <div class="props" :class="{ 'tm-ro': ro }">
                    <!-- status -->
                    <div class="prow">
                      <span class="plabel">{{ t('task.field.status') }}</span>
                      <n-popover
                        v-if="!ro && sortedCols.length"
                        :show="fieldOpen === 'statusPanel'"
                        trigger="click"
                        placement="bottom-start"
                        @update:show="setField('statusPanel', $event)"
                      >
                        <template #trigger>
                          <button class="val">
                            <TesseraIcon
                              :name="statusIcon(currentColumn)"
                              :size="14"
                              :style="{ color: currentColumn?.color || 'var(--t-text3)' }"
                            />
                            <span>{{
                              columnName(currentColumn) || t('task.value.columnNone')
                            }}</span>
                          </button>
                        </template>
                        <PopMenu class="pmenu">
                          <div
                            v-for="c in sortedCols"
                            :key="c.id"
                            class="menu-item col-item"
                            :class="{ cur: c.id === task?.column_id }"
                            @click="pickColumn(c.id)"
                          >
                            <TesseraIcon
                              :name="statusIcon(c)"
                              :size="14"
                              :style="{ color: c.color || 'var(--t-text3)' }"
                            />
                            <span>{{ columnName(c) }}</span>
                          </div>
                        </PopMenu>
                      </n-popover>
                      <span v-else class="val static">
                        <TesseraIcon
                          :name="statusIcon(currentColumn)"
                          :size="14"
                          :style="{ color: currentColumn?.color || 'var(--t-text3)' }"
                        />
                        <span>{{ columnName(currentColumn) || '—' }}</span>
                      </span>
                      <ShortcutKeys id="task.status" class="pkey" />
                    </div>

                    <!-- priority -->
                    <!-- data-tour-set marks a filled field for the Get Started guide
                       (#2759): its steps end when the user actually sets the value,
                       not when the picker was opened. Absent (not empty) when unset,
                       so [data-tour-set] alone selects the filled ones. -->
                    <div class="prow" data-tour="tm-priority" :data-tour-set="priority ? '' : null">
                      <span class="plabel">{{ t('task.field.priority') }}</span>
                      <n-popover
                        :show="fieldOpen === 'priority'"
                        trigger="click"
                        placement="bottom-start"
                        @update:show="setField('priority', $event)"
                      >
                        <template #trigger>
                          <button class="val">
                            <span v-if="priority" class="prio-pill" :style="priorityTone">
                              <n-icon :component="FlagOutline" :size="13" />
                              {{ priorityLabel(priority) }}
                            </span>
                            <span v-else class="muted">{{ priorityLabel(0) }}</span>
                          </button>
                        </template>
                        <PopMenu>
                          <div
                            v-for="o in priorityOptions"
                            :key="o.value"
                            class="menu-item"
                            @click="pickPriority(o.value)"
                          >
                            <span
                              class="dot"
                              :style="{ background: hueGrad(PRIORITY_COLORS[o.value]) }"
                            />
                            <span class="grow">{{ o.label }}</span>
                            <span class="muted">{{ o.value }}</span>
                          </div>
                        </PopMenu>
                      </n-popover>
                      <ShortcutKeys id="task.priorityPick" class="pkey" />
                    </div>

                    <!-- assignees -->
                    <div
                      class="prow"
                      data-tour="tm-assignees"
                      :data-tour-set="assigneeObjs.length || glAssignees.length ? '' : null"
                    >
                      <span class="plabel">{{ t('task.field.assignees') }}</span>
                      <n-popover
                        :show="fieldOpen === 'assignees'"
                        trigger="click"
                        placement="bottom-start"
                        @update:show="setField('assignees', $event)"
                      >
                        <template #trigger>
                          <button class="val val-people">
                            <span v-if="assigneeObjs.length || glAssignees.length" class="cascade">
                              <UserAvatar
                                v-for="u in assigneeObjs.slice(0, 4)"
                                :key="u.user_id"
                                class="avatar"
                                :user-id="u.user_id"
                                :name="u.name"
                                :title="u.name"
                              />
                              <UserAvatar
                                v-for="(g, i) in glAssignees.slice(0, 4)"
                                :key="`g${i}`"
                                class="avatar ext-ava"
                                :src="g.gl_avatar_url"
                                :name="g.gl_name || g.gl_username"
                                :title="`${g.gl_name || g.gl_username} (GitLab)`"
                              />
                            </span>
                            <span v-if="assigneeNames" class="people-names">{{
                              assigneeNames
                            }}</span>
                            <span v-else class="muted">{{ t('task.value.assigneesNone') }}</span>
                          </button>
                        </template>
                        <PopMenu class="pmenu">
                          <div
                            v-for="m in members"
                            :key="m.user_id"
                            class="menu-item"
                            @click="toggleAssignee(m.user_id)"
                          >
                            <UserAvatar class="avatar sm" :user-id="m.user_id" :name="m.name" />
                            <span class="grow">{{ m.name }}</span>
                            <n-icon
                              v-if="selectedAssignees.includes(m.user_id)"
                              :component="CheckmarkOutline"
                              class="chk"
                            />
                          </div>
                          <template v-if="glPickerList.length">
                            <div class="menu-sep">GitLab</div>
                            <div
                              v-for="m in glPickerList"
                              :key="m.gl_user_id"
                              class="menu-item"
                              @click="toggleGlAssignee(m)"
                            >
                              <UserAvatar
                                class="avatar sm"
                                :src="m.gl_avatar_url"
                                :name="m.gl_name || m.gl_username"
                              />
                              <span class="grow">{{ m.gl_name || m.gl_username }}</span>
                              <n-icon
                                v-if="isGlAssigned(m.gl_username)"
                                :component="CheckmarkOutline"
                                class="chk"
                              />
                            </div>
                          </template>
                        </PopMenu>
                      </n-popover>
                      <ShortcutKeys id="task.assignees" class="pkey" />
                    </div>

                    <!-- author (read-only) -->
                    <div v-if="author" class="prow">
                      <span class="plabel">{{ t('task.field.author') }}</span>
                      <div
                        class="val static"
                        :title="author.gl ? `@${author.login} · GitLab` : author.name"
                      >
                        <UserAvatar
                          class="avatar sm"
                          :user-id="author.id"
                          :src="author.avatar"
                          :name="author.name"
                        />
                        <span class="author-name">{{ author.name }}</span>
                        <span v-if="author.gl" class="author-gl">GitLab</span>
                      </div>
                    </div>

                    <!-- due -->
                    <div class="prow" data-tour="tm-due" :data-tour-set="dueTs ? '' : null">
                      <span class="plabel">{{ t('task.field.due') }}</span>
                      <n-popover
                        :show="fieldOpen === 'due'"
                        trigger="click"
                        placement="bottom-start"
                        @update:show="setField('due', $event)"
                      >
                        <template #trigger>
                          <button class="val">
                            <n-icon :component="CalendarClearOutline" :size="14" class="vico" />
                            <span :class="{ muted: !dueLabel }">{{
                              dueLabel || t('task.value.dueUnset')
                            }}</span>
                            <n-icon
                              v-if="recurrence"
                              :component="RepeatOutline"
                              :size="14"
                              class="recur-mark"
                              :title="t('task.field.recurring')"
                            />
                          </button>
                        </template>
                        <DueEditor
                          :due="dueTs"
                          :start="startTs"
                          :recurrence="recurrence"
                          :notify="dueNotify"
                          :columns="columns"
                          @apply="onDueApply"
                          @notify="onDueNotify"
                        />
                      </n-popover>
                      <ShortcutKeys id="task.due" class="pkey" />
                    </div>

                    <!-- estimate -->
                    <div class="prow">
                      <span class="plabel">{{ t('task.field.estimate') }}</span>
                      <n-popover
                        :show="fieldOpen === 'estimate'"
                        trigger="click"
                        placement="bottom-start"
                        @update:show="onEstShow"
                      >
                        <template #trigger>
                          <button class="val">
                            <n-icon :component="TimeOutline" :size="14" class="vico" />
                            <span :class="{ muted: !estFull }">{{
                              estFull || t('task.value.estimateUnset')
                            }}</span>
                            <span v-if="estRange" class="est-range">· {{ estRange }}</span>
                            <span
                              v-if="subtaskEstimateLabel"
                              class="est-rollup"
                              :title="t('task.estimate.rollupHint')"
                              >Σ {{ subtaskEstimateLabel }}</span
                            >
                          </button>
                        </template>
                        <div class="est-pop">
                          <PopMenu v-if="estOptions.length" class="est-menu">
                            <div class="menu-item" @click="clearEstimate">
                              <span class="grow muted">{{ t('task.value.estimateUnset') }}</span>
                              <n-icon
                                v-if="estimate == null"
                                :component="CheckmarkOutline"
                                class="chk"
                              />
                            </div>
                            <div
                              v-for="o in estOptions"
                              :key="o.value"
                              class="menu-item"
                              @click="setEstimate(o.value)"
                            >
                              <span class="grow">{{ o.label }}</span>
                              <n-icon
                                v-if="estimate === o.value"
                                :component="CheckmarkOutline"
                                class="chk"
                              />
                            </div>
                          </PopMenu>
                          <div v-else class="est-edit">
                            <n-input
                              v-model:value="estInput"
                              size="small"
                              :placeholder="estPlaceholder"
                              @keydown.esc.stop="fieldOpen = ''"
                              @keydown.enter.prevent="applyEstInput"
                            />
                            <div class="est-actions">
                              <n-button size="tiny" tertiary @click="clearEstimate">{{
                                t('task.estimate.clear')
                              }}</n-button>
                              <n-button size="tiny" type="primary" @click="applyEstInput">{{
                                t('task.estimate.apply')
                              }}</n-button>
                            </div>
                          </div>
                        </div>
                      </n-popover>
                    </div>

                    <!-- milestone («Этап») -->
                    <div class="prow">
                      <span class="plabel">{{ t('task.field.milestone') }}</span>
                      <n-popover
                        :show="fieldOpen === 'milestone'"
                        trigger="click"
                        placement="bottom-start"
                        @update:show="setField('milestone', $event)"
                      >
                        <template #trigger>
                          <button class="val">
                            <n-icon :component="TrailSignOutline" :size="14" class="vico" />
                            <span :class="{ muted: !taskMilestone }">
                              {{
                                taskMilestone ? taskMilestone.title : t('task.value.milestoneUnset')
                              }}
                            </span>
                            <span v-if="taskMilestone?.state === 'closed'" class="est-range"
                              >· {{ t('task.value.milestoneClosed') }}</span
                            >
                          </button>
                        </template>
                        <div class="ms-pop">
                          <PopMenu class="ms-menu">
                            <div class="menu-item" @click="setMilestone(null)">
                              <span class="grow muted">{{ t('task.value.milestoneUnset') }}</span>
                              <n-icon
                                v-if="!task?.milestone_id"
                                :component="CheckmarkOutline"
                                class="chk"
                              />
                            </div>
                            <div
                              v-for="m in milestoneOptions"
                              :key="m.id"
                              class="menu-item"
                              @click="setMilestone(m.id)"
                            >
                              <span
                                class="grow ms-opt"
                                :class="{ 'ms-closed': m.state === 'closed' }"
                              >
                                <span class="ms-opt-title">{{ m.title }}</span>
                                <span v-if="milestoneRange(m, formatters)" class="ms-opt-range">{{
                                  milestoneRange(m, formatters)
                                }}</span>
                              </span>
                              <n-icon
                                v-if="task?.milestone_id === m.id"
                                :component="CheckmarkOutline"
                                class="chk"
                              />
                            </div>
                          </PopMenu>
                          <div class="ms-new">
                            <n-input
                              v-model:value="newMilestoneTitle"
                              size="small"
                              :placeholder="t('task.milestone.newPlaceholder')"
                              @keydown.esc.stop="fieldOpen = ''"
                              @keydown.enter.prevent="createMilestone"
                            />
                            <n-button
                              size="small"
                              type="primary"
                              :disabled="!newMilestoneTitle.trim()"
                              @click="createMilestone"
                            >
                              {{ t('common.action.create') }}
                            </n-button>
                          </div>
                        </div>
                      </n-popover>
                    </div>

                    <!-- tags -->
                    <div
                      class="prow"
                      data-tour="tm-tags"
                      :data-tour-set="tagObjs.length ? '' : null"
                    >
                      <span class="plabel">{{ t('task.field.tags') }}</span>
                      <n-popover
                        :show="fieldOpen === 'tags'"
                        trigger="click"
                        placement="bottom-start"
                        @update:show="setField('tags', $event)"
                      >
                        <template #trigger>
                          <button ref="tagsValEl" class="val tags-val">
                            <template v-if="tagObjs.length">
                              <TagPill
                                v-for="tg in tagObjs.slice(0, visibleTagCount)"
                                :key="tg.id"
                                class="chip"
                                :tag="tg"
                                :prefix-names="tagPrefixNames"
                                variant="outline"
                              />
                              <span
                                v-if="visibleTagCount < tagObjs.length"
                                class="chip chip-more"
                                :style="{
                                  color: tagText(tagObjs[0].color),
                                  background: softFill(tagObjs[0].color),
                                }"
                                >+{{ tagObjs.length - visibleTagCount }}</span
                              >
                              <!-- invisible measurement row: natural chip widths, never
                                 sliced. Same component + props as above, else the scope
                                 segment wouldn't be measured and the fit would lie. -->
                              <span ref="tagsMeasureEl" class="tags-measure" aria-hidden="true">
                                <TagPill
                                  v-for="tg in tagObjs"
                                  :key="`m${tg.id}`"
                                  class="chip"
                                  :tag="tg"
                                  :prefix-names="tagPrefixNames"
                                  variant="outline"
                                />
                              </span>
                            </template>
                            <span v-else class="muted">{{ t('task.value.tagsNone') }}</span>
                          </button>
                        </template>
                        <PopMenu>
                          <div class="chip-groups">
                            <div v-for="g in tagPickerGroups" :key="g.key" class="chip-group">
                              <div v-if="tagPickerHeaders" class="chip-grp-head">{{ g.label }}</div>
                              <div class="chip-grid">
                                <button
                                  v-for="tg in g.tags"
                                  :key="tg.id"
                                  class="tagchip"
                                  :class="{ on: selectedTags.includes(tg.id) }"
                                  :style="
                                    selectedTags.includes(tg.id)
                                      ? {
                                          background: hueGrad(tg.color),
                                          color: onColor(tg.color),
                                          borderColor: 'transparent',
                                        }
                                      : {
                                          background: softFill(tg.color),
                                          color: tagText(tg.color),
                                          borderColor: (tg.color || '#888') + '66',
                                        }
                                  "
                                  @click="toggleTag(tg.id)"
                                >
                                  <TagPill
                                    :tag="tg"
                                    :prefix-names="tagPrefixNames"
                                    variant="inherit"
                                    :scope-mode="tagPickerHeaders ? 'hide' : 'auto'"
                                  />
                                </button>
                              </div>
                            </div>
                          </div>
                          <n-input
                            v-model:value="newTagName"
                            size="tiny"
                            :placeholder="t('task.tags.newPlaceholder')"
                            @keydown.esc.stop="fieldOpen = ''"
                            @keyup.enter="createTag"
                          />
                          <!-- The project's tag table (#2990): the «Проект» window on
                               «Теги», filtered by what was typed here. -->
                          <button
                            type="button"
                            class="menu-item tags-manage"
                            data-testid="task-tags-manage"
                            @click="manageTags"
                          >
                            <n-icon :component="SettingsOutline" :size="14" class="vico" />
                            <span class="grow">{{ t('task.tags.manage') }}</span>
                            <span class="muted">{{ t('task.tags.manageWhere') }}</span>
                          </button>
                        </PopMenu>
                      </n-popover>
                      <ShortcutKeys id="task.tags" class="pkey" />
                    </div>

                    <!-- parent -->
                    <div class="prow">
                      <span class="plabel">{{ t('task.field.parent') }}</span>
                      <div v-if="task?.parent_id" class="parent-row">
                        <button
                          class="val parent-chip"
                          :title="parentTask?.title || ''"
                          @click="emit('open', task.parent_id)"
                        >
                          <span class="parent-title">{{ parentLabel }}</span>
                        </button>
                        <button v-if="!ro" class="val detach" @click="detachFromParent">
                          {{ t('task.parent.detach') }}
                        </button>
                      </div>
                      <n-popover
                        v-else
                        :show="fieldOpen === 'parent'"
                        trigger="click"
                        placement="bottom-start"
                        @update:show="setField('parent', $event)"
                      >
                        <template #trigger>
                          <button class="val">
                            <span class="muted">{{ t('task.parent.makeSubtask') }}</span>
                          </button>
                        </template>
                        <PopMenu class="pmenu">
                          <div
                            v-for="cand in parentCandidates"
                            :key="cand.id"
                            class="menu-item"
                            @click="attachTo(cand.id)"
                          >
                            {{ cand.title }}
                          </div>
                          <span v-if="!parentCandidates.length" class="muted small">{{
                            t('task.parent.noCandidates')
                          }}</span>
                        </PopMenu>
                      </n-popover>
                    </div>
                  </div>

                  <!-- Разработка: the GitLab issue this task mirrors, or the way to make one. -->
                  <template v-if="task?.gitlab || (task && gitlabCanCreate && !ro)">
                    <div class="tp-sec">{{ t('task.panel.dev') }}</div>
                    <div v-if="task.gitlab" class="dev-card">
                      <a
                        class="dev-line"
                        :href="task.gitlab.web_url"
                        target="_blank"
                        rel="noopener noreferrer"
                        :title="t('task.head.openInGitlab', { iid: task.gitlab.iid })"
                      >
                        <n-icon :component="LogoGitlab" :size="15" class="dev-ico" />
                        <b>!{{ task.gitlab.iid }}</b>
                        <span class="dev-title">{{ t('task.panel.gitlabIssue') }}</span>
                        <n-icon :component="OpenOutline" :size="13" class="muted" />
                      </a>
                      <div class="dev-sub">
                        <span class="dev-path">{{ task.gitlab.project_path }}</span>
                      </div>
                      <!-- grouped-task marker (#2592): the parent half of the GitLab
                         hierarchy. A button and not a tag — the grouping label lives in
                         a prefix the pull rules own. -->
                      <div v-if="gitlabCanGroup" class="dev-group">
                        <n-icon :component="LayersOutline" :size="13" class="muted" />
                        <span v-if="task.gitlab.is_group" class="group-chip">{{
                          t('task.gitlab.grouped')
                        }}</span>
                        <button
                          v-if="!ro"
                          class="val"
                          :disabled="glGrouping"
                          @click="toggleGlGroup"
                        >
                          <span class="muted">{{ glGroupAction }}</span>
                        </button>
                      </div>
                    </div>
                    <button v-else class="dev-create" :disabled="glCreating" @click="createGlIssue">
                      <n-icon :component="LogoGitlab" :size="15" />
                      {{ glCreating ? t('task.gitlab.creating') : t('task.gitlab.create') }}
                    </button>
                  </template>

                  <div class="tp-sec" data-testid="tab-relations" data-tour="tm-links">
                    {{ t('task.panel.links') }}
                  </div>
                  <TaskRelationsTab
                    v-model:relations="relations"
                    :task-id="taskId"
                    :ws-id="wsId"
                    @changed="emit('changed')"
                    @open-related="openRelated"
                  />

                  <template v-if="docLinks.length">
                    <div class="tp-sec" data-testid="tab-documents">
                      {{ t('task.panel.documents') }}
                    </div>
                    <TaskDocumentsTab v-model:links="docLinks" @open-document="openDocument" />
                  </template>
                </aside>
              </Teleport>

              <TaskDescriptionTab
                v-model="description"
                v-model:template-value="glTemplate"
                class="tm-desc"
                data-tour="tm-description"
                :data-tour-set="descSaved ? '' : null"
                :readonly="ro"
                :members="mentionItems"
                :task-id="taskId"
                :initial-mode="descInitialMode"
                :show-template="
                  !!(task && !task.gitlab && gitlabFetchTemplates && glTemplates.length)
                "
                :template-options="glTemplateOptions"
                @apply-template="applyGlTemplate"
                @attachments-changed="reloadAttachments"
                @blur="saveDesc"
                @persist="saveDesc"
              />

              <TaskSubtasksTab
                ref="subtasksTab"
                class="tm-subs"
                data-tour="tm-subtasks"
                :task="task"
                :columns="columns"
                :readonly="ro"
                :gitlab-can-group="gitlabCanGroup"
                :done-column-id="doneCol?.id || null"
                @open="emit('open', $event)"
                @changed="onSubtaskChanged"
                @group-parent="toggleGlGroup"
              />

              <TaskInboxSection ref="inboxSection" class="tm-inbox" :task="task" :readonly="ro" />

              <section class="tm-activity" data-tour="tm-activity">
                <div class="act-head">
                  <span class="act-title">{{ t('task.activity.title') }}</span>
                  <div class="seg" role="tablist">
                    <button
                      type="button"
                      class="seg-btn"
                      :class="{ on: activityMode === 'all' }"
                      data-testid="tab-history"
                      @click="activityMode = 'all'"
                    >
                      {{ t('task.activity.all') }}
                    </button>
                    <button
                      type="button"
                      class="seg-btn"
                      :class="{ on: activityMode === 'comments' }"
                      data-testid="tab-comments"
                      @click="activityMode = 'comments'"
                    >
                      {{ t('task.activity.comments') }}
                      <span v-if="comments.length" class="seg-n">· {{ comments.length }}</span>
                    </button>
                    <button
                      type="button"
                      class="seg-btn"
                      :class="{ on: activityMode === 'files' }"
                      data-testid="tab-files"
                      @click="activityMode = 'files'"
                    >
                      {{ t('task.activity.files') }}
                      <span v-if="attachments.length" class="seg-n"
                        >· {{ attachments.length }}</span
                      >
                    </button>
                  </div>
                </div>
                <TaskCommentsTab
                  v-show="activityMode !== 'files'"
                  ref="commentsTab"
                  v-model:comments="comments"
                  :task-id="taskId"
                  :readonly="ro"
                  :hydrated="commentsHydrated"
                  :mode="activityMode === 'all' ? 'all' : 'comments'"
                  :events="events"
                  :composer-to="composerEl"
                  :composer-rows="1"
                  @changed="emit('changed')"
                  @reload-detail="loadDetail"
                />
                <TaskFilesTab
                  v-if="activityMode === 'files'"
                  v-model:attachments="attachments"
                  :task-id="taskId"
                  @changed="emit('changed')"
                />
              </section>
            </div>
            <!-- The comments composer is teleported here: pinned under the column,
                 outside its scroll (TaskCommentsTab, composer-to). -->
            <div v-show="!ro" ref="composerEl" class="tm-composer" />
          </div>
          <div v-show="wide && !rightHidden" ref="sideEl" class="tm-side" />
        </div>
      </n-spin>
    </div>
  </n-modal>
</template>

<style scoped>
@import './task/tab-shared.css';

/* ── v2 card (#2958): 1080×800, radius 18, the pop level, the drag shadow ── */
.tm-card {
  display: flex;
  flex-direction: column;
  width: min(1080px, 96vw);
  height: min(800px, 92vh);
  border-radius: var(--t-r-col);
  background: var(--t-pop);
  box-shadow: var(--t-sh-drag);
  color: var(--t-text1);
  /* Popovers inside teleport into this card (#2807), so it must not clip:
     the columns scroll on their own instead. */
  --t-surface: var(--t-pop);
}
/* Narrow screens: one column, the card as tall as its content allows. */
.tm-card:not(.tm-wide):not(.tm-sidebar):not(.tm-fullscreen):not(.tm-phone) {
  width: min(680px, 94vw);
  height: auto;
  max-height: 92vh;
}
/* Edge-to-edge. */
.tm-card.tm-fullscreen {
  width: 100vw;
  height: 100vh;
  max-height: none;
  border-radius: 0;
}
/* Right-hand panel: pinned to the edge, board visible (and usable) beside it.
   The shadow is load-bearing — without a mask the panel would otherwise bleed
   into the board behind it. */
.tm-card.tm-sidebar {
  position: fixed;
  inset: 0 0 0 auto;
  width: min(560px, 100vw);
  height: 100vh;
  max-height: none;
  border-radius: var(--t-r-col) 0 0 var(--t-r-col);
}
/* Slide in from the right edge and back out to it — a panel docked to the edge
   has to look like it came from there. Naive puts the transition classes on the
   card itself; three classes out-specify its two. */
.tm-card.tm-sidebar.fade-in-scale-up-transition-enter-from,
.tm-card.tm-sidebar.fade-in-scale-up-transition-leave-to {
  opacity: 1;
  transform: translateX(100%);
}
.tm-card.tm-sidebar.fade-in-scale-up-transition-enter-to,
.tm-card.tm-sidebar.fade-in-scale-up-transition-leave-from {
  opacity: 1;
  transform: translateX(0);
}
.tm-card.tm-sidebar.fade-in-scale-up-transition-enter-active,
.tm-card.tm-sidebar.fade-in-scale-up-transition-leave-active {
  transition: transform 0.24s cubic-bezier(0.4, 0, 0.2, 1);
}
@media (prefers-reduced-motion: reduce) {
  .tm-card.tm-sidebar.fade-in-scale-up-transition-enter-active,
  .tm-card.tm-sidebar.fade-in-scale-up-transition-leave-active {
    transition: none;
  }
}

/* The spin wraps the body; both have to pass the height down. */
.tm-spin {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}
.tm-spin :deep(.n-spin-content) {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}
.tm-body {
  flex: 1;
  min-height: 0;
  display: flex;
}
.tm-main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
}
.tm-scroll {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 20px;
  padding: 6px 36px 12px 32px;
  scrollbar-width: thin;
  scrollbar-color: transparent transparent;
}
.tm-scroll:hover,
.tm-scroll:focus-within {
  scrollbar-color: var(--t-scroll-thumb, rgba(128, 128, 140, 0.35)) transparent;
}
.tm-composer {
  flex: none;
  padding: 10px 36px 18px 32px;
}
.tm-composer:empty {
  display: none;
}
/* The properties panel's column (wide layouts): one level below the card. */
.tm-side {
  flex: 0 0 330px;
  width: 330px;
  min-width: 0;
  min-height: 0;
  display: flex;
  margin: 0 12px 12px 0;
}
.tm-side > .tm-panel {
  flex: 1;
  min-width: 0;
  min-height: 0;
  overflow-y: auto;
  scrollbar-width: thin;
}
.tm-panel {
  display: flex;
  flex-direction: column;
  padding: 4px 12px 16px;
  border-radius: 14px;
  background: var(--t-sub);
  --t-surface: var(--t-sub);
}
/* Stacked (narrow / side panel): the panel sits in the flow after the title. */
.tm-scroll > .tm-panel {
  flex: none;
}
.tm-card:not(.tm-wide) .tm-scroll {
  padding: 6px 20px 12px;
}
.tm-card:not(.tm-wide) .tm-composer {
  padding: 10px 20px 16px;
}

/* ── read-only (archive) ── */
.tm-ro-banner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 8px 8px 8px 14px;
  border-radius: 12px;
  background: color-mix(in srgb, #f0a020 14%, var(--t-pop));
  color: var(--t-text2);
  font-size: 13px;
}

/* ── action row: status · shift · complete · modified ── */
.tm-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  min-height: 30px;
}
.tm-status {
  height: 30px;
  max-width: 260px;
  padding: 0 8px 0 9px;
  gap: 7px;
  border: none;
  border-radius: 9px;
  color: var(--t-text1);
  font: inherit;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
}
.tm-status.static {
  cursor: default;
}
.tm-status-ico {
  flex: none;
  color: var(--st-c);
}
.tm-status-chev {
  flex: none;
  color: var(--t-text3);
}
.st-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 7px;
  height: 30px;
  min-width: 30px;
  padding: 0 8px;
  border: none;
  border-radius: 9px;
  background: var(--t-pill);
  color: var(--t-text2);
  font: inherit;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
  transition: background 0.12s ease;
}
.st-btn:hover:not(:disabled) {
  background: var(--t-hover);
  color: var(--t-text1);
}
.st-btn:disabled {
  opacity: 0.4;
  cursor: default;
}
.tm-done {
  padding: 0 8px 0 10px;
}
.tm-done-ico {
  color: #18a058;
}
.tm-done:hover:not(:disabled) {
  background: color-mix(in srgb, #18a058 18%, var(--t-pop));
}
.tm-done.on {
  background: color-mix(in srgb, #18a058 16%, var(--t-pop));
  color: var(--t-text1);
}
.tm-modified {
  font-size: 12.5px;
  color: var(--t-text3);
  white-space: nowrap;
}

/* ── title + meta ── */
.tm-head-text {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.tm-title {
  display: block;
  width: 100%;
  margin: 0;
  padding: 0;
  border: none;
  outline: none;
  resize: none;
  overflow: hidden;
  background: transparent;
  color: var(--t-text1);
  font: inherit;
  font-size: 23px;
  line-height: 1.3;
  font-weight: 650;
  letter-spacing: -0.01em;
}
.tm-title::placeholder {
  color: var(--t-placeholder);
}
.tm-title:focus-visible {
  border-radius: 6px;
  box-shadow: 0 0 0 2px color-mix(in srgb, var(--t-primary) 40%, transparent);
}
.tm-meta {
  display: flex;
  gap: 6px;
  font-size: 13px;
  color: var(--t-text3);
}
.tm-desc {
  max-width: 680px;
}

/* ── activity ── */
.tm-activity {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.act-head {
  display: flex;
  align-items: center;
  gap: 12px;
}
.act-title {
  font-size: 14.5px;
  font-weight: 650;
}
.seg {
  display: inline-flex;
  gap: 2px;
  padding: 3px;
  border-radius: 9px;
  background: var(--t-pill);
}
.seg-btn {
  height: 24px;
  padding: 0 10px;
  border: none;
  border-radius: 7px;
  background: transparent;
  color: var(--t-text2);
  font: inherit;
  font-size: 12.5px;
  cursor: pointer;
  white-space: nowrap;
}
.seg-btn:hover {
  color: var(--t-text1);
}
.seg-btn.on {
  background: var(--t-pop);
  box-shadow: var(--t-sh-card);
  color: var(--t-text1);
  font-weight: 600;
}
.seg-n {
  color: var(--t-text3);
  font-weight: 400;
}

/* ── properties panel ── */
.tp-sec {
  padding: 14px 8px 6px;
  font-size: 11.5px;
  font-weight: 600;
  letter-spacing: 0.05em;
  text-transform: uppercase;
  color: var(--t-text3);
}
.props {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.prow {
  display: grid;
  grid-template-columns: 100px minmax(0, 1fr) auto;
  align-items: center;
  gap: 8px;
  min-height: 36px;
  padding: 2px 8px;
  border-radius: 9px;
}
.prow:hover {
  background: var(--t-hover);
}
.plabel {
  font-size: 13px;
  color: var(--t-text3);
}
.pkey {
  opacity: 0.6;
}
.val {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  min-width: 0;
  max-width: 100%;
  min-height: 28px;
  padding: 2px 0;
  border: none;
  background: transparent;
  color: var(--t-text1);
  font: inherit;
  font-size: 13px;
  text-align: left;
  cursor: pointer;
}
.val > span {
  min-width: 0;
}
.val.static {
  cursor: default;
}
.vico {
  flex: none;
  color: var(--t-text2);
}
.prio-pill {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  height: 24px;
  padding: 0 10px 0 8px;
  border-radius: 12px;
  font-size: 12.5px;
  font-weight: 500;
}
.val-people {
  flex-direction: column;
  align-items: flex-start;
  gap: 4px;
  padding: 4px 0;
}
.cascade {
  display: inline-flex;
}
.people-names {
  font-size: 12.5px;
  color: var(--t-text3);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 100%;
}
.avatar {
  flex: none;
  width: 22px;
  height: 22px;
  border-radius: 50%;
  background: var(--t-accent-grad);
  color: var(--t-on-primary);
  font-size: 9px;
  font-weight: 600;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  margin-left: -6px;
  box-shadow: 0 0 0 2px var(--t-sub);
}
.avatar:first-child {
  margin-left: 0;
}
.avatar.sm {
  width: 20px;
  height: 20px;
  margin-left: 0;
  box-shadow: none;
}
/* external GitLab assignee avatar (no Tessera account) */
.ext-ava {
  background: var(--t-text3);
}
.author-name {
  color: var(--t-text1);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.author-gl {
  color: var(--t-text3);
  font-size: 12px;
}
.recur-mark {
  color: var(--t-primary);
}
.muted {
  color: var(--t-text3);
}
.grow {
  flex: 1;
}
.dot {
  flex: none;
  width: 9px;
  height: 9px;
  border-radius: 50%;
}
.chip {
  font-size: 11px;
  padding: 1px 8px;
  border-radius: 10px;
  flex: none;
  white-space: nowrap;
}
.chip-more {
  color: var(--t-text3);
  background: var(--t-pill);
}
/* Tags trigger: one line; only the chips that wholly fit are rendered (+ a
   measured "+N"); the measurement row is invisible. */
.tags-manage {
  margin-top: 6px;
  width: 100%;
  border: 0;
  background: transparent;
  font: inherit;
  text-align: left;
}
.tags-manage .muted {
  font-size: 11.5px;
}
.tags-val {
  overflow: hidden;
  flex-wrap: nowrap;
  gap: 5px;
}
.tags-measure {
  position: absolute;
  visibility: hidden;
  pointer-events: none;
  white-space: nowrap;
  left: -9999px;
  display: inline-flex;
  gap: 6px;
}
/* ── parent row (#2861): [parent title …] [Открепить] ── */
.parent-row {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}
.parent-chip {
  min-width: 0;
}
.parent-title {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.parent-row .detach {
  flex: none;
  color: var(--t-text3);
}
/* ── «Разработка» ── */
.dev-card {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin: 0 2px;
  padding: 10px 12px;
  border-radius: 11px;
  background: var(--t-pop);
  transition: box-shadow 0.12s ease;
}
.dev-card:hover {
  box-shadow: var(--t-sh-card);
}
.dev-line {
  display: flex;
  align-items: center;
  gap: 7px;
  min-width: 0;
  color: var(--t-text1);
  font-size: 13px;
  text-decoration: none;
}
.dev-ico {
  flex: none;
  color: #e24329;
}
.dev-title {
  flex: 1;
  min-width: 0;
  color: var(--t-text2);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.dev-sub {
  font-size: 12px;
  color: var(--t-text3);
}
.dev-path {
  font-family: ui-monospace, SFMono-Regular, 'JetBrains Mono', Menlo, Consolas, monospace;
}
.dev-group {
  display: flex;
  align-items: center;
  gap: 8px;
}
/* A fact about the issue in GitLab, not an action — a neutral pill. */
.group-chip {
  padding: 1px 8px;
  border-radius: 999px;
  background: var(--t-pill);
  color: var(--t-text2);
  font-size: 11px;
  white-space: nowrap;
}
.dev-create {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  margin: 0 2px;
  height: 34px;
  padding: 0 12px;
  border: none;
  border-radius: 10px;
  background: var(--t-pop);
  color: var(--t-text2);
  font: inherit;
  font-size: 13px;
  cursor: pointer;
}
.dev-create:hover:not(:disabled) {
  color: var(--t-text1);
  box-shadow: var(--t-sh-card);
}

/* ── popover menus ── */
.menu-item.danger {
  color: #e0533d;
}
a.menu-item {
  color: inherit;
  text-decoration: none;
}
.menu-sep {
  font-size: 10px;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  opacity: 0.6;
  padding: 6px 6px 2px;
}
.chk {
  color: var(--t-primary);
}
.small {
  font-size: 12px;
  padding: 4px;
}
/* estimate rollup hint on the value + popover editor */
.est-rollup,
.est-range {
  color: var(--t-text3);
  font-size: 12px;
  white-space: nowrap;
}
.est-menu {
  min-width: 160px;
  max-height: 280px;
  overflow-y: auto;
}
.est-edit {
  display: flex;
  flex-direction: column;
  gap: 8px;
  width: 220px;
}
.est-actions {
  display: flex;
  justify-content: flex-end;
  gap: 6px;
}
.ms-pop {
  /* Fit the widest option (title + date range on one line) up to a cap. */
  width: max-content;
  min-width: 240px;
  max-width: 380px;
}
.ms-menu {
  max-height: 260px;
  overflow-y: auto;
}
.ms-closed {
  opacity: 0.6;
}
.ms-opt {
  display: flex;
  align-items: baseline;
  gap: 6px;
  min-width: 0;
}
.ms-opt-title {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.ms-opt-range {
  flex: none;
  color: var(--t-text3);
  font-size: 11px;
  white-space: nowrap;
}
.ms-new {
  display: flex;
  gap: 6px;
  align-items: center;
  margin-top: 8px;
}
.chip-groups {
  max-height: 320px;
  overflow-y: auto;
  max-width: 260px;
}
.chip-grp-head {
  font-size: 10px;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  opacity: 0.6;
  margin: 6px 0 4px;
}
.chip-group:first-child .chip-grp-head {
  margin-top: 0;
}
.chip-grid {
  display: flex;
  flex-wrap: wrap;
  gap: 5px;
  margin-bottom: 8px;
  max-width: 260px;
}
.tagchip {
  font-size: 12px;
  padding: 2px 9px;
  border-radius: 10px;
  border: 1px solid transparent;
  background: transparent;
  cursor: pointer;
}
.tagchip.on {
  font-weight: 600;
}
/* Read-only (archive) modal: property values become display-only. */
.props.tm-ro :is(button, .n-select, .n-tag, [role='button']) {
  pointer-events: none;
}
.props.tm-ro .pkey {
  display: none;
}
/* A phone: no room for key hints in the rows, nor for the long ones in the
   action row. */
@media (max-width: 640px) {
  .pkey,
  .tm-actions :deep(.sk),
  .tm-modified {
    display: none;
  }
  .tm-card:not(.tm-wide) .tm-scroll {
    padding: 4px 14px 12px;
  }
  .tm-card:not(.tm-wide) .tm-composer {
    padding: 8px 14px 14px;
  }
}

/* ── a phone: the full-height sheet (#2976 — package G «Задача») ── */
.tm-card.tm-phone {
  /* In the modal's own centring box, pinned to its bottom edge. */
  position: relative;
  align-self: flex-end;
  margin: auto 0 0;
  width: 100vw;
  height: calc(var(--app-vh, 100vh) - var(--safe-top) - 8px);
  max-height: none;
  border-radius: 18px 18px 0 0;
  transition: transform 0.2s ease;
}
.tm-card.tm-phone.tm-pulling {
  transition: none;
}
.tm-grab {
  flex: none;
  display: flex;
  justify-content: center;
  height: 20px;
  padding-top: 8px;
  box-sizing: border-box;
  touch-action: none;
  cursor: grab;
}
.tm-handle {
  width: 36px;
  height: 5px;
  border-radius: 3px;
  background: var(--t-kbd);
}
.tm-card.tm-phone .prow {
  min-height: 44px;
}
.tm-card.tm-phone .tm-composer {
  padding-bottom: calc(12px + var(--safe-bottom));
  background: var(--t-sub);
}
</style>
