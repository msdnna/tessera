import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { workspaces as wsApi, projects as projApi } from '@/api'
import { useTreeExpand } from '@/composables/useTreeExpand'
import { resolveEstimation } from '@/utils/estimation'

// workspaces store — holds the list, the current selection, and the
// groups/projects tree for the sidebar. Boards are loaded lazily per project.
export const useWorkspacesStore = defineStore('workspaces', () => {
  const list = ref([])
  const currentId = ref(localStorage.getItem('tessera_ws') || '')
  const groups = ref([])
  const projects = ref([])
  const boardsByProject = ref({}) // projectId -> board[]

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

  async function selectWorkspace(id) {
    currentId.value = id
    localStorage.setItem('tessera_ws', id)
    boardsByProject.value = {}
    const [g, p] = await Promise.all([wsApi.groups(id), wsApi.projects(id)])
    groups.value = g.data || []
    projects.value = p.data || []
    await prefetchExpandedBoards()
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

  // tree helpers (groups/projects are flat; build the tree by parent_id/group_id)
  function childGroups(parentId) {
    return groups.value
      .filter((g) => (g.parent_id || null) === (parentId || null))
      .sort((a, b) => a.position - b.position)
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

  return {
    list,
    currentId,
    current,
    canManage,
    groups,
    projects,
    boardsByProject,
    upsertBoard,
    loadWorkspaces,
    selectWorkspace,
    removeWorkspace,
    refresh,
    loadBoards,
    childGroups,
    projectsInGroup,
    estimationFor,
    setProjectEstimation,
    setWorkspaceEstimation,
  }
})
