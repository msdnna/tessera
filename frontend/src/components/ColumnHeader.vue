<script setup>
// A column's header (v2 #2954; interaction model #2969): the name renames in
// place (double click or F2), the «⋯» and the right click open the row menu,
// and the delete happens at once — the board offers the undo in a toast.
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon, NButton, NTooltip, useMessage } from 'naive-ui'
import {
  AddOutline,
  ArrowBackOutline,
  ArrowForwardOutline,
  CheckmarkDoneOutline,
  ChevronBackOutline,
  ColorPaletteOutline,
  CreateOutline,
  EllipsisHorizontalOutline,
  TrashOutline,
} from '@vicons/ionicons5'
import TesseraIcon from './TesseraIcon.vue'
import TagPill from './TagPill.vue'
import ShortcutKeys from './ShortcutKeys.vue'
import InlineEdit from './kit/InlineEdit.vue'
import ActionMenu from './kit/ActionMenu.vue'
import { columns as columnsApi } from '@/api'
import { columnStatusName } from '@/utils/columnStatus'
import { humanizeError } from '@/utils/errors'

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
  last: { type: Boolean, default: false }, // rightmost column (no «move right»)
  collapsed: { type: Boolean, default: false }, // rendered as a narrow strip
  // The board's last status column can't be deleted: its cards would have
  // nowhere to move (the server answers 409).
  canDelete: { type: Boolean, default: true },
  // { key, n } — the board asks this header to open its rename (F2 on the
  // column, Tab from the previous header). `n` makes a repeat request count.
  renameRequest: { type: Object, default: null },
  // A tag column (grouping by tag, #2954) titles itself with the tag's own pill,
  // in the board's friendly prefix names (canonical prefix → label).
  prefixNames: { type: Object, default: () => ({}) },
})
const emit = defineEmits([
  'changed',
  'set-done',
  'toggle-collapse',
  'rename-next',
  'delete',
  'move',
  'add-right',
])

// `editable` says the lane is a real status column (tag/milestone lanes are not);
// `readonly` says the board is being *looked at*, not worked on — the archive view.
// Only the two together open the menu, the rename and the drag handle: the
// archive shows — and counts — only archived cards, so a menu acting on the
// whole column over a header reading «3» would mislead. Card DnD, column DnD and
// the card menus are already closed in the archive (KanbanBoard `archivedMode`).
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

const message = useMessage()

// ── the row menu: «⋯» or a right click ──
const menuOpen = ref(false)
const menuAt = ref(null) // { x, y } for a right click, null for the «⋯» button
const menuBtn = ref(null)
const menuAnchor = () => menuAt.value || menuBtn.value?.$el || null
const menuItems = computed(() => [
  { key: 'rename', label: t('board.column.rename'), icon: CreateOutline, keys: 'col.rename' },
  { key: 'color', label: t('board.column.color'), icon: ColorPaletteOutline, swatch: props.dcol.color || '' },
  { key: 'done', label: t('board.column.doneToggle'), icon: CheckmarkDoneOutline, toggle: props.isDone },
  { gap: true },
  { key: 'collapse', label: t('board.column.collapseShort'), icon: ChevronBackOutline, keys: 'view.collapse' },
  ...(props.first
    ? []
    : [{ key: 'moveLeft', label: t('board.column.moveLeft'), icon: ArrowBackOutline, keys: 'col.moveLeft' }]),
  ...(props.last
    ? []
    : [{ key: 'moveRight', label: t('board.column.moveRight'), icon: ArrowForwardOutline, keys: 'col.moveRight' }]),
  { key: 'addRight', label: t('board.column.addRight'), icon: AddOutline },
  { gap: true },
  {
    key: 'delete',
    label: t('board.column.delete'),
    icon: TrashOutline,
    danger: true,
    keys: 'col.delete',
    disabled: !props.canDelete,
    disabledTitle: t('board.column.lastColumn'),
  },
])
function openMenu(at = null) {
  if (!canEdit.value) return
  menuAt.value = at
  menuOpen.value = true
}
function onCtx(e) {
  if (!canEdit.value) return
  e.preventDefault()
  openMenu({ x: e.clientX, y: e.clientY })
}
function onMenu(key, value) {
  if (key === 'rename') startRename()
  else if (key === 'color') setColor(value)
  else if (key === 'done') emit('set-done', value ? props.dcol.key : null)
  else if (key === 'collapse') emit('toggle-collapse')
  else if (key === 'moveLeft') emit('move', 'left')
  else if (key === 'moveRight') emit('move', 'right')
  else if (key === 'addRight') emit('add-right')
  else if (key === 'delete') emit('delete')
}

// ── rename in place ──
const renaming = ref(false)
const nameEdit = ref('')
const renameError = ref('')
const renameBusy = ref(false)
const editor = ref(null)

function startRename() {
  if (!canEdit.value || renaming.value) return
  nameEdit.value = props.dcol.name
  renameError.value = ''
  renaming.value = true
}
watch(
  () => props.renameRequest,
  (req) => {
    if (req && req.key === props.dcol.key) startRename()
  },
)
function cancelRename() {
  renaming.value = false
  renameError.value = ''
}
async function commitRename(value, { via }) {
  if (!renaming.value || renameBusy.value) return
  const n = value.trim()
  if (!n) {
    // An empty name is never saved. Clicking away just drops the edit; ↵ and
    // Tab say why and keep the field.
    if (via === 'blur') return cancelRename()
    renameError.value = t('board.column.nameEmpty')
    editor.value?.focus()
    return
  }
  if (n !== props.dcol.name) {
    renameBusy.value = true
    try {
      await columnsApi.update(props.dcol.key, { name: n, color: props.dcol.color || '' })
      emit('changed')
    } catch (e) {
      renameBusy.value = false
      renameError.value = humanizeError(e)
      editor.value?.focus()
      return
    }
    renameBusy.value = false
  }
  cancelRename()
  if (via === 'tab') emit('rename-next')
}
async function setColor(c) {
  try {
    // dcol.name is the caption the reader sees; the column's own name is what the
    // server stores. Sending the caption back would rename the column into the UI
    // language and drop its name_key (see UpdateColumn).
    await columnsApi.update(props.dcol.key, { name: rawName.value, color: c })
    emit('changed')
  } catch (e) {
    message.error(humanizeError(e))
  }
}

defineExpose({ startRename, openMenu })
</script>

<template>
  <div class="col-head" :class="{ ro: readonly, editing: renaming }" @contextmenu="onCtx">
    <!-- The status glyph stays in the archive (it is what the column *is*), but
         `col-drag` — the Sortable handle and its grab cursor — comes off with the
         rest of the editing: the column draggable is disabled there anyway. -->
    <TesseraIcon
      v-if="editable"
      :name="statusName"
      :variant="statusVariant"
      class="col-stat"
      :class="{ 'col-drag': canEdit && !renaming }"
      :style="{ color: statusColor }"
      :title="isDone ? t('board.column.doneTitle') : readonly ? '' : t('board.column.dragTitle')"
    />
    <InlineEdit
      v-if="renaming"
      ref="editor"
      v-model="nameEdit"
      variant="header"
      hint="column"
      :error="renameError"
      :busy="renameBusy"
      :placeholder="t('board.column.namePlaceholder')"
      :hint-left="-6"
      data-testid="column-rename-input"
      @commit="commitRename"
      @cancel="cancelRename"
    />
    <TagPill v-else-if="dcol.tag" :tag="dcol.tag" :prefix-names="prefixNames" class="col-tag" />
    <!-- `title` is not decoration here: the caption is clipped to one line below,
         and a tag lane's caption is whatever the reader named the tag. -->
    <n-tooltip v-else-if="canEdit" placement="bottom-start" :delay="700" :show-arrow="false">
      <template #trigger>
        <span class="col-title col-drag can-rename" :aria-label="dcol.name" @dblclick="startRename">{{
          dcol.name
        }}</span>
      </template>
      <span class="ch-tip">
        {{ t('board.column.renameHint') }}
        <ShortcutKeys id="col.rename" />
      </span>
    </n-tooltip>
    <span
      v-else
      class="col-title"
      :class="{ neutral: !editable && !dcol.color }"
      :title="dcol.name"
      >{{ dcol.name }}</span
    >
    <template v-if="!renaming">
      <span class="count" :title="countTitle" data-testid="column-count"
        >{{ count }}<span v-if="showTotal" class="count-sub">{{ ' (' + total + ')' }}</span></span
      >
      <span v-if="estimate" class="col-est" :title="t('board.column.estimateTitle')"
        >Σ {{ estimate }}</span
      >
      <span class="col-gap" />
      <n-button
        text
        size="tiny"
        class="col-collapse"
        :title="t('board.column.collapse')"
        @click.stop="emit('toggle-collapse')"
      >
        <n-icon :component="ChevronBackOutline" />
      </n-button>
      <n-button
        v-if="canEdit"
        ref="menuBtn"
        text
        size="tiny"
        class="col-menu"
        :class="{ open: menuOpen && !menuAt }"
        data-testid="column-menu"
        @click.stop="openMenu()"
      >
        <n-icon :component="EllipsisHorizontalOutline" />
      </n-button>
    </template>

    <ActionMenu
      v-model:show="menuOpen"
      :anchor="menuAnchor"
      :placement="menuAt ? 'point' : 'bottom-end'"
      :offset="menuAt ? { x: 0, y: 0 } : { x: 0, y: 6 }"
      :width="284"
      :header="dcol.name"
      :header-aside="t('board.column.tasks', count)"
      :items="menuItems"
      testid="column-actions"
      @select="onMenu"
    />
  </div>
</template>

<style scoped>
/* v2 (#2954): a 30px row straight on the column's tint — no chips, counts in
   muted text, the controls are bare 22×24 icon buttons. */
.col-head {
  display: flex;
  align-items: center;
  gap: 5px;
  height: 30px;
  padding: 0 0 0 6px;
  flex: none;
}
.col-stat {
  font-size: 16px;
  flex: none;
}
/* The grab hand marks the Sortable handle only — the caption and the glyph carry
   it just while the column can be reordered (canEdit), never as decoration (#2932). */
.col-drag {
  cursor: grab;
}
.col-title {
  flex: 0 1 auto;
  /* min-width: 0 lets a long caption shrink (a tag name often has no break
     opportunity) instead of pushing the count, chevron and «…» over the next
     column (#2932); clipped to one line like the collapsed strip. */
  min-width: 0;
  font-size: 13px;
  font-weight: 600;
  color: var(--t-text1);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
/* «Без тегов» / «Без этапа»: the neutral catch-all column reads a step quieter. */
.col-title.neutral {
  color: var(--t-text2);
  cursor: default;
}
/* TagPill leaves its box to the call site (a scoped two-tone tag overrides the
   padding with its own segments). */
.col-tag {
  flex: 0 1 auto;
  min-width: 0;
  font-size: 12.5px;
  font-weight: 600;
  padding: 1px 9px;
  border-radius: var(--t-r-pill);
}
.count,
.col-est {
  flex: none;
  font-size: 12.5px;
  color: var(--t-text3);
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}
.count-sub {
  opacity: 0.85;
}
.col-gap {
  flex: 1 1 0;
  min-width: 0;
}
.col-collapse,
.col-menu {
  flex: none;
  width: 22px;
  height: 24px;
  border-radius: 7px;
  font-size: 14px;
  color: var(--t-text3);
}
.col-menu {
  font-size: 15px;
}
.col-collapse:hover,
.col-menu:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.col-menu.open {
  background: var(--t-active);
  color: var(--t-primary);
}
.can-rename {
  padding: 2px 5px;
  margin: 0 -5px;
  border-radius: 6px;
}
.can-rename:hover {
  background: color-mix(in srgb, var(--t-text1) 8%, transparent);
}
.col-head.editing {
  gap: 6px;
  padding-right: 4px;
}
.ch-tip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
</style>
