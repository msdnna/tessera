import { projects as projApi, groups as groupsApi } from '@/api'

// Shared vuedraggable @change handlers for the sidebar tree. `list` is the
// mirror array already mutated by vuedraggable; newIndex points into it.
// destId is the destination group_id (projects) / parent_id (groups), null at root.

export async function moveSidebarProject(evt, list, destId, store, message) {
  const info = evt.added || evt.moved
  if (!info) return
  const before = list[info.newIndex - 1]
  const after = list[info.newIndex + 1]
  try {
    await projApi.move(info.element.id, {
      group_id: destId,
      before_id: before ? before.id : null,
      after_id: after ? after.id : null,
    })
  } catch (e) {
    message.error(e.message)
  }
  await store.refresh()
}

export async function moveSidebarGroup(evt, list, destId, store, message) {
  const info = evt.added || evt.moved
  if (!info) return
  // Guard against dropping a group into itself.
  if (destId && info.element.id === destId) {
    await store.refresh()
    return
  }
  const before = list[info.newIndex - 1]
  const after = list[info.newIndex + 1]
  try {
    await groupsApi.move(info.element.id, {
      parent_id: destId,
      before_id: before ? before.id : null,
      after_id: after ? after.id : null,
    })
  } catch (e) {
    message.error(e.message)
  }
  await store.refresh()
}
