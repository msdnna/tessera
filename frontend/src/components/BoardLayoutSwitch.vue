<script setup>
import { NButton } from 'naive-ui'
import { useBoardViewStore } from '@/stores/boardView'
import TesseraIcon from './TesseraIcon.vue'

const store = useBoardViewStore()
// Tessera icon-pack names per visualization (filled when active for emphasis).
const opts = [
  { value: 'board', label: 'Доска', icon: 'layout-kanban' },
  { value: 'list', label: 'Список', icon: 'layout-list' },
  { value: 'calendar', label: 'Календарь', icon: 'layout-calendar' },
  { value: 'timeline', label: 'Таймлайн', icon: 'layout-timeline' },
  { value: 'gantt', label: 'Гант', icon: 'layout-gantt' },
  { value: 'matrix', label: 'Матрица', icon: 'layout-matrix' },
]
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
