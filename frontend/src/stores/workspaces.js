import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { workspaces as wsApi, projects as projApi } from '@/api'
import { useTreeExpand } from '@/composables/useTreeExpand'
import { resolveEstimation } from '@/utils/estimation'
import { commandItems } from '@/utils/commands'
import { milestoneTreeTarget } from '@/utils/treeEvents'
import { sGet, sSet } from '@/utils/serverStorage'

// workspaces store — holds the list, the current selection, and the
// groups/projects tree for the sidebar. Boards are loaded lazily per project.
export const useWorkspacesStore = defineStore('workspaces', () => {
  const list = ref([])
  const currentId = ref(sGet('tessera_ws') || '')
  const groups = ref([])
  const projects = ref([])
  const boardsByProject = ref({}) // projectId -> board[]
  // Quick-action registry for the editor popup — loaded once per workspace, not
  // per opened task. `customCommands` keeps the raw dictionary for the settings
  // editor; `commands` is the flattened popup list (built-in + custom).
  const builtinCommands = ref([])
  const customCommands = ref([])
  const commandsCanManage = ref(false)
  const commands = computed(() => commandItems(builtinCommands.value, customCommands.value))

  const current = computed(() => list.value.find((w) => w.id === currentId.value) || null)

  // The caller's role in the current workspace, as reported by the workspace
  // list. Only for hiding controls the server would refuse anyway — every
  // manager-only action is still gated server-side (requireManager).
  const canManage = computed(() => ['owner', 'admin'].includes(current.value?.my_role))

  async function loadWorkspaces() {
    const res = await wsApi.list()
    list.value = res.data || []
    if (!currentId.value || !list.value.some((w) => w.id === currentId.value)) {
      currentId.value = list.value[0]?.id || ''
    }
    if (currentId.value) await selectWorkspace(currentId.value)
  }

  // Delete a workspace (owner only, enforced server-side) and re-sync the list.
  // If the deleted one was active, loadWorkspaces() re-picks the first available.
  async function removeWorkspace(id) {
    await wsApi.remove(id)
    if (currentId.value === id) currentId.value = ''
    await loadWorkspaces()
  }

  // Two switches can be in flight at once: on startup AppLayout selects the
  // remembered workspace while a deep-linked board selects its own (#2721). The
  // sequence token makes the later call win the tree — otherwise groups/projects
  // land in network-response order and the stale one can overwrite the current.
  let selectSeq = 0

  async function selectWorkspace(id) {
    const seq = ++selectSeq
    currentId.value = id
    sSet('tessera_ws', id)
    boardsByProject.value = {}
    const [g, p] = await Promise.all([wsApi.groups(id), wsApi.projects(id)])
    if (seq !== selectSeq) return // superseded by a later switch — leave its tree alone
    groups.value = g.data || []
    projects.value = p.data || []
    await loadCommands(id)
    await prefetchExpandedBoards()
  }

  // Point the app at `id` unless it is already there. A deep link resolves a
  // board that may live outside the remembered workspace, and every scope
  // decision (realtime filter, members, tags) reads currentId — so it has to
  // follow the board. Idempotent: re-resolving the same board is a no-op.
  async function ensureWorkspace(id) {
    if (!id || id === currentId.value) return
    await selectWorkspace(id)
  }

  // Load the command registry. Best-effort: a failure must not break the board,
  // it only means the `/`-popup stays silent (same as an empty dictionary).
  async function loadCommands(id) {
    try {
      const res = await wsApi.commands(id)
      builtinCommands.value = res.data?.builtin || []
      customCommands.value = res.data?.custom || []
      commandsCanManage.value = !!res.data?.can_manage
    } catch {
      builtinCommands.value = []
      customCommands.value = []
      commandsCanManage.value = false
    }
  }

  // Patch the custom dictionary from a save response or a
  // `workspace_commands.updated` WS event (both carry custom entries only, so
  // the built-in half stays as loaded), letting open tabs pick it up live.
  function setCustomCommands(custom) {
    customCommands.value = custom || []
  }

  // Eagerly load boards for projects the user has expanded so they show right
  // after login / a workspace switch. The per-project lazy load (on ProjectRow
  // mount) races with the sidebar mount + component reuse and could leave a
  // restored-expanded project showing «нет досок» until a manual re-expand;
  // loading centrally here (the one place that sets projects + clears boards)
  // is race-free.
  async function prefetchExpandedBoards() {
    const { isExpanded } = useTreeExpand()
    await Promise.all(
      projects.value.filter((p) => isExpanded(p.id, false)).map((p) => loadBoards(p.id)),
    )
  }

  // refresh reloads groups + projects without touching expanded boards (used
  // after sidebar create/rename/delete/move so the tree doesn't collapse).
  async function refresh() {
    const id = currentId.value
    if (!id) return
    const [g, p] = await Promise.all([wsApi.groups(id), wsApi.projects(id)])
    groups.value = g.data || []
    projects.value = p.data || []
  }

  // What a reconnect has to re-read: the workspace *set* first, then the tree.
  //
  // The server closes a user's sockets on every membership change (Hub.DropUser)
  // precisely so the client re-reads which workspaces it belongs to and what it
  // is in each — that is the whole mechanism by which an add, a removal or a
  // role change reaches a session that is already open. The resync handler only
  // ever called refresh(), which re-reads groups and projects of the *current*
  // workspace, so the server's half of the contract landed nowhere: the list
  // itself (and with it `my_role`, i.e. all of `canManage`) was fetched once at
  // mount and never again. A workspace you were just added to stayed missing
  // from the sidebar, one you were removed from stayed in it and 403'd on every
  // click, and a promotion to admin took effect only on a page reload (#2932).
  //
  // The selection is deliberately left alone while the current workspace is
  // still ours: a resync is a mid-session event — it also fires after a plain
  // network outage — and re-picking the first workspace would move the reader
  // off the board they are looking at. It moves only when that workspace is gone
  // from the list, which is exactly the removed-from-it case.
  async function resync() {
    const res = await wsApi.list()
    list.value = res.data || []
    if (currentId.value && list.value.some((w) => w.id === currentId.value)) {
      await refresh()
      return
    }
    currentId.value = list.value[0]?.id || ''
    if (currentId.value) {
      await selectWorkspace(currentId.value)
      return
    }
    // Removed from the last workspace there was: the tree below belongs to a
    // workspace we can no longer read, and leaving it on screen would offer
    // projects every click 403s on.
    groups.value = []
    projects.value = []
    boardsByProject.value = {}
  }

  async function loadBoards(projectId) {
    const res = await projApi.boards(projectId)
    boardsByProject.value = { ...boardsByProject.value, [projectId]: res.data || [] }
  }

  // Patch a single loaded board in place (e.g. after a rename / icon change) so the
  // sidebar tree reflects it without a full reload.
  function upsertBoard(board) {
    if (!board?.project_id) return
    const list = boardsByProject.value[board.project_id]
    if (!list) return
    boardsByProject.value = {
      ...boardsByProject.value,
      [board.project_id]: list.map((b) => (b.id === board.id ? { ...b, ...board } : b)),
    }
  }

  // Apply somebody else's board CRUD to the sidebar tree (#2932).
  //
  // `board.created` / `board.updated` / `board.deleted` are workspace-scoped
  // events, but until now only the client that pressed the button ever re-read
  // `boardsByProject`: `refresh()` deliberately leaves expanded boards alone, and
  // the open board ignores board.* CRUD (utils/boardEvents.js). So a board a
  // colleague added stayed invisible, their rename stayed stale, and — worst —
  // a board they deleted kept sitting in everybody's sidebar, handing out a
  // «Не найдено» to whoever clicked it.
  //
  // Only lists already loaded are touched: a project nobody expanded yet will
  // fetch the current truth on its first expand anyway.
  function onBoardEvent(ev) {
    const data = ev?.data
    if (!data?.id) return
    if (ev.type === 'board.updated') {
      upsertBoard(data)
    } else if (ev.type === 'board.created') {
      addBoard(data)
    } else if (ev.type === 'board.deleted') {
      dropBoard(data.id)
    }
  }

  // Insert a board a colleague just created, keeping the list in the order the
  // server lists it in (by position). Ignores a board we somehow already hold —
  // the creator's own loadBoards can land first.
  function addBoard(board) {
    const list = boardsByProject.value[board.project_id]
    if (!list || list.some((b) => b.id === board.id)) return
    const next = [...list, board].sort((a, b) => (a.position ?? 0) - (b.position ?? 0))
    boardsByProject.value = { ...boardsByProject.value, [board.project_id]: next }
  }

  // Drop a deleted board. `board.deleted` carries a bare id — no project — so the
  // board is looked up across the loaded lists.
  function dropBoard(id) {
    const entries = Object.entries(boardsByProject.value)
    if (!entries.some(([, list]) => list.some((b) => b.id === id))) return
    boardsByProject.value = Object.fromEntries(
      entries.map(([pid, list]) => [pid, list.filter((b) => b.id !== id)]),
    )
  }

  // tree helpers (groups/projects are flat; build the tree by parent_id/group_id)
  //
  // effectiveParent is the parent_id the tree is actually drawn from: an edge
  // whose chain of ancestors comes back around to the group itself is read as
  // null, so the group shows up at the root instead of closing the tree into a
  // ring. Every group in a ring nulls its own edge.
  //
  // A ring costs twice over. Drawing starts at the root and walks down, so a
  // ring is simply unreachable from it: the groups in it, and every project
  // inside them, stop appearing at all — and the sidebar is the only UI that
  // could drag them back out. Walking *up* is worse: the active-rail lookup
  // follows parent_id from the open project's group and in a ring never reaches
  // a root, and that loop runs inside a computed.
  //
  // The server refuses to create such a cycle (#2932), so this is about data
  // that predates that guard: nothing is hidden, the ring is simply flattened
  // to roots and stays visible and movable.
  const effectiveParent = computed(() => {
    const raw = new Map(groups.value.map((g) => [g.id, g.parent_id || null]))
    const out = new Map()
    for (const id of raw.keys()) {
      const own = raw.get(id) || null
      const seen = new Set([id])
      let p = own
      while (p) {
        if (p === id) {
          out.set(id, null) // the chain closes on this group — draw it as a root
          break
        }
        if (seen.has(p)) break // a ring further up, already broken on its own turn
        seen.add(p)
        p = raw.get(p) || null
      }
      if (!out.has(id)) out.set(id, own)
    }
    return out
  })

  function childGroups(parentId) {
    const parents = effectiveParent.value
    return groups.value
      .filter((g) => (parents.get(g.id) ?? null) === (parentId || null))
      .sort((a, b) => a.position - b.position)
  }

  // parentGroupOf returns the group a group hangs under, following the same
  // cycle-broken edges childGroups draws — so callers walking up to the root
  // (the sidebar's active-rail lookup) terminate on the same tree the nodes show.
  function parentGroupOf(groupId) {
    const pid = effectiveParent.value.get(groupId) ?? null
    return pid ? groups.value.find((g) => g.id === pid) || null : null
  }
  function projectsInGroup(groupId) {
    return projects.value
      .filter((p) => (p.group_id || null) === (groupId || null))
      .sort((a, b) => a.position - b.position)
  }

  // Resolve the effective estimation config for a project: its override, else the
  // current workspace default, else the built-in default. Drives the estimate
  // input/chip unit everywhere a board is shown.
  function estimationFor(projectId) {
    const project = projects.value.find((p) => p.id === projectId) || null
    return resolveEstimation(project, current.value)
  }

  // Patch a project's stored estimation in place (after a settings save / WS
  // event) so open boards re-resolve without a full refresh.
  function setProjectEstimation(projectId, estimation) {
    projects.value = projects.value.map((p) => (p.id === projectId ? { ...p, estimation } : p))
  }

  // Patch the current workspace's default estimation in place.
  function setWorkspaceEstimation(workspaceId, estimation) {
    list.value = list.value.map((w) => (w.id === workspaceId ? { ...w, estimation } : w))
  }

  // Apply somebody else's estimation change (#2932).
  //
  // Both patchers above used to be reachable only from EstimationModal, i.e. only
  // on the client that did the saving — the backend's `workspace.estimation` /
  // `project.estimation` events arrived at the socket and stopped there, since the
  // board deliberately ignores them (utils/boardEvents.js) and nothing else
  // listened. That is not a cosmetic staleness: a task's `estimate` is a bare
  // number whose *unit* this config decides, and parseEstimate/formatEstimate read
  // it in both directions. A colleague who left the board open kept showing 480
  // as «1д» after the team moved to six-hour days, and kept writing minutes into a
  // project that had switched to story points.
  //
  // Patching the store is all that is needed: every consumer resolves through
  // `estimationFor` inside a computed, so the re-render follows.
  // Sprint-tree revision (#2932). The sidebar's milestone rows live in ProjectRow,
  // one component instance per expanded project, and the socket is opened once at
  // the layout — so the event can't be delivered to them directly. Instead the
  // layout bumps a counter here and each row watches its own:
  // `milestoneRevFor(projectId)` moves when an event named that project, and when
  // one named no project at all (see milestoneTreeTarget) it moves for everybody.
  // A counter rather than the row itself on purpose: a sprint node is drawn from
  // fields a `{id}` delete can't patch, and the list is two requests at most.
  const milestoneRev = ref(0) // events with no project → every list is stale
  const milestoneRevByProject = ref({})
  function milestoneRevFor(projectId) {
    return milestoneRev.value + (milestoneRevByProject.value[projectId] || 0)
  }
  function onMilestoneEvent(ev) {
    const target = milestoneTreeTarget(ev, currentId.value)
    if (target === undefined) return
    if (target === null) {
      milestoneRev.value += 1
      return
    }
    milestoneRevByProject.value = {
      ...milestoneRevByProject.value,
      [target]: (milestoneRevByProject.value[target] || 0) + 1,
    }
  }

  function onEstimationEvent(ev) {
    const data = ev?.data
    if (ev?.type === 'workspace.estimation' && data?.workspace_id) {
      setWorkspaceEstimation(data.workspace_id, data.estimation ?? null)
    } else if (ev?.type === 'project.estimation' && data?.project_id) {
      setProjectEstimation(data.project_id, data.estimation ?? null)
    }
  }

  return {
    list,
    currentId,
    current,
    canManage,
    groups,
    projects,
    boardsByProject,
    commands,
    customCommands,
    commandsCanManage,
    loadCommands,
    setCustomCommands,
    upsertBoard,
    onBoardEvent,
    loadWorkspaces,
    selectWorkspace,
    ensureWorkspace,
    removeWorkspace,
    refresh,
    resync,
    loadBoards,
    childGroups,
    parentGroupOf,
    projectsInGroup,
    estimationFor,
    setProjectEstimation,
    setWorkspaceEstimation,
    onEstimationEvent,
    milestoneRevFor,
    onMilestoneEvent,
  }
})
