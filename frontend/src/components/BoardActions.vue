<script setup>
import { ref, computed } from 'vue'
import { NButton, NPopover, NIcon } from 'naive-ui'
import { PricetagsOutline, ArchiveOutline, RibbonOutline } from '@vicons/ionicons5'
import { useBoardViewStore } from '@/stores/boardView'
import { useWorkspacesStore } from '@/stores/workspaces'
import TagManager from './TagManager.vue'
import ArchiveModal from './ArchiveModal.vue'
import MilestoneManager from './MilestoneManager.vue'

const store = useBoardViewStore()
const wsStore = useWorkspacesStore()
const msShow = ref(false)
const projectName = computed(
  () => wsStore.projects.find((p) => p.id === store.projectId)?.name || '',
)
// Tag/archive/milestone changes happen outside the board component now, so nudge
// it to reload via the shared store.
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
      <TagManager
        :project-id="store.projectId"
        :tags="store.tagsList"
        :prefix-names="store.prefixNames"
        @changed="onChanged"
      />
    </n-popover>

    <n-button size="small" quaternary @click="msShow = true">
      <template #icon><n-icon :component="RibbonOutline" /></template>
      Этапы
    </n-button>

    <n-button size="small" quaternary @click="store.archiveOpen = true">
      <template #icon><n-icon :component="ArchiveOutline" /></template>
      Архив
    </n-button>

    <ArchiveModal v-model:show="store.archiveOpen" :board-id="store.boardId" @changed="onChanged" />
    <MilestoneManager
      v-model:show="msShow"
      :project-id="store.projectId"
      :project-name="projectName"
      :ws-id="store.wsId"
      @changed="onChanged"
    />
  </div>
</template>

<style scoped>
.board-actions {
  display: flex;
  align-items: center;
  gap: 4px;
}
</style>
