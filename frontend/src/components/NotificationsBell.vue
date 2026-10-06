<script setup>
// The notification bell (v2): in the header on every screen, as in the mockup.
// It used to sit in the tool row under the workspace name; the feed spans every
// workspace (opening an item may switch to another one), so it belongs to the
// shell rather than to the current workspace.
import { ref, watch } from 'vue'
import { NButton, NIcon, NBadge, NPopover, useMessage } from 'naive-ui'
import { NotificationsOutline } from '@vicons/ionicons5'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import EmptyState from '@/components/EmptyState.vue'
import { notificationText } from '@/utils/notificationText'
import { notificationRoute } from '@/utils/notificationRoute'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useNotificationsStore } from '@/stores/notifications'
import { useFormat } from '@/composables/useFormat'

defineProps({ placement: { type: String, default: 'bottom-end' } })

const ws = useWorkspacesStore()
const notes = useNotificationsStore()
const router = useRouter()
const message = useMessage()
const { formatTime, formatters } = useFormat()
const { t, te } = useI18n()

// Bound so opening a notification can close the feed before navigating — a
// conference invite (#2875) opens a whole page the popover would otherwise sit
// on top of. `g i` and other screens ask for the feed through `feedRequest`.
const showFeed = ref(false)
watch(
  () => notes.feedRequest,
  () => (showFeed.value = true),
)

function openNotification(n) {
  // The toast travels as an argument because the store cannot reach useMessage()
  // (see stores/notifications.js): a refused mark rolls the row back to unread,
  // and the reader is told why instead of watching the dot reappear (#2932).
  notes.markRead(n.id, message)
  // Switch the active workspace if the task lives in another one, so the sidebar
  // tree reflects where the opened task actually is.
  if (n.workspace_id && n.workspace_id !== ws.currentId) {
    ws.selectWorkspace(n.workspace_id)
  }
  const to = notificationRoute(n)
  if (!to) return
  showFeed.value = false
  router.push(to)
}
// The feed line is rendered from the notification's payload, so switching the
// language re-renders the whole feed — including rows fetched minutes ago.
function noteText(n) {
  return notificationText(n, { t, te, formatters: formatters.value })
}
</script>

<template>
  <n-popover v-model:show="showFeed" trigger="click" :placement="placement">
    <template #trigger>
      <n-button
        quaternary
        circle
        :aria-label="t('shell.tools.notifications')"
        class="bell-btn"
        data-testid="notifications-bell"
        data-tour="footer-notifications"
      >
        <n-badge :value="notes.unread" :max="9" :show="notes.unread > 0" class="bell-badge">
          <n-icon :component="NotificationsOutline" :size="18" />
        </n-badge>
      </n-button>
    </template>
    <div class="feed">
      <div class="feed-head">
        <span>{{ t('shell.tools.notifications') }}</span>
        <n-button
          v-if="notes.unread"
          text
          size="tiny"
          type="primary"
          class="ngrad"
          @click="notes.markAllRead(message)"
        >
          {{ t('shell.tools.markAllRead') }}
        </n-button>
      </div>
      <button
        v-for="it in notes.items"
        :key="it.id"
        class="feed-item"
        :class="{ unread: !it.read_at }"
        @click="openNotification(it)"
      >
        <span class="ft">{{ noteText(it) }}</span>
        <span class="fa">{{ formatTime(it.created_at) }}</span>
      </button>
      <empty-state
        v-if="!notes.items.length"
        :icon="NotificationsOutline"
        :text="t('shell.tools.feedEmpty')"
        size="small"
      />
    </div>
  </n-popover>
</template>

<style scoped>
.bell-btn {
  flex: none;
  color: var(--t-text2);
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
/* v2: rows are separated by space and the hover level, not by lines. */
.feed {
  width: 300px;
  max-height: 380px;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.feed-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 11.5px;
  font-weight: 600;
  color: var(--t-text3);
  text-transform: uppercase;
  letter-spacing: 0.05em;
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
  padding: 8px;
  border-radius: 7px;
  font: inherit;
}
.feed-item:hover {
  background: var(--t-hover);
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
