<script setup>
import { ref, computed, watch, nextTick, h } from 'vue'
import draggable from 'vuedraggable'
import { NIcon, NPopover, NTooltip, NInput, NDropdown, NPopconfirm } from 'naive-ui'
import {
  FlagOutline,
  CalendarClearOutline,
  PersonAddOutline,
  PricetagOutline,
  CheckmarkCircle,
  EllipseOutline,
  CheckmarkOutline,
  OpenOutline,
  CheckmarkDoneOutline,
  ArrowForwardOutline,
  GitBranchOutline,
  ArchiveOutline,
  TrashOutline,
  LogoGitlab,
  RepeatOutline,
  TimerOutline,
  RibbonOutline,
  WarningOutline,
  EllipsisHorizontal,
  ReorderThreeOutline,
  ArrowUndoOutline,
} from '@vicons/ionicons5'

// Render a dropdown-option icon (naive's `icon` option field wants a render fn).
const menuIcon = (icon) => () => h(NIcon, null, { default: () => h(icon) })
const dangerIcon = (icon) => () => h(NIcon, { color: '#e0533d' }, { default: () => h(icon) })
import { tasks as tasksApi, projects as projectsApi, boards as boardsApi } from '@/api'
import { PRIORITY_COLORS, PRIORITY_LABELS } from '@/styles/tokens'
import { hueGrad, hueGradVert, tagPillBg, softFill, readableHue, onColor } from '@/utils/gradient'
import { buildTagGroups, tagParts } from '@/utils/tagGroups'
import { milestoneRange } from '@/utils/milestones'
import {
  formatEstimate,
  formatEstimateFull,
  estimateTooltip,
  sumEstimates,
} from '@/utils/estimation'
import { pressMoved } from '@/utils/dnd'
import UserAvatar from './UserAvatar.vue'
import DueEditor from './DueEditor.vue'
import RichContent from './RichContent.vue'
import TaskMiniCard from './TaskMiniCard.vue'
import TagPill from './TagPill.vue'
import { useThemeStore } from '@/stores/theme'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useConflictsStore } from '@/stores/conflicts'
import { useDateLocale } from '@/composables/useDateLocale'
import { useTagFit } from '@/composables/useTagFit'

const theme = useThemeStore()
const wsStore = useWorkspacesStore()
const conflictsStore = useConflictsStore()
const hasConflict = computed(() => conflictsStore.has(props.task.id))
const { formatDue } = useDateLocale()
// Tag/label colour clamped for legibility on the active theme (used for text).
const tagText = (c) => readableHue(c, theme.isDark)

const props = defineProps({
  task: { type: Object, required: true },
  subtasks: { type: Array, default: () => [] },
  // How many subtasks the task really has. When the composer filter narrowed the
  // list (`subtasks` is shorter), the card says "N из M" and locks child DnD —
  // reordering a partial list would write meaningless float8 positions.
  subtasksTotal: { type: Number, default: 0 },
  // Render subtasks as full property cards (vs compact name-only rows).
  subtasksExpanded: { type: Boolean, default: false },
  // This card is itself a first-level subtask shown below its parent: darker
  // shade, no nested-subtask cascade, no "create subtask" button.
  nested: { type: Boolean, default: false },
  // A board drag is in progress → reveal the "drop to nest" zone on childless cards.
  dragging: { type: Boolean, default: false },
  // Board status columns [{ id, name }] for the context-menu "move to column".
  columns: { type: Array, default: () => [] },
  tagsMap: { type: Object, default: () => ({}) },
  membersMap: { type: Object, default: () => ({}) },
  tags: { type: Array, default: () => [] },
  tagPrefixNames: { type: Object, default: () => ({}) },
  // Canonical tag prefixes governed by status/priority/meta GitLab rules — hidden
  // from the tag picker so they can't be toggled out of sync with the mapped field.
  metaTagPrefixes: { type: Set, default: () => new Set() },
  members: { type: Array, default: () => [] },
  gitlabMembers: { type: Array, default: () => [] },
  milestonesMap: { type: Object, default: () => ({}) },
  wsId: { type: String, default: null },
  projectId: { type: String, default: null },
  // Customize-view: per-field pill visibility (key → false hides), whether empty
  // (unset) pills render, whether pills stack vertically, and the card size preset
  // which controls the card's composition (compact = title only, medium = key
  // fields, large = everything).
  fieldVis: { type: Object, default: () => ({}) },
  showEmpty: { type: Boolean, default: true },
  stackFields: { type: Boolean, default: false },
  cardSize: { type: String, default: 'medium' },
  // Archive view: card is display-only (no inline edits/DnD/menu); shows a Restore
  // affordance instead. Clicking the card still opens the read-only task modal.
  readonly: { type: Boolean, default: false },
})
const emit = defineEmits(['open', 'changed', 'restore'])

// A field is visible unless its customize toggle is explicitly false (missing key
// defaults to shown → back-compat with older saved views).
const fv = (k) => props.fieldVis?.[k] !== false
// Card-size composition: compact shows only the title; medium a curated subset;
// large everything. The per-field fieldVis toggles further refine within that.
const isCompact = computed(() => props.cardSize === 'compact')
// Compact cards still show subtasks, but always collapsed and name-only (no
// checkbox / priority dot / due), so the expanded stack never applies there.
const subsExpanded = computed(() => props.subtasksExpanded && !isCompact.value)
const SIZE_FIELDS = {
  compact: [],
  medium: ['number', 'priority', 'due', 'tags', 'assignee'],
  large: null, // null = all fields allowed
}
const sizeAllows = (k) => {
  const set = SIZE_FIELDS[props.cardSize] ?? null
  return set === null ? true : set.includes(k)
}
// A field renders when the size preset allows it AND its fieldVis toggle is on.
const show = (k) => sizeAllows(k) && fv(k)
// Whether the pills row has any content (drives whether we render the container).
const hasAnyPill = computed(
  () =>
    hasConflict.value ||
    (show('priority') && (props.showEmpty || props.task.priority)) ||
    (show('due') && (props.showEmpty || due.value)) ||
    (show('estimate') && estText.value) ||
    (show('milestone') && taskMilestone.value) ||
    (show('description') && props.task.description && props.task.description.trim()) ||
    (show('tags') && (props.showEmpty || taskTags.value.length)) ||
    (show('assignee') &&
      (props.showEmpty || author.value || assignees.value.length || glAssignees.value.length)),
)

// Picker tags grouped by prefix (friendly name); a single prefix-less bucket
// renders flat without a header.
const tagPickerGroups = computed(() =>
  buildTagGroups(props.tags, props.tagPrefixNames, props.metaTagPrefixes),
)
const tagPickerHeaders = computed(() => tagPickerGroups.value.length > 1)

const newTagName = ref('')
const editingTitle = ref(false)
const titleEdit = ref('')
const titleInput = ref(null)
// The title is clamped to 2 lines; show the full-text tooltip only when it
// actually overflows (measured on hover, like a pill tooltip).
const titleEl = ref(null)
const titleTruncated = ref(false)
function checkTruncated() {
  const el = titleEl.value
  titleTruncated.value = !!el && el.scrollHeight > el.clientHeight + 1
}
// Single click on the title opens the modal (like the rest of the card), double
// click edits it. A real double-click fires click→click→dblclick, so debounce
// the open just for the title: dblclick cancels the pending open and edits.
let titleClickTimer = null
function onTitleClick() {
  if (titleClickTimer) return
  titleClickTimer = setTimeout(() => {
    titleClickTimer = null
    emit('open', props.task.id)
  }, 220)
}
function onTitleDblClick() {
  if (titleClickTimer) {
    clearTimeout(titleClickTimer)
    titleClickTimer = null
  }
  startTitleEdit()
}
function startTitleEdit() {
  if (props.readonly) return
  titleEdit.value = props.task.title
  editingTitle.value = true
  nextTick(() => titleInput.value?.focus?.())
}
async function commitTitle() {
  editingTitle.value = false
  const n = titleEdit.value.trim()
  if (!n || n === props.task.title) return
  await apply({ title: n })
}

const taskTags = computed(() =>
  (props.task.tag_ids || []).map((id) => props.tagsMap[id]).filter(Boolean),
)
// The single-tag pill hands its box over to TagPill when the tag is scoped, so
// the GitLab-EE two-tone pill isn't boxed-in by the button's own soft fill.
const firstTagScoped = computed(() => {
  const t = taskTags.value[0]
  return t ? tagParts(t.name, props.tagPrefixNames).hasScope : false
})
// Stacked tags row: fit as many chips as the row width allows, rest → +N.
const stagValEl = ref(null)
const stagMeasureEl = ref(null)
const { visibleCount: visibleTagCount } = useTagFit(stagValEl, stagMeasureEl, taskTags, { pad: 4 })
const taskMilestone = computed(() =>
  props.task.milestone_id ? props.milestonesMap[props.task.milestone_id] || null : null,
)
const assignees = computed(() =>
  (props.task.assignee_ids || []).map((id) => props.membersMap[id]).filter(Boolean),
)
// External GitLab assignees (no Tessera account). The board query carries only
// their names/logins (no avatar), so resolve the avatar from the workspace
// GitLab-members list by username — same source the picker/filter already use.
const glAssignees = computed(() => {
  const logins = props.task.gitlab_assignee_logins || []
  const names = props.task.gitlab_assignees || []
  if (!logins.length) return names.map((n) => ({ name: n }))
  return logins.map((login) => {
    const m = props.gitlabMembers.find((x) => x.gl_username === login)
    return { login, name: m?.gl_name || login, avatar_url: m?.gl_avatar_url || null }
  })
})
// Author (read-only): GitLab issue author for synced cards, else the Tessera
// creator resolved from created_by.
const author = computed(() => {
  const t = props.task
  if (t.gitlab_author)
    return {
      name: t.gitlab_author_name || t.gitlab_author,
      login: t.gitlab_author,
      avatar: t.gitlab_author_avatar_url,
      gl: true,
    }
  if (t.created_by) {
    const m = props.membersMap[t.created_by]
    if (m) return { name: m.name, id: t.created_by }
  }
  return null
})
// When the author is also an assignee, don't render a separate (muted) author
// avatar — the person already shows once as the accent assignee. The tooltip
// still lists both roles.
const authorIsAssignee = computed(() => {
  const a = author.value
  if (!a) return false
  if (a.id) return (props.task.assignee_ids || []).includes(a.id)
  // GitLab author (no Tessera id): match by login/name against GitLab assignees.
  const key = (a.login || a.name || '').toLowerCase()
  return glAssignees.value.some((g) => {
    const gn = (g.name || g || '').toString().toLowerCase()
    return gn === key || (g.login || '').toLowerCase() === key
  })
})

// Combined assignee names (Tessera + GitLab) for the merged people tooltip.
const assigneeNames = computed(() =>
  [
    ...assignees.value.map((u) => u.name),
    ...glAssignees.value.map((g) => `${g.name || g} (GitLab)`),
  ].join(', '),
)

// Assignee picker: a search box + a list capped at 10, ordered assigned →
// recently-picked → alphabetical. "Recent" is a small cross-board localStorage
// MRU of user ids the user has assigned, so the people you actually use surface
// first; falls back to alphabetical when there's no history.
const RECENT_ASSIGNEES_KEY = 'tessera_recent_assignees'
function readRecentAssignees() {
  try {
    const v = JSON.parse(localStorage.getItem(RECENT_ASSIGNEES_KEY) || '[]')
    return Array.isArray(v) ? v : []
  } catch {
    return []
  }
}
const assigneeQuery = ref('')
const recentAssignees = ref(readRecentAssignees())
const pickerMembers = computed(() => {
  const q = assigneeQuery.value.trim().toLowerCase()
  if (q) return props.members.filter((m) => (m.name || '').toLowerCase().includes(q))
  // No query: assigned first, then MRU, then alphabetical — deduped, capped at 10
  // (but never hiding a currently-assigned member so they can be removed).
  const byId = new Map(props.members.map((m) => [m.user_id, m]))
  const seen = new Set()
  const out = []
  const add = (id) => {
    if (seen.has(id) || !byId.has(id)) return
    seen.add(id)
    out.push(byId.get(id))
  }
  const assigned = props.task.assignee_ids || []
  assigned.forEach(add)
  recentAssignees.value.forEach(add)
  ;[...props.members]
    .sort((a, b) => (a.name || '').localeCompare(b.name || ''))
    .forEach((m) => add(m.user_id))
  return out.slice(0, Math.max(10, assigned.length))
})

const due = computed(() => formatDue(props.task.due_date))
// Long form (capitalised, full weekday) for the stacked row, where the terse
// lowercase pill form would clash with capitalised siblings like the priority.
const dueLong = computed(() => formatDue(props.task.due_date, { long: true }))
// Overdue: due date in the past on a not-yet-done task.
const overdue = computed(
  () => !!props.task.due_date && !done.value && Date.parse(props.task.due_date) < Date.now(),
)
const dueTs = computed(() => (props.task.due_date ? Date.parse(props.task.due_date) : null))
const startTs = computed(() => (props.task.start_date ? Date.parse(props.task.start_date) : null))
const done = computed(() => !!props.task.completed_at)
// Estimate chip: the task's own estimate, or — if unset — the rollup sum of its
// subtasks (so a parent shows "Σ …"). Unit resolved from the project config.
const estCfg = computed(() => wsStore.estimationFor(props.projectId))
const ownEstimate = computed(() => props.task?.estimate ?? null)
const rollupEstimate = computed(() => sumEstimates(props.subtasks))
const estIsRollup = computed(() => ownEstimate.value == null && rollupEstimate.value != null)
const estText = computed(() => {
  const v = ownEstimate.value ?? rollupEstimate.value
  return v != null ? formatEstimate(v, estCfg.value) : ''
})
// Hover tooltip: full spelled-out estimate + projected window (own estimate only).
const estTooltip = computed(() => {
  const v = ownEstimate.value ?? rollupEstimate.value
  if (v == null) return ''
  const prefix = estIsRollup.value ? 'Сумма оценок подзадач: ' : 'Оценка: '
  const body = estIsRollup.value
    ? formatEstimateFull(v, estCfg.value)
    : estimateTooltip(props.task?.start_date, v, estCfg.value)
  return `${prefix}${body}`
})
const priorityOptions = PRIORITY_LABELS.map((label, value) => ({ label, value }))
// Stacked-cards effect: offset colored shadows behind the top tag pill.
// Stacked-cards: each deeper layer peeks 5px further right and is a little
// shorter (larger negative spread) so it reads as a stack behind the top pill.
const stackLayers = computed(() => Math.min(taskTags.value.length - 1, 2))
const stackShadow = computed(() => {
  if (taskTags.value.length < 2) return ''
  // Each deeper layer peeks 5px further right and is a touch shorter (negative
  // spread). Colours are mixed to an *opaque* soft tint of the tag's hue so the
  // layers don't show through one another (the old translucent `55` alpha did).
  return taskTags.value
    .slice(1, 3)
    .map(
      (t, i) =>
        `${(i + 1) * 5}px 0 0 ${-(i + 1)}px color-mix(in srgb, ${t.color || '#888'} 45%, var(--t-surface))`,
    )
    .join(', ')
})
const cardStyle = computed(() => {
  if (!props.task.priority) return {}
  const c = PRIORITY_COLORS[props.task.priority]
  return {
    '--card-bar': hueGradVert(c),
    // Whole-card border tinted a very muted priority hue (roughly as subtle as a
    // column's background wash) — enough to read as "coloured", not to shout.
    '--card-border': `color-mix(in srgb, ${c} 12%, var(--t-border))`,
  }
})
// Shared flag gradient defs live in App.vue (one per priority level), so a board
// with 100s of cards references 4 defs instead of inlining an <svg> per card.
const flagGradId = computed(() => (props.task.priority ? `t-prio-grad-${props.task.priority}` : ''))

function isAssigned(uid) {
  return (props.task.assignee_ids || []).includes(uid)
}
function hasTag(id) {
  return (props.task.tag_ids || []).includes(id)
}

function base() {
  return {
    title: props.task.title,
    description: props.task.description || '',
    priority: props.task.priority || 0,
    due_date: props.task.due_date || null,
    start_date: props.task.start_date || null,
    recurrence: props.task.recurrence || null,
    completed: done.value,
  }
}
async function apply(patch) {
  await tasksApi.update(props.task.id, { ...base(), ...patch })
  emit('changed')
}
const toggleDone = () => apply({ completed: !done.value })
const setPriority = (p) => apply({ priority: p })

// ── right-click context menu (works for the card and for collapsed subtasks) ──
const ctxShow = ref(false)
const ctxX = ref(0)
const ctxY = ref(0)
const ctxTarget = ref(null) // task object the menu acts on
const showDeleteConfirm = ref(false)
const pendingDeleteTarget = ref(null)
const showArchiveConfirm = ref(false)
const pendingArchiveTarget = ref(null)
const ctxOptions = computed(() => {
  const t = ctxTarget.value
  const isMain = t && t.id === props.task.id
  const cols = props.columns.filter((c) => c.id !== t?.column_id)
  return [
    { label: 'Открыть', key: 'open', icon: menuIcon(OpenOutline) },
    {
      label: t?.completed_at ? 'Снять выполнение' : 'Отметить выполненной',
      key: 'toggle',
      icon: menuIcon(CheckmarkDoneOutline),
    },
    {
      label: 'Приоритет',
      key: 'prio',
      icon: menuIcon(FlagOutline),
      children: PRIORITY_LABELS.map((l, i) => ({ label: l, key: 'prio:' + i })),
    },
    ...(cols.length
      ? [
          {
            label: 'Переместить в колонку',
            key: 'move',
            icon: menuIcon(ArrowForwardOutline),
            children: cols.map((c) => ({ label: c.name, key: 'col:' + c.id })),
          },
        ]
      : []),
    { type: 'divider', key: 'd1' },
    ...(isMain
      ? [{ label: 'Создать подзадачу', key: 'subtask', icon: menuIcon(GitBranchOutline) }]
      : []),
    { label: 'В архив', key: 'archive', icon: menuIcon(ArchiveOutline) },
    {
      label: 'Удалить',
      key: 'delete',
      icon: dangerIcon(TrashOutline),
      props: { style: 'color:#e0533d' },
    },
  ]
})
function baseOf(t) {
  return {
    title: t.title,
    description: t.description || '',
    priority: t.priority || 0,
    due_date: t.due_date || null,
    start_date: t.start_date || null,
    recurrence: t.recurrence || null,
    completed: !!t.completed_at,
  }
}
function onCtx(e, target) {
  if (props.readonly) return // archive view: no context menu
  if (pressMoved()) return // the finger moved (a drag) — don't pop the menu
  ctxTarget.value = target || props.task
  ctxShow.value = false
  ctxX.value = e.clientX
  ctxY.value = e.clientY
  nextTick(() => (ctxShow.value = true))
}
async function onCtxSelect(key) {
  const t = ctxTarget.value || props.task
  ctxShow.value = false
  if (key === 'open') return emit('open', t.id)
  if (key === 'subtask') return startAddSub()
  if (key === 'delete') {
    pendingDeleteTarget.value = t
    showDeleteConfirm.value = true
    return
  }
  if (key === 'archive') {
    pendingArchiveTarget.value = t
    showArchiveConfirm.value = true
    return
  }
  try {
    if (key === 'toggle') await tasksApi.update(t.id, { ...baseOf(t), completed: !t.completed_at })
    else if (key.startsWith('prio:'))
      await tasksApi.update(t.id, { ...baseOf(t), priority: Number(key.slice(5)) })
    else if (key.startsWith('col:'))
      await tasksApi.move(t.id, { column_id: key.slice(4), before_id: null, after_id: null })
    emit('changed')
  } catch {
    /* surfaced by the board's reload path */
  }
}
async function doCtxDelete() {
  const t = pendingDeleteTarget.value
  if (!t) return
  try {
    await tasksApi.remove(t.id)
    emit('changed')
  } catch {
    /* surfaced by the board's reload path */
  }
}
async function doCtxArchive() {
  const t = pendingArchiveTarget.value
  if (!t) return
  try {
    await tasksApi.archive(t.id)
    emit('changed')
  } catch {
    /* surfaced by the board's reload path */
  }
}
// Per-task due-notification override sentinels (-1 / 'inherit' = user default).
const dueEnabledSel = computed(() => {
  const v = props.task.due_notify_enabled
  return v == null ? 'inherit' : v ? 'on' : 'off'
})
const dueLeadSel = computed(() => props.task.due_lead_minutes ?? -1)
const dueRepeatSel = computed(() => props.task.due_repeat_minutes ?? -1)
async function saveDueNotify(patch) {
  const lead = patch.lead ?? dueLeadSel.value
  const repeat = patch.repeat ?? dueRepeatSel.value
  const enabled = patch.enabled ?? dueEnabledSel.value
  try {
    await tasksApi.dueNotify(props.task.id, {
      lead_minutes: lead === -1 ? null : lead,
      repeat_minutes: repeat === -1 ? null : repeat,
      enabled: enabled === 'inherit' ? null : enabled === 'on',
    })
    emit('changed')
  } catch (e) {
    void e
  }
}

async function toggleTag(id) {
  if (hasTag(id)) await tasksApi.removeTag(props.task.id, id)
  else await tasksApi.addTag(props.task.id, id)
  emit('changed')
}
async function createTag() {
  const n = newTagName.value.trim()
  if (!n) return
  const palette = ['#7c5cff', '#2f80ed', '#0eb0a9', '#18a058', '#f0a020', '#e0533d', '#eb2f96']
  const res = await projectsApi.createTag(props.projectId, {
    name: n,
    color: palette[Math.floor(Math.random() * palette.length)],
  })
  await tasksApi.addTag(props.task.id, res.data.id)
  newTagName.value = ''
  emit('changed')
}
async function toggleAssignee(uid) {
  const adding = !isAssigned(uid)
  if (adding) await tasksApi.addAssignee(props.task.id, uid)
  else await tasksApi.removeAssignee(props.task.id, uid)
  if (adding) {
    // Bump to the front of the MRU list (cap the stored history).
    const next = [uid, ...recentAssignees.value.filter((x) => x !== uid)].slice(0, 30)
    recentAssignees.value = next
    localStorage.setItem(RECENT_ASSIGNEES_KEY, JSON.stringify(next))
  }
  emit('changed')
}
// GitLab-member assignees: board tasks carry their logins in gitlab_assignee_logins.
const glAssigneeLogins = computed(() => props.task.gitlab_assignee_logins || [])
function isGlAssigned(username) {
  return glAssigneeLogins.value.includes(username)
}
async function toggleGlAssignee(m) {
  if (isGlAssigned(m.gl_username)) await tasksApi.removeGitlabAssignee(props.task.id, m.gl_username)
  else
    await tasksApi.pinGitlabAssignee(props.task.id, {
      gl_username: m.gl_username,
      gl_name: m.gl_name,
      gl_avatar_url: m.gl_avatar_url,
    })
  emit('changed')
}

// ── subtasks ──
// The composer filter hid some of this card's children (task #2602): the board
// shows only the matching ones, the modal still lists them all.
const subsNarrowed = computed(() => props.subtasksTotal > props.subtasks.length)
const addingSub = ref(false)
const newSubTitle = ref('')
const subInput = ref(null)
// Mutable mirror for drag-reorder of subtasks; resynced from the prop.
const subModel = ref([])
watch(
  () => props.subtasks,
  (v) => (subModel.value = [...v]),
  { immediate: true, deep: true },
)
// A card dropped into this card's subtask list becomes its subtask; a subtask
// dragged within the list is just reordered.
async function onSubChange(evt) {
  if (evt.added) {
    try {
      await tasksApi.setParent(evt.added.element.id, props.task.id)
    } catch (e) {
      void e
    }
    emit('changed')
    return
  }
  if (!evt.moved) return
  const arr = subModel.value
  const before = arr[evt.moved.newIndex - 1]
  const after = arr[evt.moved.newIndex + 1]
  try {
    await tasksApi.move(evt.moved.element.id, {
      column_id: props.task.column_id,
      before_id: before ? before.id : null,
      after_id: after ? after.id : null,
    })
    emit('changed')
  } catch (e) {
    void e
    emit('changed')
  }
}
function subDue(s) {
  return formatDue(s.due_date)
}
async function toggleSubDone(s) {
  if (props.readonly) return
  await tasksApi.update(s.id, {
    title: s.title,
    description: s.description || '',
    priority: s.priority || 0,
    due_date: s.due_date || null,
    start_date: s.start_date || null,
    recurrence: s.recurrence || null,
    completed: !s.completed_at,
  })
  emit('changed')
}
function startAddSub() {
  addingSub.value = true
  newSubTitle.value = ''
  nextTick(() => subInput.value?.focus?.())
}
async function submitAddSub() {
  const t = newSubTitle.value.trim()
  // Clear + close BEFORE awaiting so the @blur that fires when the input is
  // removed doesn't re-submit the same title (was creating a duplicate).
  newSubTitle.value = ''
  addingSub.value = false
  if (!t) return
  await boardsApi.createTask(props.task.board_id, {
    column_id: props.task.column_id,
    parent_id: props.task.id,
    title: t,
  })
  emit('changed')
}
</script>

<template>
  <div class="tw">
    <div
      class="card"
      :class="{
        done,
        nested,
        'has-subs': !nested && subtasks.length,
        'has-prio': task.priority,
        'tc-readonly': readonly,
      }"
      :style="cardStyle"
      @click="emit('open', task.id)"
      @contextmenu.prevent.stop="onCtx"
    >
      <!-- Archive view: a single Restore affordance replaces the edit action bar. -->
      <div v-if="readonly && !nested" class="card-actions" @click.stop>
        <button
          class="ca-btn ca-restore"
          title="Вернуть из архива"
          @click.stop="emit('restore', task.id)"
        >
          <n-icon :component="ArrowUndoOutline" :size="15" />
        </button>
      </div>
      <!-- hover quick-actions: complete · add subtask · more.
           Hidden while renaming (they'd overlap the inline editor). On touch
           (no hover) complete + more persist — see @media(hover:none). -->
      <div v-if="!nested && !editingTitle && !readonly" class="card-actions" @click.stop>
        <button
          class="ca-btn ca-complete"
          :title="done ? 'Вернуть в работу' : 'Отметить выполненной'"
          @click.stop="toggleDone"
        >
          <n-icon :component="done ? CheckmarkCircle : CheckmarkOutline" :size="15" />
        </button>
        <button class="ca-btn ca-sub" title="Добавить подзадачу" @click.stop="startAddSub">
          <n-icon :component="GitBranchOutline" :size="15" />
        </button>
        <button class="ca-btn ca-more" title="Ещё" @click.stop="onCtx">
          <n-icon :component="EllipsisHorizontal" :size="16" />
        </button>
      </div>

      <!-- No on-card checkbox: completion lives in the hover bar / context menu
           (the check icon left of the title was redundant). -->
      <div class="card-top">
        <n-input
          v-if="editingTitle"
          ref="titleInput"
          v-model:value="titleEdit"
          size="small"
          class="title-edit"
          @click.stop
          @keyup.enter="commitTitle"
          @blur="commitTitle"
        />
        <n-tooltip
          v-else
          :disabled="!titleTruncated"
          placement="top-start"
          :style="{ maxWidth: '320px' }"
        >
          <template #trigger>
            <span
              ref="titleEl"
              class="title"
              @mouseenter="checkTruncated"
              @click.stop="onTitleClick"
              @dblclick.stop="onTitleDblClick"
              >{{ task.title }}</span
            >
          </template>
          {{ task.title }}
        </n-tooltip>
      </div>

      <!-- meta line: task number + GitLab issue link, on their own row so long
           titles never wrap around them (kept aligned under the title text). -->
      <div
        v-if="(show('number') && task.number) || (show('gitlab') && task.gitlab_iid)"
        class="card-sub"
      >
        <span v-if="show('number') && task.number" class="tnum">#{{ task.number }}</span>
        <a
          v-if="show('gitlab') && task.gitlab_iid"
          class="gl-chip"
          :href="task.gitlab_url"
          target="_blank"
          rel="noopener"
          :title="`GitLab issue !${task.gitlab_iid} — открыть`"
          @click.stop
        >
          <n-icon :component="LogoGitlab" :size="11" />!{{ task.gitlab_iid }}
        </a>
      </div>

      <div v-if="hasAnyPill" class="pills" :class="{ stacked: stackFields }">
        <!-- unresolved GitLab write-back conflict on this task -->
        <n-tooltip v-if="hasConflict">
          <template #trigger>
            <button
              class="pill set conf-pill"
              @click.stop="conflictsStore.openResolver(props.task.id)"
            >
              <n-icon :component="WarningOutline" :size="13" />
              <span class="pill-text">Конфликт</span>
            </button>
          </template>
          Конфликт обратной записи GitLab — нажмите, чтобы разрешить
        </n-tooltip>

        <!-- Fields render as horizontal pills, or (stack mode) as full-width
             "icon + value" rows — same triggers/pickers, so every field stays
             clickable; hover highlights the row (see .pills.stacked CSS). -->
        <!-- priority -->
        <n-popover
          v-if="show('priority') && (showEmpty || task.priority || stackFields)"
          trigger="click"
          placement="bottom-start"
        >
          <template #trigger>
            <button
              class="pill"
              :class="{ set: task.priority }"
              :title="stackFields ? 'Приоритет' : ''"
              @click.stop
            >
              <n-icon
                :component="FlagOutline"
                :size="13"
                :style="
                  task.priority
                    ? {
                        color: PRIORITY_COLORS[task.priority],
                        '--icon-grad': `url(#${flagGradId})`,
                      }
                    : {}
                "
              />
              <span v-if="stackFields" class="pill-text" :class="{ 'sf-empty': !task.priority }">{{
                task.priority ? PRIORITY_LABELS[task.priority] : '—'
              }}</span>
            </button>
          </template>
          <div class="menu">
            <div
              v-for="o in priorityOptions"
              :key="o.value"
              class="menu-item"
              @click="setPriority(o.value)"
            >
              <span class="dot" :style="{ background: hueGrad(PRIORITY_COLORS[o.value]) }" />
              {{ o.label }}
            </div>
          </div>
        </n-popover>

        <!-- due date: opens the calendar directly -->
        <n-popover
          v-if="show('due') && (showEmpty || due || stackFields)"
          trigger="click"
          placement="bottom-start"
        >
          <template #trigger>
            <button
              class="pill"
              :class="{ set: due, overdue }"
              :title="stackFields ? 'Срок' : ''"
              @click.stop
            >
              <n-icon :component="CalendarClearOutline" :size="13" />
              <span v-if="due" class="pill-text">{{ stackFields ? dueLong : due }}</span>
              <span v-else-if="stackFields" class="pill-text sf-empty">—</span>
              <n-icon
                v-if="task.recurrence"
                :component="RepeatOutline"
                :size="11"
                class="pill-recur"
                title="Повторяемая задача"
              />
            </button>
          </template>
          <DueEditor
            :due="dueTs"
            :start="startTs"
            :recurrence="task.recurrence"
            :notify="{ enabled: dueEnabledSel, lead: dueLeadSel, repeat: dueRepeatSel }"
            :columns="columns"
            @apply="apply"
            @notify="saveDueNotify"
          />
        </n-popover>

        <!-- estimate: display-only chip (own value, or Σ subtask rollup) -->
        <n-tooltip v-if="show('estimate') && (estText || stackFields)">
          <template #trigger>
            <div
              class="pill"
              :class="{ set: estText, 'est-pill': estText }"
              :title="stackFields ? 'Оценка' : ''"
            >
              <n-icon :component="TimerOutline" :size="13" />
              <span v-if="estText" class="pill-text"
                >{{ estIsRollup ? 'Σ ' : '' }}{{ estText }}</span
              >
              <span v-else-if="stackFields" class="pill-text sf-empty">—</span>
            </div>
          </template>
          {{ estText ? estTooltip : 'Оценка' }}
        </n-tooltip>

        <!-- milestone («Этап»): display-only chip; editing lives in the task modal -->
        <n-tooltip v-if="show('milestone') && (taskMilestone || stackFields)">
          <template #trigger>
            <div
              class="pill"
              :class="{
                set: taskMilestone,
                'ms-pill': taskMilestone,
                closed: taskMilestone && taskMilestone.state === 'closed',
              }"
              :title="stackFields ? 'Этап' : ''"
            >
              <n-icon :component="RibbonOutline" :size="13" />
              <span v-if="taskMilestone" class="pill-text">{{ taskMilestone.title }}</span>
              <span v-else-if="stackFields" class="pill-text sf-empty">—</span>
            </div>
          </template>
          <template v-if="taskMilestone">
            Этап: {{ taskMilestone.title }}{{ taskMilestone.state === 'closed' ? ' (закрыт)' : '' }}
            <template v-if="milestoneRange(taskMilestone)">
              · {{ milestoneRange(taskMilestone) }}</template
            >
          </template>
          <template v-else>Этап</template>
        </n-tooltip>

        <!-- description: shown only when set (and not in stack mode); hover previews
             the rendered markdown, click opens the task. -->
        <n-popover
          v-if="show('description') && !stackFields && task.description && task.description.trim()"
          trigger="hover"
          placement="top-start"
          :style="{ padding: '0' }"
        >
          <template #trigger>
            <div
              class="pill set desc-pill"
              title="Есть описание"
              @click.stop="emit('open', task.id)"
            >
              <n-icon :component="ReorderThreeOutline" :size="14" />
            </div>
          </template>
          <div class="desc-pop">
            <RichContent :source="task.description" :members="members" />
          </div>
        </n-popover>

        <!-- tags: stacked when >1; hover previews full list, click opens picker -->
        <n-popover
          v-if="show('tags') && (showEmpty || taskTags.length || stackFields)"
          trigger="click"
          placement="bottom-start"
        >
          <template #trigger>
            <n-popover trigger="hover" :disabled="taskTags.length < 2" placement="top-start">
              <template #trigger>
                <button v-if="!stackFields && !taskTags.length" class="pill" @click.stop>
                  <n-icon :component="PricetagOutline" :size="13" />
                </button>
                <button
                  v-else-if="!stackFields"
                  class="pill tag-pill"
                  :style="
                    firstTagScoped
                      ? {
                          border: 'none',
                          background: 'none',
                          padding: 0,
                          boxShadow: stackShadow,
                          marginRight: stackLayers ? stackLayers * 4 + 'px' : undefined,
                        }
                      : {
                          border: '1px solid transparent',
                          background: tagPillBg(taskTags[0].color),
                          boxShadow: stackShadow,
                          marginRight: stackLayers ? stackLayers * 4 + 'px' : undefined,
                        }
                  "
                  @click.stop
                >
                  <TagPill
                    class="tname"
                    :tag="taskTags[0]"
                    :prefix-names="tagPrefixNames"
                    variant="grad-text"
                  />
                  <span
                    v-if="taskTags.length > 1"
                    class="more"
                    :style="{ color: tagText(taskTags[0].color) }"
                    >+{{ taskTags.length - 1 }}</span
                  >
                </button>
                <!-- stacked: leading tag icon + outlined-oval chips that fit on the
                     row, rest → +N (same behaviour as the task modal). -->
                <button v-else class="pill" title="Теги" @click.stop>
                  <n-icon
                    :component="PricetagOutline"
                    :size="13"
                    :style="taskTags.length ? { color: tagText(taskTags[0].color) } : {}"
                  />
                  <span v-if="taskTags.length" ref="stagValEl" class="stag-val">
                    <TagPill
                      v-for="t in taskTags.slice(0, visibleTagCount)"
                      :key="t.id"
                      class="mchip"
                      :tag="t"
                      :prefix-names="tagPrefixNames"
                      variant="outline"
                    />
                    <span
                      v-if="visibleTagCount < taskTags.length"
                      class="mchip chip-more"
                      :style="{
                        color: tagText(taskTags[0].color),
                        background: softFill(taskTags[0].color),
                      }"
                      >+{{ taskTags.length - visibleTagCount }}</span
                    >
                    <!-- measurement copies must be the same component with the same
                         props, or the scope segment wouldn't be measured (useTagFit). -->
                    <span ref="stagMeasureEl" class="stag-measure" aria-hidden="true">
                      <TagPill
                        v-for="t in taskTags"
                        :key="`m${t.id}`"
                        class="mchip"
                        :tag="t"
                        :prefix-names="tagPrefixNames"
                        variant="outline"
                      />
                    </span>
                  </span>
                  <span v-else class="pill-text sf-empty">—</span>
                </button>
              </template>
              <div class="preview">
                <TagPill
                  v-for="t in taskTags"
                  :key="t.id"
                  class="chip"
                  :tag="t"
                  :prefix-names="tagPrefixNames"
                  variant="ghost"
                />
              </div>
            </n-popover>
          </template>
          <div class="menu tagmenu">
            <div class="chip-groups">
              <div v-for="g in tagPickerGroups" :key="g.key" class="chip-group">
                <div v-if="tagPickerHeaders" class="chip-grp-head">{{ g.label }}</div>
                <div class="chip-grid">
                  <button
                    v-for="t in g.tags"
                    :key="t.id"
                    class="tagchip"
                    :class="{ on: hasTag(t.id) }"
                    :style="
                      hasTag(t.id)
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
              @click.stop
            />
          </div>
        </n-popover>

        <!-- author + assignees merged into one overlapping avatar group: the
           author (muted) leads, then the assignee(s). Hover shows the full
           breakdown; click opens the assignee picker (search + recent). The
           group right-aligns (margin-left:auto) and stays right-aligned even
           when it wraps to its own line. -->
        <div
          v-if="
            show('assignee') &&
            (showEmpty || author || assignees.length || glAssignees.length || stackFields)
          "
          class="people"
        >
          <n-popover
            trigger="click"
            placement="bottom-end"
            @update:show="(v) => !v && (assigneeQuery = '')"
          >
            <template #trigger>
              <n-tooltip placement="top">
                <template #trigger>
                  <button
                    class="pill assignee-pill"
                    :title="stackFields ? 'Исполнитель' : ''"
                    @click.stop
                  >
                    <n-icon
                      v-if="stackFields"
                      :component="PersonAddOutline"
                      :size="13"
                      class="sf-people-ic"
                    />
                    <UserAvatar
                      v-if="author && !authorIsAssignee"
                      class="avatar author-ava"
                      :user-id="author.id"
                      :src="author.avatar"
                      :name="author.name"
                    />
                    <UserAvatar
                      v-for="u in assignees"
                      :key="u.user_id"
                      class="avatar"
                      :user-id="u.user_id"
                      :name="u.name"
                    />
                    <UserAvatar
                      v-for="(g, i) in glAssignees"
                      :key="`g${i}`"
                      class="avatar ext-ava"
                      :src="g.avatar_url"
                      :name="g.name || g"
                    />
                    <n-icon
                      v-if="!stackFields && !author && !assignees.length && !glAssignees.length"
                      :component="PersonAddOutline"
                      :size="13"
                    />
                    <span
                      v-else-if="stackFields && !author && !assignees.length && !glAssignees.length"
                      class="pill-text sf-empty"
                      >—</span
                    >
                  </button>
                </template>
                <div class="people-tip">
                  <div v-if="author">
                    Автор: {{ author.gl ? `@${author.login} (GitLab)` : author.name }}
                  </div>
                  <div v-if="assigneeNames">Исполнитель: {{ assigneeNames }}</div>
                  <div v-if="!author && !assigneeNames">Нет исполнителя</div>
                </div>
              </n-tooltip>
            </template>
            <div class="menu assignee-menu">
              <n-input
                v-model:value="assigneeQuery"
                size="tiny"
                placeholder="Поиск"
                clearable
                @click.stop
              />
              <div class="assignee-list">
                <div
                  v-for="m in pickerMembers"
                  :key="m.user_id"
                  class="menu-item assignee-item"
                  @click="toggleAssignee(m.user_id)"
                >
                  <UserAvatar class="avatar sm" :user-id="m.user_id" :name="m.name" />
                  <span class="aname">{{ m.name }}</span>
                  <n-icon v-if="isAssigned(m.user_id)" :component="CheckmarkOutline" class="chk" />
                </div>
                <template v-if="gitlabMembers.length">
                  <div class="assignee-sep">GitLab</div>
                  <div
                    v-for="m in gitlabMembers"
                    :key="m.gl_user_id"
                    class="menu-item assignee-item"
                    @click="toggleGlAssignee(m)"
                  >
                    <UserAvatar
                      class="avatar sm"
                      :src="m.gl_avatar_url"
                      :name="m.gl_name || m.gl_username"
                    />
                    <span class="aname">{{ m.gl_name || m.gl_username }}</span>
                    <n-icon
                      v-if="isGlAssigned(m.gl_username)"
                      :component="CheckmarkOutline"
                      class="chk"
                    />
                  </div>
                </template>
                <div v-if="!pickerMembers.length && !gitlabMembers.length" class="assignee-empty">
                  Никого не найдено
                </div>
              </div>
            </div>
          </n-popover>
        </div>
      </div>
    </div>
    <!-- /.card -->

    <!-- Subtasks: one always-mounted drop list (shared "tasks" group), so a task
         can be dropped to nest even on a childless card. Renders as a fanned
         stack (expanded) or an emerging list card (collapsed); empty → a dashed
         drop hint while a board drag is in progress. -->
    <transition name="sub-morph" mode="out-in">
      <draggable
        v-if="!nested"
        :key="subsExpanded ? 'stack' : 'list'"
        :list="subModel"
        group="tasks"
        item-key="id"
        class="subs"
        :class="{
          stack: subsExpanded,
          list: !subsExpanded,
          compact: isCompact,
          collapsed: !subModel.length && !dragging,
          pending: !subModel.length && dragging,
        }"
        :animation="150"
        :delay="300"
        :touch-start-threshold="6"
        :disabled="subsNarrowed"
        @click.stop
        @change="onSubChange"
      >
        <template #item="{ element: s, index }">
          <div v-if="subsExpanded" class="sub-layer" :style="{ zIndex: 40 - index }">
            <TaskCard
              :task="s"
              :subtasks="[]"
              :nested="true"
              :columns="columns"
              :tags-map="tagsMap"
              :members-map="membersMap"
              :tags="tags"
              :members="members"
              :gitlab-members="gitlabMembers"
              :ws-id="wsId"
              :project-id="projectId"
              :field-vis="fieldVis"
              :show-empty="showEmpty"
              :stack-fields="stackFields"
              :card-size="cardSize"
              @open="emit('open', $event)"
              @changed="emit('changed')"
            />
          </div>
          <!-- Plain wrapper is the draggable item root: vuedraggable needs a single
             real element per item, so the hover n-popover lives INSIDE it (making
             the popover the item root breaks Sortable → the whole parent card drags). -->
          <div v-else class="subrow-slot">
            <n-popover trigger="hover" placement="right" :delay="250">
              <template #trigger>
                <div
                  class="subrow"
                  :class="{ done: s.completed_at }"
                  @click="emit('open', s.id)"
                  @contextmenu.prevent.stop="onCtx($event, s)"
                >
                  <span v-if="!isCompact" class="check sm" @click.stop="toggleSubDone(s)">
                    <n-icon
                      :component="s.completed_at ? CheckmarkCircle : EllipseOutline"
                      :size="15"
                    />
                  </span>
                  <span
                    v-if="!isCompact && s.priority"
                    class="pr-dot"
                    :style="{ background: hueGradVert(PRIORITY_COLORS[s.priority]) }"
                  />
                  <span class="sub-title">{{ s.title }}</span>
                  <span v-if="!isCompact && subDue(s)" class="sub-due">{{ subDue(s) }}</span>
                </div>
              </template>
              <TaskMiniCard
                :task="s"
                :tags-map="tagsMap"
                :members-map="membersMap"
                :tag-prefix-names="tagPrefixNames"
              />
            </n-popover>
          </div>
        </template>
      </draggable>
    </transition>

    <!-- The filter hid part of the children: say so, so a short list doesn't read
         as "this parent only has one subtask". -->
    <div v-if="!nested && subsNarrowed" class="subs-narrowed" @click.stop>
      {{ subtasks.length }} из {{ subtasksTotal }} подзадач — остальные скрыты фильтром
    </div>

    <!-- Adding a subtask is triggered from the hover action bar / context menu;
         this is just the inline title input it reveals. -->
    <div v-if="!nested && addingSub" class="sub-add-input" @click.stop>
      <n-input
        ref="subInput"
        v-model:value="newSubTitle"
        size="tiny"
        placeholder="Название подзадачи, Enter"
        @keyup.enter="submitAddSub"
        @keyup.esc="addingSub = false"
        @blur="submitAddSub"
      />
    </div>

    <n-dropdown
      trigger="manual"
      placement="bottom-start"
      :show="ctxShow"
      :x="ctxX"
      :y="ctxY"
      :options="ctxOptions"
      @select="onCtxSelect"
      @clickoutside="ctxShow = false"
    />
    <n-popconfirm
      v-model:show="showDeleteConfirm"
      :x="ctxX"
      :y="ctxY"
      :positive-button-props="{ type: 'error' }"
      positive-text="Удалить"
      @positive-click="doCtxDelete"
      @clickoutside="showDeleteConfirm = false"
    >
      <template #trigger><span /></template>
      Удалить безвозвратно? Это действие необратимо.
    </n-popconfirm>
    <n-popconfirm
      v-model:show="showArchiveConfirm"
      :x="ctxX"
      :y="ctxY"
      positive-text="В архив"
      @positive-click="doCtxArchive"
      @clickoutside="showArchiveConfirm = false"
    >
      <template #trigger><span /></template>
      Перенести задачу в архив?
    </n-popconfirm>
  </div>
</template>

<style scoped>
.tw {
  margin-bottom: 8px;
}
/* "Drop to nest" zone — collapsed (and unhittable) until a drag is in progress. */
/* Empty + idle → fully hidden (overrides the list/stack block styling). Empty
   while a board drag is in progress → keep the block (a small drop area) so a
   dropped task attaches under the card, same as a card that already has subs. */
.subs.collapsed {
  /* Fully hidden when idle: `display:none` beats the `.subs.list` padding/border
     that (being later in source) would otherwise leak a ~20px empty ghost box
     under childless cards. The drop zone still appears via `.subs.pending`
     while a board drag is in progress. */
  display: none;
}
.subs.pending {
  min-height: 26px;
}
.card {
  --card-fill: var(--t-surface);
  position: relative;
  background: var(--card-fill);
  border: 1px solid var(--t-border);
  border-radius: 12px;
  padding: 8px 10px;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.04);
  cursor: pointer;
}
/* Priority accent: a 3px vertical-gradient LEFT border that wraps the rounded
   top-left / bottom-left corners (extending onto the top/bottom edges by the
   radius), exactly like the Android client. Implementation: the left border is
   transparent and the gradient is painted on the border-box background layer
   (so it shows through the transparent border and follows the corner radius);
   the padding-box layer keeps the interior flat. The other three borders stay
   the opaque neutral colour, hiding the gradient there. */
.card.has-prio {
  /* All four borders take a muted priority tint; the left one is then made
     transparent again to reveal the 3px gradient accent bar below. */
  border-color: var(--card-border, var(--t-border));
  border-left-width: 3px;
  border-left-color: transparent;
  background:
    linear-gradient(var(--card-fill), var(--card-fill)) padding-box,
    var(--card-bar) border-box;
}
/* The parent keeps its rounded corners and sits above the subtask stack so the
   children appear to emerge from under it. */
.card.has-subs {
  position: relative;
  z-index: 50;
}
/* Archive (read-only) card: property pills/checkboxes become display-only so a
   click passes through to open the read-only modal; only the Restore button acts. */
.card.tc-readonly .pill,
.card.tc-readonly .check,
.card.tc-readonly .assignee-pill,
.card.tc-readonly .tag-chip {
  pointer-events: none;
}
.card.tc-readonly .ca-restore {
  color: #b5792a;
}
/* Hover action bar — floats over the card's top-right corner; revealed on hover
   (or keyboard focus within the card). Sits above the title. */
.card-actions {
  position: absolute;
  top: 6px;
  right: 6px;
  display: flex;
  gap: 2px;
  padding: 2px;
  border-radius: 8px;
  background: var(--t-surface);
  border: 1px solid var(--t-border);
  box-shadow: 0 1px 4px rgba(0, 0, 0, 0.1);
  opacity: 0;
  transform: translateY(-2px);
  transition:
    opacity 0.12s ease,
    transform 0.12s ease;
  pointer-events: none;
  z-index: 6;
}
.card:hover .card-actions,
.card:focus-within .card-actions {
  opacity: 1;
  transform: none;
  pointer-events: auto;
}
.ca-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border: none;
  background: transparent;
  color: var(--t-text2);
  border-radius: 6px;
  cursor: pointer;
}
.ca-btn:hover {
  background: var(--t-hover);
  color: var(--t-primary);
}
/* Touch devices have no hover — keep complete + more persistently reachable
   (there's no on-card checkbox anymore); add-subtask stays in the "⋯" menu. */
@media (hover: none) {
  .card-actions {
    opacity: 1;
    transform: none;
    pointer-events: auto;
    background: transparent;
    border: none;
    box-shadow: none;
  }
  .ca-sub {
    display: none;
  }
  .ca-btn {
    color: var(--t-text3);
  }
}
/* Background shared by subtask cards / the collapsed list. Tweak here:
   alternatives — var(--t-hover), var(--t-border) (greyer),
   color-mix(in srgb, var(--t-primary) 8%, var(--t-surface)) (accent). */
.sub-layer,
.subs.list {
  --sub-bg: color-mix(in srgb, var(--t-surface) 70%, var(--t-bg));
}
/* Each expanded subtask card: rounded bottom only, peeking ~8px from under the
   card above it, with its own shadow → a fanned-down stack. */
.sub-layer {
  position: relative;
  margin-top: -8px;
}
.sub-layer > .tw {
  margin-bottom: 0;
}
.card.nested {
  --card-fill: var(--sub-bg);
  border-radius: 0 0 12px 12px;
  padding-top: 16px;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.04);
}
/* Collapsed: a single card emerging from under the parent, holding the list. */
.subs.list {
  position: relative;
  z-index: 1;
  margin-top: -8px;
  padding: 14px 8px 6px;
  background: var(--sub-bg);
  border: 1px solid var(--t-border);
  border-radius: 0 0 12px 12px;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.04);
}
/* A CHILDLESS card's drop-to-nest target during a drag: don't paint the solid
   emerging-list card (it read as stray empty space under every card) — just a
   slim dashed accent slot that clearly says "drop here to nest". Higher
   specificity so it wins over the .subs.list block above regardless of order. */
.subs.list.pending {
  min-height: 14px;
  margin-top: 4px;
  padding: 0;
  background: transparent;
  border: 1px dashed color-mix(in srgb, var(--t-primary) 45%, transparent);
  border-radius: 8px;
  box-shadow: none;
}
.title-edit {
  flex: 1;
  width: 100%;
}
.card-top {
  display: flex;
  align-items: flex-start;
  gap: 8px;
}
.check {
  cursor: pointer;
  color: var(--t-text3);
  display: inline-flex;
}
.card.done .title {
  text-decoration: line-through;
  opacity: 0.6;
}
/* Title takes the full row and clamps to two lines with an ellipsis; the number
   and GitLab chip live on the meta row below, so the title never wraps around
   them (which used to squeeze it to one word per line). */
.title {
  flex: 1;
  min-width: 0;
  font-size: 14px;
  line-height: 20px;
  color: var(--t-text1);
  cursor: pointer;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
  overflow-wrap: anywhere;
}
/* meta row: number + GitLab chip, directly under the title. */
.card-sub {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 3px;
}
.tnum {
  flex: none;
  font-size: 11px;
  color: var(--t-text3);
}
/* "synced from GitLab" chip — links to the source issue. */
.gl-chip {
  flex: none;
  display: inline-flex;
  align-items: center;
  gap: 2px;
  font-size: 10px;
  font-weight: 600;
  line-height: 1;
  padding: 2px 5px;
  border-radius: 999px;
  text-decoration: none;
  color: var(--t-text2);
  border: 1px solid var(--t-border);
  background: var(--t-hover);
}
.gl-chip:hover {
  color: var(--t-primary);
  border-color: var(--t-primary);
}
.pills {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  margin-top: 8px;
}
/* Stacked mode (customize view): the same pill triggers, laid out as full-width
   "icon + value" rows. Field icons stay flush-left so values align on one axis;
   rows highlight on hover and remain clickable (open the same pickers). */
.pills.stacked {
  flex-direction: column;
  align-items: stretch;
  gap: 1px;
}
.pills.stacked .people {
  margin-left: 0;
  width: 100%;
}
/* box-sizing:border-box is the key fix: <button> pills default to border-box but
   the display-only <div> pills (estimate/milestone) are content-box, so width:100%
   + padding made them overflow the card and mismatch height. Force border-box so
   every row is the same height and its hover spans exactly the card width. */
.pills.stacked .pill,
.pills.stacked .assignee-pill {
  box-sizing: border-box;
  width: 100%;
  max-width: 100%;
  justify-content: flex-start;
  gap: 8px;
  min-height: 26px;
  padding: 3px 6px;
  border: 1px solid transparent;
  background: transparent;
  border-radius: 6px;
  overflow: hidden;
}
.pills.stacked .pill:hover,
.pills.stacked .assignee-pill:hover {
  background: var(--t-hover);
}
.pills.stacked .pill .n-icon {
  flex: none;
}
.pills.stacked .pill-text {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.pills.stacked .sf-empty {
  color: var(--t-text3);
}
/* Assignee row: keep the avatar overlap cascade (no flex gap between avatars),
   give the field icon its own spacing, and paint the avatar ring in the row's
   current background so it doesn't flash white on the grey hover. */
.pills.stacked .assignee-pill {
  gap: 0;
}
.pills.stacked .sf-people-ic {
  margin-right: 8px;
  flex: none;
}
.pills.stacked .sf-people-ic + .avatar {
  margin-left: 0;
}
.pills.stacked .assignee-pill:hover .avatar {
  border-color: var(--t-hover);
}
/* Stacked tags row: chips that fit, rest → +N (mirrors the task modal). */
.stag-val {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  flex: 1;
  min-width: 0;
  overflow: hidden;
  position: relative;
}
/* Outlined-oval tag chip: gradient border-box + soft fill (via inline background),
   like the modal. The 1px transparent border is on every chip incl. the invisible
   measurement copies so widths match exactly. */
.mchip {
  font-size: 11px;
  padding: 1px 8px;
  border: 1px solid transparent;
  border-radius: 10px;
  flex: none;
  white-space: nowrap;
}
.mchip.chip-more {
  color: var(--t-text3);
  background: var(--t-surface-alt);
}
/* Invisible natural-width measurement row (never sliced). */
.stag-measure {
  position: absolute;
  left: 0;
  top: 0;
  display: inline-flex;
  gap: 6px;
  visibility: hidden;
  pointer-events: none;
  white-space: nowrap;
}
.pill {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  min-height: 22px;
  padding: 2px 6px;
  border-radius: 6px;
  border: 1px dashed var(--t-border);
  background: transparent;
  color: var(--t-text3);
  cursor: pointer;
}
/* The estimate pill is a <div>, not a <button>: form controls get UA
   `line-height: normal`, but a bare div inherits the card's taller line-height,
   making it ~5px higher than the sibling pills. Pin its box model so it matches. */
.est-pill {
  box-sizing: border-box;
  height: 22px;
  line-height: 1;
}
/* milestone chip: same neutral look as the other set pills (no fill) */
.ms-pill {
  box-sizing: border-box;
  height: 22px;
  line-height: 1;
  max-width: 140px;
  border-style: solid;
  color: var(--t-text2);
}
.ms-pill .pill-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.ms-pill.closed {
  opacity: 0.6;
}
/* conflict warning pill: orange, draws the eye to an unresolved write-back conflict */
.conf-pill {
  box-sizing: border-box;
  height: 22px;
  line-height: 1;
  border-style: solid;
  border-color: color-mix(in srgb, #e0922f 55%, transparent);
  background: color-mix(in srgb, #e0922f 14%, transparent);
  color: #b96a08;
}
.pill.set {
  border-style: solid;
  color: var(--t-text2);
}
/* repeat glyph on a recurring task's due pill — inherits the pill's text
   colour (purple accent clashed inside the neutral pill, worse on dark) */
.pill-recur {
  color: inherit;
}
/* overdue due-date pill: soft red tint (like a warning tag) */
.pill.overdue {
  color: #e0533d;
  border-color: #e0533d;
  border-style: solid;
  background: color-mix(in srgb, #e0533d 12%, transparent);
}
/* Flag (priority) icon carries the active priority's gradient (set --icon-grad
   inline on the flag only; the date pill is also .pill.set but has no
   --icon-grad, so it falls back to currentColor). */
.pill.set :deep(svg [stroke='currentColor']) {
  stroke: var(--icon-grad, currentColor);
}
.pill.set :deep(svg [fill='currentColor']) {
  fill: var(--icon-grad, currentColor);
}
.pill-text {
  font-size: 11px;
}
/* description indicator: icon-only pill; hover opens the rendered-markdown card.
   It's a <div>, so pin the box model like .est-pill to match the sibling pills. */
.desc-pill {
  box-sizing: border-box;
  height: 22px;
  line-height: 1;
  padding: 2px 5px;
}
.desc-pop {
  max-width: 340px;
  max-height: 320px;
  overflow: auto;
  padding: 10px 12px;
  font-size: 13px;
}
.chip {
  font-size: 11px;
  padding: 1px 8px;
  border-radius: 10px;
}
.tag-pill {
  border-style: solid;
  gap: 5px;
}
.tname {
  font-size: 11px;
  max-width: 120px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.more {
  font-size: 10px;
  opacity: 0.8;
}
.preview {
  display: flex;
  flex-wrap: wrap;
  gap: 5px;
  max-width: 220px;
}
.avatar {
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
  /* cascade: overlap the previous avatar with a ring in the card colour */
  margin-left: -9px;
  border: 2px solid var(--t-surface);
}
.avatar:first-child {
  margin-left: 0;
}
.avatar.sm {
  margin-left: 0;
  border: none;
}
.assignee-pill {
  border: none;
  padding: 2px;
}
/* author + assignee group — pushed to the right edge of the row; margin-left
   auto keeps it right-aligned even when it wraps onto its own line. */
.people {
  display: inline-flex;
  align-items: center;
  margin-left: auto;
}
/* the author avatar: read-only, visually muted vs the accent assignees, and it
   leads the stack (so it sits flush-left like any first avatar) */
.author-ava {
  background: var(--t-text3);
  opacity: 0.85;
}
/* Multiline people tooltip (Автор / Исполнитель breakdown). */
.people-tip {
  display: flex;
  flex-direction: column;
  gap: 2px;
  font-size: 12px;
  line-height: 1.35;
}
/* Assignee picker: a pinned search box above a scrollable, capped list. */
.assignee-menu {
  gap: 6px;
}
.assignee-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
  max-height: 240px;
  overflow-y: auto;
}
.assignee-empty {
  padding: 8px 6px;
  text-align: center;
  font-size: 12px;
  color: var(--t-text3);
}
.assignee-sep {
  font-size: 10px;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: var(--t-text3);
  padding: 6px 6px 2px;
}
/* external GitLab assignee (no Tessera account): neutral, slightly muted */
.ext-ava {
  background: var(--t-text3);
  opacity: 0.9;
}
.menu {
  min-width: 180px;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.tagmenu {
  width: 300px;
  max-width: 80vw;
  max-height: 260px;
  overflow-y: auto;
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
.menu-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 5px 6px;
  border-radius: 6px;
  cursor: pointer;
  font-size: 13px;
}
.menu-item:hover {
  background: var(--t-hover);
}
.assignee-item .aname {
  flex: 1;
}
.assignee-item .chk {
  color: var(--t-primary);
}
.dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
}

/* First-level subtasks cascade directly below the parent card (no indent).
   Expanded cards attach with no gap; collapsed text rows get a little spacing. */
.subs.stack {
  display: flex;
  flex-direction: column;
}
.subrow {
  display: flex;
  align-items: center;
  gap: 7px;
  padding: 5px 6px;
  border-radius: 6px;
  font-size: 13px;
  color: var(--t-text2);
  cursor: pointer;
}
.subrow:hover {
  background: var(--t-hover);
}
.subrow.done .sub-title {
  text-decoration: line-through;
  opacity: 0.6;
}
.check.sm {
  display: inline-flex;
  color: var(--t-text3);
}
.subrow.done .check.sm {
  color: var(--t-primary);
}
.pr-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  flex: none;
}
.sub-title {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.sub-due {
  font-size: 11px;
  color: var(--t-text3);
}
.sub-add-input {
  margin-top: 6px;
}
/* "N из M подзадач" hint under a filter-narrowed child list. */
.subs-narrowed {
  margin: 4px 0 0 8px;
  font-size: 11px;
  line-height: 1.3;
  color: var(--t-text3);
  opacity: 0.85;
}
/* Subtask collapse/expand: cross-fade + slight slide when the board toggles
   between the compact rows ("list") and full property cards ("stack"). The
   keyed draggable swaps under <transition mode="out-in">. */
.sub-morph-enter-active,
.sub-morph-leave-active {
  transition:
    opacity 0.16s ease,
    transform 0.16s ease;
}
.sub-morph-enter-from {
  opacity: 0;
  transform: translateY(-4px);
}
.sub-morph-leave-to {
  opacity: 0;
  transform: translateY(4px);
}
@media (prefers-reduced-motion: reduce) {
  .sub-morph-enter-active,
  .sub-morph-leave-active {
    transition: none;
  }
}
</style>
