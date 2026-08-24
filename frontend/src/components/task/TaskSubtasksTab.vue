<script setup>
// «Подзадачи» tab of the task modal: the child list with inline complete/move and
// a one-line composer. Every mutation reloads the parent task, so the tab reports
// `changed` and lets the modal re-fetch rather than patching its own copy.
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon, NInput, NPopover, useMessage } from 'naive-ui'
import { CheckmarkCircle, EllipseOutline, GitBranchOutline } from '@vicons/ionicons5'
import { tasks as tasksApi, boards as boardsApi, gitlab as glApi } from '@/api'
import { useBoardViewStore } from '@/stores/boardView'
import { useDateLocale } from '@/composables/useDateLocale'
import { PRIORITY_COLORS } from '@/styles/tokens'
import { sortedColumns, columnById, siblingNeighbors } from '@/utils/status'
import { columnName } from '@/utils/defaultNames'
import TaskMiniCard from '../TaskMiniCard.vue'
import EmptyState from '../EmptyState.vue'

const props = defineProps({
  task: { type: Object, default: null },
  // The columns of the task's own board (which is not always the open board — a
  // deep-link can open a task from elsewhere), so this stays a prop.
  columns: { type: Array, default: () => [] },
  readonly: { type: Boolean, default: false },
  // writeback.push_children on the board's integration (#2592) — gates the whole
  // GitLab column of this list, including the "parent is not grouped" hint.
  gitlabCanGroup: { type: Boolean, default: false },
})
const emit = defineEmits(['open', 'changed', 'group-parent'])

const message = useMessage()
const { t } = useI18n()
const bv = useBoardViewStore()
const { formatDue } = useDateLocale()
const newSubtask = ref('')

const sortedCols = computed(() => sortedColumns(props.columns))
const columnOf = (sub) => columnById(props.columns, sub?.column_id)
function subDue(d) {
  return formatDue(d)
}

async function addSubtask() {
  const title = newSubtask.value.trim()
  if (!title || !props.task) return
  try {
    await boardsApi.createTask(props.task.board_id, {
      column_id: props.task.column_id,
      parent_id: props.task.id,
      title,
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
// ── GitLab hierarchy state of each subtask (#2592) ──
// The three states the row can be in, in the order they are worth telling apart:
//   'child'    — has its own issue AND a parent work item in GitLab: nothing to do.
//   'detached' — has an issue, but GitLab refused the hierarchy (empty parent gid).
//                The subtask is NOT lost; it is simply top-level over there → retry.
//   'absent'   — no issue at all: created before the parent was grouped, or the push
//                is still queued. Offered as a manual push.
// The whole column is hidden unless the parent itself is a linked, grouped issue —
// otherwise nothing here can succeed, and the hint below says why instead.
const parentLinked = computed(() => !!props.task?.gitlab)
const parentGrouped = computed(() => props.task?.gitlab?.is_group === true)
const showGitlab = computed(() => props.gitlabCanGroup && parentLinked.value)

function glState(sub) {
  if (!sub?.gl_iid) return 'absent'
  return sub.gl_parent_global_id ? 'child' : 'detached'
}
// Rows in flight, so a double click cannot queue the same push twice.
const pushing = ref(new Set())
async function pushChild(sub) {
  if (props.readonly || !sub || pushing.value.has(sub.id)) return
  pushing.value = new Set(pushing.value).add(sub.id)
  try {
    await glApi.pushChild(sub.id)
    // 202: the worker owns the GitLab round trip. Say "queued" rather than "done" —
    // the row updates when the write-back lands and the parent is re-fetched.
    message.success(t('task.subtasks.pushQueued'))
  } catch (e) {
    message.error(e?.response?.data?.error || e.message)
  } finally {
    const next = new Set(pushing.value)
    next.delete(sub.id)
    pushing.value = next
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
          <!-- GitLab hierarchy state (#2592): a plain chip when the subtask really is a
               child work item over there, an actionable one in the two states that are
               not yet that. -->
          <template v-if="showGitlab">
            <a
              v-if="glState(sub) === 'child'"
              class="gl-chip"
              :href="sub.gl_web_url || undefined"
              target="_blank"
              rel="noopener"
              :title="t('task.subtasks.glChild')"
              @click.stop
              >!{{ sub.gl_iid }}</a
            >
            <button
              v-else-if="!readonly && parentGrouped"
              class="gl-chip act"
              :disabled="pushing.has(sub.id)"
              :title="
                glState(sub) === 'detached'
                  ? t('task.subtasks.glDetachedHint', { iid: sub.gl_iid })
                  : t('task.subtasks.glAbsentHint')
              "
              @click.stop="pushChild(sub)"
            >
              {{
                glState(sub) === 'detached'
                  ? t('task.subtasks.glDetached')
                  : t('task.subtasks.glAbsent')
              }}
            </button>
          </template>
          <!-- status of the subtask, changeable without opening it -->
          <n-popover v-if="!readonly && sortedCols.length" trigger="click" placement="bottom-end">
            <template #trigger>
              <span class="col-chip mini" @click.stop>
                <span class="col-dot" :style="{ background: columnOf(sub)?.color }" />
                <span>{{ columnName(columnOf(sub)) || '—' }}</span>
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
                <span>{{ columnName(c) }}</span>
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
      :text="t('task.subtasks.empty')"
    />
    <!-- The one case the per-row chips cannot express: the parent is linked but not a
         grouped issue, so no subtask here can reach the GitLab hierarchy at all. Said
         once, with the fix next to it, instead of repeated on every row. -->
    <div
      v-if="showGitlab && !parentGrouped && !readonly && (task?.subtasks || []).length"
      class="gl-hint"
    >
      <span>{{ t('task.subtasks.parentNotGrouped') }}</span>
      <button class="gl-hint-act" @click="emit('group-parent')">
        {{ t('task.subtasks.markGrouped') }}
      </button>
    </div>
    <n-input
      v-model:value="newSubtask"
      size="small"
      class="plain"
      :placeholder="t('task.subtasks.addPlaceholder')"
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
/* GitLab hierarchy chips (#2592) — neutral, like the column chip next to them: this
   is provenance, not an accent action. */
.gl-chip {
  flex: none;
  padding: 1px 6px;
  border: 1px solid var(--t-border);
  border-radius: 999px;
  background: transparent;
  color: var(--t-text3);
  font-size: 11px;
  line-height: 16px;
  white-space: nowrap;
  text-decoration: none;
}
.gl-chip.act {
  cursor: pointer;
}
.gl-chip.act:hover {
  color: var(--t-text1);
  border-color: var(--t-text3);
}
.gl-chip.act:disabled {
  cursor: default;
  opacity: 0.5;
}
.gl-hint {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 8px;
  color: var(--t-text3);
  font-size: 11px;
  line-height: 1.4;
}
.gl-hint-act {
  flex: none;
  padding: 1px 7px;
  border: 1px solid var(--t-border);
  border-radius: 6px;
  background: transparent;
  color: var(--t-text2);
  font-size: 11px;
  cursor: pointer;
}
.gl-hint-act:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
</style>
