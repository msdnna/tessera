import { defineStore } from 'pinia'
import { ref } from 'vue'

// Bridge between the open board (KanbanBoard) and the global header (Topbar),
// which now hosts the board's layout switcher and the Теги / Архив actions.
// Group/sort/filter/subtask-toggle stay local to the board's own sub-toolbar.
export const useBoardViewStore = defineStore('boardView', () => {
  const active = ref(false) // a board is currently open
  const boardId = ref(null)
  const wsId = ref(null)
  const projectId = ref(null) // the open board's project — scopes its tags
  const tagsList = ref([]) // for the header Теги manager
  const prefixNames = ref({}) // canonical tag-prefix → friendly label
  const layout = ref('board') // 'board' | 'list' | 'calendar'
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
  function setTags(list) {
    tagsList.value = list || []
  }
  function setPrefixNames(map) {
    prefixNames.value = map || {}
  }
  function reset() {
    active.value = false
    boardId.value = null
    wsId.value = null
    projectId.value = null
    tagsList.value = []
    prefixNames.value = {}
    archiveOpen.value = false
  }

  return {
    active,
    boardId,
    wsId,
    projectId,
    tagsList,
    prefixNames,
    layout,
    archiveOpen,
    reloadNonce,
    bumpReload,
    setContext,
    setTags,
    setPrefixNames,
    reset,
  }
})
