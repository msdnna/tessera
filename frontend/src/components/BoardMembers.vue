<script setup>
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon, NPopover } from 'naive-ui'
import { EyeOutline } from '@vicons/ionicons5'
import { useBoardViewStore } from '@/stores/boardView'
import { stubTask } from '@/utils/featureStubs'
import UserAvatar from './UserAvatar.vue'

// The people of this board, as a cascade in the sub-toolbar (v2, #2954, #3112):
// the members who have tasks on it — assignees of the loaded tasks — the busiest
// first; three avatars and «+N», the list «На доске · N» with roles and task
// counts on hover. No task with an assignee — no cascade. Who is looking at the
// board right now is a stub until #3116.
const props = defineProps({
  // The board's loaded tasks (subtasks included).
  tasks: { type: Array, default: () => [] },
})
const store = useBoardViewStore()
const { t } = useI18n()
const SHOWN = 3
const ROLE_ORDER = { owner: 0, admin: 1, member: 2 }
const presenceStub = stubTask('board.presence')

const counts = computed(() => {
  const out = new Map()
  for (const task of props.tasks)
    for (const id of task.assignee_ids || []) out.set(id, (out.get(id) || 0) + 1)
  return out
})
const members = computed(() =>
  store.membersList
    .filter((m) => counts.value.has(m.user_id))
    .map((m) => ({ ...m, tasks: counts.value.get(m.user_id) }))
    .sort(
      (a, b) =>
        b.tasks - a.tasks ||
        (ROLE_ORDER[a.role] ?? 3) - (ROLE_ORDER[b.role] ?? 3) ||
        (a.name || '').localeCompare(b.name || ''),
    ),
)
const shown = computed(() => members.value.slice(0, SHOWN))
const extra = computed(() => Math.max(0, members.value.length - SHOWN))
const roleLabel = (r) => (r && ROLE_ORDER[r] !== undefined ? t(`shell.members.${r}`) : '')
</script>

<template>
  <n-popover
    v-if="members.length"
    trigger="hover"
    placement="bottom-end"
    :delay="300"
    :show-arrow="false"
    raw
  >
    <template #trigger>
      <div class="bm" data-testid="board-members">
        <UserAvatar
          v-for="m in shown"
          :key="m.user_id"
          class="bm-ava"
          :user-id="m.user_id"
          :name="m.name"
        />
        <span v-if="extra" class="bm-ava bm-more">+{{ extra }}</span>
      </div>
    </template>
    <div class="bm-list" data-testid="board-members-list">
      <div class="bm-head">
        <span>{{ t('board.members.title', { count: members.length }) }}</span>
        <span class="bm-head-aside">{{ t('board.members.tasks') }}</span>
      </div>
      <div class="bm-rows">
        <div v-for="m in members" :key="m.user_id" class="bm-row">
          <UserAvatar class="bm-ava sm" :user-id="m.user_id" :name="m.name" />
          <span class="bm-name">{{ m.name }}</span>
          <span class="bm-role">{{ roleLabel(m.role) }}</span>
          <span class="bm-n">{{ m.tasks }}</span>
        </div>
      </div>
      <div class="bm-gap" />
      <div
        class="bm-row bm-stub"
        aria-disabled="true"
        :title="t('common.stub.soon', { n: presenceStub })"
        data-testid="board-members-presence"
      >
        <n-icon :component="EyeOutline" :size="16" class="bm-stub-ic" />
        <span class="bm-name">{{ t('board.members.watching') }}</span>
        <span class="bm-soon">{{ t('common.stub.soon', { n: presenceStub }) }}</span>
      </div>
    </div>
  </n-popover>
</template>

<style scoped>
.bm {
  display: flex;
  align-items: center;
  padding-left: 7px;
  cursor: default;
}
.bm-ava {
  width: 26px;
  height: 26px;
  flex: none;
  margin-left: -7px;
  border-radius: 50%;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  background: var(--t-accent-grad);
  color: var(--t-on-primary);
  font-size: 10.5px;
  font-weight: 600;
  box-shadow: 0 0 0 2px var(--t-bg);
}
.bm-more {
  width: auto;
  min-width: 26px;
  padding: 0 6px;
  box-sizing: border-box;
  border-radius: 13px;
  background: var(--t-pill);
  color: var(--t-text2);
  font-size: 11.5px;
}
.bm-ava.sm {
  width: 20px;
  height: 20px;
  margin-left: 0;
  font-size: 9px;
  box-shadow: none;
}
/* The v2 popover (#3112): --t-pop, radius 16, the popover shadow. */
.bm-list {
  width: 280px;
  padding: 6px;
  box-sizing: border-box;
  border-radius: 16px;
  background: var(--t-pop);
  box-shadow: var(--t-sh-pop);
  display: flex;
  flex-direction: column;
  gap: 2px;
  --t-surface: var(--t-pop);
}
.bm-head {
  display: flex;
  gap: 8px;
  padding: 6px 10px 4px;
  font-size: 11.5px;
  font-weight: 600;
  letter-spacing: 0.05em;
  text-transform: uppercase;
  color: var(--t-text3);
}
.bm-head > span:first-child {
  flex: 1;
}
.bm-head-aside {
  font-weight: 500;
  letter-spacing: 0;
  text-transform: none;
}
.bm-rows {
  max-height: 50vh;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.bm-row {
  display: flex;
  align-items: center;
  gap: 10px;
  min-height: 32px;
  padding: 0 10px;
  border-radius: 7px;
}
.bm-row:not(.bm-stub):hover {
  background: var(--t-hover);
}
.bm-name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 13.5px;
  color: var(--t-text1);
}
.bm-role {
  font-size: 11.5px;
  color: var(--t-text3);
}
.bm-n {
  min-width: 22px;
  text-align: right;
  font-size: 12px;
  font-weight: 600;
  color: var(--t-text2);
  font-variant-numeric: tabular-nums;
}
.bm-gap {
  height: 4px;
}
.bm-stub {
  cursor: not-allowed;
}
.bm-stub .bm-name,
.bm-stub-ic {
  color: var(--t-placeholder);
}
.bm-soon {
  font-size: 11.5px;
  color: var(--t-text3);
  white-space: nowrap;
}
</style>
