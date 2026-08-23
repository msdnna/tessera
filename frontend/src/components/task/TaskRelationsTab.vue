<script setup>
// «Связи» tab of the task modal: the task's relations plus the cross-board picker
// that creates them. The list stays owned by the modal (its count is on the tab
// label), so edits are published back through `update:relations`. Navigating to a
// related task closes the modal, which only the modal can do — hence `open-related`.
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon, NInput, NButton, NSelect, NPopover, NPopconfirm, useMessage } from 'naive-ui'
import { GitMergeOutline, CloseOutline } from '@vicons/ionicons5'
import { tasks as tasksApi, workspaces as wsApi } from '@/api'
import { sourceMeta, isExternalSource } from '@/utils/sources'
import EmptyState from '../EmptyState.vue'

const props = defineProps({
  taskId: { type: String, default: null },
  relations: { type: Array, default: () => [] },
  wsId: { type: String, default: null },
})
const emit = defineEmits(['update:relations', 'changed', 'open-related'])

const message = useMessage()
const { t } = useI18n()

const relNumber = ref(null)
const relKind = ref('relates')
// Kind labels are resolved per render (a plain array would freeze them at the
// language the tab first mounted in — pitfall 1 of #2799).
const REL_KINDS = ['relates', 'blocks', 'blocked_by', 'duplicates']
const relKindOptions = computed(() =>
  REL_KINDS.map((value) => ({ label: t(`task.relations.kind.${value}`), value })),
)
// Cross-board task autocomplete for linking relations.
const relPickerOpen = ref(false)
const relTasks = ref([]) // workspace tasks, lazily loaded
async function ensureRelTasks() {
  if (relTasks.value.length || !props.wsId) return
  try {
    // include_subtasks so subtasks can be linked (e.g. blocking deps between subtasks).
    const res = await wsApi.tasks(props.wsId, { include_subtasks: 1 })
    relTasks.value = res.data || []
  } catch {
    /* non-fatal — manual number entry still works */
  }
}
function openRelPicker() {
  relPickerOpen.value = true
  ensureRelTasks()
}
// Filter by typed number/title, drop the current task and numberless ones,
// then group by project → board so long lists stay navigable.
const relGroups = computed(() => {
  const q = String(relNumber.value || '')
    .trim()
    .toLowerCase()
  const out = []
  const index = {}
  for (const rt of relTasks.value) {
    if (rt.id === props.taskId || rt.number == null) continue
    if (q && !(`#${rt.number}`.includes(q) || rt.title.toLowerCase().includes(q))) continue
    const pk = rt.project_name || '—'
    const bk = rt.board_name || '—'
    const key = `${pk} / ${bk}`
    if (!index[key]) {
      index[key] = { project: pk, board: bk, tasks: [] }
      out.push(index[key])
    }
    index[key].tasks.push(rt)
  }
  return out.slice(0, 50)
})
async function chooseRelTask(rt) {
  relNumber.value = rt.number
  relPickerOpen.value = false
  await addRelation()
}

function relKindLabel(k) {
  return REL_KINDS.includes(k) ? t(`task.relations.kind.${k}`) : k
}
// Badge meta for a relation owned by an integration; null for hand-made ones (no
// badge at all — "Tessera" on every row would be noise).
function relSource(r) {
  return isExternalSource(r.source) ? sourceMeta(r.source) : null
}
// Deleting an integration-owned relation only holds until the next sync re-projects
// it, so the confirm says so instead of promising a permanent removal.
function relDeleteHint(r) {
  const src = relSource(r)
  return src
    ? t('task.relations.deleteExternalConfirm', { source: src.label })
    : t('task.relations.deleteConfirm')
}
async function addRelation() {
  const n = Number(relNumber.value)
  if (!n) return
  try {
    await tasksApi.addRelation(props.taskId, n, relKind.value)
    relNumber.value = null
    relPickerOpen.value = false
    const r = await tasksApi.relations(props.taskId)
    emit('update:relations', r.data || [])
    emit('changed')
  } catch (e) {
    message.error(e.message)
  }
}
async function removeRelation(id) {
  try {
    await tasksApi.removeRelation(id)
    emit(
      'update:relations',
      props.relations.filter((x) => x.id !== id),
    )
  } catch (e) {
    message.error(e.message)
  }
}
</script>

<template>
  <div class="relations">
    <div v-for="r in relations" :key="r.id" class="relrow">
      <span class="rel-kind">{{ relKindLabel(r.kind) }}</span>
      <span v-if="relSource(r)" class="rel-src" :title="relSource(r).label">
        <n-icon v-if="relSource(r).icon" :component="relSource(r).icon" :size="12" />
        {{ relSource(r).label }}
      </span>
      <button
        class="rel-link"
        :class="{ done: r.related_completed_at }"
        @click="emit('open-related', r)"
      >
        <span class="rel-num">#{{ r.related_number }}</span>
        <span class="rel-title">{{ r.related_title }}</span>
      </button>
      <n-popconfirm
        :positive-button-props="{ type: 'error' }"
        :positive-text="t('common.action.delete')"
        @positive-click="removeRelation(r.id)"
      >
        <template #trigger>
          <button class="c-act" :title="t('task.relations.unlink')">
            <n-icon :component="CloseOutline" />
          </button>
        </template>
        {{ relDeleteHint(r) }}
      </n-popconfirm>
    </div>
    <EmptyState
      v-if="!relations.length"
      size="small"
      :icon="GitMergeOutline"
      :text="t('task.relations.empty')"
    />
    <div class="rel-add">
      <n-select
        v-model:value="relKind"
        :options="relKindOptions"
        size="small"
        style="width: 150px"
      />
      <n-popover
        trigger="manual"
        :show="relPickerOpen"
        placement="bottom-start"
        :width="320"
        @clickoutside="relPickerOpen = false"
      >
        <template #trigger>
          <n-input
            v-model:value="relNumber"
            size="small"
            :placeholder="t('task.relations.searchPlaceholder')"
            style="width: 240px"
            @focus="openRelPicker"
            @keyup.enter="addRelation"
          >
            <template #prefix>#</template>
          </n-input>
        </template>
        <div class="rel-picker">
          <div v-if="!relGroups.length" class="empty-hint">{{ t('task.relations.noMatches') }}</div>
          <div v-for="g in relGroups" :key="g.project + '/' + g.board" class="rp-group">
            <div class="rp-head">{{ g.project }} · {{ g.board }}</div>
            <button
              v-for="rt in g.tasks"
              :key="rt.id"
              type="button"
              class="rp-item"
              @click="chooseRelTask(rt)"
            >
              <span class="rp-num">#{{ rt.number }}</span>
              <span class="rp-title">{{ rt.title }}</span>
            </button>
          </div>
        </div>
      </n-popover>
      <n-button size="small" class="rel-go" @click="addRelation">{{
        t('task.relations.link')
      }}</n-button>
    </div>
  </div>
</template>

<style scoped>
@import './tab-shared.css';

.relations {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.relrow {
  display: flex;
  align-items: center;
  gap: 8px;
}
.rel-kind {
  font-size: 12px;
  color: var(--t-text3);
  width: 96px;
  flex: none;
}
/* Source badge: deliberately flat neutral grey — it marks provenance, not an accent,
   so it must not compete with the accent-gradient chips elsewhere in the modal. */
.rel-src {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  flex: none;
  padding: 0 6px;
  height: 18px;
  border: 1px solid var(--t-border);
  border-radius: 9px;
  background: var(--t-surface-alt);
  color: var(--t-text3);
  font-size: 11px;
  line-height: 1;
}
.rel-link {
  display: flex;
  align-items: center;
  gap: 6px;
  flex: 1;
  min-width: 0;
  border: none;
  background: none;
  cursor: pointer;
  text-align: left;
  padding: 4px 6px;
  border-radius: 6px;
  color: var(--t-text1);
  font-size: 13px;
}
.rel-link:hover {
  background: var(--t-hover);
}
.rel-link.done .rel-title {
  text-decoration: line-through;
  opacity: 0.6;
}
.rel-num {
  color: var(--t-text3);
  font-variant-numeric: tabular-nums;
  flex: none;
}
.rel-title {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.rel-add {
  display: flex;
  gap: 8px;
  align-items: center;
  margin-top: 8px;
}
.rel-go {
  margin-left: auto;
}
.rel-picker {
  max-height: 300px;
  overflow-y: auto;
}
.empty-hint {
  font-size: 13px;
  color: var(--t-text3);
  padding: 4px 0 8px;
}
.rp-group {
  margin-bottom: 8px;
}
.rp-head {
  font-size: 11px;
  font-weight: 600;
  color: var(--t-text3);
  padding: 2px 4px;
  position: sticky;
  top: 0;
  background: var(--t-surface);
}
.rp-item {
  display: flex;
  align-items: baseline;
  gap: 8px;
  width: 100%;
  text-align: left;
  border: none;
  background: transparent;
  border-radius: 6px;
  padding: 6px 8px;
  cursor: pointer;
}
.rp-item:hover {
  background: var(--t-hover);
}
.rp-num {
  flex: none;
  font-size: 12px;
  color: var(--t-text3);
}
.rp-title {
  color: var(--t-text1);
  font-size: 13px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
