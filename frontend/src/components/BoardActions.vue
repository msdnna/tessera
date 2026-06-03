<script setup>
import { NButton, NPopover, NIcon } from 'naive-ui'
import { PricetagsOutline, ArchiveOutline } from '@vicons/ionicons5'
import { useBoardViewStore } from '@/stores/boardView'
import TagManager from './TagManager.vue'
import ArchiveModal from './ArchiveModal.vue'

const store = useBoardViewStore()
// Tag/archive changes happen outside the board component now, so nudge it to
// reload via the shared store.
function onChanged() {
  store.bumpReload()
}
</script>

<template>
  <div class="board-actions">
    <n-popover trigger="click" placement="bottom-end">
      <template #trigger>
        <n-button size="small" quaternary>
          <template #icon><n-icon :component="PricetagsOutline" /></template>
          Теги
        </n-button>
      </template>
      <TagManager :ws-id="store.wsId" :tags="store.tagsList" @changed="onChanged" />
    </n-popover>

    <n-button size="small" quaternary @click="store.archiveOpen = true">
      <template #icon><n-icon :component="ArchiveOutline" /></template>
      Архив
    </n-button>

    <ArchiveModal v-model:show="store.archiveOpen" :board-id="store.boardId" @changed="onChanged" />
  </div>
</template>

<style scoped>
.board-actions {
  display: flex;
  align-items: center;
  gap: 4px;
}
</style>
