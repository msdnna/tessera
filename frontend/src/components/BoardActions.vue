<script setup>
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { NButton, NPopover, NIcon } from 'naive-ui'
import { PricetagsOutline, ArchiveOutline, RibbonOutline } from '@vicons/ionicons5'
import { useBoardViewStore } from '@/stores/boardView'
import { useWorkspacesStore } from '@/stores/workspaces'
import { archiveQuery, withoutArchiveQuery, isArchiveQuery } from '@/utils/boardScope'
import TagManager from './TagManager.vue'
import MilestoneManager from './MilestoneManager.vue'

const { t } = useI18n()
const store = useBoardViewStore()
const wsStore = useWorkspacesStore()
const route = useRoute()
const router = useRouter()
const msShow = ref(false)
// Highlight «Архив» while the board is in the read-only archive scope.
const isArchive = computed(() => isArchiveQuery(route.query))

// Archive is a read-only scope of the board itself (?archived=1) — reuses all
// filters/grouping — so the button is a toggle: the pressed look above is the
// scope being on, and pressing it again has to turn it off (utils/boardScope).
// Leaving replaces rather than pushes, same as the board's own scope chip: the
// archive should not become a step in the back history of the board it overlays.
function toggleArchive() {
  if (isArchive.value) router.replace({ query: withoutArchiveQuery(route.query) })
  else router.push({ query: archiveQuery(route.query) })
}
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
  <div class="board-actions" data-tour="board-actions">
    <n-popover trigger="click" placement="bottom-end">
      <template #trigger>
        <n-button size="small" quaternary>
          <template #icon><n-icon :component="PricetagsOutline" /></template>
          {{ t('board.actions.tags') }}
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
      {{ t('board.actions.milestones') }}
    </n-button>

    <n-button
      size="small"
      :quaternary="!isArchive"
      :secondary="isArchive"
      :type="isArchive ? 'primary' : undefined"
      :title="isArchive ? t('board.scope.archiveExit') : t('board.actions.archive')"
      data-testid="board-archive"
      @click="toggleArchive"
    >
      <template #icon><n-icon :component="ArchiveOutline" /></template>
      {{ t('board.actions.archive') }}
    </n-button>

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
