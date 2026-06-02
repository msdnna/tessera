<script setup>
import { ref, computed } from 'vue'
import {
  NButton,
  NIcon,
  NBadge,
  NPopover,
  NTooltip,
  NSwitch,
  NText,
  NEmpty,
  NAvatar,
} from 'naive-ui'
import {
  SunnyOutline,
  MoonOutline,
  NotificationsOutline,
  PeopleOutline,
  LogOutOutline,
} from '@vicons/ionicons5'
import { useRouter } from 'vue-router'
import { useThemeStore, COLOR_THEMES } from '@/stores/theme'
import { useAuthStore } from '@/stores/auth'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useActivityStore } from '@/stores/activity'
import MembersModal from './MembersModal.vue'

defineProps({ mobile: { type: Boolean, default: false } })

const theme = useThemeStore()
const authStore = useAuthStore()
const ws = useWorkspacesStore()
const activity = useActivityStore()
const router = useRouter()

const showMembers = ref(false)

const initials = computed(() => {
  const n = (authStore.user?.name || authStore.user?.email || '?').trim()
  const parts = n.split(/\s+/)
  if (parts.length >= 2) return (parts[0][0] + parts[1][0]).toUpperCase()
  return n.slice(0, 2).toUpperCase()
})

function logout() {
  authStore.logout()
  router.push('/login')
}

function fmtTime(d) {
  return new Date(d).toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' })
}
</script>

<template>
  <div class="sb-footer">
    <div class="icons">
      <!-- Members -->
      <n-tooltip>
        <template #trigger>
          <n-button quaternary circle size="small" aria-label="Участники" @click="showMembers = true">
            <n-icon :component="PeopleOutline" />
          </n-button>
        </template>
        Участники
      </n-tooltip>

      <!-- Activity bell -->
      <n-popover trigger="click" placement="top-start" @update:show="(s) => s && activity.markRead()">
        <template #trigger>
          <n-badge :value="activity.unread" :max="9" :show="activity.unread > 0">
            <n-button quaternary circle size="small" aria-label="Активность">
              <n-icon :component="NotificationsOutline" />
            </n-button>
          </n-badge>
        </template>
        <div class="feed">
          <div v-for="it in activity.items" :key="it.id" class="feed-item">
            <span class="ft">{{ it.text }}</span>
            <span class="fa">{{ fmtTime(it.at) }}</span>
          </div>
          <n-empty v-if="!activity.items.length" description="Пока тихо" size="small" />
        </div>
      </n-popover>

      <!-- Appearance -->
      <n-popover trigger="click" placement="top-start">
        <template #trigger>
          <n-tooltip>
            <template #trigger>
              <n-button quaternary circle size="small" aria-label="Оформление">
                <span class="swatch" :style="{ background: theme.primaryColor }" />
              </n-button>
            </template>
            Оформление
          </n-tooltip>
        </template>
        <div class="appearance">
          <div class="row">
            <n-text depth="2">Тёмная тема</n-text>
            <n-switch :value="theme.isDark" @update:value="theme.toggle()">
              <template #checked-icon><n-icon :component="MoonOutline" /></template>
              <template #unchecked-icon><n-icon :component="SunnyOutline" /></template>
            </n-switch>
          </div>
          <div class="swatches">
            <button
              v-for="t in COLOR_THEMES"
              :key="t.key"
              class="swatch-btn"
              :class="{ active: t.key === theme.activeTheme.key }"
              :style="{ background: t.primary }"
              :title="t.name"
              @click="theme.selectColor(t)"
            />
          </div>
        </div>
      </n-popover>
    </div>

    <!-- User: desktop = avatar + name + logout; mobile = avatar popover -->
    <n-popover v-if="mobile" trigger="click" placement="top-start">
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

    <MembersModal v-model:show="showMembers" :ws-id="ws.currentId" />
  </div>
</template>

<style scoped>
.sb-footer {
  border-top: 1px solid var(--t-border);
  padding: 8px 10px 10px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.icons {
  display: flex;
  align-items: center;
  gap: 2px;
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
.swatch {
  width: 14px;
  height: 14px;
  border-radius: 50%;
  display: inline-block;
}
.appearance {
  width: 200px;
}
.row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 10px;
}
.swatches {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.swatch-btn {
  width: 24px;
  height: 24px;
  border-radius: 50%;
  border: 2px solid transparent;
  cursor: pointer;
  padding: 0;
}
.swatch-btn.active {
  border-color: var(--t-text1);
  box-shadow: 0 0 0 2px var(--t-surface);
}
.feed {
  width: 260px;
  max-height: 320px;
  overflow-y: auto;
}
.feed-item {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  padding: 6px 4px;
  border-bottom: 1px solid var(--t-border);
}
.feed-item:last-child {
  border-bottom: none;
}
.ft {
  color: var(--t-text1);
  font-size: 13px;
}
.fa {
  color: var(--t-text3);
  font-size: 11px;
  flex: none;
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
