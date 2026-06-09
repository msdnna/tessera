<script setup>
import { ref } from 'vue'
import { NButton, NIcon, NBadge, NPopover, NTooltip, NSwitch, NText, NEmpty } from 'naive-ui'
import {
  SunnyOutline,
  MoonOutline,
  NotificationsOutline,
  PeopleOutline,
  ColorPaletteOutline,
} from '@vicons/ionicons5'
import { useRouter } from 'vue-router'
import { useThemeStore, COLOR_THEMES } from '@/stores/theme'
import { hueGrad } from '@/utils/gradient'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useNotificationsStore } from '@/stores/notifications'
import MembersModal from './MembersModal.vue'

defineProps({ placement: { type: String, default: 'bottom-end' } })

const theme = useThemeStore()
const ws = useWorkspacesStore()
const notes = useNotificationsStore()
const router = useRouter()

const showMembers = ref(false)

function openNotification(n) {
  notes.markRead(n.id)
  if (n.task_id && n.task_board_id) {
    router.push(`/board/${n.task_board_id}?task=${n.task_id}`)
  }
}
function fmtTime(d) {
  return new Date(d).toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' })
}
</script>

<template>
  <div class="tools">
    <!-- Members -->
    <n-tooltip>
      <template #trigger>
        <n-button quaternary circle size="small" aria-label="Участники" @click="showMembers = true">
          <n-icon :component="PeopleOutline" />
        </n-button>
      </template>
      Участники
    </n-tooltip>

    <!-- Notifications -->
    <n-popover trigger="click" :placement="placement">
      <template #trigger>
        <n-button quaternary circle size="small" aria-label="Уведомления" class="bell-btn">
          <n-badge
            :value="notes.unread"
            :max="9"
            :show="notes.unread > 0"
            class="bell-badge"
          >
            <n-icon :component="NotificationsOutline" />
          </n-badge>
        </n-button>
      </template>
      <div class="feed">
        <div class="feed-head">
          <span>Уведомления</span>
          <n-button v-if="notes.unread" text size="tiny" type="primary" class="ngrad" @click="notes.markAllRead()">
            Прочитать все
          </n-button>
        </div>
        <button
          v-for="it in notes.items"
          :key="it.id"
          class="feed-item"
          :class="{ unread: !it.read_at }"
          @click="openNotification(it)"
        >
          <span class="ft">{{ it.text }}</span>
          <span class="fa">{{ fmtTime(it.created_at) }}</span>
        </button>
        <n-empty v-if="!notes.items.length" description="Пока тихо" size="small" />
      </div>
    </n-popover>

    <!-- Appearance -->
    <n-popover trigger="click" :placement="placement">
      <template #trigger>
        <n-tooltip>
          <template #trigger>
            <n-button quaternary circle size="small" aria-label="Оформление">
              <n-icon :component="ColorPaletteOutline" />
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
            :style="{ background: hueGrad(t.primary) }"
            :title="t.name"
            @click="theme.selectColor(t)"
          />
        </div>
      </div>
    </n-popover>

    <MembersModal v-model:show="showMembers" :ws-id="ws.currentId" />
  </div>
</template>

<style scoped>
.tools {
  display: inline-flex;
  align-items: center;
  gap: 2px;
}
/* Smaller notification badge so it doesn't rival the bell icon's size.
   Equal min-width/height + a pill radius keeps it a clean circle for a single
   digit (and a neat pill for "9+"). */
.bell-badge :deep(.n-badge-sup) {
  height: 16px;
  min-width: 16px;
  padding: 0 4px;
  box-sizing: border-box;
  font-size: 10px;
  line-height: 16px;
  border-radius: 999px;
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
  appearance: none;
  -webkit-appearance: none;
  width: 24px;
  height: 24px;
  border-radius: 50%;
  border: 2px solid transparent;
  background-origin: border-box;
  cursor: pointer;
  padding: 0;
}
.swatch-btn.active {
  border-color: var(--t-text1);
  box-shadow: 0 0 0 2px var(--t-surface);
}
.feed {
  width: 280px;
  max-height: 360px;
  overflow-y: auto;
}
.feed-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 12px;
  color: var(--t-text3);
  text-transform: uppercase;
  letter-spacing: 0.4px;
  padding: 2px 6px 6px;
}
.feed-item {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 8px;
  width: 100%;
  text-align: left;
  border: none;
  background: none;
  cursor: pointer;
  padding: 8px 6px;
  border-radius: 6px;
  border-bottom: 1px solid var(--t-border);
}
.feed-item:hover {
  background: var(--t-hover);
}
.feed-item:last-child {
  border-bottom: none;
}
.feed-item.unread {
  background: color-mix(in srgb, var(--t-primary) 10%, transparent);
}
.feed-item.unread .ft {
  font-weight: 600;
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
