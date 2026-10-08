<script setup>
// «Связи» section of the task modal (v2, #2958 — in the properties panel): the
// task's relations plus the cross-board picker that creates them. «+ Связать»
// opens a popover form (v2 tier 1, #3113): the kind as a list, the task by number
// or title with the matches under the field. Unlinking is at once with an undo
// toast — a hand-made link is put back by re-creating it; one owned by an
// integration comes back with the next sync anyway, so it asks first instead. The list stays owned by the modal (its count is on the tab
// label), so edits are published back through `update:relations`. Navigating to a
// related task closes the modal, which only the modal can do — hence `open-related`.
import { ref, computed, nextTick, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon, useMessage } from 'naive-ui'
import { CloseOutline, AddOutline } from '@vicons/ionicons5'
import { tasks as tasksApi, workspaces as wsApi } from '@/api'
import { sourceMeta, isExternalSource } from '@/utils/sources'
import { columnStatusName } from '@/utils/columnStatus'
import { useUndoStore } from '@/stores/undo'
import TesseraIcon from '../TesseraIcon.vue'
import AnchorLayer from '../kit/AnchorLayer.vue'
import ArmButton from '../kit/ArmButton.vue'
import PopoverForm from '../kit/PopoverForm.vue'
import FormField from '../kit/FormField.vue'

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
const openBtn = ref(null)

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
// The matches under the field, flat — what ↑↓ walk.
const relFlat = computed(() => relGroups.value.flatMap((g) => g.tasks))
const relActive = ref(-1)
watch(relNumber, () => (relActive.value = -1))
function onRelKey(e) {
  if (e.isComposing) return
  const n = relFlat.value.length
  if ((e.key === 'ArrowDown' || e.key === 'ArrowUp') && n) {
    e.preventDefault()
    relPickerOpen.value = true
    const step = e.key === 'ArrowDown' ? 1 : -1
    relActive.value = relActive.value < 0 && step < 0 ? n - 1 : (relActive.value + step + n) % n
    nextTick(() =>
      document
        .querySelector('[data-testid="rel-form"] .rp-item.on')
        ?.scrollIntoView?.({ block: 'nearest' }),
    )
    return
  }
  // ↵ on a highlighted match links it; otherwise the typed number / title.
  if (e.key === 'Enter' && !e.metaKey && !e.ctrlKey && !e.altKey) {
    e.preventDefault()
    e.stopPropagation()
    const rt = relFlat.value[relActive.value]
    if (rt) chooseRelTask(rt)
    else addRelation()
  }
}
// Picking the kind hands the keyboard back to the task field — the next thing
// to type.
const numInput = ref(null)
function pickKind(k) {
  relKind.value = k
  numInput.value?.focus()
}
function openForm() {
  adding.value = true
  relPickerOpen.value = true
  ensureRelTasks()
}
watch(adding, (on) => {
  if (on) return
  relNumber.value = null
  relPickerOpen.value = false
})

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
async function removeRelation(r) {
  if (props.readonly) return
  const tid = props.taskId
  try {
    await tasksApi.removeRelation(r.id)
  } catch (e) {
    message.error(e.message)
    return
  }
  emit('changed')
  if (tid === props.taskId)
    emit(
      'update:relations',
      props.relations.filter((x) => x.id !== r.id),
    )
  // A hand-made link is put back by making it again — same number, same kind,
  // in this task's frame (the list arrives with inverse kinds already flipped).
  if (relSource(r)) return
  const number = Number(r.related_number)
  // Taken here, not in setup: the tab mounts in tests without a store.
  useUndoStore().push({
    text: { key: 'task.relations.removed', params: { number } },
    doneText: { key: 'task.relations.restored', params: { number } },
    undo: async () => {
      await tasksApi.addRelation(tid, number, r.kind)
      emit('changed')
      const res = await tasksApi.relations(tid)
      if (tid === props.taskId) emit('update:relations', res.data || [])
    },
  })
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
      <template v-if="!readonly">
        <!-- An integration's link returns with its next sync: ask first. -->
        <ArmButton
          v-if="relSource(r)"
          :icon="CloseOutline"
          :title="relDeleteHint(r)"
          :confirm="t('task.arm.unlink')"
          testid="rel-unlink"
          @confirm="removeRelation(r)"
        />
        <button
          v-else
          type="button"
          class="rel-x"
          :title="t('task.relations.unlink')"
          :aria-label="t('task.relations.unlink')"
          data-testid="rel-unlink"
          @click="removeRelation(r)"
        >
          <n-icon :component="CloseOutline" :size="15" />
        </button>
      </template>
    </div>
    <div v-if="!relations.length" class="rel-empty">
      {{ t('task.relations.empty') }}
    </div>
    <!-- Archive view: look, don't touch (#2932) — no way to add a link. -->
    <button
      v-if="!readonly"
      ref="openBtn"
      type="button"
      class="rel-open"
      data-testid="rel-open"
      @click="adding ? (adding = false) : openForm()"
    >
      <n-icon :component="AddOutline" :size="14" />
      {{ t('task.relations.link') }}
    </button>

    <AnchorLayer
      v-if="!readonly"
      :show="adding"
      :anchor="() => openBtn"
      placement="bottom-start"
      :offset="{ x: 0, y: 6 }"
      focus=".kf-input"
      testid="rel-form"
      @update:show="adding = $event"
    >
      <PopoverForm
        inline
        :show="adding"
        :width="340"
        :title="t('task.relations.formTitle')"
        :primary="t('task.relations.link')"
        :busy="linking"
        :disabled="!String(relNumber ?? '').trim()"
        hint="list"
        testid="rel-form-card"
        @update:show="adding = $event"
        @submit="addRelation"
      >
        <FormField :label="t('task.relations.kindLabel')">
          <div class="rel-kinds" role="radiogroup">
            <button
              v-for="k in relKindOptions"
              :key="k.value"
              type="button"
              role="radio"
              class="rel-kind-b"
              :class="{ on: relKind === k.value }"
              :aria-checked="relKind === k.value"
              :data-testid="`rel-kind-${k.value}`"
              @click="pickKind(k.value)"
            >
              {{ k.label }}
            </button>
          </div>
        </FormField>
        <FormField :label="t('task.relations.taskLabel')">
          <div class="rel-num-field">
            <span class="rel-hash" aria-hidden="true">#</span>
            <input
              ref="numInput"
              v-model="relNumber"
              class="kf-input"
              :placeholder="t('task.relations.searchPlaceholder')"
              data-testid="rel-number"
              data-no-select
              autocomplete="off"
              @focus="openRelPicker"
              @keydown="onRelKey"
            />
          </div>
        </FormField>
        <div v-if="relPickerOpen" class="rel-picker">
          <div v-if="!relGroups.length" class="empty-hint">{{ t('task.relations.noMatches') }}</div>
          <div v-for="g in relGroups" :key="g.project + '/' + g.board" class="rp-group">
            <div class="rp-head">{{ g.project }} · {{ g.board }}</div>
            <button
              v-for="rt in g.tasks"
              :key="rt.id"
              type="button"
              class="rp-item"
              :class="{ on: relFlat[relActive]?.id === rt.id }"
              tabindex="-1"
              @mousedown.prevent
              @click="chooseRelTask(rt)"
            >
              <span class="rp-num">#{{ rt.number }}</span>
              <span class="rp-title">{{ rt.title }}</span>
            </button>
          </div>
        </div>
      </PopoverForm>
    </AnchorLayer>
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
.relrow :is(.rel-x, .arm:not(.armed)) {
  opacity: 0;
  transition: opacity 0.12s ease;
}
.relrow:hover :is(.rel-x, .arm),
.relrow :is(.rel-x, .arm):focus-visible {
  opacity: 1;
}
@media (hover: none) {
  .relrow :is(.rel-x, .arm) {
    opacity: 1;
  }
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
.rel-x {
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  padding: 0;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--t-text3);
  cursor: pointer;
}
.rel-x:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
/* The kind: four rows, one picked — a list rather than a select (v2 kit). */
.rel-kinds {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 4px;
}
.rel-kind-b {
  height: 32px;
  padding: 0 10px;
  border: 0;
  border-radius: 8px;
  background: var(--t-input-bg);
  color: var(--t-text2);
  font: inherit;
  font-size: 13px;
  text-align: left;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  cursor: pointer;
}
.rel-kind-b:hover {
  color: var(--t-text1);
}
.rel-kind-b.on {
  background: linear-gradient(
    to top right,
    color-mix(in srgb, var(--t-primary) 16%, var(--t-pop)),
    color-mix(in srgb, var(--t-primary) 26%, var(--t-pop))
  );
  color: var(--t-text1);
  font-weight: 600;
}
.rel-num-field {
  position: relative;
  display: flex;
  align-items: center;
}
.rel-hash {
  position: absolute;
  left: 10px;
  color: var(--t-text3);
  font-size: 13.5px;
  pointer-events: none;
}
.rel-num-field .kf-input {
  padding-left: 22px;
}
.rel-picker {
  max-height: 220px;
  overflow-y: auto;
  overscroll-behavior: contain;
  margin: 0 -6px;
  scrollbar-width: thin;
}
.empty-hint {
  font-size: 12.5px;
  color: var(--t-text3);
  padding: 4px 6px 8px;
}
.rp-group {
  margin-bottom: 4px;
}
.rp-head {
  padding: 6px 8px 4px;
  font-size: 11.5px;
  font-weight: 600;
  letter-spacing: 0.05em;
  text-transform: uppercase;
  color: var(--t-text3);
  position: sticky;
  top: 0;
  background: var(--t-pop);
}
.rp-item {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  min-height: 32px;
  text-align: left;
  border: none;
  background: transparent;
  border-radius: 7px;
  padding: 0 8px;
  font: inherit;
  cursor: pointer;
}
.rp-item:hover,
.rp-item.on {
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
