<script setup>
// «Подписаны» at the foot of the task window's panel (#2963, TaskModal.dc): the
// cascade of who hears about the task and a text button to follow / unfollow.
// The author, the assignees and the commenters follow implicitly — the list
// comes computed from the server (GET /tasks/:id → subscribers, subscribed).
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { NTooltip } from 'naive-ui'
import { tasks as tasksApi } from '@/api'
import { useMessage } from '@/composables/useMessage'
import { humanizeError } from '@/utils/errors'
import UserAvatar from '@/components/UserAvatar.vue'

const props = defineProps({
  taskId: { type: String, required: true },
  subscribers: { type: Array, default: () => [] },
  subscribed: { type: Boolean, default: false },
})
const emit = defineEmits(['update'])

const { t } = useI18n()
const message = useMessage()
const busy = ref(false)

const SHOWN = 5
const shown = computed(() => props.subscribers.slice(0, SHOWN))
const extra = computed(() => Math.max(0, props.subscribers.length - SHOWN))
const names = computed(() => props.subscribers.map((u) => u.name || u.email).join(', '))

async function toggle() {
  if (busy.value) return
  busy.value = true
  try {
    const { data } = await tasksApi.setSubscription(props.taskId, !props.subscribed)
    emit('update', { subscribers: data.subscribers || [], subscribed: !!data.subscribed })
  } catch (e) {
    message.error(`${t('task.subscribers.failed')}: ${humanizeError(e)}`)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="tsub" data-testid="task-subscribers">
    <n-tooltip :delay="400" :style="{ maxWidth: '280px' }">
      <template #trigger>
        <span class="tsub-label">{{ t('task.subscribers.title') }}</span>
      </template>
      {{ t('task.subscribers.hint') }}
    </n-tooltip>
    <span v-if="subscribers.length" class="tsub-cascade" :title="names">
      <UserAvatar
        v-for="u in shown"
        :key="u.id"
        class="tsub-ava"
        :user-id="u.id"
        :name="u.name || u.email"
        data-testid="task-subscriber"
      />
      <span v-if="extra" class="tsub-more">{{ t('task.subscribers.more', { n: extra }) }}</span>
    </span>
    <span v-else class="tsub-none">{{ t('task.subscribers.nobody') }}</span>
    <span class="tsub-grow" />
    <button
      type="button"
      class="tsub-btn"
      :disabled="busy"
      data-testid="task-subscribe-toggle"
      @click="toggle"
    >
      {{ subscribed ? t('task.subscribers.unsubscribe') : t('task.subscribers.subscribe') }}
    </button>
  </div>
</template>

<style scoped>
/* TaskModal.dc: 12.5px text3, avatars 20 overlapping by 6 with a 2px ring of
   the panel's surface, the action as plain text2 on the right. */
.tsub {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: auto;
  padding: 10px 8px 0;
  font-size: 12.5px;
  color: var(--t-text3);
}
.tsub-label {
  cursor: default;
}
.tsub-cascade {
  display: flex;
  align-items: center;
  padding-left: 6px;
}
.tsub-ava {
  flex: none;
  width: 20px;
  height: 20px;
  margin-left: -6px;
  border-radius: 50%;
  box-shadow: 0 0 0 2px var(--t-sub);
  /* Accent-gradient like every avatar of the panel (CLAUDE.md «Accent-gradient»). */
  background: var(--t-accent-grad);
  color: var(--t-on-primary);
  font-size: 8px;
  font-weight: 600;
}
.tsub-more {
  margin-left: 6px;
}
.tsub-grow {
  flex: 1;
}
.tsub-btn {
  border: 0;
  padding: 2px 4px;
  border-radius: 6px;
  background: transparent;
  color: var(--t-text2);
  font: inherit;
  cursor: pointer;
}
.tsub-btn:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.tsub-btn:focus-visible {
  outline: 2px solid var(--t-primary);
  outline-offset: 1px;
}
.tsub-btn:disabled {
  opacity: 0.6;
  cursor: default;
}
</style>
