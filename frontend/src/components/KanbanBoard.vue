<script setup>
import { ref, reactive, computed, watch, onMounted, onBeforeUnmount, nextTick, h } from 'vue'
import { useRoute } from 'vue-router'
import draggable from 'vuedraggable'
import {
  NSpin,
  NButton,
  NInput,
  NText,
  NSelect,
  NPopover,
  NCheckboxGroup,
  NCheckbox,
  NSpace,
  NIcon,
  NTooltip,
  NDropdown,
  useMessage,
} from 'naive-ui'
import {
  AlbumsOutline,
  SwapVerticalOutline,
  FilterOutline,
  GitBranchOutline,
  SearchOutline,
  CheckmarkOutline,
} from '@vicons/ionicons5'
import { boards, tasks as tasksApi, workspaces as wsApi, columns as columnsApi } from '@/api'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useBoardViewStore } from '@/stores/boardView'
import { useRealtime } from '@/composables/useRealtime'
import { useResponsive } from '@/composables/useResponsive'
import { PRIORITY_LABELS } from '@/styles/tokens'
import { storeToRefs } from 'pinia'
import TaskCard from './TaskCard.vue'
import TaskModal from './TaskModal.vue'
import TesseraSpinner from './TesseraSpinner.vue'
import ColumnHeader from './ColumnHeader.vue'
import BoardListView from './BoardListView.vue'
import BoardCalendarView from './BoardCalendarView.vue'

const props = defineProps({ boardId: { type: String, required: true } })

const message = useMessage()
const wsStore = useWorkspacesStore()
const boardViewStore = useBoardViewStore()
// `layout` lives in the store so the header switcher and the board stay in sync.
const { layout } = storeToRefs(boardViewStore)
const route = useRoute()
const { isMobile } = useResponsive()

const loading = ref(false)
const board = ref(null)
const columns = ref([])
const allTasks = ref([])
const subtasksByParent = ref({})
const lists = ref({})
const tagsMap = reactive({})
const membersMap = reactive({})
const tagsList = computed(() => Object.values(tagsMap))
const membersList = computed(() => Object.values(membersMap))

// view controls (layout comes from the store, above)
const subtasksExpanded = ref(false) // full property cards vs compact rows
const groupMode = ref('status') // 'status' | 'tag'
const sortBy = ref('position') // 'position' | 'priority' | 'due'
const filters = reactive({ priorities: [], assignees: [], tags: [], due: '', q: '' })
const sortOptions = [
  { label: 'Вручную', value: 'position' },
  { label: 'По приоритету', value: 'priority' },
  { label: 'По сроку', value: 'due' },
]
// Grouping + sort share a dropdown form (open straight from the toolbar button);
// a right-aligned check icon marks the active option.
const groupOptions = [
  { label: 'По статусам', value: 'status' },
  { label: 'По тегам', value: 'tag' },
]
const groupMenuOptions = computed(() => groupOptions.map((o) => ({ key: o.value, label: o.label })))
const sortMenuOptions = computed(() => sortOptions.map((o) => ({ key: o.value, label: o.label })))
function renderCheckLabel(active, label) {
  return h(
    'div',
    {
      style:
        'display:flex;align-items:center;justify-content:space-between;gap:28px;min-width:140px',
    },
    [
      h('span', label),
      active
        ? h(NIcon, { size: 16, style: 'color:var(--t-primary)' }, { default: () => h(CheckmarkOutline) })
        : null,
    ],
  )
}
const renderGroupLabel = (option) => renderCheckLabel(option.key === groupMode.value, option.label)
const renderSortLabel = (option) => renderCheckLabel(option.key === sortBy.value, option.label)
const dueOptions = [
  { label: 'Все', value: '' },
  { label: 'Просроченные', value: 'overdue' },
  { label: 'Сегодня', value: 'today' },
  { label: 'Ближайшая неделя', value: 'week' },
  { label: 'Со сроком', value: 'has' },
  { label: 'Без срока', value: 'none' },
]
const priorityFilterOptions = PRIORITY_LABELS.map((label, value) => ({ label, value }))
const memberFilterOptions = computed(() =>
  membersList.value.map((m) => ({ label: m.name, value: m.user_id })),
)
const tagFilterOptions = computed(() =>
  tagsList.value.map((t) => ({ label: t.name, value: t.id })),
)
const activeFilterCount = computed(
  () =>
    filters.priorities.length +
    filters.assignees.length +
    filters.tags.length +
    (filters.due ? 1 : 0) +
    (filters.q.trim() ? 1 : 0),
)
function resetFilters() {
  filters.priorities = []
  filters.assignees = []
  filters.tags = []
  filters.due = ''
  filters.q = ''
}

// ── per-board view persistence (localStorage, per device) ──
const viewKey = computed(() => `tessera_view_${props.boardId}`)
let restoring = false
function persistView() {
  if (restoring) return
  try {
    localStorage.setItem(
      viewKey.value,
      JSON.stringify({
        layout: layout.value,
        groupMode: groupMode.value,
        sortBy: sortBy.value,
        subtasksExpanded: subtasksExpanded.value,
        filters,
      }),
    )
  } catch {
    /* storage full / disabled — non-fatal */
  }
}
function restoreView() {
  restoring = true
  try {
    const raw = localStorage.getItem(viewKey.value)
    if (raw) {
      const v = JSON.parse(raw)
      if (v.layout) layout.value = v.layout
      if (v.groupMode) groupMode.value = v.groupMode
      subtasksExpanded.value = !!v.subtasksExpanded
      if (v.sortBy) sortBy.value = v.sortBy
      if (v.filters) {
        filters.priorities = v.filters.priorities || []
        filters.assignees = v.filters.assignees || []
        filters.tags = v.filters.tags || []
        filters.due = v.filters.due || ''
        filters.q = v.filters.q || ''
      }
    } else {
      resetFilters()
    }
  } catch {
    /* corrupt entry — ignore */
  } finally {
    nextTick(() => (restoring = false))
  }
}
watch([layout, groupMode, sortBy, subtasksExpanded, filters], persistView, { deep: true })

// ── adaptive column width (#7): fill the viewport, leave room for "+ колонка";
//    exactly one full-width column on mobile. We measure the real scroll
//    container (clientWidth already excludes the sidebar, padding and its own
//    scrollbar) instead of guessing from window size, so columns never overflow.
const boardScroll = ref(null)
const containerWidth = ref(0)
let ro = null
function measure() {
  if (boardScroll.value) containerWidth.value = boardScroll.value.clientWidth
}
watch(boardScroll, (el) => {
  if (!ro || !el) return
  ro.disconnect()
  ro.observe(el)
  measure()
})
const GAP = 12
const ADD_COL_W = 220
const COL_MIN = 220 // narrowest a column may get before we switch to scrolling
const COL_MAX = 420
const colWidth = computed(() => {
  const cw = containerWidth.value
  if (!cw) return 280 // pre-measure fallback
  // Mobile: one column a little under full width so the next one peeks and the
  // swipe (scroll-snap, see CSS) reads as a smooth page-turn.
  if (isMobile.value) return Math.max(cw - 28, 200)
  const n = displayColumns.value.length || 1
  // Reserve the "+ колонка" tile (status mode) + the gaps, plus a few px slack
  // so sub-pixel rounding can't trip the horizontal scrollbar.
  const reserved = (groupMode.value === 'status' ? ADD_COL_W + GAP : 0) + (n - 1) * GAP + 4
  const w = Math.floor((cw - reserved) / n)
  // Fill the viewport while comfortable; once columns would get narrower than
  // COL_MIN, stop shrinking and let the board scroll horizontally instead.
  return Math.min(Math.max(w, COL_MIN), COL_MAX)
})
const colStyleVars = computed(() => ({ '--col-w': colWidth.value + 'px' }))

// modals
const selectedTaskId = ref(null)
const showTaskModal = ref(false)
function openTask(id) {
  selectedTaskId.value = id
  showTaskModal.value = true
}

const dragging = ref(false) // reactive: also shown to cards for the nest dropzone
let suppressReloadUntil = 0
function suppress() {
  suppressReloadUntil = Date.now() + 1500
}

// ── custom edge auto-scroll during drag ──
// Sortable's built-in auto-scroll doesn't reliably scroll a nested horizontal
// container on touch. Rather than chase flaky move events, we read the position
// of Sortable's own drag image (`.sortable-fallback` on touch, `.sortable-drag`
// on desktop) each animation frame, with a dragover fallback for desktop.
const EDGE = 72 // px from a board edge that triggers scrolling
const STEP_COOLDOWN = 600 // ms between one-column steps while held at the edge
let edgeRAF = null
let pointerX = null // last desktop dragover X (touch uses the drag image)
let lastStep = 0
function onDragOver(e) {
  pointerX = e.clientX
}
function dragX() {
  // Touch: the moving clone follows the finger. (On desktop `.sortable-drag` is
  // the static original, so there we fall back to the dragover X instead.)
  const clone = document.querySelector('.sortable-fallback')
  if (clone) {
    const r = clone.getBoundingClientRect()
    return r.left + r.width / 2
  }
  return pointerX
}
let scrollIdx = null // tracked target column index (avoids reading mid-animation scrollLeft)
// Scroll exactly one column in `dir` (-1 left / +1 right), snapping to its start.
function stepColumn(dir) {
  const el = boardScroll.value
  if (!el) return
  const stride = colWidth.value + GAP
  const maxIdx = Math.round((el.scrollWidth - el.clientWidth) / stride)
  if (scrollIdx == null) scrollIdx = Math.round(el.scrollLeft / stride)
  scrollIdx = Math.max(0, Math.min(maxIdx, scrollIdx + dir))
  el.scrollTo({ left: scrollIdx * stride, behavior: 'smooth' })
}
function autoScrollTick() {
  const el = boardScroll.value
  const px = dragX()
  if (dragging.value && el && px != null) {
    const rect = el.getBoundingClientRect()
    let dir = 0
    if (px < rect.left + EDGE) dir = -1
    else if (px > rect.right - EDGE) dir = 1
    if (dir !== 0) {
      // One column per entry, then one more every cooldown if held at the edge.
      const now = performance.now()
      if (now - lastStep > STEP_COOLDOWN) {
        stepColumn(dir)
        lastStep = now
      }
    } else {
      // Centre: re-sync the target to where we actually are so the next step
      // moves exactly one column (no skipping from a mid-animation read).
      lastStep = 0
      scrollIdx = Math.round(el.scrollLeft / (colWidth.value + GAP))
    }
  }
  edgeRAF = requestAnimationFrame(autoScrollTick)
}
function onDragStart() {
  dragging.value = true
  pointerX = null
  scrollIdx = null
  lastStep = 0
  // Mobile uses scroll-snap (x mandatory) + smooth scrolling, which both revert
  // our per-frame scrollLeft nudges — disable them for the duration of the drag.
  const el = boardScroll.value
  if (el) {
    el.style.scrollSnapType = 'none'
    el.style.scrollBehavior = 'auto'
  }
  window.addEventListener('dragover', onDragOver, { passive: true })
  if (!edgeRAF) edgeRAF = requestAnimationFrame(autoScrollTick)
}
function onDragEnd() {
  dragging.value = false
  pointerX = null
  const el = boardScroll.value
  if (el) {
    el.style.scrollSnapType = ''
    el.style.scrollBehavior = ''
  }
  window.removeEventListener('dragover', onDragOver)
  if (edgeRAF) {
    cancelAnimationFrame(edgeRAF)
    edgeRAF = null
  }
}
let reloadTimer = null
function scheduleReload() {
  clearTimeout(reloadTimer)
  reloadTimer = setTimeout(() => load(props.boardId), 200)
}

async function load(id) {
  loading.value = true
  try {
    const [b, c, t, s] = await Promise.all([
      boards.get(id),
      boards.columns(id),
      boards.tasks(id),
      boards.subtasks(id),
    ])
    board.value = b.data
    columns.value = c.data || []
    allTasks.value = t.data || []
    const byParent = {}
    for (const sub of s.data || []) (byParent[sub.parent_id] ||= []).push(sub)
    subtasksByParent.value = byParent
    await loadWorkspaceMeta()
    rebuildLists()
  } catch (e) {
    message.error(e.message)
  } finally {
    loading.value = false
  }
}

async function loadWorkspaceMeta() {
  const wsId = wsStore.currentId
  if (!wsId) return
  const [tg, mem] = await Promise.all([wsApi.tags(wsId), wsApi.members(wsId)])
  for (const k of Object.keys(tagsMap)) delete tagsMap[k]
  for (const t of tg.data || []) tagsMap[t.id] = t
  for (const k of Object.keys(membersMap)) delete membersMap[k]
  for (const m of mem.data || []) membersMap[m.user_id] = m
  // Mirror tags + context to the store so the header Теги manager works.
  boardViewStore.setTags(tagsList.value)
  boardViewStore.setContext(props.boardId, wsId)
}

// Due-date predicate for the "Срок" filter.
function matchesDue(t, mode) {
  if (mode === 'none') return !t.due_date
  if (!t.due_date) return false
  if (mode === 'has') return true
  const now = new Date()
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate())
  const due = new Date(t.due_date)
  const dueDay = new Date(due.getFullYear(), due.getMonth(), due.getDate())
  const dayMs = 86400000
  if (mode === 'overdue') return dueDay < today && !t.completed_at
  if (mode === 'today') return dueDay.getTime() === today.getTime()
  if (mode === 'week') return dueDay >= today && dueDay - today <= 7 * dayMs
  return true
}

// filter + sort applied before grouping
const filteredTasks = computed(() => {
  let arr = allTasks.value
  if (filters.priorities.length) arr = arr.filter((t) => filters.priorities.includes(t.priority))
  if (filters.assignees.length)
    arr = arr.filter((t) => (t.assignee_ids || []).some((a) => filters.assignees.includes(a)))
  if (filters.tags.length)
    arr = arr.filter((t) => (t.tag_ids || []).some((id) => filters.tags.includes(id)))
  if (filters.due) arr = arr.filter((t) => matchesDue(t, filters.due))
  const q = filters.q.trim().toLowerCase()
  if (q) arr = arr.filter((t) => t.title.toLowerCase().includes(q))

  const s = [...arr]
  if (sortBy.value === 'priority') s.sort((a, b) => (b.priority || 0) - (a.priority || 0))
  else if (sortBy.value === 'due')
    s.sort(
      (a, b) =>
        (a.due_date ? Date.parse(a.due_date) : Infinity) -
        (b.due_date ? Date.parse(b.due_date) : Infinity),
    )
  return s
})

// The board's task-completing column: explicit if set, else the rightmost.
const doneColumnId = computed(() => {
  if (board.value?.done_column_id) return board.value.done_column_id
  const cols = columns.value
  return cols.length ? cols[cols.length - 1].id : null
})
async function onSetDone(columnId) {
  try {
    const r = await boards.setDoneColumn(props.boardId, columnId)
    board.value = r.data
  } catch (e) {
    message.error(e.message)
  }
}

const displayColumns = computed(() => {
  if (groupMode.value === 'status') {
    return columns.value.map((c) => ({ key: c.id, name: c.name, color: c.color, status: c }))
  }
  return [
    ...tagsList.value.map((t) => ({ key: t.id, name: t.name, color: t.color, tag: t })),
    { key: '__none__', name: 'Без тегов', color: '', tag: null },
  ]
})

function rebuildLists() {
  const map = {}
  if (groupMode.value === 'status') {
    for (const col of columns.value) map[col.id] = []
    for (const t of filteredTasks.value) (map[t.column_id] ||= []).push(t)
  } else {
    for (const tg of tagsList.value) map[tg.id] = []
    map.__none__ = []
    for (const t of filteredTasks.value) {
      const ids = t.tag_ids || []
      if (!ids.length) map.__none__.push(t)
      else for (const id of ids) if (map[id]) map[id].push(t)
    }
  }
  lists.value = map
}
watch([filteredTasks, groupMode], rebuildLists)

// Mutable mirror of displayColumns for column drag-reorder (status mode only).
const colModel = ref([])
watch(displayColumns, (v) => (colModel.value = [...v]), { immediate: true })

async function onColumnReorder(evt) {
  if (groupMode.value !== 'status') return
  const info = evt.moved || evt.added
  if (!info) return
  const arr = colModel.value
  const before = arr[info.newIndex - 1]
  const after = arr[info.newIndex + 1]
  suppress()
  try {
    await columnsApi.move(info.element.key, {
      before_id: before ? before.key : null,
      after_id: after ? after.key : null,
    })
    scheduleReload()
  } catch (e) {
    message.error(e.message)
    load(props.boardId)
  }
}

// Drag persistence: status mode = reposition; tag mode = add/remove tag.
async function onColChange(evt, dcol) {
  suppress()
  try {
    if (groupMode.value === 'status') {
      const info = evt.added || evt.moved
      if (info) {
        const arr = lists.value[dcol.key]
        const before = arr[info.newIndex - 1]
        const after = arr[info.newIndex + 1]
        // A subtask dragged out onto a column becomes top-level again.
        if (evt.added && info.element.parent_id) {
          await tasksApi.setParent(info.element.id, null)
        }
        await tasksApi.move(info.element.id, {
          column_id: dcol.key,
          before_id: before ? before.id : null,
          after_id: after ? after.id : null,
        })
      }
    } else {
      if (evt.added && dcol.tag) await tasksApi.addTag(evt.added.element.id, dcol.tag.id)
      if (evt.removed && dcol.tag) await tasksApi.removeTag(evt.removed.element.id, dcol.tag.id)
    }
    scheduleReload()
  } catch (e) {
    message.error(e.message)
    load(props.boardId)
  }
}

// ── inline task creation (a reference tracker-style "+ New task" at column bottom) ──
const addingInColumn = ref(null) // dcol.key currently adding into
const newTaskTitle = ref('')
const taskInput = ref(null)
function focusTaskInput() {
  nextTick(() => taskInput.value?.focus?.())
}
function startAddTask(dcol) {
  addingInColumn.value = dcol.key
  newTaskTitle.value = ''
  focusTaskInput()
}
function cancelAddTask() {
  addingInColumn.value = null
  newTaskTitle.value = ''
}
async function submitAddTask(dcol) {
  const title = newTaskTitle.value.trim()
  if (!title) {
    cancelAddTask()
    return
  }
  // Tag mode has no status columns to drop into → use the first board column
  // and pre-tag with the tag column's tag.
  const columnId = groupMode.value === 'status' ? dcol.key : columns.value[0]?.id
  if (!columnId) {
    message.warning('Сначала создайте хотя бы одну колонку-статус')
    return
  }
  suppress()
  newTaskTitle.value = '' // keep the input open for rapid entry
  try {
    const res = await boards.createTask(board.value.id, { column_id: columnId, title })
    if (groupMode.value === 'tag' && dcol.tag) await tasksApi.addTag(res.data.id, dcol.tag.id)
    await load(props.boardId)
    addingInColumn.value = dcol.key // load() reset refs; re-open
    focusTaskInput()
  } catch (e) {
    message.error(e.message)
  }
}

// ── inline column creation ("+ New column" to the right) ──
const addingColumn = ref(false)
const newColumnName = ref('')
const colInput = ref(null)
function startAddColumn() {
  addingColumn.value = true
  newColumnName.value = ''
  nextTick(() => colInput.value?.focus?.())
}
async function submitAddColumn() {
  const name = newColumnName.value.trim()
  if (!name) {
    addingColumn.value = false
    return
  }
  suppress()
  try {
    await boards.createColumn(board.value.id, { name })
    newColumnName.value = ''
    addingColumn.value = false
    await load(props.boardId)
  } catch (e) {
    message.error(e.message)
  }
}

// A task edit or tag-list change touches both tasks and workspace meta, so do
// a full (debounced) board reload.
function onChanged() {
  suppress()
  scheduleReload()
}

useRealtime((ev) => {
  if (ev.scope !== wsStore.currentId) return
  if (dragging.value || Date.now() < suppressReloadUntil) return
  scheduleReload()
})

// Header-hosted actions (Теги manager, Архив) ask the board to reload.
watch(
  () => boardViewStore.reloadNonce,
  () => onChanged(),
)

// Open a task when arriving via a search deep-link (?task=<id>).
function applyTaskQuery() {
  const id = route.query.task
  if (id) openTask(String(id))
}

onMounted(async () => {
  ro = new ResizeObserver(() => measure())
  if (boardScroll.value) {
    ro.observe(boardScroll.value)
    measure()
  }
  restoreView()
  await load(props.boardId)
  applyTaskQuery()
})
onBeforeUnmount(() => {
  ro?.disconnect()
  onDragEnd()
  boardViewStore.reset()
})
watch(
  () => props.boardId,
  async (id) => {
    if (!id) return
    restoreView()
    await load(id)
    applyTaskQuery()
  },
)
watch(
  () => route.query.task,
  () => applyTaskQuery(),
)
</script>

<template>
  <n-spin :show="loading" :rotate="false">
    <template #icon><TesseraSpinner /></template>
    <div v-if="board" class="board-wrap">
      <!-- Sub-toolbar under the header: grouping / sort / filters / subtasks +
           a task-name search on the right. (Layout + Теги/Архив live in the
           global header now.) -->
      <div class="subbar">
        <n-dropdown
          trigger="click"
          placement="bottom-start"
          :options="groupMenuOptions"
          :render-label="renderGroupLabel"
          @select="(k) => (groupMode = k)"
        >
          <n-button size="small" quaternary :type="groupMode === 'tag' ? 'primary' : 'default'">
            <template #icon><n-icon :component="AlbumsOutline" /></template>
            <span class="sb-label">Группировка</span>
          </n-button>
        </n-dropdown>

        <n-dropdown
          trigger="click"
          placement="bottom-start"
          :options="sortMenuOptions"
          :render-label="renderSortLabel"
          @select="(k) => (sortBy = k)"
        >
          <n-button size="small" quaternary>
            <template #icon><n-icon :component="SwapVerticalOutline" /></template>
            <span class="sb-label">Сортировка</span>
          </n-button>
        </n-dropdown>

        <n-popover trigger="click" placement="bottom-start">
          <template #trigger>
            <n-button size="small" quaternary :type="activeFilterCount ? 'primary' : 'default'">
              <template #icon><n-icon :component="FilterOutline" /></template>
              <span class="sb-label">Фильтры</span>{{
                activeFilterCount ? ` (${activeFilterCount})` : ''
              }}
            </n-button>
          </template>
          <div class="vp">
            <div v-if="activeFilterCount" class="vp-fhead">
              <n-button text size="tiny" type="primary" @click="resetFilters">Сбросить</n-button>
            </div>
            <n-text depth="3" class="flbl flbl-0">Срок</n-text>
            <n-select v-model:value="filters.due" :options="dueOptions" size="small" />
            <n-text depth="3" class="flbl">Приоритет</n-text>
            <n-checkbox-group v-model:value="filters.priorities">
              <n-space vertical :size="4">
                <n-checkbox
                  v-for="o in priorityFilterOptions"
                  :key="o.value"
                  :value="o.value"
                  :label="o.label"
                />
              </n-space>
            </n-checkbox-group>
            <n-text depth="3" class="flbl">Исполнитель</n-text>
            <n-checkbox-group v-model:value="filters.assignees">
              <n-space vertical :size="4">
                <n-checkbox
                  v-for="o in memberFilterOptions"
                  :key="o.value"
                  :value="o.value"
                  :label="o.label"
                />
              </n-space>
            </n-checkbox-group>
            <template v-if="tagFilterOptions.length">
              <n-text depth="3" class="flbl">Теги</n-text>
              <n-checkbox-group v-model:value="filters.tags">
                <n-space vertical :size="4">
                  <n-checkbox
                    v-for="o in tagFilterOptions"
                    :key="o.value"
                    :value="o.value"
                    :label="o.label"
                  />
                </n-space>
              </n-checkbox-group>
            </template>
          </div>
        </n-popover>

        <n-tooltip>
          <template #trigger>
            <n-button
              size="small"
              quaternary
              :type="subtasksExpanded ? 'primary' : 'default'"
              @click="subtasksExpanded = !subtasksExpanded"
            >
              <template #icon><n-icon :component="GitBranchOutline" /></template>
            </n-button>
          </template>
          {{ subtasksExpanded ? 'Свернуть подзадачи' : 'Развернуть подзадачи' }}
        </n-tooltip>

        <div class="subbar-spacer" />

        <n-input
          v-model:value="filters.q"
          size="small"
          placeholder="Поиск по названию"
          clearable
          class="task-search"
        >
          <template #prefix><n-icon :component="SearchOutline" /></template>
        </n-input>
      </div>

      <BoardListView
        v-if="layout === 'list'"
        :columns="displayColumns"
        :status-columns="columns"
        :lists="lists"
        :tags-map="tagsMap"
        :members-map="membersMap"
        @open="openTask"
        @changed="onChanged"
      />

      <BoardCalendarView
        v-else-if="layout === 'calendar'"
        :tasks="filteredTasks"
        :status-columns="columns"
        @open="openTask"
        @changed="onChanged"
      />

      <div v-else ref="boardScroll" class="board-scroll" :style="colStyleVars">
        <draggable
          :list="colModel"
          group="columns"
          item-key="key"
          handle=".col-drag"
          :disabled="groupMode !== 'status'"
          class="cols"
          :animation="150"
          :delay="160"
          :delay-on-touch-only="true"
          :touch-start-threshold="6"
          @start="onDragStart"
          @end="onDragEnd"
          @change="onColumnReorder"
        >
          <template #item="{ element: dcol }">
            <div class="col" :style="{ '--col-accent': dcol.color || 'var(--t-primary)' }">
              <ColumnHeader
                :dcol="dcol"
                :count="(lists[dcol.key] || []).length"
                :editable="groupMode === 'status'"
                :is-done="groupMode === 'status' && dcol.key === doneColumnId"
                :first="groupMode === 'status' && dcol.key === (colModel[0] && colModel[0].key)"
                @changed="onChanged"
                @set-done="onSetDone"
              />
              <draggable
                :list="lists[dcol.key]"
                group="tasks"
                item-key="id"
                class="drop"
                ghost-class="ghost"
                filter=".add-sub, .sub-add-input"
                :prevent-on-filter="false"
                :animation="150"
                :delay="160"
                :delay-on-touch-only="true"
                :touch-start-threshold="6"
                @start="onDragStart"
                @end="onDragEnd"
                @change="onColChange($event, dcol)"
              >
                <template #item="{ element }">
                  <div>
                    <TaskCard
                      :task="element"
                      :subtasks="subtasksByParent[element.id] || []"
                      :subtasks-expanded="subtasksExpanded"
                      :dragging="dragging"
                      :columns="columns"
                      :tags-map="tagsMap"
                      :members-map="membersMap"
                      :tags="tagsList"
                      :members="membersList"
                      :ws-id="wsStore.currentId"
                      @open="openTask"
                      @changed="onChanged"
                    />
                  </div>
                </template>
              </draggable>

              <div v-if="addingInColumn === dcol.key" class="add-task-input">
                <n-input
                  :ref="(el) => (taskInput = el)"
                  v-model:value="newTaskTitle"
                  type="textarea"
                  size="small"
                  :autosize="{ minRows: 1, maxRows: 4 }"
                  placeholder="Название задачи, Enter — создать"
                  @keyup.enter.prevent="submitAddTask(dcol)"
                  @keyup.esc="cancelAddTask"
                  @blur="submitAddTask(dcol)"
                />
              </div>
              <n-button v-else text size="tiny" class="add-btn" @click="startAddTask(dcol)">
                ＋ Создать задачу
              </n-button>
            </div>
          </template>
        </draggable>

        <!-- inline new column (status mode) -->
        <div v-if="groupMode === 'status'" class="add-col" :class="{ 'add-col-mobile': isMobile }">
          <n-input
            v-if="addingColumn"
            ref="colInput"
            v-model:value="newColumnName"
            size="small"
            placeholder="Название колонки"
            @keyup.enter="submitAddColumn"
            @keyup.esc="addingColumn = false"
            @blur="submitAddColumn"
          />
          <n-button v-else dashed block class="add-btn" @click="startAddColumn">
            ＋ Создать колонку
          </n-button>
        </div>
      </div>

      <div v-if="layout === 'board' && !displayColumns.length" class="empty-board">
        <n-text depth="3">
          {{
            groupMode === 'status'
              ? 'Нет колонок — создайте первую кнопкой «＋ Колонка».'
              : 'Нет тегов — добавьте в «Теги».'
          }}
        </n-text>
      </div>
    </div>

    <TaskModal
      v-model:show="showTaskModal"
      :task-id="selectedTaskId"
      :ws-id="wsStore.currentId"
      :tags="tagsList"
      :members="membersList"
      @changed="onChanged"
      @open="openTask"
    />

  </n-spin>
</template>

<style scoped>
.subbar {
  display: flex;
  align-items: center;
  gap: 4px;
  margin-bottom: 12px;
  padding-bottom: 8px;
  border-bottom: 1px solid var(--t-border);
}
.subbar-spacer {
  flex: 1;
}
.task-search {
  width: 220px;
  max-width: 40%;
}
/* Mobile: drop the button labels (icons only) and let the search fill the rest,
   so the sub-toolbar fits the screen instead of overflowing off the right. */
@media (max-width: 768px) {
  .sb-label {
    display: none;
  }
  .subbar-spacer {
    display: none;
  }
  .task-search {
    flex: 1;
    min-width: 0;
    max-width: none;
    margin-left: 4px;
  }
}
.vp {
  width: 240px;
  display: flex;
  flex-direction: column;
  gap: 6px;
  max-height: 70vh;
  overflow-y: auto;
}
.vp-div {
  margin: 8px 0 2px;
}
.vp-fhead {
  display: flex;
  align-items: center;
  justify-content: flex-end;
}
.flbl {
  font-size: 12px;
  margin-top: 6px;
}
.flbl-0 {
  margin-top: 0;
}
.board-scroll {
  display: flex;
  gap: 12px;
  align-items: flex-start;
  overflow-x: auto;
  padding-bottom: 8px;
}
.cols {
  display: flex;
  gap: 12px;
  align-items: flex-start;
}
.add-col {
  box-sizing: border-box;
  flex: 0 0 220px;
}
.add-col-mobile {
  flex: 0 0 var(--col-w, 220px);
}

/* Mobile: snap each column to the start for a smooth one-at-a-time swipe, and
   stretch the columns to fill the screen so the whole area is a drop target
   (offset ≈ header + sub-toolbar; tune if a gap/extra scroll appears). */
@media (max-width: 768px) {
  .board-scroll {
    scroll-snap-type: x mandatory;
    scroll-behavior: smooth;
    align-items: stretch;
    height: calc(100vh - 130px);
    height: calc(100dvh - 130px);
  }
  .cols {
    align-items: stretch;
  }
  .col,
  .add-col-mobile {
    scroll-snap-align: start;
  }
  .col {
    align-self: stretch;
    max-height: none;
  }
}
.add-task-input {
  margin-top: 4px;
}
.col {
  box-sizing: border-box;
  width: var(--col-w, 280px);
  flex: 0 0 var(--col-w, 280px);
  background: var(--t-surface-alt);
  border-radius: 10px;
  padding: 10px;
  align-self: flex-start;
  max-height: calc(100vh - 180px);
  display: flex;
  flex-direction: column;
  /* Same-hue diagonal of the column's accent for its top bar. */
  --col-grad: linear-gradient(
    to top right,
    color-mix(in srgb, var(--col-accent) 86%, #000),
    var(--col-accent) 50%,
    color-mix(in srgb, var(--col-accent) 86%, #fff)
  );
  position: relative;
}
/* Top accent bar — a thin gradient strip (replaces the flat 3px border-top). */
.col::before {
  content: '';
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  height: 3px;
  border-radius: 10px 10px 0 0;
  background: var(--col-grad);
}
.col-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;
  padding: 0 2px;
}
.col-grip {
  cursor: grab;
  color: var(--t-text3);
  font-size: 12px;
  line-height: 1;
}
.col-title {
  flex: 1;
  font-weight: 600;
  color: var(--t-text1);
}
.col-menu {
  font-size: 16px;
  line-height: 1;
}
.col-swatches {
  display: flex;
  gap: 8px;
  margin-top: 12px;
}
.cw {
  width: 24px;
  height: 24px;
  border-radius: 50%;
  border: 2px solid transparent;
  cursor: pointer;
}
.cw.active {
  border-color: var(--t-text1);
}
.cs-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.count {
  font-size: 12px;
  color: var(--t-text3);
  background: var(--t-hover);
  border-radius: 10px;
  padding: 0 7px;
}
.drop {
  flex: 1;
  overflow-y: auto;
  min-height: 8px;
}
.ghost {
  opacity: 0.5;
}
.add-btn {
  margin-top: 6px;
  width: 100%;
  justify-content: center;
  text-transform: uppercase;
  font-size: 11px;
  letter-spacing: 0.4px;
  color: var(--t-text3);
}
.empty-board {
  padding: 24px;
}
</style>
