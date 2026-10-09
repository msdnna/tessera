import { defineStore } from 'pinia'
import { computed, reactive, ref } from 'vue'

// Per-field pill visibility on a card (key → false hides). Lives here because the
// store owns the card display settings; KanbanBoard's saved-view snapshot/restore
// seeds them from here so an older saved view missing a key still gets a default.
export function defaultFieldVis() {
  return {
    priority: true,
    due: true,
    assignee: true,
    tags: true,
    estimate: true,
    milestone: true,
    description: true,
    number: true,
    gitlab: true,
  }
}

// The open board's shared context. Originally just a bridge between KanbanBoard and
// the global header (layout switcher, Теги / Архив actions); it is now the single
// owner of everything about the board that isn't tied to one card instance —
// reference data (tags, members, milestones, columns), the GitLab integration flags
// and the card display settings. Cards and the task modal read it directly instead
// of receiving 15-23 props threaded through the board.
//
// Ownership rule: board context → store, instance identity (which task, is it
// nested, is it being dragged) → props.
//
// Group/sort/filter state stays local to the board's own sub-toolbar.
export const useBoardViewStore = defineStore('boardView', () => {
  const active = ref(false) // a board is currently open
  const boardId = ref(null)
  const wsId = ref(null)
  const projectId = ref(null) // the open board's project — scopes its tags
  const board = ref(null) // the board object itself
  const columns = ref([]) // real status columns [{ id, name, color, position }]

  // ── reference data, keyed by id; KanbanBoard fills these in place ──
  const tagsMap = reactive({})
  const membersMap = reactive({})
  // GitLab project members (assignable on integration boards even without a Tessera
  // account), keyed by gl_user_id; empty for non-integration boards.
  const gitlabMembersMap = reactive({})
  // Project milestones («Этап»), keyed by id; cards/modal resolve a task's milestone_id.
  const milestonesMap = reactive({})
  const prefixNames = reactive({}) // canonical tag-prefix → friendly label
  // Canonical tag prefixes governed by non-"tag" GitLab rules (status/priority/…),
  // hidden from tag pickers.
  const metaTagPrefixes = ref(new Set())
  // What the property pickers count (#3118), published by KanbanBoard from the
  // tasks it holds: { boardId, counts: column id → tasks, load: user id → open }.
  const taskStats = ref({ boardId: null, counts: {}, load: {} })

  const tagsList = computed(() => Object.values(tagsMap))
  const membersList = computed(() => Object.values(membersMap))
  const milestonesList = computed(() => Object.values(milestonesMap))
  // GitLab roster minus members that already map to a Tessera user in this workspace
  // (they appear in the Tessera member list instead — avoids showing one person twice).
  const gitlabMembersList = computed(() =>
    Object.values(gitlabMembersMap).filter(
      (g) => !(g.tessera_user_id && membersMap[g.tessera_user_id]),
    ),
  )

  // True when this board is the workspace's GitLab integration board and the
  // integration allows creating issues from tasks (writeback.push_create) — gates the
  // "Создать issue в GitLab" action in the task modal.
  const gitlabCanCreate = ref(false)
  const gitlabFetchTemplates = ref(false)
  const gitlabIntegrationId = ref(null)
  // Same shape for the hierarchy half (#2592): writeback.push_children gates both the
  // "Сделать сгруппированной" button on a parent and the per-subtask push/retry. Kept
  // separate from gitlabCanCreate because an integration can allow one and not the other.
  const gitlabCanGroup = ref(false)

  // ── card display (customize-view; part of the per-board saved view) ──
  const cardSize = ref('medium') // 'compact' | 'medium' | 'large'
  const stackFields = ref(false) // pills stacked vertically vs. horizontal wrap
  const showEmpty = ref(true) // render empty (unset) placeholder pills
  const fieldVis = reactive(defaultFieldVis())

  const layout = ref('board') // 'board' | 'list' | 'calendar' | 'timeline' | 'gantt' | 'matrix'
  const archiveOpen = ref(false)
  const reloadNonce = ref(0) // header-driven changes ask the board to reload
  // Header search on a board (v2, #2954): the header field is the board's
  // title filter (KanbanBoard mirrors it into its own filters.q), and the board
  // publishes how many tasks the current filter leaves so the header knows when
  // to fall back to a workspace-wide search (null = no board count yet).
  const searchQ = ref('')
  const shownTasks = ref(null)
  // Its tooltip («показано 5 из 12…») — the count sits next to the board in the
  // sidebar (v2), not in the sub-toolbar.
  const shownTitle = ref('')
  // «Filter by this person» from the palette (#2957): a request the board
  // applies to its own filters; `n` makes a repeat of the same person fire.
  const assigneeFilter = ref(null)
  // «N, then T» (#2956) asks the open board to start its «К работе» quick-add.
  const quickAdd = ref(0)
  function requestQuickAdd() {
    quickAdd.value++
  }
  function requestAssigneeFilter(userId) {
    assigneeFilter.value = { userId, n: (assigneeFilter.value?.n || 0) + 1 }
  }
  // The view panel (#2986) is owned by KanbanBoard; the phone's menu lives in the
  // header, outside the board, so it asks for the panel through a nonce the way
  // the header actions ask for a reload above.
  const customizeNonce = ref(0)
  // The docked view panel is open (desktop): the FAB steps left of it.
  const panelOpen = ref(false)
  // Saved views (#2970) belong to KanbanBoard; the phone's board menu lists them
  // too (the sub-toolbar there has no room for the chip's menu). KanbanBoard
  // publishes the whole list — one list for all view kinds — and the loaded one
  // here, and answers the request below. Saving happens in the view panel.
  const savedViews = ref([]) // [{ id, name, layout }]
  const currentViewId = ref('') // the loaded view, '' when none
  // { nonce, op: 'apply' | 'clear', id? }. A nonce and not a bare payload:
  // loading the same view twice in a row is a legitimate ask (it re-applies a
  // toolbar the reader has since changed), and an unchanged object wouldn't wake
  // the watcher.
  const viewRequest = ref(null)
  let viewNonce = 0

  function bumpReload() {
    reloadNonce.value++
  }
  function openCustomize() {
    customizeNonce.value++
  }
  function requestApplyView(id) {
    viewRequest.value = { nonce: ++viewNonce, op: 'apply', id }
  }
  function requestClearView() {
    viewRequest.value = { nonce: ++viewNonce, op: 'clear' }
  }
  function setContext(b, w, p) {
    boardId.value = b
    wsId.value = w
    projectId.value = p
    active.value = true
  }
  // Refill a keyed map in place: the maps are handed out by reference (KanbanBoard
  // writes into them, cards read from them), so they must never be reassigned.
  function refill(target, entries) {
    for (const k of Object.keys(target)) delete target[k]
    Object.assign(target, entries || {})
  }
  function reset() {
    active.value = false
    boardId.value = null
    wsId.value = null
    projectId.value = null
    board.value = null
    columns.value = []
    refill(tagsMap, null)
    refill(membersMap, null)
    refill(gitlabMembersMap, null)
    refill(milestonesMap, null)
    refill(prefixNames, null)
    metaTagPrefixes.value = new Set()
    taskStats.value = { boardId: null, counts: {}, load: {} }
    gitlabCanCreate.value = false
    gitlabFetchTemplates.value = false
    gitlabIntegrationId.value = null
    gitlabCanGroup.value = false
    cardSize.value = 'medium'
    stackFields.value = false
    showEmpty.value = true
    refill(fieldVis, defaultFieldVis())
    archiveOpen.value = false
    panelOpen.value = false
    searchQ.value = ''
    shownTasks.value = null
    shownTitle.value = ''
    assigneeFilter.value = null
    savedViews.value = []
    currentViewId.value = ''
    viewRequest.value = null
  }

  return {
    active,
    searchQ,
    shownTasks,
    shownTitle,
    assigneeFilter,
    requestAssigneeFilter,
    quickAdd,
    requestQuickAdd,
    boardId,
    wsId,
    projectId,
    board,
    columns,
    tagsMap,
    membersMap,
    gitlabMembersMap,
    milestonesMap,
    prefixNames,
    metaTagPrefixes,
    taskStats,
    tagsList,
    membersList,
    milestonesList,
    gitlabMembersList,
    gitlabCanCreate,
    gitlabFetchTemplates,
    gitlabIntegrationId,
    gitlabCanGroup,
    cardSize,
    stackFields,
    showEmpty,
    fieldVis,
    layout,
    archiveOpen,
    reloadNonce,
    customizeNonce,
    panelOpen,
    savedViews,
    currentViewId,
    viewRequest,
    bumpReload,
    openCustomize,
    requestApplyView,
    requestClearView,
    setContext,
    refill,
    reset,
  }
})
