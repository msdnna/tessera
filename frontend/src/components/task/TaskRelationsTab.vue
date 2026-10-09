<script setup>
// «Связи» section of the task modal (v2, #2958 — in the properties panel): the
// task's relations plus the form that creates them. «+ Связать ⇧L» opens the
// relation popover form of package J (#3118, task/props/RelationPicker): the
// kind as a segment (⌥1–4), the task by number or title, ↵ links and stays for
// the next one. Unlinking is at once with an undo toast — a hand-made link is
// put back by re-creating it; one owned by an integration comes back with the
// next sync anyway, so it asks first instead. The list stays owned by the modal
// (its count is on the tab label), so edits are published back through
// `update:relations`. Navigating to a related task closes the modal, which only
// the modal can do — hence `open-related`.
//
// The modal puts «Связать» into the section's header (`anchor` + v-model:open);
// mounted on its own the tab draws the button under the list itself.
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import { useMessage } from '@/composables/useMessage'
import { CloseOutline, AddOutline } from '@vicons/ionicons5'
import { tasks as tasksApi } from '@/api'
import { sourceMeta, isExternalSource } from '@/utils/sources'
import { columnStatusName } from '@/utils/columnStatus'
import { useUndoStore } from '@/stores/undo'
import TesseraIcon from '../TesseraIcon.vue'
import ArmButton from '../kit/ArmButton.vue'
import RelationPicker from './props/RelationPicker.vue'

const props = defineProps({
  taskId: { type: String, default: null },
  taskNumber: { type: [Number, String], default: null },
  relations: { type: Array, default: () => [] },
  wsId: { type: String, default: null },
  // The modal opened from the archive is read-only: the links are there to be
  // followed, not rewired. Without this the ✕ and the "link" form stayed live on
  // an archived task, and both went through to the API (#2932).
  readonly: { type: Boolean, default: false },
  // The host's own «Связать» (the section header) and the form's state.
  anchor: { type: Function, default: null },
  open: { type: Boolean, default: undefined },
})
const emit = defineEmits(['update:relations', 'changed', 'open-related', 'update:open'])

const message = useMessage()
const { t } = useI18n()
const openBtn = ref(null)

const ownOpen = ref(false)
const adding = computed({
  get: () => (props.open === undefined ? ownOpen.value : props.open),
  set: (v) => {
    ownOpen.value = v
    emit('update:open', v)
  },
})
const formAnchor = () => (props.anchor ? props.anchor() : openBtn.value)
// The linked task's status glyph, from the column the relations list carries.
function relStatus(r) {
  return columnStatusName({
    isDone: !!r.related_completed_at,
    first: !!r.related_column_first,
    name: r.related_column_name,
    nameKey: r.related_column_name_key,
  })
}
const REL_KINDS = ['relates', 'blocks', 'blocked_by', 'duplicates']
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
      v-if="!readonly && !anchor"
      ref="openBtn"
      type="button"
      class="rel-open"
      data-testid="rel-open"
      @click="adding = !adding"
    >
      <n-icon :component="AddOutline" :size="14" />
      {{ t('task.relations.link') }}
    </button>

    <RelationPicker
      v-if="!readonly"
      v-model:show="adding"
      :anchor="formAnchor"
      :task-id="taskId"
      :task-number="taskNumber"
      :relations="relations"
      :ws-id="wsId"
      @update:relations="emit('update:relations', $event)"
      @changed="emit('changed')"
      @remove="removeRelation"
    />
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
</style>
