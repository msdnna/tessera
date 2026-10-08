<script setup>
// The notification bell (v2, #3107 — PackageE «NotifPop»): in the header on
// every screen, with the unread count. On a desktop it opens the feed
// (NotifFeed) as a 420px popover under itself; on a phone (#2976 — package G)
// the feed is a page of its own, /notifications, so the system Back leaves it.
//
// `G N` and other screens ask for the feed through the store's `feedRequest`.
import { ref, watch } from 'vue'
import { NIcon, NPopover } from 'naive-ui'
import { NotificationsOutline } from '@vicons/ionicons5'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import NotifFeed from './NotifFeed.vue'
import { useNotificationsStore } from '@/stores/notifications'
import { useResponsive } from '@/composables/useResponsive'

defineProps({ placement: { type: String, default: 'bottom-end' } })

const notes = useNotificationsStore()
const router = useRouter()
const { t } = useI18n()
const { isMobile } = useResponsive()

const showFeed = ref(false)
function openPage() {
  if (router.currentRoute.value.path !== '/notifications') router.push('/notifications')
}
watch(
  () => notes.feedRequest,
  () => (isMobile.value ? openPage() : (showFeed.value = true)),
)
function onTrigger() {
  if (isMobile.value) openPage()
}
</script>

<template>
  <n-popover
    :show="showFeed && !isMobile"
    :trigger="isMobile ? 'manual' : 'click'"
    :placement="placement"
    :show-arrow="false"
    raw
    class="np-pop"
    @update:show="(v) => (showFeed = v)"
  >
    <template #trigger>
      <button
        type="button"
        class="bell-btn"
        :class="{ on: showFeed && !isMobile, touch: isMobile }"
        :aria-label="t('shell.tools.notifications')"
        data-testid="notifications-bell"
        data-tour="footer-notifications"
        @click="onTrigger"
      >
        <n-icon :component="NotificationsOutline" :size="isMobile ? 22 : 18" />
        <span v-if="notes.unread" class="bell-n" data-testid="notifications-unread">{{
          notes.unread > 9 ? '9+' : notes.unread
        }}</span>
      </button>
    </template>

    <NotifFeed @close="showFeed = false" />
  </n-popover>
</template>

<style scoped>
/* The bell: a quiet header button like «Входящие» next to it; the count is the
   accent gradient. */
.bell-btn {
  position: relative;
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 34px;
  height: 34px;
  border: 0;
  border-radius: 10px;
  background: transparent;
  color: var(--t-text2);
  cursor: pointer;
}
.bell-btn:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.bell-btn.on {
  background: color-mix(in srgb, var(--t-primary) 18%, transparent);
  color: var(--t-primary);
}
.bell-btn:focus-visible {
  outline: 2px solid var(--t-primary);
  outline-offset: 1px;
}
.bell-n {
  position: absolute;
  top: 3px;
  right: 2px;
  min-width: 16px;
  height: 16px;
  padding: 0 4px;
  box-sizing: border-box;
  border-radius: 999px;
  background: var(--t-accent-grad);
  color: #fff;
  font-size: 10px;
  font-weight: 700;
  line-height: 16px;
  text-align: center;
  pointer-events: none;
}

/* A phone: a 44px target. */
.bell-btn.touch {
  width: 44px;
  height: 44px;
  border-radius: 12px;
}
.bell-btn.touch .bell-n {
  top: 6px;
  right: 6px;
}
</style>
