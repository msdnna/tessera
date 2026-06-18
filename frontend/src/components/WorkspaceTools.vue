<script setup>
import { ref, h } from 'vue'
import {
  NButton,
  NIcon,
  NBadge,
  NPopover,
  NTooltip,
  NSwitch,
  NText,
  NDropdown,
} from 'naive-ui'
import {
  SunnyOutline,
  MoonOutline,
  NotificationsOutline,
  PeopleOutline,
  ColorPaletteOutline,
  ExtensionPuzzleOutline,
  LogoGitlab,
} from '@vicons/ionicons5'
import EmptyState from '@/components/EmptyState.vue'
import { useRouter } from 'vue-router'
import { useThemeStore, COLOR_THEMES } from '@/stores/theme'
import { hueGrad } from '@/utils/gradient'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useNotificationsStore } from '@/stores/notifications'
import { useResponsive } from '@/composables/useResponsive'
import { useOverlayBack } from '@/composables/useOverlayBack'
import MembersModal from './MembersModal.vue'
import GitLabModal from './GitLabModal.vue'

defineProps({ placement: { type: String, default: 'bottom-end' } })

const theme = useThemeStore()
const ws = useWorkspacesStore()
const notes = useNotificationsStore()
const router = useRouter()
// On touch (mobile) tooltips fire on tap and overlap the dropdown/popover they
// label — suppress them there.
const { isMobile } = useResponsive()

const showMembers = ref(false)
const showGitlab = ref(false)
// Browser Back closes these modals instead of leaving the board.
useOverlayBack(showMembers, () => (showMembers.value = false))
useOverlayBack(showGitlab, () => (showGitlab.value = false))
const integrationOptions = [
  { label: 'GitLab', key: 'gitlab', icon: () => h(NIcon, null, { default: () => h(LogoGitlab) }) },
]
function onIntegrationSelect(key) {
  if (key === 'gitlab') showGitlab.value = true
}

function openNotification(n) {
  notes.markRead(n.id)
  // Switch the active workspace if the task lives in another one, so the sidebar
  // tree reflects where the opened task actually is.
  if (n.workspace_id && n.workspace_id !== ws.currentId) {
    ws.selectWorkspace(n.workspace_id)
  }
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
    <n-tooltip :disabled="isMobile">
      <template #trigger>
        <n-button quaternary circle size="small" aria-label="Участники" @click="showMembers = true">
          <n-icon :component="PeopleOutline" />
        </n-button>
      </template>
      Участники
    </n-tooltip>

    <!-- Integrations -->
    <n-dropdown
      trigger="click"
      :placement="placement"
      :options="integrationOptions"
      @select="onIntegrationSelect"
    >
      <n-tooltip :disabled="isMobile">
        <template #trigger>
          <n-button quaternary circle size="small" aria-label="Интеграции">
            <n-icon :component="ExtensionPuzzleOutline" />
          </n-button>
        </template>
        Интеграции
      </n-tooltip>
    </n-dropdown>

    <!-- Notifications -->
    <n-popover trigger="click" :placement="placement">
      <template #trigger>
        <n-button quaternary circle size="small" aria-label="Уведомления" class="bell-btn">
          <n-badge :value="notes.unread" :max="9" :show="notes.unread > 0" class="bell-badge">
            <n-icon :component="NotificationsOutline" />
          </n-badge>
        </n-button>
      </template>
      <div class="feed">
        <div class="feed-head">
          <span>Уведомления</span>
          <n-button
            v-if="notes.unread"
            text
            size="tiny"
            type="primary"
            class="ngrad"
            @click="notes.markAllRead()"
          >
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
        <empty-state
          v-if="!notes.items.length"
          :icon="NotificationsOutline"
          text="Пока тихо"
          size="small"
        />
      </div>
    </n-popover>

    <!-- Appearance -->
    <n-popover trigger="click" :placement="placement">
      <template #trigger>
        <n-tooltip :disabled="isMobile">
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
            :style="{ backgroundImage: hueGrad(t.primary) }"
            :title="t.name"
            @click="theme.selectColor(t)"
          />
        </div>
      </div>
    </n-popover>

    <MembersModal v-model:show="showMembers" :ws-id="ws.currentId" />
    <GitLabModal v-model:show="showGitlab" :ws-id="ws.currentId" />
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
  background: var(--t-accent-grad);
  color: var(--t-on-primary);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  height: 16px;
  min-width: 16px;
  padding: 0 4px;
  box-sizing: border-box;
  font-size: 10px;
  font-weight: 600;
  border-radius: 999px;
}
/* Centre the animated number inside the badge. */
.bell-badge :deep(.n-badge-sup .n-base-slot-machine) {
  line-height: 16px;
  height: 16px;
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
