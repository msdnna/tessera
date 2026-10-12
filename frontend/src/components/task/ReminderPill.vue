<script setup>
// The card's bell pill (#2998, mockup PackageB frame 7): the reader's next
// reminder on this task — after the due pill, with its time. A fired one is
// soft orange and bold. A click opens «Отложить» at the pill. A separate
// component so only the cards that have a reminder pay for the menu and the
// actions; the reminder itself comes from the shell's «Входящие» copy, not a
// request per card.
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import { NotificationsOutline } from '@vicons/ionicons5'
import SnoozeMenu from '../inbox/SnoozeMenu.vue'
import { useInboxStore } from '@/stores/inbox'
import { useInboxActions } from '@/composables/useInboxActions'
import { useInboxFormat } from '@/composables/useInboxFormat'
import { recordState } from '@/utils/inbox'

const props = defineProps({
  reminder: { type: Object, required: true },
  task: { type: Object, required: true },
  stacked: { type: Boolean, default: false },
  readonly: { type: Boolean, default: false },
})
const { t } = useI18n()
const inbox = useInboxStore()
const act = useInboxActions()
const { whenShort } = useInboxFormat()

const el = ref(null)
const menu = ref(false)
const fired = computed(() => recordState(props.reminder, inbox.now) === 'fired')
const label = computed(() => whenShort(props.reminder.remind_at, inbox.now))
</script>

<template>
  <button
    ref="el"
    type="button"
    class="pill set rem-pill"
    :class="{ fired }"
    :title="t('inbox.pill.tip', { when: label })"
    data-testid="card-reminder"
    @click.stop="!readonly && (menu = true)"
  >
    <n-icon :component="NotificationsOutline" :size="13" />
    <span class="pill-text">{{ label }}</span>
    <!-- Inside the button so the pill stays one root (the row's scoped pill
         styles reach a component's root only); the menu teleports to body. -->
    <SnoozeMenu
      v-if="menu"
      :show="menu"
      :anchor="() => el"
      :task="task"
      placement="bottom-start"
      testid="card-snooze"
      @update:show="(v) => (menu = v)"
      @pick="(at) => act.snooze(reminder, at)"
    />
  </button>
</template>

<style scoped>
.pill.rem-pill.fired {
  background: linear-gradient(
    to top right,
    color-mix(in srgb, #f0a020 11%, var(--card-fill, var(--t-card))),
    color-mix(in srgb, #f0a020 19%, var(--card-fill, var(--t-card)))
  );
  color: #f0a020;
  font-weight: 600;
}
</style>
