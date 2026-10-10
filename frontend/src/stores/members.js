import { defineStore } from 'pinia'
import { ref } from 'vue'
import { workspaces as wsApi } from '@/api'

// Members v2 (#3063): the member groups of each workspace (#3074) — read by the
// «Участники» window, the assignee picker and the @-popup of the editor — and a
// revision the window watches to re-read its lists when the socket says the
// roster, the access requests or the groups changed elsewhere.
//
// Groups are names for sets of members and grant nothing, so a stale copy is a
// display nit, never a leak: the server decides who may see what.
export const useMembersStore = defineStore('members', () => {
  // workspace id → [{ id, name, color, member_ids }]
  const groupsByWs = ref({})
  // Bumped per workspace on members.changed / access_requests.changed.
  const rev = ref({})
  const inflight = new Map()

  function groupsFor(wsId) {
    return (wsId && groupsByWs.value[wsId]) || []
  }

  // loadGroups reads a workspace's groups once (force re-reads). Failures leave
  // the list empty: a picker without groups still assigns people.
  async function loadGroups(wsId, { force = false } = {}) {
    if (!wsId) return []
    if (!force && groupsByWs.value[wsId]) return groupsByWs.value[wsId]
    if (inflight.has(wsId)) return inflight.get(wsId)
    const p = wsApi
      .memberGroups(wsId)
      .then((res) => {
        groupsByWs.value = { ...groupsByWs.value, [wsId]: res.data || [] }
        return groupsByWs.value[wsId]
      })
      .catch(() => groupsByWs.value[wsId] || [])
      .finally(() => inflight.delete(wsId))
    inflight.set(wsId, p)
    return p
  }

  function setGroups(wsId, list) {
    groupsByWs.value = { ...groupsByWs.value, [wsId]: list || [] }
  }

  function bump(wsId) {
    rev.value = { ...rev.value, [wsId]: (rev.value[wsId] || 0) + 1 }
  }

  // onSocket: the three workspace events of #3063. A group list nobody loaded is
  // left alone — whoever needs it reads the current truth on first use.
  function onSocket(ev) {
    const wsId = ev?.scope || ev?.data?.workspace_id
    if (!wsId) return
    if (ev.type === 'member_groups.changed') {
      if (groupsByWs.value[wsId]) loadGroups(wsId, { force: true })
      bump(wsId)
    } else if (ev.type === 'members.changed' || ev.type === 'access_requests.changed') {
      bump(wsId)
    }
  }

  function reset() {
    groupsByWs.value = {}
    rev.value = {}
  }

  return { groupsByWs, rev, groupsFor, loadGroups, setGroups, onSocket, reset }
})
