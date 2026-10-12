import { defineStore } from 'pinia'
import { ref } from 'vue'

// Shell overlays driven by the keyboard as much as by the mouse (v2, #2956):
// the quick-actions FAB menu, the quick-create dialog and the cheat sheet.
export const useQuickStore = defineStore('quick', () => {
  const fabOpen = ref(false)
  // 'task' while the quick-create dialog is up (reminders and notes are the
  // «Входящие» capture's, stores/inbox, #2996).
  const createMode = ref(null)
  const sheetOpen = ref(false)

  function create(mode) {
    fabOpen.value = false
    createMode.value = mode
  }

  return { fabOpen, createMode, sheetOpen, create }
})
