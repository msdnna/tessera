<script setup>
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { NPopover } from 'naive-ui'
import { useBoardViewStore } from '@/stores/boardView'
import UserAvatar from './UserAvatar.vue'

// Who works on this board, as a cascade in the sub-toolbar (v2, #2954): three
// avatars and «+N», the full list with roles on hover. A board has no members
// of its own — they are the workspace's.
const store = useBoardViewStore()
const { t } = useI18n()
const SHOWN = 3
const ROLE_ORDER = { owner: 0, admin: 1, member: 2 }
const members = computed(() =>
  [...store.membersList].sort(
    (a, b) =>
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
    :delay="400"
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
    <div class="bm-list">
      <div class="bm-head">{{ t('board.members.title', { count: members.length }) }}</div>
      <div v-for="m in members" :key="m.user_id" class="bm-row">
        <UserAvatar class="bm-ava sm" :user-id="m.user_id" :name="m.name" />
        <span class="bm-name">{{ m.name }}</span>
        <span class="bm-role">{{ roleLabel(m.role) }}</span>
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
.bm-list {
  width: 250px;
  max-height: 60vh;
  overflow-y: auto;
  padding: 6px;
  border-radius: var(--t-r-tip);
  background: var(--t-pop);
  box-shadow: var(--t-sh-pop);
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.bm-head {
  padding: 4px 6px;
  font-size: 11.5px;
  font-weight: 600;
  letter-spacing: 0.05em;
  text-transform: uppercase;
  color: var(--t-text3);
}
.bm-row {
  display: flex;
  align-items: center;
  gap: 8px;
  height: 32px;
  padding: 0 6px;
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
  font-size: 13px;
  color: var(--t-text1);
}
.bm-role {
  font-size: 11.5px;
  color: var(--t-text3);
}
</style>
