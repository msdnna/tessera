<script setup>
import { ref, watch, computed } from 'vue'
import {
  NModal,
  NCard,
  NInput,
  NSelect,
  NDatePicker,
  NSwitch,
  NButton,
  NSpace,
  NText,
  NCheckbox,
  NPopconfirm,
  NSpin,
  useMessage,
} from 'naive-ui'
import { tasks as tasksApi, boards as boardsApi, workspaces as wsApi } from '@/api'
import { PRIORITY_LABELS, PRIORITY_COLORS } from '@/styles/tokens'

const props = defineProps({
  show: { type: Boolean, default: false },
  taskId: { type: String, default: null },
  wsId: { type: String, default: null },
  tags: { type: Array, default: () => [] }, // workspace tags
  members: { type: Array, default: () => [] }, // workspace members
})
const emit = defineEmits(['update:show', 'changed'])

const message = useMessage()
const loading = ref(false)
const task = ref(null)

// editable fields
const title = ref('')
const description = ref('')
const priority = ref(0)
const dueTs = ref(null)
const completed = ref(false)
const selectedTags = ref([])
const selectedAssignees = ref([])
const newSubtask = ref('')

const priorityOptions = PRIORITY_LABELS.map((label, value) => ({ label, value }))
const tagOptions = computed(() => props.tags.map((t) => ({ label: t.name, value: t.id })))
const memberOptions = computed(() =>
  props.members.map((m) => ({ label: m.name, value: m.user_id })),
)

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

// applyMeta persists a tappable field (priority/due/completed) immediately and
// reflects it on the board — without requiring the Save button (which is only
// for the text fields). Sends the full current state.
async function applyMeta() {
  try {
    const res = await tasksApi.update(props.taskId, buildPayload())
    task.value = res.data
    emit('changed')
  } catch (e) {
    message.error(e.message)
  }
}

// Save persists the text fields (title/description) and closes.
async function save() {
  try {
    await tasksApi.update(props.taskId, buildPayload())
    emit('changed')
    close()
  } catch (e) {
    message.error(e.message)
  }
}

async function removeTask() {
  try {
    await tasksApi.remove(props.taskId)
    emit('changed')
    close()
  } catch (e) {
    message.error(e.message)
  }
}

const tagPalette = ['#7c5cff', '#2f80ed', '#0eb0a9', '#18a058', '#f0a020', '#e0533d', '#eb2f96']

// Tags apply immediately. Values that aren't existing tag ids are treated as
// new tag names the user typed (tag mode) — create them on the fly.
async function onTagsChange(next) {
  const prev = selectedTags.value
  try {
    const resolved = []
    for (const v of next) {
      if (props.tags.some((t) => t.id === v)) {
        resolved.push(v)
        continue
      }
      // New tag typed in-place.
      const color = tagPalette[Math.floor(Math.random() * tagPalette.length)]
      const res = await wsApi.createTag(props.wsId, { name: v, color })
      resolved.push(res.data.id)
    }
    selectedTags.value = resolved
    for (const id of resolved.filter((x) => !prev.includes(x)))
      await tasksApi.addTag(props.taskId, id)
    for (const id of prev.filter((x) => !resolved.includes(x)))
      await tasksApi.removeTag(props.taskId, id)
    emit('changed')
  } catch (e) {
    message.error(e.message)
  }
}
async function onAssigneesChange(next) {
  const prev = selectedAssignees.value
  selectedAssignees.value = next
  try {
    for (const id of next.filter((x) => !prev.includes(x)))
      await tasksApi.addAssignee(props.taskId, id)
    for (const id of prev.filter((x) => !next.includes(x)))
      await tasksApi.removeAssignee(props.taskId, id)
    emit('changed')
  } catch (e) {
    message.error(e.message)
  }
}

// ── subtasks ──
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
    <n-card style="width: 560px; max-width: 92vw" role="dialog" :bordered="false">
      <n-spin :show="loading">
        <div class="form">
          <n-input v-model:value="title" placeholder="Название задачи" class="title-input" />

          <div class="row">
            <div class="field">
              <n-text depth="3" class="lbl">Приоритет</n-text>
              <n-select
                :value="priority"
                :options="priorityOptions"
                size="small"
                @update:value="
                  (v) => {
                    priority = v
                    applyMeta()
                  }
                "
              >
                <template #arrow>
                  <span class="pr-dot" :style="{ background: PRIORITY_COLORS[priority] }" />
                </template>
              </n-select>
            </div>
            <div class="field">
              <n-text depth="3" class="lbl">Срок</n-text>
              <n-date-picker
                :value="dueTs"
                type="date"
                clearable
                size="small"
                @update:value="
                  (v) => {
                    dueTs = v
                    applyMeta()
                  }
                "
              />
            </div>
            <div class="field done-field">
              <n-text depth="3" class="lbl">Выполнено</n-text>
              <n-switch
                :value="completed"
                @update:value="
                  (v) => {
                    completed = v
                    applyMeta()
                  }
                "
              />
            </div>
          </div>

          <div class="field">
            <n-text depth="3" class="lbl">Теги</n-text>
            <n-select
              :value="selectedTags"
              :options="tagOptions"
              multiple
              filterable
              tag
              size="small"
              placeholder="Выберите или введите тег"
              @update:value="onTagsChange"
            />
          </div>

          <div class="field">
            <n-text depth="3" class="lbl">Исполнители</n-text>
            <n-select
              :value="selectedAssignees"
              :options="memberOptions"
              multiple
              filterable
              size="small"
              placeholder="Назначьте участников"
              @update:value="onAssigneesChange"
            />
          </div>

          <div class="field">
            <n-text depth="3" class="lbl">Описание</n-text>
            <n-input
              v-model:value="description"
              type="textarea"
              :autosize="{ minRows: 3, maxRows: 10 }"
              placeholder="Добавьте описание…"
            />
          </div>

          <div class="field">
            <n-text depth="3" class="lbl">Подзадачи</n-text>
            <div v-for="sub in task?.subtasks || []" :key="sub.id" class="subtask">
              <n-checkbox :checked="!!sub.completed_at" @update:checked="toggleSubtask(sub)" />
              <span :class="{ done: sub.completed_at }">{{ sub.title }}</span>
            </div>
            <n-input
              v-model:value="newSubtask"
              size="small"
              placeholder="+ подзадача (Enter)"
              @keyup.enter="addSubtask"
            />
          </div>
        </div>
      </n-spin>

      <template #footer>
        <div class="footer">
          <n-popconfirm @positive-click="removeTask">
            <template #trigger>
              <n-button quaternary type="error">Удалить</n-button>
            </template>
            Удалить задачу?
          </n-popconfirm>
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
  gap: 14px;
}
.title-input :deep(input) {
  font-size: 17px;
  font-weight: 600;
}
.row {
  display: flex;
  gap: 12px;
}
.field {
  flex: 1;
  min-width: 0;
}
.done-field {
  flex: 0 0 auto;
}
.lbl {
  display: block;
  font-size: 12px;
  margin-bottom: 4px;
}
.pr-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  display: inline-block;
}
.subtask {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 3px 0;
}
.subtask .done {
  text-decoration: line-through;
  opacity: 0.6;
}
.footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
</style>
