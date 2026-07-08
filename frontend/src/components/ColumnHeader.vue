<script setup>
import { ref, computed, nextTick, h } from 'vue'
import { NIcon, NButton, NInput, NPopover, NPopconfirm, NDropdown, useMessage } from 'naive-ui'
import {
  EllipsisHorizontalOutline,
  TrashOutline,
  CheckmarkDoneOutline,
  CheckmarkCircle,
  EllipseOutline,
  ContrastOutline,
  CreateOutline,
  ChevronBackOutline,
} from '@vicons/ionicons5'

const menuIcon = (icon) => () => h(NIcon, null, { default: () => h(icon) })
const dangerIcon = (icon) => () => h(NIcon, { color: '#e0533d' }, { default: () => h(icon) })
import { columns as columnsApi } from '@/api'

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

// a reference tracker-style status glyph: done = check, first = empty circle, middle =
// half (in-progress). Tinted with the column colour.
const statusIcon = computed(() =>
  props.isDone ? CheckmarkCircle : props.first ? EllipseOutline : ContrastOutline,
)
// Tint the glyph with the column colour; fall back to the accent (same as the
// column's top bar) rather than a dull grey so the icon always reads as coloured.
const statusColor = computed(() => props.dcol.color || 'var(--t-primary)')
// Per-column SVG gradient so the glyph carries the same-hue diagonal gradient
// (Android parity). An SVG <linearGradient> referenced by fill/stroke: url(#id);
// applied via attribute selectors so fill-based and stroke-based ionicons each
// gradient correctly without cross-contamination. Stops use the column hue.
const gradId = computed(() => `cgrad-${props.dcol.key}`)
const glyphDark = computed(() => `color-mix(in srgb, ${statusColor.value} 86%, #000)`)
const glyphLight = computed(() => `color-mix(in srgb, ${statusColor.value} 86%, #fff)`)

function toggleDone() {
  emit('set-done', props.isDone ? null : props.dcol.key)
  settingsOpen.value = false
}

// ── right-click context menu (status columns only) ──
const ctxShow = ref(false)
const ctxX = ref(0)
const ctxY = ref(0)
const ctxOptions = computed(() => [
  { label: 'Переименовать', key: 'rename', icon: menuIcon(CreateOutline) },
  {
    label: props.isDone ? 'Снять завершение' : 'Сделать завершающей',
    key: 'done',
    icon: menuIcon(CheckmarkDoneOutline),
  },
  { type: 'divider', key: 'd1' },
  { label: 'Удалить колонку', key: 'delete', icon: dangerIcon(TrashOutline), props: { style: 'color:#e0533d' } },
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
    await columnsApi.update(props.dcol.key, { name: props.dcol.name, color: c })
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
    <svg v-if="editable" class="cg-def" width="0" height="0" aria-hidden="true">
      <defs>
        <linearGradient :id="gradId" x1="0" y1="1" x2="1" y2="0">
          <stop offset="0" :style="{ stopColor: glyphDark }" />
          <stop offset="0.5" :style="{ stopColor: statusColor }" />
          <stop offset="1" :style="{ stopColor: glyphLight }" />
        </linearGradient>
      </defs>
    </svg>
    <n-icon
      v-if="editable"
      :component="statusIcon"
      class="col-stat col-drag"
      :style="{ color: statusColor, '--icon-grad': `url(#${gradId})` }"
      :title="isDone ? 'Завершающая колонка' : 'Перетащите за заголовок'"
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
    <span v-if="estimate" class="col-est" title="Суммарная оценка задач этапа">Σ {{ estimate }}</span>
    <n-button
      text
      size="tiny"
      class="col-collapse"
      title="Свернуть колонку"
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
            :title="s || 'По умолчанию'"
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
          {{ isDone ? 'Снять завершение' : 'Сделать завершающей' }}
        </n-button>
        <n-popconfirm :positive-button-props="{ type: 'error' }" positive-text="Удалить" @positive-click="removeCol">
          <template #trigger>
            <n-button type="error" ghost size="small" block>
              <template #icon><n-icon :component="TrashOutline" /></template>
              Удалить колонку
            </n-button>
          </template>
          Удалить колонку со всеми задачами?
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
/* Paint the glyph with the per-column SVG gradient. Attribute selectors keep
   fill-based and stroke-based ionicons correct (only recolour where the icon
   used currentColor). */
.col-stat :deep(svg [fill='currentColor']) {
  fill: var(--icon-grad);
}
.col-stat :deep(svg [stroke='currentColor']) {
  stroke: var(--icon-grad);
}
.cg-def {
  position: absolute;
  width: 0;
  height: 0;
  pointer-events: none;
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
