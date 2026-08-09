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

  // ── card display (customize-view; part of the per-board saved view) ──
  const cardSize = ref('medium') // 'compact' | 'medium' | 'large'
  const stackFields = ref(false) // pills stacked vertically vs. horizontal wrap
  const showEmpty = ref(true) // render empty (unset) placeholder pills
  const fieldVis = reactive(defaultFieldVis())

  const layout = ref('board') // 'board' | 'list' | 'calendar' | 'timeline' | 'gantt' | 'matrix'
  const archiveOpen = ref(false)
  const reloadNonce = ref(0) // header-driven changes ask the board to reload

  function bumpReload() {
    reloadNonce.value++
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
    gitlabCanCreate.value = false
    gitlabFetchTemplates.value = false
    gitlabIntegrationId.value = null
    cardSize.value = 'medium'
    stackFields.value = false
    showEmpty.value = true
    refill(fieldVis, defaultFieldVis())
    archiveOpen.value = false
  }

  return {
    active,
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
    tagsList,
    membersList,
    milestonesList,
    gitlabMembersList,
    gitlabCanCreate,
    gitlabFetchTemplates,
    gitlabIntegrationId,
    cardSize,
    stackFields,
    showEmpty,
    fieldVis,
    layout,
    archiveOpen,
    reloadNonce,
    bumpReload,
    setContext,
    refill,
    reset,
  }
})
