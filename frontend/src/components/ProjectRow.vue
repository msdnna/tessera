<script setup>
import { ref, computed, nextTick, watch, onBeforeUnmount, h } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import {
  NIcon,
  NButton,
  NInput,
  NPopover,
  NPopconfirm,
  NText,
  NDropdown,
  NModal,
  NCard,
  NSelect,
  useMessage,
} from 'naive-ui'
import {
  EllipsisHorizontalOutline,
  ChevronForwardOutline,
  AddOutline,
  CreateOutline,
  TrashOutline,
  OpenOutline,
  TimerOutline,
  RibbonOutline,
  GitBranchOutline,
  CheckmarkOutline,
  CheckboxOutline,
  SquareOutline,
  SwapHorizontalOutline,
  LinkOutline,
} from '@vicons/ionicons5'

const menuIcon = (icon) => () => h(NIcon, null, { default: () => h(icon) })
// Red icon for destructive menu entries (the label is reddened via the option's
// props.style; NIcon needs the colour set explicitly — it doesn't inherit it).
const dangerIcon = (icon) => () => h(NIcon, { color: '#e0533d' }, { default: () => h(icon) })
// Orange (warning) icon for dangerous-but-not-destructive entries (e.g. transfer).
const warnIcon = (icon) => () => h(NIcon, { color: '#f0a020' }, { default: () => h(icon) })
// Primary-tinted icon for the "selected" checkmark / checkbox in the tree menu.
const primIcon = (icon) => () => h(NIcon, { color: 'var(--t-primary)' }, { default: () => h(icon) })
import { projects as projApi, boards as boardsApi } from '@/api'
import { hueGrad } from '@/utils/gradient'
import { makeSlug } from '@/utils/slug'
import { useWorkspacesStore } from '@/stores/workspaces'
import ProjectIcon from './ProjectIcon.vue'
import TesseraIcon from './TesseraIcon.vue'
import IconColorPicker from './IconColorPicker.vue'
import ConfirmByName from './ConfirmByName.vue'
import EstimationModal from './EstimationModal.vue'
import MilestoneManager from './MilestoneManager.vue'
import { DEFAULT_ESTIMATION, resolveEstimation } from '@/utils/estimation'
import { pressMoved } from '@/utils/dnd'
import { useLongPress } from '@/composables/useLongPress'
import { useTreeExpand } from '@/composables/useTreeExpand'

const props = defineProps({
  project: { type: Object, required: true },
  depth: { type: Number, default: 0 },
})

const store = useWorkspacesStore()
const router = useRouter()
const route = useRoute()
const message = useMessage()
const tree = useTreeExpand()

// Persisted expand state; projects default closed.
const expanded = computed({
  get: () => tree.isExpanded(props.project.id, false),
  set: (v) => tree.setExpanded(props.project.id, v),
})
const addingBoard = ref(false)
const newBoardName = ref('')
const renaming = ref(false)
const nameEdit = ref('')
const renameInput = ref(null)
const boardInput = ref(null)
const settingsShow = ref(false)

// board inline edit
const editingBoardId = ref(null)
const boardNameEdit = ref('')
const boardEditInput = ref(null)
function startBoardRename(b) {
  editingBoardId.value = b.id
  boardNameEdit.value = b.name
  nextTick(() => boardEditInput.value?.focus?.())
}
async function commitBoardRename(b) {
  editingBoardId.value = null
  const n = boardNameEdit.value.trim()
  if (!n || n === b.name) return
  try {
    await boardsApi.update(b.id, { name: n })
    await store.loadBoards(props.project.id)
  } catch (e) {
    message.error(e.message)
  }
}
async function removeBoard(b) {
  try {
    await boardsApi.remove(b.id)
    await store.loadBoards(props.project.id)
  } catch (e) {
    message.error(e.message)
  }
}

const boards = computed(() => store.boardsByProject[props.project.id] || [])
const initials = computed(() => (props.project.name || '?').trim().slice(0, 2).toUpperCase())

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
    message.warning('В проекте нет доски для отображения этапа')
    return
  }
  router.push({
    path: `/project/${props.project.slug}/board/${b.slug}`,
    query: { milestone: m ? m.id : 'backlog' },
  })
}
function msActive(m) {
  return (
    route.params.projectSlug === props.project.slug &&
    String(route.query.milestone || '') === (m ? m.id : 'backlog')
  )
}

async function toggle() {
  expanded.value = !expanded.value
  if (expanded.value && !store.boardsByProject[props.project.id]) {
    await store.loadBoards(props.project.id)
  }
  if (expanded.value) loadMilestones()
}

// inline rename — click-outside saves if changed, else cancels
function startRename() {
  settingsShow.value = false
  nameEdit.value = props.project.name
  renaming.value = true
  nextTick(() => renameInput.value?.focus())
}
async function commitRename() {
  renaming.value = false
  const n = nameEdit.value.trim()
  if (!n || n === props.project.name) return
  await updateField({ name: n })
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
  } catch (e) {
    message.error(e.message)
  }
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
const confirmDelete = ref(false)
function remove() {
  confirmDelete.value = true
}
async function doRemove() {
  try {
    await projApi.remove(props.project.id)
    // If a board of this project is open, it would start 404-ing — go Home first.
    if (route.params.projectSlug === props.project.slug) router.push('/')
    await store.refresh()
  } catch (e) {
    message.error(e.message)
  }
}

// right-click menus (project row + board rows)
const pcShow = ref(false)
const pcX = ref(0)
const pcY = ref(0)
const pcOptions = computed(() => {
  const opts = [
    { label: 'Новая доска', key: 'add-board', icon: menuIcon(AddOutline) },
    { type: 'divider', key: 'd1' },
    { label: 'Переименовать', key: 'rename', icon: menuIcon(CreateOutline) },
    { label: 'Оценка задач…', key: 'estimation', icon: menuIcon(TimerOutline) },
    // Manager-only — hidden for members, and refused server-side regardless.
    ...(store.canManage
      ? [{ label: 'Изменить адрес…', key: 'slug', icon: menuIcon(LinkOutline) }]
      : []),
    { label: 'Этапы…', key: 'milestones', icon: menuIcon(RibbonOutline) },
    {
      label: 'Показывать в дереве',
      key: 'tree-mode',
      icon: menuIcon(GitBranchOutline),
      children: [
        {
          label: 'Доски',
          key: 'tm-boards',
          icon: treeMode.value === 'boards' ? primIcon(CheckmarkOutline) : undefined,
        },
        {
          label: 'Этапы',
          key: 'tm-milestones',
          icon: treeMode.value === 'milestones' ? primIcon(CheckmarkOutline) : undefined,
        },
        {
          label: 'Доски и этапы',
          key: 'tm-both',
          icon: treeMode.value === 'both' ? primIcon(CheckmarkOutline) : undefined,
        },
      ],
    },
  ]
  if (showMilestones.value) {
    opts.push({
      label: 'Показывать закрытые этапы',
      key: 'toggle-closed',
      icon: primIcon(showClosedSprints.value ? CheckboxOutline : SquareOutline),
    })
  }
  opts.push({ type: 'divider', key: 'd2' })
  opts.push({
    label: 'Перенести в другое пространство',
    key: 'transfer',
    icon: warnIcon(SwapHorizontalOutline),
    props: { style: 'color:#f0a020' },
  })
  opts.push({
    label: 'Удалить проект',
    key: 'delete',
    icon: dangerIcon(TrashOutline),
    props: { style: 'color:#e0533d' },
  })
  return opts
})
function onProjectCtx(e) {
  if (pressMoved()) return
  pcShow.value = false
  pcX.value = e.clientX
  pcY.value = e.clientY
  nextTick(() => (pcShow.value = true))
}
function onProjectCtxSelect(key) {
  pcShow.value = false
  if (key === 'add-board') startAddBoard()
  else if (key === 'rename') startRename()
  else if (key === 'estimation') estShow.value = true
  else if (key === 'slug') openSlugEdit()
  else if (key === 'milestones') msShow.value = true
  else if (key === 'tm-boards') updateField({ tree_mode: 'boards' })
  else if (key === 'tm-milestones') updateField({ tree_mode: 'milestones' })
  else if (key === 'tm-both') updateField({ tree_mode: 'both' })
  else if (key === 'toggle-closed') toggleShowClosed()
  else if (key === 'transfer') openTransfer()
  else if (key === 'delete') remove()
}

// ── Change the project's URL address (owner/admin) ──
// The address is assigned once at creation and deliberately survives renames,
// so links keep working. Changing it here is the escape hatch — at the cost of
// every link already handed out, hence the warning.
const slugShow = ref(false)
const slugInput = ref('')
const slugError = ref('')
const slugBusy = ref(false)
const slugPreview = computed(() => makeSlug(slugInput.value))
function openSlugEdit() {
  settingsShow.value = false
  slugInput.value = props.project.slug || ''
  slugError.value = ''
  slugShow.value = true
}
async function doSetSlug() {
  const next = slugPreview.value
  if (!next || slugBusy.value) return
  if (next === props.project.slug) {
    slugShow.value = false
    return
  }
  slugBusy.value = true
  slugError.value = ''
  try {
    const res = await projApi.setSlug(props.project.id, next)
    slugShow.value = false
    // Keep an open board of this project reachable: same board, new address.
    if (route.params.projectSlug === props.project.slug) {
      router.replace({
        ...route,
        params: { ...route.params, projectSlug: res.data.slug },
      })
    }
    await store.refresh()
    message.success('Адрес проекта изменён')
  } catch (e) {
    if (e.status === 409) slugError.value = 'Такой адрес уже занят — выберите другой'
    else message.error(e.message)
  } finally {
    slugBusy.value = false
  }
}

// ── Transfer project to another workspace (dangerous) ──
const transferShow = ref(false)
const transferTarget = ref(null)
const transferBusy = ref(false)
// Other workspaces the user belongs to — the current one is excluded (nothing to
// move to). `store.list` holds every workspace the user can access.
const transferOptions = computed(() =>
  store.list.filter((w) => w.id !== store.currentId).map((w) => ({ label: w.name, value: w.id })),
)
function openTransfer() {
  transferTarget.value = null
  transferShow.value = true
}
async function doTransfer() {
  if (!transferTarget.value || transferBusy.value) return
  transferBusy.value = true
  try {
    const res = await projApi.transfer(props.project.id, { workspace_id: transferTarget.value })
    transferShow.value = false
    const stripped = res.data?.stripped_assignees || 0
    message.success(
      stripped > 0
        ? `Проект перенесён. Снято исполнителей (не участников пространства): ${stripped}`
        : 'Проект перенесён в другое пространство',
    )
    // The project just left the current workspace — if a board of it is open,
    // it no longer belongs here, so return home. Then refresh the tree.
    if (route.params.projectSlug === props.project.slug) router.push('/')
    await store.refresh()
  } catch (e) {
    message.error(e.message)
  } finally {
    transferBusy.value = false
  }
}

// Milestones («Этап») manager for this project.
const msShow = ref(false)

// Estimation override editor for this project (inherits the workspace default).
const estShow = ref(false)
const estInherited = computed(() => resolveEstimation(null, store.current) || DEFAULT_ESTIMATION)

const bcShow = ref(false)
const bcX = ref(0)
const bcY = ref(0)
const bcTarget = ref(null)
const bcOptions = [
  { label: 'Открыть', key: 'open', icon: menuIcon(OpenOutline) },
  { label: 'Переименовать', key: 'rename', icon: menuIcon(CreateOutline) },
  {
    label: 'Удалить доску',
    key: 'delete',
    icon: dangerIcon(TrashOutline),
    props: { style: 'color:#e0533d' },
  },
]
function onBoardCtx(e, b) {
  bcTarget.value = b
  bcShow.value = false
  bcX.value = e.clientX
  bcY.value = e.clientY
  nextTick(() => (bcShow.value = true))
}
function onBoardCtxSelect(key) {
  const b = bcTarget.value
  bcShow.value = false
  if (!b) return
  if (key === 'open') router.push(`/project/${props.project.slug}/board/${b.slug}`)
  else if (key === 'rename') startBoardRename(b)
  else if (key === 'delete') removeBoard(b)
}

// Touch long-press → context menus (the native contextmenu is unreliable on the
// draggable rows inside the mobile drawer).
const lpProj = useLongPress(onProjectCtx)
let bTimer = null
function bStart(e, b) {
  const t = e.touches && e.touches[0]
  if (!t) return
  const x = t.clientX
  const y = t.clientY
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

// inline board creation via the "+" button
function startAddBoard() {
  expanded.value = true
  addingBoard.value = true
  newBoardName.value = ''
  nextTick(() => boardInput.value?.focus())
}
async function addBoard() {
  const n = newBoardName.value.trim()
  // Clear + close before await so the @blur on input removal doesn't duplicate.
  newBoardName.value = ''
  addingBoard.value = false
  if (!n) return
  try {
    await projApi.createBoard(props.project.id, { name: n })
    await store.loadBoards(props.project.id)
    expanded.value = true
  } catch (e) {
    message.error(e.message)
  }
}
</script>

<template>
  <div class="project-block">
    <div
      class="row project-row"
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
      <n-input
        v-if="renaming"
        ref="renameInput"
        v-model:value="nameEdit"
        size="tiny"
        @keyup.enter="commitRename"
        @blur="commitRename"
      />
      <span v-else class="name" @click="toggle" @dblclick="startRename">{{ project.name }}</span>

      <n-button
        class="hover-btn"
        text
        size="tiny"
        title="Добавить доску"
        @click.stop="startAddBoard"
      >
        <n-icon :component="AddOutline" />
      </n-button>
      <n-popover v-model:show="settingsShow" trigger="click" placement="right-start">
        <template #trigger>
          <n-button class="hover-btn" text size="tiny" @click.stop>
            <n-icon :component="EllipsisHorizontalOutline" />
          </n-button>
        </template>
        <div class="settings">
          <IconColorPicker
            :icon="project.icon"
            :color="project.color"
            :mode="project.icon_mode || 'badge'"
            :initials="initials"
            allow-upload
            @update:icon="updateField({ icon: $event })"
            @update:color="updateField({ color: $event })"
            @update:mode="updateField({ icon_mode: $event })"
          />
          <div class="action-row">
            <n-button type="primary" ghost size="small" @click="startRename">
              <template #icon><n-icon :component="CreateOutline" /></template>
              Переименовать
            </n-button>
            <n-button type="error" ghost size="small" @click="remove">
              <template #icon><n-icon :component="TrashOutline" /></template>
              Удалить
            </n-button>
          </div>
        </div>
      </n-popover>
    </div>

    <div v-if="expanded" class="boards">
      <!-- Sprint (milestone) nodes: a navigation overlay over the project's board.
           Shown when tree mode includes milestones. "Бэклог" = tasks with no sprint. -->
      <template v-if="showMilestones">
        <div
          v-for="m in displayMilestones"
          :key="m.id"
          class="row board-row ms-row"
          :class="{ active: msActive(m), closed: m.state === 'closed' }"
          @click="openMilestone(m)"
        >
          <span class="chev-spacer" />
          <span class="bicon"><n-icon :component="RibbonOutline" /></span>
          <span class="name">{{ m.title }}</span>
        </div>
        <div
          class="row board-row ms-row"
          :class="{ active: msActive(null) }"
          @click="openMilestone(null)"
        >
          <span class="chev-spacer" />
          <span class="bicon"><n-icon :component="GitBranchOutline" /></span>
          <span class="name">Бэклог</span>
        </div>
        <n-text v-if="!displayMilestones.length" depth="3" class="empty">нет этапов</n-text>
      </template>

      <template v-if="showBoards">
        <div
          v-for="b in boards"
          :key="b.id"
          class="row board-row"
          :class="{
            active:
              route.params.projectSlug === project.slug &&
              route.params.boardSlug === b.slug &&
              !route.query.milestone,
          }"
          @click="
            editingBoardId !== b.id && router.push(`/project/${project.slug}/board/${b.slug}`)
          "
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
          <n-input
            v-if="editingBoardId === b.id"
            :ref="(el) => el && (boardEditInput = el)"
            v-model:value="boardNameEdit"
            size="tiny"
            @click.stop
            @keyup.enter="commitBoardRename(b)"
            @blur="commitBoardRename(b)"
          />
          <span v-else class="name" @dblclick.stop="startBoardRename(b)">{{ b.name }}</span>
          <n-popover trigger="click" placement="right-start">
            <template #trigger>
              <n-button class="hover-btn" text size="tiny" @click.stop>
                <n-icon :component="EllipsisHorizontalOutline" />
              </n-button>
            </template>
            <div class="action-col" @click.stop>
              <n-button size="small" block @click="startBoardRename(b)">
                <template #icon><n-icon :component="CreateOutline" /></template>
                Переименовать
              </n-button>
              <n-popconfirm
                :positive-button-props="{ type: 'error' }"
                positive-text="Удалить"
                @positive-click="removeBoard(b)"
              >
                <template #trigger>
                  <n-button type="error" ghost size="small" block>
                    <template #icon><n-icon :component="TrashOutline" /></template>
                    Удалить доску
                  </n-button>
                </template>
                Удалить доску «{{ b.name }}» со всеми задачами?
              </n-popconfirm>
            </div>
          </n-popover>
        </div>
        <div v-if="addingBoard" class="row">
          <n-input
            ref="boardInput"
            v-model:value="newBoardName"
            size="tiny"
            placeholder="Название доски"
            @keyup.enter="addBoard"
            @blur="addBoard"
          />
        </div>
        <n-text v-if="!boards.length && !addingBoard" depth="3" class="empty">нет досок</n-text>
      </template>
    </div>

    <n-dropdown
      trigger="manual"
      placement="bottom-start"
      :show="pcShow"
      :x="pcX"
      :y="pcY"
      :options="pcOptions"
      @select="onProjectCtxSelect"
      @clickoutside="pcShow = false"
    />
    <n-dropdown
      trigger="manual"
      placement="bottom-start"
      :show="bcShow"
      :x="bcX"
      :y="bcY"
      :options="bcOptions"
      @select="onBoardCtxSelect"
      @clickoutside="bcShow = false"
    />

    <ConfirmByName
      v-model:show="confirmDelete"
      :name="project.name"
      title="Удалить проект"
      message="Проект будет удалён со всеми досками и задачами. Действие необратимо."
      @confirm="doRemove"
    />

    <n-modal v-model:show="slugShow">
      <n-card title="Адрес проекта" style="max-width: 420px" role="dialog" :bordered="false">
        <div class="slug-body">
          <n-text depth="3" class="slug-hint">
            Текущий адрес: <span class="slug-url">/project/{{ project.slug }}</span>
          </n-text>
          <n-input
            v-model:value="slugInput"
            placeholder="адрес-проекта"
            :status="slugError ? 'error' : undefined"
            :disabled="slugBusy"
            @keyup.enter="doSetSlug"
          />
          <n-text v-if="slugError" type="error" class="slug-hint">{{ slugError }}</n-text>
          <n-text depth="3" class="slug-hint">
            Новый адрес:
            <span v-if="slugPreview" class="slug-url">/project/{{ slugPreview }}</span>
            <span v-else>—</span>
          </n-text>
          <p class="slug-warn">
            Ссылки на доски и задачи этого проекта, отправленные раньше,
            <strong>перестанут работать</strong>. Открытые вкладки переключатся на новый адрес.
          </p>
        </div>
        <template #footer>
          <div class="slug-actions">
            <n-button size="small" :disabled="slugBusy" @click="slugShow = false">Отмена</n-button>
            <n-button
              type="primary"
              size="small"
              :disabled="!slugPreview || slugBusy"
              :loading="slugBusy"
              @click="doSetSlug"
            >
              Изменить адрес
            </n-button>
          </div>
        </template>
      </n-card>
    </n-modal>

    <n-modal v-model:show="transferShow">
      <n-card
        title="Перенести в другое пространство"
        style="max-width: 440px"
        role="dialog"
        :bordered="false"
      >
        <div class="transfer-body">
          <p class="transfer-warn">
            Проект «{{ project.name }}» будет перенесён в выбранное пространство
            <strong>со всеми досками, задачами, тегами и заметками</strong>. В новом пространстве
            проект окажется без группы. Исполнители, не состоящие в целевом пространстве, будут
            сняты с задач.
          </p>
          <n-select
            v-model:value="transferTarget"
            :options="transferOptions"
            placeholder="Выберите пространство"
            :disabled="transferBusy"
          />
          <n-text v-if="!transferOptions.length" depth="3" class="transfer-empty">
            Нет других пространств — сначала создайте ещё одно.
          </n-text>
        </div>
        <template #footer>
          <div class="transfer-actions">
            <n-button size="small" :disabled="transferBusy" @click="transferShow = false">
              Отмена
            </n-button>
            <n-button
              type="warning"
              size="small"
              :disabled="!transferTarget"
              :loading="transferBusy"
              @click="doTransfer"
            >
              <template #icon><n-icon :component="SwapHorizontalOutline" /></template>
              Перенести
            </n-button>
          </div>
        </template>
      </n-card>
    </n-modal>

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
  padding: 5px 8px;
  border-radius: 6px;
  cursor: pointer;
  font-size: 14px;
}
.row:hover {
  background: var(--t-hover);
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
  width: 22px;
  height: 22px;
  border-radius: 5px;
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
   one indentation style. */
.boards {
  margin-left: 15px;
  padding-left: 6px;
  border-left: 1px solid var(--t-border);
}
.board-row {
  font-size: 13px;
  color: var(--t-text2);
}
.board-row.active {
  background: color-mix(in srgb, var(--t-primary) 16%, transparent);
  color: var(--t-primary);
  font-weight: 600;
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
.settings {
  width: 264px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.action-row {
  display: flex;
  gap: 8px;
}
.action-row :deep(.n-button) {
  flex: 1;
}
.action-col {
  display: flex;
  flex-direction: column;
  gap: 8px;
  width: 190px;
}
.transfer-body {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.transfer-warn {
  margin: 0;
  font-size: 13px;
  line-height: 1.5;
  color: var(--t-text2);
}
.transfer-empty {
  font-size: 12px;
}
.transfer-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
.slug-body {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.slug-hint {
  font-size: 12px;
}
.slug-url {
  font-family: var(--t-font-mono, ui-monospace, monospace);
}
.slug-warn {
  margin: 0;
  font-size: 13px;
  line-height: 1.5;
  color: var(--t-text2);
}
.slug-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
</style>
