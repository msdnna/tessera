<script setup>
import { ref, watch, computed, nextTick, onBeforeUnmount } from 'vue'
import { useRouter } from 'vue-router'
import {
  NModal,
  NCard,
  NInput,
  NDatePicker,
  NSwitch,
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
  CheckmarkCircle,
  EllipseOutline,
  ArchiveOutline,
  GitMergeOutline,
  LogoGitlab,
  AttachOutline,
  TrashOutline,
  DownloadOutline,
  CloseOutline,
} from '@vicons/ionicons5'
import {
  tasks as tasksApi,
  boards as boardsApi,
  workspaces as wsApi,
  projects as projApi,
} from '@/api'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useAuthStore } from '@/stores/auth'
import { PRIORITY_LABELS, PRIORITY_COLORS } from '@/styles/tokens'
import { hueGrad, tagPillBg, softFill, readableHue, onColor } from '@/utils/gradient'
import { useThemeStore } from '@/stores/theme'
import { useDateLocale } from '@/composables/useDateLocale'
import MarkdownEditor from './MarkdownEditor.vue'
import RichContent from './RichContent.vue'
import TaskMiniCard from './TaskMiniCard.vue'
import UserAvatar from './UserAvatar.vue'
import TesseraSpinner from './TesseraSpinner.vue'

const props = defineProps({
  show: { type: Boolean, default: false },
  taskId: { type: String, default: null },
  wsId: { type: String, default: null },
  projectId: { type: String, default: null },
  tags: { type: Array, default: () => [] },
  members: { type: Array, default: () => [] },
})
const emit = defineEmits(['update:show', 'changed', 'open'])

const store = useWorkspacesStore()
const theme = useThemeStore()
const { firstDayOfWeek, dateTimeFormat, formatDue } = useDateLocale()
// Tag colour clamped for legible text on the active theme.
const tagText = (c) => readableHue(c, theme.isDark)
const auth = useAuthStore()
const router = useRouter()
const message = useMessage()
const loading = ref(false)
const task = ref(null)
const boardInfo = ref(null) // { name, projectId } for the breadcrumb
const parentCandidates = ref([]) // top-level tasks on the board (for attach)

// ── rich detail (#8): comments, relations, files, journal
// Open an existing description on the Preview tab; an empty one on Write.
const descInitialMode = ref('write')

// Workspace members offered for @-mentions in comments.
const mentionItems = computed(() =>
  (props.members || []).map((m) => ({ id: m.user_id, label: m.name })),
)

// Lookup maps for subtask hover cards (built from the tags/members props).
const tagsById = computed(() => Object.fromEntries((props.tags || []).map((t) => [t.id, t])))
const membersById = computed(() =>
  Object.fromEntries((props.members || []).map((m) => [m.user_id, m])),
)

const comments = ref([])
const newComment = ref('')
const commentEditor = ref(null)
const editingCommentId = ref(null)
const editingCommentBody = ref('')

const relations = ref([])
const relNumber = ref(null)
const relKind = ref('relates')
const relKindOptions = [
  { label: 'связана с', value: 'relates' },
  { label: 'блокирует', value: 'blocks' },
  { label: 'заблокирована', value: 'blocked_by' },
  { label: 'дублирует', value: 'duplicates' },
]
// Cross-board task autocomplete for linking relations.
const relPickerOpen = ref(false)
const relTasks = ref([]) // workspace tasks, lazily loaded
async function ensureRelTasks() {
  if (relTasks.value.length || !props.wsId) return
  try {
    const res = await wsApi.tasks(props.wsId)
    relTasks.value = res.data || []
  } catch {
    /* non-fatal — manual number entry still works */
  }
}
function openRelPicker() {
  relPickerOpen.value = true
  ensureRelTasks()
}
// Filter by typed number/title, drop the current task and numberless ones,
// then group by project → board so long lists stay navigable.
const relGroups = computed(() => {
  const q = String(relNumber.value || '')
    .trim()
    .toLowerCase()
  const out = []
  const index = {}
  for (const t of relTasks.value) {
    if (t.id === props.taskId || t.number == null) continue
    if (q && !(`#${t.number}`.includes(q) || t.title.toLowerCase().includes(q))) continue
    const pk = t.project_name || '—'
    const bk = t.board_name || '—'
    const key = `${pk} / ${bk}`
    if (!index[key]) {
      index[key] = { project: pk, board: bk, tasks: [] }
      out.push(index[key])
    }
    index[key].tasks.push(t)
  }
  return out.slice(0, 50)
})
async function chooseRelTask(t) {
  relNumber.value = t.number
  relPickerOpen.value = false
  await addRelation()
}

const attachments = ref([])
const fileInput = ref(null)
const uploading = ref(false)

const events = ref([])
const meId = computed(() => auth.user?.id)

const title = ref('')
const description = ref('')
const priority = ref(0)
const dueTs = ref(null)
const completed = ref(false)
const selectedTags = ref([])
const selectedAssignees = ref([])
const newSubtask = ref('')
const newTagName = ref('')

const priorityOptions = PRIORITY_LABELS.map((label, value) => ({ label, value }))

const tagObjs = computed(() =>
  selectedTags.value.map((id) => props.tags.find((t) => t.id === id)).filter(Boolean),
)

// The tags trigger shows as many WHOLE tag chips as fit on one line, then a
// "+N". To pick the count without a render⇄measure feedback loop, an invisible
// measurement row (natural widths, never sliced) is measured against the
// trigger's width, reserving room for the +N chip when not everything fits.
const tagsValEl = ref(null)
const tagsMeasureEl = ref(null)
const visibleTagCount = ref(99)
const PLUS_N_W = 46 // reserved px for the "+N" chip (incl. gap)
let tagsRO = null
function fitCount(avail, widths, gap, reserve) {
  let used = 0
  let n = 0
  for (let i = 0; i < widths.length; i++) {
    const add = widths[i] + (i > 0 ? gap : 0)
    if (used + add + reserve > avail) break
    used += add
    n++
  }
  return n
}
function measureTags() {
  const val = tagsValEl.value
  const measure = tagsMeasureEl.value
  if (!val || !measure) return
  const avail = val.clientWidth - 24 // button h-padding
  const widths = [...measure.children].map((ch) => ch.offsetWidth)
  if (!widths.length || avail <= 0) {
    visibleTagCount.value = widths.length
    return
  }
  let n = fitCount(avail, widths, 6, 0)
  if (n < widths.length) n = fitCount(avail, widths, 6, PLUS_N_W) // make room for +N
  visibleTagCount.value = Math.max(n, 1) // always show at least one
}
watch(
  [tagObjs, tagsValEl],
  () => {
    nextTick(measureTags)
    if (tagsValEl.value && typeof ResizeObserver !== 'undefined' && !tagsRO) {
      tagsRO = new ResizeObserver(() => measureTags())
      tagsRO.observe(tagsValEl.value)
    }
  },
  { immediate: true },
)
onBeforeUnmount(() => tagsRO?.disconnect())
const assigneeObjs = computed(() =>
  selectedAssignees.value.map((id) => props.members.find((m) => m.user_id === id)).filter(Boolean),
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
    const m = membersById.value[t.created_by]
    if (m) return { name: m.name, id: t.created_by }
  }
  return null
})
const dueLabel = computed(() => (dueTs.value ? formatDue(new Date(dueTs.value).toISOString()) : ''))

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

function subDue(d) {
  return formatDue(d)
}

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
    completed.value = !!t.completed_at
    selectedTags.value = (t.tags || []).map((x) => x.id)
    selectedAssignees.value = (t.assignees || []).map((x) => x.id)
    try {
      const b = await boardsApi.get(t.board_id)
      boardInfo.value = { name: b.data.name, projectId: b.data.project_id }
      const bt = await boardsApi.tasks(t.board_id)
      parentCandidates.value = (bt.data || []).filter((x) => x.id !== t.id)
    } catch {
      boardInfo.value = null
      parentCandidates.value = []
    }
    loadExtras()
  } catch (e) {
    message.error(e.message)
  } finally {
    loading.value = false
  }
}

// Comments / relations / attachments / journal load in parallel — none of them
// should block the modal opening, so failures are swallowed individually.
async function loadExtras() {
  const id = props.taskId
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
}

watch(
  () => [props.show, props.taskId],
  ([show]) => {
    if (show) loadDetail()
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
    completed: completed.value,
  }
}
// Tappable fields persist immediately; Save commits the text fields.
async function applyMeta() {
  try {
    const res = await tasksApi.update(props.taskId, buildPayload())
    task.value = res.data
    emit('changed')
  } catch (e) {
    message.error(e.message)
  }
}
function setPriority(p) {
  priority.value = p
  applyMeta()
}
function setDue(ts) {
  dueTs.value = ts
  applyMeta()
}
function setCompleted(v) {
  completed.value = v
  applyMeta()
}
async function save() {
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
    const res = await projApi.createTag(props.projectId, {
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

async function addSubtask() {
  const t = newSubtask.value.trim()
  if (!t || !task.value) return
  try {
    await boardsApi.createTask(task.value.board_id, {
      column_id: task.value.column_id,
      parent_id: task.value.id,
      title: t,
    })
    newSubtask.value = ''
    await loadDetail()
    emit('changed')
  } catch (e) {
    message.error(e.message)
  }
}
async function toggleSubtask(sub) {
  try {
    await tasksApi.update(sub.id, {
      title: sub.title,
      description: sub.description || '',
      priority: sub.priority || 0,
      due_date: sub.due_date || null,
      completed: !sub.completed_at,
    })
    await loadDetail()
    emit('changed')
  } catch (e) {
    message.error(e.message)
  }
}

// ── rich description ──
async function saveDesc() {
  await applyMeta()
}

// ── comments ──
function fmtWhen(d) {
  return new Date(d).toLocaleString('ru-RU', {
    day: '2-digit',
    month: 'short',
    hour: '2-digit',
    minute: '2-digit',
  })
}
async function postComment() {
  const body = newComment.value.trim()
  if (!body) return
  const mentions = commentEditor.value?.getMentions?.() || []
  try {
    await tasksApi.addComment(props.taskId, body, mentions)
    newComment.value = ''
    commentEditor.value?.clear?.()
    const c = await tasksApi.comments(props.taskId)
    comments.value = c.data || []
    emit('changed')
  } catch (e) {
    message.error(e.message)
  }
}
function startEditComment(c) {
  editingCommentId.value = c.id
  editingCommentBody.value = c.body
}
async function saveComment() {
  const body = editingCommentBody.value.trim()
  if (!body) return
  try {
    await tasksApi.updateComment(editingCommentId.value, body)
    editingCommentId.value = null
    const c = await tasksApi.comments(props.taskId)
    comments.value = c.data || []
  } catch (e) {
    message.error(e.message)
  }
}
async function deleteComment(id) {
  try {
    await tasksApi.removeComment(id)
    comments.value = comments.value.filter((x) => x.id !== id)
  } catch (e) {
    message.error(e.message)
  }
}

// ── relations (by #N) ──
function relKindLabel(k) {
  return relKindOptions.find((o) => o.value === k)?.label || k
}
async function addRelation() {
  const n = Number(relNumber.value)
  if (!n) return
  try {
    await tasksApi.addRelation(props.taskId, n, relKind.value)
    relNumber.value = null
    relPickerOpen.value = false
    const r = await tasksApi.relations(props.taskId)
    relations.value = r.data || []
    emit('changed')
  } catch (e) {
    message.error(e.message)
  }
}
async function removeRelation(id) {
  try {
    await tasksApi.removeRelation(id)
    relations.value = relations.value.filter((x) => x.id !== id)
  } catch (e) {
    message.error(e.message)
  }
}
function openRelated(rel) {
  if (rel.related_board_id) {
    close()
    router.push(`/board/${rel.related_board_id}?task=${rel.related_task_id}`)
  }
}

// ── attachments ──
function fmtSize(bytes) {
  if (bytes < 1024) return `${bytes} Б`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(0)} КБ`
  return `${(bytes / 1024 / 1024).toFixed(1)} МБ`
}
function pickFile() {
  fileInput.value?.click?.()
}
async function onFileChosen(ev) {
  const file = ev.target.files?.[0]
  if (!file) return
  uploading.value = true
  try {
    const fd = new FormData()
    fd.append('file', file)
    await tasksApi.uploadAttachment(props.taskId, fd)
    const a = await tasksApi.attachments(props.taskId)
    attachments.value = a.data || []
    emit('changed')
  } catch (e) {
    message.error(e.message)
  } finally {
    uploading.value = false
    ev.target.value = ''
  }
}
async function downloadAttachment(att) {
  try {
    const res = await tasksApi.downloadAttachment(att.id)
    const url = URL.createObjectURL(res.data)
    const a = document.createElement('a')
    a.href = url
    a.download = att.filename
    document.body.appendChild(a)
    a.click()
    a.remove()
    URL.revokeObjectURL(url)
  } catch (e) {
    message.error(e.message)
  }
}
async function deleteAttachment(id) {
  try {
    await tasksApi.removeAttachment(id)
    attachments.value = attachments.value.filter((x) => x.id !== id)
  } catch (e) {
    message.error(e.message)
  }
}

// ── journal ──
function eventText(e) {
  const d = e.data || {}
  switch (e.kind) {
    case 'created':
      return 'создал(а) задачу'
    case 'renamed':
      return `переименовал(а) → «${d.to ?? ''}»`
    case 'description':
      return 'изменил(а) описание'
    case 'priority':
      return `изменил(а) приоритет → ${PRIORITY_LABELS[d.to] ?? d.to}`
    case 'due':
      return d.set ? 'установил(а) срок' : 'убрал(а) срок'
    case 'completed':
      return 'отметил(а) выполненной'
    case 'reopened':
      return 'вернул(а) в работу'
    case 'moved':
      return `переместил(а)${d.to ? ` → «${d.to}»` : ''}`
    case 'assigned':
      return 'назначил(а) исполнителя'
    case 'unassigned':
      return 'снял(а) исполнителя'
    case 'archived':
      return 'отправил(а) в архив'
    case 'restored':
      return 'восстановил(а) из архива'
    case 'comment':
      return 'оставил(а) комментарий'
    case 'relation':
      return `добавил(а) связь с #${d.related ?? ''}`
    case 'attachment':
      return `прикрепил(а) файл${d.filename ? ` «${d.filename}»` : ''}`
    default:
      return e.kind
  }
}
</script>

<template>
  <n-modal :show="show" @update:show="emit('update:show', $event)">
    <n-card style="width: 640px; max-width: 94vw" role="dialog" :bordered="false">
      <n-spin :show="loading" :rotate="false">
        <template #icon><TesseraSpinner /></template>
        <div class="form">
          <div class="modal-head">
            <n-popover trigger="click" placement="bottom-start">
              <template #trigger>
                <div v-if="breadcrumb.length" class="crumbs" title="Перенести в другую доску">
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
            <span v-if="task?.number" class="tnum">#{{ task.number }}</span>
          </div>
          <a
            v-if="task?.gitlab"
            class="gl-line"
            :href="task.gitlab.web_url"
            target="_blank"
            rel="noopener"
            :title="`Открыть issue !${task.gitlab.iid} в GitLab`"
          >
            <n-icon :component="LogoGitlab" :size="13" />
            <span>GitLab !{{ task.gitlab.iid }}</span>
          </a>
          <n-input v-model:value="title" placeholder="Название задачи" class="title-input plain" />

          <div class="props">
            <!-- priority -->
            <div class="prow">
              <span class="plabel"><n-icon :component="FlagOutline" :size="15" /> Приоритет</span>
              <n-popover trigger="click" placement="bottom-start">
                <template #trigger>
                  <button class="val">
                    <span class="dot" :style="{ background: hueGrad(PRIORITY_COLORS[priority]) }" />
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
                    <span class="dot" :style="{ background: hueGrad(PRIORITY_COLORS[o.value]) }" />
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
                  <button class="val">{{ dueLabel || 'Не задан' }}</button>
                </template>
                <n-date-picker
                  panel
                  type="datetime"
                  default-time="00:00:00"
                  :value="dueTs"
                  :first-day-of-week="firstDayOfWeek"
                  :format="dateTimeFormat"
                  @update:value="setDue"
                />
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
                      <span
                        v-for="t in tagObjs.slice(0, visibleTagCount)"
                        :key="t.id"
                        class="chip"
                        :style="{
                          border: '1px solid transparent',
                          background: tagPillBg(t.color, true),
                        }"
                      >
                        <span
                          class="accent-grad-text"
                          :style="{ '--grad': hueGrad(tagText(t.color)) }"
                          >{{ t.name }}</span
                        >
                      </span>
                      <span v-if="visibleTagCount < tagObjs.length" class="chip chip-more"
                        >+{{ tagObjs.length - visibleTagCount }}</span
                      >
                      <!-- invisible measurement row: natural chip widths, never sliced -->
                      <span ref="tagsMeasureEl" class="tags-measure" aria-hidden="true">
                        <span v-for="t in tagObjs" :key="`m${t.id}`" class="chip">{{
                          t.name
                        }}</span>
                      </span>
                    </template>
                    <span v-else class="muted">Нет</span>
                  </button>
                </template>
                <div class="menu">
                  <div class="chip-grid">
                    <button
                      v-for="t in tags"
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
                      {{ t.name }}
                    </button>
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

            <!-- completed -->
            <div class="prow">
              <span class="plabel"
                ><n-icon :component="CheckmarkDoneOutline" :size="15" /> Выполнено</span
              >
              <n-switch :value="completed" @update:value="setCompleted" />
            </div>

            <!-- parent -->
            <div class="prow">
              <span class="plabel"
                ><n-icon :component="GitMergeOutline" :size="15" /> Родитель</span
              >
              <n-button v-if="task?.parent_id" quaternary size="small" @click="detachFromParent">
                Открепить
              </n-button>
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
                  <span v-if="!parentCandidates.length" class="muted small">Нет других задач</span>
                </div>
              </n-popover>
            </div>
          </div>

          <div class="section">
            <span class="slabel">Описание</span>
            <MarkdownEditor
              :key="taskId"
              v-model="description"
              placeholder="Добавьте описание…"
              :min-rows="3"
              :initial-mode="descInitialMode"
              @blur="saveDesc"
            />
          </div>

          <!-- Subtasks / comments / relations / files / history (#8) -->
          <!-- Keyed by task so the line indicator doesn't slide when switching
               between a task and its subtask / related task. -->
          <n-tabs :key="taskId" type="line" size="small" class="detail-tabs">
            <n-tab-pane name="comments">
              <template #tab>
                Комментарии
                <n-badge
                  v-if="comments.length"
                  :value="comments.length"
                  :max="99"
                  class="tab-badge"
                />
              </template>
              <div class="comments">
                <div v-for="c in comments" :key="c.id" class="comment">
                  <UserAvatar
                    class="c-ava"
                    :user-id="c.author_id || ''"
                    :src="c.gl_author_avatar_url"
                    :name="c.author_name || c.gl_author_name || '?'"
                  />
                  <div class="c-body">
                    <div class="c-head">
                      <span class="c-author">{{
                        c.author_name || c.gl_author_name || 'Кто-то'
                      }}</span>
                      <span v-if="!c.author_name && c.gl_author_name" class="c-gl">· GitLab</span>
                      <span class="c-when">{{ fmtWhen(c.created_at) }}</span>
                      <span v-if="c.author_id === meId" class="c-acts">
                        <button class="c-act" title="Изменить" @click="startEditComment(c)">
                          ✎
                        </button>
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
                        :mention-items="mentionItems"
                        :min-rows="2"
                        placeholder="Комментарий…"
                        @submit="saveComment"
                      />
                      <n-space :size="6" style="margin-top: 6px">
                        <n-button size="tiny" type="primary" @click="saveComment"
                          >Сохранить</n-button
                        >
                        <n-button size="tiny" @click="editingCommentId = null">Отмена</n-button>
                      </n-space>
                    </template>
                    <RichContent v-else class="c-text" :source="c.body" :members="mentionItems" />
                  </div>
                </div>
                <div v-if="!comments.length" class="empty-hint">Комментариев пока нет</div>
                <div class="comment-add">
                  <MarkdownEditor
                    ref="commentEditor"
                    v-model="newComment"
                    :mention-items="mentionItems"
                    :min-rows="1"
                    placeholder="Написать комментарий… (@ — упоминание, Ctrl+Enter — отправить)"
                    @submit="postComment"
                  />
                  <n-button size="small" type="primary" @click="postComment">Отправить</n-button>
                </div>
              </div>
            </n-tab-pane>

            <n-tab-pane name="subtasks">
              <template #tab>
                Подзадачи
                <n-badge
                  v-if="task?.subtasks?.length"
                  :value="task.subtasks.length"
                  :max="99"
                  class="tab-badge"
                />
              </template>
              <div class="subtasks">
                <n-popover
                  v-for="sub in task?.subtasks || []"
                  :key="sub.id"
                  trigger="hover"
                  placement="right"
                  :delay="250"
                >
                  <template #trigger>
                    <div
                      class="subrow"
                      :class="{ done: sub.completed_at }"
                      @click="emit('open', sub.id)"
                    >
                      <span class="check" @click.stop="toggleSubtask(sub)">
                        <n-icon
                          :component="sub.completed_at ? CheckmarkCircle : EllipseOutline"
                          :size="17"
                        />
                      </span>
                      <span
                        v-if="sub.priority"
                        class="pr-dot"
                        :style="{ background: PRIORITY_COLORS[sub.priority] }"
                      />
                      <span class="sub-title">{{ sub.title }}</span>
                      <span v-if="sub.due_date" class="sub-due">{{ subDue(sub.due_date) }}</span>
                    </div>
                  </template>
                  <TaskMiniCard :task="sub" :tags-map="tagsById" :members-map="membersById" />
                </n-popover>
                <div v-if="!(task?.subtasks || []).length" class="empty-hint">
                  Подзадач пока нет
                </div>
                <n-input
                  v-model:value="newSubtask"
                  size="small"
                  class="plain"
                  placeholder="+ подзадача (Enter)"
                  @keyup.enter="addSubtask"
                />
              </div>
            </n-tab-pane>

            <n-tab-pane name="relations">
              <template #tab>
                Связи
                <n-badge
                  v-if="relations.length"
                  :value="relations.length"
                  :max="99"
                  class="tab-badge"
                />
              </template>
              <div class="relations">
                <div v-for="r in relations" :key="r.id" class="relrow">
                  <span class="rel-kind">{{ relKindLabel(r.kind) }}</span>
                  <button
                    class="rel-link"
                    :class="{ done: r.related_completed_at }"
                    @click="openRelated(r)"
                  >
                    <span class="rel-num">#{{ r.related_number }}</span>
                    <span class="rel-title">{{ r.related_title }}</span>
                  </button>
                  <n-popconfirm
                    :positive-button-props="{ type: 'error' }"
                    positive-text="Удалить"
                    @positive-click="removeRelation(r.id)"
                  >
                    <template #trigger>
                      <button class="c-act" title="Убрать связь">
                        <n-icon :component="CloseOutline" />
                      </button>
                    </template>
                    Убрать связь?
                  </n-popconfirm>
                </div>
                <div v-if="!relations.length" class="empty-hint">Связей пока нет</div>
                <div class="rel-add">
                  <n-select
                    v-model:value="relKind"
                    :options="relKindOptions"
                    size="small"
                    style="width: 150px"
                  />
                  <n-popover
                    trigger="manual"
                    :show="relPickerOpen"
                    placement="bottom-start"
                    :width="320"
                    @clickoutside="relPickerOpen = false"
                  >
                    <template #trigger>
                      <n-input
                        v-model:value="relNumber"
                        size="small"
                        placeholder="№ или название"
                        style="width: 240px"
                        @focus="openRelPicker"
                        @keyup.enter="addRelation"
                      >
                        <template #prefix>#</template>
                      </n-input>
                    </template>
                    <div class="rel-picker">
                      <div v-if="!relGroups.length" class="empty-hint">Ничего не найдено</div>
                      <div v-for="g in relGroups" :key="g.project + '/' + g.board" class="rp-group">
                        <div class="rp-head">{{ g.project }} · {{ g.board }}</div>
                        <button
                          v-for="t in g.tasks"
                          :key="t.id"
                          type="button"
                          class="rp-item"
                          @click="chooseRelTask(t)"
                        >
                          <span class="rp-num">#{{ t.number }}</span>
                          <span class="rp-title">{{ t.title }}</span>
                        </button>
                      </div>
                    </div>
                  </n-popover>
                  <n-button size="small" class="rel-go" @click="addRelation">Связать</n-button>
                </div>
              </div>
            </n-tab-pane>

            <n-tab-pane name="files">
              <template #tab>
                Файлы
                <n-badge
                  v-if="attachments.length"
                  :value="attachments.length"
                  :max="99"
                  class="tab-badge"
                />
              </template>
              <div class="files">
                <div v-for="a in attachments" :key="a.id" class="filerow">
                  <n-icon :component="AttachOutline" class="f-ico" />
                  <button class="f-name" @click="downloadAttachment(a)">{{ a.filename }}</button>
                  <span class="f-size">{{ fmtSize(a.size) }}</span>
                  <button class="c-act" title="Скачать" @click="downloadAttachment(a)">
                    <n-icon :component="DownloadOutline" />
                  </button>
                  <n-popconfirm
                    :positive-button-props="{ type: 'error' }"
                    positive-text="Удалить"
                    @positive-click="deleteAttachment(a.id)"
                  >
                    <template #trigger>
                      <button class="c-act" title="Удалить">
                        <n-icon :component="TrashOutline" />
                      </button>
                    </template>
                    Удалить файл «{{ a.filename }}»?
                  </n-popconfirm>
                </div>
                <div v-if="!attachments.length" class="empty-hint">Файлов пока нет</div>
                <input ref="fileInput" type="file" hidden @change="onFileChosen" />
                <n-button size="small" :loading="uploading" @click="pickFile">
                  <template #icon><n-icon :component="AttachOutline" /></template>
                  Прикрепить файл
                </n-button>
              </div>
            </n-tab-pane>

            <n-tab-pane name="history" tab="История">
              <div class="history">
                <div v-for="e in events" :key="e.id" class="histrow">
                  <UserAvatar class="h-ava" :user-id="e.actor_id" :name="e.actor_name" />
                  <span class="h-text">
                    <b>{{ e.actor_name || 'Кто-то' }}</b> {{ eventText(e) }}
                  </span>
                  <span class="h-when">{{ fmtWhen(e.created_at) }}</span>
                </div>
                <div v-if="!events.length" class="empty-hint">История пуста</div>
              </div>
            </n-tab-pane>
          </n-tabs>
        </div>
      </n-spin>

      <template #footer>
        <div class="footer">
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
.form {
  display: flex;
  flex-direction: column;
  gap: 16px;
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
.tnum {
  font-size: 12px;
  color: var(--t-text3);
  flex: none;
}
/* GitLab provenance line for synced tasks. */
.gl-line {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  margin: 4px 0 2px;
  font-size: 12px;
  color: var(--t-text2);
  text-decoration: none;
  width: fit-content;
}
.gl-line:hover {
  color: var(--t-primary);
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
.subrow {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 8px;
  border-radius: 6px;
  background: var(--t-surface-alt);
  margin-bottom: 4px;
  cursor: pointer;
  font-size: 13px;
}
.subrow:hover {
  background: var(--t-hover);
}
.subrow .check {
  display: inline-flex;
  color: var(--t-text3);
  cursor: pointer;
}
.subrow.done .check {
  color: var(--t-primary);
}
.subrow.done .sub-title {
  text-decoration: line-through;
  opacity: 0.6;
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
.pmenu {
  max-height: 240px;
  overflow-y: auto;
}
.small {
  font-size: 12px;
  padding: 4px;
}
.menu {
  min-width: 200px;
  display: flex;
  flex-direction: column;
  gap: 2px;
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
.grow {
  flex: 1;
}
.chk {
  color: var(--t-primary);
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
.tab-badge {
  margin-left: 6px;
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

/* comments */
.comments {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.comment {
  display: flex;
  gap: 10px;
}
.c-ava,
.h-ava {
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
.c-act {
  border: none;
  background: none;
  cursor: pointer;
  color: var(--t-text3);
  font-size: 12px;
  display: inline-flex;
  align-items: center;
  padding: 2px;
}
.c-act:hover {
  color: var(--t-text1);
}
.c-text {
  font-size: 13px;
}
.comment-add {
  display: flex;
  flex-direction: column;
  gap: 8px;
  align-items: flex-start;
  margin-top: 4px;
}
.comment-add > :first-child {
  width: 100%;
}
.empty-hint {
  font-size: 13px;
  color: var(--t-text3);
  padding: 4px 0 8px;
}

/* relations */
.relations {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.relrow {
  display: flex;
  align-items: center;
  gap: 8px;
}
.rel-kind {
  font-size: 12px;
  color: var(--t-text3);
  width: 96px;
  flex: none;
}
.rel-link {
  display: flex;
  align-items: center;
  gap: 6px;
  flex: 1;
  min-width: 0;
  border: none;
  background: none;
  cursor: pointer;
  text-align: left;
  padding: 4px 6px;
  border-radius: 6px;
  color: var(--t-text1);
  font-size: 13px;
}
.rel-link:hover {
  background: var(--t-hover);
}
.rel-link.done .rel-title {
  text-decoration: line-through;
  opacity: 0.6;
}
.rel-num {
  color: var(--t-text3);
  font-variant-numeric: tabular-nums;
  flex: none;
}
.rel-title {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.rel-add {
  display: flex;
  gap: 8px;
  align-items: center;
  margin-top: 8px;
}
.rel-go {
  margin-left: auto;
}
.rel-picker {
  max-height: 300px;
  overflow-y: auto;
}
.rp-group {
  margin-bottom: 8px;
}
.rp-head {
  font-size: 11px;
  font-weight: 600;
  color: var(--t-text3);
  padding: 2px 4px;
  position: sticky;
  top: 0;
  background: var(--t-surface);
}
.rp-item {
  display: flex;
  align-items: baseline;
  gap: 8px;
  width: 100%;
  text-align: left;
  border: none;
  background: transparent;
  border-radius: 6px;
  padding: 6px 8px;
  cursor: pointer;
}
.rp-item:hover {
  background: var(--t-hover);
}
.rp-num {
  flex: none;
  font-size: 12px;
  color: var(--t-text3);
}
.rp-title {
  color: var(--t-text1);
  font-size: 13px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.subtasks {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

/* files */
.files {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.filerow {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
}
.f-ico {
  color: var(--t-text3);
  flex: none;
}
.f-name {
  flex: 1;
  min-width: 0;
  border: none;
  background: none;
  cursor: pointer;
  text-align: left;
  color: var(--t-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.f-size {
  font-size: 11px;
  color: var(--t-text3);
  flex: none;
}

/* history */
.history {
  display: flex;
  flex-direction: column;
  gap: 8px;
  max-height: 280px;
  overflow-y: auto;
}
.histrow {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
}
.h-ava {
  width: 24px;
  height: 24px;
  font-size: 10px;
}
.h-text {
  flex: 1;
  color: var(--t-text2);
}
.h-text b {
  color: var(--t-text1);
}
.h-when {
  font-size: 11px;
  color: var(--t-text3);
  flex: none;
}
</style>
