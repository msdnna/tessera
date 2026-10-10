import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { favorites as favApi } from '@/api'
import { favoriteNeighbors, favoritePatch } from '@/utils/favorites'

// Favourite boards (#3115): the reader's own starred boards in the current
// workspace, in their own order — the star after the board's crumbs, the
// «Избранное» section on top of the sidebar tree and the palette's group.
//
// Changes are optimistic: the star fills at once and the request follows; a
// refusal puts the list back as the server has it. The socket keeps it honest —
// `favorites.changed` reaches only this reader's sockets (another tab, the
// phone), and the board/project events of the workspace patch names and icons
// or drop a board that went to the trash.
export const useFavoritesStore = defineStore('favorites', () => {
  const wsId = ref('')
  const items = ref([])
  const loaded = ref(false)
  // Boards that left the list because they were deleted: a `board.created` for
  // one of them is its restore, and only then is a re-read worth it.
  const dropped = new Set()
  let seq = 0

  const ids = computed(() => new Set(items.value.map((f) => f.board_id)))
  const isFavorite = (boardId) => !!boardId && ids.value.has(boardId)

  async function load(id = wsId.value) {
    if (!id) return
    if (id !== wsId.value) {
      items.value = []
      loaded.value = false
      dropped.clear()
    }
    wsId.value = id
    const mine = ++seq
    try {
      const res = await favApi.list(id)
      if (mine !== seq || id !== wsId.value) return
      items.value = res.data || []
      loaded.value = true
    } catch {
      // Best-effort: the section simply stays as it was (or hidden).
    }
  }

  // add stars a board. `board` is the board row (name, slug, icon…) and
  // `project` its project — enough to draw the sidebar row before the answer.
  async function add(board, project = null) {
    if (!board?.id || isFavorite(board.id)) return
    const last = items.value[items.value.length - 1]
    items.value = [
      ...items.value,
      {
        board_id: board.id,
        position: (last?.position ?? 0) + 65536,
        name: board.name,
        slug: board.slug,
        icon: board.icon || '',
        color: board.color || '',
        icon_mode: board.icon_mode || 'badge',
        project_id: board.project_id || project?.id,
        project_name: project?.name || '',
        project_slug: project?.slug || '',
        project_color: project?.color || '',
        workspace_id: wsId.value,
      },
    ]
    seq++ // a list read in flight predates this change
    try {
      const { data } = await favApi.add(board.id)
      items.value = items.value.map((f) => (f.board_id === board.id ? data : f))
    } catch (e) {
      await load()
      throw e
    }
  }

  // remove takes the star off. Answers the neighbours the row had, so an undo
  // can put it back in its place.
  async function remove(boardId) {
    const i = items.value.findIndex((f) => f.board_id === boardId)
    if (i < 0) return null
    const nb = favoriteNeighbors(items.value, i)
    const row = items.value[i]
    items.value = items.value.filter((f) => f.board_id !== boardId)
    seq++
    try {
      await favApi.remove(boardId)
    } catch (e) {
      await load()
      throw e
    }
    return { row, neighbors: nb }
  }

  async function toggle(board, project = null) {
    if (isFavorite(board?.id)) return remove(board.id)
    return add(board, project)
  }

  // restore is the undo of a removal: star again, then back between the
  // neighbours it had (if they are still there).
  async function restore({ row, neighbors }) {
    await add(
      { ...row, id: row.board_id },
      {
        id: row.project_id,
        name: row.project_name,
        slug: row.project_slug,
        color: row.project_color,
      },
    )
    const still = (id) => id && ids.value.has(id)
    const nb = {}
    if (still(neighbors.before_id)) nb.before_id = neighbors.before_id
    if (still(neighbors.after_id)) nb.after_id = neighbors.after_id
    if (nb.before_id || nb.after_id) {
      // The restored row sits at the end, so the neighbours' indexes are the
      // same with or without it.
      const at = (id) => items.value.findIndex((f) => f.board_id === id)
      await move(row.board_id, nb.after_id ? at(nb.after_id) : at(nb.before_id) + 1)
    }
  }

  // move puts a favourite at index `to` of the list as it is now drawn (the
  // index it should have once it is out of its old place counted the same way
  // vuedraggable reports newIndex).
  async function move(boardId, to) {
    const from = items.value.findIndex((f) => f.board_id === boardId)
    if (from < 0) return
    const next = [...items.value]
    const [row] = next.splice(from, 1)
    const at = Math.max(0, Math.min(to, next.length))
    next.splice(at, 0, row)
    if (next.every((f, k) => f === items.value[k])) return
    items.value = next
    seq++
    try {
      const { data } = await favApi.move(boardId, favoriteNeighbors(next, at))
      items.value = items.value.map((f) => (f.board_id === boardId ? data : f))
    } catch (e) {
      await load()
      throw e
    }
  }

  // Socket events (the layout feeds every event here).
  function onEvent(ev) {
    if (!ev?.type || !wsId.value) return
    if (ev.type === 'favorites.changed') {
      if (ev.scope === wsId.value) load()
      return
    }
    if (ev.scope !== wsId.value) return
    const d = ev.data || {}
    if (ev.type === 'board.deleted') {
      if (ids.value.has(d.id)) {
        dropped.add(d.id)
        items.value = items.value.filter((f) => f.board_id !== d.id)
      }
    } else if (ev.type === 'board.created') {
      if (dropped.has(d.id)) {
        dropped.delete(d.id)
        load()
      }
    } else if (ev.type === 'board.updated' || ev.type === 'project.updated') {
      const patched = favoritePatch(items.value, ev.type, d)
      if (patched) items.value = patched
    } else if (ev.type === 'project.deleted') {
      if (items.value.some((f) => f.project_id === d.id))
        items.value = items.value.filter((f) => f.project_id !== d.id)
    } else if (ev.type === 'project.created') {
      // A project transferred in brings the reader's stars on its boards.
      load()
    }
  }

  function reset() {
    seq++
    wsId.value = ''
    items.value = []
    loaded.value = false
    dropped.clear()
  }

  return {
    wsId,
    items,
    loaded,
    ids,
    isFavorite,
    load,
    add,
    remove,
    toggle,
    restore,
    move,
    onEvent,
    reset,
  }
})
