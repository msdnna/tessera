<script setup>
// A board card: title, meta row, hover actions and the child list. The property
// pills and the child list are their own components (`task/TaskCardPills.vue`,
// `task/TaskCardSubtasks.vue`), and the row menu (right-click, «⋯») is the
// shared `useTaskMenu` the list/calendar/timeline views use too, drawn by
// `task/TaskActionMenu` (#3112) — what's left here is the card frame itself.
import { ref, computed, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon, NTooltip } from 'naive-ui'
import { useMessage } from '@/composables/useMessage'
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
import TaskActionMenu from './task/TaskActionMenu.vue'
import InlineEdit from './kit/InlineEdit.vue'
import QuickAddField from './kit/QuickAddField.vue'
import { useBoardViewStore } from '@/stores/boardView'
import { useTaskMenu, subtaskRow } from '@/composables/useTaskMenu'
import { keysText } from '@/shortcuts/labels'

const props = defineProps({
  task: { type: Object, required: true },
  subtasks: { type: Array, default: () => [] },
  // How many subtasks the task really has. When the composer filter narrowed the
  // list (`subtasks` is shorter), the card says "N из M" and locks child DnD —
  // reordering a partial list would write meaningless float8 positions.
  subtasksTotal: { type: Number, default: 0 },
  // Render subtasks as full property cards (vs compact name-only rows).
  subtasksExpanded: { type: Boolean, default: false },
  // The board's composer holds a manual sort, so `subtasks` arrives in the SORT's
  // order and not in the stored one — the child rows may be re-ordered on screen
  // while `position` says something else. Locks child re-ordering for the same
  // reason `subtasksTotal` does: the neighbours a drop names would be the wrong
  // siblings. Drops from elsewhere (nesting) stay on.
  sorted: { type: Boolean, default: false },
  // This card is itself a first-level subtask shown below its parent: darker
  // shade, no nested-subtask cascade, no "create subtask" button.
  nested: { type: Boolean, default: false },
  // A board drag is in progress → reveal the "drop to nest" zone on childless cards.
  dragging: { type: Boolean, default: false },
  // Picked with the arrow keys on the board (v2, #2956).
  selected: { type: Boolean, default: false },
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
const message = useMessage()

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
// v2 (#2954): priority is a 3px vertical-gradient bar inset at the card's left
// edge. A finished task drops it — its priority no longer asks for attention.
const prioBar = computed(() =>
  props.task.priority && !done.value ? hueGradVert(PRIORITY_COLORS[props.task.priority]) : '',
)
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
// Esc abandons the edit — the rule every other inline field on the board already
// follows (the column header's rename, «＋ задача», «＋ колонка», the card's own
// «＋ подзадача», the matrix quick-add). This one had no handler at all, so Esc
// did nothing visible: the field stayed open with the edited text, and the click
// away that a reader makes next went out through `@blur` and PUT the rename they
// had just cancelled. A task title is workspace-shared data, broadcast to every
// other open client — not local view state a stray write merely annoys.
function cancelTitleEdit() {
  editingTitle.value = false
  titleEdit.value = ''
}
async function commitTitle() {
  // Closing the field is what marks the gesture finished: Esc unmounts the input
  // and a browser fires no blur for a node that is simply removed, but the guard
  // is on the commit rather than on that behaviour — it also makes the blur that
  // follows an Enter (or a second commit from any other path) a no-op instead of
  // a duplicate PUT.
  if (!editingTitle.value) return
  editingTitle.value = false
  const n = normalizeTitle(titleEdit.value)
  if (!n || n === props.task.title) return
  if (!(await apply({ title: n }))) restoreFailedTitle(n)
}
// A refused rename must not take the typed title with it. The editor is closed
// before the PUT goes out (see commitTitle), so a failure otherwise leaves the
// old title on the card and nothing to retry from — the same loss KanbanBoard's
// restoreFailedTitle exists to prevent on the create side, and retyping is the
// reader's work the error has no claim on.
//
// Only into an editor the reader hasn't moved on from, though: while the PUT was
// on the wire they may have reopened it and typed the next name (their text
// wins). A closed editor is reopened on the title that failed.
function restoreFailedTitle(title) {
  if (editingTitle.value) return
  titleEdit.value = title
  editingTitle.value = true
  nextTick(() => titleInput.value?.focus?.())
}
// Inline edits send the full-replace body minus the description — see
// utils/taskPatch.js for why omitting it matters.
//
// A refusal is reported and the board re-read: the card draws every field off
// its prop, so without the resync it keeps showing state the server never
// accepted (and misses what a racing second client DID change). Same contract
// the right-click menu over this card already follows — useTaskMenu wraps each
// of these writes and calls onChanged() on both paths.
//
// @returns {boolean} whether the write went through
async function apply(patch) {
  try {
    await tasksApi.update(props.task.id, { ...taskBasePatch(props.task), ...patch })
  } catch (e) {
    message.error(e.message)
    emit('changed')
    return false
  }
  emit('changed')
  return true
}
const toggleDone = () => apply({ completed: !done.value })

// ── the row menu: right-click (the card and its collapsed subtasks) or «⋯» ──
const menu = useTaskMenu({
  onOpen: (id) => emit('open', id),
  onChanged: () => emit('changed'),
  onSelect: (key) => {
    if (key === 'subtask') startAddSub()
  },
  columns,
  // "Create subtask" only makes sense on the card itself, not on a child row.
  // Built per open, not once: the label has to follow a language change.
  extra: (target) => (target?.id === props.task.id ? [subtaskRow()] : []),
})
function openCtx(e, target, el = null) {
  if (props.readonly) return // archive view: no context menu
  menu.open(e, target || props.task, el)
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
        'tc-readonly': readonly,
        selected,
        [`size-${cardSize}`]: true,
      }"
      @click="emit('open', task.id)"
      @contextmenu.prevent.stop="openCtx"
    >
      <span v-if="prioBar" class="prio-bar" :style="{ background: prioBar }" />
      <!-- Archive view: a single Restore affordance replaces the edit action bar. -->
      <div v-if="readonly && !nested && task.archived_at" class="card-actions" @click.stop>
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
          :title="`${done ? t('task.menu.uncomplete') : t('task.menu.complete')} · ${keysText('card.complete')}`"
          @click.stop="toggleDone"
        >
          <n-icon :component="done ? CheckmarkCircle : CheckmarkOutline" :size="15" />
        </button>
        <button
          class="ca-btn ca-sub"
          :title="`${t('task.menu.addSubtask')} · ${keysText('card.subtask')}`"
          @click.stop="startAddSub"
        >
          <n-icon :component="GitBranchOutline" :size="15" />
        </button>
        <button
          class="ca-btn ca-more"
          :title="t('common.action.more')"
          @click.stop="(e) => openCtx(e, null, e.currentTarget)"
        >
          <n-icon :component="EllipsisHorizontal" :size="16" />
        </button>
      </div>

      <!-- No on-card checkbox: completion lives in the hover bar / context menu
           (the check icon left of the title was redundant). -->
      <div class="card-top">
        <InlineEdit
          v-if="editingTitle"
          ref="titleInput"
          v-model="titleEdit"
          class="title-edit"
          :hint-left="-2"
          @click.stop
          @commit="commitTitle"
          @cancel="cancelTitleEdit"
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
          :style="{ '--chip-c': ownColumnChip.color || undefined }"
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
          <n-icon :component="LogoGitlab" :size="12" />!{{ task.gitlab_iid }}
        </a>
      </div>

      <TaskCardPills
        :task="task"
        :subtasks="subtasks"
        :readonly="readonly"
        @open="emit('open', $event)"
        @changed="emit('changed')"
      />

      <!-- v2 (#2954): the compact subtask list is a tray INSIDE the card. Its rows
           open their own task, so clicks stop here instead of opening the parent. -->
      <div v-if="!nested && !subsExpanded" class="tray-slot" @click.stop>
        <TaskCardSubtasks
          :task="task"
          :subtasks="subtasks"
          :expanded="false"
          :compact="isCompact"
          :dragging="dragging"
          :narrowed="subsNarrowed"
          :readonly="readonly"
          @open="emit('open', $event)"
          @changed="emit('changed')"
          @ctx="openCtx"
        />
      </div>
    </div>
    <!-- /.card -->

    <!-- Subtasks: one always-mounted drop list (shared "tasks" group), so a task
         can be dropped to nest even on a childless card. The expanded stack of
         full child cards fans out below the card. -->
    <TaskCardSubtasks
      v-if="!nested && subsExpanded"
      :task="task"
      :subtasks="subtasks"
      :expanded="subsExpanded"
      :compact="isCompact"
      :dragging="dragging"
      :narrowed="subsNarrowed"
      :sorted="sorted"
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
      <QuickAddField
        ref="subInput"
        v-model="newSubTitle"
        size="small"
        :max-rows="2"
        :placeholder="t('task.card.subtaskPlaceholder')"
        @submit="submitAddSub"
        @cancel="addingSub = false"
        @blur="submitAddSub"
      />
    </div>

    <TaskActionMenu :menu="menu" />
  </div>
</template>

<style scoped>
.tw {
  margin-bottom: 8px;
  /* Background shared by the expanded subtask cards and the collapsed list card
     below (TaskCardSubtasks reads it from here, so there's one definition):
     the v2 subtask tray level. */
  --sub-bg: var(--t-sub);
}
/* v2 card (#2954): one step above the lane with a soft shadow — no border. The
   padding follows the card size, plus 5px on the left for the priority bar. */
.card {
  /* Pills and the avatar ring read --card-fill. :hover paints the background
     directly and declares no custom property: Claude Design's token check flags
     any property re-declared under a state selector (#2966). */
  --card-fill: var(--t-card);
  --cpx: 12px;
  position: relative;
  background: var(--card-fill);
  border-radius: var(--t-r-card);
  padding: 10px var(--cpx);
  padding-left: calc(var(--cpx) + 5px);
  box-shadow: var(--t-sh-card);
  cursor: pointer;
  transition:
    background 0.12s ease,
    box-shadow 0.12s ease;
}
.card.size-compact {
  padding-top: 8px;
  padding-bottom: 8px;
}
.card.size-large {
  --cpx: 14px;
  padding-top: 13px;
  padding-bottom: 13px;
}
.card:hover {
  background: var(--t-card-hover);
  box-shadow: var(--t-sh-hover);
}
/* The avatar cascade's ring matches the hovered fill (it reads --card-fill,
   which stays at rest so the property is never re-declared under :hover). */
.card:not(.selected):not(.nested):hover :deep(.avatar) {
  box-shadow: 0 0 0 2px var(--t-card-hover);
}
/* Keyboard selection (v2, #2956): an accent wash and a ring 2px off the edge.
   The background is restated so the wash outranks .card:hover. */
.card.selected {
  --card-fill: color-mix(in srgb, var(--t-primary) 9%, var(--t-card));
  background: var(--card-fill);
}
.card.selected::after {
  content: '';
  position: absolute;
  /* inside the card's edge: a ring outside it was cut off by the column's
     scroller (top and left of the first card, the left of the rest) */
  inset: 0;
  border-radius: var(--t-r-card);
  box-shadow: inset 0 0 0 2px var(--t-primary);
  pointer-events: none;
}
.prio-bar {
  position: absolute;
  left: 6px;
  top: 10px;
  bottom: 10px;
  width: 3px;
  border-radius: 3px;
  pointer-events: none;
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
  padding: 3px;
  border-radius: var(--t-r-tip);
  background: var(--t-pop);
  box-shadow: var(--t-sh-pop);
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
  width: 26px;
  height: 26px;
  border: none;
  background: transparent;
  color: var(--t-text2);
  border-radius: 7px;
  cursor: pointer;
}
.ca-btn:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
/* Touch devices have no hover — keep complete + more persistently reachable
   (there's no on-card checkbox anymore); add-subtask stays in the "⋯" menu. */
@media (hover: none) {
  .card-actions {
    top: 2px;
    right: 2px;
    opacity: 1;
    transform: none;
    pointer-events: auto;
    background: transparent;
    box-shadow: none;
  }
  /* On a phone (#2976) only «⋯» stays on the card — completing is its first
     item and the task sheet's button; two buttons ate the title's first line.
     The title keeps the corner free for it. */
  .ca-sub,
  .ca-complete {
    display: none;
  }
  .ca-btn {
    position: relative;
    width: 32px;
    height: 32px;
    color: var(--t-text3);
  }
  .ca-btn::after {
    content: '';
    position: absolute;
    inset: -6px;
  }
  .card-top {
    padding-right: 26px;
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
  background: var(--card-fill);
  border-radius: 0 0 var(--t-r-card) var(--t-r-card);
  padding-top: 16px;
  box-shadow: var(--t-sh-card);
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
  color: var(--t-text3);
}
/* Title takes the full row and clamps to two lines with an ellipsis; the number
   and GitLab chip live on the meta row below, so the title never wraps around
   them (which used to squeeze it to one word per line). */
.title {
  flex: 1;
  min-width: 0;
  font-size: 14px;
  line-height: 20px;
  font-weight: 500;
  color: var(--t-text1);
  cursor: pointer;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
  overflow-wrap: anywhere;
}
/* meta row: number + chips, directly under the title. */
.card-sub {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  margin-top: 4px;
  font-size: 12px;
  line-height: 18px;
  color: var(--t-text3);
  font-variant-numeric: tabular-nums;
}
.tnum {
  flex: none;
}
/* "synced from GitLab" chip — links to the source issue. Neutral pill. */
.gl-chip {
  flex: none;
  display: inline-flex;
  align-items: center;
  gap: 3px;
  height: 18px;
  padding: 0 6px;
  border-radius: var(--t-r-pill);
  text-decoration: none;
  color: var(--t-text2);
  background: var(--t-pill);
}
.gl-chip:hover {
  color: var(--t-text1);
}
/* "this subtask is in another column than its parent" chip — a soft wash of the
   column's own colour, never the accent. */
.col-chip {
  flex: 0 1 auto;
  min-width: 0;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  max-width: min(140px, 45%);
  height: 18px;
  padding: 0 7px 0 6px;
  border-radius: var(--t-r-pill);
  color: var(--t-text2);
  font-weight: 500;
  background: linear-gradient(
    to top right,
    color-mix(in srgb, var(--chip-c, var(--t-text3)) 11%, var(--card-fill)),
    color-mix(in srgb, var(--chip-c, var(--t-text3)) 19%, var(--card-fill))
  );
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
