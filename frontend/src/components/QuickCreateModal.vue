<script setup>
// Quick create (v2, #2956): a task into «К работе», a reminder or a note, from
// anywhere — the FAB menu and «N, then T/R/M». On a board «N, T» uses the
// board's own inline quick-add instead (see QuickActionsFab); this dialog is
// the off-board path and the only path for reminders and notes.
import { ref, computed, watch, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { NModal, NCard, NInput, NSelect, NDatePicker, NButton, useMessage } from 'naive-ui'
import { useRouter } from 'vue-router'
import {
  boards as boardsApi,
  projects as projApi,
  reminders as remApi,
  notes as notesApi,
} from '@/api'
import { useQuickStore } from '@/stores/quick'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useBoardViewStore } from '@/stores/boardView'
import { useFormat } from '@/composables/useFormat'
import { useOverlayBack } from '@/composables/useOverlayBack'
import { sGet } from '@/utils/serverStorage'
import { sortedColumns } from '@/utils/status'
import { todoColumn } from '@/utils/columnStatus'
import { normalizeTitle } from '@/utils/title'
import { humanizeError } from '@/utils/errors'

const { t } = useI18n()
const message = useMessage()
const router = useRouter()
const quick = useQuickStore()
const ws = useWorkspacesStore()
const boardView = useBoardViewStore()
const { firstDayOfWeek, dateTimePattern } = useFormat()

const show = computed({
  get: () => !!quick.createMode,
  set: (v) => {
    if (!v) quick.createMode = null
  },
})
useOverlayBack(show, () => (show.value = false))

const mode = computed(() => quick.createMode)
const title = ref('')
const body = ref('')
const remindAt = ref(null)
const boardId = ref(null)
const boardOptions = ref([])
const busy = ref(false)
const titleEl = ref(null)

// The next whole hour, an hour from now at the earliest — a reminder default
// that is always in the future and reads as a round time.
function nextHour() {
  const d = new Date(Date.now() + 60 * 60 * 1000)
  d.setMinutes(0, 0, 0)
  return d.getTime()
}

watch(mode, async (m) => {
  if (!m) return
  title.value = ''
  body.value = ''
  remindAt.value = nextHour()
  await nextTick()
  titleEl.value?.focus()
  if (m === 'task') loadBoards()
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

const canSubmit = computed(() => {
  if (busy.value) return false
  if (mode.value === 'task') return !!normalizeTitle(title.value) && !!boardId.value
  if (mode.value === 'reminder') return !!title.value.trim() && !!remindAt.value
  return !!title.value.trim()
})

async function submit() {
  if (!canSubmit.value) return
  busy.value = true
  try {
    if (mode.value === 'task') {
      const cols = (await boardsApi.columns(boardId.value)).data || []
      const col = todoColumn(sortedColumns(cols))
      if (!col) throw new Error(t('board.column.needStatus'))
      const res = await boardsApi.createTask(boardId.value, {
        column_id: col.id,
        title: normalizeTitle(title.value),
      })
      message.success(t('shell.quick.taskCreated', { n: res.data.number }))
    } else if (mode.value === 'reminder') {
      await remApi.create({
        message: title.value.trim(),
        remind_at: new Date(remindAt.value).toISOString(),
      })
      message.success(t('shell.quick.reminderCreated'))
    } else {
      const res = await notesApi.create(ws.currentId, {
        title: title.value.trim(),
        body: body.value,
      })
      message.success(t('shell.quick.noteCreated'))
      router.push(`/notes?note=${res.data.slug || res.data.id}`)
    }
    show.value = false
  } catch (e) {
    message.error(humanizeError(e))
  } finally {
    busy.value = false
  }
}

// ⌘↵ submits from the multi-line note body, like the comment composer.
function onBodyKey(e) {
  if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
    e.preventDefault()
    submit()
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
          v-if="mode === 'task'"
          v-model:value="boardId"
          :options="boardOptions"
          :placeholder="t('shell.quick.board')"
          :disabled="busy"
          filterable
        />
        <n-date-picker
          v-if="mode === 'reminder'"
          v-model:value="remindAt"
          type="datetime"
          :format="dateTimePattern"
          :first-day-of-week="firstDayOfWeek"
          :disabled="busy"
        />
        <n-input
          v-if="mode === 'note'"
          v-model:value="body"
          type="textarea"
          :autosize="{ minRows: 4, maxRows: 10 }"
          :placeholder="t('shell.quick.placeholder.noteBody')"
          :disabled="busy"
          @keydown="onBodyKey"
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
