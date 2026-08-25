<script setup>
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton } from 'naive-ui'
import { useBoardViewStore } from '@/stores/boardView'
import TesseraIcon from './TesseraIcon.vue'

const store = useBoardViewStore()
const { t } = useI18n()
// Tessera icon-pack names per visualization (filled when active for emphasis).
// Labels are resolved per render so the switcher follows a language change.
const LAYOUTS = [
  { value: 'board', icon: 'layout-kanban' },
  { value: 'list', icon: 'layout-list' },
  { value: 'calendar', icon: 'layout-calendar' },
  { value: 'timeline', icon: 'layout-timeline' },
  { value: 'gantt', icon: 'layout-gantt' },
  { value: 'matrix', icon: 'layout-matrix' },
]
const opts = computed(() => LAYOUTS.map((o) => ({ ...o, label: t(`board.layout.${o.value}`) })))
</script>

<template>
  <div class="layout-switch" data-tour="board-layout">
    <n-button
      v-for="o in opts"
      :key="o.value"
      text
      size="small"
      class="ngrad"
      :type="store.layout === o.value ? 'primary' : 'default'"
      @click="store.layout = o.value"
    >
      <template #icon>
        <TesseraIcon :name="o.icon" :variant="store.layout === o.value ? 'filled' : 'outline'" />
      </template>
      {{ o.label }}
    </n-button>
  </div>
</template>

<style scoped>
.layout-switch {
  display: flex;
  align-items: center;
  gap: 14px;
}
</style>
