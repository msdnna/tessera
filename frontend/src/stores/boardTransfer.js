import { defineStore } from 'pinia'
import { ref } from 'vue'

// Board export and import (#3068). The doors — «⋯ Действия доски», the phone's
// board menu, the palette's `>export` — only ask; the board answers the export
// (it alone knows which cards the current view shows) and WorkspaceDialogsHost
// renders the import window.
export const useBoardTransferStore = defineStore('boardTransfer', () => {
  // { format: 'csv' | 'json', n } — a nonce, so the same format asked twice fires twice.
  const exportRequest = ref(null)
  function requestExport(format) {
    exportRequest.value = { format, n: (exportRequest.value?.n || 0) + 1 }
  }

  // The import window: the board it imports onto.
  const importBoard = ref(null) // { id, name } | null
  function openImport(board) {
    if (board?.id) importBoard.value = { id: board.id, name: board.name || '' }
  }
  function closeImport() {
    importBoard.value = null
  }

  return { exportRequest, requestExport, importBoard, openImport, closeImport }
})
