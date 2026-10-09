import { defineStore } from 'pinia'
import { ref } from 'vue'

// The «Проект» window (#2971 / #2989): the one place a project is set up —
// Общее · Теги · Этапы · Оценка. Its doors (the project menu in the sidebar,
// ⌘,, the board's «Теги» / «Этапы», the palette, «Управлять тегами…» in the
// tag pickers) only call `open(projectId, tab)`; ProjectWindow renders it.
export const TABS = ['general', 'tags', 'stages', 'estimation']

export const useProjectWindowStore = defineStore('projectWindow', () => {
  const projectId = ref(null)
  const tab = ref('general')
  // Text for the tag table's filter — «Управлять тегами…» carries over what was
  // typed in the picker.
  const filter = ref('')
  // Where focus goes back to on close (Esc returns it to the door).
  let returnTo = null

  function open(id, which = 'general', { filter: f = '' } = {}) {
    if (!id) return
    returnTo = typeof document !== 'undefined' ? document.activeElement : null
    projectId.value = id
    tab.value = TABS.includes(which) ? which : 'general'
    filter.value = f
  }

  function close() {
    projectId.value = null
    const el = returnTo
    returnTo = null
    if (el && typeof el.focus === 'function' && el.isConnected) {
      setTimeout(() => el.focus(), 0)
    }
  }

  return { projectId, tab, filter, open, close }
})
