<script setup>
// «Список» (v2, #2973 / #3020; mockups ListView, PackageD frame 5, MobileD).
//
// The board's lanes as groups — status, tag or stage, the same grouping as the
// kanban — each a header and a tray tinted with the lane's colour, the rows in
// it. A header folds its group (the board's own column collapse, per layout).
// The field columns (priority, due, people, estimate, tags) are resized by
// dragging the header edge; the widths are the board's state and part of a
// saved view (`list.widths`).
//
// Keyboard (desktop): ↑/↓ walk headers and rows, ⇧↑/↓ grow a selection, ←/→
// fold / unfold the group, ↵ opens (a header: folds), F2 renames in place,
// E / P / D / A / L / S as on the board, ⌘↑/↓ re-orders inside the status
// column, ⌘←/→ moves to the next lane, C types a new task into the group.
// The cursor ring and its key hint show only once the keyboard is in use; a
// pointer — or a phone — never sees them. ⇧-click / ⌘-click select with the mouse.
//
// Only the rows near the viewport are mounted: the rows have one height, so
// the geometry is plain arithmetic (utils/listView) — a 500-task board scrolls
// like a short one.
import { computed, nextTick, onBeforeUnmount, onMounted, provide, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import {
  AddOutline,
  ChevronDownOutline,
  ChevronForwardOutline,
  TrailSignOutline,
} from '@vicons/ionicons5'
import { useTaskMenu } from '@/composables/useTaskMenu'
import TaskActionMenu from './task/TaskActionMenu.vue'
import { useDateLocale } from '@/composables/useDateLocale'
import { useResponsive } from '@/composables/useResponsive'
import { useShortcuts } from '@/composables/useShortcuts'
import { isTaskOverdue } from '@/utils/taskFilter'
import { hueGrad } from '@/utils/gradient'
import { sumLaneEffort, formatEstimate } from '@/utils/estimation'
import { columnStatusName } from '@/utils/columnStatus'
import { LIST_FIELDS, LIST_WIDTHS, clampListWidth } from '@/utils/boardViews'
import {
  listLayout,
  trayWindow,
  spotTop,
  spotKey,
  navOrder,
  stepCursor,
  rangeBetween,
  reorderNeighbours,
} from '@/utils/listView'
import ListRow from './list/ListRow.vue'
import TesseraIcon from './TesseraIcon.vue'
import InlineEdit from './kit/InlineEdit.vue'

const props = defineProps({
  // The board's lanes, in order: [{ key, name, color, status?, tag?, milestone? }].
  columns: { type: Array, default: () => [] },
  // Real board status columns [{ id, name }] for the "move to column" menu.
  statusColumns: { type: Array, default: () => [] },
  // Map lane key -> task[] (already filtered & sorted).
  lists: { type: Object, default: () => ({}) },
  // The lane grouping: status | tag | milestone.
  groupMode: { type: String, default: 'status' },
  doneColumnId: { type: String, default: null },
  // Header counts per lane: { tasks, total } (total counts subtasks too).
  counts: { type: Object, default: () => ({}) },
  subtasksByParent: { type: Object, default: () => ({}) },
  estCfg: { type: Object, default: null },
  tagsMap: { type: Object, default: () => ({}) },
  membersMap: { type: Object, default: () => ({}) },
  // Workspace GitLab roster (`boardView.gitlabMembersList`), for the assignees who
  // have no Tessera account — same prop the timeline and the Gantt take.
  gitlabMembers: { type: Array, default: () => [] },
  tagPrefixNames: { type: Object, default: () => ({}) },
  // Field-column widths: { priority, due, people, estimate, tags }.
  widths: { type: Object, default: () => ({ ...LIST_WIDTHS }) },
  // Folded lanes (the board's per-layout column collapse).
  isFolded: { type: Function, default: () => false },
  // ⌘↑/↓ re-orders only where the list shows the stored order.
  canReorder: { type: Boolean, default: false },
  // The board's task actions (KanbanBoard): rename, create, complete, priority,
  // shift, reorder, field. Each may return a promise.
  actions: { type: Object, default: () => ({}) },
  // Archive view (?archived=1): look, don't touch.
  readonly: { type: Boolean, default: false },
})
const emit = defineEmits(['open', 'changed', 'restore', 'fold', 'resize'])

const { t } = useI18n()
const { isMobile } = useResponsive()

const menu = useTaskMenu({
  onOpen: (id) => emit('open', id),
  onChanged: () => emit('changed'),
  // The archive's own action: this layout has no card, so the menu is the only
  // place «Вернуть из архива» can live (see useTaskMenu).
  onRestore: (id) => emit('restore', id),
  columns: () => props.statusColumns,
  readonly: () => props.readonly,
})

const { formatDue: fmtDue, formatters } = useDateLocale()
// Everything a row reads from the board, in one object for all rows.
provide('listCtx', {
  fmtDue: (v) => fmtDue(v),
  // Shared predicate: this view and the board show the same rows, so they agree
  // about which of them are late (in the calendar fmtDue prints).
  isOverdue: (task) => isTaskOverdue(task, new Date(), formatters.value),
  tagsMap: props.tagsMap,
  membersMap: props.membersMap,
  prefixNames: props.tagPrefixNames,
  gitlabMembers: () => props.gitlabMembers,
  estCfg: () => props.estCfg,
})

// ── geometry ──
const METRICS = {
  desktop: { head: 38, headGap: 6, row: 40, gap: 2, pad: 4, empty: 32 },
  mobile: { head: 40, headGap: 8, row: 56, gap: 2, pad: 4, empty: 36 },
}
const m = computed(() => (isMobile.value ? METRICS.mobile : METRICS.desktop))
const folded = (key) => {
  const dcol = props.columns.find((c) => c.key === key)
  return dcol ? !!props.isFolded(dcol) : false
}
const addingKey = ref(null)
const layout = computed(() =>
  listLayout({
    groups: props.columns,
    lists: props.lists,
    folded,
    extra: (key) => (addingKey.value === key ? m.value.row : 0),
    m: m.value,
  }),
)

const scroller = ref(null)
const body = ref(null)
const scrollTop = ref(0)
const viewH = ref(800)
const bodyTop = ref(0)
let raf = 0
function measure() {
  const el = scroller.value
  if (!el) return
  scrollTop.value = el.scrollTop
  viewH.value = el.clientHeight || 800
  bodyTop.value = body.value ? body.value.offsetTop : 0
}
function onScroll() {
  if (raf) return
  raf = requestAnimationFrame(() => {
    raf = 0
    measure()
  })
}
let ro = null
onMounted(() => {
  measure()
  if (typeof ResizeObserver !== 'undefined' && scroller.value) {
    ro = new ResizeObserver(measure)
    ro.observe(scroller.value)
  }
})
onBeforeUnmount(() => {
  ro?.disconnect()
  cancelAnimationFrame(raf)
})

const MARGIN = 600
const view = computed(() => {
  const lo = scrollTop.value - bodyTop.value - MARGIN
  const hi = scrollTop.value - bodyTop.value + viewH.value + MARGIN
  const step = m.value.row + m.value.gap
  return layout.value.blocks.map((b, gi) => {
    const dcol = props.columns[gi]
    const list = props.lists[b.key] || []
    const [i0, i1] = trayWindow(b, lo, hi, m.value)
    const rows = []
    for (let i = i0; i < i1; i++) {
      rows.push({ task: list[i], top: m.value.pad + i * step, spot: { g: b.key, id: list[i].id } })
    }
    return { ...b, dcol, rows }
  })
})

// ── groups: glyph, colour, counts ──
const NEUTRAL = '#9aa0aa'
const groupColor = (dcol) => dcol.color || NEUTRAL
function groupGlyph(dcol, index) {
  if (props.groupMode !== 'status') return null
  const isDone = dcol.key === props.doneColumnId
  return {
    name: columnStatusName({
      isDone,
      first: index === 0,
      name: dcol.rawName ?? dcol.name,
      nameKey: dcol.status?.name_key,
    }),
    variant: isDone ? 'filled' : 'outline',
  }
}
function countText(key) {
  const c = props.counts[key]
  const n = c ? c.tasks : (props.lists[key] || []).length
  return c && c.total > c.tasks ? `${n} (${c.total})` : String(n)
}
const effort = computed(() => {
  const out = {}
  for (const dcol of props.columns) {
    const v = sumLaneEffort(props.lists[dcol.key] || [], props.subtasksByParent)
    out[dcol.key] = v ? formatEstimate(v, props.estCfg) : ''
  }
  return out
})

// ── field columns and their resize ──
const widthOf = (k) => props.widths?.[k] ?? LIST_WIDTHS[k]
const fieldCols = computed(() => LIST_FIELDS.map((k) => `${widthOf(k)}px`).join(' '))
const gridCols = computed(() => `28px 52px minmax(0, 1fr) ${fieldCols.value}`)
let resizing = null
function onResizeDown(key, e) {
  e.preventDefault()
  e.stopPropagation()
  resizing = { key, x: e.clientX, w: widthOf(key) }
  window.addEventListener('pointermove', onResizeMove)
  window.addEventListener('pointerup', onResizeUp)
}
function onResizeMove(e) {
  if (!resizing) return
  emit('resize', resizing.key, clampListWidth(resizing.w + e.clientX - resizing.x))
}
function onResizeUp() {
  resizing = null
  window.removeEventListener('pointermove', onResizeMove)
  window.removeEventListener('pointerup', onResizeUp)
}
onBeforeUnmount(onResizeUp)
function resetWidth(key) {
  emit('resize', key, LIST_WIDTHS[key])
}

// ── selection: a cursor (header or row) and a set of selected rows ──
const order = computed(() => navOrder(props.columns, props.lists, folded))
const cursor = ref(null)
const anchor = ref(null)
const picked = ref([])
const pickedKeys = computed(() => new Set(picked.value.map(spotKey)))
// The cursor ring and its hint: keyboard only, never on a phone.
const kbd = ref(false)
function onAnyPointer() {
  kbd.value = false
}
onMounted(() => window.addEventListener('pointerdown', onAnyPointer, true))
onBeforeUnmount(() => window.removeEventListener('pointerdown', onAnyPointer, true))

// A row the filter, a move or a delete took away drops out of the selection.
watch(order, (o) => {
  const live = new Set(o.map(spotKey))
  if (cursor.value && !live.has(spotKey(cursor.value))) cursor.value = null
  if (anchor.value && !live.has(spotKey(anchor.value))) anchor.value = null
  if (picked.value.some((s) => !live.has(spotKey(s))))
    picked.value = picked.value.filter((s) => live.has(spotKey(s)))
})

const taskOf = (spot) =>
  spot && spot.id != null ? (props.lists[spot.g] || []).find((x) => x.id === spot.id) || null : null
// What an action works on: the selected rows, or the cursor row alone.
function targets() {
  const spots = picked.value.length ? picked.value : cursor.value?.id != null ? [cursor.value] : []
  const seen = new Set()
  const out = []
  for (const s of spots) {
    const task = taskOf(s)
    if (task && !seen.has(task.id)) {
      seen.add(task.id)
      out.push({ task, laneKey: s.g })
    }
  }
  return out
}

function reveal() {
  nextTick(() => {
    const el = scroller.value
    const at = cursor.value && spotTop(layout.value, cursor.value, props.lists, m.value)
    if (!el || !at) return
    const head = bodyTop.value
    const top = at.top + head
    const sticky = isMobile.value ? 0 : head
    if (top - sticky < el.scrollTop) el.scrollTop = Math.max(0, top - sticky - 6)
    else if (top + at.height > el.scrollTop + el.clientHeight)
      el.scrollTop = top + at.height - el.clientHeight + 6
  })
}
function step(dir) {
  // The first key after the mouse shows where the cursor is, without moving it.
  if (!kbd.value && cursor.value) {
    kbd.value = true
    reveal()
    return
  }
  kbd.value = true
  cursor.value = stepCursor(order.value, cursor.value, dir)
  anchor.value = cursor.value
  picked.value = []
  reveal()
}
function extend(dir) {
  kbd.value = true
  if (!cursor.value) {
    step(dir)
    return
  }
  if (!anchor.value) anchor.value = cursor.value
  cursor.value = stepCursor(order.value, cursor.value, dir)
  picked.value = rangeBetween(order.value, anchor.value, cursor.value)
  reveal()
}
function fold(open) {
  const g = cursor.value?.g
  const dcol = g && props.columns.find((c) => c.key === g)
  if (!dcol) return
  if (!!props.isFolded(dcol) === !open) return
  emit('fold', dcol)
  // Folding a group puts the cursor on its header — its rows are gone.
  if (!open) cursor.value = { g, id: null }
  reveal()
}
function onPick(spot, e) {
  if (e?.shiftKey && anchor.value) {
    picked.value = rangeBetween(order.value, anchor.value, spot)
    cursor.value = spot
    return
  }
  if (e?.metaKey || e?.ctrlKey) {
    const k = spotKey(spot)
    const base = picked.value.length ? picked.value : cursor.value?.id != null ? [cursor.value] : []
    picked.value = base.some((s) => spotKey(s) === k)
      ? base.filter((s) => spotKey(s) !== k)
      : [...base, spot]
    cursor.value = spot
    anchor.value = spot
    return
  }
  cursor.value = spot
  anchor.value = spot
  picked.value = []
}
function clear() {
  cursor.value = null
  anchor.value = null
  picked.value = []
}
function openCursor() {
  const c = cursor.value
  if (!c) return
  if (c.id == null) {
    const dcol = props.columns.find((d) => d.key === c.g)
    if (dcol) emit('fold', dcol)
    return
  }
  emit('open', c.id)
}

// ── actions ──
const run = (name, ...args) => props.actions?.[name]?.(...args)
function completeTargets() {
  const list = targets().map((x) => x.task)
  if (list.length) run('complete', list)
}
function priorityTargets(value) {
  const list = targets().map((x) => x.task)
  if (list.length) run('priority', list, value)
}
function shiftTargets(dir) {
  const items = targets()
  if (!items.length) return
  // The rows ride along into their new lane, the cursor with them.
  const one = items.length === 1 && cursor.value?.id != null ? items[0] : null
  Promise.resolve(run('shift', items, dir)).then(() => {
    if (!one) return
    const at = props.columns.findIndex((c) => c.key === one.laneKey)
    const to = props.columns[at + (dir === 'left' ? -1 : 1)]
    if (to && (props.lists[to.key] || []).some((x) => x.id === one.task.id)) {
      cursor.value = { g: to.key, id: one.task.id }
      anchor.value = cursor.value
      picked.value = []
      reveal()
    }
  })
}
function reorder(dir) {
  const c = cursor.value
  const task = taskOf(c)
  if (!task || !props.canReorder) return
  const plan = reorderNeighbours(props.lists[c.g] || [], task.id, dir)
  if (plan) Promise.resolve(run('reorder', { task, laneKey: c.g, ...plan })).then(reveal)
}
function openField(field) {
  const task = taskOf(cursor.value)
  if (task) run('field', task, field)
}

// ── in-place rename (F2 / double click) ──
const editing = ref(null) // spot key of the row being renamed
const draft = ref('')
function startRename(spot) {
  const task = taskOf(spot)
  if (!task || props.readonly || isMobile.value) return
  cursor.value = spot
  anchor.value = spot
  picked.value = []
  draft.value = task.title
  editing.value = spotKey(spot)
}
async function commitRename(spot, value) {
  const task = taskOf(spot)
  editing.value = null
  if (task) await run('rename', task, value)
}

// ── a new task in a group (C, «＋ Задача») ──
const addDraft = ref('')
function startAdd(dcol) {
  if (props.readonly || !dcol) return
  if (props.isFolded(dcol)) emit('fold', dcol)
  addDraft.value = ''
  addingKey.value = dcol.key
  nextTick(() => {
    const b = layout.value.blocks.find((x) => x.key === dcol.key)
    const el = scroller.value
    if (b?.tray && el) {
      const y = b.tray.top + b.tray.addTop + bodyTop.value
      if (y + m.value.row > el.scrollTop + el.clientHeight)
        el.scrollTop = y + m.value.row - el.clientHeight + 12
    }
  })
}
async function commitAdd(dcol, value, info) {
  const title = (value || '').trim()
  if (!title) {
    addingKey.value = null
    return
  }
  addDraft.value = ''
  // Rapid entry: ↵ keeps the field open for the next title; leaving it closes.
  if (info?.via !== 'enter') addingKey.value = null
  await run('create', dcol, title)
}
function addHere() {
  const g = cursor.value?.g
  const dcol =
    props.columns.find((c) => c.key === g) || props.columns.find((c) => !props.isFolded(c))
  startAdd(dcol)
}

// ── keys ──
const listOn = () => !isMobile.value && !editing.value && !addingKey.value
const hasTargets = () => picked.value.length > 0 || (kbd.value && cursor.value?.id != null)
useShortcuts(
  {
    'list.up': () => step(-1),
    'list.down': () => step(1),
    'list.extendUp': () => extend(-1),
    'list.extendDown': () => extend(1),
    'list.collapse': () => fold(false),
    'list.expand': () => fold(true),
  },
  { when: listOn },
)
useShortcuts(
  { 'card.open': openCursor, 'card.clear': clear },
  { when: () => listOn() && (kbd.value || picked.value.length > 0) && !!cursor.value },
)
useShortcuts(
  {
    'card.complete': completeTargets,
    'card.priority': priorityTargets,
    'card.priorityPick': () => openField('priority'),
    'card.due': () => openField('due'),
    'card.assignees': () => openField('assignees'),
    'card.tags': () => openField('tags'),
    'card.status': () => openField('status'),
    'card.moveLeft': () => shiftTargets('left'),
    'card.moveRight': () => shiftTargets('right'),
    'list.rename': () => cursor.value && startRename(cursor.value),
    'list.moveUp': () => reorder(-1),
    'list.moveDown': () => reorder(1),
  },
  { when: () => listOn() && !props.readonly && hasTargets() },
)
useShortcuts({ 'board.addTask': addHere }, { when: () => listOn() && !props.readonly })

const cursorKey = computed(() => (kbd.value && !isMobile.value ? spotKey(cursor.value) : ''))
// The hint sits over the row; at the top of the viewport (under the sticky
// captions) it goes under the row instead.
const hintBelow = (r, b) => bodyTop.value + b.tray.top + r.top - scrollTop.value < 66
defineExpose({ startRename, startAdd })
</script>

<template>
  <div class="list-view" :class="{ mobile: isMobile }">
    <div ref="scroller" class="lv-scroll" data-testid="list-view" @scroll="onScroll">
      <div v-if="!isMobile" class="lv-cols" :style="{ gridTemplateColumns: gridCols }">
        <span />
        <span>{{ t('board.list.col.number') }}</span>
        <span>{{ t('board.list.col.task') }}</span>
        <span
          v-for="k in LIST_FIELDS"
          :key="k"
          class="lv-col"
          :class="{ right: k === 'estimate' }"
          :data-testid="`list-col-${k}`"
        >
          {{ t(`board.list.col.${k}`) }}
          <span
            class="lv-grip"
            :title="t('board.list.resize')"
            @pointerdown="onResizeDown(k, $event)"
            @dblclick.stop="resetWidth(k)"
          />
        </span>
      </div>

      <div ref="body" class="lv-body" :style="{ height: `${layout.height}px` }">
        <template v-for="(b, gi) in view" :key="b.key">
          <div
            class="lv-ghead"
            :class="{ cursor: cursorKey === `${b.key}::` }"
            :style="{ top: `${b.headTop}px`, '--gc': groupColor(b.dcol) }"
            data-testid="list-group"
            :data-group="b.dcol.rawName ?? b.dcol.name"
            @click="emit('fold', b.dcol)"
          >
            <n-icon
              class="lv-chev"
              :component="b.folded ? ChevronForwardOutline : ChevronDownOutline"
              :size="14"
            />
            <TesseraIcon
              v-if="groupGlyph(b.dcol, gi)"
              :name="groupGlyph(b.dcol, gi).name"
              :variant="groupGlyph(b.dcol, gi).variant"
              class="lv-glyph"
              :style="{ color: groupColor(b.dcol) }"
            />
            <n-icon
              v-else-if="groupMode === 'milestone'"
              class="lv-glyph ms"
              :component="TrailSignOutline"
              :size="15"
            />
            <span v-else class="lv-dot" :style="{ background: hueGrad(groupColor(b.dcol)) }" />
            <span class="lv-gname">{{ b.dcol.name }}</span>
            <span class="lv-gmeta">{{ countText(b.key) }}</span>
            <span v-if="effort[b.key]" class="lv-gmeta">Σ {{ effort[b.key] }}</span>
            <span class="lv-gspace" />
            <button
              v-if="!readonly && !b.folded"
              type="button"
              class="lv-gadd"
              data-testid="list-group-add"
              @click.stop="startAdd(b.dcol)"
            >
              <n-icon :component="AddOutline" :size="14" />
              <span v-if="!isMobile">{{ t('board.list.addTask') }}</span>
            </button>
          </div>

          <div
            v-if="b.tray"
            class="lv-tray"
            :style="{
              top: `${b.tray.top}px`,
              height: `${b.tray.height}px`,
              '--gc': groupColor(b.dcol),
            }"
          >
            <ListRow
              v-for="r in b.rows"
              :key="r.task.id"
              v-model:draft="draft"
              class="lv-row"
              :style="{ top: `${r.top}px` }"
              :task="r.task"
              :mobile="isMobile"
              :cols="fieldCols"
              :tags-width="widthOf('tags')"
              :selected="pickedKeys.has(spotKey(r.spot))"
              :cursor="cursorKey === spotKey(r.spot)"
              :hint="cursorKey === spotKey(r.spot) && editing !== spotKey(r.spot)"
              :hint-below="hintBelow(r, b)"
              :editing="editing === spotKey(r.spot)"
              :readonly="readonly"
              @pick="(e) => onPick(r.spot, e)"
              @open="emit('open', r.task.id)"
              @toggle="run('complete', [r.task])"
              @rename="startRename(r.spot)"
              @commit="(v) => commitRename(r.spot, v)"
              @cancel="editing = null"
              @menu="menu.open($event, r.task)"
              @people="run('field', r.task, 'assignees')"
            />
            <div v-if="!b.count && addingKey !== b.key" class="lv-empty">
              {{ t('board.list.empty') }}
            </div>
            <div
              v-if="addingKey === b.key"
              class="lv-add"
              :style="{ top: `${b.tray.addTop}px`, height: `${m.row}px` }"
            >
              <InlineEdit
                v-model="addDraft"
                hint="create"
                :placeholder="t('board.column.taskPlaceholder')"
                data-testid="list-add-input"
                @commit="(v, info) => commitAdd(b.dcol, v, info)"
                @cancel="addingKey = null"
              />
            </div>
          </div>
        </template>
      </div>
    </div>

    <TaskActionMenu :menu="menu" />
  </div>
</template>

<style scoped>
.list-view {
  width: 100%;
}
.lv-scroll {
  position: relative;
  overflow: auto;
  max-height: calc(100vh - 150px);
  max-height: calc(100dvh - 150px);
  padding: 0 2px 24px;
  scrollbar-width: thin;
}
/* The field captions: sticky over the rows, no line under them. */
.lv-cols {
  position: sticky;
  top: 0;
  z-index: 4;
  display: grid;
  align-items: center;
  gap: 10px;
  height: 30px;
  padding: 0 14px 0 12px;
  background: var(--t-bg);
  font-size: 11.5px;
  font-weight: 600;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  color: var(--t-text3);
  white-space: nowrap;
}
.lv-col {
  position: relative;
  min-width: 0;
  overflow: visible;
  text-overflow: ellipsis;
}
.lv-col.right {
  text-align: right;
}
/* The edge to drag: the gap right of the caption. */
.lv-grip {
  position: absolute;
  top: -4px;
  right: -8px;
  width: 6px;
  height: 22px;
  border-radius: 3px;
  cursor: col-resize;
}
.lv-grip:hover {
  background: color-mix(in srgb, var(--t-primary) 35%, transparent);
}
.lv-body {
  position: relative;
}
.lv-ghead {
  position: absolute;
  left: 0;
  right: 0;
  display: flex;
  align-items: center;
  gap: 8px;
  height: 38px;
  padding: 0 10px 0 8px;
  box-sizing: border-box;
  border-radius: 9px;
  cursor: pointer;
}
.lv-ghead:hover {
  background: var(--t-hover);
}
.lv-ghead.cursor {
  box-shadow: inset 0 0 0 2px var(--t-primary);
}
.lv-chev {
  flex: 0 0 28px;
  color: var(--t-text3);
}
.lv-glyph {
  flex: none;
  width: 16px;
  height: 16px;
}
.lv-glyph.ms {
  color: var(--t-text3);
}
.lv-dot {
  flex: none;
  width: 8px;
  height: 8px;
  margin: 0 4px;
  border-radius: 3px;
}
.lv-gname {
  font-size: 13.5px;
  font-weight: 650;
  color: var(--t-text1);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.lv-gmeta {
  flex: none;
  font-size: 12.5px;
  color: var(--t-text3);
  white-space: nowrap;
}
.lv-gspace {
  flex: 1;
}
.lv-gadd {
  display: flex;
  align-items: center;
  gap: 6px;
  height: 26px;
  padding: 0 8px;
  border: none;
  border-radius: 8px;
  background: transparent;
  color: var(--t-text3);
  font: inherit;
  font-size: 12.5px;
  cursor: pointer;
}
.lv-gadd:hover {
  background: var(--t-ctl);
  color: var(--t-text1);
}
/* The group's tray: a tint of the lane's colour over the column surface. */
.lv-tray {
  position: absolute;
  left: 0;
  right: 0;
  border-radius: 14px;
  background: color-mix(in srgb, var(--gc) 6%, var(--t-col));
}
.lv-row {
  position: absolute;
  left: 4px;
  right: 4px;
}
.lv-empty {
  position: absolute;
  left: 4px;
  right: 4px;
  top: 4px;
  height: 32px;
  display: flex;
  align-items: center;
  padding-left: 46px;
  font-size: 12.5px;
  color: var(--t-text3);
}
.lv-add {
  position: absolute;
  left: 4px;
  right: 4px;
  display: flex;
  align-items: center;
  padding: 0 10px 0 88px;
}
.lv-add > * {
  flex: 1;
}

/* ── phone: no field captions, the header and the rows taller ── */
.list-view.mobile .lv-scroll {
  max-height: calc(var(--app-vh, 100vh) - var(--m-head-h, 52px) - var(--mnav-h, 0px) - 78px);
  padding: 0 0 80px;
}
.list-view.mobile .lv-ghead {
  height: 40px;
  padding: 0 6px;
}
.list-view.mobile .lv-ghead:hover {
  background: transparent;
}
.list-view.mobile .lv-chev {
  flex-basis: 20px;
}
.list-view.mobile .lv-gname {
  font-size: 15px;
}
.list-view.mobile .lv-add {
  padding-left: 40px;
}
.list-view.mobile .lv-empty {
  padding-left: 40px;
}
</style>
