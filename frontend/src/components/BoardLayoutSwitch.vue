<script setup>
import { NButton, NButtonGroup, NIcon, NTooltip } from 'naive-ui'
import { GridOutline, ListOutline, CalendarClearOutline } from '@vicons/ionicons5'
import { useBoardViewStore } from '@/stores/boardView'

const store = useBoardViewStore()
const opts = [
  { value: 'board', label: 'Доска', icon: GridOutline },
  { value: 'list', label: 'Список', icon: ListOutline },
  { value: 'calendar', label: 'Календарь', icon: CalendarClearOutline },
]
</script>

<template>
  <n-button-group size="small">
    <n-tooltip v-for="o in opts" :key="o.value">
      <template #trigger>
        <n-button
          :type="store.layout === o.value ? 'primary' : 'default'"
          @click="store.layout = o.value"
        >
          <template #icon><n-icon :component="o.icon" /></template>
        </n-button>
      </template>
      {{ o.label }}
    </n-tooltip>
  </n-button-group>
</template>
