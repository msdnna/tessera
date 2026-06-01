<script setup>
import { NButton, NSpace, NText, NDropdown } from 'naive-ui'
import { useRouter } from 'vue-router'
import { useThemeStore } from '@/stores/theme'
import { useAuthStore } from '@/stores/auth'
import { useWorkspacesStore } from '@/stores/workspaces'

const theme = useThemeStore()
const authStore = useAuthStore()
const ws = useWorkspacesStore()
const router = useRouter()

const userOptions = [{ label: 'Выйти', key: 'logout' }]

function onUserSelect(key) {
  if (key === 'logout') {
    authStore.logout()
    router.push('/login')
  }
}
</script>

<template>
  <div class="topbar">
    <n-text strong>{{ ws.current?.name || 'Tessera' }}</n-text>
    <n-space align="center">
      <n-button
        quaternary
        circle
        :title="theme.isDark ? 'Светлая тема' : 'Тёмная тема'"
        @click="theme.toggle()"
      >
        {{ theme.isDark ? '☀' : '☾' }}
      </n-button>
      <n-dropdown trigger="click" :options="userOptions" @select="onUserSelect">
        <n-button quaternary>{{ authStore.user?.name || 'Профиль' }}</n-button>
      </n-dropdown>
    </n-space>
  </div>
</template>

<style scoped>
.topbar {
  height: 52px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 16px;
}
</style>
