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
  count: { type: Number, default: 0 },
  estimate: { type: String, default: '' }, // Σ estimate (milestone grouping); '' = hide
  editable: { type: Boolean, default: false }, // status columns only
  isDone: { type: Boolean, default: false }, // task-completing column
  first: { type: Boolean, default: false }, // leftmost column (To-Do icon)
  collapsed: { type: Boolean, default: false }, // rendered as a narrow strip
})
const emit = defineEmits(['changed', 'set-done', 'toggle-collapse'])

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
  if (!props.editable) return
  e.preventDefault()
  ctxShow.value = false
  ctxX.value = e.clientX
  ctxY.value = e.clientY
  nextTick(() => (ctxShow.value = true))
}
function onCtxSelect(key) {
  ctxShow.value = false
  if (key === 'rename') startRename()
  else if (key === 'done') toggleDone()
  else if (key === 'delete') removeCol()
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
  if (!props.editable) return
  nameEdit.value = props.dcol.name
  renaming.value = true
}
async function commitRename() {
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
  <div class="col-head" @contextmenu="onCtx">
    <TesseraIcon
      v-if="editable"
      :name="statusName"
      :variant="statusVariant"
      class="col-stat col-drag"
      :style="{ color: statusColor }"
      :title="isDone ? t('board.column.doneTitle') : t('board.column.dragTitle')"
    />
    <n-input
      v-if="renaming"
      v-model:value="nameEdit"
      size="tiny"
      autofocus
      @keyup.enter="commitRename"
      @blur="commitRename"
    />
    <span v-else class="col-title col-drag" @dblclick="startRename">{{ dcol.name }}</span>
    <span class="count">{{ count }}</span>
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
    <n-popover v-if="editable" v-model:show="settingsOpen" trigger="click" placement="bottom-end">
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
  cursor: grab;
  flex: none;
}
.col-title {
  flex: 1;
  font-weight: 600;
  color: var(--t-text1);
  cursor: grab;
}
.count {
  font-size: 12px;
  color: var(--t-text3);
  background: var(--t-hover);
  border-radius: 10px;
  padding: 0 7px;
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
