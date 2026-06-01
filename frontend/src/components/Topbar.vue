<script setup>
import { ref } from 'vue'
import {
  NButton,
  NSpace,
  NText,
  NDropdown,
  NPopover,
  NSwitch,
  NTooltip,
  NIcon,
  NBadge,
  NEmpty,
} from 'naive-ui'
import {
  MenuOutline,
  SunnyOutline,
  MoonOutline,
  NotificationsOutline,
  PeopleOutline,
} from '@vicons/ionicons5'
import { useRouter } from 'vue-router'
import { useThemeStore, COLOR_THEMES } from '@/stores/theme'
import { useAuthStore } from '@/stores/auth'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useActivityStore } from '@/stores/activity'
import MembersModal from './MembersModal.vue'

defineProps({ mobile: { type: Boolean, default: false } })
defineEmits(['menu'])

const theme = useThemeStore()
const authStore = useAuthStore()
const ws = useWorkspacesStore()
const activity = useActivityStore()
const router = useRouter()

const showMembers = ref(false)

const userOptions = [{ label: 'Выйти', key: 'logout' }]
function onUserSelect(key) {
  if (key === 'logout') {
    authStore.logout()
    router.push('/login')
  }
}

function fmtTime(d) {
  return new Date(d).toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' })
}
</script>

<template>
  <div class="topbar">
    <div class="left">
      <n-button v-if="mobile" quaternary circle @click="$emit('menu')">
        <n-icon :component="MenuOutline" />
      </n-button>
      <n-text strong>{{ ws.current?.name || 'Tessera' }}</n-text>
    </div>

    <n-space align="center" :size="6">
      <!-- Members -->
      <n-tooltip>
        <template #trigger>
          <n-button quaternary circle aria-label="Участники" @click="showMembers = true">
            <n-icon :component="PeopleOutline" />
          </n-button>
        </template>
        Участники
      </n-tooltip>

      <!-- Activity bell -->
      <n-popover
        trigger="click"
        placement="bottom-end"
        @update:show="(s) => s && activity.markRead()"
      >
        <template #trigger>
          <n-badge :value="activity.unread" :max="9" :show="activity.unread > 0">
            <n-button quaternary circle aria-label="Активность">
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
      <n-popover trigger="click" placement="bottom-end">
        <template #trigger>
          <n-tooltip>
            <template #trigger>
              <n-button quaternary circle aria-label="Оформление">
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

      <n-dropdown trigger="click" :options="userOptions" @select="onUserSelect">
        <n-button quaternary>{{ authStore.user?.name || 'Профиль' }}</n-button>
      </n-dropdown>
    </n-space>

    <MembersModal v-model:show="showMembers" :ws-id="ws.currentId" />
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
.left {
  display: flex;
  align-items: center;
  gap: 8px;
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
</style>
