<script setup>
import { ref, watch, computed, nextTick, onBeforeUnmount } from 'vue'
import { useRouter } from 'vue-router'
import {
  NModal,
  NCard,
  NInput,
  NButton,
  NSpace,
  NPopover,
  NIcon,
  NSpin,
  NTabs,
  NTabPane,
  NSelect,
  NBadge,
  NPopconfirm,
  useMessage,
} from 'naive-ui'
import {
  FlagOutline,
  CalendarClearOutline,
  PeopleOutline,
  PersonOutline,
  PricetagOutline,
  CheckmarkDoneOutline,
  CheckmarkOutline,
  PlayForwardOutline,
  CheckmarkCircle,
  EllipseOutline,
  ArchiveOutline,
  ArrowUndoOutline,
  GitMergeOutline,
  LogoGitlab,
  RepeatOutline,
  AttachOutline,
  TrashOutline,
  TimerOutline,
  RibbonOutline,
  ChatbubbleEllipsesOutline,
  GitBranchOutline,
  TimeOutline,
  ChatbubbleEllipses,
  GitBranch,
  GitMerge,
  Attach,
  Time,
  ImageOutline,
  GitNetworkOutline,
  EyeOutline,
  CreateOutline,
  ExpandOutline,
  ShareSocialOutline,
  ChevronForwardOutline,
  ChevronBackOutline,
} from '@vicons/ionicons5'
import { tasks as tasksApi, boards as boardsApi, projects as projApi, gitlab as glApi } from '@/api'
import { storeToRefs } from 'pinia'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useBoardViewStore } from '@/stores/boardView'
import { PRIORITY_LABELS, PRIORITY_COLORS } from '@/styles/tokens'
import { hueGrad, softFill, readableHue, onColor } from '@/utils/gradient'
import { buildTagGroups } from '@/utils/tagGroups'
import { milestoneRange } from '@/utils/milestones'
import {
  sortedColumns,
  columnById,
  nextColumn,
  doneTarget,
  siblingNeighbors,
  columnTail,
} from '@/utils/status'
import { taskLink } from '@/utils/taskLink'
import { copyText } from '@/utils/clipboard'
import { useResponsive } from '@/composables/useResponsive'
import {
  formatEstimate,
  formatEstimateFull,
  estimateRangeShort,
  parseEstimate,
  scaleOptions,
  estimatePlaceholder,
  sumEstimates,
} from '@/utils/estimation'
import { useThemeStore } from '@/stores/theme'
import { useDateLocale } from '@/composables/useDateLocale'
import { useTagFit } from '@/composables/useTagFit'
import DueEditor from './DueEditor.vue'
import MarkdownEditor from './MarkdownEditor.vue'
import RichContent from './RichContent.vue'
import TagPill from './TagPill.vue'
import UserAvatar from './UserAvatar.vue'
import TesseraSpinner from './TesseraSpinner.vue'
import TaskCommentsTab from './task/TaskCommentsTab.vue'
import TaskSubtasksTab from './task/TaskSubtasksTab.vue'
import TaskRelationsTab from './task/TaskRelationsTab.vue'
import TaskFilesTab from './task/TaskFilesTab.vue'
import TaskHistoryTab from './task/TaskHistoryTab.vue'

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
})
const emit = defineEmits(['update:show', 'changed', 'open', 'restore'])

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
} = storeToRefs(bv)
const tagPrefixNames = bv.prefixNames
const wsId = computed(() => bv.wsId)
const projectId = computed(() => bv.projectId)

const store = useWorkspacesStore()
const theme = useThemeStore()
const { formatDue } = useDateLocale()
// Tag colour clamped for legible text on the active theme.
const tagText = (c) => readableHue(c, theme.isDark)
const router = useRouter()
const message = useMessage()
const loading = ref(false)
const task = ref(null)
const boardInfo = ref(null) // { name, projectId } for the breadcrumb
const parentCandidates = ref([]) // top-level tasks on the board (for attach)

// ── rich detail (#8): comments, relations, files, journal
// Open an existing description on the Preview tab; an empty one on Write.
const descInitialMode = ref('write')
// The description editor's toolbar lives in the section header (.desc-head); we drive
// it through the editor ref and mirror its write/preview mode here for the toggle icon.
const descEditor = ref(null)
const descMode = ref('write')

// ── wide-screen split pane: draggable divider between the left (properties +
// description) and right (tabs) columns, ratio persisted in localStorage. Only
// active in the two-column layout (≥1100px); a no-op in the stacked layout. ──
const SPLIT_KEY = 'tessera_task_split'
const SPLIT_MIN = 0.3
const SPLIT_MAX = 0.7
const formEl = ref(null)
const splitRatio = ref(loadSplitRatio())
function loadSplitRatio() {
  const v = parseFloat(localStorage.getItem(SPLIT_KEY))
  return v >= SPLIT_MIN && v <= SPLIT_MAX ? v : 0.46
}

// ── wide-screen only: hide the right column (tabs) to focus on the task's own
// fields + description. Two handles reach it (a header button and a double-click
// on the divider grip), since a bare 14px grip isn't a touch target. Persisted in
// localStorage (non-critical UX state). In the stacked layout the right column IS
// the comments in the main flow — hiding it would hide the conversation — so the
// button is gated on `wide` and the state is a no-op there. ──
const { isMobile: narrow } = useResponsive(1099)
const wide = computed(() => !narrow.value)
const PANE_KEY = 'tessera_task_pane'
const rightHidden = ref(localStorage.getItem(PANE_KEY) === 'hidden')
function toggleRightPane() {
  rightHidden.value = !rightHidden.value
  try {
    localStorage.setItem(PANE_KEY, rightHidden.value ? 'hidden' : 'shown')
  } catch {
    /* storage disabled — non-fatal */
  }
  // The composer's textarea can't measure its height while the panel is collapsed
  // (0-width) — recompute it once the panel is visible again, or it stays at
  // whatever it last measured.
  if (!rightHidden.value) nextTick(() => commentsTab.value?.autoGrow?.())
}

// grid-template-columns for .form (ignored in the stacked flex layout). The 14px
// middle track is the divider handle; when the right column is hidden its track
// collapses to 0fr (the .form transition animates the reflow).
const splitCols = computed(() => {
  if (rightHidden.value) return `minmax(0, 1fr) 14px 0fr`
  return `minmax(0, ${splitRatio.value}fr) 14px minmax(0, ${1 - splitRatio.value}fr)`
})
let splitDragging = false
function startSplitDrag(e) {
  // Don't start a resize when there's no right column to resize (collapsed or the
  // stacked layout) — dragging would otherwise "revive" the hidden panel.
  if (!formEl.value || rightHidden.value || !wide.value) return
  splitDragging = true
  e.preventDefault()
  window.addEventListener('pointermove', onSplitDrag)
  window.addEventListener('pointerup', endSplitDrag)
}
function onSplitDrag(e) {
  if (!splitDragging || !formEl.value) return
  const r = formEl.value.getBoundingClientRect()
  const ratio = (e.clientX - r.left) / r.width
  splitRatio.value = Math.min(SPLIT_MAX, Math.max(SPLIT_MIN, ratio))
}
function endSplitDrag() {
  splitDragging = false
  window.removeEventListener('pointermove', onSplitDrag)
  window.removeEventListener('pointerup', endSplitDrag)
  try {
    localStorage.setItem(SPLIT_KEY, String(splitRatio.value))
  } catch {
    /* storage disabled — non-fatal */
  }
}

const comments = ref([])
// Gates the enter fade to genuinely-new comments: false during a task's initial
// population (the whole thread shouldn't fade in on open), flipped true once loaded.
// Owned here because only this component knows when a population is a load and when
// it is a user posting; the tab just renders it.
const commentsHydrated = ref(false)
const commentsTab = ref(null)
const detailTabs = ref(null)

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
    message.error('Не удалось скопировать ссылку')
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

// The tab counters load async (comments/relations/files fetched after the modal
// opens). Naive measures the active-tab underline before the badge exists, so it
// stops short of the counter until you switch tabs. Re-sync the bar whenever a
// counter that can affect the active tab's width changes. Declared after all the
// count refs so the watch's initial getter run doesn't hit a TDZ.
watch(
  () => [
    comments.value.length,
    relations.value.length,
    attachments.value.length,
    task.value?.subtasks?.length,
  ],
  () => nextTick(() => detailTabs.value?.syncBarPosition?.()),
)

const title = ref('')
const description = ref('')
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

const priorityOptions = PRIORITY_LABELS.map((label, value) => ({ label, value }))

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
  estimateRangeShort(task.value?.start_date, estimate.value, estCfg.value),
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
function applyEstInput() {
  setEstimate(parseEstimate(estInput.value, estCfg.value))
}
function clearEstimate() {
  estInput.value = ''
  setEstimate(null)
}
// Prefill the free-text buffer with the current value when the popover opens.
function onEstShow(shown) {
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

// Location breadcrumb: group chain → project → board (resolved from the store).
const breadcrumb = computed(() => {
  const parts = []
  const proj = store.projects.find((p) => p.id === boardInfo.value?.projectId)
  if (proj) {
    const chain = []
    let gid = proj.group_id
    while (gid) {
      const g = store.groups.find((x) => x.id === gid)
      if (!g) break
      chain.unshift(g.name)
      gid = g.parent_id
    }
    parts.push(...chain, proj.name)
  }
  if (boardInfo.value?.name) parts.push(boardInfo.value.name)
  return parts
})

async function loadDetail() {
  if (!props.taskId) return
  loading.value = true
  try {
    const res = await tasksApi.get(props.taskId)
    const t = res.data
    task.value = t
    title.value = t.title
    description.value = t.description || ''
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
        parentCandidates.value = (bt.data || []).filter((x) => x.id !== t.id)
        // color/position feed the status chip and the "shift right" button; the
        // recurrence selects only ever needed id/name.
        columns.value = (cols.data || []).map((c) => ({
          id: c.id,
          name: c.name,
          color: c.color,
          position: c.position,
        }))
        doneColumnId.value = b.data.done_column_id || null
      } catch {
        boardInfo.value = null
        parentCandidates.value = []
        columns.value = []
        doneColumnId.value = null
      }
    }
    loadSiblings(t)
    loadExtras()
    if (!t.gitlab) loadGlTemplates()
  } catch (e) {
    message.error(e.message)
  } finally {
    loading.value = false
  }
}

// The parent's subtask list (ordered) is only needed when this task is itself a
// subtask and only to pin its position on a move — best-effort, one extra GET.
async function loadSiblings(t) {
  siblings.value = []
  if (!t?.parent_id) return
  try {
    const res = await tasksApi.get(t.parent_id)
    siblings.value = res.data?.subtasks || []
  } catch {
    siblings.value = []
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
  try {
    const a = await tasksApi.attachments(props.taskId)
    attachments.value = a.data || []
    emit('changed')
  } catch {
    /* the file is uploaded either way; the list refreshes on reopen */
  }
}
async function loadExtras() {
  const id = props.taskId
  // Populate the thread without the enter fade (only later, user-posted comments fade).
  commentsHydrated.value = false
  const [c, r, a, e] = await Promise.allSettled([
    tasksApi.comments(id),
    tasksApi.relations(id),
    tasksApi.attachments(id),
    tasksApi.events(id),
  ])
  comments.value = c.status === 'fulfilled' ? c.value.data || [] : []
  relations.value = r.status === 'fulfilled' ? r.value.data || [] : []
  attachments.value = a.status === 'fulfilled' ? a.value.data || [] : []
  events.value = e.status === 'fulfilled' ? e.value.data || [] : []
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
    }
  },
)

function close() {
  emit('update:show', false)
}
function buildPayload() {
  return {
    title: title.value,
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
  if (props.readonly) return
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
  } catch (e) {
    message.error(e.message)
  }
}
function setPriority(p) {
  priority.value = p
  applyMeta()
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

// Neighbours for PATCH move: a subtask holds its place in the parent's list, a
// top-level task appends to the end of the target column. Either way we send
// something — bare nulls mean positionBetween(nil, nil) = 65536, which drops the
// card near the top of the column and quietly reshuffles it.
function moveNeighbors(columnId) {
  if (task.value?.parent_id) return siblingNeighbors(siblings.value, task.value.id)
  return { before_id: columnTail(parentCandidates.value, columnId, task.value?.id), after_id: null }
}

async function moveToColumn(columnId) {
  if (props.readonly || !task.value || !columnId || columnId === task.value.column_id) return
  moving.value = true
  try {
    await tasksApi.move(task.value.id, {
      column_id: columnId,
      ...moveNeighbors(columnId),
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
  if (props.readonly || !task.value) return
  if (completed.value) return setCompleted(false)
  if (doneCol.value && doneCol.value.id !== task.value.column_id)
    return moveToColumn(doneCol.value.id)
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
async function loadGlTemplates() {
  glTemplates.value = []
  glTemplate.value = null
  if (!gitlabFetchTemplates.value || !wsId.value) return
  try {
    const res = await glApi.issueTemplates(wsId.value, gitlabIntegrationId.value)
    glTemplates.value = res.data || []
  } catch {
    glTemplates.value = []
  }
}
// Picking a template fills the description editor (which is the task's description,
// and becomes the issue body on create); the editor's blur persists it.
function applyGlTemplate(name) {
  const tpl = glTemplates.value.find((t) => t.name === name)
  if (tpl) description.value = tpl.content
}
async function createGlIssue() {
  glCreating.value = true
  try {
    // Persist the current title/description/properties first so the issue mirrors
    // the on-screen state, then create (the backend reads the saved task).
    await applyMeta()
    await glApi.createIssue(props.taskId, {})
    // Re-fetch so the task picks up its GitLab provenance and renders as synced.
    const res = await tasksApi.get(props.taskId)
    task.value = res.data
    message.success(`Создан issue !${res.data.gitlab?.iid} в GitLab`)
    emit('changed')
  } catch (e) {
    message.error(e?.response?.data?.error || e.message)
  } finally {
    glCreating.value = false
  }
}
async function save() {
  if (props.readonly) return
  try {
    await tasksApi.update(props.taskId, buildPayload())
    emit('changed')
    close()
  } catch (e) {
    message.error(e.message)
  }
}
// Archive (soft delete). Subtasks go to the archive with the parent unless the
// user chooses to detach them (keep them on the board).
async function archiveTask(detachChildren) {
  try {
    await tasksApi.archive(props.taskId, detachChildren ? { subtasks: 'detach' } : undefined)
    emit('changed')
    close()
  } catch (e) {
    message.error(e.message)
  }
}
const archiveHasSubs = computed(() => (task.value?.subtasks?.length || 0) > 0)
async function handleArchiveNegative() {
  if (archiveHasSubs.value) await archiveTask(true)
}
async function doDelete() {
  try {
    await tasksApi.remove(props.taskId)
    emit('changed')
    close()
  } catch (e) {
    message.error(e.message)
  }
}

// ── transfer to another board (via the breadcrumb) ──
const moveBoards = ref({}) // projectId -> board[]
const expandedProj = ref(new Set())
async function toggleProj(pid) {
  const s = new Set(expandedProj.value)
  if (s.has(pid)) s.delete(pid)
  else {
    s.add(pid)
    if (!moveBoards.value[pid]) {
      const r = await projApi.boards(pid)
      moveBoards.value = { ...moveBoards.value, [pid]: r.data || [] }
    }
  }
  expandedProj.value = s
}
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
async function createTag() {
  const n = newTagName.value.trim()
  if (!n) return
  const palette = ['#7c5cff', '#2f80ed', '#0eb0a9', '#18a058', '#f0a020', '#e0533d', '#eb2f96']
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
  await applyMeta()
}

function openRelated(rel) {
  if (rel.related_board_id) {
    close()
    router.push(`/board/${rel.related_board_id}?task=${rel.related_task_id}`)
  }
}

// Adding, completing or moving a subtask changes the parent task itself (its
// subtask list, its rollups), so the tab reports the mutation and the reload
// stays here — it is the modal that owns the task detail.
async function onSubtaskChanged() {
  await loadDetail()
  emit('changed')
}
</script>

<template>
  <n-modal :show="show" @update:show="emit('update:show', $event)">
    <n-card class="tm-card" role="dialog" data-testid="task-modal" :bordered="false">
      <n-spin :show="loading" :rotate="false">
        <template #icon><TesseraSpinner /></template>
        <div ref="formEl" class="form" :style="{ '--tm-cols': splitCols }">
          <div class="modal-head">
            <!-- Render the breadcrumb popover only when there's a breadcrumb: an
                 n-popover #trigger slot must resolve to exactly one child, and an
                 empty breadcrumb (before boardInfo loads, or if it fails) left the
                 slot empty and threw during render, white-screening the modal. -->
            <n-popover v-if="breadcrumb.length" trigger="click" placement="bottom-start">
              <template #trigger>
                <div class="crumbs" title="Перенести в другую доску">
                  <template v-for="(c, i) in breadcrumb" :key="i">
                    <span class="crumb">{{ c }}</span>
                    <span v-if="i < breadcrumb.length - 1" class="sep">/</span>
                  </template>
                </div>
              </template>
              <div class="menu move-menu">
                <div class="move-hint">Перенести в доску:</div>
                <div v-for="p in store.projects" :key="p.id" class="move-proj">
                  <div class="menu-item" @click="toggleProj(p.id)">
                    <span class="grow">{{ p.name }}</span>
                    <span class="chev">{{ expandedProj.has(p.id) ? '▾' : '▸' }}</span>
                  </div>
                  <div v-if="expandedProj.has(p.id)" class="move-boards">
                    <div
                      v-for="bd in moveBoards[p.id] || []"
                      :key="bd.id"
                      class="menu-item board"
                      @click="transferTo(bd.id)"
                    >
                      {{ bd.name }}
                    </div>
                    <span v-if="!(moveBoards[p.id] || []).length" class="move-hint">нет досок</span>
                  </div>
                </div>
              </div>
            </n-popover>
            <span class="head-right">
              <span v-if="task?.number" class="tnum">#{{ task.number }}</span>
              <!-- Share: copy a link to this task; the icon morphs to a checkmark. -->
              <button
                v-if="shareUrl"
                class="head-btn"
                :class="{ done: shared }"
                :title="shared ? 'Ссылка скопирована' : 'Скопировать ссылку на задачу'"
                @click="shareTask"
              >
                <Transition name="tm-share" mode="out-in">
                  <n-icon
                    v-if="shared"
                    key="ok"
                    :component="CheckmarkOutline"
                    :size="15"
                    class="grad-icon"
                  />
                  <n-icon v-else key="share" :component="ShareSocialOutline" :size="15" />
                </Transition>
              </button>
              <!-- Wide layout only: hide/show the right column (tabs). -->
              <button
                v-if="wide"
                class="head-btn"
                :title="rightHidden ? 'Показать панель' : 'Скрыть панель'"
                @click="toggleRightPane"
              >
                <n-icon
                  :component="rightHidden ? ChevronBackOutline : ChevronForwardOutline"
                  :size="15"
                />
              </button>
              <!-- GitLab provenance moved next to the Tessera number to free up
                   vertical space (icon + issue iid in parens, links to GitLab). -->
              <a
                v-if="task?.gitlab"
                class="gl-num"
                :href="task.gitlab.web_url"
                target="_blank"
                rel="noopener noreferrer"
                :title="`Открыть issue !${task.gitlab.iid} в GitLab`"
              >
                (<n-icon :component="LogoGitlab" :size="12" /> !{{ task.gitlab.iid }})
              </a>
            </span>
          </div>
          <n-input
            v-model:value="title"
            placeholder="Название задачи"
            class="title-input plain"
            :readonly="readonly"
          />

          <div class="tm-col-left">
            <div class="props" :class="{ 'tm-ro': readonly }">
              <!-- priority -->
              <div class="prow">
                <span class="plabel"><n-icon :component="FlagOutline" :size="15" /> Приоритет</span>
                <n-popover trigger="click" placement="bottom-start">
                  <template #trigger>
                    <button class="val">
                      <span
                        class="dot"
                        :style="{ background: hueGrad(PRIORITY_COLORS[priority]) }"
                      />
                      {{ PRIORITY_LABELS[priority] }}
                    </button>
                  </template>
                  <div class="menu">
                    <div
                      v-for="o in priorityOptions"
                      :key="o.value"
                      class="menu-item"
                      @click="setPriority(o.value)"
                    >
                      <span
                        class="dot"
                        :style="{ background: hueGrad(PRIORITY_COLORS[o.value]) }"
                      />
                      {{ o.label }}
                    </div>
                  </div>
                </n-popover>
              </div>

              <!-- due -->
              <div class="prow">
                <span class="plabel"
                  ><n-icon :component="CalendarClearOutline" :size="15" /> Срок</span
                >
                <n-popover trigger="click" placement="bottom-start">
                  <template #trigger>
                    <button class="val">
                      <span>{{ dueLabel || 'Не задан' }}</span>
                      <n-icon
                        v-if="recurrence"
                        :component="RepeatOutline"
                        :size="14"
                        class="recur-mark"
                        title="Повторяемая задача"
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
              </div>

              <!-- estimate -->
              <div class="prow">
                <span class="plabel"><n-icon :component="TimerOutline" :size="15" /> Оценка</span>
                <n-popover trigger="click" placement="bottom-start" @update:show="onEstShow">
                  <template #trigger>
                    <button class="val">
                      <span :class="{ muted: !estFull }">{{ estFull || 'Не задана' }}</span>
                      <span v-if="estRange" class="est-range">· {{ estRange }}</span>
                      <span
                        v-if="subtaskEstimateLabel"
                        class="est-rollup"
                        title="Сумма оценок подзадач"
                        >Σ {{ subtaskEstimateLabel }}</span
                      >
                    </button>
                  </template>
                  <div class="est-pop">
                    <div v-if="estOptions.length" class="menu est-menu">
                      <div class="menu-item" @click="clearEstimate">
                        <span class="grow muted">Не задана</span>
                        <n-icon v-if="estimate == null" :component="CheckmarkOutline" class="chk" />
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
                    </div>
                    <div v-else class="est-edit">
                      <n-input
                        v-model:value="estInput"
                        size="small"
                        :placeholder="estPlaceholder"
                        @keydown.enter.prevent="applyEstInput"
                      />
                      <div class="est-actions">
                        <n-button size="tiny" tertiary @click="clearEstimate">Очистить</n-button>
                        <n-button size="tiny" type="primary" @click="applyEstInput">ОК</n-button>
                      </div>
                    </div>
                  </div>
                </n-popover>
              </div>

              <!-- milestone («Этап») -->
              <div class="prow">
                <span class="plabel"><n-icon :component="RibbonOutline" :size="15" /> Этап</span>
                <n-popover trigger="click" placement="bottom-start">
                  <template #trigger>
                    <button class="val">
                      <span :class="{ muted: !taskMilestone }">
                        {{ taskMilestone ? taskMilestone.title : 'Не задан' }}
                      </span>
                      <span v-if="taskMilestone && milestoneRange(taskMilestone)" class="est-range">
                        · {{ milestoneRange(taskMilestone) }}
                      </span>
                      <span v-if="taskMilestone?.state === 'closed'" class="est-range"
                        >· закрыт</span
                      >
                    </button>
                  </template>
                  <div class="ms-pop">
                    <div class="menu ms-menu">
                      <div class="menu-item" @click="setMilestone(null)">
                        <span class="grow muted">Не задан</span>
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
                        <span class="grow ms-opt" :class="{ 'ms-closed': m.state === 'closed' }">
                          <span class="ms-opt-title">{{ m.title }}</span>
                          <span v-if="milestoneRange(m)" class="ms-opt-range">{{
                            milestoneRange(m)
                          }}</span>
                        </span>
                        <n-icon
                          v-if="task?.milestone_id === m.id"
                          :component="CheckmarkOutline"
                          class="chk"
                        />
                      </div>
                    </div>
                    <div class="ms-new">
                      <n-input
                        v-model:value="newMilestoneTitle"
                        size="small"
                        placeholder="Новый этап…"
                        @keydown.enter.prevent="createMilestone"
                      />
                      <n-button
                        size="small"
                        type="primary"
                        :disabled="!newMilestoneTitle.trim()"
                        @click="createMilestone"
                      >
                        Создать
                      </n-button>
                    </div>
                  </div>
                </n-popover>
              </div>

              <!-- author (read-only) -->
              <div v-if="author" class="prow">
                <span class="plabel"><n-icon :component="PersonOutline" :size="15" /> Автор</span>
                <div
                  class="val static"
                  :title="author.gl ? `@${author.login} · GitLab` : author.name"
                >
                  <UserAvatar
                    class="avatar"
                    :user-id="author.id"
                    :src="author.avatar"
                    :name="author.name"
                  />
                  <span class="author-name">{{ author.name }}</span>
                  <span v-if="author.gl" class="author-gl">@{{ author.login }} · GitLab</span>
                </div>
              </div>

              <!-- assignees -->
              <div class="prow">
                <span class="plabel"
                  ><n-icon :component="PeopleOutline" :size="15" /> Исполнители</span
                >
                <n-popover trigger="click" placement="bottom-start">
                  <template #trigger>
                    <button class="val">
                      <UserAvatar
                        v-for="u in assigneeObjs"
                        :key="u.user_id"
                        class="avatar"
                        :user-id="u.user_id"
                        :name="u.name"
                        :title="u.name"
                      />
                      <UserAvatar
                        v-for="(g, i) in glAssignees"
                        :key="`g${i}`"
                        class="avatar ext-ava"
                        :src="g.gl_avatar_url"
                        :name="g.gl_name || g.gl_username"
                        :title="`${g.gl_name || g.gl_username} (GitLab)`"
                      />
                      <span v-if="!assigneeObjs.length && !glAssignees.length" class="muted"
                        >Никто</span
                      >
                    </button>
                  </template>
                  <div class="menu">
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
                    <template v-if="gitlabMembers.length">
                      <div class="menu-sep">GitLab</div>
                      <div
                        v-for="m in gitlabMembers"
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
                  </div>
                </n-popover>
              </div>

              <!-- tags -->
              <div class="prow">
                <span class="plabel"><n-icon :component="PricetagOutline" :size="15" /> Теги</span>
                <n-popover trigger="click" placement="bottom-start">
                  <template #trigger>
                    <button ref="tagsValEl" class="val tags-val">
                      <template v-if="tagObjs.length">
                        <TagPill
                          v-for="t in tagObjs.slice(0, visibleTagCount)"
                          :key="t.id"
                          class="chip"
                          :tag="t"
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
                        <!-- invisible measurement row: natural chip widths, never sliced.
                             Same component + props as above, else the scope segment
                             wouldn't be measured and the fit calculation would lie. -->
                        <span ref="tagsMeasureEl" class="tags-measure" aria-hidden="true">
                          <TagPill
                            v-for="t in tagObjs"
                            :key="`m${t.id}`"
                            class="chip"
                            :tag="t"
                            :prefix-names="tagPrefixNames"
                            variant="outline"
                          />
                        </span>
                      </template>
                      <span v-else class="muted">Нет</span>
                    </button>
                  </template>
                  <div class="menu">
                    <div class="chip-groups">
                      <div v-for="g in tagPickerGroups" :key="g.key" class="chip-group">
                        <div v-if="tagPickerHeaders" class="chip-grp-head">{{ g.label }}</div>
                        <div class="chip-grid">
                          <button
                            v-for="t in g.tags"
                            :key="t.id"
                            class="tagchip"
                            :class="{ on: selectedTags.includes(t.id) }"
                            :style="
                              selectedTags.includes(t.id)
                                ? {
                                    background: hueGrad(t.color),
                                    color: onColor(t.color),
                                    borderColor: 'transparent',
                                  }
                                : {
                                    background: softFill(t.color),
                                    color: tagText(t.color),
                                    borderColor: (t.color || '#888') + '66',
                                  }
                            "
                            @click="toggleTag(t.id)"
                          >
                            <TagPill
                              :tag="t"
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
                      placeholder="Новый тег, Enter"
                      @keyup.enter="createTag"
                    />
                  </div>
                </n-popover>
              </div>

              <!-- status: current column · shift right · close.
                   Works the same for a task and for a subtask — one modal. -->
              <div class="prow">
                <span class="plabel"
                  ><n-icon :component="CheckmarkDoneOutline" :size="15" /> Статус</span
                >
                <div class="status-row">
                  <n-popover
                    v-if="!readonly && sortedCols.length"
                    trigger="click"
                    placement="bottom-start"
                  >
                    <template #trigger>
                      <button class="val col-chip" :disabled="moving">
                        <span class="col-dot" :style="{ background: currentColumn?.color }" />
                        <span>{{ currentColumn?.name || 'Без колонки' }}</span>
                      </button>
                    </template>
                    <div class="menu pmenu">
                      <div
                        v-for="c in sortedCols"
                        :key="c.id"
                        class="menu-item col-item"
                        :class="{ cur: c.id === task?.column_id }"
                        @click="moveToColumn(c.id)"
                      >
                        <span class="col-dot" :style="{ background: c.color }" />
                        <span>{{ c.name }}</span>
                      </div>
                    </div>
                  </n-popover>
                  <span v-else class="val col-chip static">
                    <span class="col-dot" :style="{ background: currentColumn?.color }" />
                    <span>{{ currentColumn?.name || '—' }}</span>
                  </span>
                  <template v-if="!readonly">
                    <button
                      class="st-btn"
                      :disabled="!nextCol || moving"
                      :title="nextCol ? `Сдвинуть → «${nextCol.name}»` : 'Это последняя колонка'"
                      @click="moveToColumn(nextCol?.id)"
                    >
                      <n-icon :component="PlayForwardOutline" :size="15" />
                    </button>
                    <button
                      class="st-btn"
                      :class="{ on: completed }"
                      :disabled="moving"
                      :title="completed ? 'Вернуть в работу' : 'Выполнено'"
                      @click="closeTask"
                    >
                      <n-icon
                        :component="completed ? CheckmarkCircle : EllipseOutline"
                        :size="15"
                      />
                    </button>
                  </template>
                </div>
              </div>

              <!-- parent -->
              <div class="prow">
                <span class="plabel"
                  ><n-icon :component="GitMergeOutline" :size="15" /> Родитель</span
                >
                <button v-if="task?.parent_id" class="val" @click="detachFromParent">
                  Открепить
                </button>
                <n-popover v-else trigger="click" placement="bottom-start">
                  <template #trigger>
                    <button class="val"><span class="muted">Сделать подзадачей…</span></button>
                  </template>
                  <div class="menu pmenu">
                    <div
                      v-for="cand in parentCandidates"
                      :key="cand.id"
                      class="menu-item"
                      @click="attachTo(cand.id)"
                    >
                      {{ cand.title }}
                    </div>
                    <span v-if="!parentCandidates.length" class="muted small"
                      >Нет других задач</span
                    >
                  </div>
                </n-popover>
              </div>

              <!-- create GitLab issue from this task -->
              <div v-if="task && !task.gitlab && gitlabCanCreate" class="prow">
                <span class="plabel"><n-icon :component="LogoGitlab" :size="15" /> GitLab</span>
                <button class="val" :disabled="glCreating" @click="createGlIssue">
                  <span class="muted">{{ glCreating ? 'Создание…' : 'Создать issue' }}</span>
                </button>
              </div>
            </div>

            <div class="section">
              <div class="desc-head">
                <span class="slabel">Описание</span>
                <div class="desc-head-r">
                  <n-select
                    v-if="task && !task.gitlab && gitlabFetchTemplates && glTemplates.length"
                    v-model:value="glTemplate"
                    :options="glTemplateOptions"
                    size="small"
                    clearable
                    placeholder="Шаблон issue…"
                    class="tpl-select"
                    @update:value="applyGlTemplate"
                  />
                  <div v-if="!readonly" class="desc-acts">
                    <template v-if="descMode === 'write'">
                      <button
                        class="desc-act"
                        title="Вставить изображение"
                        @click="descEditor?.pickImage()"
                      >
                        <n-icon :component="ImageOutline" :size="16" />
                      </button>
                      <button
                        class="desc-act"
                        title="Вставить Mermaid-диаграмму"
                        @click="descEditor?.insertMermaid()"
                      >
                        <n-icon :component="GitNetworkOutline" :size="16" />
                      </button>
                    </template>
                    <button
                      class="desc-act"
                      title="Открыть на весь экран"
                      @click="descEditor?.openFullscreen()"
                    >
                      <n-icon :component="ExpandOutline" :size="16" />
                    </button>
                    <button
                      class="desc-act"
                      :title="descMode === 'write' ? 'Предпросмотр' : 'Редактировать'"
                      @click="descEditor?.toggleMode()"
                    >
                      <n-icon
                        :component="descMode === 'write' ? EyeOutline : CreateOutline"
                        :size="16"
                      />
                    </button>
                  </div>
                </div>
              </div>
              <RichContent
                v-if="readonly"
                :source="description || '_Нет описания_'"
                :members="members"
                task-refs
              />
              <MarkdownEditor
                v-else
                ref="descEditor"
                :key="taskId"
                v-model="description"
                :toolbar="false"
                placeholder="Добавьте описание…"
                :min-rows="3"
                :initial-mode="descInitialMode"
                :attach-task-id="taskId"
                @update:mode="descMode = $event"
                @attachments-changed="reloadAttachments"
                @blur="saveDesc"
                @persist="saveDesc"
              />
            </div>
          </div>

          <!-- Draggable divider (wide layout only); drag resizes the two columns,
               a double-click hides/shows the right column. -->
          <div
            class="tm-divider"
            :title="rightHidden ? 'Двойной клик — показать панель' : 'Потяните / двойной клик'"
            @pointerdown="startSplitDrag"
            @dblclick="toggleRightPane"
          >
            <span class="tm-divider-grip"></span>
          </div>

          <div class="tm-col-right" :class="{ 'tm-col-hidden': rightHidden }">
            <!-- Subtasks / comments / relations / files / history (#8) -->
            <!-- Keyed by task so the line indicator doesn't slide when switching
               between a task and its subtask / related task. -->
            <n-tabs ref="detailTabs" :key="taskId" type="line" size="small" class="detail-tabs">
              <n-tab-pane name="comments">
                <template #tab>
                  <span class="tab-lbl">
                    <n-icon
                      :component="ChatbubbleEllipsesOutline"
                      :size="15"
                      class="tab-ico tab-ico--out"
                    />
                    <n-icon
                      :component="ChatbubbleEllipses"
                      :size="15"
                      class="tab-ico tab-ico--fill"
                    />
                    Комментарии
                    <n-badge
                      v-if="comments.length"
                      :value="comments.length"
                      :max="99"
                      class="tab-badge"
                    />
                  </span>
                </template>
                <TaskCommentsTab
                  ref="commentsTab"
                  v-model:comments="comments"
                  :task-id="taskId"
                  :readonly="readonly"
                  :hydrated="commentsHydrated"
                  @changed="emit('changed')"
                  @reload-detail="loadDetail"
                />
              </n-tab-pane>

              <n-tab-pane name="subtasks">
                <template #tab>
                  <span class="tab-lbl">
                    <n-icon :component="GitBranchOutline" :size="15" class="tab-ico tab-ico--out" />
                    <n-icon :component="GitBranch" :size="15" class="tab-ico tab-ico--fill" />
                    Подзадачи
                    <n-badge
                      v-if="task?.subtasks?.length"
                      :value="task.subtasks.length"
                      :max="99"
                      class="tab-badge"
                    />
                  </span>
                </template>
                <TaskSubtasksTab
                  :task="task"
                  :columns="columns"
                  :readonly="readonly"
                  @open="emit('open', $event)"
                  @changed="onSubtaskChanged"
                />
              </n-tab-pane>

              <n-tab-pane name="relations">
                <template #tab>
                  <span class="tab-lbl">
                    <n-icon :component="GitMergeOutline" :size="15" class="tab-ico tab-ico--out" />
                    <n-icon :component="GitMerge" :size="15" class="tab-ico tab-ico--fill" />
                    Связи
                    <n-badge
                      v-if="relations.length"
                      :value="relations.length"
                      :max="99"
                      class="tab-badge"
                    />
                  </span>
                </template>
                <TaskRelationsTab
                  v-model:relations="relations"
                  :task-id="taskId"
                  :ws-id="wsId"
                  @changed="emit('changed')"
                  @open-related="openRelated"
                />
              </n-tab-pane>

              <n-tab-pane name="files">
                <template #tab>
                  <span class="tab-lbl">
                    <n-icon :component="AttachOutline" :size="15" class="tab-ico tab-ico--out" />
                    <n-icon :component="Attach" :size="15" class="tab-ico tab-ico--fill" />
                    Файлы
                    <n-badge
                      v-if="attachments.length"
                      :value="attachments.length"
                      :max="99"
                      class="tab-badge"
                    />
                  </span>
                </template>
                <TaskFilesTab
                  v-model:attachments="attachments"
                  :task-id="taskId"
                  @changed="emit('changed')"
                />
              </n-tab-pane>

              <n-tab-pane name="history">
                <template #tab>
                  <span class="tab-lbl">
                    <n-icon :component="TimeOutline" :size="15" class="tab-ico tab-ico--out" />
                    <n-icon :component="Time" :size="15" class="tab-ico tab-ico--fill" />
                    История
                  </span>
                </template>
                <TaskHistoryTab :events="events" />
              </n-tab-pane>
            </n-tabs>
          </div>
        </div>
      </n-spin>

      <template #footer>
        <div v-if="readonly" class="footer">
          <span class="ro-note">Архивная задача — только просмотр</span>
          <n-space :wrap="false" :size="8">
            <n-button @click="close">Закрыть</n-button>
            <n-button type="primary" @click="emit('restore', taskId)">
              <template #icon><n-icon :component="ArrowUndoOutline" /></template>
              Вернуть из архива
            </n-button>
          </n-space>
        </div>
        <div v-else class="footer">
          <n-space :wrap="false" :size="8">
            <n-popconfirm
              :positive-text="archiveHasSubs ? 'В архив вместе' : 'В архив'"
              :negative-text="archiveHasSubs ? 'Открепить подзадачи' : 'Отмена'"
              @positive-click="() => archiveTask(false)"
              @negative-click="handleArchiveNegative"
            >
              <template #trigger>
                <n-button type="primary" ghost>
                  <template #icon><n-icon :component="ArchiveOutline" /></template>
                  <span class="fbtn-label">В архив</span>
                </n-button>
              </template>
              {{
                archiveHasSubs
                  ? 'У задачи есть подзадачи — что с ними сделать?'
                  : 'Перенести задачу в архив?'
              }}
            </n-popconfirm>
            <n-popconfirm
              :positive-button-props="{ type: 'error' }"
              positive-text="Удалить"
              @positive-click="doDelete"
            >
              <template #trigger>
                <n-button type="error" ghost>
                  <template #icon><n-icon :component="TrashOutline" /></template>
                  <span class="fbtn-label">Удалить</span>
                </n-button>
              </template>
              Удалить безвозвратно? Это действие необратимо.
            </n-popconfirm>
          </n-space>
          <n-space :wrap="false" :size="8">
            <n-button @click="close">Отмена</n-button>
            <n-button type="primary" @click="save">Сохранить</n-button>
          </n-space>
        </div>
      </template>
    </n-card>
  </n-modal>
</template>

<style scoped>
@import './task/tab-shared.css';

.tm-card {
  width: 640px;
  max-width: 94vw;
}
.form {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
/* Narrow (default): the column wrappers are transparent, so title / props /
   description / tabs stack exactly as before. Two-column layout kicks in only
   on wide screens below. The tabs node is never re-mounted (CSS-only reflow),
   so tab + editor state survive resizing across the breakpoint. */
.tm-col-left,
.tm-col-right {
  display: contents;
}
/* The resize divider only exists in the two-column layout. */
.tm-divider {
  display: none;
}
.tm-divider-grip {
  width: 4px;
  height: 48px;
  border-radius: 3px;
  background: var(--t-border);
  transition: background 0.15s ease;
}
.tm-divider:hover .tm-divider-grip {
  background: var(--t-primary);
}
@media (min-width: 1100px) {
  .tm-card {
    width: min(1240px, 96vw);
  }
  .form {
    display: grid;
    /* left | divider track | right — overridable via the --tm-cols var that the
       draggable divider writes (persisted in localStorage). The transition
       animates the collapse/expand when the right column is hidden/shown. */
    grid-template-columns: var(--tm-cols, minmax(0, 1fr) 14px minmax(0, 1.05fr));
    align-items: start;
    transition: grid-template-columns 0.18s ease;
  }
  /* Right column collapsed to a 0fr track: fade it out and stop it catching
     clicks while it shrinks. */
  .tm-col-right {
    transition: opacity 0.12s ease;
  }
  .tm-col-right.tm-col-hidden {
    opacity: 0;
    overflow: hidden;
    pointer-events: none;
  }
  /* Head + title span all three tracks. */
  .modal-head,
  .title-input {
    grid-column: 1 / -1;
  }
  .tm-col-left {
    grid-column: 1;
    display: flex;
    flex-direction: column;
    gap: 16px;
    min-width: 0;
  }
  .tm-divider {
    grid-column: 2;
    display: flex;
    align-items: center;
    justify-content: center;
    align-self: stretch;
    cursor: col-resize;
    touch-action: none;
  }
  /* The right column is a fixed-height flex box that does NOT scroll as a whole:
     the tab strip stays pinned while the active pane scrolls inside it (so
     scrolling comments never carry the tabs off-screen). The footer lives in the
     card's #footer slot so it stays pinned full-width. */
  .tm-col-right {
    grid-column: 3;
    min-width: 0;
    max-height: calc(90vh - 210px);
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }
  .tm-col-left {
    max-height: calc(90vh - 210px);
    overflow-y: auto;
  }
  /* The tabs fill the right column; the nav is pinned (flex:none), the active pane
     takes the rest and scrolls. */
  .tm-col-right .detail-tabs {
    margin: 0;
    flex: 1;
    min-height: 0;
    display: flex;
    flex-direction: column;
  }
  .detail-tabs :deep(.n-tabs-nav) {
    flex: none;
  }
  .detail-tabs :deep(.n-tab-pane) {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
  }
  /* Comments tab: the list scrolls internally, the composer is a pinned footer that
     sits OUTSIDE the scroll area — so scrolling content can't bleed under its top
     border (which the previous position:sticky composer did). */
  .tm-col-right .comments {
    flex: 1;
    min-height: 0;
  }
  .tm-col-right .c-list {
    overflow-y: auto;
  }
  .tm-col-right .comment-add {
    position: static;
  }
  /* Idle: reserved gutter, transparent thumb; reveal on hover/focus — one skin for
     all three scrollers (left column, the active tab pane, the comment list). */
  .tm-col-left,
  .detail-tabs :deep(.n-tab-pane),
  .tm-col-right .c-list {
    scrollbar-gutter: stable;
    scrollbar-width: thin;
    scrollbar-color: transparent transparent;
    transition: scrollbar-color 0.15s ease;
  }
  .tm-col-left:hover,
  .tm-col-left:focus-within,
  .detail-tabs :deep(.n-tab-pane):hover,
  .detail-tabs :deep(.n-tab-pane):focus-within,
  .tm-col-right .c-list:hover,
  .tm-col-right .c-list:focus-within {
    scrollbar-color: var(--t-border) transparent;
  }
  .tm-col-left::-webkit-scrollbar,
  .detail-tabs :deep(.n-tab-pane)::-webkit-scrollbar,
  .tm-col-right .c-list::-webkit-scrollbar {
    width: 10px;
  }
  .tm-col-left::-webkit-scrollbar-thumb,
  .detail-tabs :deep(.n-tab-pane)::-webkit-scrollbar-thumb,
  .tm-col-right .c-list::-webkit-scrollbar-thumb {
    border-radius: 5px;
    border: 3px solid transparent;
    background: transparent;
    background-clip: padding-box;
  }
  .tm-col-left:hover::-webkit-scrollbar-thumb,
  .tm-col-left:focus-within::-webkit-scrollbar-thumb,
  .detail-tabs :deep(.n-tab-pane):hover::-webkit-scrollbar-thumb,
  .detail-tabs :deep(.n-tab-pane):focus-within::-webkit-scrollbar-thumb,
  .tm-col-right .c-list:hover::-webkit-scrollbar-thumb,
  .tm-col-right .c-list:focus-within::-webkit-scrollbar-thumb {
    background: var(--t-border);
    background-clip: padding-box;
  }
}
.title-input :deep(input) {
  font-size: 18px;
  font-weight: 600;
}
.modal-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}
.head-right {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  flex: none;
  /* Stay pinned right even when the breadcrumb (the other flex child) is absent. */
  margin-left: auto;
}
.tnum {
  font-size: 12px;
  color: var(--t-text3);
  flex: none;
}
/* GitLab provenance, compact, next to the Tessera number. */
.gl-num {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  font-size: 12px;
  color: var(--t-text3);
  text-decoration: none;
}
.gl-num:hover {
  color: var(--t-primary);
}
/* Small neutral icon buttons in the head (share, hide/show panel). */
.head-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border: none;
  background: transparent;
  color: var(--t-text3);
  border-radius: 6px;
  cursor: pointer;
  transition:
    background 0.12s ease,
    color 0.12s ease;
}
.head-btn:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
/* Share icon morph (share ⇄ checkmark). */
.tm-share-enter-active,
.tm-share-leave-active {
  transition:
    opacity 0.14s ease,
    transform 0.14s ease;
}
.tm-share-enter-from,
.tm-share-leave-to {
  opacity: 0;
  transform: scale(0.7);
}
@media (prefers-reduced-motion: reduce) {
  .tm-share-enter-active,
  .tm-share-leave-active {
    transition: none;
  }
  .tm-share-enter-from,
  .tm-share-leave-to {
    transform: none;
  }
  .form,
  .tm-col-right {
    transition: none;
  }
}
.desc-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}
.desc-head-r {
  display: flex;
  align-items: center;
  gap: 4px;
}
.desc-head .tpl-select {
  width: 200px;
}
.desc-acts {
  display: flex;
  align-items: center;
  gap: 2px;
}
.desc-act {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: none;
  background: transparent;
  color: var(--t-text2);
  cursor: pointer;
  padding: 4px 6px;
  border-radius: 6px;
  transition:
    background 0.12s ease,
    color 0.12s ease;
}
.desc-act:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.gl-author {
  color: var(--t-text3);
}
.crumbs {
  display: inline-flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--t-text3);
  cursor: pointer;
  padding: 2px 4px;
  border-radius: 6px;
}
.crumbs:hover {
  background: var(--t-hover);
  color: var(--t-text2);
}
.move-menu {
  width: 220px;
  max-height: 300px;
  overflow-y: auto;
}
.move-hint {
  font-size: 12px;
  color: var(--t-text3);
  padding: 4px 6px;
}
.move-boards {
  padding-left: 12px;
}
.menu-item.board {
  color: var(--t-text2);
}
.chev {
  color: var(--t-text3);
  font-size: 11px;
}
.crumb {
  white-space: nowrap;
}
.sep {
  opacity: 0.5;
}
/* The `plain` class lands on the NInput ROOT element (attr fallthrough), which
   IS `.n-input` — so a `:deep(.n-input)` descendant selector never matched it
   (and the modal is teleported to <body>). Set Naive's --n-color vars on
   `.plain` itself (!important beats the inline ones Naive writes); they inherit
   into the inner elements, so the field keeps the modal colour even on focus. */
.plain {
  --n-color: transparent !important;
  --n-color-focus: transparent !important;
}
.plain :deep(.n-input__border),
.plain :deep(.n-input__state-border) {
  display: none !important;
}
.props {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.prow {
  display: flex;
  align-items: center;
  gap: 12px;
  min-height: 34px;
}
.plabel {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  width: 140px;
  flex: none;
  color: var(--t-text3);
  font-size: 13px;
}
.val {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  min-height: 28px;
  padding: 3px 10px;
  border-radius: 6px;
  border: 1px solid transparent;
  background: transparent;
  color: var(--t-text1);
  font-size: 13px;
  cursor: pointer;
}
.val:hover {
  background: var(--t-hover);
}
/* recurrence repeat glyph on the due value */
.recur-mark {
  color: var(--t-primary);
}
/* Read-only value (Author): no hover affordance, default cursor. */
.val.static {
  cursor: default;
}
.val.static:hover {
  background: transparent;
}
.author-name {
  color: var(--t-text1);
}
.author-gl {
  color: var(--t-text3);
  font-size: 12px;
}
/* external GitLab assignee avatar (no Tessera account) */
.ext-ava {
  background: var(--t-text3);
}
.muted {
  color: var(--t-text3);
}
.dot {
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
  background: var(--t-surface-alt);
}
/* Tags trigger: one line, never spilling past the modal. Only the chips that
   wholly fit are rendered (+ a measured "+N"); the measurement row is invisible. */
.tags-val {
  max-width: 100%;
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
.avatar {
  flex: none;
  width: 24px;
  height: 24px;
  border-radius: 50%;
  background: var(--t-accent-grad);
  color: var(--t-on-primary);
  font-size: 10px;
  font-weight: 600;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  margin-left: -9px;
  box-shadow: 0 0 0 2px var(--t-surface-alt);
}
.avatar:first-child {
  margin-left: 0;
}
.avatar.sm {
  margin-left: 0;
  box-shadow: none;
}
.section {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.slabel {
  font-size: 12px;
  color: var(--t-text3);
}
/* ── status row: [● column ▾] [▸ shift] [✓ close] ──
   The chip and its popover menu are shared with the subtasks tab — see
   task/tab-shared.css, imported at the top of this block. */
.status-row {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}
.st-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border-radius: 6px;
  border: 1px solid var(--t-border);
  background: transparent;
  color: var(--t-text2);
  cursor: pointer;
}
.st-btn:hover:not(:disabled) {
  background: var(--t-hover);
}
.st-btn:disabled {
  opacity: 0.4;
  cursor: default;
}
.st-btn.on {
  color: var(--t-primary);
}
.small {
  font-size: 12px;
  padding: 4px;
}
.menu-sep {
  font-size: 10px;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  opacity: 0.6;
  padding: 6px 6px 2px;
}
.grow {
  flex: 1;
}
.chk {
  color: var(--t-primary);
}
/* estimate rollup hint on the value + popover editor */
.est-rollup {
  color: var(--t-text3);
  font-size: 12px;
}
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
.ms-pop {
  /* Fit the widest option (title + date range on one line) up to a cap, instead
     of a fixed 240px that forced the period to wrap. */
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
/* Title + date range share one line; the title ellipsises if extreme, the range
   never wraps so the period stays intact. */
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
  padding-top: 8px;
  border-top: 1px solid var(--t-border);
}
.est-actions {
  display: flex;
  justify-content: flex-end;
  gap: 6px;
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
.footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: nowrap;
  gap: 8px;
}
/* Read-only (archive) modal: property pills become display-only so nothing edits. */
.props.tm-ro :is(.pill, button, .n-select, .n-tag, [role='button']) {
  pointer-events: none;
}
.ro-note {
  font-size: 12px;
  color: #b5792a;
}
/* Remove naive's tab-strip scroll shadow (shows in the overflow gutter on
   mobile where the tabs scroll horizontally). */
.detail-tabs :deep(.n-tabs-nav-scroll-wrapper)::before,
.detail-tabs :deep(.n-tabs-nav-scroll-wrapper)::after {
  box-shadow: none !important;
}
/* Mobile: drop the archive/delete labels (icons only) so all four footer
   buttons fit on one row. */
@media (max-width: 768px) {
  .fbtn-label {
    display: none;
  }
  /* Icon-only: drop the icon's trailing margin (reserved for the hidden label)
     so it sits centred. */
  .footer :deep(.n-button__icon) {
    margin: 0;
  }
}

/* rendered markdown (description + comments) */
.md {
  font-size: 14px;
  line-height: 1.55;
  color: var(--t-text1);
  word-break: break-word;
}
.md:empty::before {
  content: 'Добавьте описание…';
  color: var(--t-text3);
}
.section > .md {
  padding: 8px 10px;
  border-radius: 8px;
  cursor: text;
  min-height: 40px;
}
.section > .md:hover {
  background: var(--t-surface-alt);
}
.md :deep(p) {
  margin: 0 0 8px;
}
.md :deep(p:last-child) {
  margin-bottom: 0;
}
.md :deep(ul),
.md :deep(ol) {
  margin: 0 0 8px;
  padding-left: 20px;
}
.md :deep(code) {
  background: var(--t-surface-alt);
  padding: 1px 5px;
  border-radius: 4px;
  font-size: 0.9em;
}
.md :deep(pre) {
  background: var(--t-surface-alt);
  padding: 10px;
  border-radius: 8px;
  overflow-x: auto;
}
.md :deep(a) {
  color: var(--t-primary);
}
.md :deep(blockquote) {
  margin: 0 0 8px;
  padding-left: 10px;
  border-left: 3px solid var(--t-border);
  color: var(--t-text2);
}

.detail-tabs {
  margin-top: 16px;
  margin-bottom: 12px;
}
/* Breathing room between the tab strip and its content. */
.detail-tabs :deep(.n-tab-pane) {
  padding-top: 14px;
}
/* Accent-gradient underline under the active tab. */
.detail-tabs :deep(.n-tabs-bar) {
  background: var(--t-accent-grad);
}
/* Tab label = icon + text + counter on one baseline. The icon inherits the tab's
   text color (currentColor), so the active tab's icon turns accent automatically,
   matching the text and counter. */
.tab-lbl {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.tab-lbl .tab-ico {
  /* inherit currentColor (tab text) — accent when active, dim otherwise */
  opacity: 0.9;
}
/* Inactive tabs show the outline glyph; the active tab swaps to the filled one
   (accent, matching the visualisation icons). */
.tab-ico--fill {
  display: none;
}
.detail-tabs :deep(.n-tabs-tab--active .tab-ico--out) {
  display: none;
}
.detail-tabs :deep(.n-tabs-tab--active .tab-ico--fill) {
  display: inline-flex;
}
.tab-badge {
  margin-left: 0;
}
/* Accent (not naive's default red) tab counters, matching the Android client —
   a small fixed circle (not naive's oversized pill). */
.tab-badge :deep(.n-badge-sup) {
  background: var(--t-accent-grad);
  color: var(--t-on-primary);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
  min-width: 16px;
  padding: 0;
  border-radius: 50%;
  font-size: 10px;
  font-weight: 600;
}
/* Centre the animated number inside the fixed circle. */
.tab-badge :deep(.n-badge-sup .n-base-slot-machine) {
  line-height: 16px;
  height: 16px;
}
/* Dim the counter on inactive tabs (neutral, recedes). */
.detail-tabs :deep(.n-tabs-tab:not(.n-tabs-tab--active) .n-badge-sup) {
  background: var(--t-text3);
  opacity: 0.55;
}
</style>
