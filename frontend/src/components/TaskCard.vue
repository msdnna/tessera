<script setup>
import { ref, computed, watch, nextTick } from 'vue'
import draggable from 'vuedraggable'
import { NIcon, NPopover, NDatePicker, NInput, NDropdown } from 'naive-ui'
import {
  FlagOutline,
  CalendarClearOutline,
  PersonAddOutline,
  PricetagOutline,
  CheckmarkCircle,
  EllipseOutline,
  CheckmarkOutline,
} from '@vicons/ionicons5'
import { tasks as tasksApi, workspaces as wsApi, boards as boardsApi } from '@/api'
import { PRIORITY_COLORS, PRIORITY_LABELS } from '@/styles/tokens'
import { dragActive } from '@/utils/dnd'

const props = defineProps({
  task: { type: Object, required: true },
  subtasks: { type: Array, default: () => [] },
  // Render subtasks as full property cards (vs compact name-only rows).
  subtasksExpanded: { type: Boolean, default: false },
  // This card is itself a first-level subtask shown below its parent: darker
  // shade, no nested-subtask cascade, no "create subtask" button.
  nested: { type: Boolean, default: false },
  // Board status columns [{ id, name }] for the context-menu "move to column".
  columns: { type: Array, default: () => [] },
  tagsMap: { type: Object, default: () => ({}) },
  membersMap: { type: Object, default: () => ({}) },
  tags: { type: Array, default: () => [] },
  members: { type: Array, default: () => [] },
  wsId: { type: String, default: null },
})
const emit = defineEmits(['open', 'changed'])

const newTagName = ref('')
const editingTitle = ref(false)
const titleEdit = ref('')
const titleInput = ref(null)
function startTitleEdit() {
  titleEdit.value = props.task.title
  editingTitle.value = true
  nextTick(() => titleInput.value?.focus?.())
}
async function commitTitle() {
  editingTitle.value = false
  const n = titleEdit.value.trim()
  if (!n || n === props.task.title) return
  await apply({ title: n })
}

const taskTags = computed(() =>
  (props.task.tag_ids || []).map((id) => props.tagsMap[id]).filter(Boolean),
)
const assignees = computed(() =>
  (props.task.assignee_ids || []).map((id) => props.membersMap[id]).filter(Boolean),
)
const due = computed(() =>
  props.task.due_date
    ? new Date(props.task.due_date).toLocaleDateString('ru-RU', { day: '2-digit', month: 'short' })
    : '',
)
const dueTs = computed(() => (props.task.due_date ? Date.parse(props.task.due_date) : null))
const done = computed(() => !!props.task.completed_at)
const priorityOptions = PRIORITY_LABELS.map((label, value) => ({ label, value }))
// Stacked-cards effect: offset colored shadows behind the top tag pill.
// Stacked-cards: each deeper layer peeks 5px further right and is a little
// shorter (larger negative spread) so it reads as a stack behind the top pill.
const stackLayers = computed(() => Math.min(taskTags.value.length - 1, 2))
const stackShadow = computed(() => {
  if (taskTags.value.length < 2) return ''
  return taskTags.value
    .slice(1, 3)
    .map((t, i) => `${(i + 1) * 5}px 0 0 ${-(i * 1 + 1)}px ${(t.color || '#888') + '55'}`)
    .join(', ')
})
const cardStyle = computed(() =>
  props.task.priority
    ? { borderLeftColor: PRIORITY_COLORS[props.task.priority], borderLeftWidth: '3px' }
    : {},
)

function initials(name) {
  return (name || '?').trim().slice(0, 2).toUpperCase()
}
function isAssigned(uid) {
  return (props.task.assignee_ids || []).includes(uid)
}
function hasTag(id) {
  return (props.task.tag_ids || []).includes(id)
}

function base() {
  return {
    title: props.task.title,
    description: props.task.description || '',
    priority: props.task.priority || 0,
    due_date: props.task.due_date || null,
    completed: done.value,
  }
}
async function apply(patch) {
  await tasksApi.update(props.task.id, { ...base(), ...patch })
  emit('changed')
}
const toggleDone = () => apply({ completed: !done.value })
const setPriority = (p) => apply({ priority: p })

// ── right-click context menu (works for the card and for collapsed subtasks) ──
const ctxShow = ref(false)
const ctxX = ref(0)
const ctxY = ref(0)
const ctxTarget = ref(null) // task object the menu acts on
const ctxOptions = computed(() => {
  const t = ctxTarget.value
  const isMain = t && t.id === props.task.id
  const cols = props.columns.filter((c) => c.id !== t?.column_id)
  return [
    { label: 'Открыть', key: 'open' },
    { label: t?.completed_at ? 'Снять выполнение' : 'Отметить выполненной', key: 'toggle' },
    {
      label: 'Приоритет',
      key: 'prio',
      children: PRIORITY_LABELS.map((l, i) => ({ label: l, key: 'prio:' + i })),
    },
    ...(cols.length
      ? [
          {
            label: 'Переместить в колонку',
            key: 'move',
            children: cols.map((c) => ({ label: c.name, key: 'col:' + c.id })),
          },
        ]
      : []),
    { type: 'divider', key: 'd1' },
    ...(isMain ? [{ label: 'Создать подзадачу', key: 'subtask' }] : []),
    { label: 'В архив', key: 'archive' },
    { label: 'Удалить', key: 'delete' },
  ]
})
function baseOf(t) {
  return {
    title: t.title,
    description: t.description || '',
    priority: t.priority || 0,
    due_date: t.due_date || null,
    completed: !!t.completed_at,
  }
}
function onCtx(e, target) {
  if (dragActive()) return // a touch drag is underway — don't pop the menu
  ctxTarget.value = target || props.task
  ctxShow.value = false
  ctxX.value = e.clientX
  ctxY.value = e.clientY
  nextTick(() => (ctxShow.value = true))
}
async function onCtxSelect(key) {
  const t = ctxTarget.value || props.task
  ctxShow.value = false
  if (key === 'open') return emit('open', t.id)
  if (key === 'subtask') return startAddSub()
  try {
    if (key === 'toggle') await tasksApi.update(t.id, { ...baseOf(t), completed: !t.completed_at })
    else if (key.startsWith('prio:'))
      await tasksApi.update(t.id, { ...baseOf(t), priority: Number(key.slice(5)) })
    else if (key.startsWith('col:'))
      await tasksApi.move(t.id, { column_id: key.slice(4), before_id: null, after_id: null })
    else if (key === 'archive') await tasksApi.archive(t.id)
    else if (key === 'delete') await tasksApi.remove(t.id)
    emit('changed')
  } catch {
    /* surfaced by the board's reload path */
  }
}
const setDue = (ts) => apply({ due_date: ts ? new Date(ts).toISOString() : null })

async function toggleTag(id) {
  if (hasTag(id)) await tasksApi.removeTag(props.task.id, id)
  else await tasksApi.addTag(props.task.id, id)
  emit('changed')
}
async function createTag() {
  const n = newTagName.value.trim()
  if (!n) return
  const palette = ['#7c5cff', '#2f80ed', '#0eb0a9', '#18a058', '#f0a020', '#e0533d', '#eb2f96']
  const res = await wsApi.createTag(props.wsId, {
    name: n,
    color: palette[Math.floor(Math.random() * palette.length)],
  })
  await tasksApi.addTag(props.task.id, res.data.id)
  newTagName.value = ''
  emit('changed')
}
async function toggleAssignee(uid) {
  if (isAssigned(uid)) await tasksApi.removeAssignee(props.task.id, uid)
  else await tasksApi.addAssignee(props.task.id, uid)
  emit('changed')
}

// ── subtasks ──
const addingSub = ref(false)
const newSubTitle = ref('')
const subInput = ref(null)
// Mutable mirror for drag-reorder of subtasks; resynced from the prop.
const subModel = ref([])
watch(
  () => props.subtasks,
  (v) => (subModel.value = [...v]),
  { immediate: true, deep: true },
)
async function onSubReorder(evt) {
  const info = evt.moved || evt.added
  if (!info) return
  const arr = subModel.value
  const before = arr[info.newIndex - 1]
  const after = arr[info.newIndex + 1]
  try {
    await tasksApi.move(info.element.id, {
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
  return s.due_date
    ? new Date(s.due_date).toLocaleDateString('ru-RU', { day: '2-digit', month: 'short' })
    : ''
}
async function toggleSubDone(s) {
  await tasksApi.update(s.id, {
    title: s.title,
    description: s.description || '',
    priority: s.priority || 0,
    due_date: s.due_date || null,
    completed: !s.completed_at,
  })
  emit('changed')
}
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
      :class="{ done, nested, 'has-subs': !nested && subtasks.length }"
      :style="cardStyle"
      @click="emit('open', task.id)"
      @contextmenu.prevent.stop="onCtx"
    >
      <div class="card-top">
      <span
        class="check"
        :title="done ? 'Выполнено' : 'Отметить выполненной'"
        @click.stop="toggleDone"
      >
        <n-icon :component="done ? CheckmarkCircle : EllipseOutline" :size="20" />
      </span>
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
      <span
        v-else
        class="title"
        title="Клик — изменить; клик по карточке — открыть"
        @click.stop="startTitleEdit"
        >{{ task.title }}</span
      >
      <span v-if="task.number" class="tnum">#{{ task.number }}</span>
    </div>

    <div class="pills">
      <!-- priority -->
      <n-popover trigger="click" placement="bottom-start">
        <template #trigger>
          <button class="pill" :class="{ set: task.priority }" @click.stop>
            <n-icon
              :component="FlagOutline"
              :size="13"
              :style="{ color: task.priority ? PRIORITY_COLORS[task.priority] : undefined }"
            />
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

      <!-- tags: stacked when >1; hover previews full list, click opens picker -->
      <n-popover trigger="click" placement="bottom-start">
        <template #trigger>
          <n-popover trigger="hover" :disabled="taskTags.length < 2" placement="top-start">
            <template #trigger>
              <button v-if="!taskTags.length" class="pill" @click.stop>
                <n-icon :component="PricetagOutline" :size="13" />
              </button>
              <button
                v-else
                class="pill tag-pill"
                :style="{
                  background: (taskTags[0].color || '#888') + '22',
                  borderColor: (taskTags[0].color || '#888') + '55',
                  color: taskTags[0].color || '#888',
                  boxShadow: stackShadow,
                  marginRight: stackLayers ? stackLayers * 5 + 4 + 'px' : undefined,
                }"
                @click.stop
              >
                <span class="tname">{{ taskTags[0].name }}</span>
                <span v-if="taskTags.length > 1" class="more">+{{ taskTags.length - 1 }}</span>
              </button>
            </template>
            <div class="preview">
              <span
                v-for="t in taskTags"
                :key="t.id"
                class="chip"
                :style="{ background: (t.color || '#888') + '22', color: t.color || '#888' }"
                >{{ t.name }}</span
              >
            </div>
          </n-popover>
        </template>
        <div class="menu tagmenu">
          <div class="chip-grid">
            <button
              v-for="t in tags"
              :key="t.id"
              class="tagchip"
              :class="{ on: hasTag(t.id) }"
              :style="
                hasTag(t.id)
                  ? { background: t.color || '#888', color: '#fff', borderColor: t.color || '#888' }
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
            @click.stop
          />
        </div>
      </n-popover>

      <!-- due date: opens the calendar directly -->
      <n-popover trigger="click" placement="bottom-start">
        <template #trigger>
          <button class="pill" :class="{ set: due }" @click.stop>
            <n-icon :component="CalendarClearOutline" :size="13" />
            <span v-if="due" class="pill-text">{{ due }}</span>
          </button>
        </template>
        <n-date-picker panel type="date" :value="dueTs" @update:value="setDue" />
      </n-popover>

      <span class="spacer" />

      <!-- assignees: avatar + name list -->
      <n-popover trigger="click" placement="bottom-end">
        <template #trigger>
          <button class="pill assignee-pill" @click.stop>
            <template v-if="assignees.length">
              <span v-for="u in assignees" :key="u.user_id" class="avatar" :title="u.name">
                {{ initials(u.name) }}
              </span>
            </template>
            <n-icon v-else :component="PersonAddOutline" :size="13" />
          </button>
        </template>
        <div class="menu">
          <div
            v-for="m in members"
            :key="m.user_id"
            class="menu-item assignee-item"
            @click="toggleAssignee(m.user_id)"
          >
            <span class="avatar sm">{{ initials(m.name) }}</span>
            <span class="aname">{{ m.name }}</span>
            <n-icon v-if="isAssigned(m.user_id)" :component="CheckmarkOutline" class="chk" />
          </div>
        </div>
      </n-popover>
      </div>
    </div>
    <!-- /.card -->

    <!-- Expanded: a stack of full subtask cards peeking from under the parent. -->
    <draggable
      v-if="!nested && subtasksExpanded && subtasks.length"
      :list="subModel"
      :group="'sub-' + task.id"
      item-key="id"
      class="substack"
      :animation="150"
      :delay="300"
      :touch-start-threshold="6"
      @click.stop
      @change="onSubReorder"
    >
      <template #item="{ element: s, index }">
        <div class="sub-layer" :style="{ zIndex: 40 - index }">
          <TaskCard
            :task="s"
            :subtasks="[]"
            :nested="true"
            :tags-map="tagsMap"
            :members-map="membersMap"
            :tags="tags"
            :members="members"
            :ws-id="wsId"
            @open="emit('open', $event)"
            @changed="emit('changed')"
          />
        </div>
      </template>
    </draggable>

    <!-- Collapsed: one card peeking from under the parent, listing the subtasks. -->
    <div v-else-if="!nested && subtasks.length" class="sub-list">
      <draggable
        :list="subModel"
        :group="'sub-' + task.id"
        item-key="id"
        :animation="150"
        :delay="300"
        :touch-start-threshold="6"
        @click.stop
        @change="onSubReorder"
      >
        <template #item="{ element: s }">
          <div
            class="subrow"
            :class="{ done: s.completed_at }"
            @click="emit('open', s.id)"
            @contextmenu.prevent.stop="onCtx($event, s)"
          >
            <span class="check sm" @click.stop="toggleSubDone(s)">
              <n-icon :component="s.completed_at ? CheckmarkCircle : EllipseOutline" :size="15" />
            </span>
            <span
              v-if="s.priority"
              class="pr-dot"
              :style="{ background: PRIORITY_COLORS[s.priority] }"
            />
            <span class="sub-title">{{ s.title }}</span>
            <span v-if="subDue(s)" class="sub-due">{{ subDue(s) }}</span>
          </div>
        </template>
      </draggable>
    </div>

    <template v-if="!nested">
      <div v-if="addingSub" class="sub-add-input" @click.stop>
        <n-input
          ref="subInput"
          v-model:value="newSubTitle"
          size="tiny"
          placeholder="Название подзадачи, Enter"
          @keyup.enter="submitAddSub"
          @keyup.esc="addingSub = false"
          @blur="submitAddSub"
        />
      </div>
      <button v-else class="add-sub" @click.stop="startAddSub">＋ Создать подзадачу</button>
    </template>

    <n-dropdown
      trigger="manual"
      placement="bottom-start"
      :show="ctxShow"
      :x="ctxX"
      :y="ctxY"
      :options="ctxOptions"
      @select="onCtxSelect"
      @clickoutside="ctxShow = false"
    />
  </div>
</template>

<style scoped>
.tw {
  margin-bottom: 8px;
}
.card {
  background: var(--t-surface);
  border: 1px solid var(--t-border);
  border-radius: 8px;
  padding: 8px 10px;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.04);
  cursor: pointer;
}
/* The parent keeps its rounded corners and sits above the subtask stack so the
   children appear to emerge from under it. */
.card.has-subs {
  position: relative;
  z-index: 50;
}
/* Background shared by subtask cards / the collapsed list. Tweak here:
   alternatives — var(--t-hover), var(--t-border) (greyer),
   color-mix(in srgb, var(--t-primary) 8%, var(--t-surface)) (accent). */
.sub-layer,
.sub-list {
  --sub-bg: color-mix(in srgb, var(--t-surface) 70%, var(--t-bg));
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
  background: var(--sub-bg);
  border-radius: 0 0 8px 8px;
  padding-top: 16px;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.04);
}
/* Collapsed: a single card emerging from under the parent, holding the list. */
.sub-list {
  position: relative;
  z-index: 1;
  margin-top: -8px;
  padding: 14px 8px 6px;
  background: var(--sub-bg);
  border: 1px solid var(--t-border);
  border-radius: 0 0 8px 8px;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.04);
}
.title-edit {
  flex: 1;
}
.card-top {
  display: flex;
  align-items: flex-start;
  gap: 8px;
}
.check {
  cursor: pointer;
  color: var(--t-text3);
  display: inline-flex;
}
.card.done .check {
  color: var(--t-primary);
}
.card.done .title {
  text-decoration: line-through;
  opacity: 0.6;
}
.title {
  flex: 1;
  font-size: 14px;
  color: var(--t-text1);
  cursor: pointer;
  min-width: 0;
  padding-top: 2px;
}
.tnum {
  flex: none;
  font-size: 11px;
  color: var(--t-text3);
  padding-top: 3px;
}
.pills {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  margin-top: 8px;
}
.pill {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  min-height: 22px;
  padding: 2px 6px;
  border-radius: 6px;
  border: 1px dashed var(--t-border);
  background: transparent;
  color: var(--t-text3);
  cursor: pointer;
}
.pill.set {
  border-style: solid;
  color: var(--t-text2);
}
.pill-text {
  font-size: 11px;
}
.chip {
  font-size: 11px;
  padding: 1px 8px;
  border-radius: 10px;
}
.tag-pill {
  border-style: solid;
  gap: 5px;
}
.tname {
  font-size: 11px;
  max-width: 120px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.more {
  font-size: 10px;
  opacity: 0.8;
}
.preview {
  display: flex;
  flex-wrap: wrap;
  gap: 5px;
  max-width: 220px;
}
.spacer {
  flex: 1;
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
  /* cascade: overlap the previous avatar with a ring in the card colour */
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
.assignee-pill {
  border: none;
  padding: 2px;
}
.menu {
  min-width: 180px;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.tagmenu {
  max-height: 260px;
  overflow-y: auto;
}
.chip-grid {
  display: flex;
  flex-wrap: wrap;
  gap: 5px;
  margin-bottom: 8px;
}
.tagchip {
  font-size: 12px;
  padding: 2px 9px;
  border-radius: 10px;
  border: 1px solid transparent;
  background: transparent;
  cursor: pointer;
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
.assignee-item .aname {
  flex: 1;
}
.assignee-item .chk {
  color: var(--t-primary);
}
.dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
}

/* First-level subtasks cascade directly below the parent card (no indent).
   Expanded cards attach with no gap; collapsed text rows get a little spacing. */
.substack {
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
.sub-add-input {
  margin-top: 6px;
}
.add-sub {
  margin-top: 6px;
  width: 100%;
  background: transparent;
  border: none;
  color: var(--t-text3);
  font-size: 10px;
  letter-spacing: 0.4px;
  text-transform: uppercase;
  padding: 4px;
  border-radius: 6px;
  cursor: pointer;
  opacity: 0;
  transition: opacity 0.12s;
}
.tw:hover .add-sub {
  opacity: 1;
}
.add-sub:hover {
  background: var(--t-hover);
}
</style>
