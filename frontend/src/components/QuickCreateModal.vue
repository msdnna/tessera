<script setup>
// Quick create (v2, #2956): a task into «К работе» from anywhere off a board —
// the FAB menu and «N, then T». On a board «N, T» uses the board's own inline
// quick-add instead (see QuickActionsFab). Reminders and notes moved to the
// «Входящие» capture popover (#2996).
import { ref, computed, watch, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { NModal, NCard, NInput, NSelect, NButton, useMessage } from 'naive-ui'
import { boards as boardsApi, projects as projApi } from '@/api'
import { useQuickStore } from '@/stores/quick'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useBoardViewStore } from '@/stores/boardView'
import { useOverlayBack } from '@/composables/useOverlayBack'
import { sGet } from '@/utils/serverStorage'
import { sortedColumns } from '@/utils/status'
import { todoColumn } from '@/utils/columnStatus'
import { normalizeTitle } from '@/utils/title'
import { humanizeError } from '@/utils/errors'

const { t } = useI18n()
const message = useMessage()
const quick = useQuickStore()
const ws = useWorkspacesStore()
const boardView = useBoardViewStore()

const show = computed({
  get: () => quick.createMode === 'task',
  set: (v) => {
    if (!v) quick.createMode = null
  },
})
useOverlayBack(show, () => (show.value = false))

const mode = computed(() => quick.createMode)
const title = ref('')
const boardId = ref(null)
const boardOptions = ref([])
const busy = ref(false)
const titleEl = ref(null)

watch(mode, async (m) => {
  if (!m) return
  title.value = ''
  await nextTick()
  titleEl.value?.focus()
  loadBoards()
})

// Boards of every project in the workspace, grouped by project. The board
// the user was last on is the default.
async function loadBoards() {
  const projects = ws.projects || []
  const lists = await Promise.allSettled(projects.map((p) => projApi.boards(p.id)))
  boardOptions.value = projects
    .map((p, i) => ({
      type: 'group',
      key: p.id,
      label: p.name,
      children: (lists[i].status === 'fulfilled' ? lists[i].value.data || [] : []).map((b) => ({
        label: b.name,
        value: b.id,
      })),
    }))
    .filter((g) => g.children.length)
  const all = boardOptions.value.flatMap((g) => g.children.map((c) => c.value))
  const preferred = [boardView.boardId, sGet('tessera_last_board_id')]
  boardId.value = preferred.find((id) => id && all.includes(id)) || all[0] || null
}

const canSubmit = computed(() => !busy.value && !!normalizeTitle(title.value) && !!boardId.value)

async function submit() {
  if (!canSubmit.value) return
  busy.value = true
  try {
    const cols = (await boardsApi.columns(boardId.value)).data || []
    const col = todoColumn(sortedColumns(cols))
    if (!col) throw new Error(t('board.column.needStatus'))
    const res = await boardsApi.createTask(boardId.value, {
      column_id: col.id,
      title: normalizeTitle(title.value),
    })
    message.success(t('shell.quick.taskCreated', { n: res.data.number }))
    show.value = false
  } catch (e) {
    message.error(humanizeError(e))
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <n-modal v-model:show="show" :auto-focus="false">
    <n-card
      v-if="mode"
      :title="t(`shell.quick.title.${mode}`)"
      style="width: 460px; max-width: calc(100vw - 24px)"
      role="dialog"
      :bordered="false"
      data-testid="quick-create"
    >
      <div class="qc-body">
        <n-input
          ref="titleEl"
          v-model:value="title"
          :placeholder="t(`shell.quick.placeholder.${mode}`)"
          :disabled="busy"
          :input-props="{ 'data-testid': 'quick-title' }"
          @keydown.enter.prevent="submit"
        />
        <n-select
          v-model:value="boardId"
          :options="boardOptions"
          :placeholder="t('shell.quick.board')"
          :disabled="busy"
          filterable
        />
      </div>
      <template #footer>
        <div class="qc-actions">
          <n-button size="small" :disabled="busy" @click="show = false">
            {{ t('common.action.cancel') }}
          </n-button>
          <n-button
            type="primary"
            size="small"
            :disabled="!canSubmit"
            :loading="busy"
            data-testid="quick-submit"
            @click="submit"
          >
            {{ t('common.action.create') }}
          </n-button>
        </div>
      </template>
    </n-card>
  </n-modal>
</template>

<style scoped>
.qc-body {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.qc-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
</style>
