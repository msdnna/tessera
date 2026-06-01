<script setup>
import { computed } from 'vue'
import { NIcon } from 'naive-ui'
import { CalendarClearOutline } from '@vicons/ionicons5'
import { PRIORITY_COLORS } from '@/styles/tokens'

const props = defineProps({
  task: { type: Object, required: true },
  tagsMap: { type: Object, default: () => ({}) },
  membersMap: { type: Object, default: () => ({}) },
})

const tags = computed(() =>
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

function initials(name) {
  return (name || '?').trim().slice(0, 2).toUpperCase()
}
</script>

<template>
  <div class="card" :class="{ done: task.completed_at }">
    <div class="top">
      <span
        class="pr"
        :style="{ background: PRIORITY_COLORS[task.priority] || PRIORITY_COLORS[0] }"
      />
      <span class="title">{{ task.title }}</span>
    </div>
    <div v-if="tags.length || due || assignees.length" class="meta">
      <span
        v-for="t in tags"
        :key="t.id"
        class="chip"
        :style="{
          background: (t.color || '#888') + '22',
          color: t.color || '#888',
          borderColor: (t.color || '#888') + '55',
        }"
        >{{ t.name }}</span
      >
      <span v-if="due" class="due">
        <n-icon :component="CalendarClearOutline" :size="13" />
        {{ due }}
      </span>
      <span class="spacer" />
      <span v-for="u in assignees" :key="u.user_id" class="avatar" :title="u.name">{{
        initials(u.name)
      }}</span>
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
  cursor: pointer;
}
.card:hover {
  border-color: var(--t-primary);
}
.card.done .title {
  text-decoration: line-through;
  opacity: 0.6;
}
.top {
  display: flex;
  align-items: center;
  gap: 8px;
}
.pr {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex: none;
}
.title {
  font-size: 14px;
  color: var(--t-text1);
}
.meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  margin-top: 8px;
}
.chip {
  font-size: 11px;
  padding: 1px 7px;
  border-radius: 10px;
  border: 1px solid transparent;
}
.due {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  font-size: 11px;
  color: var(--t-text3);
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
}
</style>
