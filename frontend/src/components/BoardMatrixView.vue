<script setup>
import { ref, watch } from 'vue'
import draggable from 'vuedraggable'
import { NIcon, NInput, useMessage } from 'naive-ui'
import { CloseCircleOutline } from '@vicons/ionicons5'
import TaskCard from './TaskCard.vue'
import { tasks as tasksApi } from '@/api'

const props = defineProps({
  // Filtered top-level board tasks (same array the kanban groups).
  tasks: { type: Array, default: () => [] },
  subtasksByParent: { type: Object, default: () => ({}) },
  // Real board status columns — passed straight through to TaskCard.
  columns: { type: Array, default: () => [] },
  tagsMap: { type: Object, default: () => ({}) },
  membersMap: { type: Object, default: () => ({}) },
  tags: { type: Array, default: () => [] },
  tagPrefixNames: { type: Object, default: () => ({}) },
  members: { type: Array, default: () => [] },
  wsId: { type: String, default: '' },
  projectId: { type: String, default: '' },
  subtasksExpanded: { type: Boolean, default: false },
  // A task is "urgent" when its due date falls within this many days (or is
  // overdue). Default = roughly the current week.
  urgentWindowDays: { type: Number, default: 7 },
})
const emit = defineEmits(['open', 'changed', 'create'])
const message = useMessage()

// ── quadrant model ────────────────────────────────────────────
// Index encoding mirrors the backend (migration 0028):
//   0 = urgent + important      2 = urgent + not-important
//   1 = not-urgent + important   3 = not-urgent + not-important
// Laid out as a 2×2 grid: columns = Срочно | Несрочно, rows = Важно | Неважно.
const QUADRANTS = [
  { i: 0, title: 'Срочно и важно', hint: 'Сделать сейчас', cls: 'q-do' },
  { i: 1, title: 'Важно, не срочно', hint: 'Запланировать', cls: 'q-plan' },
  { i: 2, title: 'Срочно, не важно', hint: 'Делегировать', cls: 'q-delegate' },
  { i: 3, title: 'Не срочно, не важно', hint: 'Может подождать', cls: 'q-drop' },
]

// Derived quadrant when there's no manual override: importance from priority
// (high/urgent), urgency from due-date proximity.
function derive(t) {
  const important = (t.priority || 0) >= 3
  let urgent = false
  if (t.due_date) {
    const due = new Date(t.due_date).getTime()
    urgent = due <= Date.now() + props.urgentWindowDays * 86400000
  }
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
  for (const t of props.tasks) b[quadrantOf(t)].push(t)
  buckets.value = b
}
watch(() => props.tasks, rebuild, { immediate: true })

const dragging = ref(false)

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

// ── per-quadrant quick add (a reference tracker-style «＋») ──────────────────
const addingIn = ref(-1)
const newTitle = ref('')
function startAdd(qi) {
  addingIn.value = qi
  newTitle.value = ''
}
function submitAdd(qi) {
  const title = newTitle.value.trim()
  addingIn.value = -1
  if (!title) return
  // The parent creates the task in the board's first column, then pins the
  // quadrant — the matrix doesn't own a column to drop into.
  emit('create', { title, quadrant: qi })
}
</script>

<template>
  <div class="matrix">
    <div class="m-corner" />
    <div class="m-colhead m-col-urgent">Срочно</div>
    <div class="m-colhead">Несрочно</div>

    <div class="m-rowhead m-row-important"><span>Важно</span></div>
    <div class="m-rowhead m-row-unimportant"><span>Неважно</span></div>

    <div
      v-for="q in QUADRANTS"
      :key="q.i"
      class="m-quad"
      :class="[q.cls, `m-cell-${q.i}`]"
    >
      <div class="m-quad-head">
        <div class="m-quad-titles">
          <span class="m-quad-title">{{ q.title }}</span>
          <span class="m-quad-hint">{{ q.hint }}</span>
        </div>
        <span class="m-count">{{ buckets[q.i].length }}</span>
        <button type="button" class="m-add" title="Добавить задачу" @click="startAdd(q.i)">＋</button>
      </div>

      <draggable
        :list="buckets[q.i]"
        group="eisenhower"
        item-key="id"
        class="m-list"
        :animation="150"
        :delay="120"
        :delay-on-touch-only="true"
        :touch-start-threshold="6"
        @start="dragging = true"
        @end="dragging = false"
        @change="onQuadChange($event, q.i)"
      >
        <template #item="{ element }">
          <div class="m-card-wrap">
            <button
              v-if="isPinned(element)"
              type="button"
              class="m-pin"
              title="Размещено вручную — вернуть на авто"
              @click.stop="resetAuto(element)"
            >
              <n-icon :component="CloseCircleOutline" />
              <span>вручную</span>
            </button>
            <TaskCard
              :task="element"
              :subtasks="subtasksByParent[element.id] || []"
              :subtasks-expanded="subtasksExpanded"
              :dragging="dragging"
              :columns="columns"
              :tags-map="tagsMap"
              :members-map="membersMap"
              :tags="tags"
              :tag-prefix-names="tagPrefixNames"
              :members="members"
              :ws-id="wsId"
              :project-id="projectId"
              @open="$emit('open', $event)"
              @changed="$emit('changed')"
            />
          </div>
        </template>
      </draggable>

      <div v-if="addingIn === q.i" class="m-add-input">
        <n-input
          v-model:value="newTitle"
          type="textarea"
          size="small"
          autofocus
          :autosize="{ minRows: 1, maxRows: 4 }"
          placeholder="Название задачи, Enter — создать"
          @keyup.enter.prevent="submitAdd(q.i)"
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
  grid-template-columns: 22px 1fr 1fr;
  grid-template-rows: 26px 1fr 1fr;
  gap: 8px;
  /* Fill the content area below header + toolbar so quadrants scroll internally
     (mirrors the kanban board-scroll height). */
  height: calc(100vh - 140px);
  height: calc(100dvh - 140px);
  min-height: 420px;
}
.m-corner {
  grid-row: 1;
  grid-column: 1;
}
.m-colhead {
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 13px;
  font-weight: 600;
  color: var(--t-text2);
  letter-spacing: 0.02em;
}
.m-col-urgent {
  color: var(--t-text1);
}
.m-rowhead {
  grid-column: 1;
  display: flex;
  align-items: center;
  justify-content: center;
}
.m-rowhead span {
  writing-mode: vertical-rl;
  transform: rotate(180deg);
  font-size: 13px;
  font-weight: 600;
  color: var(--t-text2);
  letter-spacing: 0.04em;
}
.m-row-important {
  grid-row: 2;
}
.m-row-important span {
  color: var(--t-text1);
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

.m-quad {
  display: flex;
  flex-direction: column;
  min-height: 0;
  border: 1px solid var(--t-border);
  border-radius: 12px;
  background: var(--t-surface);
  overflow: hidden;
  /* A soft same-hue wash per quadrant (subtle, matches the calm palette). */
  border-top: 3px solid var(--q-accent, var(--t-border));
}
.q-do {
  --q-accent: #ef5d5d;
  background: color-mix(in srgb, #ef5d5d 6%, var(--t-surface));
}
.q-plan {
  --q-accent: #5b8def;
  background: color-mix(in srgb, #5b8def 5%, var(--t-surface));
}
.q-delegate {
  --q-accent: #e6a43b;
  background: color-mix(in srgb, #e6a43b 6%, var(--t-surface));
}
.q-drop {
  --q-accent: #9aa0aa;
  background: color-mix(in srgb, #9aa0aa 5%, var(--t-surface));
}

.m-quad-head {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 10px 6px;
}
.m-quad-titles {
  display: flex;
  flex-direction: column;
  line-height: 1.15;
  min-width: 0;
}
.m-quad-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--t-text1);
}
.m-quad-hint {
  font-size: 11px;
  color: var(--t-text3);
}
.m-count {
  margin-left: auto;
  font-size: 12px;
  font-weight: 600;
  color: var(--t-text3);
  min-width: 18px;
  text-align: center;
}
.m-add {
  border: none;
  background: transparent;
  color: var(--t-text3);
  font-size: 17px;
  line-height: 1;
  cursor: pointer;
  padding: 0 2px;
  border-radius: 6px;
}
.m-add:hover {
  color: var(--t-text1);
  background: var(--t-hover);
}
.m-list {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 2px 8px 10px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.m-card-wrap {
  position: relative;
}
.m-pin {
  position: absolute;
  top: 5px;
  right: 5px;
  z-index: 2;
  display: inline-flex;
  align-items: center;
  gap: 3px;
  font-size: 10px;
  line-height: 1;
  color: var(--t-text3);
  background: var(--t-surface);
  border: 1px solid var(--t-border);
  border-radius: 999px;
  padding: 2px 6px 2px 4px;
  cursor: pointer;
}
.m-pin:hover {
  color: var(--t-error, #e5484d);
  border-color: var(--t-error, #e5484d);
}
.m-add-input {
  padding: 0 8px 10px;
}
</style>
