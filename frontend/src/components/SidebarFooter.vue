<script setup>
import { computed } from 'vue'
import { NButton, NIcon, NPopover, NTooltip, NAvatar } from 'naive-ui'
import {
  LogOutOutline,
  SchoolOutline,
  SettingsOutline,
  ShieldCheckmarkOutline,
} from '@vicons/ionicons5'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useTourStore } from '@/stores/tour'
import { useApiImage } from '@/composables/useApiImage'

const props = defineProps({
  mobile: { type: Boolean, default: false },
  collapsed: { type: Boolean, default: false },
})

const authStore = useAuthStore()
const router = useRouter()
const tour = useTourStore()

// useApiImage: direct URL on web; an axios-fetched blob: URL on desktop (the
// webview can't load the remote '/api/…/avatar' <img> directly).
const avatarUrl = useApiImage(() => authStore.user?.avatar_url || '')
const isAdmin = computed(() => authStore.isAdmin)
function openSettings() {
  router.push('/settings')
}
function openAdmin() {
  router.push('/admin')
}

// The Get Started guide's permanent entry point (#2753): the autostart only ever
// fires once per account, so this is how anyone re-runs it. The first step points
// at the workspace switcher, so the guide is started from Home rather than from
// whatever screen the user happened to be on.
function startTour() {
  router.push('/')
  tour.startGuide()
}

const initials = computed(() => {
  const n = (authStore.user?.name || authStore.user?.email || '?').trim()
  const parts = n.split(/\s+/)
  if (parts.length >= 2) return (parts[0][0] + parts[1][0]).toUpperCase()
  return n.slice(0, 2).toUpperCase()
})

// Only the collapsed desktop rail uses the avatar-popover; the mobile drawer is
// wide enough for the inline name + icon buttons (settings / admin / logout).
const compact = computed(() => props.collapsed)

function logout() {
  authStore.logout()
  router.push('/login')
}
</script>

<template>
  <div class="sb-footer" :class="{ collapsed }" data-tour="sb-footer">
    <n-popover v-if="compact" trigger="click" :placement="collapsed ? 'right-end' : 'top-start'">
      <template #trigger>
        <img v-if="avatarUrl" :src="avatarUrl" class="ava ava-img" alt="" />
        <n-avatar v-else round :size="32" class="ava">{{ initials }}</n-avatar>
      </template>
      <div class="user-pop">
        <div class="up-name">{{ authStore.user?.name || 'Профиль' }}</div>
        <div class="up-mail">{{ authStore.user?.email }}</div>
        <!-- Same anchor on both shapes of the footer (compact popover / full
             row): only one of them is ever in the DOM. -->
        <n-button size="small" block data-tour="footer-settings" @click="openSettings">
          <template #icon><n-icon :component="SettingsOutline" /></template>
          Настройки
        </n-button>
        <n-button size="small" block data-tour="footer-tour" @click="startTour">
          <template #icon><n-icon :component="SchoolOutline" /></template>
          Обучение
        </n-button>
        <n-button v-if="isAdmin" size="small" block @click="openAdmin">
          <template #icon><n-icon :component="ShieldCheckmarkOutline" /></template>
          Администрирование
        </n-button>
        <n-button size="small" block @click="logout">
          <template #icon><n-icon :component="LogOutOutline" /></template>
          Выйти
        </n-button>
      </div>
    </n-popover>
    <div v-else class="user">
      <img v-if="avatarUrl" :src="avatarUrl" class="ava ava-img" alt="" @click="openSettings" />
      <n-avatar v-else round :size="30" class="ava" @click="openSettings">{{ initials }}</n-avatar>
      <span class="uname" @click="openSettings">{{ authStore.user?.name || 'Профиль' }}</span>
      <n-tooltip v-if="isAdmin">
        <template #trigger>
          <n-button
            quaternary
            circle
            size="small"
            aria-label="Администрирование"
            @click="openAdmin"
          >
            <n-icon :component="ShieldCheckmarkOutline" />
          </n-button>
        </template>
        Администрирование
      </n-tooltip>
      <n-tooltip>
        <template #trigger>
          <n-button
            quaternary
            circle
            size="small"
            aria-label="Обучение"
            data-tour="footer-tour"
            @click="startTour"
          >
            <n-icon :component="SchoolOutline" />
          </n-button>
        </template>
        Обучение
      </n-tooltip>
      <n-tooltip>
        <template #trigger>
          <n-button
            quaternary
            circle
            size="small"
            aria-label="Настройки"
            data-tour="footer-settings"
            @click="openSettings"
          >
            <n-icon :component="SettingsOutline" />
          </n-button>
        </template>
        Настройки
      </n-tooltip>
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
  background: var(--t-accent-grad);
  color: var(--t-on-primary);
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
}
.ava-img {
  width: 32px;
  height: 32px;
  border-radius: 50%;
  object-fit: cover;
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
/* Left-align the icon+label inside the block buttons (default is centred). */
.user-pop :deep(.n-button__content) {
  width: 100%;
  justify-content: flex-start;
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
