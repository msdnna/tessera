<script setup>
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { NPopover } from 'naive-ui'
import { useBoardViewStore } from '@/stores/boardView'
import { useAuthStore } from '@/stores/auth'
import UserAvatar from './UserAvatar.vue'

// The people of this board, as a cascade in the sub-toolbar (v2, #2954, #3112):
// the members who have tasks on it — assignees of the loaded tasks — the busiest
// first; three avatars and «+N», the list «На доске · N» with roles and task
// counts on hover. Who is looking at the board right now (#3116, board.presence)
// comes first, with a green dot; nobody with tasks and nobody else here — no
// cascade.
const props = defineProps({
  // The board's loaded tasks (subtasks included).
  tasks: { type: Array, default: () => [] },
})
const store = useBoardViewStore()
const { t } = useI18n()
const SHOWN = 3
const ROLE_ORDER = { owner: 0, admin: 1, member: 2 }
const auth = useAuthStore()

// The others on this board now (me excluded — I know I'm here), as members.
const watching = computed(() => {
  const p = store.present
  if (!p.boardId || p.boardId !== store.boardId) return []
  const byId = new Map(store.membersList.map((m) => [m.user_id, m]))
  return p.userIds.filter((id) => id !== auth.user?.id && byId.has(id)).map((id) => byId.get(id))
})
const watchingIds = computed(() => new Set(watching.value.map((m) => m.user_id)))

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
// The cascade: who is here first, then the busiest of the rest.
const cascade = computed(() => [
  ...watching.value,
  ...members.value.filter((m) => !watchingIds.value.has(m.user_id)),
])
const shown = computed(() => cascade.value.slice(0, SHOWN))
const extra = computed(() => Math.max(0, cascade.value.length - SHOWN))
const roleLabel = (r) => (r && ROLE_ORDER[r] !== undefined ? t(`shell.members.${r}`) : '')
</script>

<template>
  <!-- A real root, so the sub-toolbar's class (its place in the row) lands on
       an element rather than on the popover's fragment. -->
  <div v-if="cascade.length" class="bm-root">
    <n-popover trigger="hover" placement="bottom-end" :delay="300" :show-arrow="false" raw>
      <template #trigger>
        <div class="bm" data-testid="board-members">
          <span
            v-for="m in shown"
            :key="m.user_id"
            class="bm-slot"
            :class="{ here: watchingIds.has(m.user_id) }"
            :data-here="watchingIds.has(m.user_id) || undefined"
          >
            <UserAvatar class="bm-ava" :user-id="m.user_id" :name="m.name" />
          </span>
          <span v-if="extra" class="bm-ava bm-more">+{{ extra }}</span>
        </div>
      </template>
      <div class="bm-list" data-testid="board-members-list">
        <template v-if="watching.length">
          <div class="bm-head">
            <span>{{ t('board.members.watching', { count: watching.length }) }}</span>
          </div>
          <div class="bm-rows" data-testid="board-members-presence">
            <div v-for="m in watching" :key="m.user_id" class="bm-row">
              <span class="bm-slot here">
                <UserAvatar class="bm-ava sm" :user-id="m.user_id" :name="m.name" />
              </span>
              <span class="bm-name">{{ m.name }}</span>
              <span class="bm-role">{{ roleLabel(m.role) }}</span>
            </div>
          </div>
          <div v-if="members.length" class="bm-gap" />
        </template>
        <div v-if="members.length" class="bm-head">
          <span>{{ t('board.members.title', { count: members.length }) }}</span>
          <span class="bm-head-aside">{{ t('board.members.tasks') }}</span>
        </div>
        <div v-if="members.length" class="bm-rows">
          <div v-for="m in members" :key="m.user_id" class="bm-row">
            <UserAvatar class="bm-ava sm" :user-id="m.user_id" :name="m.name" />
            <span class="bm-name">{{ m.name }}</span>
            <span class="bm-role">{{ roleLabel(m.role) }}</span>
            <span class="bm-n">{{ m.tasks }}</span>
          </div>
        </div>
      </div>
    </n-popover>
  </div>
</template>

<style scoped>
.bm-root {
  display: flex;
}
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
  width: 304px;
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
.bm-row:hover {
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
.bm-slot {
  position: relative;
  display: inline-flex;
}
/* On the board right now (#3116): a green dot at the avatar's corner. */
.bm-slot.here::after {
  content: '';
  position: absolute;
  right: -1px;
  bottom: -1px;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #18a058;
  box-shadow: 0 0 0 2px var(--t-bg);
}
.bm-rows .bm-slot.here::after {
  box-shadow: 0 0 0 2px var(--t-pop);
}
</style>
