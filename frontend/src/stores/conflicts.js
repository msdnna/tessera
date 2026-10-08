import { defineStore } from 'pinia'
import { ref, computed, reactive } from 'vue'
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

  // Choices made in the resolver window but not applied yet (#2975): Esc closes
  // the window and keeps them, so reopening lands where the user left. Per
  // conflict id: { [field]: { pick: 'base'|'ours'|'theirs'|'manual', text } }.
  // Session-only — a reload starts over, like any unsaved draft.
  const drafts = reactive({})
  function draftFor(id) {
    if (!drafts[id]) drafts[id] = {}
    return drafts[id]
  }
  function dropDraft(id) {
    delete drafts[id]
  }

  // Open the conflicts resolver, optionally focused on one task's conflict.
  function openResolver(taskId = null) {
    focusTaskId.value = taskId
    resolverOpen.value = true
  }

  // Only the newest request may write to the list. Both callers can supersede one
  // another — a workspace switch and a `gitlab.conflict` event — and the request
  // being abandoned is still out: without the comparison the badges would end up
  // counting the conflicts of the workspace the user has left, and `has()` would
  // put warning marks on cards that have none. `clear()` bumps it too: logout
  // empties the list, and a reply still in flight must not refill it.
  let loadSeq = 0

  async function load(id) {
    if (id) wsId.value = id
    const mine = ++loadSeq
    if (!wsId.value) {
      list.value = []
      return
    }
    try {
      const { data } = await glApi.conflicts(wsId.value)
      if (mine !== loadSeq) return
      list.value = data || []
    } catch {
      // No integration / offline — treat as no conflicts (endpoint returns [] now).
      if (mine !== loadSeq) return
      list.value = []
    }
  }

  // Realtime: a new or resolved conflict in the current workspace → refresh.
  function onEvent(ev) {
    if (ev?.type === 'gitlab.conflict' && (!wsId.value || ev.scope === wsId.value)) load()
  }

  function clear() {
    loadSeq++
    list.value = []
    wsId.value = null
    for (const id of Object.keys(drafts)) delete drafts[id]
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
    draftFor,
    dropDraft,
  }
})
