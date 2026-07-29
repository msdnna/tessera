import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { gitlab as glApi } from '@/api'

// Open GitLab write-back conflicts for the current workspace. Surfaced as orange
// warning badges (Integrations button + GitLab item + sync button + task cards +
// bell) so an unresolved conflict stays visible until the user resolves it — not
// just a one-time toast. Refreshed on workspace change and on `gitlab.conflict`.
export const useConflictsStore = defineStore('conflicts', () => {
  const list = ref([])
  const wsId = ref(null)
  // The resolver modal is mounted once (AppLayout); any surface opens it via the store.
  const resolverOpen = ref(false)
  const focusTaskId = ref(null) // pre-select this task's conflict when opening

  const count = computed(() => list.value.length)
  const taskIds = computed(() => new Set(list.value.map((c) => c.task_id)))
  const has = (taskId) => taskIds.value.has(taskId)

  // Open the conflicts resolver, optionally focused on one task's conflict.
  function openResolver(taskId = null) {
    focusTaskId.value = taskId
    resolverOpen.value = true
  }

  async function load(id) {
    if (id) wsId.value = id
    if (!wsId.value) {
      list.value = []
      return
    }
    try {
      const { data } = await glApi.conflicts(wsId.value)
      list.value = data || []
    } catch {
      // No integration / offline — treat as no conflicts (endpoint returns [] now).
      list.value = []
    }
  }

  // Realtime: a new or resolved conflict in the current workspace → refresh.
  function onEvent(ev) {
    if (ev?.type === 'gitlab.conflict' && (!wsId.value || ev.scope === wsId.value)) load()
  }

  function clear() {
    list.value = []
    wsId.value = null
  }

  return {
    list,
    count,
    taskIds,
    has,
    load,
    onEvent,
    clear,
    resolverOpen,
    focusTaskId,
    openResolver,
  }
})
