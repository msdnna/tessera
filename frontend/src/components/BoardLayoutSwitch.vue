<script setup>
import { NButton, NIcon } from 'naive-ui'
import {
  GridOutline,
  ListOutline,
  CalendarClearOutline,
  AppsOutline,
  BarChartOutline,
  GitNetworkOutline,
} from '@vicons/ionicons5'
import { useBoardViewStore } from '@/stores/boardView'

const store = useBoardViewStore()
const opts = [
  { value: 'board', label: 'Доска', icon: GridOutline },
  { value: 'list', label: 'Список', icon: ListOutline },
  { value: 'calendar', label: 'Календарь', icon: CalendarClearOutline },
  { value: 'timeline', label: 'Таймлайн', icon: BarChartOutline },
  { value: 'gantt', label: 'Гант', icon: GitNetworkOutline },
  { value: 'matrix', label: 'Матрица', icon: AppsOutline },
]
</script>

<template>
  <div class="layout-switch">
    <n-button
      v-for="o in opts"
      :key="o.value"
      text
      size="small"
      class="ngrad"
      :type="store.layout === o.value ? 'primary' : 'default'"
      @click="store.layout = o.value"
    >
      <template #icon><n-icon :component="o.icon" /></template>
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
