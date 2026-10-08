<script setup>
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { NButton, NIcon } from 'naive-ui'
import { PricetagsOutline, ArchiveOutline, TrailSignOutline } from '@vicons/ionicons5'
import { useBoardViewStore } from '@/stores/boardView'
import { useProjectWindowStore } from '@/stores/projectWindow'
import { archiveQuery, withoutArchiveQuery, isArchiveQuery } from '@/utils/boardScope'

const { t } = useI18n()
const store = useBoardViewStore()
const projectWindow = useProjectWindowStore()
const route = useRoute()
const router = useRouter()
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
// «Теги» and «Этапы» open the «Проект» window on their tab (#2971); the tag
// popover and the stage modal it replaced are gone.
function openWindow(tab) {
  projectWindow.open(store.projectId, tab)
}
</script>

<template>
  <div class="board-actions" data-tour="board-actions">
    <n-button
      size="small"
      quaternary
      :title="t('board.actions.tags')"
      data-testid="board-tags"
      data-tour="board-tags"
      @click="openWindow('tags')"
    >
      <template #icon><n-icon :component="PricetagsOutline" /></template>
      <span class="ba-lbl">{{ t('board.actions.tags') }}</span>
    </n-button>

    <n-button
      size="small"
      quaternary
      :title="t('board.actions.milestones')"
      data-testid="board-stages"
      @click="openWindow('stages')"
    >
      <template #icon><n-icon :component="TrailSignOutline" /></template>
      <span class="ba-lbl">{{ t('board.actions.milestones') }}</span>
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
      <span class="ba-lbl">{{ t('board.actions.archive') }}</span>
    </n-button>
  </div>
</template>

<style scoped>
.board-actions {
  display: flex;
  align-items: center;
  gap: 4px;
}
/* A narrow window keeps the icons (each button still carries its title). */
@media (max-width: 1280px) {
  .ba-lbl {
    display: none;
  }
}
</style>
