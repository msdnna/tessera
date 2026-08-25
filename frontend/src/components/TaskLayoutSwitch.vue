<script setup>
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NPopover } from 'naive-ui'
import TesseraIcon from './TesseraIcon.vue'
import { TASK_LAYOUTS } from '@/utils/taskLayout'

// Where to show the task (#2716). Purely a controlled input: the owner (TaskModal)
// holds the state and persists it — this only renders the choice, like
// BoardLayoutSwitch does for board visualizations.
const props = defineProps({
  value: { type: String, default: 'modal' },
})
const emit = defineEmits(['update:value'])

const { t } = useI18n()
// Labels resolved per render, so the menu follows a language change.
const LAYOUTS = [
  { value: 'modal', icon: 'task-modal' },
  { value: 'fullscreen', icon: 'task-fullscreen' },
  { value: 'sidebar', icon: 'task-sidebar' },
]
const opts = computed(() => LAYOUTS.map((o) => ({ ...o, label: t(`task.layout.${o.value}`) })))
const current = () => (TASK_LAYOUTS.includes(props.value) ? props.value : 'modal')
</script>

<template>
  <n-popover trigger="click" placement="bottom-end" :show-arrow="false">
    <template #trigger>
      <button class="head-btn" :title="t('task.layout.hint')" data-testid="task-layout-trigger">
        <TesseraIcon :name="`task-${current()}`" :size="15" />
      </button>
    </template>
    <div class="tls-menu">
      <n-button
        v-for="o in opts"
        :key="o.value"
        text
        size="small"
        class="ngrad tls-item"
        :type="current() === o.value ? 'primary' : 'default'"
        :data-testid="`task-layout-${o.value}`"
        @click="emit('update:value', o.value)"
      >
        <template #icon>
          <TesseraIcon :name="o.icon" :variant="current() === o.value ? 'filled' : 'outline'" />
        </template>
        {{ o.label }}
      </n-button>
    </div>
  </n-popover>
</template>

<style scoped>
.tls-menu {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 10px;
  padding: 2px;
}
.tls-item {
  white-space: nowrap;
}
/* Matches the neighbouring head buttons in TaskModal's header (scoped styles from
   the parent don't reach this child's root). */
.head-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border: none;
  background: transparent;
  color: var(--t-text3);
  border-radius: 6px;
  cursor: pointer;
  transition:
    background 0.12s ease,
    color 0.12s ease;
}
.head-btn:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
</style>
