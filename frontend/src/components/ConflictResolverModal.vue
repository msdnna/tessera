<script setup>
import { NModal, NCard, NIcon } from 'naive-ui'
import { WarningOutline } from '@vicons/ionicons5'
import ConflictResolverPanel from '@/components/ConflictResolverPanel.vue'

// Thin modal wrapper around ConflictResolverPanel — used at the app level
// (opened from the «Конфликт» pill on a task card via the conflicts store).
// The GitLab integration modal embeds ConflictResolverPanel directly in its
// right pane instead of opening this modal.
const props = defineProps({
  show: { type: Boolean, default: false },
  wsId: { type: String, default: null },
  focusTaskId: { type: String, default: null },
})
const emit = defineEmits(['update:show', 'resolved'])
</script>

<template>
  <n-modal :show="show" @update:show="emit('update:show', $event)">
    <div class="c-wrap">
      <n-card class="c-card" style="width: 900px; max-width: 96vw" role="dialog">
        <template #header>
          <span class="c-title">
            <n-icon :component="WarningOutline" class="c-warn" /> Конфликты обратной записи GitLab
          </span>
        </template>

        <conflict-resolver-panel
          :ws-id="props.wsId"
          :focus-task-id="props.focusTaskId"
          @resolved="emit('resolved', $event)"
          @empty="emit('update:show', false)"
        />
      </n-card>
    </div>
  </n-modal>
</template>

<style scoped>
.c-wrap {
  display: flex;
  justify-content: center;
}
.c-title {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-weight: 600;
}
.c-warn {
  color: #e0a23d;
}
</style>
