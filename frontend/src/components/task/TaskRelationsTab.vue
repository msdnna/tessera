<script setup>
// «Связи» section of the task modal (v2, #2958 — in the properties panel): the
// task's relations plus the cross-board picker that creates them, folded behind
// «+ Связать» so the narrow panel isn't a permanent form. The list stays owned by the modal (its count is on the tab
// label), so edits are published back through `update:relations`. Navigating to a
// related task closes the modal, which only the modal can do — hence `open-related`.
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon, NInput, NButton, NSelect, NPopover, NPopconfirm, useMessage } from 'naive-ui'
import { CloseOutline, AddOutline } from '@vicons/ionicons5'
import { tasks as tasksApi, workspaces as wsApi } from '@/api'
import { sourceMeta, isExternalSource } from '@/utils/sources'
import { columnStatusName } from '@/utils/columnStatus'
import TesseraIcon from '../TesseraIcon.vue'

const props = defineProps({
  taskId: { type: String, default: null },
  relations: { type: Array, default: () => [] },
  wsId: { type: String, default: null },
  // The modal opened from the archive is read-only: the links are there to be
  // followed, not rewired. Without this the ✕ and the "link" form stayed live on
  // an archived task, and both went through to the API (#2932).
  readonly: { type: Boolean, default: false },
})
const emit = defineEmits(['update:relations', 'changed', 'open-related'])

const message = useMessage()
const { t } = useI18n()

const relNumber = ref(null)
const relKind = ref('relates')
const adding = ref(false)
// The linked task's status glyph, from the column the relations list carries.
function relStatus(r) {
  return columnStatusName({
    isDone: !!r.related_completed_at,
    first: !!r.related_column_first,
    name: r.related_column_name,
    nameKey: r.related_column_name_key,
  })
}
// A link is being created right now. The field is cleared only once the request
// comes back, so without this an impatient second Enter sent a second create.
const linking = ref(false)
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
// Numbers already linked with the kind the picker is armed with. The relation's
// key is (pair, kind) — the list arrives in this task's own frame, with the
// inverse rows' kinds already flipped (ListTaskRelations), so a row here means
// exactly what the same (number, kind) would create.
const linkedNumbers = computed(
  () =>
    new Set(
      props.relations.filter((r) => r.kind === relKind.value).map((r) => Number(r.related_number)),
    ),
)
// Filter by typed number/title, drop the current task, numberless ones and the
// ones already linked with this kind, then group by project → board so long
// lists stay navigable.
//
// Already-linked is dropped rather than offered and refused, the same call
// DocLinks makes: re-linking the same (pair, kind) is a no-op server-side (ON
// CONFLICT DO NOTHING), so the row would be a button that visibly does nothing —
// while still writing a second «добавил связь» line into both tasks' history.
const relGroups = computed(() => {
  const q = String(relNumber.value || '')
    .trim()
    .toLowerCase()
  const out = []
  const index = {}
  for (const rt of relTasks.value) {
    if (rt.id === props.taskId || rt.number == null) continue
    if (linkedNumbers.value.has(Number(rt.number))) continue
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
// What was typed, read as a task number. The field carries a «#» prefix and every
// task in the product is named «#123», so a pasted «#123» means 123 — `Number()`
// alone made it NaN and the press was eaten.
function typedNumber() {
  const raw = String(relNumber.value ?? '')
    .trim()
    .replace(/^#/, '')
    .trim()
  const n = Number(raw)
  return raw !== '' && Number.isInteger(n) && n > 0 ? n : null
}

// Resolves the press into a task number, or null if there is nothing to send.
//
// The field is «№ или название» and the picker under it filters by both, so a
// typed title has to lead somewhere: `Number('оплата')` is NaN, and the old
// `if (!n) return` dropped Enter and «Связать» on the floor without a word — a
// dead end on the one input the tab advertises. A single match is unambiguous and
// gets linked; anything else opens the filtered list instead of going quiet.
function resolveTarget() {
  const n = typedNumber()
  if (n != null) return n
  if (!String(relNumber.value ?? '').trim()) {
    relPickerOpen.value = true // empty field: show the list, never link at random
    return null
  }
  const matches = relGroups.value.flatMap((g) => g.tasks)
  if (matches.length === 1) return Number(matches[0].number)
  relPickerOpen.value = true
  if (!matches.length) message.warning(t('task.relations.noMatches'))
  return null
}

async function addRelation() {
  if (props.readonly || linking.value) return
  linking.value = true
  try {
    // A typed title is resolved against the picker's list, and Enter can beat the
    // lazy fetch that focusing the field started.
    if (typedNumber() == null) await ensureRelTasks()
    const n = resolveTarget()
    if (!n) return
    // Already linked with this kind: the request would come back 201 having done
    // nothing, leaving the reader with an unchanged list and no explanation.
    if (linkedNumbers.value.has(n)) {
      message.info(t('task.relations.already', { number: n }))
      relNumber.value = null
      relPickerOpen.value = false
      return
    }
    // The task this tab is about, taken before the first await. The modal is not
    // remounted when the task under it changes — and this tab is the fastest way
    // there is to change it, since clicking a relation row opens the other end.
    // Reading props afterwards re-read the relations of whichever task that left
    // us with: the list of the task we walked away from landed under the open one's
    // label, and the ✕ on such a row unlinks by id — the other task's link (#2932).
    const tid = props.taskId
    await tasksApi.addRelation(tid, n, relKind.value)
    relNumber.value = null
    relPickerOpen.value = false
    adding.value = false
    const r = await tasksApi.relations(tid)
    if (tid !== props.taskId) return
    emit('update:relations', r.data || [])
    emit('changed')
  } catch (e) {
    message.error(e.message)
  } finally {
    linking.value = false
  }
}
async function removeRelation(id) {
  if (props.readonly) return
  const tid = props.taskId
  try {
    await tasksApi.removeRelation(id)
    if (tid !== props.taskId) return
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
      <button
        class="rel-link"
        :class="{ done: r.related_completed_at }"
        @click="emit('open-related', r)"
      >
        <TesseraIcon
          :name="relStatus(r)"
          :size="14"
          class="rel-st"
          :style="{ color: r.related_column_color || 'var(--t-text3)' }"
        />
        <span class="rel-num">#{{ r.related_number }}</span>
        <span class="rel-title">{{ r.related_title }}</span>
      </button>
      <span v-if="relSource(r)" class="rel-src" :title="relSource(r).label">
        <n-icon v-if="relSource(r).icon" :component="relSource(r).icon" :size="12" />
      </span>
      <n-popconfirm
        v-if="!readonly"
        :positive-button-props="{ type: 'error' }"
        :positive-text="t('common.action.delete')"
        @positive-click="removeRelation(r.id)"
      >
        <template #trigger>
          <button class="c-act rel-x" :title="t('task.relations.unlink')">
            <n-icon :component="CloseOutline" />
          </button>
        </template>
        {{ relDeleteHint(r) }}
      </n-popconfirm>
    </div>
    <div v-if="!relations.length && !adding" class="rel-empty">
      {{ t('task.relations.empty') }}
    </div>
    <!-- Archive view: look, don't touch (#2932) — no way to add a link. -->
    <button v-if="!adding && !readonly" type="button" class="rel-open" @click="adding = true">
      <n-icon :component="AddOutline" :size="14" />
      {{ t('task.relations.link') }}
    </button>
    <div v-else-if="!readonly" class="rel-add">
      <n-select v-model:value="relKind" :options="relKindOptions" size="small" />
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
      <div class="rel-add-acts">
        <n-button size="small" quaternary @click="adding = false">{{
          t('common.action.cancel')
        }}</n-button>
        <n-button size="small" type="primary" :loading="linking" @click="addRelation">{{
          t('task.relations.link')
        }}</n-button>
      </div>
    </div>
  </div>
</template>

<style scoped>
@import './tab-shared.css';

.relations {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.relrow {
  display: flex;
  align-items: center;
  gap: 6px;
  min-height: 34px;
  padding: 0 4px 0 8px;
  border-radius: 9px;
}
.relrow:hover {
  background: var(--t-hover);
}
.rel-kind {
  flex: none;
  width: 64px;
  font-size: 12px;
  color: var(--t-text3);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
/* Source badge: flat neutral — it marks provenance, not an accent. */
.rel-src {
  display: inline-flex;
  align-items: center;
  flex: none;
  color: var(--t-text3);
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
  padding: 0;
  color: var(--t-text1);
  font: inherit;
  font-size: 13px;
}
.rel-st {
  flex: none;
}
.rel-link.done .rel-title {
  text-decoration: line-through;
  color: var(--t-text3);
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
.rel-x {
  opacity: 0;
  transition: opacity 0.12s ease;
}
.relrow:hover .rel-x,
.rel-x:focus-visible {
  opacity: 1;
}
.rel-empty {
  padding: 4px 8px;
  font-size: 12.5px;
  color: var(--t-text3);
}
.rel-open {
  align-self: flex-start;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 28px;
  padding: 0 8px;
  border: none;
  border-radius: 8px;
  background: transparent;
  color: var(--t-text2);
  font: inherit;
  font-size: 12.5px;
  cursor: pointer;
}
.rel-open:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.rel-add {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 6px 4px 0;
}
.rel-add-acts {
  display: flex;
  justify-content: flex-end;
  gap: 6px;
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
