<script setup>
// «История» tab of the task modal: the task's journal, newest entries as the
// backend ordered them. Pure presentation — the modal owns loading.
import { useI18n } from 'vue-i18n'
import { TimeOutline } from '@vicons/ionicons5'
import { fmtWhen, eventText } from '@/utils/taskFeed'
import { useFormat } from '@/composables/useFormat'
import UserAvatar from '../UserAvatar.vue'
import EmptyState from '../EmptyState.vue'

defineProps({
  events: { type: Array, default: () => [] },
})

const { t } = useI18n()
const { formatters } = useFormat()
</script>

<template>
  <div class="history">
    <div v-for="e in events" :key="e.id" class="histrow">
      <UserAvatar class="h-ava" :user-id="e.actor_id" :name="e.actor_name" />
      <span class="h-text">
        <b>{{ e.actor_name || t('task.history.someone') }}</b> {{ eventText(e) }}
      </span>
      <span class="h-when">{{ fmtWhen(e.created_at, formatters) }}</span>
    </div>
    <EmptyState
      v-if="!events.length"
      size="small"
      :icon="TimeOutline"
      :text="t('task.history.empty')"
    />
  </div>
</template>

<style scoped>
.history {
  display: flex;
  flex-direction: column;
  gap: 8px;
  max-height: 280px;
  overflow-y: auto;
}
.histrow {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
}
.h-ava {
  flex: none;
  width: 24px;
  height: 24px;
  border-radius: 50%;
  background: var(--t-accent-grad);
  color: var(--t-on-primary);
  font-size: 10px;
  font-weight: 600;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}
.h-text {
  flex: 1;
  color: var(--t-text2);
}
.h-text b {
  color: var(--t-text1);
}
.h-when {
  font-size: 11px;
  color: var(--t-text3);
  flex: none;
}
</style>
