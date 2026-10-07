import { defineStore } from 'pinia'
import { ref } from 'vue'

// Workspace dialogs (v2): the members, integrations, appearance, create/delete
// and background-jobs modals used to live inside the tool row under the
// workspace name. The row is gone — its doors are now the workspace menu, the
// rail tile's menu and the footer «⋯» — so the modals sit in one host
// (WorkspaceDialogsHost) and every door opens them through this store.
export const useWsDialogsStore = defineStore('wsDialogs', () => {
  // 'members' | 'gitlab' | 'estimation' | 'commands' | 'appearance' | 'create'
  // | 'delete' | 'jobs' | '' (none)
  const current = ref('')
  // The workspace a rail tile's menu was opened on (appearance / delete can
  // target a tile other than the current workspace); null = the current one.
  const target = ref(null)
  // Where the small ones (create / appearance / delete — popover forms since
  // #2969) open: the element of the door that asked, else the sidebar header.
  const anchor = ref(null)

  function open(name, workspace = null, anchorEl = null) {
    target.value = workspace
    anchor.value = anchorEl
    current.value = name
  }
  function close() {
    current.value = ''
  }

  return { current, target, anchor, open, close }
})
