<script setup>
// The child list under a board card: one always-mounted drop target (shared
// "tasks" drag group), rendered either as a fanned stack of full cards or as
// compact name-only rows.
//
// Extracted from TaskCard (#2665). The expanded stack renders a full TaskCard
// per child, which would make this component and TaskCard import each other —
// so the card passes that markup down through the `expanded` slot instead, and
// the recursion stays where it already was, inside TaskCard.
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import draggable from 'vuedraggable'
import { NIcon, NPopover } from 'naive-ui'
import { CheckmarkCircle, EllipseOutline } from '@vicons/ionicons5'
import { storeToRefs } from 'pinia'
import { tasks as tasksApi } from '@/api'
import { PRIORITY_COLORS } from '@/styles/tokens'
import { hueGradVert } from '@/utils/gradient'
import { divergedColumn } from '@/utils/status'
import { taskBasePatch } from '@/utils/taskPatch'
import TaskMiniCard from '../TaskMiniCard.vue'
import { useBoardViewStore } from '@/stores/boardView'
import { useDateLocale } from '@/composables/useDateLocale'

const props = defineProps({
  task: { type: Object, required: true },
  subtasks: { type: Array, default: () => [] },
  // Render children as full property cards (vs compact name-only rows).
  expanded: { type: Boolean, default: false },
  // Compact card preset: rows lose the checkbox / priority dot / due date.
  compact: { type: Boolean, default: false },
  // A board drag is in progress → reveal the "drop to nest" zone on childless cards.
  dragging: { type: Boolean, default: false },
  // The composer filter hid part of the children: reordering a partial list
  // would write meaningless float8 positions, so child DnD is locked.
  narrowed: { type: Boolean, default: false },
  // Archive view: rows are display-only (no completion toggle).
  readonly: { type: Boolean, default: false },
})
const emit = defineEmits(['open', 'changed', 'ctx'])

const bv = useBoardViewStore()
const { columns } = storeToRefs(bv)
const tagsMap = bv.tagsMap
const membersMap = bv.membersMap
const tagPrefixNames = bv.prefixNames
const { formatDue } = useDateLocale()
const { t } = useI18n()

// Mutable mirror for drag-reorder of subtasks; resynced from the prop.
const subModel = ref([])
watch(
  () => props.subtasks,
  (v) => (subModel.value = [...v]),
  { immediate: true, deep: true },
)
// A card dropped into this list becomes the card's subtask; a subtask dragged
// within the list is just reordered.
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
// Column chip for a child rendered under this card — only when it diverged from
// this card's own column (otherwise it would sit on every single row).
function subColumn(s) {
  return divergedColumn(s, props.task.column_id, columns.value)
}
async function toggleSubDone(s) {
  if (props.readonly) return
  await tasksApi.update(s.id, { ...taskBasePatch(s), completed: !s.completed_at })
  emit('changed')
}
</script>

<template>
  <transition name="sub-morph" mode="out-in">
    <draggable
      :key="expanded ? 'stack' : 'list'"
      :list="subModel"
      group="tasks"
      item-key="id"
      class="subs"
      :class="{
        stack: expanded,
        list: !expanded,
        compact,
        collapsed: !subModel.length && !dragging,
        pending: !subModel.length && dragging,
        ro: readonly,
      }"
      :animation="150"
      :delay="300"
      :touch-start-threshold="6"
      :disabled="narrowed"
      @click.stop
      @change="onSubChange"
    >
      <template #item="{ element: s, index }">
        <!-- The expanded stack is a TaskCard per child — rendered by the card
             itself (see the component comment), which also owns the layer's
             stacking order. -->
        <slot v-if="expanded" name="expanded" :task="s" :index="index" />
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
                @contextmenu.prevent.stop="emit('ctx', $event, s)"
              >
                <span v-if="!compact" class="check sm" @click.stop="toggleSubDone(s)">
                  <n-icon
                    :component="s.completed_at ? CheckmarkCircle : EllipseOutline"
                    :size="15"
                  />
                </span>
                <span
                  v-if="!compact && s.priority"
                  class="pr-dot"
                  :style="{ background: hueGradVert(PRIORITY_COLORS[s.priority]) }"
                />
                <span class="sub-title">{{ s.title }}</span>
                <span v-if="!compact && subDue(s)" class="sub-due">{{ subDue(s) }}</span>
                <!-- this child ran ahead of (or behind) its parent — mark it
                     with the column's own colour. Just the marker: a board
                     column is ~250px wide, so a name here would eat the title;
                     the tooltip and the hover card spell it out. -->
                <span
                  v-if="subColumn(s)"
                  class="col-mark"
                  :style="{ background: subColumn(s).color }"
                  :title="t('task.card.columnIs', { name: subColumn(s).name })"
                />
              </div>
            </template>
            <TaskMiniCard
              :task="s"
              :tags-map="tagsMap"
              :members-map="membersMap"
              :tag-prefix-names="tagPrefixNames"
              :column="subColumn(s)"
            />
          </n-popover>
        </div>
      </template>
    </draggable>
  </transition>
</template>

<style scoped>
/* "Drop to nest" zone — collapsed (and unhittable) until a drag is in progress.
   Empty + idle → fully hidden (overrides the list/stack block styling). Empty
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
/* Collapsed: a single card emerging from under the parent, holding the list.
   `--sub-bg` is defined once on the card wrapper (TaskCard `.tw`) and shared
   with the expanded stack's layers. */
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
/* Archive (read-only) view: the completion toggle is display-only so the click
   falls through to the row and opens the read-only modal. */
.subs.ro .check {
  pointer-events: none;
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
/* Same meaning as the card's .col-chip, name-less: a rounded square so it never
   reads as the round priority dot next to it. */
.col-mark {
  flex: none;
  width: 8px;
  height: 8px;
  border-radius: 3px;
  background: var(--t-text3);
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
