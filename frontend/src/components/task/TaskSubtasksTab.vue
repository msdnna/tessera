<script setup>
// «Подзадачи» section of the task modal (v2, #2958): a header with the progress
// and «+ Подзадача ⇧N», then the tray of children with inline complete/move and a
// one-line composer that opens on demand. Every mutation reloads the parent task,
// so the section reports `changed` and lets the modal re-fetch rather than
// patching its own copy.
import { ref, computed, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon, NInput, NPopover, useMessage } from 'naive-ui'
import {
  AddOutline,
  CheckmarkCircleOutline,
  EllipseOutline,
  PersonAddOutline,
} from '@vicons/ionicons5'
import { tasks as tasksApi, boards as boardsApi, gitlab as glApi } from '@/api'
import { useBoardViewStore } from '@/stores/boardView'
import { useDateLocale } from '@/composables/useDateLocale'
import { PRIORITY_COLORS } from '@/styles/tokens'
import { sortedColumns, columnById, siblingNeighbors } from '@/utils/status'
import { columnName } from '@/utils/defaultNames'
import { columnStatusName } from '@/utils/columnStatus'
import { hueGradVert } from '@/utils/gradient'
import { normalizeTitle } from '@/utils/title'
import { useResponsive } from '@/composables/useResponsive'
import PopMenu from '../PopMenu.vue'
import TaskMiniCard from '../TaskMiniCard.vue'
import TesseraIcon from '../TesseraIcon.vue'
import UserAvatar from '../UserAvatar.vue'
import ShortcutKeys from '../ShortcutKeys.vue'

const props = defineProps({
  task: { type: Object, default: null },
  // The columns of the task's own board (which is not always the open board — a
  // deep-link can open a task from elsewhere), so this stays a prop.
  columns: { type: Array, default: () => [] },
  readonly: { type: Boolean, default: false },
  // writeback.push_children on the board's integration (#2592) — gates the whole
  // GitLab column of this list, including the "parent is not grouped" hint.
  gitlabCanGroup: { type: Boolean, default: false },
  // boards.done_column_id of the task's board — picks the «done» status glyph.
  doneColumnId: { type: String, default: null },
})
const emit = defineEmits(['open', 'changed', 'group-parent'])

const message = useMessage()
const { t } = useI18n()
const bv = useBoardViewStore()
const { formatDue } = useDateLocale()
// Same hover preview as on the board card, same reason to switch it off on a
// phone (#2893) — see the note in TaskCardSubtasks.vue. Here the row lives
// inside the modal, so the popover overhangs the modal instead of the board, but
// the tap that opens the child task still leaves a preview stuck on screen.
const { isMobile } = useResponsive()
const newSubtask = ref('')
const adding = ref(false)
const addInput = ref(null)

const subs = computed(() => props.task?.subtasks || [])
const doneCount = computed(() => subs.value.filter((s) => s.completed_at).length)
const pct = computed(() => (subs.value.length ? (doneCount.value / subs.value.length) * 100 : 0))

const sortedCols = computed(() => sortedColumns(props.columns))
const columnOf = (sub) => columnById(props.columns, sub?.column_id)
function subDue(d) {
  return formatDue(d)
}
// The status glyph a column gets on the board header — the same four shapes.
function statusIcon(col) {
  if (!col) return 'status-todo'
  const done = props.doneColumnId
    ? col.id === props.doneColumnId
    : col.id === sortedCols.value[sortedCols.value.length - 1]?.id
  return columnStatusName({
    isDone: done,
    first: col.id === sortedCols.value[0]?.id,
    name: col.name,
    nameKey: col.name_key,
  })
}
const assigneeOf = (sub) => {
  const id = (sub.assignee_ids || [])[0]
  return id ? bv.membersMap[id] || null : null
}

// «+ Подзадача» (and ⇧N, through the modal) opens the one-line composer.
async function startAdd() {
  if (props.readonly) return
  adding.value = true
  await nextTick()
  addInput.value?.focus?.()
}
function onAddBlur() {
  if (!newSubtask.value.trim()) adding.value = false
}
defineExpose({ startAdd })

async function addSubtask() {
  const title = normalizeTitle(newSubtask.value)
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
    message.error(e.message)
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
  <section class="subs">
    <div class="subs-head">
      <span class="subs-title">{{ t('task.subtasks.title') }}</span>
      <template v-if="subs.length">
        <span class="subs-count">{{
          t('task.subtasks.progress', { done: doneCount, total: subs.length })
        }}</span>
        <span class="subs-bar"><span :style="{ width: pct + '%' }" /></span>
      </template>
      <span class="grow" />
      <button v-if="!readonly" type="button" class="subs-add" @click="startAdd">
        <n-icon :component="AddOutline" :size="15" />
        {{ t('task.subtasks.add') }}
        <ShortcutKeys id="task.subtask" />
      </button>
    </div>
    <div v-if="subs.length || adding" class="subs-tray">
      <n-popover
        v-for="sub in subs"
        :key="sub.id"
        trigger="hover"
        placement="right"
        :delay="250"
        :disabled="isMobile"
      >
        <template #trigger>
          <div class="subrow" :class="{ done: sub.completed_at }" @click="emit('open', sub.id)">
            <span class="check" @click.stop="toggleSubtask(sub)">
              <n-icon
                :component="sub.completed_at ? CheckmarkCircleOutline : EllipseOutline"
                :size="17"
              />
            </span>
            <span
              class="pr-bar"
              :style="
                sub.priority ? { background: hueGradVert(PRIORITY_COLORS[sub.priority]) } : {}
              "
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
                <!-- A button, not a span: it opens a menu, so it has to be
                     reachable by Tab and openable by Enter (#2932). -->
                <button type="button" class="col-chip mini" @click.stop>
                  <TesseraIcon
                    :name="statusIcon(columnOf(sub))"
                    :size="13"
                    :style="{ color: columnOf(sub)?.color || 'var(--t-text3)' }"
                  />
                  <span>{{ columnName(columnOf(sub)) || '—' }}</span>
                </button>
              </template>
              <PopMenu class="pmenu" @click.stop>
                <div
                  v-for="c in sortedCols"
                  :key="c.id"
                  class="menu-item col-item"
                  :class="{ cur: c.id === sub.column_id }"
                  @click="moveSubtask(sub, c.id)"
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
            <span v-else class="col-chip mini static">
              <TesseraIcon
                :name="statusIcon(columnOf(sub))"
                :size="13"
                :style="{ color: columnOf(sub)?.color || 'var(--t-text3)' }"
              />
              <span>{{ columnName(columnOf(sub)) || '—' }}</span>
            </span>
            <UserAvatar
              v-if="assigneeOf(sub)"
              class="sub-ava"
              :user-id="assigneeOf(sub).user_id"
              :name="assigneeOf(sub).name"
              :title="assigneeOf(sub).name"
            />
            <n-icon
              v-else
              :component="PersonAddOutline"
              :size="14"
              class="sub-noava"
              :title="t('task.subtasks.unassigned')"
            />
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
      <div v-if="adding" class="subrow subrow-new">
        <n-icon :component="EllipseOutline" :size="17" class="check" />
        <n-input
          ref="addInput"
          v-model:value="newSubtask"
          size="small"
          class="plain"
          :placeholder="t('task.subtasks.addPlaceholder')"
          @keyup.enter="addSubtask"
          @keydown.esc.stop="adding = false"
          @blur="onAddBlur"
        />
      </div>
    </div>
    <!-- The one case the per-row chips cannot express: the parent is linked but not a
         grouped issue, so no subtask here can reach the GitLab hierarchy at all. Said
         once, with the fix next to it, instead of repeated on every row. -->
    <div v-if="showGitlab && !parentGrouped && !readonly && subs.length" class="gl-hint">
      <span>{{ t('task.subtasks.parentNotGrouped') }}</span>
      <button class="gl-hint-act" @click="emit('group-parent')">
        {{ t('task.subtasks.markGrouped') }}
      </button>
    </div>
  </section>
</template>

<style scoped>
@import './tab-shared.css';

.subs {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.subs-head {
  display: flex;
  align-items: center;
  gap: 10px;
  min-height: 28px;
}
.subs-title {
  font-size: 14.5px;
  font-weight: 650;
  color: var(--t-text1);
}
.subs-count {
  font-size: 13px;
  color: var(--t-text3);
  white-space: nowrap;
}
.subs-bar {
  width: 120px;
  height: 4px;
  border-radius: 2px;
  background: var(--t-pill);
  overflow: hidden;
}
.subs-bar > span {
  display: block;
  height: 100%;
  border-radius: 2px;
  background: var(--t-accent-grad);
  transition: width 0.25s ease;
}
.grow {
  flex: 1;
}
.subs-add {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 28px;
  padding: 0 6px 0 8px;
  border: none;
  border-radius: 8px;
  background: transparent;
  color: var(--t-text2);
  font: inherit;
  font-size: 13px;
  cursor: pointer;
}
.subs-add:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
/* v2: a tray one level below the modal instead of bordered rows. */
.subs-tray {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 4px;
  border-radius: 12px;
  background: color-mix(in srgb, var(--t-sub) 65%, var(--t-pop));
}
/* Borderless inline composer — same treatment as the modal's own plain inputs. */
.plain {
  flex: 1;
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
  gap: 10px;
  min-height: 38px;
  padding: 0 10px;
  border-radius: 9px;
  cursor: pointer;
  font-size: 13.5px;
  color: var(--t-text1);
}
.subrow:hover {
  background: var(--t-hover);
}
.subrow-new {
  cursor: text;
}
.subrow-new:hover {
  background: transparent;
}
.check {
  flex: none;
  display: inline-flex;
  color: var(--t-text3);
  cursor: pointer;
}
.subrow:not(.subrow-new) .check:hover {
  color: #18a058;
}
.subrow.done .check {
  color: #18a058;
}
.subrow.done .sub-title {
  text-decoration: line-through;
  color: var(--t-text3);
}
.pr-bar {
  flex: none;
  width: 3px;
  height: 16px;
  border-radius: 2px;
}
.sub-title {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.sub-due {
  flex: none;
  font-size: 12px;
  color: var(--t-text3);
}
/* Status: the column glyph in its colour + the name, flat — no chip box. */
.col-chip.mini {
  flex: none;
  font: inherit;
  color: inherit;
  cursor: pointer;
  max-width: 140px;
  gap: 5px;
  border: none;
  background: transparent;
  padding: 2px 6px;
  border-radius: 7px;
  font-size: 12px;
}
.col-chip.mini.static {
  cursor: default;
}
.sub-ava {
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
.sub-noava {
  flex: none;
  width: 20px;
  color: var(--t-text3);
}
/* GitLab hierarchy chips (#2592) — neutral, like the status next to them: this is
   provenance, not an accent action. */
.gl-chip {
  flex: none;
  padding: 1px 7px;
  border: none;
  border-radius: 999px;
  background: var(--t-pill);
  color: var(--t-text3);
  font-size: 11px;
  line-height: 18px;
  white-space: nowrap;
  text-decoration: none;
}
.gl-chip.act {
  cursor: pointer;
}
.gl-chip.act:hover {
  color: var(--t-text1);
}
.gl-chip.act:disabled {
  cursor: default;
  opacity: 0.5;
}
.gl-hint {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 8px;
  color: var(--t-text3);
  font-size: 12px;
  line-height: 1.4;
}
.gl-hint-act {
  flex: none;
  padding: 2px 8px;
  border: none;
  border-radius: 7px;
  background: var(--t-pill);
  color: var(--t-text2);
  font-size: 12px;
  cursor: pointer;
}
.gl-hint-act:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
</style>
