<script setup>
// A board card: title, meta row, hover actions and the child list. The property
// pills and the child list are their own components (`task/TaskCardPills.vue`,
// `task/TaskCardSubtasks.vue`), and the right-click menu is the shared
// `useTaskMenu` the list/calendar/timeline views already use — what's left here
// is the card frame itself.
import { ref, computed, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon, NTooltip, NInput, NDropdown, NPopconfirm } from 'naive-ui'
import {
  CheckmarkCircle,
  CheckmarkOutline,
  GitBranchOutline,
  LogoGitlab,
  EllipsisHorizontal,
  ArrowUndoOutline,
} from '@vicons/ionicons5'
import { storeToRefs } from 'pinia'
import { tasks as tasksApi, boards as boardsApi } from '@/api'
import { PRIORITY_COLORS } from '@/styles/tokens'
import { hueGradVert } from '@/utils/gradient'
import { divergedColumn } from '@/utils/status'
import { columnName } from '@/utils/defaultNames'
import { cardFieldVisible } from '@/utils/cardFields'
import { taskBasePatch } from '@/utils/taskPatch'
import { normalizeTitle } from '@/utils/title'
import TaskCardPills from './task/TaskCardPills.vue'
import TaskCardSubtasks from './task/TaskCardSubtasks.vue'
import { useBoardViewStore } from '@/stores/boardView'
import { useTaskMenu, menuIcon } from '@/composables/useTaskMenu'

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
  // Column of the parent card, set only on a nested subtask card: a subtask that
  // sits elsewhere than its parent shows that column as a chip.
  parentColumnId: { type: String, default: null },
  // Archive view: card is display-only (no inline edits/DnD/menu); shows a Restore
  // affordance instead. Clicking the card still opens the read-only task modal.
  readonly: { type: Boolean, default: false },
})
const emit = defineEmits(['open', 'changed', 'restore'])

// Everything about the open board — reference data (tags, members, milestones,
// status columns), the tag-prefix names and the customize-view display settings —
// comes from the board store rather than being threaded through as props.
const bv = useBoardViewStore()
const { columns, cardSize } = storeToRefs(bv)
const fieldVis = bv.fieldVis
const { t } = useI18n()

// Size preset × customize toggle, shared with the pill row (utils/cardFields.js)
// so the two can't drift apart. The card asks it only about the meta row.
const show = (k) => cardFieldVisible(cardSize.value, fieldVis, k)
// Card-size composition: compact shows only the title.
const isCompact = computed(() => cardSize.value === 'compact')
// Compact cards still show subtasks, but always collapsed and name-only (no
// checkbox / priority dot / due), so the expanded stack never applies there.
const subsExpanded = computed(() => props.subtasksExpanded && !isCompact.value)
// The composer filter hid some of this card's children (task #2602): the board
// shows only the matching ones, the modal still lists them all.
const subsNarrowed = computed(() => props.subtasksTotal > props.subtasks.length)

const done = computed(() => !!props.task.completed_at)
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
// Same chip the child rows use, for this card when it is itself a nested subtask.
const ownColumnChip = computed(() =>
  props.nested ? divergedColumn(props.task, props.parentColumnId, columns.value) : null,
)

// ── title ──
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
  const n = normalizeTitle(titleEdit.value)
  if (!n || n === props.task.title) return
  await apply({ title: n })
}
// Inline edits send the full-replace body minus the description — see
// utils/taskPatch.js for why omitting it matters.
async function apply(patch) {
  await tasksApi.update(props.task.id, { ...taskBasePatch(props.task), ...patch })
  emit('changed')
}
const toggleDone = () => apply({ completed: !done.value })

// ── right-click context menu (works for the card and for collapsed subtasks) ──
const menu = useTaskMenu({
  onOpen: (id) => emit('open', id),
  onChanged: () => emit('changed'),
  onSelect: (key) => {
    if (key === 'subtask') startAddSub()
  },
  columns,
  // "Create subtask" only makes sense on the card itself, not on a child row.
  // Built per open, not once: the label has to follow a language change.
  extra: (target) =>
    target?.id === props.task.id
      ? [{ label: t('task.menu.addSubtask'), key: 'subtask', icon: menuIcon(GitBranchOutline) }]
      : [],
  dangerDelete: true,
})
function openCtx(e, target) {
  if (props.readonly) return // archive view: no context menu
  menu.open(e, target || props.task)
}

// ── adding a subtask ──
// Triggered from the hover action bar / context menu; the list itself lives in
// TaskCardSubtasks, this is just the inline title input the trigger reveals.
const addingSub = ref(false)
const newSubTitle = ref('')
const subInput = ref(null)
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
      data-testid="task-card"
      :data-task-id="task.id"
      :class="{
        done,
        nested,
        'has-subs': !nested && subtasks.length,
        'has-prio': task.priority,
        'tc-readonly': readonly,
      }"
      :style="cardStyle"
      @click="emit('open', task.id)"
      @contextmenu.prevent.stop="openCtx"
    >
      <!-- Archive view: a single Restore affordance replaces the edit action bar. -->
      <div v-if="readonly && !nested" class="card-actions" @click.stop>
        <button
          class="ca-btn ca-restore"
          :title="t('task.action.restore')"
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
          :title="done ? t('task.menu.uncomplete') : t('task.menu.complete')"
          @click.stop="toggleDone"
        >
          <n-icon :component="done ? CheckmarkCircle : CheckmarkOutline" :size="15" />
        </button>
        <button class="ca-btn ca-sub" :title="t('task.menu.addSubtask')" @click.stop="startAddSub">
          <n-icon :component="GitBranchOutline" :size="15" />
        </button>
        <button class="ca-btn ca-more" :title="t('common.action.more')" @click.stop="openCtx">
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
        v-if="
          (show('number') && task.number) || (show('gitlab') && task.gitlab_iid) || ownColumnChip
        "
        class="card-sub"
      >
        <span v-if="show('number') && task.number" class="tnum">#{{ task.number }}</span>
        <!-- expanded subtask card: its column differs from the parent's -->
        <span
          v-if="ownColumnChip"
          class="col-chip"
          :title="t('task.card.columnIs', { name: columnName(ownColumnChip) })"
        >
          <span class="col-dot" :style="{ background: ownColumnChip.color }" />
          <span class="col-name">{{ columnName(ownColumnChip) }}</span>
        </span>
        <a
          v-if="show('gitlab') && task.gitlab_iid"
          class="gl-chip"
          :href="task.gitlab_url"
          target="_blank"
          rel="noopener noreferrer"
          :title="t('task.card.openIssue', { iid: task.gitlab_iid })"
          @click.stop
        >
          <n-icon :component="LogoGitlab" :size="11" />!{{ task.gitlab_iid }}
        </a>
      </div>

      <TaskCardPills
        :task="task"
        :subtasks="subtasks"
        :readonly="readonly"
        @open="emit('open', $event)"
        @changed="emit('changed')"
      />
    </div>
    <!-- /.card -->

    <!-- Subtasks: one always-mounted drop list (shared "tasks" group), so a task
         can be dropped to nest even on a childless card. -->
    <TaskCardSubtasks
      v-if="!nested"
      :task="task"
      :subtasks="subtasks"
      :expanded="subsExpanded"
      :compact="isCompact"
      :dragging="dragging"
      :narrowed="subsNarrowed"
      :readonly="readonly"
      @open="emit('open', $event)"
      @changed="emit('changed')"
      @ctx="openCtx"
    >
      <!-- The expanded stack is a card per child. It renders here, in the card
           itself, so TaskCard and TaskCardSubtasks don't have to import each
           other; each layer peeks ~8px from under the one above it. -->
      <template #expanded="{ task: s, index }">
        <div class="sub-layer" :style="{ zIndex: 40 - index }">
          <TaskCard
            :task="s"
            :subtasks="[]"
            :nested="true"
            :parent-column-id="task.column_id"
            @open="emit('open', $event)"
            @changed="emit('changed')"
          />
        </div>
      </template>
    </TaskCardSubtasks>

    <!-- The filter hid part of the children: say so, so a short list doesn't read
         as "this parent only has one subtask". -->
    <div v-if="!nested && subsNarrowed" class="subs-narrowed" @click.stop>
      {{ t('task.card.narrowed', { shown: subtasks.length, total: subtasksTotal }) }}
    </div>

    <!-- Adding a subtask is triggered from the hover action bar / context menu;
         this is just the inline title input it reveals. -->
    <div v-if="!nested && addingSub" class="sub-add-input" @click.stop>
      <n-input
        ref="subInput"
        v-model:value="newSubTitle"
        size="tiny"
        :placeholder="t('task.card.subtaskPlaceholder')"
        @keyup.enter="submitAddSub"
        @keyup.esc="addingSub = false"
        @blur="submitAddSub"
      />
    </div>

    <n-dropdown
      trigger="manual"
      placement="bottom-start"
      :show="menu.show.value"
      :x="menu.x.value"
      :y="menu.y.value"
      :options="menu.options.value"
      @select="menu.select"
      @clickoutside="menu.show.value = false"
    />
    <n-popconfirm
      v-model:show="menu.deleteConfirmShow.value"
      :x="menu.x.value"
      :y="menu.y.value"
      :positive-button-props="{ type: 'error' }"
      :positive-text="t('task.confirm.deleteYes')"
      @positive-click="menu.confirmDelete()"
      @clickoutside="menu.deleteConfirmShow.value = false"
    >
      <template #trigger><span /></template>
      {{ t('task.confirm.delete') }}
    </n-popconfirm>
    <n-popconfirm
      v-model:show="menu.archiveConfirmShow.value"
      :x="menu.x.value"
      :y="menu.y.value"
      :positive-text="t('task.confirm.archiveYes')"
      @positive-click="menu.confirmArchive()"
      @clickoutside="menu.archiveConfirmShow.value = false"
    >
      <template #trigger><span /></template>
      {{ t('task.confirm.archive') }}
    </n-popconfirm>
  </div>
</template>

<style scoped>
.tw {
  margin-bottom: 8px;
  /* Background shared by the expanded subtask cards and the collapsed list card
     below (TaskCardSubtasks reads it from here, so there's one definition).
     Tweak here: alternatives — var(--t-hover), var(--t-border) (greyer),
     color-mix(in srgb, var(--t-primary) 8%, var(--t-surface)) (accent). */
  --sub-bg: color-mix(in srgb, var(--t-surface) 70%, var(--t-bg));
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
.title-edit {
  flex: 1;
  width: 100%;
}
.card-top {
  display: flex;
  align-items: flex-start;
  gap: 8px;
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
/* "this subtask is in another column than its parent" chip — flat neutral, the
   colour comes from the column itself (a dot), never the accent gradient. */
.col-chip {
  flex: 0 1 auto;
  min-width: 0;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  max-width: min(140px, 45%);
  font-size: 10px;
  line-height: 1;
  padding: 2px 6px;
  border-radius: 999px;
  color: var(--t-text3);
  border: 1px solid var(--t-border);
  background: var(--t-surface-alt);
}
.col-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.col-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  flex: none;
  background: var(--t-text3);
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
</style>
