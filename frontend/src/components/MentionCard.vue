<script setup>
import { computed } from 'vue'
import UserAvatar from './UserAvatar.vue'
import { roleLabel } from '@/utils/mentions'

// The hover card behind an @-mention chip (see RichContent's `mention-cards`).
// Deliberately neutral — no accent gradient on the card itself; only the avatar
// carries one, as it does everywhere else.
//
// A GitLab-only user has no Tessera account, so there is no email and no
// workspace role to show: the task asks for name, handle and avatar, and that is
// all such a person has.
const props = defineProps({
  item: { type: Object, default: null },
})

const name = computed(() => props.item?.display || props.item?.label || '')
const handle = computed(() => {
  const it = props.item
  if (!it) return ''
  return it.gitlab ? `@${it.username || it.label}` : it.email || ''
})
const role = computed(() => (props.item?.gitlab ? '' : roleLabel(props.item?.role)))
</script>

<template>
  <div v-if="item" class="mc">
    <UserAvatar
      class="mc-ava"
      :user-id="item.avatarUserId || ''"
      :src="item.avatarSrc || ''"
      :name="name"
    />
    <div class="mc-body">
      <div class="mc-name">{{ name }}</div>
      <div v-if="handle" class="mc-handle">{{ handle }}</div>
      <div v-if="role" class="mc-role">{{ role }}</div>
    </div>
  </div>
</template>

<style scoped>
.mc {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 14px;
  min-width: 200px;
  max-width: 300px;
  background: var(--t-surface);
  border: 1px solid var(--t-border);
  border-radius: 10px;
  box-shadow: var(--t-shadow-2, 0 6px 20px rgba(0, 0, 0, 0.18));
}
.mc-ava {
  flex: none;
  width: 48px;
  height: 48px;
  border-radius: 50%;
  background: var(--t-accent-grad);
  color: var(--t-on-primary);
  font-size: 17px;
  font-weight: 600;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}
.mc-body {
  min-width: 0;
}
.mc-name {
  font-weight: 600;
  font-size: 14px;
  color: var(--t-text);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.mc-handle {
  font-size: 12px;
  color: var(--t-text2);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.mc-role {
  margin-top: 3px;
  font-size: 11px;
  color: var(--t-text3);
}
</style>
