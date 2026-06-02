<script setup>
import { ref, watch, computed } from 'vue'
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
  useMessage,
  useDialog,
} from 'naive-ui'
import {
  FlagOutline,
  CalendarClearOutline,
  PeopleOutline,
  PricetagOutline,
  CheckmarkDoneOutline,
  CheckmarkOutline,
  CheckmarkCircle,
  EllipseOutline,
  ArchiveOutline,
  GitMergeOutline,
} from '@vicons/ionicons5'
import {
  tasks as tasksApi,
  boards as boardsApi,
  workspaces as wsApi,
  projects as projApi,
} from '@/api'
import { useWorkspacesStore } from '@/stores/workspaces'
import { PRIORITY_LABELS, PRIORITY_COLORS } from '@/styles/tokens'

const props = defineProps({
  show: { type: Boolean, default: false },
  taskId: { type: String, default: null },
  wsId: { type: String, default: null },
  tags: { type: Array, default: () => [] },
  members: { type: Array, default: () => [] },
})
const emit = defineEmits(['update:show', 'changed', 'open'])

const store = useWorkspacesStore()
const message = useMessage()
const dialog = useDialog()
const loading = ref(false)
const task = ref(null)
const boardInfo = ref(null) // { name, projectId } for the breadcrumb
const parentCandidates = ref([]) // top-level tasks on the board (for attach)

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
const assigneeObjs = computed(() =>
  selectedAssignees.value.map((id) => props.members.find((m) => m.user_id === id)).filter(Boolean),
)
const dueLabel = computed(() =>
  dueTs.value
    ? new Date(dueTs.value).toLocaleDateString('ru-RU', {
        day: '2-digit',
        month: 'short',
        year: 'numeric',
      })
    : '',
)

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

function initials(name) {
  return (name || '?').trim().slice(0, 2).toUpperCase()
}
function subDue(d) {
  return new Date(d).toLocaleDateString('ru-RU', { day: '2-digit', month: 'short' })
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
  } catch (e) {
    message.error(e.message)
  } finally {
    loading.value = false
  }
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
function onArchiveClick() {
  const hasSubs = (task.value?.subtasks?.length || 0) > 0
  dialog.warning({
    title: 'В архив',
    content: hasSubs
      ? 'У задачи есть подзадачи — что с ними сделать?'
      : 'Перенести задачу в архив?',
    positiveText: hasSubs ? 'В архив вместе' : 'В архив',
    negativeText: hasSubs ? 'Открепить подзадачи' : 'Отмена',
    onPositiveClick: () => archiveTask(false),
    onNegativeClick: hasSubs ? () => archiveTask(true) : undefined,
  })
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
    const res = await wsApi.createTag(props.wsId, {
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
</script>

<template>
  <n-modal :show="show" @update:show="emit('update:show', $event)">
    <n-card style="width: 640px; max-width: 94vw" role="dialog" :bordered="false">
      <n-spin :show="loading">
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
          <n-input v-model:value="title" placeholder="Название задачи" class="title-input plain" />

          <div class="props">
            <!-- priority -->
            <div class="prow">
              <span class="plabel"><n-icon :component="FlagOutline" :size="15" /> Приоритет</span>
              <n-popover trigger="click" placement="bottom-start">
                <template #trigger>
                  <button class="val">
                    <span class="dot" :style="{ background: PRIORITY_COLORS[priority] }" />
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
                    <span class="dot" :style="{ background: PRIORITY_COLORS[o.value] }" />
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
                <n-date-picker panel type="date" :value="dueTs" @update:value="setDue" />
              </n-popover>
            </div>

            <!-- assignees -->
            <div class="prow">
              <span class="plabel"
                ><n-icon :component="PeopleOutline" :size="15" /> Исполнители</span
              >
              <n-popover trigger="click" placement="bottom-start">
                <template #trigger>
                  <button class="val">
                    <template v-if="assigneeObjs.length">
                      <span
                        v-for="u in assigneeObjs"
                        :key="u.user_id"
                        class="avatar"
                        :title="u.name"
                        >{{ initials(u.name) }}</span
                      >
                    </template>
                    <span v-else class="muted">Никто</span>
                  </button>
                </template>
                <div class="menu">
                  <div
                    v-for="m in members"
                    :key="m.user_id"
                    class="menu-item"
                    @click="toggleAssignee(m.user_id)"
                  >
                    <span class="avatar sm">{{ initials(m.name) }}</span>
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
                  <button class="val">
                    <template v-if="tagObjs.length">
                      <span
                        v-for="t in tagObjs"
                        :key="t.id"
                        class="chip"
                        :style="{
                          background: (t.color || '#888') + '22',
                          color: t.color || '#888',
                        }"
                        >{{ t.name }}</span
                      >
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
                      :style="
                        selectedTags.includes(t.id)
                          ? {
                              background: t.color || '#888',
                              color: '#fff',
                              borderColor: t.color || '#888',
                            }
                          : { color: t.color || '#888', borderColor: (t.color || '#888') + '88' }
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
            <n-input
              v-model:value="description"
              type="textarea"
              class="plain"
              :autosize="{ minRows: 3, maxRows: 12 }"
              placeholder="Добавьте описание…"
            />
          </div>

          <div class="section">
            <span class="slabel">Подзадачи</span>
            <div
              v-for="sub in task?.subtasks || []"
              :key="sub.id"
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
            <n-input
              v-model:value="newSubtask"
              size="small"
              class="plain"
              placeholder="+ подзадача (Enter)"
              @keyup.enter="addSubtask"
            />
          </div>
        </div>
      </n-spin>

      <template #footer>
        <div class="footer">
          <n-space>
            <n-button type="error" ghost @click="onArchiveClick">
              <template #icon><n-icon :component="ArchiveOutline" /></template>
              В архив
            </n-button>
            <n-button v-if="task?.parent_id" quaternary @click="detachFromParent">
              Открепить от родителя
            </n-button>
          </n-space>
          <n-space>
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
}
.avatar {
  width: 24px;
  height: 24px;
  border-radius: 50%;
  background: var(--t-primary);
  color: var(--t-on-primary);
  font-size: 10px;
  font-weight: 600;
  display: inline-flex;
  align-items: center;
  justify-content: center;
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
.section {
  display: flex;
  flex-direction: column;
  gap: 6px;
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
.footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
</style>
