import { defineStore } from 'pinia'
import { reactive } from 'vue'
import { codeLinksPatch } from '@/utils/codeLinks'

// Live «Разработка» of tasks (#2964): the merge requests the socket reported
// since the task window last fetched the task. The window reads the task's own
// `code_links` and lets an entry here win, so a pipeline finishing in GitLab
// repaints the open window without a request. Fed from AppLayout's socket — the
// task window has no socket of its own.
export const useCodeLinksStore = defineStore('codeLinks', () => {
  const byTask = reactive({})

  function onEvent(ev) {
    const p = codeLinksPatch(ev)
    if (p) byTask[p.taskId] = p.links
  }
  // A fresh GET of the task supersedes whatever the socket said before it.
  function forget(taskId) {
    delete byTask[taskId]
  }
  return { byTask, onEvent, forget }
})
