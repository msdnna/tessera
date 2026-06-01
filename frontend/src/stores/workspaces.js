import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { workspaces as wsApi, projects as projApi } from '@/api'

// workspaces store — holds the list, the current selection, and the
// groups/projects tree for the sidebar. Boards are loaded lazily per project.
export const useWorkspacesStore = defineStore('workspaces', () => {
  const list = ref([])
  const currentId = ref(localStorage.getItem('tessera_ws') || '')
  const groups = ref([])
  const projects = ref([])
  const boardsByProject = ref({}) // projectId -> board[]

  const current = computed(() => list.value.find((w) => w.id === currentId.value) || null)

  async function loadWorkspaces() {
    const res = await wsApi.list()
    list.value = res.data || []
    if (!currentId.value || !list.value.some((w) => w.id === currentId.value)) {
      currentId.value = list.value[0]?.id || ''
    }
    if (currentId.value) await selectWorkspace(currentId.value)
  }

  async function selectWorkspace(id) {
    currentId.value = id
    localStorage.setItem('tessera_ws', id)
    boardsByProject.value = {}
    const [g, p] = await Promise.all([wsApi.groups(id), wsApi.projects(id)])
    groups.value = g.data || []
    projects.value = p.data || []
  }

  async function loadBoards(projectId) {
    const res = await projApi.boards(projectId)
    boardsByProject.value = { ...boardsByProject.value, [projectId]: res.data || [] }
  }

  return {
    list,
    currentId,
    current,
    groups,
    projects,
    boardsByProject,
    loadWorkspaces,
    selectWorkspace,
    loadBoards,
  }
})
