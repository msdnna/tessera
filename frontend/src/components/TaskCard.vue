<script setup>
import { ref, computed } from 'vue'
import { NIcon, NPopover, NDatePicker, NInput, NCheckbox } from 'naive-ui'
import {
  FlagOutline,
  CalendarClearOutline,
  PersonAddOutline,
  PricetagOutline,
  CheckmarkCircle,
  EllipseOutline,
} from '@vicons/ionicons5'
import { tasks as tasksApi, workspaces as wsApi } from '@/api'
import { PRIORITY_COLORS, PRIORITY_LABELS } from '@/styles/tokens'

const props = defineProps({
  task: { type: Object, required: true },
  tagsMap: { type: Object, default: () => ({}) },
  membersMap: { type: Object, default: () => ({}) },
  tags: { type: Array, default: () => [] }, // workspace tags (for the picker)
  members: { type: Array, default: () => [] },
  wsId: { type: String, default: null },
})
const emit = defineEmits(['open', 'changed'])

const newTagName = ref('')

const taskTags = computed(() =>
  (props.task.tag_ids || []).map((id) => props.tagsMap[id]).filter(Boolean),
)
const assignees = computed(() =>
  (props.task.assignee_ids || []).map((id) => props.membersMap[id]).filter(Boolean),
)
const due = computed(() => {
  if (!props.task.due_date) return ''
  return new Date(props.task.due_date).toLocaleDateString('ru-RU', {
    day: '2-digit',
    month: 'short',
  })
})
const dueTs = computed(() => (props.task.due_date ? Date.parse(props.task.due_date) : null))
const done = computed(() => !!props.task.completed_at)
const priorityOptions = PRIORITY_LABELS.map((label, value) => ({ label, value }))

function initials(name) {
  return (name || '?').trim().slice(0, 2).toUpperCase()
}

// All edits send the full task payload + the changed field, then reload.
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
const setDue = (ts) => apply({ due_date: ts ? new Date(ts).toISOString() : null })

async function toggleTag(id) {
  if ((props.task.tag_ids || []).includes(id)) await tasksApi.removeTag(props.task.id, id)
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
  if ((props.task.assignee_ids || []).includes(uid))
    await tasksApi.removeAssignee(props.task.id, uid)
  else await tasksApi.addAssignee(props.task.id, uid)
  emit('changed')
}
</script>

<template>
  <div class="card" :class="{ done }">
    <div class="card-top">
      <span
        class="check"
        :title="done ? 'Выполнено' : 'Отметить выполненной'"
        @click.stop="toggleDone"
      >
        <n-icon :component="done ? CheckmarkCircle : EllipseOutline" :size="16" />
      </span>
      <span class="title" @click="emit('open', task.id)">{{ task.title }}</span>
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

      <!-- tags -->
      <n-popover trigger="click" placement="bottom-start">
        <template #trigger>
          <button class="pill" :class="{ set: taskTags.length }" @click.stop>
            <n-icon v-if="!taskTags.length" :component="PricetagOutline" :size="13" />
            <span
              v-for="t in taskTags"
              :key="t.id"
              class="chip"
              :style="{ background: (t.color || '#888') + '22', color: t.color || '#888' }"
              >{{ t.name }}</span
            >
          </button>
        </template>
        <div class="menu tagmenu">
          <div v-for="t in tags" :key="t.id" class="menu-item" @click="toggleTag(t.id)">
            <n-checkbox :checked="(task.tag_ids || []).includes(t.id)" />
            <span class="dot" :style="{ background: t.color || '#888' }" />
            {{ t.name }}
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

      <!-- due date -->
      <n-popover trigger="click" placement="bottom-start">
        <template #trigger>
          <button class="pill" :class="{ set: due }" @click.stop>
            <n-icon :component="CalendarClearOutline" :size="13" />
            <span v-if="due" class="pill-text">{{ due }}</span>
          </button>
        </template>
        <n-date-picker :value="dueTs" type="date" clearable @update:value="setDue" />
      </n-popover>

      <span class="spacer" />

      <!-- assignees -->
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
            class="menu-item"
            @click="toggleAssignee(m.user_id)"
          >
            <n-checkbox :checked="(task.assignee_ids || []).includes(m.user_id)" />
            {{ m.name }}
          </div>
        </div>
      </n-popover>
    </div>
  </div>
</template>

<style scoped>
.card {
  background: var(--t-surface);
  border: 1px solid var(--t-border);
  border-radius: 8px;
  padding: 8px 10px;
  margin-bottom: 8px;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.04);
}
.card-top {
  display: flex;
  align-items: flex-start;
  gap: 7px;
}
.check {
  cursor: pointer;
  color: var(--t-text3);
  display: inline-flex;
  margin-top: 1px;
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
  padding: 0 6px;
  border-radius: 8px;
}
.spacer {
  flex: 1;
}
.avatar {
  width: 20px;
  height: 20px;
  border-radius: 50%;
  background: var(--t-primary);
  color: var(--t-on-primary);
  font-size: 10px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  margin-left: -4px;
}
.avatar:first-child {
  margin-left: 0;
}
.assignee-pill {
  border: none;
  padding: 2px;
}
.menu {
  min-width: 160px;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.tagmenu {
  max-height: 240px;
  overflow-y: auto;
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
.dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
}
</style>
