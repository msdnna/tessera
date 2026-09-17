<script setup>
import { ref, computed, nextTick, h } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon, NButton, NInput, NPopover, NPopconfirm, NDropdown, useMessage } from 'naive-ui'
import {
  EllipsisHorizontalOutline,
  TrashOutline,
  CheckmarkDoneOutline,
  CreateOutline,
  ChevronBackOutline,
} from '@vicons/ionicons5'
import TesseraIcon from './TesseraIcon.vue'

const menuIcon = (icon) => () => h(NIcon, null, { default: () => h(icon) })
const dangerIcon = (icon) => () => h(NIcon, { color: '#e0533d' }, { default: () => h(icon) })
import { columns as columnsApi } from '@/api'
import { columnStatusName } from '@/utils/columnStatus'

const { t } = useI18n()

const props = defineProps({
  dcol: { type: Object, required: true },
  count: { type: Number, default: 0 }, // cards in the column
  total: { type: Number, default: 0 }, // same, with every subtask folded in (#2850)
  estimate: { type: String, default: '' }, // Σ estimate (milestone grouping); '' = hide
  editable: { type: Boolean, default: false }, // status columns only
  readonly: { type: Boolean, default: false }, // archive view: look, don't touch
  isDone: { type: Boolean, default: false }, // task-completing column
  first: { type: Boolean, default: false }, // leftmost column (To-Do icon)
  collapsed: { type: Boolean, default: false }, // rendered as a narrow strip
})
const emit = defineEmits(['changed', 'set-done', 'toggle-collapse'])

// `editable` says the lane is a real status column (tag/milestone lanes are not);
// `readonly` says the board is being *looked at*, not worked on — the archive view.
// Only the two together open the menu, the rename and the drag handle: a column's
// delete cascades through tasks.column_id and takes every card standing in it,
// archived or not (TaskIDsInColumn filters on column_id alone), while the archive
// shows — and counts — only the archived ones. Confirming «delete the column and
// its tasks» over a header reading «3» would silently destroy the column's live
// cards and their uploads, with no undo. Card DnD, column DnD and the card menus
// are already closed in the archive (KanbanBoard `archivedMode`); this is the
// header catching up.
const canEdit = computed(() => props.editable && !props.readonly)

// Status glyph from the Tessera icon pack: done = check circle,
// first = empty circle, review = ⅔-pie, other middle = in-progress (half). Filled
// for done so it stands out; the glyph is tinted flat with the column colour (matches
// the pack preview). Review columns are detected by name (no explicit column type) —
// that name-matching lives in utils/columnStatus.js, where the Russian words are
// data (what the seeded rows are called), not interface text.
// The glyph is picked from the column's own name (and its name_key when it has
// one), not from the caption: the table in columnStatus.js matches the words the
// row actually contains.
const rawName = computed(() => props.dcol.rawName ?? props.dcol.name)
const statusName = computed(() =>
  columnStatusName({
    isDone: props.isDone,
    first: props.first,
    name: rawName.value,
    nameKey: props.dcol.status?.name_key,
  }),
)
const statusVariant = computed(() => (props.isDone ? 'filled' : 'outline'))
// Tint the glyph with the column colour; fall back to the accent (same as the
// column's top bar) rather than a dull grey so the icon always reads as coloured.
const statusColor = computed(() => props.dcol.color || 'var(--t-primary)')

// The count chip reads `5 (12)`: cards before the brackets, the total including
// subtasks inside them. A column whose cards have no children would only show
// `5 (5)`, so there the brackets are dropped instead of repeating the number.
const showTotal = computed(() => props.total > props.count)
const countTitle = computed(() =>
  showTotal.value
    ? t('board.column.countTitle', { tasks: props.count, total: props.total })
    : t('board.column.countTitleFlat', { tasks: props.count }),
)

function toggleDone() {
  emit('set-done', props.isDone ? null : props.dcol.key)
  settingsOpen.value = false
}

// ── right-click context menu (status columns only) ──
const ctxShow = ref(false)
const ctxX = ref(0)
const ctxY = ref(0)
const ctxOptions = computed(() => [
  { label: t('board.column.rename'), key: 'rename', icon: menuIcon(CreateOutline) },
  {
    label: props.isDone ? t('board.column.unsetDone') : t('board.column.setDone'),
    key: 'done',
    icon: menuIcon(CheckmarkDoneOutline),
  },
  { type: 'divider', key: 'd1' },
  {
    label: t('board.column.delete'),
    key: 'delete',
    icon: dangerIcon(TrashOutline),
    props: { style: 'color:#e0533d' },
  },
])
function onCtx(e) {
  if (!canEdit.value) return
  e.preventDefault()
  ctxShow.value = false
  ctxX.value = e.clientX
  ctxY.value = e.clientY
  nextTick(() => (ctxShow.value = true))
}
// Deleting a column takes every card standing in it (the server cascades through
// tasks.column_id and purges their uploads), so the menu never calls the API
// straight off: it opens the same confirm the «…» popover shows, anchored where
// the menu stood. Same shape as the task card's right-click delete.
const ctxDeleteConfirm = ref(false)
function onCtxSelect(key) {
  ctxShow.value = false
  if (key === 'rename') startRename()
  else if (key === 'done') toggleDone()
  else if (key === 'delete') ctxDeleteConfirm.value = true
}

const message = useMessage()
const renaming = ref(false)
const nameEdit = ref('')
const settingsOpen = ref(false)
const swatches = [
  '',
  '#9aa0aa',
  '#7c5cff',
  '#2f80ed',
  '#0eb0a9',
  '#18a058',
  '#f0a020',
  '#e0533d',
  '#eb2f96',
]

// Same-hue diagonal gradient for a colour swatch (flat fallback for "default").
function swatchBg(s) {
  if (!s) return 'var(--t-border)'
  return `linear-gradient(to top right, color-mix(in srgb, ${s} 86%, #000), ${s} 50%, color-mix(in srgb, ${s} 86%, #fff))`
}

function startRename() {
  if (!canEdit.value) return
  nameEdit.value = props.dcol.name
  renaming.value = true
}
// Esc abandons the rename. Every other inline field on the board already answers
// it (the column's «＋ задача», the board's «＋ колонка», a card's «＋ подзадача»,
// the matrix quick-add), and this one had no handler at all — so Esc did nothing
// visible, the field stayed open with the edited text, and the next click away
// went through `@blur` and PUT the rename the reader had just cancelled. A column
// name is workspace-shared data, not local view state.
function cancelRename() {
  renaming.value = false
  nameEdit.value = ''
}
async function commitRename() {
  // Closing the field is what marks the gesture finished: Esc unmounts the input
  // and a browser fires no blur for a node that is simply removed, but the guard
  // is on the commit rather than on that behaviour — it also makes the blur that
  // follows an Enter (or a second commit from any other path) a no-op instead of
  // a duplicate PUT.
  if (!renaming.value) return
  renaming.value = false
  const n = nameEdit.value.trim()
  if (!n || n === props.dcol.name) return
  try {
    await columnsApi.update(props.dcol.key, { name: n, color: props.dcol.color || '' })
    emit('changed')
  } catch (e) {
    message.error(e.message)
  }
}
async function setColor(c) {
  try {
    // dcol.name is the caption the reader sees; the column's own name is what the
    // server stores. Sending the caption back would rename the column into the UI
    // language and drop its name_key (see UpdateColumn).
    await columnsApi.update(props.dcol.key, { name: rawName.value, color: c })
    emit('changed')
  } catch (e) {
    message.error(e.message)
  }
}
async function removeCol() {
  try {
    await columnsApi.remove(props.dcol.key)
    settingsOpen.value = false
    emit('changed')
  } catch (e) {
    message.error(e.message)
  }
}
</script>

<template>
  <div class="col-head" :class="{ ro: readonly }" @contextmenu="onCtx">
    <!-- The status glyph stays in the archive (it is what the column *is*), but
         `col-drag` — the Sortable handle and its grab cursor — comes off with the
         rest of the editing: the column draggable is disabled there anyway. -->
    <TesseraIcon
      v-if="editable"
      :name="statusName"
      :variant="statusVariant"
      class="col-stat"
      :class="{ 'col-drag': canEdit }"
      :style="{ color: statusColor }"
      :title="isDone ? t('board.column.doneTitle') : readonly ? '' : t('board.column.dragTitle')"
    />
    <n-input
      v-if="renaming"
      v-model:value="nameEdit"
      size="tiny"
      autofocus
      data-testid="column-rename-input"
      @keyup.enter="commitRename"
      @keyup.esc="cancelRename"
      @blur="commitRename"
    />
    <!-- `title` is not decoration here: the caption is clipped to one line below,
         and a tag lane's caption is whatever the reader named the tag. -->
    <span
      v-else
      class="col-title"
      :class="{ 'col-drag': canEdit }"
      :title="dcol.name"
      @dblclick="startRename"
      >{{ dcol.name }}</span
    >
    <span class="count" :title="countTitle" data-testid="column-count"
      >{{ count }}<span v-if="showTotal" class="count-sub">{{ ' (' + total + ')' }}</span></span
    >
    <span v-if="estimate" class="col-est" :title="t('board.column.estimateTitle')"
      >Σ {{ estimate }}</span
    >
    <n-button
      text
      size="tiny"
      class="col-collapse"
      :title="t('board.column.collapse')"
      @click.stop="emit('toggle-collapse')"
    >
      <n-icon :component="ChevronBackOutline" />
    </n-button>
    <n-popover v-if="canEdit" v-model:show="settingsOpen" trigger="click" placement="bottom-end">
      <template #trigger>
        <n-button text size="tiny" class="col-menu">
          <n-icon :component="EllipsisHorizontalOutline" />
        </n-button>
      </template>
      <div class="settings">
        <div class="swatches">
          <button
            v-for="s in swatches"
            :key="s || 'none'"
            class="sw"
            :class="{ active: s === (dcol.color || '') }"
            :style="{ backgroundImage: swatchBg(s) }"
            :title="s || t('board.column.colorDefault')"
            @click="setColor(s)"
          />
        </div>
        <n-button
          :type="isDone ? 'success' : 'default'"
          :ghost="isDone"
          size="small"
          block
          @click="toggleDone"
        >
          <template #icon><n-icon :component="CheckmarkDoneOutline" /></template>
          {{ isDone ? t('board.column.unsetDone') : t('board.column.setDone') }}
        </n-button>
        <n-popconfirm
          :positive-button-props="{ type: 'error', 'data-testid': 'column-delete-confirm' }"
          :positive-text="t('common.action.delete')"
          @positive-click="removeCol"
        >
          <template #trigger>
            <n-button type="error" ghost size="small" block data-testid="column-delete">
              <template #icon><n-icon :component="TrashOutline" /></template>
              {{ t('board.column.delete') }}
            </n-button>
          </template>
          {{ t('board.column.deleteConfirm') }}
        </n-popconfirm>
      </div>
    </n-popover>

    <n-dropdown
      trigger="manual"
      placement="bottom-start"
      :show="ctxShow"
      :x="ctxX"
      :y="ctxY"
      :options="ctxOptions"
      @select="onCtxSelect"
      @clickoutside="ctxShow = false"
    />
    <n-popconfirm
      v-model:show="ctxDeleteConfirm"
      :x="ctxX"
      :y="ctxY"
      :positive-button-props="{ type: 'error', 'data-testid': 'column-ctx-delete-confirm' }"
      :positive-text="t('common.action.delete')"
      @positive-click="removeCol"
      @clickoutside="ctxDeleteConfirm = false"
    >
      <template #trigger><span /></template>
      {{ t('board.column.deleteConfirm') }}
    </n-popconfirm>
  </div>
</template>

<style scoped>
.col-head {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 8px;
  padding: 0 2px;
}
.col-stat {
  font-size: 16px;
  flex: none;
}
.col-title {
  flex: 1;
  /* A flex item's `min-width` is `auto`, so the caption refuses to shrink below its
     longest word — and in tag-grouping mode (the board's headline feature) the
     caption IS a tag name, which routinely has no break opportunity at all
     («сложность::высокая» with the raw prefix shown). The header then grew past the
     fixed column width and pushed the count chip, the collapse chevron and the «…»
     menu out over the neighbouring column, where they overlap its cards and steal
     clicks — and in a tag lane the chevron is the only way to fold that lane.
     `min-width: 0` lets it shrink; clipping to one line matches what the collapsed
     strip (`.strip-title`) and the Android lane title already do, and it keeps every
     column's header the same height so the card lists start level. */
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-weight: 600;
  color: var(--t-text1);
}
/* The grab hand belongs to the drag handle, and `.col-drag` IS the handle: the
   board's column draggable takes `handle: '.col-drag'`, and this component stamps
   that class exactly when `canEdit` — a real status column outside the archive.
   The cursor used to sit on `.col-stat`/`.col-title` unconditionally with a
   `.col-head.ro` rule taking it back, which covered only the archive. The other
   half of `canEdit` was left promising a grip nothing honours: under tag or
   milestone grouping — the board's headline feature — the header is `editable:
   false`, so the lanes are not columns at all. There the column draggable is
   `:disabled`, dblclick-to-rename returns on `!canEdit` and the «…» menu isn't
   rendered, yet every lane caption still offered a grab hand, and a tag lane's
   caption is the widest target in the header. Hanging the cursor on the handle
   keeps the two in step by construction. */
.col-drag {
  cursor: grab;
}
.count {
  font-size: 12px;
  color: var(--t-text3);
  background: var(--t-hover);
  border-radius: 10px;
  padding: 0 7px;
  white-space: nowrap;
}
.count-sub {
  opacity: 0.7;
}
.col-est {
  font-size: 11px;
  color: var(--t-text3);
  white-space: nowrap;
}
.col-menu {
  font-size: 16px;
}
.col-collapse {
  font-size: 15px;
  color: var(--t-text3);
  flex: none;
}
.col-collapse:hover {
  color: var(--t-text1);
}
.settings {
  width: 220px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.settings :deep(.n-button__content) {
  white-space: nowrap;
}
.swatches {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.sw {
  appearance: none;
  -webkit-appearance: none;
  width: 22px;
  height: 22px;
  border-radius: 50%;
  border: 2px solid transparent;
  background-origin: border-box;
  cursor: pointer;
}
.sw.active {
  border-color: var(--t-text1);
}
</style>
