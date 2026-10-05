<script setup>
// The start screen's right column (v2, #2960): today's reminders, the week
// (my tasks due each day — done stacked under what is still open), and the
// newest notifications.
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { NIcon } from 'naive-ui'
import { AlarmOutline, BarChartOutline, FileTrayOutline } from '@vicons/ionicons5'
import { useFormat } from '@/composables/useFormat'
import { useNotificationsStore } from '@/stores/notifications'
import { useWorkspacesStore } from '@/stores/workspaces'
import { notificationText } from '@/utils/notificationText'
import { notificationRoute } from '@/utils/notificationRoute'
import ShortcutKeys from '../ShortcutKeys.vue'

const props = defineProps({
  // Today's open reminders, soonest first.
  reminders: { type: Array, default: () => [] },
  // weekBars(): { days: [{ ms, done, open, today }], done, total }
  week: { type: Object, default: () => ({ days: [], done: 0, total: 0 }) },
})

const { t, te } = useI18n()
const router = useRouter()
const { formatTime, formatDate, formatRelative, formatters } = useFormat()
const notes = useNotificationsStore()
const ws = useWorkspacesStore()

const nextId = computed(() => props.reminders.find((r) => Date.parse(r.remind_at) > Date.now())?.id)
const inbox = computed(() => notes.items.slice(0, 4))
const maxDay = computed(() => Math.max(1, ...props.week.days.map((d) => d.done + d.open)))
const H = 84 // px of a day's column
const barH = (n) => (n ? Math.max(8, Math.round((n / maxDay.value) * H)) : 0)

function openNote(n) {
  notes.markRead(n.id)
  if (n.workspace_id && n.workspace_id !== ws.currentId) ws.selectWorkspace(n.workspace_id)
  const to = notificationRoute(n)
  if (to) router.push(to)
}
const noteText = (n) => notificationText(n, { t, te, formatters: formatters.value })
</script>

<template>
  <div class="hs">
    <section class="hs-card">
      <div class="hs-head">
        <n-icon :component="AlarmOutline" :size="15" class="hs-ico" />
        <span class="hs-title">{{ t('shell.home.side.reminders') }}</span>
        <ShortcutKeys id="create.reminder" />
      </div>
      <div
        v-for="r in reminders"
        :key="r.id"
        class="hs-row"
        :class="{ next: r.id === nextId }"
        @click="router.push('/reminders')"
      >
        <span class="hs-time" :class="{ 'accent-grad-text': r.id === nextId }">{{
          formatTime(r.remind_at)
        }}</span>
        <span class="hs-text">{{ r.message }}</span>
        <span v-if="r.id === nextId" class="hs-when">{{ formatRelative(r.remind_at) }}</span>
      </div>
      <div v-if="!reminders.length" class="hs-empty">{{ t('shell.home.side.noReminders') }}</div>
    </section>

    <section class="hs-card">
      <div class="hs-head">
        <n-icon :component="BarChartOutline" :size="15" class="hs-ico" />
        <span class="hs-title">{{ t('shell.home.side.week') }}</span>
        <span class="hs-meta">{{
          t('shell.home.side.weekDone', { done: week.done, total: week.total })
        }}</span>
      </div>
      <div class="hs-week">
        <div v-for="d in week.days" :key="d.ms" class="hs-day" :class="{ today: d.today }">
          <div class="hs-col" :style="{ height: `${H}px` }">
            <div v-if="d.open" class="hs-bar open" :style="{ height: `${barH(d.open)}px` }" />
            <div v-if="d.done" class="hs-bar done" :style="{ height: `${barH(d.done)}px` }" />
          </div>
          <span class="hs-dow">{{ formatDate(d.ms, { weekday: 'short', timeZone: null }) }}</span>
        </div>
      </div>
    </section>

    <section class="hs-card">
      <div class="hs-head">
        <n-icon :component="FileTrayOutline" :size="15" class="hs-ico" />
        <span class="hs-title">{{ t('shell.home.side.inbox') }}</span>
        <span v-if="notes.unread" class="hs-badge">{{
          notes.unread > 99 ? '99+' : notes.unread
        }}</span>
      </div>
      <div
        v-for="n in inbox"
        :key="n.id"
        class="hs-row hs-note"
        :class="{ unread: !n.read_at }"
        @click="openNote(n)"
      >
        <span class="hs-text">{{ noteText(n) }}</span>
        <span class="hs-when">{{ formatRelative(n.created_at) }}</span>
      </div>
      <div v-if="!inbox.length" class="hs-empty">{{ t('shell.home.side.inboxEmpty') }}</div>
    </section>
  </div>
</template>

<style scoped>
.hs {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.hs-card {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 12px 12px 10px;
  border-radius: 16px;
  background: var(--t-col);
}
.hs-head {
  display: flex;
  align-items: center;
  gap: 8px;
  height: 24px;
  padding: 0 4px;
  font-size: 13px;
  font-weight: 650;
}
.hs-ico {
  color: var(--t-text2);
}
.hs-title {
  flex: 1;
}
.hs-meta {
  font-size: 12px;
  font-weight: 500;
  color: var(--t-text3);
}
.hs-badge {
  min-width: 20px;
  height: 18px;
  padding: 0 6px;
  box-sizing: border-box;
  border-radius: 9px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: 11px;
  font-weight: 700;
  color: #fff;
  background: var(--t-accent-grad);
}
.hs-row {
  display: flex;
  align-items: center;
  gap: 10px;
  min-height: 38px;
  padding: 0 6px;
  border-radius: 9px;
  cursor: pointer;
}
.hs-row:hover {
  background: var(--t-hover);
}
.hs-time {
  flex: 0 0 44px;
  font-size: 13px;
  font-weight: 600;
  color: var(--t-text2);
  font-variant-numeric: tabular-nums;
}
.hs-text {
  flex: 1;
  min-width: 0;
  font-size: 13px;
  line-height: 1.4;
  color: var(--t-text2);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.hs-row.next .hs-text,
.hs-note.unread .hs-text {
  color: var(--t-text1);
}
.hs-note {
  min-height: 40px;
}
.hs-note .hs-text {
  white-space: normal;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
}
.hs-when {
  flex: none;
  font-size: 11.5px;
  color: var(--t-text3);
  white-space: nowrap;
}
.hs-empty {
  padding: 6px;
  font-size: 12.5px;
  color: var(--t-text3);
}
.hs-week {
  display: flex;
  align-items: flex-end;
  gap: 4px;
  padding: 6px 4px 2px;
}
.hs-day {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
}
.hs-col {
  width: 22px;
  display: flex;
  flex-direction: column;
  justify-content: flex-end;
  gap: 2px;
}
.hs-bar {
  border-radius: 5px;
}
.hs-bar.open {
  background: color-mix(in srgb, var(--t-text3) 26%, var(--t-col));
}
.hs-bar.done {
  background: color-mix(in srgb, #18a058 38%, var(--t-col));
}
.hs-day.today .hs-bar.open {
  background: var(--t-accent-grad);
}
.hs-day.today .hs-bar.done {
  background: color-mix(in srgb, #18a058 55%, var(--t-col));
}
.hs-dow {
  font-size: 11.5px;
  color: var(--t-text3);
}
.hs-day.today .hs-dow {
  font-weight: 700;
  color: var(--t-text1);
}
</style>
