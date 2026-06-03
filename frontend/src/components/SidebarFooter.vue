<script setup>
import { computed } from 'vue'
import { NButton, NIcon, NPopover, NTooltip, NAvatar } from 'naive-ui'
import { LogOutOutline } from '@vicons/ionicons5'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const props = defineProps({
  mobile: { type: Boolean, default: false },
  collapsed: { type: Boolean, default: false },
})

const authStore = useAuthStore()
const router = useRouter()

const initials = computed(() => {
  const n = (authStore.user?.name || authStore.user?.email || '?').trim()
  const parts = n.split(/\s+/)
  if (parts.length >= 2) return (parts[0][0] + parts[1][0]).toUpperCase()
  return n.slice(0, 2).toUpperCase()
})

// On mobile (drawer) or when the rail is collapsed, show the avatar with a
// popover instead of the inline name + logout button.
const compact = computed(() => props.mobile || props.collapsed)

function logout() {
  authStore.logout()
  router.push('/login')
}
</script>

<template>
  <div class="sb-footer" :class="{ collapsed }">
    <n-popover v-if="compact" trigger="click" :placement="collapsed ? 'right-end' : 'top-start'">
      <template #trigger>
        <n-avatar round :size="32" class="ava">{{ initials }}</n-avatar>
      </template>
      <div class="user-pop">
        <div class="up-name">{{ authStore.user?.name || 'Профиль' }}</div>
        <div class="up-mail">{{ authStore.user?.email }}</div>
        <n-button size="small" block @click="logout">
          <template #icon><n-icon :component="LogOutOutline" /></template>
          Выйти
        </n-button>
      </div>
    </n-popover>
    <div v-else class="user">
      <n-avatar round :size="30" class="ava">{{ initials }}</n-avatar>
      <span class="uname">{{ authStore.user?.name || 'Профиль' }}</span>
      <n-tooltip>
        <template #trigger>
          <n-button quaternary circle size="small" aria-label="Выйти" @click="logout">
            <n-icon :component="LogOutOutline" />
          </n-button>
        </template>
        Выйти
      </n-tooltip>
    </div>
  </div>
</template>

<style scoped>
.sb-footer {
  border-top: 1px solid var(--t-border);
  padding: 10px;
}
.sb-footer.collapsed {
  display: flex;
  justify-content: center;
}
.user {
  display: flex;
  align-items: center;
  gap: 8px;
}
.ava {
  flex: none;
  background: var(--t-primary);
  color: var(--t-on-primary);
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
}
.uname {
  flex: 1;
  font-size: 13px;
  font-weight: 600;
  color: var(--t-text1);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.user-pop {
  width: 200px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.up-name {
  font-weight: 600;
  color: var(--t-text1);
}
.up-mail {
  font-size: 12px;
  color: var(--t-text3);
  overflow: hidden;
  text-overflow: ellipsis;
}
</style>
