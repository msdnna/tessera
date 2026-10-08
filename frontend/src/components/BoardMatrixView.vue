<script setup>
// The Eisenhower matrix (v2, #2973; mockups MatrixView, PackageD frame 7).
//
// Four quadrants tinted like columns (radius 18, no lines), compact cards two to
// a row by default, the axis captions saying the thresholds («Срочно · срок ≤ 3
// дней», «Важно · приоритет ≥ высокий» — the view panel's «Матрица», #3018).
// A card dragged into another quadrant is pinned there and wears «вручную»;
// the mark (or ⌫ on the selected card) gives it back to the automatic place.
//
// Keyboard (desktop): the arrows walk the cards and cross into the neighbouring
// quadrant at an edge, ⌘ + arrow pins the card into the quadrant that way, ⌫
// unpins, ↵ opens, E / P / D / A / L / S as on the board. The selection shows
// only once the keyboard is in use.
import { ref, computed, watch, nextTick, onBeforeUnmount, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import draggable from 'vuedraggable'
import { NIcon, NInput, useMessage } from 'naive-ui'
import { AddOutline, CloseOutline, FlameOutline, PinOutline } from '@vicons/ionicons5'
import { storeToRefs } from 'pinia'
import TaskCard from './TaskCard.vue'
import { tasks as tasksApi } from '@/api'
import { normalizeTitle } from '@/utils/title'
import { isDueWithinDays } from '@/utils/taskFilter'
import { priorityLabel } from '@/utils/priority'
import { hueGrad } from '@/utils/gradient'
import { matrixMove, neighbourQuad } from '@/utils/matrixNav'
import { useFormat } from '@/composables/useFormat'
import { useResponsive } from '@/composables/useResponsive'
import { useShortcuts } from '@/composables/useShortcuts'
import { useFillHeight } from '@/composables/useFillHeight'
import { useBoardViewStore } from '@/stores/boardView'

const props = defineProps({
  // Filtered top-level board tasks (same array the kanban groups).
  tasks: { type: Array, default: () => [] },
  subtasksByParent: { type: Object, default: () => ({}) },
  // Unfiltered child lists — only to tell a card how many subtasks the filter hid.
  subtasksTotalByParent: { type: Object, default: () => ({}) },
  subtasksExpanded: { type: Boolean, default: false },
  // A task is "urgent" when its due date falls within this many days (or is
  // overdue), "important" from this priority up. Both are the board's view
  // settings (#3018, panel «Матрица»); the defaults are the mockup's ≤ 3 days
  // and ≥ high.
  urgentWindowDays: { type: Number, default: 3 },
  importantPriority: { type: Number, default: 3 },
  // The archive view (?archived=1) shows archived cards and promises to be
  // read-only. The kanban keeps that promise everywhere (column/card draggables,
  // «+ задача», TaskCard, TaskModal); the matrix is the same board in another
  // layout, so it has to keep it too — otherwise the archive still offers the
  // quadrant «＋» (which creates a LIVE task in the board's first column, gone
  // from view the moment it lands), the quadrant drag and the live card menu.
  readonly: { type: Boolean, default: false },
  // The board's task actions behind the keys (KanbanBoard): complete, priority,
  // field.
  actions: { type: Object, default: () => ({}) },
})
const emit = defineEmits(['open', 'changed', 'create', 'restore', 'drag-state'])
const message = useMessage()
const { t } = useI18n()
// The urgency axis is a calendar-day window in the user's timezone, so it needs
// the same formatter set the due pill on the card is printed with.
const { formatters } = useFormat()

// ── quadrant model ────────────────────────────────────────────
// Index encoding mirrors the backend (migration 0028):
//   0 = urgent + important      2 = urgent + not-important
//   1 = not-urgent + important   3 = not-urgent + not-important
// Laid out as a 2×2 grid: columns = urgent | not urgent, rows = important | not.
// Index and class stay constant; the title/hint are looked up per render so the
// grid follows a language change.
const QUADRANT_KEYS = [
  { i: 0, key: 'do', cls: 'q-do', color: '#e0533d' },
  { i: 1, key: 'plan', cls: 'q-plan', color: '#2f80ed' },
  { i: 2, key: 'delegate', cls: 'q-delegate', color: '#f0a020' },
  { i: 3, key: 'drop', cls: 'q-drop', color: '#9aa0aa' },
]
const quadrants = computed(() =>
  QUADRANT_KEYS.map((q) => ({
    ...q,
    title: t(`board.matrix.quadrant.${q.key}Title`),
    hint: t(`board.matrix.quadrant.${q.key}Hint`),
  })),
)

// Derived quadrant when there's no manual override: importance from priority
// (high/urgent), urgency from due-date proximity.
//
// Proximity is counted in whole calendar days of the user's timezone, not in
// milliseconds — see isDueWithinDays(), the same day count the «Срок» facet uses.
function derive(t) {
  const important = (t.priority || 0) >= props.importantPriority
  const urgent = isDueWithinDays(t, props.urgentWindowDays, new Date(), formatters.value)
  if (important) return urgent ? 0 : 1
  return urgent ? 2 : 3
}
function quadrantOf(t) {
  return t.eisenhower_quadrant != null ? t.eisenhower_quadrant : derive(t)
}
function isPinned(t) {
  return t.eisenhower_quadrant != null
}

// Local per-quadrant lists vuedraggable can mutate; rebuilt whenever the source
// task list changes (a reload after any drag reconciles them).
const buckets = ref([[], [], [], []])
function rebuild() {
  const b = [[], [], [], []]
  // Open tasks only — completed cards belong to «done», not the triage matrix.
  for (const t of props.tasks) {
    if (t.completed_at) continue
    b[quadrantOf(t)].push(t)
  }
  buckets.value = b
}
// Also on the formatter set: the urgency window is read in the user's timezone,
// so changing that preference re-sorts the grid instead of leaving it on the
// calendar it was first built in.
watch(
  [() => props.tasks, formatters, () => props.urgentWindowDays, () => props.importantPriority],
  rebuild,
  { immediate: true },
)

const dragging = ref(false)

// A drag has to hold the whole BOARD still, not just this grid (#2932).
//
// SortableJS owns the DOM of the list a card is being dragged through, so the
// board mutes realtime-driven reloads for the duration of a gesture — that is
// what `dragging` in useBoardDragScroll is for. Only the kanban's own draggables
// raise it, though, and the flag above is local: a colleague's event arriving
// mid-gesture took the ordinary route, the board re-fetched its tasks, and
// `rebuild()` replaced all four buckets with fresh arrays underneath Sortable.
// The card flickers back, and worse, the drop then reads its index off a list Vue
// has just rewritten — `onQuadChange` can pin someone else's task to the
// quadrant. So the grid reports its gesture upwards, where the board's existing
// drag guard (and its missed-event backlog) already know what to do with it.
function setDragging(active) {
  dragging.value = active
  emit('drag-state', active)
}
// Switching layout mid-gesture unmounts the grid, and Sortable's `end` never
// arrives — without this the board would stay muted for the rest of the session.
onBeforeUnmount(() => {
  if (dragging.value) setDragging(false)
})

// A card dropped into a quadrant pins it there (manual override). Same-quadrant
// reorders (evt.moved) aren't persisted — the matrix has no meaningful order.
async function onQuadChange(evt, qi) {
  const info = evt.added
  if (!info) return
  try {
    await tasksApi.eisenhower(info.element.id, qi)
    emit('changed')
  } catch (e) {
    message.error(e.message)
    emit('changed')
  }
}

async function resetAuto(t) {
  try {
    await tasksApi.eisenhower(t.id, null)
    emit('changed')
  } catch (e) {
    message.error(e.message)
  }
}

// ── per-quadrant quick add («＋») ──────────────────
const addingIn = ref(-1)
const newTitle = ref('')
const addInput = ref(null)
// A function ref — a static `ref=` inside the v-for would collect an array, so
// `.focus()` would be called on the array, not the input.
const setAddInput = (el) => {
  addInput.value = el
}
function startAdd(qi) {
  if (props.readonly) return
  addingIn.value = qi
  newTitle.value = ''
  // Focus once the input mounts (n-input `autofocus` doesn't fire on a v-if mount).
  nextTick(() => addInput.value?.focus())
}
function submitAdd(qi) {
  const title = normalizeTitle(newTitle.value)
  addingIn.value = -1
  if (!title) return
  // The parent creates the task in the board's first column, then pins the
  // quadrant — the matrix doesn't own a column to drop into.
  emit('create', { title, quadrant: qi })
}

// ── the axis captions: the thresholds in words ──
const urgentCaption = computed(() =>
  t('board.matrix.urgentAxis', {
    days: t('board.customize.matrix.dueDays', props.urgentWindowDays, {
      named: { n: props.urgentWindowDays },
    }),
  }),
)
const importantCaption = computed(() =>
  t('board.matrix.importantAxis', {
    priority: priorityLabel(props.importantPriority).toLowerCase(),
  }),
)

// ── height: the grid fills the window down to its bottom ──
const root = ref(null)
const { height: fillH } = useFillHeight(root, { bottom: 16, min: 360 })

// ── keyboard ──
const { isMobile } = useResponsive()
const { cardSize } = storeToRefs(useBoardViewStore())
// Compact cards go two to a row (the mockup), bigger ones one.
const cols = computed(() => (cardSize.value === 'compact' ? 2 : 1))
const cursor = ref(null) // { q, id }
const kbd = ref(false)
function onAnyPointer() {
  kbd.value = false
}
onMounted(() => window.addEventListener('pointerdown', onAnyPointer, true))
onBeforeUnmount(() => window.removeEventListener('pointerdown', onAnyPointer, true))
const quadIds = computed(() => buckets.value.map((b) => b.map((x) => x.id)))
const cursorTask = computed(() => {
  const c = cursor.value
  return (c && (buckets.value[c.q] || []).find((x) => x.id === c.id)) || null
})
watch(buckets, () => {
  // A card that moved quadrant keeps the selection; one that left drops it.
  const c = cursor.value
  if (!c) return
  const q = quadIds.value.findIndex((ids) => ids.includes(c.id))
  cursor.value = q < 0 ? null : { q, id: c.id }
})
const isPicked = (q, id) =>
  kbd.value && !isMobile.value && cursor.value?.q === q && cursor.value?.id === id
function reveal() {
  nextTick(() => {
    const id = cursor.value?.id
    root.value
      ?.querySelector(`.m-card-wrap[data-task-id="${CSS.escape(id || '')}"]`)
      ?.scrollIntoView({ block: 'nearest' })
  })
}
function step(dir) {
  if (!kbd.value && cursorTask.value) {
    kbd.value = true
    reveal()
    return
  }
  kbd.value = true
  cursor.value = matrixMove(quadIds.value, cols.value, cursor.value, dir)
  reveal()
}
function onCardOpen(q, id) {
  cursor.value = { q, id }
  emit('open', id)
}
// ⌘ + arrow: pin the selected card into the quadrant that way.
async function pinToward(dir) {
  const task = cursorTask.value
  const q = task && neighbourQuad(cursor.value.q, dir)
  if (q == null) return
  try {
    await tasksApi.eisenhower(task.id, q)
    cursor.value = { q, id: task.id }
    emit('changed')
  } catch (e) {
    message.error(e.message)
  }
}
const run = (name, ...args) => props.actions?.[name]?.(...args)
const on = () => !isMobile.value && !props.readonly
useShortcuts(
  {
    'card.up': () => step('up'),
    'card.down': () => step('down'),
    'card.left': () => step('left'),
    'card.right': () => step('right'),
  },
  { when: () => !isMobile.value },
)
useShortcuts(
  {
    'card.open': () => emit('open', cursor.value.id),
    'card.clear': () => (cursor.value = null),
  },
  { when: () => !isMobile.value && kbd.value && !!cursorTask.value },
)
useShortcuts(
  {
    'card.complete': () => run('complete', [cursorTask.value]),
    'card.priority': (v) => run('priority', [cursorTask.value], v),
    'card.priorityPick': () => run('field', cursorTask.value, 'priority'),
    'card.due': () => run('field', cursorTask.value, 'due'),
    'card.assignees': () => run('field', cursorTask.value, 'assignees'),
    'card.tags': () => run('field', cursorTask.value, 'tags'),
    'card.status': () => run('field', cursorTask.value, 'status'),
    'card.moveLeft': () => pinToward('left'),
    'card.moveRight': () => pinToward('right'),
    'matrix.pinUp': () => pinToward('up'),
    'matrix.pinDown': () => pinToward('down'),
    'matrix.auto': () => isPinned(cursorTask.value) && resetAuto(cursorTask.value),
  },
  { when: () => on() && kbd.value && !!cursorTask.value },
)
useShortcuts(
  {
    'board.addTask': () => startAdd(cursor.value?.q ?? 0),
  },
  { when: on },
)
defineExpose({ startAdd })
</script>

<template>
  <div
    ref="root"
    class="matrix"
    :class="{ mobile: isMobile }"
    :style="fillH && !isMobile ? { height: `${fillH}px` } : null"
    data-testid="matrix-view"
  >
    <div class="m-corner" />
    <div class="m-colhead m-col-urgent">
      <n-icon :component="FlameOutline" :size="13" />{{ urgentCaption }}
    </div>
    <div class="m-colhead">{{ t('board.matrix.notUrgent') }}</div>

    <div class="m-rowhead m-row-important">
      <span>{{ importantCaption }}</span>
    </div>
    <div class="m-rowhead m-row-unimportant">
      <span>{{ t('board.matrix.notImportant') }}</span>
    </div>

    <div
      v-for="q in quadrants"
      :key="q.i"
      class="m-quad"
      :class="[q.cls, `m-cell-${q.i}`]"
      :style="{ '--q': q.color }"
      data-testid="matrix-quadrant"
    >
      <div class="m-quad-head">
        <span class="m-dot" :style="{ background: hueGrad(q.color) }" />
        <span class="m-quad-title">{{ q.title }}</span>
        <span class="m-quad-hint">{{ q.hint }}</span>
        <span class="m-space" />
        <span class="m-count">{{ buckets[q.i].length }}</span>
        <button
          v-if="!readonly"
          type="button"
          class="m-add"
          :title="t('board.matrix.addTask')"
          @click="startAdd(q.i)"
        >
          <n-icon :component="AddOutline" :size="14" />
        </button>
      </div>

      <draggable
        :list="buckets[q.i]"
        group="eisenhower"
        item-key="id"
        class="m-list"
        :class="{ 'fade-bottom': addingIn === q.i, two: cols === 2 }"
        :disabled="readonly"
        :animation="150"
        :delay="120"
        :delay-on-touch-only="true"
        :touch-start-threshold="6"
        @start="setDragging(true)"
        @end="setDragging(false)"
        @change="onQuadChange($event, q.i)"
      >
        <template #item="{ element }">
          <div class="m-card-wrap" :data-task-id="element.id">
            <TaskCard
              :task="element"
              :subtasks="subtasksByParent[element.id] || []"
              :subtasks-total="(subtasksTotalByParent[element.id] || []).length"
              :subtasks-expanded="subtasksExpanded"
              :dragging="dragging"
              :selected="isPicked(q.i, element.id)"
              :readonly="readonly"
              @open="onCardOpen(q.i, $event)"
              @changed="$emit('changed')"
              @restore="$emit('restore', $event)"
            />
            <button
              v-if="isPinned(element) && !readonly"
              type="button"
              class="m-pin"
              :title="t('board.matrix.pinned')"
              data-testid="matrix-pin"
              @click.stop="resetAuto(element)"
            >
              <n-icon class="m-pin-ic" :component="PinOutline" :size="11" />
              <span>{{ t('board.matrix.pinnedShort') }}</span>
              <n-icon class="m-pin-x" :component="CloseOutline" :size="11" />
            </button>
          </div>
        </template>
      </draggable>

      <div v-if="addingIn === q.i" class="m-add-input">
        <n-input
          :ref="setAddInput"
          v-model:value="newTitle"
          type="textarea"
          size="small"
          :autosize="{ minRows: 1, maxRows: 4 }"
          :placeholder="t('board.column.taskPlaceholder')"
          @keydown.enter.exact.prevent="submitAdd(q.i)"
          @keyup.esc="addingIn = -1"
          @blur="submitAdd(q.i)"
        />
      </div>
    </div>
  </div>
</template>

<style scoped>
.matrix {
  display: grid;
  grid-template-columns: 28px minmax(0, 1fr) minmax(0, 1fr);
  grid-template-rows: 24px minmax(0, 1fr) minmax(0, 1fr);
  gap: 10px 12px;
  height: calc(100vh - 156px);
  height: calc(100dvh - 156px);
  min-height: 360px;
}
.m-corner {
  grid-row: 1;
  grid-column: 1;
}
/* The axis captions: small caps, the thresholds in words. */
.m-colhead,
.m-rowhead span {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  color: var(--t-text3);
  white-space: nowrap;
}
.m-rowhead {
  grid-column: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 0;
  overflow: hidden;
}
.m-rowhead span {
  writing-mode: vertical-rl;
  transform: rotate(180deg);
}
.m-row-important {
  grid-row: 2;
}
.m-row-unimportant {
  grid-row: 3;
}

/* Quadrant cells — explicit placement so headers/labels line up. */
.m-cell-0 {
  grid-row: 2;
  grid-column: 2;
}
.m-cell-1 {
  grid-row: 2;
  grid-column: 3;
}
.m-cell-2 {
  grid-row: 3;
  grid-column: 2;
}
.m-cell-3 {
  grid-row: 3;
  grid-column: 3;
}

/* A tray in the quadrant's tone over the column surface — no frame. */
.m-quad {
  display: flex;
  flex-direction: column;
  gap: 8px;
  min-height: 0;
  padding: 8px;
  border-radius: 18px;
  background: color-mix(in srgb, var(--q) var(--t-col-tone), var(--t-col));
  overflow: hidden;
}
.m-quad-head {
  display: flex;
  align-items: center;
  gap: 8px;
  height: 30px;
  padding: 0 4px 0 6px;
  flex: none;
}
.m-dot {
  flex: none;
  width: 8px;
  height: 8px;
  border-radius: 3px;
}
.m-quad-title {
  font-size: 13.5px;
  font-weight: 650;
  color: var(--t-text1);
  white-space: nowrap;
}
.m-quad-hint {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 12.5px;
  color: var(--t-text3);
}
.m-space {
  flex: 1;
}
.m-count {
  font-size: 12.5px;
  color: var(--t-text3);
}
.m-add {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  padding: 0;
  border: none;
  border-radius: 7px;
  background: transparent;
  color: var(--t-text3);
  cursor: pointer;
  opacity: 0;
}
.m-quad:hover .m-add,
.m-add:focus-visible {
  opacity: 1;
}
.m-add:hover {
  color: var(--t-text1);
  background: var(--t-hover);
}
@media (hover: none) {
  .m-add {
    opacity: 1;
  }
}
.m-list {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 2px;
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 8px;
  align-content: start;
  scrollbar-width: thin;
}
.m-list.two {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}
/* When the quick-add input is open below, fade the last cards into it (no hard
   overlap edge) — the card softly dissolves as it nears the input. */
.m-list.fade-bottom {
  -webkit-mask-image: linear-gradient(to bottom, #000 calc(100% - 44px), transparent);
  mask-image: linear-gradient(to bottom, #000 calc(100% - 44px), transparent);
}
.m-card-wrap {
  position: relative;
  min-width: 0;
}
.m-card-wrap:hover {
  z-index: 2;
}
/* «вручную»: a small pill on the card's top edge; a click gives the card back
   to its automatic quadrant. */
.m-pin {
  position: absolute;
  top: -6px;
  right: 10px;
  z-index: 3;
  display: inline-flex;
  align-items: center;
  gap: 3px;
  height: 18px;
  padding: 0 6px;
  border: none;
  border-radius: var(--t-r-pill);
  background: var(--t-pop);
  box-shadow: var(--t-sh-card);
  color: var(--t-text2);
  font: inherit;
  font-size: 10.5px;
  font-weight: 600;
  line-height: 1;
  cursor: pointer;
}
.m-pin-ic {
  color: var(--q);
}
.m-pin-x {
  display: none;
}
.m-pin:hover {
  color: var(--t-text1);
}
.m-pin:hover .m-pin-ic {
  display: none;
}
.m-pin:hover .m-pin-x {
  display: inline-flex;
}
.m-add-input {
  flex: none;
}
.m-add-input :deep(.n-input) {
  background-color: var(--t-card);
  border-radius: 9px;
}

/* ── phone: one column of quadrants, each as tall as its cards ── */
.matrix.mobile {
  grid-template-columns: minmax(0, 1fr);
  grid-template-rows: none;
  height: auto;
  min-height: 0;
}
.matrix.mobile .m-corner,
.matrix.mobile .m-colhead,
.matrix.mobile .m-rowhead {
  display: none;
}
.matrix.mobile .m-quad {
  grid-row: auto;
  grid-column: auto;
  overflow: visible;
}
.matrix.mobile .m-list {
  overflow: visible;
}
</style>
