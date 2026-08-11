<script setup>
// «Подзадачи» tab of the task modal: the child list with inline complete/move and
// a one-line composer. Every mutation reloads the parent task, so the tab reports
// `changed` and lets the modal re-fetch rather than patching its own copy.
import { ref, computed } from 'vue'
import { NIcon, NInput, NPopover, useMessage } from 'naive-ui'
import { CheckmarkCircle, EllipseOutline, GitBranchOutline } from '@vicons/ionicons5'
import { tasks as tasksApi, boards as boardsApi } from '@/api'
import { useBoardViewStore } from '@/stores/boardView'
import { useDateLocale } from '@/composables/useDateLocale'
import { PRIORITY_COLORS } from '@/styles/tokens'
import { sortedColumns, columnById, siblingNeighbors } from '@/utils/status'
import TaskMiniCard from '../TaskMiniCard.vue'
import EmptyState from '../EmptyState.vue'

const props = defineProps({
  task: { type: Object, default: null },
  // The columns of the task's own board (which is not always the open board — a
  // deep-link can open a task from elsewhere), so this stays a prop.
  columns: { type: Array, default: () => [] },
  readonly: { type: Boolean, default: false },
})
const emit = defineEmits(['open', 'changed'])

const message = useMessage()
const bv = useBoardViewStore()
const { formatDue } = useDateLocale()
const newSubtask = ref('')

const sortedCols = computed(() => sortedColumns(props.columns))
const columnOf = (t) => columnById(props.columns, t?.column_id)
function subDue(d) {
  return formatDue(d)
}

async function addSubtask() {
  const t = newSubtask.value.trim()
  if (!t || !props.task) return
  try {
    await boardsApi.createTask(props.task.board_id, {
      column_id: props.task.column_id,
      parent_id: props.task.id,
      title: t,
    })
    newSubtask.value = ''
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
      start_date: sub.start_date || null,
      recurrence: sub.recurrence || null,
      completed: !sub.completed_at,
    })
    emit('changed')
  } catch (e) {
    message.error(e.message)
  }
}
// Same move as the status row, for a subtask row, without opening it.
async function moveSubtask(sub, columnId) {
  if (props.readonly || !sub || columnId === sub.column_id) return
  try {
    await tasksApi.move(sub.id, {
      column_id: columnId,
      ...siblingNeighbors(props.task?.subtasks || [], sub.id),
    })
    emit('changed')
  } catch (e) {
    message.error(e.message)
  }
}
</script>

<template>
  <div class="subtasks">
    <n-popover
      v-for="sub in task?.subtasks || []"
      :key="sub.id"
      trigger="hover"
      placement="right"
      :delay="250"
    >
      <template #trigger>
        <div class="subrow" :class="{ done: sub.completed_at }" @click="emit('open', sub.id)">
          <span class="check" @click.stop="toggleSubtask(sub)">
            <n-icon :component="sub.completed_at ? CheckmarkCircle : EllipseOutline" :size="17" />
          </span>
          <span
            v-if="sub.priority"
            class="pr-dot"
            :style="{ background: PRIORITY_COLORS[sub.priority] }"
          />
          <span class="sub-title">{{ sub.title }}</span>
          <span v-if="sub.due_date" class="sub-due">{{ subDue(sub.due_date) }}</span>
          <!-- status of the subtask, changeable without opening it -->
          <n-popover v-if="!readonly && sortedCols.length" trigger="click" placement="bottom-end">
            <template #trigger>
              <span class="col-chip mini" @click.stop>
                <span class="col-dot" :style="{ background: columnOf(sub)?.color }" />
                <span>{{ columnOf(sub)?.name || '—' }}</span>
              </span>
            </template>
            <div class="menu pmenu" @click.stop>
              <div
                v-for="c in sortedCols"
                :key="c.id"
                class="menu-item col-item"
                :class="{ cur: c.id === sub.column_id }"
                @click="moveSubtask(sub, c.id)"
              >
                <span class="col-dot" :style="{ background: c.color }" />
                <span>{{ c.name }}</span>
              </div>
            </div>
          </n-popover>
        </div>
      </template>
      <TaskMiniCard
        :task="sub"
        :tags-map="bv.tagsMap"
        :members-map="bv.membersMap"
        :tag-prefix-names="bv.prefixNames"
        :column="columnOf(sub)"
      />
    </n-popover>
    <EmptyState
      v-if="!(task?.subtasks || []).length"
      size="small"
      :icon="GitBranchOutline"
      text="Подзадач пока нет"
    />
    <n-input
      v-model:value="newSubtask"
      size="small"
      class="plain"
      placeholder="+ подзадача (Enter)"
      @keyup.enter="addSubtask"
    />
  </div>
</template>

<style scoped>
@import './tab-shared.css';

.subtasks {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
/* Borderless inline composer — same treatment as the modal's own plain inputs. */
.plain {
  --n-color: transparent !important;
  --n-color-focus: transparent !important;
}
.plain :deep(.n-input__border),
.plain :deep(.n-input__state-border) {
  display: none !important;
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
</style>
