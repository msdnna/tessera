<script setup>
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import { CalendarClearOutline } from '@vicons/ionicons5'
import { PRIORITY_COLORS } from '@/styles/tokens'
import { priorityLabel } from '@/utils/priority'
import { hueGrad } from '@/utils/gradient'
import UserAvatar from './UserAvatar.vue'
import TagPill from './TagPill.vue'
import { useFormat } from '@/composables/useFormat'

const props = defineProps({
  task: { type: Object, required: true },
  tagsMap: { type: Object, default: () => ({}) },
  membersMap: { type: Object, default: () => ({}) },
  tagPrefixNames: { type: Object, default: () => ({}) },
  // Column of the task, passed only where a divergence chip is shown on the row
  // it belongs to — the hover card then explains what that chip means.
  column: { type: Object, default: null },
})

const { t } = useI18n()
const { formatDate } = useFormat()

const tags = computed(() =>
  (props.task.tag_ids || []).map((id) => props.tagsMap[id]).filter(Boolean),
)
const assignees = computed(() =>
  (props.task.assignee_ids || []).map((id) => props.membersMap[id]).filter(Boolean),
)
const due = computed(() => {
  if (!props.task.due_date) return null
  return formatDate(props.task.due_date, { day: '2-digit', month: 'short' })
})
</script>

<template>
  <div class="mini" :class="{ done: task.completed_at }">
    <div class="mini-top">
      <span
        class="mini-pr"
        :style="{ background: hueGrad(PRIORITY_COLORS[task.priority || 0]) }"
        :title="priorityLabel(task.priority)"
      />
      <span v-if="task.number" class="mini-num">#{{ task.number }}</span>
      <span class="mini-title">{{ task.title }}</span>
    </div>

    <div v-if="tags.length" class="mini-tags">
      <TagPill
        v-for="tag in tags"
        :key="tag.id"
        class="mini-tag"
        :style="{ borderColor: tag.color || 'var(--t-border)' }"
        :tag="tag"
        :prefix-names="tagPrefixNames"
        variant="plain"
      />
    </div>

    <div v-if="column" class="mini-col">
      <span class="mini-dot" :style="{ background: column.color }" />
      {{ t('task.card.columnIs', { name: column.name }) }}
    </div>

    <div class="mini-foot">
      <span v-if="due" class="mini-due">
        <n-icon :component="CalendarClearOutline" :size="13" />
        {{ due }}
      </span>
      <span class="mini-avas">
        <UserAvatar
          v-for="a in assignees"
          :key="a.user_id"
          class="mini-ava"
          :user-id="a.user_id"
          :name="a.name"
          :title="a.name"
        />
      </span>
    </div>
  </div>
</template>

<style scoped>
.mini {
  width: 240px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.mini-top {
  display: flex;
  align-items: center;
  gap: 7px;
}
.mini-pr {
  flex: none;
  width: 9px;
  height: 9px;
  border-radius: 50%;
}
.mini-num {
  flex: none;
  font-size: 12px;
  color: var(--t-text3);
}
.mini-title {
  font-weight: 600;
  color: var(--t-text1);
  line-height: 1.3;
}
.mini.done .mini-title {
  text-decoration: line-through;
  color: var(--t-text3);
}
.mini-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}
.mini-tag {
  font-size: 11px;
  padding: 0 6px;
  border: 1px solid;
  border-radius: 10px;
  line-height: 17px;
}
.mini-foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}
.mini-col {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--t-text3);
}
.mini-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  flex: none;
  background: var(--t-text3);
}
.mini-due {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  color: var(--t-text2);
}
.mini-avas {
  display: flex;
  gap: 0;
}
.mini-ava {
  width: 22px;
  height: 22px;
  border-radius: 50%;
  background: var(--t-accent-grad);
  color: var(--t-on-primary);
  font-size: 10px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  margin-left: -4px;
  border: 2px solid var(--t-surface);
}
</style>
