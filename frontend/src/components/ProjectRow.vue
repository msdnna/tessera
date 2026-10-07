<script setup>
// A project in the sidebar tree, with its boards and sprint nodes (v2
// interaction model, #2969). Rows are keyboard stops (↑/↓ from the tree, ←/→
// fold, ↵ open, F2 rename, ⇧N new board, «.» menu, I/U/M on the project), names
// edit in place, a new board is an empty row, and the menus are ActionMenus
// whose small forms (address, move, icon) open as popovers at the same row.
import { ref, computed, nextTick, watch, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter, useRoute } from 'vue-router'
import { NIcon, NButton, NText, useMessage } from 'naive-ui'
import {
  EllipsisHorizontalOutline,
  ChevronForwardOutline,
  ChevronDownOutline,
  AddOutline,
  CreateOutline,
  TrashOutline,
  OpenOutline,
  TimerOutline,
  RibbonOutline,
  GitBranchOutline,
  SwapHorizontalOutline,
  LinkOutline,
  CopyOutline,
  SettingsOutline,
  ArchiveOutline,
  ColorPaletteOutline,
  EyeOutline,
} from '@vicons/ionicons5'
import { projects as projApi, boards as boardsApi } from '@/api'
import { hueGrad } from '@/utils/gradient'
import { humanizeError } from '@/utils/errors'
import { stubTask } from '@/utils/featureStubs'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useTourStore } from '@/stores/tour'
import { useBoardViewStore } from '@/stores/boardView'
import ProjectIcon from './ProjectIcon.vue'
import TesseraIcon from './TesseraIcon.vue'
import EstimationModal from './EstimationModal.vue'
import MilestoneManager from './MilestoneManager.vue'
import InlineEdit from './kit/InlineEdit.vue'
import ActionMenu from './kit/ActionMenu.vue'
import AppearancePopover from './sidebar/AppearancePopover.vue'
import ProjectSlugPopover from './sidebar/ProjectSlugPopover.vue'
import ProjectMovePopover from './sidebar/ProjectMovePopover.vue'
import { DEFAULT_ESTIMATION, resolveEstimation } from '@/utils/estimation'
import { BACKLOG_SCOPE, matchesScope, milestoneKey } from '@/utils/milestones'
import { pressMoved } from '@/utils/dnd'
import { useBoardTrash } from '@/composables/useBoardTrash'
import { useLongPress } from '@/composables/useLongPress'
import { useTreeExpand } from '@/composables/useTreeExpand'
import { handleRowKeys } from '@/composables/useRowKeys'

const props = defineProps({
  project: { type: Object, required: true },
  depth: { type: Number, default: 0 },
})

const store = useWorkspacesStore()
const tour = useTourStore()
const boardView = useBoardViewStore()
const router = useRouter()
const route = useRoute()
const message = useMessage()
const { t } = useI18n()
const tree = useTreeExpand()

// Persisted expand state; projects default closed.
const expanded = computed({
  get: () => tree.isExpanded(props.project.id, false),
  set: (v) => tree.setExpanded(props.project.id, v),
})
const rowEl = ref(null)
const boards = computed(() => store.boardsByProject[props.project.id] || [])
const initials = computed(() => (props.project.name || '?').trim().slice(0, 2).toUpperCase())
const groupName = computed(() => store.groups.find((g) => g.id === props.project.group_id)?.name || '')
const boardPath = (b) => `/project/${props.project.slug}/board/${b.slug}`
function openBoard(b) {
  router.push(boardPath(b))
}

// ── Sidebar tree mode: boards | milestones | both ──
const treeMode = computed(() => props.project.tree_mode || 'boards')
const showBoards = computed(() => treeMode.value === 'boards' || treeMode.value === 'both')
const showMilestones = computed(() => treeMode.value === 'milestones' || treeMode.value === 'both')

// Milestone (sprint) nodes are a navigation overlay over the project's board — a
// sprint opens the (first) board scoped to that milestone; "Бэклог" = no milestone.
const milestones = ref([])
async function loadMilestones() {
  if (!showMilestones.value) return
  try {
    milestones.value = (await projApi.milestones(props.project.id)).data || []
  } catch {
    milestones.value = []
  }
}

// Per-project sprint-tree prefs (non-critical UX state → localStorage, not DB).
const MS_PREF_KEY = `tessera_ms_tree_${props.project.id}`
const showClosedSprints = ref(localStorage.getItem(MS_PREF_KEY) === '1')
function toggleShowClosed() {
  showClosedSprints.value = !showClosedSprints.value
  localStorage.setItem(MS_PREF_KEY, showClosedSprints.value ? '1' : '0')
}

// Open sprints first, then by due date descending (recent/active on top).
const displayMilestones = computed(() => {
  let list = milestones.value.slice()
  if (!showClosedSprints.value) list = list.filter((m) => m.state !== 'closed')
  list.sort((a, b) => {
    const ao = a.state === 'closed' ? 1 : 0
    const bo = b.state === 'closed' ? 1 : 0
    if (ao !== bo) return ao - bo
    const ad = a.due_date ? new Date(a.due_date).getTime() : 0
    const bd = b.due_date ? new Date(b.due_date).getTime() : 0
    return bd - ad
  })
  return list
})

// Milestone navigation targets the project's first board (the overlay board).
const msBoard = computed(() => boards.value[0] || null)
function openMilestone(m) {
  const b = msBoard.value
  if (!b) {
    message.warning(t('project.milestone.noBoard'))
    return
  }
  router.push({ path: boardPath(b), query: { milestone: m ? milestoneKey(m) : BACKLOG_SCOPE } })
}
function msActive(m) {
  const scope = String(route.query.milestone || '')
  return (
    route.params.projectSlug === props.project.slug &&
    (m ? matchesScope(m, scope) : scope === BACKLOG_SCOPE)
  )
}

async function ensureBoards() {
  if (!store.boardsByProject[props.project.id]) await store.loadBoards(props.project.id)
}
async function toggle() {
  expanded.value = !expanded.value
  if (expanded.value) await ensureBoards()
  if (expanded.value) loadMilestones()
}

// settings apply immediately (mirrors the column header pattern)
async function updateField(patch) {
  try {
    await projApi.update(props.project.id, {
      name: props.project.name,
      color: props.project.color || '',
      icon: props.project.icon || '',
      icon_mode: props.project.icon_mode || 'badge',
      tree_mode: props.project.tree_mode || 'boards',
      group_id: props.project.group_id || null,
      ...patch,
    })
    await store.refresh()
    return true
  } catch (e) {
    message.error(humanizeError(e))
    return false
  }
}

// ── project rename in place ──
const renaming = ref(false)
const nameEdit = ref('')
const renameBusy = ref(false)
function startRename() {
  nameEdit.value = props.project.name
  renaming.value = true
}
async function commitRename(value, { via }) {
  const n = value.trim()
  if (!n || n === props.project.name) {
    // An empty name is never saved: ↵ does nothing, a click away gives up.
    if (!n && via === 'enter') return
    renaming.value = false
    return
  }
  renameBusy.value = true
  const ok = await updateField({ name: n })
  renameBusy.value = false
  if (ok) {
    renaming.value = false
    rowEl.value?.focus()
  }
}
function cancelRename() {
  renaming.value = false
  rowEl.value?.focus()
}

// Icon colouring: "badge" tints the box (legacy; no colour ⇒ primary badge),
// "icon" leaves the box transparent and tints the glyph instead.
const iconMode = computed(() => props.project.icon_mode === 'icon')
const colored = computed(() => props.project.color && props.project.color !== 'transparent')
const boxStyle = computed(() => {
  if (iconMode.value || props.project.color === 'transparent') return { background: 'transparent' }
  return { background: hueGrad(props.project.color || 'var(--t-primary)') }
})
const bare = computed(() => iconMode.value || props.project.color === 'transparent')
const glyphColor = computed(() =>
  iconMode.value ? (colored.value ? props.project.color : 'var(--t-primary)') : '',
)

// Board-node icon (same badge/icon logic per board): a board can carry its own
// icon/colour (set in the board customize panel); default is the kanban glyph.
const boardColored = (b) => b.color && b.color !== 'transparent'
const boardHasIcon = (b) => !!(b.icon || boardColored(b))
const boardBare = (b) => b.icon_mode === 'icon' || b.color === 'transparent'
const boardBox = (b) =>
  boardBare(b)
    ? { background: 'transparent' }
    : { background: hueGrad(b.color || 'var(--t-primary)') }
const boardGlyph = (b) =>
  b.icon_mode === 'icon' ? (boardColored(b) ? b.color : 'var(--t-primary)') : ''
const boardInitials = (b) => (b.name || '?').trim().slice(0, 2).toUpperCase()
const boardActive = (b) =>
  route.params.projectSlug === props.project.slug &&
  route.params.boardSlug === b.slug &&
  !route.query.milestone

// ── project delete: irreversible → two steps in the menu ──
const deleting = ref(false)
const deleteFailed = ref('')
async function doRemove() {
  deleting.value = true
  deleteFailed.value = ''
  try {
    await projApi.remove(props.project.id)
    projMenuOpen.value = false
    // If a board of this project is open, it would start 404-ing — go Home first.
    if (route.params.projectSlug === props.project.slug) router.push('/')
    await store.refresh()
  } catch (e) {
    deleteFailed.value = humanizeError(e)
  } finally {
    deleting.value = false
  }
}

// ── the project menu ──
const projMenuOpen = ref(false)
const projMenuAt = ref(null)
const projMenuArm = ref('')
function openProjectMenu(at = null, arm = '') {
  projMenuAt.value = at
  projMenuArm.value = arm
  projMenuOpen.value = true
}
function onProjectCtx(e) {
  if (pressMoved()) return
  openProjectMenu({ x: e.clientX, y: e.clientY })
}
const TREE_MODES = ['boards', 'milestones', 'both']
const TREE_LABEL = { boards: 'treeBoards', milestones: 'treeMilestones', both: 'treeBoth' }
const projMenuItems = computed(() => [
  { key: 'open', label: t('project.board.open'), icon: OpenOutline, keys: 'row.open' },
  { key: 'add-board', label: t('project.menu.addBoard'), icon: AddOutline, keys: 'row.new' },
  { gap: true },
  { key: 'rename', label: t('project.menu.rename'), icon: CreateOutline, keys: 'row.rename' },
  { key: 'icon', label: t('project.menu.appearance'), icon: ColorPaletteOutline, keys: 'row.icon' },
  {
    key: 'slug',
    label: t('project.menu.slug'),
    icon: LinkOutline,
    keys: 'row.slug',
    disabled: !store.canManage,
    disabledTitle: t('project.menu.managerOnly'),
  },
  { key: 'move', label: t('project.menu.move'), icon: SwapHorizontalOutline, keys: 'row.move' },
  { gap: true },
  { key: 'copy-link', label: t('project.menu.copyLink'), icon: CopyOutline, keys: 'row.copyLink' },
  {
    key: 'settings',
    label: t('project.menu.settings'),
    icon: SettingsOutline,
    keys: 'row.settings',
    stub: stubTask('project.window'),
  },
  // Until the project window (#2971) takes them, the project's own settings
  // stay reachable here.
  { key: 'milestones', label: t('project.menu.milestones'), icon: RibbonOutline },
  { key: 'estimation', label: t('project.menu.estimation'), icon: TimerOutline },
  {
    key: 'tree-mode',
    label: `${t('project.menu.treeMode')}: ${t(`project.menu.${TREE_LABEL[treeMode.value]}`).toLowerCase()}`,
    icon: GitBranchOutline,
  },
  ...(showMilestones.value
    ? [{ key: 'toggle-closed', label: t('project.menu.showClosedMilestones'), icon: EyeOutline, toggle: showClosedSprints.value }]
    : []),
  { gap: true },
  {
    key: 'archive',
    label: t('project.menu.archive'),
    icon: ArchiveOutline,
    keys: 'row.archive',
    stub: stubTask('project.archive'),
  },
  {
    key: 'delete',
    label: t('project.menu.delete'),
    icon: TrashOutline,
    danger: true,
    keys: 'row.delete',
    disabled: !store.canManage,
    disabledTitle: t('project.menu.managerOnly'),
    confirm: {
      title: t('project.remove.armed', { name: props.project.name }),
      sub: boards.value.length
        ? t('project.remove.armedSubBoards', { boards: t('project.remove.boards', boards.value.length) })
        : t('project.remove.armedSub'),
    },
    busy: deleting.value,
    failed: deleteFailed.value ? { title: t('project.remove.failed'), sub: deleteFailed.value } : null,
  },
])
async function onProjectMenu(key, value) {
  if (key === 'open') openProject()
  else if (key === 'add-board') startAddBoard()
  else if (key === 'rename') startRename()
  else if (key === 'icon') openForm('icon', true)
  else if (key === 'slug') openForm('slug', true)
  else if (key === 'move') openForm('move', true)
  else if (key === 'copy-link') copyLink()
  else if (key === 'milestones') msShow.value = true
  else if (key === 'estimation') estShow.value = true
  else if (key === 'tree-mode') {
    const next = TREE_MODES[(TREE_MODES.indexOf(treeMode.value) + 1) % TREE_MODES.length]
    updateField({ tree_mode: next })
  } else if (key === 'toggle-closed') {
    if (value !== showClosedSprints.value) toggleShowClosed()
  } else if (key === 'delete') doRemove()
}

async function openProject() {
  await ensureBoards()
  expanded.value = true
  if (boards.value[0]) openBoard(boards.value[0])
}
async function copyLink() {
  await ensureBoards()
  const b = boards.value[0]
  const url = `${window.location.origin}${b ? boardPath(b) : '/'}`
  try {
    await navigator.clipboard.writeText(url)
    message.success(t('project.menu.linkCopied'))
  } catch {
    message.error(t('project.menu.linkCopyFailed'))
  }
}

// ── popover forms at the project row: icon & colour, address, move ──
const form = ref('')
const formFromMenu = ref(false)
function openForm(name, fromMenu = false) {
  if (name === 'slug' && !store.canManage) return
  formFromMenu.value = fromMenu
  form.value = name
}
const formShow = (name) =>
  computed({
    get: () => form.value === name,
    set: (v) => {
      if (!v && form.value === name) form.value = ''
    },
  })
const iconShow = formShow('icon')
const slugShow = formShow('slug')
const moveShow = formShow('move')
// Esc in a form opened from the menu steps back to the menu.
function formBack() {
  form.value = ''
  openProjectMenu(projMenuAt.value)
}
const rowAnchor = () => rowEl.value

// Milestone manager for this project.
const msShow = ref(false)

// Estimation override editor for this project (inherits the workspace default).
const estShow = ref(false)
const estInherited = computed(() => resolveEstimation(null, store.current) || DEFAULT_ESTIMATION)

// ── boards: rename in place, menu, delete with undo ──
const editingBoardId = ref(null)
const boardNameEdit = ref('')
const boardRenameBusy = ref(false)
const boardRows = {}
function startBoardRename(b) {
  editingBoardId.value = b.id
  boardNameEdit.value = b.name
}
function cancelBoardRename(b) {
  editingBoardId.value = null
  nextTick(() => boardRows[b.id]?.focus())
}
async function commitBoardRename(b, value, { via }) {
  if (editingBoardId.value !== b.id) return
  const n = value.trim()
  if (!n || n === b.name) {
    if (!n && via === 'enter') return
    editingBoardId.value = null
    return
  }
  boardRenameBusy.value = true
  try {
    await boardsApi.update(b.id, { name: n })
    await store.loadBoards(props.project.id)
    editingBoardId.value = null
    nextTick(() => boardRows[b.id]?.focus())
  } catch (e) {
    message.error(humanizeError(e))
  } finally {
    boardRenameBusy.value = false
  }
}
// Deleting a board happens at once and offers the undo in a toast (#2969): the
// server keeps it restorable (migration 0078).
const boardTrash = useBoardTrash()
function removeBoard(b) {
  return boardTrash.removeBoard(props.project, b)
}

const boardMenuOpen = ref(false)
const boardMenuAt = ref(null)
const boardMenuTarget = ref(null)
function openBoardMenu(b, at = null) {
  boardMenuTarget.value = b
  boardMenuAt.value = at
  boardMenuOpen.value = true
}
function onBoardCtx(e, b) {
  openBoardMenu(b, { x: e.clientX, y: e.clientY })
}
const boardMenuAnchor = () =>
  boardMenuAt.value || (boardMenuTarget.value && boardRows[boardMenuTarget.value.id]) || null
const boardMenuItems = computed(() => [
  { key: 'open', label: t('project.board.open'), icon: OpenOutline, keys: 'row.open' },
  { key: 'rename', label: t('project.board.rename'), icon: CreateOutline, keys: 'row.rename' },
  { gap: true },
  { key: 'delete', label: t('project.board.delete'), icon: TrashOutline, danger: true, keys: 'row.delete' },
])
function onBoardMenu(key) {
  const b = boardMenuTarget.value
  if (!b) return
  if (key === 'open') openBoard(b)
  else if (key === 'rename') startBoardRename(b)
  else if (key === 'delete') removeBoard(b)
}

// Touch long-press → context menus (the native contextmenu is unreliable on the
// draggable rows inside the mobile drawer).
const lpProj = useLongPress(onProjectCtx)
let bTimer = null
function bStart(e, b) {
  const tt = e.touches && e.touches[0]
  if (!tt) return
  const x = tt.clientX
  const y = tt.clientY
  clearTimeout(bTimer)
  bTimer = setTimeout(() => {
    if (!pressMoved()) onBoardCtx({ clientX: x, clientY: y }, b)
  }, 450)
}
function bCancel() {
  clearTimeout(bTimer)
}
onBeforeUnmount(bCancel)

// Load boards whenever the project is (or becomes) expanded — covers both the
// initial mount and expand state restored from persistence AFTER mount (which
// doesn't go through toggle(), so an onMounted-only check missed it and left
// "нет досок" until a manual collapse+expand).
watch(
  expanded,
  (v) => {
    if (v && !store.boardsByProject[props.project.id]) store.loadBoards(props.project.id)
    if (v) loadMilestones()
  },
  { immediate: true },
)
// Reload sprint nodes when the tree mode flips to include milestones.
watch(showMilestones, (v) => {
  if (v && expanded.value) loadMilestones()
})
// …and when a sprint changed anywhere — somebody else's edit, or the reader's own
// from the board / MilestoneManager. The layout owns the socket and bumps this
// revision (stores/workspaces.js onMilestoneEvent); a collapsed project has no
// list to refresh, and loadMilestones() no-ops when sprints aren't shown.
watch(
  () => store.milestoneRevFor(props.project.id),
  () => {
    if (expanded.value) loadMilestones()
  },
)

// ── new board: an empty row at the end of the project's boards ──
// ↵ creates it and opens it, ⇧↵ creates it and offers another empty row, a
// click away creates it without opening, Esc (or an empty click-away) drops it.
// The kind chip (Kanban / List) is a stub until boards carry a kind (#3017).
const addingBoard = ref(false)
const newBoardName = ref('')
const newBoardBusy = ref(false)
const newBoardError = ref('')
const newBoardEditor = ref(null)
function startAddBoard() {
  expanded.value = true
  newBoardName.value = ''
  newBoardError.value = ''
  addingBoard.value = true
}
function cancelAddBoard() {
  addingBoard.value = false
  newBoardName.value = ''
}
async function commitNewBoard(value, { via, shift }) {
  if (via === 'tab') return // the kind switch — not built yet (#3017)
  const n = value.trim()
  if (!n) {
    if (via !== 'enter') cancelAddBoard()
    return
  }
  newBoardBusy.value = true
  newBoardError.value = ''
  try {
    const res = await projApi.createBoard(props.project.id, { name: n })
    // Point the Get Started guide's next step at this exact board (#2753 rework).
    tour.noteCreated({ boardId: res.data?.id })
    await store.loadBoards(props.project.id)
    expanded.value = true
    if (via === 'enter' && shift) {
      newBoardName.value = ''
      newBoardEditor.value?.focus()
    } else {
      addingBoard.value = false
      newBoardName.value = ''
      if (via === 'enter' && res.data?.slug) openBoard(res.data)
    }
  } catch (e) {
    newBoardError.value = humanizeError(e)
    newBoardEditor.value?.focus()
  } finally {
    newBoardBusy.value = false
  }
}

// ── keys of the focused rows ──
function onProjectKey(e) {
  if (renaming.value) return
  if (!e.metaKey && !e.ctrlKey && !e.altKey && (e.key === 'ArrowRight' || e.key === 'ArrowLeft')) {
    e.preventDefault()
    if (e.key === 'ArrowRight' && !expanded.value) toggle()
    else if (e.key === 'ArrowLeft') {
      if (expanded.value) expanded.value = false
      else rowEl.value?.closest('.group-node')?.querySelector('[data-tree-row]')?.focus()
    }
    return
  }
  handleRowKeys(e, {
    'row.open': openProject,
    'row.rename': startRename,
    'row.renameR': startRename,
    'row.new': startAddBoard,
    'row.menu': () => openProjectMenu(),
    'row.menuF10': () => openProjectMenu(),
    'row.icon': () => openForm('icon'),
    'row.slug': () => openForm('slug'),
    'row.move': () => openForm('move'),
    'row.copyLink': copyLink,
    'row.delete': () => store.canManage && openProjectMenu(null, 'delete'),
  })
}
function onBoardKey(e, b) {
  if (editingBoardId.value === b.id) return
  if (e.key === 'ArrowLeft' && !e.metaKey && !e.ctrlKey && !e.altKey) {
    e.preventDefault()
    rowEl.value?.focus()
    return
  }
  handleRowKeys(e, {
    'row.open': () => openBoard(b),
    'row.rename': () => startBoardRename(b),
    'row.renameR': () => startBoardRename(b),
    'row.new': startAddBoard,
    'row.menu': () => openBoardMenu(b),
    'row.menuF10': () => openBoardMenu(b),
    'row.delete': () => removeBoard(b),
  })
}
</script>

<template>
  <div class="project-block" :data-tour-project="project.id">
    <div
      ref="rowEl"
      class="row project-row"
      :class="{ ctx: projMenuOpen || !!form, editing: renaming }"
      data-tour="project-row"
      data-tree-row
      tabindex="-1"
      :aria-expanded="expanded"
      @keydown="onProjectKey"
      @contextmenu.prevent.stop="onProjectCtx"
      @touchstart.passive="lpProj.start"
      @touchend="lpProj.cancel"
      @touchcancel="lpProj.cancel"
    >
      <n-icon
        class="chev"
        :class="{ open: expanded }"
        :component="ChevronForwardOutline"
        @click="toggle"
      />
      <span class="picon" :class="{ 'picon-bare': bare }" :style="boxStyle" @click="toggle">
        <ProjectIcon :icon="project.icon" :initials="initials" :size="13" :color="glyphColor" />
      </span>
      <InlineEdit
        v-if="renaming"
        v-model="nameEdit"
        :busy="renameBusy"
        :hint-left="-34"
        @commit="commitRename"
        @cancel="cancelRename"
      />
      <span v-else class="name" @click="toggle" @dblclick="startRename">{{ project.name }}</span>

      <template v-if="!renaming">
        <n-button
          class="hover-btn"
          text
          size="tiny"
          :title="t('project.board.add')"
          data-tour="board-add"
          @click.stop="startAddBoard"
        >
          <n-icon :component="AddOutline" />
        </n-button>
        <n-button
          class="hover-btn"
          :class="{ open: projMenuOpen && !projMenuAt }"
          text
          size="tiny"
          data-testid="project-menu"
          @click.stop="openProjectMenu()"
        >
          <n-icon :component="EllipsisHorizontalOutline" />
        </n-button>
      </template>
    </div>

    <div v-if="expanded" class="boards">
      <!-- Sprint (milestone) nodes: a navigation overlay over the project's board.
           Shown when tree mode includes milestones. The backlog node = tasks with
           no sprint. -->
      <template v-if="showMilestones">
        <div
          v-for="m in displayMilestones"
          :key="m.id"
          class="row board-row ms-row"
          :class="{ active: msActive(m), closed: m.state === 'closed' }"
          data-tree-row
          tabindex="-1"
          @click="openMilestone(m)"
          @keydown.enter.prevent="openMilestone(m)"
        >
          <span class="chev-spacer" />
          <span class="bicon"><n-icon :component="RibbonOutline" /></span>
          <span class="name">{{ m.title }}</span>
        </div>
        <div
          class="row board-row ms-row"
          :class="{ active: msActive(null) }"
          data-tree-row
          tabindex="-1"
          @click="openMilestone(null)"
          @keydown.enter.prevent="openMilestone(null)"
        >
          <span class="chev-spacer" />
          <span class="bicon"><n-icon :component="GitBranchOutline" /></span>
          <span class="name">{{ t('project.milestone.backlog') }}</span>
        </div>
        <n-text v-if="!displayMilestones.length" depth="3" class="empty">{{
          t('project.milestone.empty')
        }}</n-text>
      </template>

      <template v-if="showBoards">
        <div
          v-for="b in boards"
          :key="b.id"
          :ref="(el) => (boardRows[b.id] = el)"
          class="row board-row"
          data-tour="board-row"
          data-tree-row
          tabindex="-1"
          :data-tour-board="b.id"
          :class="{
            active: boardActive(b),
            ctx: boardMenuOpen && boardMenuTarget?.id === b.id,
            editing: editingBoardId === b.id,
          }"
          @click="editingBoardId !== b.id && openBoard(b)"
          @keydown="onBoardKey($event, b)"
          @contextmenu.prevent.stop="onBoardCtx($event, b)"
          @touchstart.passive="bStart($event, b)"
          @touchend="bCancel"
          @touchcancel="bCancel"
        >
          <span class="chev-spacer" />
          <span class="bicon">
            <span
              v-if="boardHasIcon(b)"
              class="picon bicon-box"
              :class="{ 'picon-bare': boardBare(b) }"
              :style="boardBox(b)"
            >
              <ProjectIcon
                v-if="b.icon"
                :icon="b.icon"
                :initials="boardInitials(b)"
                :size="12"
                :color="boardGlyph(b)"
              />
              <TesseraIcon
                v-else
                name="layout-kanban"
                :variant="boardBare(b) ? 'outline' : 'filled'"
                :size="12"
                :style="boardBare(b) && boardGlyph(b) ? { color: boardGlyph(b) } : {}"
              />
            </span>
            <TesseraIcon v-else name="layout-kanban" :size="15" />
          </span>
          <InlineEdit
            v-if="editingBoardId === b.id"
            v-model="boardNameEdit"
            :busy="boardRenameBusy"
            :hint-left="-6"
            @commit="(v, how) => commitBoardRename(b, v, how)"
            @cancel="cancelBoardRename(b)"
          />
          <span v-else class="name" @dblclick.stop="startBoardRename(b)">{{ b.name }}</span>
          <template v-if="editingBoardId !== b.id">
            <!-- The open board's task count — what its filter leaves (v2, as in the
                 mockup). It gives its place to «⋯» under the pointer. -->
            <span
              v-if="boardView.boardId === b.id && boardView.shownTasks != null"
              class="bcount"
              data-testid="board-count"
              :title="boardView.shownTitle"
              >{{ boardView.shownTasks }}</span
            >
            <n-button
              class="hover-btn"
              :class="{ open: boardMenuOpen && boardMenuTarget?.id === b.id && !boardMenuAt }"
              text
              size="tiny"
              data-testid="board-menu"
              @click.stop="openBoardMenu(b)"
            >
              <n-icon :component="EllipsisHorizontalOutline" />
            </n-button>
          </template>
        </div>
        <div v-if="addingBoard" class="row board-row new-board" data-testid="new-board-row">
          <span class="chev-spacer" />
          <span class="bicon"><TesseraIcon name="layout-kanban" :size="15" /></span>
          <InlineEdit
            ref="newBoardEditor"
            v-model="newBoardName"
            hint="createView"
            :busy="newBoardBusy"
            :error="newBoardError"
            :placeholder="t('project.board.namePlaceholder')"
            :hint-left="-6"
            data-tour="board-name"
            @commit="commitNewBoard"
            @cancel="cancelAddBoard"
          >
            <template #trailing>
              <span
                class="kind-chip"
                :title="t('common.stub.soon', { n: stubTask('board.newKind') })"
                aria-disabled="true"
              >
                <TesseraIcon name="layout-kanban" :size="12" />
                <n-icon :component="ChevronDownOutline" :size="10" />
              </span>
            </template>
          </InlineEdit>
        </div>
        <n-text v-if="!boards.length && !addingBoard" depth="3" class="empty">{{
          t('project.board.empty')
        }}</n-text>
      </template>
    </div>

    <ActionMenu
      v-model:show="projMenuOpen"
      :anchor="() => projMenuAt || rowEl"
      :placement="projMenuAt ? 'point' : 'right-start'"
      :header="project.name"
      :header-aside="groupName"
      :items="projMenuItems"
      :arm="projMenuArm"
      testid="project-actions"
      @select="onProjectMenu"
    />
    <ActionMenu
      v-model:show="boardMenuOpen"
      :anchor="boardMenuAnchor"
      :placement="boardMenuAt ? 'point' : 'right-start'"
      :width="260"
      :header="boardMenuTarget?.name || ''"
      :items="boardMenuItems"
      testid="board-actions"
      @select="onBoardMenu"
    />

    <AppearancePopover
      v-model:show="iconShow"
      :anchor="rowAnchor"
      :subtitle="project.name"
      :icon="project.icon"
      :color="project.color"
      :mode="project.icon_mode || 'badge'"
      :initials="initials"
      allow-upload
      :escape-back="formFromMenu"
      @escape="formBack"
      @update:icon="updateField({ icon: $event })"
      @update:color="updateField({ color: $event })"
      @update:mode="updateField({ icon_mode: $event })"
    />
    <ProjectSlugPopover
      v-model:show="slugShow"
      :anchor="rowAnchor"
      :project="project"
      :escape-back="formFromMenu"
      @escape="formBack"
    />
    <ProjectMovePopover
      v-model:show="moveShow"
      :anchor="rowAnchor"
      :project="project"
      :escape-back="formFromMenu"
      @escape="formBack"
    />

    <EstimationModal
      v-model:show="estShow"
      scope="project"
      :target-id="project.id"
      :name="project.name"
      :value="project.estimation || null"
      :inherited="estInherited"
    />
    <MilestoneManager
      v-model:show="msShow"
      :project-id="project.id"
      :project-name="project.name"
      :ws-id="store.currentId"
    />
  </div>
</template>

<style scoped>
.row {
  display: flex;
  align-items: center;
  gap: 6px;
  min-height: 32px;
  box-sizing: border-box;
  padding: 0 8px;
  border-radius: var(--t-r-ctl);
  cursor: pointer;
  font-size: 13.5px;
}
.row:hover {
  background: var(--t-hover-side);
}
.hover-btn {
  opacity: 0;
}
.project-row:hover .hover-btn,
.board-row:hover .hover-btn {
  opacity: 1;
}
.chev {
  width: 14px;
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: var(--t-text3);
  transition: transform 0.15s;
  font-size: 12px;
}
.chev.open {
  transform: rotate(90deg);
}
/* Leaf rows (boards) have no chevron — keep the column so their icon/text line
   up with rows that do. */
.chev-spacer {
  width: 14px;
  flex: none;
}
.picon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  border-radius: 6px;
  color: #fff;
  font-size: 10px;
  line-height: 1;
  font-weight: 700;
  white-space: nowrap;
  overflow: hidden;
  flex: none;
}
/* Board (leaf) icon box — same footprint as project/group icons. */
.bicon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  color: var(--t-text3);
  flex: none;
}
/* No coloured square — let a custom icon sit on the panel; keep glyph/initials
   readable. */
.picon-bare {
  color: var(--t-text1);
}
.name {
  flex: 1;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  color: var(--t-text1);
}
/* Match the group→child gutter (SidebarNode .children) so the whole tree shares
   one indentation style — indent only, no guide line (v2, #2953). */
.boards {
  margin-left: 14px;
}
.board-row {
  font-size: 13px;
  color: var(--t-text2);
}
.bcount {
  flex: none;
  font-size: 12px;
  font-weight: 500;
  color: var(--t-text3);
  font-variant-numeric: tabular-nums;
}
.board-row:hover .bcount {
  display: none;
}
/* the hidden «⋯» keeps no width, so the count sits flush right */
.board-row .hover-btn {
  flex: none;
}
.board-row:not(:hover) .hover-btn {
  width: 0;
  overflow: hidden;
}
.board-row.active {
  background: var(--t-active);
  color: var(--t-text1);
  font-weight: 600;
}
.board-row.active .bicon :deep([fill='currentColor']) {
  fill: url(#t-accent-grad-svg);
}
.board-row.active .bicon :deep([stroke='currentColor']) {
  stroke: url(#t-accent-grad-svg);
}
.ms-row .bicon {
  color: var(--t-primary);
}
.ms-row.closed {
  opacity: 0.62;
}
.ms-row.closed .bicon {
  color: var(--t-text3);
}
.empty {
  font-size: 12px;
  padding: 2px 8px;
}
.row {
  outline: 0;
}
.row:focus-visible {
  background: var(--t-hover-side);
  box-shadow: inset 0 0 0 2px var(--t-primary);
}
.row.ctx {
  background: var(--t-hover);
  box-shadow: var(--t-sh-card);
}
.row.ctx .hover-btn,
.row:focus-visible .hover-btn {
  opacity: 1;
}
.row.editing {
  padding-right: 4px;
}
.hover-btn.open {
  opacity: 1 !important;
  background: var(--t-active);
  color: var(--t-primary);
  border-radius: 7px;
}
.kind-chip {
  flex: none;
  display: inline-flex;
  align-items: center;
  gap: 2px;
  height: 20px;
  padding: 0 4px;
  border-radius: 5px;
  background: var(--t-pill);
  color: var(--t-text2);
  cursor: not-allowed;
  opacity: 0.8;
}
</style>
