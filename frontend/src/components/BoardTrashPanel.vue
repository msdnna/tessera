<script setup>
// The board's «Корзина» (#3004, migration 0086): the tasks deleted from this
// board in the last 30 days, newest first, each with «Вернуть» and a two-step
// «Удалить навсегда». A DockPanel docked right of the board, like «Вид доски»
// (on a phone — a bottom sheet); opened from «⋯ Действия доски» or from the
// «В корзину» of a toast about a deleted task (#3120), which also names the
// row to light up (`highlight` — the task's number).
//
// The trash is not the archive: an archived task is finished work kept for
// reference (a read-only scope of the board itself), a task here is a delete
// waiting to be undone or forgotten — so it is a list with actions, not cards.
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import { ArrowUndoOutline, TrashOutline } from '@vicons/ionicons5'
import DockPanel from './kit/DockPanel.vue'
import ArmButton from './kit/ArmButton.vue'
import EmptyState from './EmptyState.vue'
import { boards as boardsApi, tasks as tasksApi } from '@/api'
import { useFormat } from '@/composables/useFormat'
import { useToast } from '@/composables/useToast'
import { humanizeError } from '@/utils/errors'
import { TRASH_DAYS, daysLeft } from '@/utils/taskTrash'

const show = defineModel('show', { type: Boolean, default: false })
const props = defineProps({
  boardId: { type: String, default: '' },
  // Bumped by the board on a task.deleted / .restored / .purged of this board.
  reloadToken: { type: Number, default: 0 },
  // A task number to light up and scroll to (the toast's «В корзину»).
  highlight: { type: [Number, String], default: null },
  // The project is archived (#3005): the list reads, the actions don't.
  readonly: { type: Boolean, default: false },
})
const emit = defineEmits(['restored'])
const { t } = useI18n()
const { formatRelative } = useFormat()
const toast = useToast()

const rows = ref([])
const loading = ref(false)
const busy = ref('') // the id of the row whose action is in flight
const body = ref(null)

async function load() {
  if (!show.value || !props.boardId) return
  loading.value = true
  try {
    const { data } = await boardsApi.trash(props.boardId)
    rows.value = data || []
  } catch (e) {
    toast?.error(humanizeError(e))
  } finally {
    loading.value = false
  }
  revealHighlight()
}

watch(
  () => [show.value, props.boardId, props.reloadToken],
  () => load(),
  { immediate: true },
)

const lit = computed(() => (props.highlight == null ? null : Number(props.highlight)))
async function revealHighlight() {
  if (lit.value == null) return
  await nextTick()
  body.value
    ?.querySelector(`[data-trash-number="${lit.value}"]`)
    ?.scrollIntoView({ block: 'nearest' })
}
watch(lit, revealHighlight)

const subtitle = computed(() =>
  rows.value.length
    ? t('board.trash.count', { n: rows.value.length }, rows.value.length)
    : t('board.trash.keep', { days: TRASH_DAYS }),
)

function meta(r) {
  const parts = []
  if (r.deleted_by_name) parts.push(t('board.trash.by', { name: r.deleted_by_name }))
  parts.push(formatRelative(r.deleted_at))
  parts.push(t('board.trash.left', { n: daysLeft(r.deleted_at) }, daysLeft(r.deleted_at)))
  return parts.join(' · ')
}

async function restore(r) {
  if (busy.value) return
  busy.value = r.id
  try {
    await tasksApi.untrash(r.id)
    rows.value = rows.value.filter((x) => x.id !== r.id)
    toast?.success(
      r.number != null
        ? t('task.trash.restored', { number: r.number })
        : t('task.trash.restoredUntitled', { title: r.title }),
    )
    emit('restored', r)
  } catch (e) {
    toast?.error(humanizeError(e))
  } finally {
    busy.value = ''
  }
}

async function purge(r) {
  if (busy.value) return
  busy.value = r.id
  try {
    await tasksApi.purge(r.id)
    rows.value = rows.value.filter((x) => x.id !== r.id)
  } catch (e) {
    toast?.error(humanizeError(e))
  } finally {
    busy.value = ''
  }
}
</script>

<template>
  <DockPanel
    v-model:show="show"
    :title="t('board.trash.title')"
    :subtitle="subtitle"
    :width="340"
    testid="board-trash"
  >
    <div ref="body" class="trash">
      <EmptyState
        v-if="!loading && !rows.length"
        class="trash-empty"
        :icon="TrashOutline"
        :title="t('board.trash.emptyTitle')"
        :text="t('board.trash.emptyText', { days: TRASH_DAYS })"
      />
      <ul v-else class="trash-list">
        <li
          v-for="r in rows"
          :key="r.id"
          class="trash-row"
          :class="{ lit: lit != null && r.number === lit }"
          :data-trash-number="r.number"
          data-testid="trash-row"
        >
          <div class="trash-title">
            <span v-if="r.number != null" class="trash-num">#{{ r.number }}</span>
            <span class="trash-name">{{ r.title || t('task.fallbackTitle') }}</span>
          </div>
          <div v-if="!readonly" class="trash-acts">
            <button
              type="button"
              class="kit-btn trash-restore"
              :disabled="busy === r.id"
              data-testid="trash-restore"
              @click="restore(r)"
            >
              <n-icon :component="ArrowUndoOutline" :size="14" />
              {{ t('board.trash.restore') }}
            </button>
            <ArmButton
              :title="t('board.trash.purge')"
              :confirm="t('board.trash.purgeArmed')"
              testid="trash-purge"
              @confirm="purge(r)"
            />
          </div>
          <div class="trash-meta">
            {{ meta(r) }}
            <template v-if="r.subtask_count">
              · {{ t('board.trash.subtasks', { n: r.subtask_count }, r.subtask_count) }}
            </template>
          </div>
        </li>
      </ul>
    </div>
  </DockPanel>
</template>

<style scoped>
.trash {
  padding: 4px 10px 12px;
}
.trash-empty {
  margin-top: 24px;
}
.trash-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
/* Two lines: the task and its actions, then who deleted it and when — the meta
   gets the whole width (a long name must not squeeze the title). */
.trash-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: center;
  column-gap: 8px;
  padding: 8px 8px 8px 10px;
  border-radius: 10px;
  transition: background 0.15s ease;
}
.trash-row:hover {
  background: var(--t-hover);
}
/* The row a toast pointed at: an accent ring that fades (PackageI «Подсветка»). */
.trash-row.lit {
  box-shadow: inset 0 0 0 1.5px var(--t-primary);
  background: var(--t-active);
}
.trash-title {
  display: flex;
  align-items: baseline;
  gap: 6px;
  font-size: 13.5px;
  min-width: 0;
}
.trash-num {
  flex: none;
  font-variant-numeric: tabular-nums;
  color: var(--t-text3);
  font-size: 12.5px;
}
.trash-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--t-text1);
}
.trash-meta {
  grid-column: 1 / -1;
  margin-top: 2px;
  font-size: 12px;
  color: var(--t-text3);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.trash-acts {
  flex: none;
  display: flex;
  align-items: center;
  gap: 4px;
}
.trash-restore {
  height: 28px;
  padding: 0 10px;
  font-size: 12.5px;
  gap: 5px;
}
</style>
